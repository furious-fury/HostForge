package workers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/repository"
)

// Operation is the unit of work handed to a Handler. It is
// repository.Operation narrowed to what a handler legitimately needs: the
// queueing columns a handler must not reason about (lease, attempt,
// priority, schedule) are the runtime's business.
type Operation = repository.Operation

// Handler executes one kind of operation.
//
// Handle runs on a context that is cancelled when the operation's lease is
// lost or its cancellation is requested — never merely because the process
// is shutting down, since a claimed operation owns real resources and must
// be drained rather than interrupted. A handler that returns
// context.Canceled is recorded as cancelled; any other error is a failure.
type Handler interface {
	Kind() string
	Handle(ctx context.Context, op Operation) error
}

// ReadinessChecker is an optional Handler capability, consulted after the
// operation is claimed and before it is dispatched.
//
// It exists because the claim query must stay generic. The database claim it
// replaces carried domain predicates in SQL — hold a restore until its
// target is healthy and its safety backup has finished — and re-expressing
// those as Go on the handler is what keeps that domain knowledge out of the
// queue.
type ReadinessChecker interface {
	// Ready reports whether op can run now. A non-zero retryAfter reschedules
	// it without consuming an attempt. ErrOperationObsolete cancels it.
	Ready(ctx context.Context, op Operation) (retryAfter time.Duration, err error)
}

// ErrOperationObsolete reports that an operation should not run at all —
// its target is gone, or the thing it would do has already happened. The
// runtime records it as cancelled rather than failed, since nothing went
// wrong.
var ErrOperationObsolete = errors.New("operation is no longer applicable")

// HandlerError lets a handler set the error code recorded against a failed
// operation. A plain error records a generic code.
type HandlerError struct {
	Code string
	Err  error
}

func (e *HandlerError) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Err.Error()
}

func (e *HandlerError) Unwrap() error { return e.Err }

// Failf builds a HandlerError with a code and a formatted message.
func Failf(code, format string, args ...any) error {
	return &HandlerError{Code: code, Err: fmt.Errorf(format, args...)}
}

type registry struct {
	handlers map[string]Handler
}

func newRegistry() *registry {
	return &registry{handlers: map[string]Handler{}}
}

func (r *registry) register(handler Handler) error {
	if handler == nil {
		return errors.New("nil handler")
	}
	kind := strings.TrimSpace(handler.Kind())
	if kind == "" {
		return errors.New("handler kind must not be empty")
	}
	if _, exists := r.handlers[kind]; exists {
		return fmt.Errorf("handler for kind %q is already registered", kind)
	}
	r.handlers[kind] = handler
	return nil
}

func (r *registry) lookup(kind string) (Handler, bool) {
	handler, ok := r.handlers[strings.TrimSpace(kind)]
	return handler, ok
}
