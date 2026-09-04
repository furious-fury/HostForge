package services

import (
	"bytes"
	"context"
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

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

// writeCert writes a self-signed leaf under certRoot in Caddy's on-disk
// layout (<root>/<acme-issuer-dir>/<domain>/<domain>.crt) and returns its
// path. notAfter controls whether observeCertificateFile sees it as valid.
func writeCert(t *testing.T, certRoot, domain string, notAfter time.Time) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now().UTC().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{domain},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})

	dir := filepath.Join(certRoot, "acme-v02.api.letsencrypt.org-directory", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, domain+".crt")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// observeCertificateFile is the only thing in the codebase that actually
// inspects a certificate, so it is the only thing that should decide
// ssl_status. These three cases are the whole state machine PR1 gives it.
func TestObserveCertificateFileValidLeafIsActive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certRoot := filepath.Join(dir, "certificates")
	writeCert(t, certRoot, "example.com", time.Now().UTC().Add(90*24*time.Hour))

	observation := observeCertificateFile(certRoot, "example.com")
	if observation.sslStatus != models.SSLStatusActive {
		t.Fatalf("sslStatus = %q, want ACTIVE", observation.sslStatus)
	}
	if !strings.Contains(observation.message, "leaf_expires=") || !strings.Contains(observation.message, "issuer=") {
		t.Fatalf("unexpected message: %q", observation.message)
	}
}

func TestObserveCertificateFileExpiredLeafIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certRoot := filepath.Join(dir, "certificates")
	writeCert(t, certRoot, "example.com", time.Now().UTC().Add(-24*time.Hour))

	observation := observeCertificateFile(certRoot, "example.com")
	if observation.sslStatus != models.SSLStatusError {
		t.Fatalf("sslStatus = %q, want ERROR for an expired leaf", observation.sslStatus)
	}
	if !strings.Contains(observation.message, "expired=true") {
		t.Fatalf("expired leaf message missing expired=true: %q", observation.message)
	}
}

func TestObserveCertificateFileMissingLeafIsPending(t *testing.T) {
	t.Parallel()
	certRoot := filepath.Join(t.TempDir(), "certificates")
	observation := observeCertificateFile(certRoot, "never-issued.example.com")
	if observation.sslStatus != models.SSLStatusPending {
		t.Fatalf("sslStatus = %q, want PENDING when no leaf exists yet", observation.sslStatus)
	}
}

// PollCaddyCertObservations used to only ever touch last_cert_message and
// cert_checked_at ("It never changes ssl_status" -- the doc comment this PR
// removes). Nothing else in the codebase set ssl_status from a real
// certificate once route-sync stopped owning it, so a domain's certificate
// state was permanently unknowable from the row. This proves the poll loop
// now writes it.
func TestPollCaddyCertObservationsWritesSSLStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Cert", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, envs[0].ID, service.ID, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if domain.SSLStatus != models.SSLStatusPending {
		t.Fatalf("new domain ssl_status = %q, want PENDING before any poll", domain.SSLStatus)
	}

	storageRoot := t.TempDir()
	writeCert(t, filepath.Join(storageRoot, "certificates"), "example.com", time.Now().UTC().Add(90*24*time.Hour))
	cfg := &config.Config{CaddyStorageRoot: storageRoot}

	if err := PollCaddyCertObservations(ctx, discardLog(), cfg, store); err != nil {
		t.Fatal(err)
	}

	updated, err := store.GetServiceDomain(ctx, app.ID, envs[0].ID, domain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SSLStatus != models.SSLStatusActive {
		t.Fatalf("ssl_status after poll = %q, want ACTIVE", updated.SSLStatus)
	}
	if !strings.Contains(updated.LastCertMessage, "leaf_expires=") {
		t.Fatalf("last_cert_message not updated: %q", updated.LastCertMessage)
	}
}

// With no storage root configured, the poll has no signal either way and
// must not claim ERROR -- PENDING (the row's own default) is the honest
// report of "unknown", not "broken".
func TestPollCaddyCertObservationsLeavesStatusAloneWithNoStorageRoot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hostforge.sqlite")
	db, err := database.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	ctx := context.Background()

	app, err := store.CreateApplication(ctx, "Cert", "")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "web", RepoURL: "https://github.com/acme/web.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := store.CreateServiceDomain(ctx, app.ID, envs[0].ID, service.ID, "example.com")
	if err != nil {
		t.Fatal(err)
	}

	if err := PollCaddyCertObservations(ctx, discardLog(), &config.Config{}, store); err != nil {
		t.Fatal(err)
	}

	updated, err := store.GetServiceDomain(ctx, app.ID, envs[0].ID, domain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SSLStatus != models.SSLStatusPending {
		t.Fatalf("ssl_status = %q, want PENDING (unchanged) with no storage root configured", updated.SSLStatus)
	}
}
