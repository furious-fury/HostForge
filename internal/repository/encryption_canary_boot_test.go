package repository

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
)

// Exercises the exact sequence main.go runs at boot (construct sealer,
// VerifyOrInitCanary against the real store), standing in for the manual
// "fresh install / restart with same key / restart with a different key"
// check the plan calls for — this environment has no Docker daemon to run
// the actual binary, so this is the closest available substitute.
func TestBootSequenceEncryptionKeyCanary(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)

	boot := func(k string) error {
		sealer, err := envcrypt.NewFromBase64Key(k)
		if err != nil {
			return err
		}
		return envcrypt.VerifyOrInitCanary(sealer,
			func() ([]byte, bool, error) { return store.GetEncryptionCanary(ctx) },
			func(sealed []byte) error { return store.SetEncryptionCanary(ctx, sealed) },
		)
	}

	if err := boot(keyB64); err != nil {
		t.Fatalf("fresh install must seed the canary and boot cleanly: %v", err)
	}
	if err := boot(keyB64); err != nil {
		t.Fatalf("restart with the same key must succeed: %v", err)
	}

	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = byte(255 - i)
	}
	if err := boot(base64.StdEncoding.EncodeToString(otherKey)); err == nil {
		t.Fatal("restart with a different key must fail, not silently proceed")
	}

	// The original key still works after a rejected mismatched-key attempt —
	// a failed boot must not have corrupted the stored canary.
	if err := boot(keyB64); err != nil {
		t.Fatalf("original key must still work after a rejected mismatch: %v", err)
	}
}
