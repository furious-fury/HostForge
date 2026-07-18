package railpack

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/builder"
)

type fakeImageStore struct {
	imageID string
	err     error
	loaded  string
	body    []byte
}

func (f *fakeImageStore) LoadAndVerify(_ context.Context, imageTar io.Reader, imageRef string) (string, error) {
	f.loaded = imageRef
	f.body, _ = io.ReadAll(imageTar)
	return f.imageID, f.err
}

func preparedBuildInput(t *testing.T) (builder.Request, Preparation) {
	t.Helper()
	worktree := t.TempDir()
	artifacts := t.TempDir()
	plan := filepath.Join(artifacts, "railpack-plan.json")
	info := filepath.Join(artifacts, "railpack-info.json")
	if err := os.WriteFile(plan, []byte(`{"steps":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(info, []byte(`{"providers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return builder.Request{Worktree: worktree, ImageRef: "hostforge/example:build-1", Platform: "linux/amd64", CacheKey: "project-1"}, Preparation{Version: DefaultVersion, PlanPath: plan, InfoPath: info}
}

func testExecutor(t *testing.T, runner *fakeRunner, images *fakeImageStore) *BuildKitExecutor {
	t.Helper()
	executor, err := NewBuildKitExecutor(BuildKitConfig{
		Binary:          "buildctl-test",
		Address:         "unix:///run/buildkit/buildkitd.sock",
		FrontendImage:   "ghcr.io/railwayapp/railpack-frontend@sha256:abcdef",
		RailpackVersion: DefaultVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	executor.runner = runner
	executor.images = images
	return executor
}

func TestBuildKitExecutor_BuildsAndVerifiesImportedImage(t *testing.T) {
	t.Parallel()
	request, preparation := preparedBuildInput(t)
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}
	images := &fakeImageStore{imageID: "sha256:abc"}

	result, err := testExecutor(t, runner, images).Build(context.Background(), request, preparation, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != builder.KindRailpack || result.ImageID != "sha256:abc" || images.loaded != request.ImageRef || string(images.body) != "docker image tar" {
		t.Fatalf("unexpected result=%+v store=%+v", result, images)
	}
	args := runner.calls[0].args
	if !reflect.DeepEqual(args[:4], []string{"--addr", "unix:///run/buildkit/buildkitd.sock", "build", "--local"}) || !strings.Contains(strings.Join(args, " "), "source=ghcr.io/railwayapp/railpack-frontend@sha256:abcdef") || !strings.Contains(strings.Join(args, " "), "cache-key=project-1") {
		t.Fatalf("unexpected buildctl args: %#v", args)
	}
}

func TestBuildKitExecutor_FailsWhenImageImportFails(t *testing.T) {
	t.Parallel()
	request, preparation := preparedBuildInput(t)
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}
	want := errors.New("docker unavailable")
	_, err := testExecutor(t, runner, &fakeImageStore{err: want}).Build(context.Background(), request, preparation, nil)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want import error", err)
	}
}

func TestBuildKitExecutor_BuildsRepositoryDockerfile(t *testing.T) {
	t.Parallel()
	request, _ := preparedBuildInput(t)
	request.BuildSecrets = map[string]string{"TOKEN": "dockerfile-secret"}
	if err := os.WriteFile(filepath.Join(request.Worktree, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var secretPath string
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		for index, arg := range args {
			if arg == "--secret" && index+1 < len(args) {
				parts := strings.SplitN(args[index+1], ",src=", 2)
				if len(parts) == 2 && parts[0] == "id=TOKEN" {
					secretPath = parts[1]
				}
			}
		}
		if value, err := os.ReadFile(secretPath); err != nil || string(value) != request.BuildSecrets["TOKEN"] {
			t.Fatalf("Dockerfile secret unavailable during solve: value=%q err=%v", value, err)
		}
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}
	result, err := testExecutor(t, runner, &fakeImageStore{imageID: "sha256:dockerfile"}).BuildDockerfile(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != builder.KindDockerfile || result.ImageID != "sha256:dockerfile" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(secretPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Dockerfile secret remains after build: %v", err)
	}
	args := strings.Join(runner.calls[0].args, " ")
	if !strings.Contains(args, "--frontend=dockerfile.v0") || !strings.Contains(args, "filename=Dockerfile") {
		t.Fatalf("unexpected BuildKit Dockerfile args: %s", args)
	}
}

func TestBuildKitExecutor_RejectsMissingRepositoryDockerfile(t *testing.T) {
	t.Parallel()
	request, _ := preparedBuildInput(t)
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, _, _ io.Writer) error {
		t.Fatal("runner must not be called")
		return nil
	}}
	_, err := testExecutor(t, runner, &fakeImageStore{}).BuildDockerfile(context.Background(), request, nil)
	if err == nil || len(runner.calls) != 0 {
		t.Fatalf("got err=%v calls=%d", err, len(runner.calls))
	}
}

func TestBuildKitExecutor_FailsWhenSolveFails(t *testing.T) {
	t.Parallel()
	request, preparation := preparedBuildInput(t)
	want := errors.New("buildkit failed")
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "partial tar")
		return want
	}}
	_, err := testExecutor(t, runner, &fakeImageStore{imageID: "sha256:abc"}).Build(context.Background(), request, preparation, nil)
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want solve error", err)
	}
}

