package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

// Before this phase, cancelling a deployment only ever touched the
// deployments row -- there was no operations row to speak of, since a
// deploy ran in a goroutine the moment it was prepared. Now
// PrepareServiceDeploy always enqueues one, so cancelling a deployment that
// is still queued behind another on the same lock_key must also cancel its
// operation outright, or the runtime would claim and run it anyway despite
// the deployments row already reading CANCELLED.
func TestDeploymentCancelV2AlsoCancelsItsQueuedOperation(t *testing.T) {
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	s := &server{
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:       &config.Config{DataDir: t.TempDir()},
		store:     repository.New(db),
		envSealer: sealer,
	}

	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{
		ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git",
		InternalPort: 3000, InitialEnvironmentID: environments[0].ID, InitialBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleServiceDeployActionV2(recorder, httptest.NewRequest(http.MethodPost, "/api/services/"+service.ID+"/deploy", nil), service.ID, environments[0].ID)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("deploy accept status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	deploymentID := decodeResponse(t, recorder)["deployment"].(map[string]any)["id"].(string)

	op, err := s.store.GetOperation(context.Background(), deploymentID)
	if err != nil {
		t.Fatalf("expected an enqueued operation: %v", err)
	}
	if op.Status != "queued" {
		t.Fatalf("operation status = %q, want queued", op.Status)
	}

	cancel := httptest.NewRecorder()
	s.handleDeploymentCancelV2(cancel, httptest.NewRequest(http.MethodPost, "/api/deployments/"+deploymentID+"/cancel", nil), deploymentID)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}

	deployment, err := s.store.GetServiceDeployment(context.Background(), deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Status != models.DeploymentCancelled {
		t.Fatalf("deployment status = %q, want CANCELLED", deployment.Status)
	}

	op, err = s.store.GetOperation(context.Background(), deploymentID)
	if err != nil {
		t.Fatal(err)
	}
	if op.Status != "cancelled" {
		t.Fatalf("operation status = %q, want cancelled -- otherwise the deploy runtime would still claim and run it", op.Status)
	}
}
