package obs

import (
	"context"
	"testing"
)

func TestWithStore_nilSafe(t *testing.T) {
	ctx := WithStore(context.Background(), nil)
	if StoreFrom(ctx) != nil {
		t.Fatal("expected nil store")
	}
}
