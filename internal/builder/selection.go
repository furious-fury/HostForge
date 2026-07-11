package builder

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Selection runs Railpack by default and permits Dockerfile fallback only when
// Railpack reports ErrUnsupported and a regular Dockerfile exists at the source
// root. It is intentionally not wired into the active deployment pipeline yet.
type Selection struct {
	Railpack   Builder
	Dockerfile Builder
}

// Build executes the selected builder. Non-unsupported Railpack failures are
// returned directly: retrying a failed build with a Dockerfile can hide build
// errors and violates the deliberate fallback policy.
func (s Selection) Build(ctx context.Context, request Request, sink EventSink) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if s.Railpack == nil {
		return Result{}, errors.New("railpack builder is required")
	}

	result, err := s.Railpack.Build(ctx, request, sink)
	if err == nil {
		return validateResult(result, request)
	}
	if !errors.Is(err, ErrUnsupported) {
		return Result{}, err
	}
	if !hasDockerfile(request.Worktree) {
		return Result{}, err
	}
	if s.Dockerfile == nil {
		return Result{}, errors.New("dockerfile fallback builder is required")
	}
	if sink != nil {
		sink(Event{Phase: "builder", Message: "Railpack unsupported; using repository Dockerfile fallback."})
	}
	result, err = s.Dockerfile.Build(ctx, request, sink)
	if err != nil {
		return Result{}, err
	}
	return validateResult(result, request)
}

func validateResult(result Result, request Request) (Result, error) {
	if err := result.Validate(request); err != nil {
		return Result{}, err
	}
	return result, nil
}

func hasDockerfile(worktree string) bool {
	if worktree == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(worktree, "Dockerfile"))
	return err == nil && info.Mode().IsRegular()
}

// HasDockerfile is exposed for UI/API validation and tests without giving
// callers a reason to duplicate the repository-root eligibility rule.
func HasDockerfile(worktree string) bool {
	return hasDockerfile(worktree)
}

// IsNotExist reports whether a Dockerfile inspection failed because the source
// path did not exist. It is retained here for future adapters that need to
// distinguish an unavailable worktree from an unsupported source.
func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// RequireWorktree makes the invalid request error stable for future API/worker
// callers without imposing filesystem checks on every builder implementation.
func RequireWorktree(request Request) error {
	if request.Worktree == "" {
		return fmt.Errorf("builder worktree is required")
	}
	return nil
}
