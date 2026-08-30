package builder

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Kind identifies the implementation that produced a build.
type Kind string

const (
	KindRailpack   Kind = "railpack"
	KindDockerfile Kind = "dockerfile"
)

var (
	// ErrUnsupported tells Selection that the source cannot be built by the
	// attempted builder. It is the only error that permits Dockerfile fallback.
	ErrUnsupported = errors.New("builder unsupported")
	// ErrImageNotImported prevents callers from starting a container until the
	// builder has confirmed the result exists in the local Docker image store.
	ErrImageNotImported = errors.New("builder image not imported")
)

// Request describes one isolated source build. Worktree must refer to the
// checked-out deployment source and ImageRef must be immutable for the
// deployment. Future adapters may use CacheKey and Platform for BuildKit.
type Request struct {
	Worktree     string
	ImageRef     string
	Platform     string
	CacheKey     string
	Runtime      string
	InstallCmd   string
	BuildCmd     string
	StartCmd     string
	BuildSecrets map[string]string
}

// Event is a structured, already-redacted builder event suitable for the
// existing deployment log service.
type Event struct {
	Phase   string
	Message string
}

// EventSink receives build events. Adapters must never send credentials or
// secret values through this interface.
type EventSink func(Event)

// Result is returned only after the built image has been imported into the
// local Docker runtime store and inspected successfully.
type Result struct {
	Kind       Kind
	ImageRef   string
	ImageID    string
	StackKind  string
	StackLabel string
	// PlanJSON and InfoJSON are the raw Railpack build plan/info, captured
	// for provenance (ADR-0002 §15.6/§15.7). Empty for a Dockerfile build,
	// or when the source file exceeded the persistence size cap.
	PlanJSON string
	InfoJSON string
}

// Builder turns an isolated source worktree into a locally runnable image.
// Build must obey ctx cancellation and must not return a successful Result
// until ImageRef and ImageID identify an imported local image.
type Builder interface {
	Kind() Kind
	Build(context.Context, Request, EventSink) (Result, error)
}

// Validate checks the invariants the deployment service needs before it can
// create a container from a successful build result.
func (r Result) Validate(request Request) error {
	if strings.TrimSpace(r.ImageRef) == "" || strings.TrimSpace(r.ImageID) == "" {
		return ErrImageNotImported
	}
	if strings.TrimSpace(request.ImageRef) != "" && r.ImageRef != request.ImageRef {
		return fmt.Errorf("builder returned image ref %q, expected %q", r.ImageRef, request.ImageRef)
	}
	if r.Kind == "" {
		return errors.New("builder result kind is required")
	}
	return nil
}
