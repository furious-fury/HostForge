// Package envcrypt seals and opens small control-plane secrets with AES-GCM.
package envcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Sealer encrypts and decrypts values using a 256-bit AES key.
type Sealer struct {
	gcm cipher.AEAD
}

// CanaryPlaintext is sealed into a singleton row on first boot and checked
// on every boot after, to confirm the configured key still matches the one
// that encrypted every other secret in the database. The version suffix
// lets a future sealed-format change be detected explicitly instead of
// silently.
const CanaryPlaintext = "hostforge-encryption-canary-v1"

// VerifyOrInitCanary confirms sealer round-trips CanaryPlaintext against the
// stored canary, or seeds it if this is the first boot. get returns the
// stored ciphertext and whether a row exists yet; set persists a freshly
// sealed canary. Both are closures so this function needs no knowledge of
// how or where the canary is persisted.
//
// A non-nil error means the configured key does not match the key that
// sealed this database's existing secrets — restoring the previous key is
// the only fix; there is no way to recover data sealed under a different key.
func VerifyOrInitCanary(sealer *Sealer, get func() (sealed []byte, found bool, err error), set func(sealed []byte) error) error {
	sealed, found, err := get()
	if err != nil {
		return fmt.Errorf("read encryption canary: %w", err)
	}
	if !found {
		ct, err := sealer.Seal([]byte(CanaryPlaintext))
		if err != nil {
			return fmt.Errorf("seal encryption canary: %w", err)
		}
		if err := set(ct); err != nil {
			return fmt.Errorf("store encryption canary: %w", err)
		}
		return nil
	}
	plain, err := sealer.Open(sealed)
	if err != nil || string(plain) != CanaryPlaintext {
		return errors.New("HOSTFORGE_ENV_ENCRYPTION_KEY does not match the key that encrypted this database's secrets; " +
			"if you rotated the key, restore the previous value — a mismatched key makes every sealed secret " +
			"(environment variables, database credentials, the GitHub App key) permanently unrecoverable")
	}
	return nil
}

// NewFromBase64Key decodes a standard base64 32-byte key (e.g. openssl rand -base64 32).
func NewFromBase64Key(b64 string) (*Sealer, error) {
	raw := strings.TrimSpace(b64)
	if raw == "" {
		return nil, errors.New("empty encryption key")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode base64 key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Sealer{gcm: gcm}, nil
}

// Seal returns nonce || ciphertext || tag suitable for storage.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	if s == nil || s.gcm == nil {
		return nil, errors.New("nil sealer")
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return s.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a blob produced by Seal.
func (s *Sealer) Open(sealed []byte) ([]byte, error) {
	if s == nil || s.gcm == nil {
		return nil, errors.New("nil sealer")
	}
	ns := s.gcm.NonceSize()
	if len(sealed) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := sealed[:ns], sealed[ns:]
	return s.gcm.Open(nil, nonce, ct, nil)
}
