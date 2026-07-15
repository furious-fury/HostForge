package railpack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hostforge/hostforge/internal/builder"
	hfdocker "github.com/hostforge/hostforge/internal/docker"
	"github.com/moby/moby/client"
)

// BuildKitConfig identifies the local BuildKit daemon and the immutable
// Railpack frontend used to execute a matching prepared plan.
type BuildKitConfig struct {
	Binary          string
	Address         string
	FrontendImage   string
	RailpackVersion string
}

// imageStore imports the BuildKit docker exporter and confirms its image tag
// exists in the local Docker daemon before reporting success.
type imageStore interface {
	LoadAndVerify(context.Context, io.Reader, string) (string, error)
}

type dockerImageStore struct{}

func (dockerImageStore) LoadAndVerify(ctx context.Context, imageTar io.Reader, imageRef string) (string, error) {
	cli, err := hfdocker.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer cli.Close()
	response, err := cli.ImageLoad(ctx, imageTar, client.ImageLoadWithQuiet(true))
	if err != nil {
		return "", fmt.Errorf("load BuildKit image: %w", err)
	}
	defer response.Close()
	if _, err := io.Copy(io.Discard, response); err != nil {
		return "", fmt.Errorf("read image load response: %w", err)
	}
	inspected, err := cli.ImageInspect(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("inspect imported image %s: %w", imageRef, err)
	}
	if strings.TrimSpace(inspected.ID) == "" {
		return "", fmt.Errorf("inspect imported image %s: empty image id", imageRef)
	}
	return inspected.ID, nil
}

// BuildKitExecutor runs the Railpack frontend through BuildKit directly.
type BuildKitExecutor struct {
	binary          string
	address         string
	frontendImage   string
	railpackVersion string
	runner          commandRunner
	images          imageStore
}

// NewBuildKitExecutor creates a direct BuildKit executor. FrontendImage must
// be digest-pinned; mutable tags are forbidden in the production path.
func NewBuildKitExecutor(config BuildKitConfig) (*BuildKitExecutor, error) {
	binary := strings.TrimSpace(config.Binary)
	if binary == "" {
		binary = "buildctl"
	}
	address := strings.TrimSpace(config.Address)
	if address == "" {
		return nil, errors.New("buildkit address is required")
	}
	frontend := strings.TrimSpace(config.FrontendImage)
	if !strings.Contains(frontend, "@sha256:") {
		return nil, errors.New("buildkit frontend image must be digest-pinned")
	}
	version := normalizeVersion(config.RailpackVersion)
	if version == "" {
		return nil, errors.New("buildkit railpack version is required")
	}
	return &BuildKitExecutor{
		binary:          binary,
		address:         address,
		frontendImage:   frontend,
		railpackVersion: version,
		runner:          execRunner{},
		images:          dockerImageStore{},
	}, nil
}

// Build submits a prepared Railpack plan to the BuildKit frontend, streams the
// Docker exporter directly into the local daemon, and verifies the deployment
// tag before returning a builder.Result.
func (e *BuildKitExecutor) Build(ctx context.Context, request builder.Request, preparation Preparation, stderr io.Writer) (builder.Result, error) {
	if err := ctx.Err(); err != nil {
		return builder.Result{}, err
	}
	if e == nil || e.runner == nil || e.images == nil {
		return builder.Result{}, errors.New("buildkit executor is not configured")
	}
	if err := validateBuildInput(request, preparation, e.railpackVersion); err != nil {
		return builder.Result{}, err
	}
	secretArgs, cleanup, err := materializeBuildSecrets(request.BuildSecrets)
	if err != nil {
		return builder.Result{}, err
	}
	defer cleanup()
	imageID, err := e.solveDockerExport(ctx, request, append(e.buildArgs(request, preparation), secretArgs...), stderr)
	if err != nil {
		return builder.Result{}, err
	}
	result := builder.Result{Kind: builder.KindRailpack, ImageRef: request.ImageRef, ImageID: imageID}
	if err := result.Validate(request); err != nil {
		return builder.Result{}, err
	}
	return result, nil
}

