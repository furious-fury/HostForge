package services

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSummarizeManagedCertFile(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now().UTC().Add(-time.Hour),
		NotAfter:     time.Now().UTC().Add(90 * 24 * time.Hour),
		DNSNames:     []string{"example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})

	dir := t.TempDir()
	certRoot := filepath.Join(dir, "certificates", "acme-v02.api.letsencrypt.org-directory", "example.com")
	if err := os.MkdirAll(certRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	crtPath := filepath.Join(certRoot, "example.com.crt")
	if err := os.WriteFile(crtPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	msg, ok := summarizeManagedCertFile(filepath.Join(dir, "certificates"), "example.com")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.Contains(msg, "leaf_expires=") || !strings.Contains(msg, "issuer=") {
		t.Fatalf("unexpected message: %q", msg)
	}
}
