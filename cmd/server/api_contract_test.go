package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/database"
	githubapp "github.com/furious-fury/HostForge/internal/github/app"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
)

type fakeGitHubRepositoryLister struct {
	repositories []githubapp.Repository
	branches     []string
	err          error
}

func (f fakeGitHubRepositoryLister) ListInstallationRepositories(context.Context, int64) ([]githubapp.Repository, error) {
	return f.repositories, f.err
}

func (f fakeGitHubRepositoryLister) ListRepositoryBranches(context.Context, int64, string, string) ([]string, error) {
	return f.branches, f.err
}

func newAPITestServer(t *testing.T) *server {
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
	return &server{log: slog.New(slog.NewTextHandler(io.Discard, nil)), cfg: &config.Config{DatabaseOperationConcurrency: 1, DatabaseTransferMaxPerHour: 60}, store: repository.New(db), envSealer: sealer}
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
	services, servicesOK := payload["services"].([]any)
	if application["name"] != "Payments" || len(environments) != 2 {
		t.Fatalf("unexpected application response: %#v", payload)
	}
	if !servicesOK || len(services) != 0 {
		t.Fatalf("empty services must be encoded as an array: %#v", payload["services"])
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

func TestApplicationEnvironmentSubresourcesAreRouted(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/applications/" + application.ID + "/environments/" + environments[0].ID

	createDomain := httptest.NewRecorder()
	s.handleApplications(createDomain, httptest.NewRequest(http.MethodPost, base+"/domains", strings.NewReader(`{"domain_name":"payments.example.test","service_id":"`+service.ID+`"}`)))
	if createDomain.Code != http.StatusCreated {
		t.Fatalf("create domain status=%d body=%s", createDomain.Code, createDomain.Body.String())
	}
	listDomains := httptest.NewRecorder()
	s.handleApplications(listDomains, httptest.NewRequest(http.MethodGet, base+"/domains", nil))
	if listDomains.Code != http.StatusOK {
		t.Fatalf("list domains status=%d body=%s", listDomains.Code, listDomains.Body.String())
	}
	if domains := decodeResponse(t, listDomains)["domains"].([]any); len(domains) != 1 {
		t.Fatalf("unexpected domains: %#v", domains)
	}

	createVariable := httptest.NewRecorder()
	s.handleApplications(createVariable, httptest.NewRequest(http.MethodPost, base+"/variables", strings.NewReader(`{"key":"DATABASE_URL","value":"postgres://secret","service_id":"`+service.ID+`"}`)))
	if createVariable.Code != http.StatusOK {
		t.Fatalf("create variable status=%d body=%s", createVariable.Code, createVariable.Body.String())
	}
	variablePayload := decodeResponse(t, createVariable)
	variable := variablePayload["variable"].(map[string]any)
	if variable["value_last4"] != "cret" {
		t.Fatalf("unexpected variable metadata: %#v", variable)
	}
	if strings.Contains(createVariable.Body.String(), "postgres://secret") || strings.Contains(createVariable.Body.String(), "value_ct") {
		t.Fatalf("secret leaked in response: %s", createVariable.Body.String())
	}
	listVariables := httptest.NewRecorder()
	s.handleApplications(listVariables, httptest.NewRequest(http.MethodGet, base+"/variables?service_id="+service.ID, nil))
	if listVariables.Code != http.StatusOK {
		t.Fatalf("list variables status=%d body=%s", listVariables.Code, listVariables.Body.String())
	}
	if variables := decodeResponse(t, listVariables)["variables"].([]any); len(variables) != 1 {
		t.Fatalf("unexpected variables: %#v", variables)
	}
}

func TestEnvironmentUpdateRecordsConfigurationEvent(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	environment := environments[0]

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/applications/"+application.ID+"/environments/"+environment.ID, strings.NewReader(`{"name":"Primary"}`))
	s.handleApplications(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated := decodeResponse(t, recorder)["environment"].(map[string]any)
	if updated["name"] != "Primary" || updated["id"] != environment.ID {
		t.Fatalf("unexpected environment: %#v", updated)
	}
	events, err := s.store.ListPlatformEvents(context.Background(), application.ID, "", "configuration", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EnvironmentID != environment.ID || events[0].Message != "Environment updated" || events[0].Detail != "Primary" {
		t.Fatalf("unexpected environment events: %#v", events)
	}
}

func TestEnvironmentUpdateRejectsEmptyName(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/applications/"+application.ID+"/environments/"+environments[0].ID, strings.NewReader(`{"name":"  "}`))
	s.handleApplications(recorder, request)
	assertAPIError(t, recorder, http.StatusBadRequest, "invalid_environment_name")
	payload := decodeResponse(t, recorder)
	fields, ok := payload["fields"].(map[string]any)
	if !ok || fields["name"] != "required" {
		t.Fatalf("unexpected field errors: %#v", payload)
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

func TestApplicationDeleteRecordsDurableEvent(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodDelete, "/api/applications/"+application.ID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if payload := decodeResponse(t, recorder); payload["status"] != "deleted" {
		t.Fatalf("unexpected delete payload: %#v", payload)
	}
	if _, err := s.store.GetApplication(context.Background(), application.ID); err == nil {
		t.Fatal("expected application to be deleted")
	}
	events, err := s.store.ListPlatformEvents(context.Background(), "", "", "application", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != "Application deleted" || events[0].Status != "deleted" || events[0].Detail != application.Name {
		t.Fatalf("unexpected application deletion events: %#v", events)
	}
}

func TestApplicationDeleteIgnoresRemovedHistoricalContainers(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(ctx, repository.CreateServiceInput{
		ApplicationID: application.ID,
		Name:          "api",
		RepoURL:       "https://github.com/acme/payments.git",
		InternalPort:  3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{
		ServiceID:     service.ID,
		EnvironmentID: environments[0].ID,
		ImageRef:      "hostforge/payments:old",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID:      deployment.ID,
		DockerContainerID: "already-removed",
		InternalPort:      3000,
		HostPort:          18080,
		Status:            "REMOVED",
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodDelete, "/api/applications/"+application.ID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := s.store.GetApplication(ctx, application.ID); err == nil {
		t.Fatal("expected application to be deleted")
	}
}

func TestServiceDeleteRecordsDurableApplicationEvent(t *testing.T) {
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
	s.handleServices(recorder, httptest.NewRequest(http.MethodDelete, "/api/services/"+service.ID, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, err := s.store.GetService(context.Background(), service.ID); err == nil {
		t.Fatal("expected service to be deleted")
	}
	events, err := s.store.ListPlatformEvents(context.Background(), application.ID, "", "service", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Message != "Service deleted" || events[0].Status != "deleted" || events[0].Detail != service.Name {
		t.Fatalf("unexpected service deletion events: %#v", events)
	}
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

func TestServicesV2RequireRepositoryFromActiveGitHubInstallation(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpsertGitHubInstallation(context.Background(), repository.UpsertGitHubInstallationInput{InstallationID: 42, AccountLogin: "acme"}); err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	s.githubRepoLister = fakeGitHubRepositoryLister{
		repositories: []githubapp.Repository{{CloneURL: "https://github.com/acme/payments.git"}},
		branches:     []string{"main"},
	}
	body := `{"name":"api","repo_url":"https://github.com/acme/payments.git","github_installation_id":42,"environment_id":"` + environments[0].ID + `","branch":"main","auto_deploy":true,"runtime":"auto","internal_port":3000,"health_check_path":"/health"}`
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/services", strings.NewReader(body)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	service := decodeResponse(t, recorder)["service"].(map[string]any)
	if service["repo_url"] != "https://github.com/acme/payments" || service["github_installation_id"] != float64(42) {
		t.Fatalf("unexpected service source: %#v", service)
	}
	binding := decodeResponse(t, recorder)["binding"].(map[string]any)
	if binding["environment_id"] != environments[0].ID || binding["branch"] != "main" || binding["auto_deploy"] != true {
		t.Fatalf("unexpected initial binding: %#v", binding)
	}
}

func TestServicesV2RejectInitialBranchWithoutCreatingPartialService(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	if err := s.store.UpsertGitHubInstallation(ctx, repository.UpsertGitHubInstallationInput{InstallationID: 42, AccountLogin: "acme"}); err != nil {
		t.Fatal(err)
	}
	s.githubRepoLister = fakeGitHubRepositoryLister{
		repositories: []githubapp.Repository{{CloneURL: "https://github.com/acme/payments.git"}},
		branches:     []string{"main"},
	}
	body := `{"name":"api","repo_url":"https://github.com/acme/payments.git","github_installation_id":42,"environment_id":"` + environments[0].ID + `","branch":"missing","auto_deploy":true,"runtime":"auto","internal_port":3000}`
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/services", strings.NewReader(body)))
	assertAPIError(t, recorder, http.StatusUnprocessableEntity, "branch_not_accessible")
	services, err := s.store.ListApplicationServices(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 0 {
		t.Fatalf("invalid initial branch created partial services: %#v", services)
	}
}

func TestServicesV2RejectRepositoryOutsideInstallation(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpsertGitHubInstallation(context.Background(), repository.UpsertGitHubInstallationInput{InstallationID: 42, AccountLogin: "acme"}); err != nil {
		t.Fatal(err)
	}
	s.githubRepoLister = fakeGitHubRepositoryLister{repositories: []githubapp.Repository{{CloneURL: "https://github.com/acme/allowed.git"}}}
	body := `{"name":"api","repo_url":"https://github.com/acme/payments.git","github_installation_id":42,"runtime":"auto","internal_port":3000}`
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/services", strings.NewReader(body)))
	assertAPIError(t, recorder, http.StatusUnprocessableEntity, "repository_not_accessible")
	fields := decodeResponse(t, recorder)["fields"].(map[string]any)
	if fields["repo_url"] != "not_accessible_by_installation" {
		t.Fatalf("unexpected field error: %#v", fields)
	}
}

func TestServicesV2RejectSuspendedGitHubInstallation(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpsertGitHubInstallation(context.Background(), repository.UpsertGitHubInstallationInput{InstallationID: 42, AccountLogin: "acme", Suspended: true}); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"api","repo_url":"https://github.com/acme/payments.git","github_installation_id":42,"runtime":"auto","internal_port":3000}`
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+application.ID+"/services", strings.NewReader(body)))
	assertAPIError(t, recorder, http.StatusConflict, "github_installation_suspended")
}

func TestServicesV2AllowNonSourceUpdateWithoutGitHubLookup(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", GitHubInstallationID: 42, InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.handleServices(recorder, httptest.NewRequest(http.MethodPatch, "/api/services/"+service.ID, strings.NewReader(`{"name":"payments-api"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated := decodeResponse(t, recorder)["service"].(map[string]any)
	if updated["name"] != "payments-api" {
		t.Fatalf("unexpected service: %#v", updated)
	}
}

func TestServicesV2ValidateChangedEnvironmentBranch(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	if err := s.store.UpsertGitHubInstallation(context.Background(), repository.UpsertGitHubInstallationInput{InstallationID: 42, AccountLogin: "acme"}); err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments", GitHubInstallationID: 42, InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/services/" + service.ID + "/environments/" + environments[0].ID
	s.githubRepoLister = fakeGitHubRepositoryLister{branches: []string{"main"}}

	rejected := httptest.NewRecorder()
	s.handleServices(rejected, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"branch":"release","auto_deploy":true}`)))
	assertAPIError(t, rejected, http.StatusUnprocessableEntity, "branch_not_accessible")
	fields := decodeResponse(t, rejected)["fields"].(map[string]any)
	if fields["branch"] != "not_found_in_repository" {
		t.Fatalf("unexpected field error: %#v", fields)
	}

	s.githubRepoLister = fakeGitHubRepositoryLister{branches: []string{"main", "release"}}
	accepted := httptest.NewRecorder()
	s.handleServices(accepted, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"branch":"release","auto_deploy":true}`)))
	if accepted.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	binding := decodeResponse(t, accepted)["binding"].(map[string]any)
	if binding["branch"] != "release" || binding["auto_deploy"] != true {
		t.Fatalf("unexpected binding: %#v", binding)
	}
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

func TestDeploymentDetailIncludesActiveReleaseContextAndURL(t *testing.T) {
	s := newAPITestServer(t)
	application, err := s.store.CreateApplication(context.Background(), "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), application.ID)
	if err != nil {
		t.Fatal(err)
	}
	environment := environments[0]
	service, err := s.store.CreateService(context.Background(), repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(context.Background(), repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environment.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.ActivateServiceDeployment(context.Background(), service.ID, environment.ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.CreateServiceDomain(context.Background(), application.ID, environment.ID, service.ID, "payments.example.com"); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	if !s.handleDeploymentV2Detail(recorder, httptest.NewRequest(http.MethodGet, "/api/deployments/"+deployment.ID, nil), deployment.ID) {
		t.Fatal("handler did not consume deployment detail request")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	item := decodeResponse(t, recorder)["deployment"].(map[string]any)
	if item["application_name"] != application.Name || item["service_name"] != service.Name || item["environment_name"] != environment.Name {
		t.Fatalf("missing deployment context: %#v", item)
	}
	if item["is_active"] != true || item["public_url"] != "https://payments.example.com" {
		t.Fatalf("missing active deployment URL: %#v", item)
	}
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

func TestDeploymentsRejectInvalidFilters(t *testing.T) {
	s := newAPITestServer(t)
	tests := []struct {
		path string
		code string
	}{
		{"/api/deployments?status=unknown", "invalid_status"},
		{"/api/deployments?trigger=unknown", "invalid_trigger"},
		{"/api/deployments?date_from=yesterday", "invalid_date_from"},
		{"/api/deployments?date_to=tomorrow", "invalid_date_to"},
		{"/api/deployments?date_from=2026-07-15T23%3A00%3A00Z&date_to=2026-07-15T01%3A00%3A00Z", "invalid_date_range"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		s.handleDeploymentsV2Collection(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		assertAPIError(t, recorder, http.StatusBadRequest, test.code)
	}
}

func TestDeleteGitHubAppResetsOnboardingAndInstallations(t *testing.T) {
	s := newAPITestServer(t)
	if _, err := s.store.UpsertGitHubApp(context.Background(), repository.UpsertGitHubAppInput{AppID: 42, Slug: "hostforge-test", PrivateKeyCT: []byte("sealed")}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpsertGitHubInstallation(context.Background(), repository.UpsertGitHubInstallationInput{InstallationID: 99, AccountLogin: "acme", AccountType: "Organization", TargetType: "Organization", RepoSelection: "selected"}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleGitHubApp(recorder, httptest.NewRequest(http.MethodDelete, "/api/github/app", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	state, err := s.store.GetOnboardingState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.GitHubAppComplete {
		t.Fatal("expected onboarding GitHub App state to reset")
	}
	installations, err := s.store.ListGitHubInstallations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 0 {
		t.Fatalf("expected installations to be removed, got %d", len(installations))
	}
}

func TestGitHubManifestExchangeUsesDocumentedRoute(t *testing.T) {
	s := newAPITestServer(t)

	documented := httptest.NewRecorder()
	s.handleGitHubAppRoutes(documented, httptest.NewRequest(http.MethodPost, "/api/github/app/manifest/exchange", strings.NewReader(`{"code":"test"}`)))
	assertAPIError(t, documented, http.StatusUnsupportedMediaType, "content_type_must_be_application_json")

	legacy := httptest.NewRecorder()
	s.handleGitHubAppRoutes(legacy, httptest.NewRequest(http.MethodPost, "/api/github/app/exchange", strings.NewReader(`{"code":"test"}`)))
	assertAPIError(t, legacy, http.StatusNotFound, "route_not_found")
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

func TestServiceMetricsReadReturnsPersistedSamplesWithoutCollecting(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Metrics", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.UpdateServiceEnvironment(ctx, service.ID, environments[0].ID, "main", false); err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.AttachContainer(ctx, repository.AttachContainerInput{DeploymentID: deployment.ID, DockerContainerID: "docker-metrics", InternalPort: 3000, HostPort: 18080}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.ActivateServiceDeployment(ctx, service.ID, environments[0].ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.InsertServiceMetricSample(ctx, repository.ServiceMetricSample{ServiceID: service.ID, EnvironmentID: environments[0].ID, CPUPercent: 7.5, SampledAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	s.handleServiceMetricsV2(recorder, httptest.NewRequest(http.MethodGet, "/api/services/"+service.ID+"/environments/"+environments[0].ID+"/metrics?points=90", nil), service.ID, environments[0].ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeResponse(t, recorder)
	if payload["supported"] != true || payload["sample_interval_seconds"] != float64(10) || len(payload["samples"].([]any)) != 1 {
		t.Fatalf("unexpected metrics response: %#v", payload)
	}
	samples, err := s.store.ListServiceMetricSamples(ctx, service.ID, environments[0].ID, 10)
	if err != nil || len(samples) != 1 {
		t.Fatalf("metrics read mutated history: samples=%+v err=%v", samples, err)
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

func TestPlatformDomainSettingsRejectInvalidAndUnconfiguredUpdates(t *testing.T) {
	s := newAPITestServer(t)
	invalid := httptest.NewRecorder()
	s.handleSettingsRoutes(invalid, httptest.NewRequest(http.MethodPatch, "/api/settings/platform-domain", strings.NewReader(`{"domain":"not a domain"}`)))
	assertAPIError(t, invalid, http.StatusBadRequest, "invalid_platform_domain")

	unconfigured := httptest.NewRecorder()
	s.handleSettingsRoutes(unconfigured, httptest.NewRequest(http.MethodPatch, "/api/settings/platform-domain", strings.NewReader(`{"domain":"forge.example.com"}`)))
	assertAPIError(t, unconfigured, http.StatusConflict, "platform_domain_not_configured")
}

func TestActiveServiceReportsPlatformDomainRegistrationRequirement(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil {
		t.Fatal(err)
	}
	service, err := s.store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.ActivateServiceDeployment(ctx, service.ID, environments[0].ID, deployment.ID); err != nil {
		t.Fatal(err)
	}
	bindings, err := s.store.ListServiceEnvironments(ctx, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := s.serviceEnvironmentStates(httptest.NewRequest(http.MethodGet, "/api/services/"+service.ID, nil), service, bindings)
	var activeState map[string]any
	for _, state := range states {
		if state["active_deployment_id"] == deployment.ID {
			activeState = state
			break
		}
	}
	if activeState == nil || activeState["public_url_status"] != "platform_domain_required" {
		t.Fatalf("unexpected environment states: %#v", states)
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
