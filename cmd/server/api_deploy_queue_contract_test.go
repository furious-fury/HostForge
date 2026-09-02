package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/services"
)

func newDeployQueueTestServer(t *testing.T) *server {
	t.Helper()
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sealer, err := envcrypt.NewFromBase64Key(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return &server{
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:            &config.Config{DataDir: t.TempDir(), WebhookSecret: "test-secret", WebhookMaxBodyBytes: 1 << 20},
		store:          repository.New(db),
		envSealer:      sealer,
		webhookLimiter: newFixedWindowLimiter(1000, time.Minute),
	}
}

// Each entry point that used to launch its own goroutine now only enqueues.
// This pins the contract that survives that change: a 202 with the
// deployment row, and an operations row shaped so the deploy runtime and
// this phase's other fixes (kind filter, non-resumable drain, lock_key ==
// worktree scope) all apply to it.
func TestDeployEntryPointsEnqueueOperationsWithExpectedShape(t *testing.T) {
	cases := []struct {
		name         string
		trigger      string
		wantPriority int
		invoke       func(t *testing.T, s *server, service repository.Service, environmentID string) *httptest.ResponseRecorder
	}{
		{
			name: "manual", trigger: "manual", wantPriority: 200,
			invoke: func(t *testing.T, s *server, service repository.Service, environmentID string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				s.handleServiceDeployActionV2(recorder, httptest.NewRequest(http.MethodPost, "/api/services/"+service.ID+"/deploy", nil), service.ID, environmentID)
				return recorder
			},
		},
		{
			name: "redeploy", trigger: "redeploy", wantPriority: 200,
			invoke: func(t *testing.T, s *server, service repository.Service, environmentID string) *httptest.ResponseRecorder {
				source, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{
					ServiceID: service.ID, EnvironmentID: environmentID, CommitHash: "abc123", TriggerKind: "manual", Actor: "operator", Branch: "main",
				})
				if err != nil {
					t.Fatal(err)
				}
				recorder := httptest.NewRecorder()
				s.handleDeploymentRedeployV2(recorder, httptest.NewRequest(http.MethodPost, "/api/deployments/"+source.ID+"/redeploy", nil), source.ID)
				return recorder
			},
		},
		{
			name: "rollback", trigger: "rollback", wantPriority: 200,
			invoke: func(t *testing.T, s *server, service repository.Service, environmentID string) *httptest.ResponseRecorder {
				source, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{
					ServiceID: service.ID, EnvironmentID: environmentID, CommitHash: "abc123", TriggerKind: "manual", Actor: "operator", Branch: "main",
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.store.UpdateDeploymentStatus(context.Background(), source.ID, models.DeploymentSuccess, ""); err != nil {
					t.Fatal(err)
				}
				recorder := httptest.NewRecorder()
				s.handleDeploymentRollbackV2(recorder, httptest.NewRequest(http.MethodPost, "/api/deployments/"+source.ID+"/rollback", nil), source.ID)
				return recorder
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newDeployQueueTestServer(t)
			application, err := s.store.CreateApplication(context.Background(), "Payments "+tc.name, "")
			if err != nil {
				t.Fatal(err)
			}
			environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
			if err != nil {
				t.Fatal(err)
			}
			service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{
				ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git",
				InternalPort: 3000, InitialEnvironmentID: environments[0].ID, InitialBranch: "main",
			})
			if err != nil {
				t.Fatal(err)
			}

			recorder := tc.invoke(t, s, service, environments[0].ID)
			if recorder.Code != http.StatusAccepted {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			body := decodeResponse(t, recorder)
			if body["status"] != "accepted" {
				t.Fatalf("response status = %v, want accepted", body["status"])
			}
			deployment := body["deployment"].(map[string]any)
			deploymentID := deployment["id"].(string)
			if deployment["status"] != models.DeploymentQueued {
				t.Fatalf("deployment status = %v, want %s", deployment["status"], models.DeploymentQueued)
			}

			op, err := s.store.GetOperation(context.Background(), deploymentID)
			if err != nil {
				t.Fatalf("expected an enqueued operation: %v", err)
			}
			if op.Kind != repository.DeployOperationKind {
				t.Errorf("kind = %q, want %q", op.Kind, repository.DeployOperationKind)
			}
			wantLockKey := services.DeployLockKey(service.ID, environments[0].ID)
			if op.LockKey != wantLockKey {
				t.Errorf("lock key = %q, want %q", op.LockKey, wantLockKey)
			}
			if op.MaxAttempts != 1 {
				t.Errorf("max attempts = %d, want 1", op.MaxAttempts)
			}
			if op.Priority != tc.wantPriority {
				t.Errorf("priority = %d, want %d", op.Priority, tc.wantPriority)
			}
			if op.Status != "queued" {
				t.Errorf("operation status = %q, want queued", op.Status)
			}
		})
	}
}

// One push can match several service/environment auto-deploy bindings
// (webhook_targets.go joins on repo_url+branch with no uniqueness
// constraint). This pins that the fan-out enqueues one independent
// deployment per match, all at the webhook priority band, rather than
// collapsing or dropping any of them.
func TestWebhookPushFansOutToOneDeploymentPerMatchingServiceEnvironment(t *testing.T) {
	s := newDeployQueueTestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Fanout", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	const repoURL = "https://github.com/acme/monorepo.git"
	var boundServices []repository.Service
	for _, name := range []string{"web", "worker"} {
		svc, err := s.store.CreateService(ctx, repository.CreateServiceInput{
			ApplicationID: application.ID, Name: name, RepoURL: repoURL, InternalPort: 3000,
			InitialEnvironmentID: environments[0].ID, InitialBranch: "main", InitialAutoDeploy: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		boundServices = append(boundServices, svc)
	}

	payload, err := json.Marshal(map[string]any{
		"ref": "refs/heads/main", "after": "deadbeef",
		"repository": map[string]any{"clone_url": repoURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Event", "push")
	recorder := httptest.NewRecorder()
	s.handleGitHubWebhook(recorder, req)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeResponse(t, recorder)
	deploymentIDs, ok := body["deployment_ids"].([]any)
	if !ok || len(deploymentIDs) != len(boundServices) {
		t.Fatalf("deployment_ids = %#v, want %d entries", body["deployment_ids"], len(boundServices))
	}
	if int(body["count"].(float64)) != len(boundServices) {
		t.Fatalf("count = %v, want %d", body["count"], len(boundServices))
	}

	seen := map[string]bool{}
	for _, raw := range deploymentIDs {
		id := raw.(string)
		if seen[id] {
			t.Fatalf("duplicate deployment id %q in fan-out", id)
		}
		seen[id] = true
		op, err := s.store.GetOperation(ctx, id)
		if err != nil {
			t.Fatalf("deployment %s: expected an enqueued operation: %v", id, err)
		}
		if op.Priority != webhookOperationPriorityForTest {
			t.Errorf("deployment %s: priority = %d, want %d", id, op.Priority, webhookOperationPriorityForTest)
		}
	}
}

// webhookOperationPriorityForTest mirrors services.webhookDeployPriority
// (unexported) so this test does not need to reach into the services
// package's internals to assert the fan-out's priority band.
const webhookOperationPriorityForTest = 150

// deployments.status must always be one of the five uppercase constants the
// frontend gates on (web/src/deployment-screens.tsx has four separate polls
// keyed on exact string equality). operations.status is lowercase and must
// never leak onto the deployment DTO -- this exercises a deployment through
// a real lowercase-status transition (claim) and checks the API response.
func TestDeploymentDetailResponseNeverExposesALowercaseOperationStatus(t *testing.T) {
	s := newDeployQueueTestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "CaseCheck", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(ctx, repository.CreateServiceInput{
		ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git",
		InternalPort: 3000, InitialEnvironmentID: environments[0].ID, InitialBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.handleServiceDeployActionV2(recorder, httptest.NewRequest(http.MethodPost, "/api/services/"+service.ID+"/deploy", nil), service.ID, environments[0].ID)
	deploymentID := decodeResponse(t, recorder)["deployment"].(map[string]any)["id"].(string)

	// Claim the operation exactly as the deploy runtime would -- this is
	// what flips operations.status to the lowercase 'running' and, through
	// the claim projection, deployments.status to BUILDING.
	if _, err := s.store.ClaimNextOperation(ctx, repository.ClaimOptions{
		Owner: "worker-a", Lease: time.Minute, Kinds: []string{repository.DeployOperationKind},
	}); err != nil {
		t.Fatal(err)
	}

	detail := httptest.NewRecorder()
	if !s.handleDeploymentV2Detail(detail, httptest.NewRequest(http.MethodGet, "/api/deployments/"+deploymentID, nil), deploymentID) {
		t.Fatal("handler did not consume the request")
	}
	status := decodeResponse(t, detail)["deployment"].(map[string]any)["status"].(string)
	switch status {
	case models.DeploymentQueued, models.DeploymentBuilding, models.DeploymentSuccess, models.DeploymentFailed, models.DeploymentCancelled:
	default:
		t.Fatalf("deployment status in response = %q, not one of the five uppercase constants (a lowercase operations status leaked onto the DTO)", status)
	}
	if status != models.DeploymentBuilding {
		t.Fatalf("status = %q, want %s after claim", status, models.DeploymentBuilding)
	}
}
