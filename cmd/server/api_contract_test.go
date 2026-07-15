package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
)

func newAPITestServer(t *testing.T) *server {
	t.Helper()
	db, err := database.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &server{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cfg: &config.Config{}, store: repository.New(db)}
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response %d %q: %v", recorder.Code, recorder.Body.String(), err)
	}
	return payload
}

func assertAPIError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	payload := decodeResponse(t, recorder)
	if payload["status"] != "error" || payload["error"] != code {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestApplicationsV2CreateAndFetch(t *testing.T) {
	s := newAPITestServer(t)
	create := httptest.NewRecorder()
	s.handleApplications(create, httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(`{"name":"Payments","description":"Billing services"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	created := decodeResponse(t, create)["application"].(map[string]any)
	id := created["id"].(string)

	get := httptest.NewRecorder()
	s.handleApplications(get, httptest.NewRequest(http.MethodGet, "/api/applications/"+id, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	payload := decodeResponse(t, get)
	application := payload["application"].(map[string]any)
	environments := payload["environments"].([]any)
	if application["name"] != "Payments" || len(environments) != 2 {
		t.Fatalf("unexpected application response: %#v", payload)
	}
}

func TestEnvironmentCreateContract(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/environments", strings.NewReader(`{"name":"QA","slug":"qa","kind":"staging"}`))
	s.handleApplications(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	environment := decodeResponse(t, recorder)["environment"].(map[string]any)
	if environment["name"] != "QA" || environment["slug"] != "qa" {
		t.Fatalf("unexpected environment: %#v", environment)
	}
	if _, err := s.store.GetServiceEnvironment(context.Background(), service.ID, environment["id"].(string)); err != nil {
		t.Fatalf("missing service binding: %v", err)
	}
}

func TestApplicationsV2RejectInvalidJSONAndMissingResource(t *testing.T) {
	s := newAPITestServer(t)
	invalid := httptest.NewRecorder()
	s.handleApplications(invalid, httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader("{")))
	assertAPIError(t, invalid, http.StatusBadRequest, "invalid_json_payload")

	missing := httptest.NewRecorder()
	s.handleApplications(missing, httptest.NewRequest(http.MethodGet, "/api/applications/missing", nil))
	assertAPIError(t, missing, http.StatusNotFound, "application_not_found")
}

func TestServicesV2RejectUnsafeRootDirectory(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"name":"api","repo_url":"https://github.com/acme/payments.git","github_installation_id":42,"root_directory":"../../etc","runtime":"auto","internal_port":3000,"health_check_path":"/health"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/services", strings.NewReader(body))
	s.handleApplications(recorder, request)
	assertAPIError(t, recorder, http.StatusBadRequest, "invalid_root_directory")
}

func TestOnboardingUsesPatchContract(t *testing.T) {
	s := newAPITestServer(t)
	recorder := httptest.NewRecorder()
	s.handleOnboardingRoutes(recorder, httptest.NewRequest(http.MethodPost, "/api/onboarding", strings.NewReader("{}")))
	assertAPIError(t, recorder, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestDeploymentDetailReturnsNormalizedNotFound(t *testing.T) {
	s := newAPITestServer(t)
	recorder := httptest.NewRecorder()
	if !s.handleDeploymentV2Detail(recorder, httptest.NewRequest(http.MethodGet, "/api/deployments/missing", nil), "missing") {
		t.Fatal("handler did not consume deployment detail request")
	}
	assertAPIError(t, recorder, http.StatusNotFound, "deployment_not_found")
}

func TestDeploymentCancellationContract(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID, TriggerKind: "manual", Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	s.handleDeploymentCancelV2(first, httptest.NewRequest(http.MethodPost, "/api/deployments/"+deployment.ID+"/cancel", nil), deployment.ID)
	if first.Code != http.StatusOK || decodeResponse(t, first)["status"] != "cancelled" {
		t.Fatalf("cancel status=%d body=%s", first.Code, first.Body.String())
	}
	stored, err := s.store.GetServiceDeployment(context.Background(), deployment.ID)
	if err != nil || stored.Status != models.DeploymentCancelled || stored.CancelledAt == "" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}

	second := httptest.NewRecorder()
	s.handleDeploymentCancelV2(second, httptest.NewRequest(http.MethodPost, "/api/deployments/"+deployment.ID+"/cancel", nil), deployment.ID)
	assertAPIError(t, second, http.StatusConflict, "deployment_not_cancellable")
}

func TestRollbackRejectsNonSuccessfulSource(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID, CommitHash: "abc123", TriggerKind: "manual", Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleDeploymentRollbackV2(recorder, httptest.NewRequest(http.MethodPost, "/api/deployments/"+deployment.ID+"/rollback", nil), deployment.ID)
	assertAPIError(t, recorder, http.StatusConflict, "rollback_source_not_successful")
}

func TestEventsRejectInvalidPagination(t *testing.T) {
	s := newAPITestServer(t)
	for _, path := range []string{"/api/events?limit=0", "/api/events?limit=invalid", "/api/events?cursor=invalid"} {
		recorder := httptest.NewRecorder()
		s.handlePlatformEvents(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestDeploymentsRejectInvalidCursor(t *testing.T) {
	s := newAPITestServer(t)
	recorder := httptest.NewRecorder()
	s.handleDeploymentsV2Collection(recorder, httptest.NewRequest(http.MethodGet, "/api/deployments?cursor=missing", nil))
	assertAPIError(t, recorder, http.StatusBadRequest, "invalid_cursor")
}

func TestObservabilityFeedsRejectInvalidPagination(t *testing.T) {
	s := newAPITestServer(t)
	for _, path := range []string{
		"/api/observability/requests?limit=0",
		"/api/observability/requests?cursor=invalid",
		"/api/observability/requests?status_class=unknown",
		"/api/observability/deploy-steps?limit=501",
		"/api/observability/deploy-steps?cursor=0",
	} {
		recorder := httptest.NewRecorder()
		s.handleObservabilityRoutes(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", path, recorder.Code, recorder.Body.String())
		}
		payload := decodeResponse(t, recorder)
		if payload["status"] != "error" || payload["error"] == "" {
			t.Fatalf("%s: malformed error: %#v", path, payload)
		}
	}
}

func TestSettingsActionsUseNormalizedErrors(t *testing.T) {
	s := newAPITestServer(t)
	for _, path := range []string{"/api/settings/actions/caddy-validate", "/api/settings/actions/caddy-sync"} {
		recorder := httptest.NewRecorder()
		s.handleSettingsRoutes(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		assertAPIError(t, recorder, http.StatusBadRequest, "caddy_root_config_not_set")
	}
}

func TestRequestResourceScopeResolvesV2Routes(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, _ := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ path, app, service, environment string }{
		{"/api/applications/" + application.ID + "/environments/" + environments[0].ID + "/domains", application.ID, "", environments[0].ID},
		{"/api/services/" + service.ID + "/environments/" + environments[0].ID + "/restart", application.ID, service.ID, environments[0].ID},
		{"/api/deployments/" + deployment.ID + "/logs", application.ID, service.ID, environments[0].ID},
	} {
		appID, serviceID, environmentID := s.requestResourceScope(context.Background(), test.path)
		if appID != test.app || serviceID != test.service || environmentID != test.environment {
			t.Fatalf("path=%s got=%s/%s/%s", test.path, appID, serviceID, environmentID)
		}
	}
}
