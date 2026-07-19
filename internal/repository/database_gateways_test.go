package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func createGatewayTestInstance(t *testing.T, store *Store) DatabaseInstance {
	return createGatewayTestInstanceNamed(t, store, "test")
}

func createGatewayTestInstanceNamed(t *testing.T, store *Store, suffix string) DatabaseInstance {
	t.Helper()
	ctx := context.Background()
	app, err := store.CreateApplication(ctx, "Gateway "+suffix, "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := store.ListApplicationEnvironments(ctx, app.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("environments=%d err=%v", len(environments), err)
	}
	created, err := store.CreateDatabaseService(ctx, CreateDatabaseServiceInput{
		ApplicationID: app.ID, Name: "postgres", Engine: "postgresql", DefaultVersion: "18",
		Instances: []CreateDatabaseInstanceInput{{
			EnvironmentID: environments[0].ID, EngineVersion: "18", ImageRef: "postgres@sha256:test",
			NetworkAlias: "gateway-postgres-" + suffix, InternalPort: 5432, VolumeName: "hostforge-db-gateway-" + suffix,
			ResourcePreset: "standard", CPULimitMillis: 1000, MemoryLimitBytes: 2 * 1024 * 1024 * 1024,
			DatabaseName: "app", Username: "app", PasswordCT: []byte{1, 2, 3},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.Instances[0]
}

func TestNormalizeExternalAccessCIDRsCanonicalizesAndGuardsOpenNetworks(t *testing.T) {
	cidrs, err := NormalizeExternalAccessCIDRs([]string{"10.0.0.42/24", "2001:db8::4/64", "10.0.0.1/24"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cidrs, ","); got != "10.0.0.0/24,2001:db8::/64" {
		t.Fatalf("canonical CIDRs=%q", got)
	}
	if _, err := NormalizeExternalAccessCIDRs([]string{"0.0.0.0/0"}, false); !errors.Is(err, ErrOpenAccessConfirmationRequired) {
		t.Fatalf("open IPv4 was not guarded: %v", err)
	}
	if _, err := NormalizeExternalAccessCIDRs([]string{"::/0"}, true); err != nil {
		t.Fatalf("confirmed open IPv6 was rejected: %v", err)
	}
	if _, err := NormalizeExternalAccessCIDRs([]string{"not-a-prefix"}, true); !errors.Is(err, ErrInvalidExternalAccessCIDR) {
		t.Fatalf("invalid CIDR error=%v", err)
	}
}

func TestDatabaseGatewayConnectionStateIsTransactionalAndSecretsStaySealed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstance(t, store)
	endpoint, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2")
	if err != nil {
		t.Fatal(err)
	}
	connection, operation, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Reporting", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.9/24"}, Actor: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if operation.ConnectionID != connection.ID || operation.OperationType != "create_connection" || operation.Status != "queued" {
		t.Fatalf("unexpected operation: %+v", operation)
	}
	access, err := store.GetDatabaseExternalAccess(ctx, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if access.Endpoint == nil || access.Endpoint.Engine != endpoint.Engine || access.Route == nil || access.Route.RouteAlias != "hf_"+instance.ID {
		t.Fatalf("unexpected external access: %+v", access)
	}
	if access.Route.RouteBackendLimit != 25 || access.Route.CredentialBackendLimit != 10 {
		t.Fatalf("standard budget=%d/%d", access.Route.RouteBackendLimit, access.Route.CredentialBackendLimit)
	}
	if len(access.Connections) != 1 || strings.Join(access.Connections[0].CIDRs, ",") != "203.0.113.0/24" {
		t.Fatalf("unexpected connections: %+v", access.Connections)
	}

	credential, err := store.CreateDatabaseExternalCredential(ctx, connection.ID, []byte("sealed-password"), []byte("sealed-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	if credential.Generation != 1 || !strings.HasPrefix(credential.RoleName, "hfc_") {
		t.Fatalf("unexpected credential: %+v", credential)
	}
	publicConnection, err := store.GetDatabaseExternalConnection(ctx, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(publicConnection.Credentials) != 1 || publicConnection.Credentials[0].PasswordCT != nil || publicConnection.Credentials[0].SCRAMVerifierCT != nil {
		t.Fatalf("ciphertext leaked through detail response: %+v", publicConnection.Credentials)
	}
	sealed, err := store.GetDatabaseExternalCredentialSealed(ctx, credential.ID)
	if err != nil || string(sealed.PasswordCT) != "sealed-password" || string(sealed.SCRAMVerifierCT) != "sealed-verifier" {
		t.Fatalf("sealed credential unavailable: %+v err=%v", sealed, err)
	}
	if err := store.RevokeDatabaseExternalCredential(ctx, credential.ID); err != nil {
		t.Fatal(err)
	}
	sealed, _ = store.GetDatabaseExternalCredentialSealed(ctx, credential.ID)
	if sealed.State != "revoked" || len(sealed.PasswordCT) != 0 || len(sealed.SCRAMVerifierCT) != 0 {
		t.Fatalf("revocation retained secret material: %+v", sealed)
	}
}

func TestDatabaseGatewayOperationsLeaseRequeueAndExpiryClaim(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstance(t, store)
	_, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour)
	connection, operation, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{Name: "Temporary", PermissionProfile: "read_write", CIDRs: []string{"198.51.100.4/32"}, ExpiresAt: expires})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimNextDatabaseGatewayOperation(ctx, "worker-too-early", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("initial public connection raced database provisioning: %v", err)
	}
	if _, err := store.UpdateDatabaseInstanceState(ctx, instance.ID, UpdateDatabaseInstanceStateInput{DesiredState: "running", Status: "healthy", HealthCheckedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextDatabaseGatewayOperation(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != operation.ID || claimed.AttemptCount != 1 || claimed.Status != "running" {
		t.Fatalf("unexpected claim: %+v", claimed)
	}
	if _, err := store.ClaimNextDatabaseGatewayOperation(ctx, "worker-b", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("active lease was not exclusive: %v", err)
	}
	if count, err := store.RequeueExpiredDatabaseGatewayOperations(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil || count != 1 {
		t.Fatalf("requeue count=%d err=%v", count, err)
	}
	if _, err := store.UpdateDatabaseGatewayOperation(ctx, operation.ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDatabaseExternalConnectionStatus(ctx, connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	queued, err := store.QueueDueDatabaseExternalConnectionExpirations(ctx, expires.Add(time.Second), 10)
	if err != nil || queued != 1 {
		t.Fatalf("expiry claim count=%d err=%v", queued, err)
	}
	expired, _ := store.GetDatabaseExternalConnection(ctx, connection.ID)
	if expired.Status != "expired" {
		t.Fatalf("expiry desired state=%q", expired.Status)
	}
}

func TestGatewayAliasesAndPoolBudgetsRemainSafe(t *testing.T) {
	alias, err := GatewayRouteAlias("ABC-123_Unsafe")
	if err != nil || alias != "hf_abc123unsafe" {
		t.Fatalf("alias=%q err=%v", alias, err)
	}
	role, err := GatewayCredentialRole("CRED-01")
	if err != nil || role != "hfc_cred01" {
		t.Fatalf("role=%q err=%v", role, err)
	}
	for _, test := range []struct {
		preset string
		memory int64
		route  int
		cred   int
	}{{"development", 1, 10, 5}, {"standard", 1, 25, 10}, {"performance", 1, 50, 20}, {"custom", 512 * 1024 * 1024, 10, 10}, {"custom", 1536 * 1024 * 1024, 15, 10}, {"custom", 8 * 1024 * 1024 * 1024, 50, 10}} {
		route, credential := GatewayPoolBudget(test.preset, test.memory)
		if route != test.route || credential != test.cred {
			t.Fatalf("%s budget=%d/%d", test.preset, route, credential)
		}
	}
}

func TestDatabaseGatewaySchemaEnforcesAliasesRolesAndCascades(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	firstInstance := createGatewayTestInstanceNamed(t, store, "first")
	secondInstance := createGatewayTestInstanceNamed(t, store, "second")
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}

	firstConnection, firstOperation, err := store.CreateDatabaseExternalConnection(ctx, firstInstance.ID, CreateExternalConnectionInput{
		Name: "First", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.1/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondConnection, _, err := store.CreateDatabaseExternalConnection(ctx, secondInstance.ID, CreateExternalConnectionInput{
		Name: "Second", PermissionProfile: "read_write", CIDRs: []string{"198.51.100.2/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRoute, err := store.GetDatabaseGatewayRouteByInstance(ctx, firstInstance.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondRoute, err := store.GetDatabaseGatewayRouteByInstance(ctx, secondInstance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE database_gateway_routes SET route_alias = ? WHERE id = ?`, firstRoute.RouteAlias, secondRoute.ID); err == nil {
		t.Fatal("duplicate immutable route alias was accepted")
	}

	firstCredential, err := store.CreateDatabaseExternalCredential(ctx, firstConnection.ID, []byte("first-password"), []byte("first-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	secondCredential, err := store.CreateDatabaseExternalCredential(ctx, secondConnection.ID, []byte("second-password"), []byte("second-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE database_external_credentials SET role_name = ? WHERE id = ?`, firstCredential.RoleName, secondCredential.ID); err == nil {
		t.Fatal("duplicate immutable credential role was accepted")
	}

	if _, err := store.db.ExecContext(ctx, `DELETE FROM database_instances WHERE id = ?`, firstInstance.ID); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		table  string
		column string
		value  string
	}{
		{"database_gateway_routes", "database_instance_id", firstInstance.ID},
		{"database_external_connections", "route_id", firstRoute.ID},
		{"database_external_credentials", "connection_id", firstConnection.ID},
		{"database_external_connection_cidrs", "connection_id", firstConnection.ID},
	} {
		var count int
		query := `SELECT COUNT(*) FROM ` + check.table + ` WHERE ` + check.column + ` = ?`
		if err := store.db.QueryRowContext(ctx, query, check.value).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s did not cascade: count=%d", check.table, count)
		}
	}
	var routeID, connectionID sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT route_id, connection_id FROM database_gateway_operations WHERE id = ?`, firstOperation.ID).Scan(&routeID, &connectionID); err != nil {
		t.Fatal(err)
	}
	if routeID.Valid || connectionID.Valid {
		t.Fatalf("retained operation references were not cleared: route=%+v connection=%+v", routeID, connectionID)
	}
}

func completeGatewayDomainTestOnboarding(t *testing.T, store *Store, domain string) {
	t.Helper()
	ctx := context.Background()
	if err := store.MarkGitHubAppComplete(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteOnboarding(ctx, domain); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformDomainChangeRequiresExternalConnectionRevocation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	completeGatewayDomainTestOnboarding(t, store, "apps.example.test")
	instance := createGatewayTestInstanceNamed(t, store, "domain-connection")
	if _, err := store.EnsureDatabaseGatewayEndpointForPlatformDomain(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2", "apps.example.test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Domain guard", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.8/32"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlatformDomain(ctx, "apps.example.test", "forge.example.test"); !errors.Is(err, ErrGatewayHasActiveConnections) {
		t.Fatalf("domain change was not blocked by an unreleased external connection: %v", err)
	}
	state, err := store.GetOnboardingState(ctx)
	if err != nil || state.PlatformDomain != "apps.example.test" {
		t.Fatalf("blocked domain change mutated onboarding: state=%+v err=%v", state, err)
	}
}

func TestPlatformDomainChangeRequiresTeardownAndRejectsStaleEndpointDiscovery(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	completeGatewayDomainTestOnboarding(t, store, "apps.example.test")
	if _, err := store.EnsureDatabaseGatewayEndpointForPlatformDomain(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2", "apps.example.test"); err != nil {
		t.Fatal(err)
	}
	operation, err := store.QueueDatabaseGatewayProvision(ctx, "postgresql", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlatformDomain(ctx, "apps.example.test", "forge.example.test"); !errors.Is(err, ErrGatewayTeardownRequired) {
		t.Fatalf("domain change was not blocked by a provisioned gateway: %v", err)
	}
	if _, err := store.UpdateDatabaseGatewayOperation(ctx, operation.ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearDatabaseGatewayEndpointRuntime(ctx, "postgresql"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlatformDomain(ctx, "apps.example.test", "forge.example.test"); err != nil {
		t.Fatal(err)
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if err != nil || endpoint.Hostname != "postgres.forge.example.test" || endpoint.DesiredStatus != "absent" || endpoint.ObservedStatus != "absent" {
		t.Fatalf("torn-down endpoint was not moved safely: endpoint=%+v err=%v", endpoint, err)
	}
	if _, err := store.EnsureDatabaseGatewayEndpointForPlatformDomain(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2", "apps.example.test"); !errors.Is(err, ErrPlatformDomainChanged) {
		t.Fatalf("stale endpoint discovery was accepted after domain change: %v", err)
	}
	if _, err := store.EnsureDatabaseGatewayEndpointForPlatformDomain(ctx, "postgresql", "postgres.forge.example.test", "pgbouncer@sha256:test", "1.25.2", "forge.example.test"); err != nil {
		t.Fatalf("current endpoint discovery failed: %v", err)
	}
}

func TestGatewayTeardownExcludesReprovisionAndConnectionCreation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstanceNamed(t, store, "teardown-race")
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	teardown, err := store.QueueDatabaseGatewayTeardown(ctx, "postgresql", "operator")
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if err != nil || endpoint.DesiredStatus != "deleting" || endpoint.ObservedStatus != "deleting" {
		t.Fatalf("gateway teardown did not reserve the endpoint: endpoint=%+v err=%v", endpoint, err)
	}
	if _, err := store.QueueDatabaseGatewayTeardown(ctx, "postgresql", "operator"); !errors.Is(err, ErrInvalidExternalConnectionState) {
		t.Fatalf("duplicate active teardown was accepted: %v", err)
	}
	if _, err := store.UpdateDatabaseGatewayOperation(ctx, teardown.ID, "failed", "failed", 50, "database_gateway_teardown_failed", "transient Docker failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueueDatabaseGatewayTeardown(ctx, "postgresql", "operator:retry"); err != nil {
		t.Fatalf("terminally failed teardown could not be retried: %v", err)
	}
	if _, err := store.QueueDatabaseGatewayProvision(ctx, "postgresql", "operator"); !errors.Is(err, ErrInvalidExternalConnectionState) {
		t.Fatalf("reprovision was accepted during teardown: %v", err)
	}
	if _, _, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Teardown race", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.10/32"},
	}); !errors.Is(err, ErrInvalidExternalConnectionState) {
		t.Fatalf("connection creation was accepted during teardown: %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_external_connections`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected teardown-race creation left state: count=%d err=%v", count, err)
	}
}

func TestFailedExternalConnectionRevocationCanBeRetriedWithoutDuplicates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstanceNamed(t, store, "revoke-retry")
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	connection, _, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{
		Name: "Revoke retry", PermissionProfile: "read_only", CIDRs: []string{"203.0.113.11/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDatabaseExternalConnectionStatus(ctx, connection.ID, "active", "", ""); err != nil {
		t.Fatal(err)
	}
	revoke, err := store.QueueDatabaseExternalConnectionAction(ctx, connection.ID, "revoke", 0, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueueDatabaseExternalConnectionAction(ctx, connection.ID, "revoke", 0, "operator"); !errors.Is(err, ErrInvalidExternalConnectionState) {
		t.Fatalf("duplicate active revocation was accepted: %v", err)
	}
	if _, err := store.UpdateDatabaseGatewayOperation(ctx, revoke.ID, "failed", "failed", 50, "database_gateway_reload_failed", "transient reload failure"); err != nil {
		t.Fatal(err)
	}
	retry, err := store.QueueDatabaseExternalConnectionAction(ctx, connection.ID, "revoke", 0, "operator:retry")
	if err != nil {
		t.Fatalf("terminally failed revocation could not be retried: %v", err)
	}
	if retry.OperationType != "revoke_connection" || retry.Status != "queued" {
		t.Fatalf("unexpected retry operation: %+v", retry)
	}
}

func TestInitialExternalConnectionFailsWhenDatabaseProvisioningFails(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	instance := createGatewayTestInstanceNamed(t, store, "initial-failure")
	if _, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer:test", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	connection, operation, err := store.CreateDatabaseExternalConnection(ctx, instance.ID, CreateExternalConnectionInput{Name: "Initial access", PermissionProfile: "read_write", CIDRs: []string{"198.51.100.9/32"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FailQueuedInitialDatabaseExternalConnections(ctx, instance.ID, "database_health_failed", "database did not become healthy"); err != nil {
		t.Fatal(err)
	}
	failedOperation, err := store.GetDatabaseGatewayOperation(ctx, operation.ID)
	if err != nil || failedOperation.Status != "failed" || failedOperation.ErrorCode != "database_health_failed" {
		t.Fatalf("dependent operation was not failed: operation=%+v err=%v", failedOperation, err)
	}
	failedConnection, err := store.GetDatabaseExternalConnection(ctx, connection.ID)
	if err != nil || failedConnection.Status != "failed" || failedConnection.LastErrorCode != "database_health_failed" {
		t.Fatalf("initial connection was not failed: connection=%+v err=%v", failedConnection, err)
	}
}
