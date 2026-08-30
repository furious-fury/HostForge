package railpack

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/builder"
)

type runCall struct {
	name string
	args []string
	dir  string
}

type fakeRunner struct {
	calls []runCall
	run   func(string, []string, string, io.Writer, io.Writer) error
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, dir string, stdout, stderr io.Writer) error {
	f.calls = append(f.calls, runCall{name: name, args: append([]string(nil), args...), dir: dir})
	return f.run(name, args, dir, stdout, stderr)
}

func newTestPlanner(t *testing.T, runner *fakeRunner) *Planner {
	t.Helper()
	planner, err := NewPlanner("railpack-test", DefaultVersion)
	if err != nil {
		t.Fatal(err)
	}
	planner.runner = runner
	return planner
}

func TestPrepare_WritesPlanAndInfoOutsideWorktree(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		planPath, infoPath := args[3], args[5]
		if err := os.WriteFile(planPath, []byte(`{"steps":{}}`), 0o600); err != nil {
			return err
		}
		return os.WriteFile(infoPath, []byte(`{"detectedProviders":["node"]}`), 0o600)
	}}

	preparation, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: artifacts}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if preparation.Version != DefaultVersion || filepath.Dir(preparation.PlanPath) != artifacts || filepath.Dir(preparation.InfoPath) != artifacts {
		t.Fatalf("unexpected preparation: %+v", preparation)
	}
	if preparation.StackKind != "node" || preparation.StackLabel != "Node.js" {
		t.Fatalf("unexpected stack metadata: %+v", preparation)
	}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[1].args[:2], []string{"prepare", worktree}) {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestStackFromInfoJSON_PrefersRecognisedLanguageProvider(t *testing.T) {
	t.Parallel()
	kind, label := StackFromInfoJSON([]byte(`{"detectedProviders":["npm", "Python"]}`))
	if kind != "python" || label != "Python" {
		t.Fatalf("got kind=%q label=%q", kind, label)
	}
}

func TestStackFromInfoJSON_UsesGenericFallbackForUnknownInfo(t *testing.T) {
	t.Parallel()
	kind, label := StackFromInfoJSON([]byte(`{"detectedProviders":["npm"]}`))
	if kind != "unknown" || label != "Unknown" {
		t.Fatalf("got kind=%q label=%q", kind, label)
	}
}

func TestStackFromInfoPathAndWorktree_RefinesViteProject(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, "package.json"), []byte(`{"devDependencies":{"vite":"^5.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	infoPath := filepath.Join(t.TempDir(), "railpack-info.json")
	if err := os.WriteFile(infoPath, []byte(`{"detectedProviders":["node"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	kind, label := StackFromInfoPathAndWorktree(infoPath, worktree)
	if kind != "node_vite" || label != "Node.js · Vite" {
		t.Fatalf("got kind=%q label=%q", kind, label)
	}
}

func TestPrepare_RejectsVersionMismatchBeforePrepare(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.22.0\n")
		}
		return nil
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: t.TempDir(), ArtifactsDir: t.TempDir()}, nil, nil)
	if err == nil || len(runner.calls) != 1 {
		t.Fatalf("got err=%v calls=%+v", err, runner.calls)
	}
}

func TestPrepare_ClassifiesUnsupportedSource(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, stderr io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		_, _ = io.WriteString(stderr, "No providers found for this repository\n")
		return errors.New("exit status 1")
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: t.TempDir(), ArtifactsDir: t.TempDir()}, nil, nil)
	if !errors.Is(err, builder.ErrUnsupported) {
		t.Fatalf("got %v, want unsupported", err)
	}
}

