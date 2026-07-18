package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hostforge/hostforge/internal/repository"
)

func TestDatabaseGatewayProvisionTeardownAndOperationStatusContract(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)

	provision := httptest.NewRecorder()
	server.handleDatabaseGateways(provision, httptest.NewRequest(http.MethodPost, "/api/database-gateways/postgresql", nil))
	if provision.Code != http.StatusAccepted {
		t.Fatalf("provision status=%d body=%s", provision.Code, provision.Body.String())
	}
	provisionPayload := decodeResponse(t, provision)
	provisionOperation := provisionPayload["operation"].(map[string]any)
	if provisionOperation["operation_type"] != "provision_gateway" || provisionOperation["status"] != "queued" {
		t.Fatalf("provision operation=%+v", provisionOperation)
	}

	status := httptest.NewRecorder()
	server.handleDatabaseGatewayOperations(status, httptest.NewRequest(http.MethodGet, "/api/database-gateway-operations/"+provisionOperation["id"].(string), nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"operation_type":"provision_gateway"`) {
		t.Fatalf("operation status=%d body=%s", status.Code, status.Body.String())
	}

	unknown := httptest.NewRecorder()
	server.handleDatabaseGatewayOperations(unknown, httptest.NewRequest(http.MethodGet, "/api/database-gateway-operations/missing", nil))
	assertAPIError(t, unknown, http.StatusNotFound, "database_gateway_operation_not_found")

	teardown := httptest.NewRecorder()
	server.handleDatabaseGateways(teardown, httptest.NewRequest(http.MethodDelete, "/api/database-gateways/postgresql", strings.NewReader(`{"confirmation":"TEAR DOWN POSTGRESQL GATEWAY"}`)))
	if teardown.Code != http.StatusAccepted || !strings.Contains(teardown.Body.String(), `"operation_type":"teardown_gateway"`) {
		t.Fatalf("teardown status=%d body=%s", teardown.Code, teardown.Body.String())
	}
}

func TestDatabaseGatewayTeardownRequiresTypedConfirmationAndNoConnections(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	instance := createGatewayAPIInstance(t, server, "postgresql")
	if _, err := server.ensurePostgreSQLGatewayEndpoint(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
		t.Fatal(err)
	}

	wrong := httptest.NewRecorder()
	server.handleDatabaseGateways(wrong, httptest.NewRequest(http.MethodDelete, "/api/database-gateways/postgresql", strings.NewReader(`{"confirmation":"yes"}`)))
	assertAPIError(t, wrong, http.StatusUnprocessableEntity, "gateway_teardown_confirmation_required")

	if _, _, err := server.store.CreateDatabaseExternalConnection(context.Background(), instance.ID, repository.CreateExternalConnectionInput{
		Name: "Active guard", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.8/32"},
	}); err != nil {
		t.Fatal(err)
	}
	guarded := httptest.NewRecorder()
	server.handleDatabaseGateways(guarded, httptest.NewRequest(http.MethodDelete, "/api/database-gateways/postgresql", strings.NewReader(`{"confirmation":"TEAR DOWN POSTGRESQL GATEWAY"}`)))
	assertAPIError(t, guarded, http.StatusConflict, "database_gateway_has_active_connections")
}

func TestPlatformDomainChangeRequiresCompletedGatewayTeardown(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	if _, err := server.ensurePostgreSQLGatewayEndpoint(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.store.QueueDatabaseGatewayProvision(context.Background(), "postgresql", "operator"); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.handleSettingsRoutes(recorder, httptest.NewRequest(
		http.MethodPatch,
		"/api/settings/platform-domain",
		strings.NewReader(`{"domain":"forge.example.test"}`),
	))
	assertAPIError(t, recorder, http.StatusConflict, "database_gateway_teardown_required")
}

func TestDatabaseExternalConnectionAPIRejectsUnsupportedEngineAndInvalidInputs(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	mysql := createGatewayAPIInstance(t, server, "mysql")

	unsupported := httptest.NewRecorder()
	server.handleDatabaseInstances(unsupported, httptest.NewRequest(http.MethodPost, "/api/database-instances/"+mysql.ID+"/external-connections", strings.NewReader(`{"name":"Client","profile":"read_only","cidrs":["203.0.113.8/32"]}`)))
	assertAPIError(t, unsupported, http.StatusUnprocessableEntity, "external_access_engine_unsupported")

	postgres := createGatewayAPIInstance(t, server, "postgresql")
	for _, test := range []struct {
		name string
		body string
		code string
	}{
		{"profile", `{"name":"Client","profile":"superuser","cidrs":["203.0.113.8/32"]}`, "invalid_external_access_profile"},
		{"CIDR", `{"name":"Client","profile":"read_only","cidrs":[]}`, "invalid_external_access_cidr"},
		{"expiry", `{"name":"Client","profile":"read_only","cidrs":["203.0.113.8/32"],"expires_at":"tomorrow"}`, "invalid_external_access_expiry"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.handleDatabaseInstances(recorder, httptest.NewRequest(http.MethodPost, "/api/database-instances/"+postgres.ID+"/external-connections", strings.NewReader(test.body)))
			assertAPIError(t, recorder, http.StatusUnprocessableEntity, test.code)
		})
	}
}

func TestDatabaseExternalConnectionUpdateRotateAndRevokeContract(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	instance := createGatewayAPIInstance(t, server, "postgresql")
	if _, err := server.ensurePostgreSQLGatewayEndpoint(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
		t.Fatal(err)
	}
	connection, _, err := server.store.CreateDatabaseExternalConnection(context.Background(), instance.ID, repository.CreateExternalConnectionInput{
		Name: "Laptop", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.8/32"},
	})
	if err != nil {
		t.Fatal(err)
	}

	update := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(update, httptest.NewRequest(http.MethodPatch, "/api/database-external-connections/"+connection.ID, strings.NewReader(`{"name":"CI","profile":"read_write","cidrs":["198.51.100.4/24"],"confirm_open_access":false}`)))
	if update.Code != http.StatusAccepted || !strings.Contains(update.Body.String(), `"operation_type":"update_connection"`) || !strings.Contains(update.Body.String(), "198.51.100.0/24") {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}

	if err := server.store.SetDatabaseExternalConnectionStatus(context.Background(), connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	rotate := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(rotate, httptest.NewRequest(http.MethodPost, "/api/database-external-connections/"+connection.ID+"/rotate", strings.NewReader(`{"grace_period_hours":168}`)))
	if rotate.Code != http.StatusAccepted || !strings.Contains(rotate.Body.String(), `"operation_type":"rotate_connection"`) || !strings.Contains(rotate.Body.String(), `"requested_grace_period_hours":168`) {
		t.Fatalf("rotate status=%d body=%s", rotate.Code, rotate.Body.String())
	}

	if err := server.store.SetDatabaseExternalConnectionStatus(context.Background(), connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	wrongRevoke := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(wrongRevoke, httptest.NewRequest(http.MethodPost, "/api/database-external-connections/"+connection.ID+"/revoke", strings.NewReader(`{}`)))
	assertAPIError(t, wrongRevoke, http.StatusUnprocessableEntity, "external_connection_revoke_confirmation_required")

	revoke := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(revoke, httptest.NewRequest(http.MethodPost, "/api/database-external-connections/"+connection.ID+"/revoke", strings.NewReader(`{"confirmation":"REVOKE EXTERNAL CONNECTION"}`)))
	if revoke.Code != http.StatusAccepted || !strings.Contains(revoke.Body.String(), `"operation_type":"revoke_connection"`) {
		t.Fatalf("revoke status=%d body=%s", revoke.Code, revoke.Body.String())
	}
}

func TestDatabaseExternalCredentialRevealRejectsInactiveConnections(t *testing.T) {
	server := newAPITestServer(t)
	configureGatewayAPIServer(t, server)
	instance := createGatewayAPIInstance(t, server, "postgresql")
	if _, err := server.ensurePostgreSQLGatewayEndpoint(httptest.NewRequest(http.MethodPost, "/", nil)); err != nil {
		t.Fatal(err)
	}
	connection, _, err := server.store.CreateDatabaseExternalConnection(context.Background(), instance.ID, repository.CreateExternalConnectionInput{
		Name: "Pending", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.8/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.handleDatabaseExternalConnections(recorder, httptest.NewRequest(http.MethodPost, "/api/database-external-connections/"+connection.ID+"/credentials", nil))
	assertAPIError(t, recorder, http.StatusConflict, "invalid_external_connection_state")
}
