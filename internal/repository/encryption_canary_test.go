package repository

import (
	"context"
	"testing"
)

func TestEncryptionCanaryRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, found, err := store.GetEncryptionCanary(ctx)
	if err != nil {
		t.Fatalf("get on fresh store: %v", err)
	}
	if found {
		t.Fatal("expected no canary row on a fresh store")
	}

	if err := store.SetEncryptionCanary(ctx, []byte("sealed-bytes")); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, found, err := store.GetEncryptionCanary(ctx)
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if !found {
		t.Fatal("expected canary row to exist after set")
	}
	if string(got) != "sealed-bytes" {
		t.Fatalf("got %q, want %q", got, "sealed-bytes")
	}
}
