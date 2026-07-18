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

func gatewayTestCertificate(t *testing.T, hostname string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname}, NotBefore: time.Now().UTC().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var certificate, privateKey bytes.Buffer
	_ = pem.Encode(&certificate, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = pem.Encode(&privateKey, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certificate.Bytes(), privateKey.Bytes()
}

func TestLoadDatabaseGatewayTLSMaterialValidatesExactSANAndPair(t *testing.T) {
	now := time.Now().UTC()
	certificate, privateKey := gatewayTestCertificate(t, "postgres.apps.example.test", now.Add(30*24*time.Hour))
	directory := t.TempDir()
	certificatePath, keyPath := filepath.Join(directory, "gateway.crt"), filepath.Join(directory, "gateway.key")
	if err := os.WriteFile(certificatePath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, privateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := LoadDatabaseGatewayTLSMaterial("postgres.apps.example.test", certificatePath, keyPath, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(material.Fingerprint) != 64 || !material.NotAfter.After(now) || !bytes.Equal(material.PrivateKeyPEM, privateKey) {
		t.Fatalf("material=%+v", material)
	}
	if _, err := LoadDatabaseGatewayTLSMaterial("postgres.other.example.test", certificatePath, keyPath, "", now); err == nil || FirstPublicCode(err) != "database_gateway_tls_unavailable" {
		t.Fatalf("mismatched SAN error=%v", err)
	}
}

func TestLoadDatabaseGatewayTLSMaterialFindsOnlyReservedCaddyPair(t *testing.T) {
	now := time.Now().UTC()
	hostname := "postgres.apps.example.test"
	certificate, privateKey := gatewayTestCertificate(t, hostname, now.Add(30*24*time.Hour))
	storage := t.TempDir()
	directory := filepath.Join(storage, "certificates", "acme.example.test", hostname)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, hostname+".crt"), certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, hostname+".key"), privateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := LoadDatabaseGatewayTLSMaterial(hostname, "", "", storage, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(material.CertificatePath, hostname) || !strings.Contains(material.PrivateKeyPath, hostname) {
		t.Fatalf("unexpected Caddy pair: %+v", material)
	}
}
