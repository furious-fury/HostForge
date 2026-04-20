package app

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func mustGenKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return key, pemBytes
}

func TestParsePrivateKeyPEM_PKCS1(t *testing.T) {
	_, pemBytes := mustGenKey(t)
	key, err := ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if key == nil || key.N == nil {
		t.Fatal("nil key")
	}
}

func TestParsePrivateKeyPEM_PKCS8(t *testing.T) {
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	key, err := ParsePrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	if key == nil {
		t.Fatal("nil key")
	}
}

func TestParsePrivateKeyPEM_Invalid(t *testing.T) {
	if _, err := ParsePrivateKeyPEM([]byte("not pem")); err == nil {
		t.Fatal("expected error for junk input")
	}
}

func TestMintAppJWT_SignatureAndClaims(t *testing.T) {
	key, _ := mustGenKey(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	tok, err := MintAppJWT(12345, key, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 jwt segments, got %d", len(parts))
	}
	claimsJSON, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if iss, _ := claims["iss"].(string); iss != "12345" {
		t.Fatalf("iss=%q", iss)
	}
	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)
	if int64(exp)-int64(iat) > 11*60 {
		t.Fatalf("exp-iat too large: %v", exp-iat)
	}
	if int64(iat) > now.Unix() {
		t.Fatalf("iat must not be in future")
	}
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature verify: %v", err)
	}
}

func TestMintAppJWT_InvalidInput(t *testing.T) {
	if _, err := MintAppJWT(0, nil, time.Now()); err == nil {
		t.Fatal("want err for zero app id + nil key")
	}
	key, _ := mustGenKey(t)
	if _, err := MintAppJWT(0, key, time.Now()); err == nil {
		t.Fatal("want err for zero app id")
	}
	if _, err := MintAppJWT(1, nil, time.Now()); err == nil {
		t.Fatal("want err for nil key")
	}
}
