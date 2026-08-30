package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/repository"
)

func createGatewayAPIInstance(t *testing.T, server *server, engine string) repository.DatabaseInstance {
	t.Helper()
	ctx := context.Background()
	application, err := server.store.CreateApplication(ctx, "Gateway API", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := server.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("environments=%d err=%v", len(environments), err)
	}
	created, err := server.store.CreateDatabaseService(ctx, repository.CreateDatabaseServiceInput{ApplicationID: application.ID, Name: engine, Engine: engine, DefaultVersion: "18", Instances: []repository.CreateDatabaseInstanceInput{{EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: engine + "@sha256:test", NetworkAlias: "gateway-api-" + engine, InternalPort: 5432, VolumeName: "hostforge-gateway-api-" + engine, ResourcePreset: "development", CPULimitMillis: 500, MemoryLimitBytes: 512 * 1024 * 1024, DatabaseName: "app", Username: "app_owner", PasswordCT: []byte{1}, AdminPasswordCT: []byte{2}}}})
	if err != nil {
		t.Fatal(err)
	}
	return created.Instances[0]
}

func configureGatewayAPIServer(t *testing.T, server *server) {
	t.Helper()
	server.cfg.DatabaseGatewaysEnabled = true
	server.cfg.PostgreSQLGatewayImage = "pgbouncer@sha256:test"
	server.cfg.PostgreSQLGatewayVersion = "1.25.2"
	if err := server.store.MarkGitHubAppComplete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := server.store.CompleteOnboarding(context.Background(), "apps.example.test"); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseGatewayStatusExposesFoundationForUnsupportedEngines(t *testing.T) {
	server := newAPITestServer(t)
	recorder := httptest.NewRecorder()
	server.handleDatabaseGateways(recorder, httptest.NewRequest(http.MethodGet, "/api/database-gateways/mysql", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeResponse(t, recorder)
	if payload["adapter_available"] != false || payload["unavailable_reason"] != "external_access_engine_unsupported" {
		t.Fatalf("unsupported foundation payload=%+v", payload)
	}
	if _, leaked := payload["reserved_hostname"]; leaked {
		t.Fatalf("unsupported engine leaked a PostgreSQL endpoint: %+v", payload)
	}
}

func TestDatabaseGatewayMutationsAreFeatureGated(t *testing.T) {
	server := newAPITestServer(t)
	recorder := httptest.NewRecorder()
	server.handleDatabaseGateways(recorder, httptest.NewRequest(http.MethodPost, "/api/database-gateways/postgresql", nil))
	assertAPIError(t, recorder, http.StatusServiceUnavailable, "database_gateways_disabled")
}

func TestDatabaseExternalConnectionAPIQueuesEnvironmentScopedDesiredStateAndGuardsOpenCIDRs(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	instance := createGatewayAPIInstance(t, server, "postgresql")

	open := httptest.NewRecorder()
	server.handleDatabaseInstances(open, httptest.NewRequest(http.MethodPost, "/api/database-instances/"+instance.ID+"/external-connections", strings.NewReader(`{"name":"Public","profile":"read_only","cidrs":["0.0.0.0/0"]}`)))
	assertAPIError(t, open, http.StatusUnprocessableEntity, "external_access_open_confirmation_required")

	create := httptest.NewRecorder()
	server.handleDatabaseInstances(create, httptest.NewRequest(http.MethodPost, "/api/database-instances/"+instance.ID+"/external-connections", strings.NewReader(`{"name":"Reporting","profile":"read_only","cidrs":["203.0.113.9/24"]}`)))
	if create.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", create.Code, create.Body.String())
	}
	payload := decodeResponse(t, create)
	operation := payload["operation"].(map[string]any)
	if operation["operation_type"] != "create_connection" || operation["status"] != "queued" {
		t.Fatalf("operation=%+v", operation)
	}

	detail := httptest.NewRecorder()
	server.handleDatabaseInstances(detail, httptest.NewRequest(http.MethodGet, "/api/database-instances/"+instance.ID+"/external-access", nil))
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "203.0.113.0/24") || strings.Contains(detail.Body.String(), "password_ct") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
}

func TestDatabaseExternalCredentialRevealIsNoStoreAndPercentEncoded(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	instance := createGatewayAPIInstance(t, server, "postgresql")
	if _, err := server.ensurePostgreSQLGatewayEndpoint(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
		t.Fatal(err)
	}
	connection, _, err := server.store.CreateDatabaseExternalConnection(context.Background(), instance.ID, repository.CreateExternalConnectionInput{Name: "Client", PermissionProfile: "read_write", CIDRs: []string{"198.51.100.7/32"}})
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("a/b:c@d?#%")
	passwordCT, err := server.envSealer.Seal(password)
	if err != nil {
		t.Fatal(err)
	}
	verifierCT, err := server.envSealer.Seal([]byte("SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:a2V5"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.CreateDatabaseExternalCredential(context.Background(), connection.ID, passwordCT, verifierCT); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetDatabaseExternalConnectionStatus(context.Background(), connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(recorder, httptest.NewRequest(http.MethodPost, "/api/database-external-connections/"+connection.ID+"/credentials", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("credential response is cacheable: %+v", recorder.Header())
	}
	var response struct {
		Password string `json:"password"`
		URL      string `json:"url"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Password != string(password) || strings.Contains(response.URL, string(password)) || !strings.Contains(response.URL, "sslmode=verify-full") {
		t.Fatalf("unsafe reveal response: %+v", response)
	}
}
