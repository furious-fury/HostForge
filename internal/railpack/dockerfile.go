package railpack

import (
	"context"
	"errors"

	"github.com/hostforge/hostforge/internal/builder"
)

// DockerfileBuilder implements the fallback branch of the builder contract
// with BuildKit's standard Dockerfile frontend.
type DockerfileBuilder struct {
	executor *BuildKitExecutor
}

func NewDockerfileBuilder(executor *BuildKitExecutor) (*DockerfileBuilder, error) {
	if executor == nil {
		return nil, errors.New("BuildKit executor is required for Dockerfile builder")
	}
	return &DockerfileBuilder{executor: executor}, nil
}

func (*DockerfileBuilder) Kind() builder.Kind { return builder.KindDockerfile }

func (b *DockerfileBuilder) Build(ctx context.Context, request builder.Request, sink builder.EventSink) (builder.Result, error) {
	if b == nil || b.executor == nil {
		return builder.Result{}, errors.New("Dockerfile builder is not configured")
	}
	if sink != nil {
		sink(builder.Event{Phase: "build", Message: "Building repository Dockerfile with BuildKit."})
	}
	logs := eventWriter{phase: "build", sink: sink}
	return b.executor.BuildDockerfile(ctx, request, &logs)
}

var _ builder.Builder = (*DockerfileBuilder)(nil)
