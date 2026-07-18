package services

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DatabaseGatewayTLSMaterial struct {
	CertificatePEM  []byte
	PrivateKeyPEM   []byte
	Fingerprint     string
	NotAfter        time.Time
	CertificatePath string
	PrivateKeyPath  string
}

// LoadDatabaseGatewayTLSMaterial loads only a certificate pair for the exact
// reserved hostname. Explicit paths take priority; otherwise Caddy's managed
// storage layout is searched without copying unrelated private keys.
func LoadDatabaseGatewayTLSMaterial(hostname, certificatePath, privateKeyPath, caddyStorageRoot string, now time.Time) (DatabaseGatewayTLSMaterial, error) {
	hostname = strings.ToLower(strings.Trim(strings.TrimSpace(hostname), "."))
	if hostname == "" {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway hostname required"))
	}
	certificatePath, privateKeyPath = strings.TrimSpace(certificatePath), strings.TrimSpace(privateKeyPath)
	if certificatePath == "" && privateKeyPath == "" {
		certificatePath, privateKeyPath = findCaddyGatewayCertificatePair(caddyStorageRoot, hostname)
	}
	if certificatePath == "" || privateKeyPath == "" {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("Caddy certificate and key for the gateway hostname are unavailable"))
	}
	certificatePEM, err := os.ReadFile(certificatePath)
	if err != nil {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", fmt.Errorf("read gateway certificate: %w", err))
	}
	privateKeyPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", fmt.Errorf("read gateway private key: %w", err))
	}
	pair, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway certificate/key pair is invalid"))
	}
	if len(pair.Certificate) == 0 {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway leaf certificate is missing"))
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway leaf certificate cannot be parsed"))
	}
	if err := leaf.VerifyHostname(hostname); err != nil {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway certificate SAN does not match the reserved hostname"))
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", errors.New("gateway certificate is outside its validity period"))
	}
	fingerprint := sha256.Sum256(leaf.Raw)
	return DatabaseGatewayTLSMaterial{CertificatePEM: certificatePEM, PrivateKeyPEM: privateKeyPEM, Fingerprint: hex.EncodeToString(fingerprint[:]), NotAfter: leaf.NotAfter.UTC(), CertificatePath: certificatePath, PrivateKeyPath: privateKeyPath}, nil
}

func findCaddyGatewayCertificatePair(storageRoot, hostname string) (string, string) {
	storageRoot = strings.TrimSpace(storageRoot)
	if storageRoot == "" {
		return "", ""
	}
	pattern := filepath.Join(storageRoot, "certificates", "*", hostname, hostname+".crt")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", ""
	}
	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := os.Stat(matches[i])
		right, rightErr := os.Stat(matches[j])
		if leftErr != nil {
			return false
		}
		if rightErr != nil {
			return true
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, certificatePath := range matches {
		keyPath := strings.TrimSuffix(certificatePath, ".crt") + ".key"
		if _, err := os.Stat(keyPath); err == nil {
			return certificatePath, keyPath
		}
	}
	return "", ""
}

func databaseGatewayCertificateLeaf(certificatePEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		return nil, errors.New("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}
