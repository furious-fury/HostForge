package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SessionClaims are embedded in signed UI session cookies.
type SessionClaims struct {
	Subject string `json:"sub"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
	Nonce   string `json:"n"`
}

// BearerToken extracts token from an Authorization header.
func BearerToken(authHeader string) (string, bool) {
	raw := strings.TrimSpace(authHeader)
	if raw == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// BearerMatches checks whether an Authorization header carries expectedToken.
func BearerMatches(authHeader, expectedToken string) bool {
	provided, ok := BearerToken(authHeader)
	if !ok {
		return false
	}
	return hmac.Equal([]byte(provided), []byte(strings.TrimSpace(expectedToken)))
}

// NewSignedSession encodes and signs session claims using HMAC-SHA256.
func NewSignedSession(secret string, ttl time.Duration) (string, SessionClaims, error) {
	if ttl <= 0 {
		return "", SessionClaims{}, fmt.Errorf("session ttl must be > 0")
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", SessionClaims{}, fmt.Errorf("session nonce: %w", err)
	}
	now := time.Now().UTC()
	claims := SessionClaims{
		Subject: "admin",
		Issued:  now.Unix(),
		Expires: now.Add(ttl).Unix(),
		Nonce:   hex.EncodeToString(nonce),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", SessionClaims{}, fmt.Errorf("marshal session claims: %w", err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(secret, payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(sig), claims, nil
}

// VerifySignedSession validates signature and expiry for a signed session value.
func VerifySignedSession(secret, raw string, now time.Time) (SessionClaims, error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 2 {
		return SessionClaims{}, fmt.Errorf("invalid session token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return SessionClaims{}, fmt.Errorf("decode session payload: %w", err)
	}
	receivedSig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return SessionClaims{}, fmt.Errorf("decode session signature: %w", err)
	}
	expectedSig := sign(secret, payload)
	if !hmac.Equal(receivedSig, expectedSig) {
		return SessionClaims{}, fmt.Errorf("invalid session signature")
	}
	var claims SessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return SessionClaims{}, fmt.Errorf("decode session claims: %w", err)
	}
	if now.UTC().Unix() >= claims.Expires {
		return SessionClaims{}, fmt.Errorf("session expired")
	}
	return claims, nil
}

// VerifyGitHubSignature verifies X-Hub-Signature-256 against request body.
func VerifyGitHubSignature(secret, signatureHeader string, body []byte) bool {
	raw := strings.TrimSpace(signatureHeader)
	if !strings.HasPrefix(raw, "sha256=") {
		return false
	}
	receivedHex := strings.TrimSpace(strings.TrimPrefix(raw, "sha256="))
	if receivedHex == "" {
		return false
	}
	received, err := hex.DecodeString(receivedHex)
	if err != nil {
		return false
	}
	expected := sign(secret, body)
	return hmac.Equal(received, expected)
}

func sign(secret string, payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
