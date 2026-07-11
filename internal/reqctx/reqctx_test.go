package reqctx

import (
	"context"
	"testing"
)

func TestRequestID_roundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "rid-1")
	if got := RequestID(ctx); got != "rid-1" {
		t.Fatalf("got %q", got)
	}
	if got := RequestID(context.Background()); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
