// Package envcrypt seals/opens small blobs (project env values) with AES-GCM.
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
