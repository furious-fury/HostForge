package railpack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hostforge/hostforge/internal/builder"
)

// AdapterConfig wires a pinned Railpack prepare helper to its matching direct
// BuildKit executor. ArtifactsRoot is platform-owned storage, not a repository
// path, and is removed after each terminal build attempt.
type AdapterConfig struct {
	Planner       *Planner
	Executor      *BuildKitExecutor
	ArtifactsRoot string
}

// Adapter implements builder.Builder with the Railpack prepare plus BuildKit
// frontend flow selected by ADR 0001.
type Adapter struct {
	planner       *Planner
	executor      *BuildKitExecutor
	artifactsRoot string
}

// NewAdapter validates the components without touching the active deployment
// path. Callers can construct and contract-test it before rollout.
func NewAdapter(config AdapterConfig) (*Adapter, error) {
	if config.Planner == nil {
		return nil, errors.New("railpack planner is required")
	}
	if config.Executor == nil {
		return nil, errors.New("railpack BuildKit executor is required")
	}
	root := strings.TrimSpace(config.ArtifactsRoot)
	if root == "" {
		return nil, errors.New("railpack artifacts root is required")
	}
	return &Adapter{planner: config.Planner, executor: config.Executor, artifactsRoot: root}, nil
}

// Kind implements builder.Builder.
func (*Adapter) Kind() builder.Kind { return builder.KindRailpack }

// Build prepares an isolated source tree, executes it with the Railpack
// BuildKit frontend, and removes temporary plan artifacts once the terminal
// result is known. It never starts a container.
func (a *Adapter) Build(ctx context.Context, request builder.Request, sink builder.EventSink) (builder.Result, error) {
	if err := ctx.Err(); err != nil {
		return builder.Result{}, err
	}
	if a == nil || a.planner == nil || a.executor == nil {
		return builder.Result{}, errors.New("railpack adapter is not configured")
	}
	if err := builder.RequireWorktree(request); err != nil {
		return builder.Result{}, err
	}
	if err := os.MkdirAll(a.artifactsRoot, 0o700); err != nil {
		return builder.Result{}, fmt.Errorf("create railpack artifacts root: %w", err)
	}
	artifactsDir, err := os.MkdirTemp(a.artifactsRoot, "deployment-")
	if err != nil {
		return builder.Result{}, fmt.Errorf("create railpack deployment artifacts: %w", err)
	}
	defer os.RemoveAll(artifactsDir)

	if sink != nil {
		sink(builder.Event{Phase: "prepare", Message: "Preparing Railpack build plan."})
	}
	planLogs := eventWriter{phase: "prepare", sink: sink}
	preparation, err := a.planner.Prepare(ctx, PrepareRequest{Worktree: request.Worktree, ArtifactsDir: artifactsDir}, &planLogs, &planLogs)
	if err != nil {
		return builder.Result{}, err
	}
	if sink != nil {
		sink(builder.Event{Phase: "build", Message: "Building image with Railpack BuildKit frontend."})
	}
	buildLogs := eventWriter{phase: "build", sink: sink}
	return a.executor.Build(ctx, request, preparation, &buildLogs)
}

// eventWriter converts external tool output into builder events. Both helpers
// receive no credential or secret values, so this adapter never emits those
// values through the builder event stream.
type eventWriter struct {
	phase string
	sink  builder.EventSink
}

func (w *eventWriter) Write(data []byte) (int, error) {
	if w == nil || w.sink == nil || len(data) == 0 {
		return len(data), nil
	}
	message := strings.TrimSpace(string(data))
	if message != "" {
		w.sink(builder.Event{Phase: w.phase, Message: message})
	}
	return len(data), nil
}

var _ builder.Builder = (*Adapter)(nil)
var _ io.Writer = (*eventWriter)(nil)
