// Package workers runs durable background operations off the operations
// queue (ADR-0002 §4).
//
// A Runtime owns a pool of workers. Each claims an operation, holds a lease
// on it, renews that lease on a timer, and dispatches it to the Handler
// registered for its kind. Handlers live in the packages that own the
// domain — this package knows nothing about databases, deploys, or Docker.
//
// The three properties it exists to provide:
//
//   - Work survives a crash. An operation whose worker dies is returned to
//     the queue at the next startup and retried, up to its attempt limit.
//   - Work sharing a lock key runs one at a time, enforced when the
//     operation is claimed rather than when it is enqueued.
//   - Cancellation lands at a step boundary. A cancel request is observed on
//     the lease-renewal tick and cancels the operation's context; the
//     handler returns, and the partial progress it reported is preserved.
package workers