// BuildDockerfile submits a repository-root Dockerfile to BuildKit and uses
// the same streamed Docker export/import path as Railpack frontend builds.
func (e *BuildKitExecutor) BuildDockerfile(ctx context.Context, request builder.Request, stderr io.Writer) (builder.Result, error) {
	if err := ctx.Err(); err != nil {
		return builder.Result{}, err
	}
	if e == nil || e.runner == nil || e.images == nil {
		return builder.Result{}, errors.New("buildkit executor is not configured")
	}
	if err := validateDockerfileBuildInput(request); err != nil {
		return builder.Result{}, err
	}
	secretArgs, cleanup, err := materializeBuildSecrets(request.BuildSecrets)
	if err != nil {
		return builder.Result{}, err
	}
	defer cleanup()
	imageID, err := e.solveDockerExport(ctx, request, append(e.dockerfileBuildArgs(request), secretArgs...), stderr)
	if err != nil {
		return builder.Result{}, err
	}
	result := builder.Result{Kind: builder.KindDockerfile, ImageRef: request.ImageRef, ImageID: imageID}
	if err := result.Validate(request); err != nil {
		return builder.Result{}, err
	}
	return result, nil
}

func (e *BuildKitExecutor) solveDockerExport(ctx context.Context, request builder.Request, args []string, stderr io.Writer) (string, error) {
	if stderr == nil {
		stderr = io.Discard
	}
	reader, writer := io.Pipe()
	buildDone := make(chan error, 1)
	go func() {
		err := e.runner.Run(ctx, e.binary, args, request.Worktree, writer, stderr)
		_ = writer.CloseWithError(err)
		buildDone <- err
	}()

	imageID, importErr := e.images.LoadAndVerify(ctx, reader, request.ImageRef)
	_ = reader.CloseWithError(importErr)
	buildErr := <-buildDone
	if buildErr != nil {
		return "", fmt.Errorf("buildkit solve: %w", buildErr)
	}
	if importErr != nil {
		return "", fmt.Errorf("import BuildKit image: %w", importErr)
	}
	return imageID, nil
}

func (e *BuildKitExecutor) buildArgs(request builder.Request, preparation Preparation) []string {
	args := []string{
		"--addr", e.address,
		"build",
		"--local", "context=" + request.Worktree,
		"--local", "dockerfile=" + filepath.Dir(preparation.PlanPath),
		"--frontend=gateway.v0",
		"--opt", "source=" + e.frontendImage,
		"--opt", "platform=" + request.Platform,
		"--output", "type=docker,name=" + request.ImageRef,
	}
	if strings.TrimSpace(request.CacheKey) != "" {
		args = append(args, "--opt", "cache-key="+request.CacheKey)
	}
	return args
}

func (e *BuildKitExecutor) dockerfileBuildArgs(request builder.Request) []string {
	args := []string{
		"--addr", e.address,
		"build",
		"--local", "context=" + request.Worktree,
		"--local", "dockerfile=" + request.Worktree,
		"--frontend=dockerfile.v0",
		"--opt", "filename=Dockerfile",
		"--opt", "platform=" + request.Platform,
		"--output", "type=docker,name=" + request.ImageRef,
	}
	if strings.TrimSpace(request.CacheKey) != "" {
		args = append(args, "--opt", "cache-key="+request.CacheKey)
	}
	return args
}

func validateBuildInput(request builder.Request, preparation Preparation, expectedVersion string) error {
	if err := builder.RequireWorktree(request); err != nil {
		return err
	}
	if strings.TrimSpace(request.ImageRef) == "" || strings.TrimSpace(request.Platform) == "" {
		return errors.New("buildkit image reference and platform are required")
	}
	if preparation.Version != expectedVersion {
		return fmt.Errorf("prepared railpack version %s does not match BuildKit frontend version %s", preparation.Version, expectedVersion)
	}
	for _, path := range []string{preparation.PlanPath, preparation.InfoPath} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat prepared railpack artifact: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("prepared railpack artifact is invalid")
		}
	}
	return nil
}

func validateDockerfileBuildInput(request builder.Request) error {
	if err := builder.RequireWorktree(request); err != nil {
		return err
	}
	if strings.TrimSpace(request.ImageRef) == "" || strings.TrimSpace(request.Platform) == "" {
		return errors.New("buildkit image reference and platform are required")
	}
	info, err := os.Stat(filepath.Join(request.Worktree, "Dockerfile"))
	if err != nil {
		return fmt.Errorf("stat repository Dockerfile: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("repository Dockerfile must be a regular file")
	}
	return nil
}

func materializeBuildSecrets(secrets map[string]string) ([]string, func(), error) {
	if len(secrets) == 0 {
		return nil, func() {}, nil
	}
	directory, err := os.MkdirTemp("", "hostforge-build-secrets-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create build secret directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	keys := make([]string, 0, len(secrets))
	for key := range secrets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for index, key := range keys {
		path := filepath.Join(directory, fmt.Sprintf("secret-%d", index))
		if err := os.WriteFile(path, []byte(secrets[key]), 0o600); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("write build secret: %w", err)
		}
		args = append(args, "--secret", "id="+key+",src="+path)
	}
	return args, cleanup, nil
}
