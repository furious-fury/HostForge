// Package reqctx carries HTTP request-scoped values (e.g. request_id) on context.Context.
package reqctx

import "context"

type ridKey struct{}

// WithRequestID returns ctx storing id for later reads by RequestID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ridKey{}, id)
}

// RequestID returns the request id from ctx, or empty if unset.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(ridKey{}).(string)
	return v
}