func TestPrepare_RejectsArtifactsInsideWorktree(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, _, _ io.Writer) error {
		t.Fatal("runner must not be called")
		return nil
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: filepath.Join(worktree, ".hostforge")}, nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPrepare_PassesServiceCommandOverrides(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		if err := os.WriteFile(args[3], []byte(`{"steps":{}}`), 0o600); err != nil {
			return err
		}
		return os.WriteFile(args[5], []byte(`{"detectedProviders":["node"]}`), 0o600)
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{
		Worktree: worktree, ArtifactsDir: artifacts, Runtime: "bun",
		InstallCmd: "bun install --frozen-lockfile", BuildCmd: "bun run build", StartCmd: "bun run start",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := runner.calls[1].args
	wantTail := []string{"--env", "RAILPACK_INSTALL_CMD=bun install --frozen-lockfile", "--build-cmd", "bun run build", "--start-cmd", "bun run start", "--env", "RAILPACK_PACKAGES=bun"}
	if !reflect.DeepEqual(got[len(got)-len(wantTail):], wantTail) {
		t.Fatalf("override args=%q want suffix=%q", got, wantTail)
	}
}

func TestPrepare_UsesRedactedEnvironmentPlaceholders(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		if strings.Contains(strings.Join(args, " "), "actual-secret") {
			t.Fatal("secret value reached planner arguments")
		}
		if err := os.WriteFile(args[3], []byte(`{"steps":{}}`), 0o600); err != nil {
			return err
		}
		return os.WriteFile(args[5], []byte(`{"detectedProviders":["node"]}`), 0o600)
	}}
	_, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{
		Worktree: worktree, ArtifactsDir: artifacts, EnvironmentKeys: []string{"DATABASE_URL", "TOKEN"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls[1].args, " ")
	for _, expected := range []string{"DATABASE_URL=__HOSTFORGE_BUILD_SECRET__", "TOKEN=__HOSTFORGE_BUILD_SECRET__"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %s in %s", expected, joined)
		}
	}
}

// PlanJSON/InfoJSON exist because Adapter.Build deletes the artifacts
// directory before returning to any caller — the content has to be
// captured here, inside Prepare, or it's gone (ADR-0002 §15.6/§15.7).
func TestPrepare_CapturesPlanAndInfoJSON(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	const planBody = `{"steps":{},"deploy":{"startCommand":"npm start"}}`
	const infoBody = `{"detectedProviders":["node"],"resolvedPackages":{"node":{"resolvedVersion":"20.11.0"}}}`
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		planPath, infoPath := args[3], args[5]
		if err := os.WriteFile(planPath, []byte(planBody), 0o600); err != nil {
			return err
		}
		return os.WriteFile(infoPath, []byte(infoBody), 0o600)
	}}

	preparation, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: artifacts}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Byte equality, not a parsed comparison: what's persisted must be
	// exactly what railpack wrote, not a re-serialization of it.
	if preparation.PlanJSON != planBody {
		t.Errorf("PlanJSON = %q, want %q", preparation.PlanJSON, planBody)
	}
	if preparation.InfoJSON != infoBody {
		t.Errorf("InfoJSON = %q, want %q", preparation.InfoJSON, infoBody)
	}
	if preparation.StackKind != "node" {
		t.Errorf("StackKind = %q, want node (must still work from the same read used for InfoJSON)", preparation.StackKind)
	}
}

// Oversize provenance must never fail a build or corrupt stack detection —
// it's captured on a best-effort basis. Truncating instead of omitting
// would be worse than omitting: truncated JSON is unparseable by anything
// that reads the stored column later.
func TestPrepare_OmitsOversizePlanJSON(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	oversizePlan := strings.Repeat("a", maxProvenanceJSONBytes+1)
	const infoBody = `{"detectedProviders":["go"]}`
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		if reflect.DeepEqual(args, []string{"--version"}) {
			_, _ = io.WriteString(stdout, "railpack 0.23.0\n")
			return nil
		}
		planPath, infoPath := args[3], args[5]
		if err := os.WriteFile(planPath, []byte(oversizePlan), 0o600); err != nil {
			return err
		}
		return os.WriteFile(infoPath, []byte(infoBody), 0o600)
	}}

	preparation, err := newTestPlanner(t, runner).Prepare(context.Background(), PrepareRequest{Worktree: worktree, ArtifactsDir: artifacts}, nil, nil)
	if err != nil {
		t.Fatalf("oversize provenance must not fail the build: %v", err)
	}
	if preparation.PlanJSON != "" {
		t.Errorf("PlanJSON = %d bytes, want omitted (empty), not truncated", len(preparation.PlanJSON))
	}
	if preparation.InfoJSON != infoBody {
		t.Errorf("InfoJSON = %q, want %q (under-cap file must still capture)", preparation.InfoJSON, infoBody)
	}
	if preparation.StackKind != "go" {
		t.Errorf("StackKind = %q, want go — oversize plan must not degrade stack detection", preparation.StackKind)
	}
}
