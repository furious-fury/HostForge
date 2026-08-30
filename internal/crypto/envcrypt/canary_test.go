package envcrypt

import (
	"encoding/base64"
	"errors"
	"testing"
)

func testSealer(t *testing.T, seed byte) *Sealer {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed + byte(i)
	}
	s, err := NewFromBase64Key(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// VerifyOrInitCanary is the core of the required-encryption-key startup
// check (ADR-0002 §20.4): first boot seeds the canary, every boot after
// checks it, and a key that doesn't match what sealed it must be rejected
// with an explanation, not a bare error.
func TestVerifyOrInitCanary(t *testing.T) {
	t.Run("first boot seeds the canary", func(t *testing.T) {
		sealer := testSealer(t, 0)
		var stored []byte
		set := func(ct []byte) error { stored = ct; return nil }
		get := func() ([]byte, bool, error) { return nil, false, nil }

		if err := VerifyOrInitCanary(sealer, get, set); err != nil {
			t.Fatalf("first boot: %v", err)
		}
		if len(stored) == 0 {
			t.Fatal("expected a canary to be sealed and stored on first boot")
		}
		plain, err := sealer.Open(stored)
		if err != nil || string(plain) != CanaryPlaintext {
			t.Fatalf("stored canary does not round-trip: plain=%q err=%v", plain, err)
		}
	})

	t.Run("matching key on a later boot succeeds without rewriting", func(t *testing.T) {
		sealer := testSealer(t, 1)
		sealed, err := sealer.Seal([]byte(CanaryPlaintext))
		if err != nil {
			t.Fatal(err)
		}
		setCalled := false
		set := func([]byte) error { setCalled = true; return nil }
		get := func() ([]byte, bool, error) { return sealed, true, nil }

		if err := VerifyOrInitCanary(sealer, get, set); err != nil {
			t.Fatalf("matching key: %v", err)
		}
		if setCalled {
			t.Fatal("existing canary must not be rewritten")
		}
	})

	t.Run("mismatched key is rejected with an explanation", func(t *testing.T) {
		sealedUnderOtherKey, err := testSealer(t, 99).Seal([]byte(CanaryPlaintext))
		if err != nil {
			t.Fatal(err)
		}
		wrongKeySealer := testSealer(t, 1)
		get := func() ([]byte, bool, error) { return sealedUnderOtherKey, true, nil }
		set := func([]byte) error { t.Fatal("must not write on mismatch"); return nil }

		err = VerifyOrInitCanary(wrongKeySealer, get, set)
		if err == nil {
			t.Fatal("expected an error for a mismatched key")
		}
		if len(err.Error()) < 40 {
			t.Fatalf("error should explain the mismatch, got a short message: %q", err.Error())
		}
	})

	t.Run("get error propagates", func(t *testing.T) {
		sealer := testSealer(t, 0)
		wantErr := errors.New("db unavailable")
		get := func() ([]byte, bool, error) { return nil, false, wantErr }
		set := func([]byte) error { t.Fatal("must not write when get failed"); return nil }

		if err := VerifyOrInitCanary(sealer, get, set); err == nil {
			t.Fatal("expected the get error to propagate")
		}
	})
}
