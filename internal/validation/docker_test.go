package validation

import (
	"context"
	"testing"
)

func TestCheckDocker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := CheckDocker(ctx); err != nil {
		t.Skip("docker daemon not available:", err)
	}
}