func TestNewBuildKitExecutor_RequiresPinnedFrontend(t *testing.T) {
	t.Parallel()
	_, err := NewBuildKitExecutor(BuildKitConfig{Address: "unix:///run/buildkit/buildkitd.sock", FrontendImage: "ghcr.io/railwayapp/railpack-frontend:latest", RailpackVersion: DefaultVersion})
	if err == nil {
		t.Fatal("expected digest pin error")
	}
}

func TestBuildKitExecutor_RejectsMismatchedPreparation(t *testing.T) {
	t.Parallel()
	request, preparation := preparedBuildInput(t)
	preparation.Version = "v0.22.0"
	runner := &fakeRunner{run: func(_ string, _ []string, _ string, _, _ io.Writer) error {
		t.Fatal("runner must not be called")
		return nil
	}}
	_, err := testExecutor(t, runner, &fakeImageStore{}).Build(context.Background(), request, preparation, bytes.NewBuffer(nil))
	if err == nil || len(runner.calls) != 0 {
		t.Fatalf("got err=%v calls=%d", err, len(runner.calls))
	}
}

func TestMaterializeBuildSecretsUsesPrivateFilesAndCleansUp(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	args, cleanup, err := materializeBuildSecrets(root, map[string]string{"TOKEN": "super-secret", "DATABASE_URL": "postgres://db"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 || args[0] != "--secret" || args[2] != "--secret" {
		t.Fatalf("unexpected secret args: %#v", args)
	}
	var paths []string
	for _, value := range []string{args[1], args[3]} {
		parts := strings.SplitN(value, ",src=", 2)
		if len(parts) != 2 || strings.Contains(value, "super-secret") || strings.Contains(value, "postgres://db") {
			t.Fatalf("secret leaked into arguments: %q", value)
		}
		info, err := os.Stat(parts[1])
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("secret mode=%o", info.Mode().Perm())
		}
		if filepath.Dir(filepath.Dir(parts[1])) != root {
			t.Fatalf("secret path %q is not rooted beneath %q", parts[1], root)
		}
		paths = append(paths, parts[1])
	}
	directory := filepath.Dir(paths[0])
	if info, err := os.Stat(directory); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o700) {
		t.Fatalf("secret directory mode: info=%v err=%v", info, err)
	}
	cleanup()
	for _, path := range paths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("secret file remains at %s: %v", path, err)
		}
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret directory remains at %s: %v", directory, err)
	}
}

func TestBuildKitExecutor_KeepsSecretsVisibleForRailpackSolveAndCleansUp(t *testing.T) {
	t.Parallel()
	request, preparation := preparedBuildInput(t)
	request.BuildSecrets = map[string]string{"DATABASE_URL": "postgres://private-db"}
	var secretPath string
	runner := &fakeRunner{run: func(_ string, args []string, _ string, stdout, _ io.Writer) error {
		for index, arg := range args {
			if arg != "--secret" || index+1 >= len(args) {
				continue
			}
			parts := strings.SplitN(args[index+1], ",src=", 2)
			if len(parts) == 2 && strings.HasPrefix(parts[0], "id=DATABASE_URL") {
				secretPath = parts[1]
			}
		}
		if secretPath == "" {
			t.Fatal("DATABASE_URL secret argument was not supplied")
		}
		value, err := os.ReadFile(secretPath)
		if err != nil {
			t.Fatalf("secret unavailable while buildctl is running: %v", err)
		}
		if string(value) != request.BuildSecrets["DATABASE_URL"] {
			t.Fatal("secret content changed before buildctl consumed it")
		}
		if filepath.Dir(filepath.Dir(secretPath)) != filepath.Dir(request.Worktree) {
			t.Fatalf("secret path %q is not beside managed worktree %q", secretPath, request.Worktree)
		}
		_, _ = io.WriteString(stdout, "docker image tar")
		return nil
	}}

	_, err := testExecutor(t, runner, &fakeImageStore{imageID: "sha256:abc"}).Build(context.Background(), request, preparation, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secretPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret remains after build: %v", err)
	}
}
