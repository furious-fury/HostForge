// Package app implements a minimal GitHub App client:
//   - Manifest conversion (app manifest -> App credentials)
//   - RS256 JWT minting for App auth
//   - Installation access token minting with an in-memory cache
//   - Repository and installation listing
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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParsePrivateKeyPEM parses a PKCS#1 or PKCS#8 RSA private key in PEM form.
func ParsePrivateKeyPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM: no block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := anyKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}

// MintAppJWT mints an RS256 JWT signed by the App's private key, valid for ~9 minutes.
// GitHub requires iat be in the past (clock skew) and exp within 10 minutes.
func MintAppJWT(appID int64, privateKey *rsa.PrivateKey, now time.Time) (string, error) {
	if privateKey == nil {
		return "", errors.New("nil private key")
	}
	if appID <= 0 {
		return "", errors.New("invalid app id")
	}
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iat": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(appID, 10),
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	hseg := b64url(hb)
	cseg := b64url(cb)
	signingInput := hseg + "." + cseg
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}
