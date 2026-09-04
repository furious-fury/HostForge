package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/repository"
)

// newCaddyValidatingTestServer is newAPITestServer plus a real
// CaddyRootConfig/CaddyBin pointed at a scripted stub that rejects any
// config mentioning rejectDomain -- letting a test exercise the add-time
// ValidateSiteBlock call the domain handlers make, rather than only the
// codepath that skips it when Caddy is unconfigured.
func newCaddyValidatingTestServer(t *testing.T, rejectDomain string) *server {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses a POSIX shell stub for the caddy binary")
	}
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "caddy")
	script := "#!/bin/sh\nprev=\"\"\nfor a; do\n  if [ \"$prev\" = \"--config\" ]; then cfg=\"$a\"; fi\n  prev=\"$a\"\ndone\n" +
		`if grep -q "` + rejectDomain + `" "$cfg" 2>/dev/null; then echo "parse error near ` + rejectDomain + `" >&2; exit 1; fi` + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "Caddyfile")
	if err := os.WriteFile(root, []byte(""), 0o640); err != nil {
		t.Fatal(err)
	}

	return &server{
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:   &config.Config{DatabaseOperationConcurrency: 1, DatabaseTransferMaxPerHour: 60, CaddyBin: bin, CaddyRootConfig: root},
		store: repository.New(db), envSealer: sealer,
	}
}

// The core of ADR-0002 §19.3 item 1: a malformed hostname is rejected
// synchronously, by the request that submitted it, and never reaches the
// database -- not discovered later as a fleet-wide reconcile failure.
func TestCreateDomainRejectsInvalidCaddyConfigSynchronouslyAndNeverPersists(t *testing.T) {
	s := newCaddyValidatingTestServer(t, "malformed")
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v", err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/applications/" + application.ID + "/environments/" + environments[0].ID

	create := httptest.NewRecorder()
	s.handleApplications(create, httptest.NewRequest(http.MethodPost, base+"/domains", strings.NewReader(`{"domain_name":"malformed.example.test","service_id":"`+service.ID+`"}`)))
	assertAPIError(t, create, http.StatusBadRequest, "invalid_domain_config")

	domains, err := s.store.ListServiceDomains(context.Background(), application.ID, environments[0].ID, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("rejected domain was persisted anyway: %+v", domains)
	}

	// A well-formed hostname on the same, otherwise-strict, server must
	// still succeed -- the stub only rejects "malformed".
	createGood := httptest.NewRecorder()
	s.handleApplications(createGood, httptest.NewRequest(http.MethodPost, base+"/domains", strings.NewReader(`{"domain_name":"fine.example.test","service_id":"`+service.ID+`"}`)))
	if createGood.Code != http.StatusCreated {
		t.Fatalf("well-formed domain create status=%d body=%s", createGood.Code, createGood.Body.String())
	}
}
