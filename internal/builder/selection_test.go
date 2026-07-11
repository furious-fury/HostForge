package builder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeBuilder struct {
	kind   Kind
	result Result
	err    error
	calls  int
}

func (f *fakeBuilder) Kind() Kind { return f.kind }

func (f *fakeBuilder) Build(ctx context.Context, _ Request, _ EventSink) (Result, error) {
	f.calls++
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return f.result, f.err
}

func requestFor(t *testing.T, dockerfile bool) Request {
	t.Helper()
	worktree := t.TempDir()
	if dockerfile {
		if err := os.WriteFile(filepath.Join(worktree, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Request{Worktree: worktree, ImageRef: "hostforge/example:build-1", Platform: "linux/amd64", CacheKey: "project-1"}
}

func validResult(kind Kind, request Request) Result {
	return Result{Kind: kind, ImageRef: request.ImageRef, ImageID: "sha256:abc"}
}

func TestSelection_UsesRailpackByDefault(t *testing.T) {
	t.Parallel()
	request := requestFor(t, true)
	railpack := &fakeBuilder{kind: KindRailpack, result: validResult(KindRailpack, request)}
	dockerfile := &fakeBuilder{kind: KindDockerfile, result: validResult(KindDockerfile, request)}

	result, err := (Selection{Railpack: railpack, Dockerfile: dockerfile}).Build(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != KindRailpack || railpack.calls != 1 || dockerfile.calls != 0 {
		t.Fatalf("unexpected selection: result=%+v railpack=%d dockerfile=%d", result, railpack.calls, dockerfile.calls)
	}
}

func TestSelection_FallsBackOnlyWhenRailpackUnsupportedAndDockerfileExists(t *testing.T) {
	t.Parallel()
	request := requestFor(t, true)
	railpack := &fakeBuilder{kind: KindRailpack, err: ErrUnsupported}
	dockerfile := &fakeBuilder{kind: KindDockerfile, result: validResult(KindDockerfile, request)}

	result, err := (Selection{Railpack: railpack, Dockerfile: dockerfile}).Build(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != KindDockerfile || railpack.calls != 1 || dockerfile.calls != 1 {
		t.Fatalf("unexpected fallback: result=%+v railpack=%d dockerfile=%d", result, railpack.calls, dockerfile.calls)
	}
}

func TestSelection_DoesNotFallbackForBuildFailure(t *testing.T) {
	t.Parallel()
	request := requestFor(t, true)
	want := errors.New("build failed")
	railpack := &fakeBuilder{kind: KindRailpack, err: want}
	dockerfile := &fakeBuilder{kind: KindDockerfile, result: validResult(KindDockerfile, request)}

	_, err := (Selection{Railpack: railpack, Dockerfile: dockerfile}).Build(context.Background(), request, nil)
	if !errors.Is(err, want) || dockerfile.calls != 0 {
		t.Fatalf("got err=%v dockerfile calls=%d", err, dockerfile.calls)
	}
}

func TestSelection_CancelledRequestDoesNotStartBuilder(t *testing.T) {
	t.Parallel()
	request := requestFor(t, false)
	railpack := &fakeBuilder{kind: KindRailpack, result: validResult(KindRailpack, request)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (Selection{Railpack: railpack}).Build(ctx, request, nil)
	if !errors.Is(err, context.Canceled) || railpack.calls != 0 {
		t.Fatalf("got err=%v calls=%d", err, railpack.calls)
	}
}

func TestSelection_RejectsResultWithoutImportedImage(t *testing.T) {
	t.Parallel()
	request := requestFor(t, false)
	railpack := &fakeBuilder{kind: KindRailpack, result: Result{Kind: KindRailpack, ImageRef: request.ImageRef}}

	_, err := (Selection{Railpack: railpack}).Build(context.Background(), request, nil)
	if !errors.Is(err, ErrImageNotImported) {
		t.Fatalf("got %v, want ErrImageNotImported", err)
	}
}
