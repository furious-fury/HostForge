package railpack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hostforge/hostforge/internal/builder"
)

// DefaultVersion is the initial Railpack release accepted by ADR 0001. The
// matching frontend image digest will be pinned when BuildKit execution is
// implemented; prepare refuses a helper reporting another release.
const DefaultVersion = "v0.23.0"

// PrepareRequest describes where a plan is generated. ArtifactsDir must be
// outside Worktree so generated plan/info files never become source context.
type PrepareRequest struct {
	Worktree     string
	ArtifactsDir string
}

// Preparation is the non-secret metadata a future BuildKit adapter needs to
// invoke the Railpack frontend and persist build provenance.
type Preparation struct {
	Version  string
	PlanPath string
	InfoPath string
}

// commandRunner exists to keep CLI execution isolated and fully testable.
type commandRunner interface {
	Run(context.Context, string, []string, string, io.Writer, io.Writer) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, dir string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Planner invokes only Railpack's plan-generation helper. Production image
// builds are deliberately left to the BuildKit frontend in a later adapter.
type Planner struct {
	binary          string
	expectedVersion string
	runner          commandRunner
}

// NewPlanner creates a pinned Railpack prepare helper. binary may be an
// operator-installed absolute path or a PATH-resolved command; expectedVersion
// must be an explicit Railpack release such as v0.23.0.
func NewPlanner(binary, expectedVersion string) (*Planner, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "railpack"
	}
	expectedVersion = normalizeVersion(expectedVersion)
	if expectedVersion == "" {
		return nil, errors.New("railpack expected version is required")
	}
	return &Planner{binary: binary, expectedVersion: expectedVersion, runner: execRunner{}}, nil
}

// Prepare verifies the helper release, creates the requested artifacts outside
// the source tree, and runs `railpack prepare`. No environment variables or
// credentials are supplied here; build secrets will use BuildKit secret mounts
// in the execution adapter.
func (p *Planner) Prepare(ctx context.Context, request PrepareRequest, stdout, stderr io.Writer) (Preparation, error) {
	if err := ctx.Err(); err != nil {
		return Preparation{}, err
	}
	if p == nil || p.runner == nil {
		return Preparation{}, errors.New("railpack planner is not configured")
	}
	if err := validateRequest(request); err != nil {
		return Preparation{}, err
	}
	if err := p.verifyVersion(ctx, stdout, stderr); err != nil {
		return Preparation{}, err
	}
	if err := os.MkdirAll(request.ArtifactsDir, 0o700); err != nil {
		return Preparation{}, fmt.Errorf("create railpack artifacts directory: %w", err)
	}

	planPath := filepath.Join(request.ArtifactsDir, "railpack-plan.json")
	infoPath := filepath.Join(request.ArtifactsDir, "railpack-info.json")
	args := []string{"prepare", request.Worktree, "--plan-out", planPath, "--info-out", infoPath}
	var output bytes.Buffer
	prepareOut := io.MultiWriter(&output, writerOrDiscard(stdout))
	prepareErr := io.MultiWriter(&output, writerOrDiscard(stderr))
	if err := p.runner.Run(ctx, p.binary, args, request.Worktree, prepareOut, prepareErr); err != nil {
		return Preparation{}, classifyPrepareError(err, output.String())
	}
	for _, path := range []string{planPath, infoPath} {
		info, err := os.Stat(path)
		if err != nil {
			return Preparation{}, fmt.Errorf("railpack prepare did not write %s: %w", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return Preparation{}, fmt.Errorf("railpack prepare wrote invalid %s", filepath.Base(path))
		}
	}
	return Preparation{Version: p.expectedVersion, PlanPath: planPath, InfoPath: infoPath}, nil
}

func (p *Planner) verifyVersion(ctx context.Context, stdout, stderr io.Writer) error {
	var output bytes.Buffer
	out := io.MultiWriter(&output, writerOrDiscard(stdout))
	errOut := io.MultiWriter(&output, writerOrDiscard(stderr))
	if err := p.runner.Run(ctx, p.binary, []string{"--version"}, "", out, errOut); err != nil {
		return fmt.Errorf("run railpack --version: %w", err)
	}
	if !versionMatches(output.String(), p.expectedVersion) {
		return fmt.Errorf("railpack version mismatch: expected %s", p.expectedVersion)
	}
	return nil
}

func validateRequest(request PrepareRequest) error {
	worktree := strings.TrimSpace(request.Worktree)
	artifacts := strings.TrimSpace(request.ArtifactsDir)
	if worktree == "" || artifacts == "" {
		return errors.New("railpack worktree and artifacts directory are required")
	}
	worktreeInfo, err := os.Stat(worktree)
	if err != nil {
		return fmt.Errorf("stat railpack worktree: %w", err)
	}
	if !worktreeInfo.IsDir() {
		return errors.New("railpack worktree must be a directory")
	}
	worktreeAbs, err := filepath.Abs(worktree)
	if err != nil {
		return fmt.Errorf("resolve railpack worktree: %w", err)
	}
	artifactsAbs, err := filepath.Abs(artifacts)
	if err != nil {
		return fmt.Errorf("resolve railpack artifacts directory: %w", err)
	}
	rel, err := filepath.Rel(worktreeAbs, artifactsAbs)
	if err != nil {
		return fmt.Errorf("compare railpack paths: %w", err)
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return errors.New("railpack artifacts directory must be outside the worktree")
	}
	return nil
}

func classifyPrepareError(err error, output string) error {
	lower := strings.ToLower(output)
	for _, marker := range []string{"no providers found", "could not detect a provider", "unsupported project"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("railpack prepare: %w: %v", builder.ErrUnsupported, err)
		}
	}
	return fmt.Errorf("railpack prepare: %w", err)
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}

func versionMatches(output, expected string) bool {
	output = strings.ToLower(output)
	expected = normalizeVersion(expected)
	return strings.Contains(output, expected) || strings.Contains(output, strings.TrimPrefix(expected, "v"))
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}
