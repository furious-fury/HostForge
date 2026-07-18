package services

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/repository"
)

func TestRequireDatabaseGatewayAddressAvailableRejectsOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	address := listener.Addr().String()
	err = requireDatabaseGatewayAddressAvailable(address)
	if err == nil || !strings.Contains(err.Error(), "already occupied") {
		t.Fatalf("occupied address %q was accepted: %v", address, err)
	}
}

func TestPostgreSQLGatewayProbeUsesLoopbackWithHostnameVerification(t *testing.T) {
	environment := strings.Join(postgreSQLGatewayProbeEnvironment("secret"), "\n")
	for _, required := range []string{
		"PGPASSWORD=secret",
		"PGSSLMODE=verify-full",
		"PGSSLROOTCERT=system",
		"PGHOSTADDR=127.0.0.1",
	} {
		if !strings.Contains(environment, required) {
			t.Fatalf("gateway probe environment is missing %q: %s", required, environment)
		}
	}
}

func TestPostgreSQLGatewayRuntimeVersionsMatchSecureImageContract(t *testing.T) {
	if err := ValidatePostgreSQLGatewayRuntimeVersions("PgBouncer 1.25.2\nlibevent 2.1.12", "psql (PostgreSQL) 16.14 - Percona Distribution", "1.25.2"); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		pgBouncer string
		psql      string
		declared  string
	}{
		"missing psql":        {"PgBouncer 1.25.2", "", "1.25.2"},
		"old psql":            {"PgBouncer 1.25.2", "psql (PostgreSQL) 15.12", "1.25.2"},
		"old PgBouncer":       {"PgBouncer 1.25.1", "psql (PostgreSQL) 16.14", "1.25.1"},
		"mismatched declared": {"PgBouncer 1.25.2", "psql (PostgreSQL) 16.14", "1.26.0"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePostgreSQLGatewayRuntimeVersions(test.pgBouncer, test.psql, test.declared); err == nil {
				t.Fatal("incompatible gateway runtime was accepted")
			}
		})
	}
}

func TestPostgreSQLGatewayRendererEnforcesBackendAndClientLimits(t *testing.T) {
	adapter := NewPostgreSQLGatewayAdapter(nil)
	generation, err := adapter.Render(context.Background(), GatewayRenderRequest{
		Hostname:   "postgres.apps.example.test",
		Generation: 1,
		Routes: []GatewayRenderRoute{{
			RouteAlias: "hf_instance", BackendAlias: "hfb_instance", BackendPort: 5432,
			DatabaseName: "app", RoutePoolSize: 10, DesiredStatus: "active",
			Credentials: []GatewayRenderCredential{{
				RoleName: "hfc_credential", SCRAMVerifier: testSCRAMVerifier,
				CIDRs: []string{"203.0.113.8/32"}, BackendPoolSize: 5, ClientLimit: 20,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ini := string(generation.PgBouncerINI)
	if !strings.Contains(ini, "hfc_credential = pool_size=5 max_user_connections=5 max_user_client_connections=20") {
		t.Fatalf("per-credential backend/client limits are missing:\n%s", ini)
	}
}

func TestPostgreSQLPermissionReconciliationRemovesOwnershipAndArbitraryGrants(t *testing.T) {
	for _, profile := range []string{"read_only", "read_write", "migration"} {
		script := PostgreSQLPermissionSQL("app", "app_owner", "hfc_credential", profile)
		if !strings.Contains(script, `REASSIGN OWNED BY "hfc_credential" TO "app_owner";`) || !strings.Contains(script, `DROP OWNED BY "hfc_credential";`) {
			t.Fatalf("%s does not remove credential ownership and arbitrary grants:\n%s", profile, script)
		}
		if !strings.Contains(script, `ALTER DEFAULT PRIVILEGES FOR ROLE "app_owner" REVOKE ALL ON SCHEMAS FROM "hfc_credential";`) {
			t.Fatalf("%s does not revoke future schema grants:\n%s", profile, script)
		}
		if profile != "migration" && !strings.Contains(script, `ALTER DEFAULT PRIVILEGES FOR ROLE "app_owner" GRANT USAGE ON SCHEMAS TO "hfc_credential";`) {
			t.Fatalf("%s does not grant future schema usage:\n%s", profile, script)
		}
	}
}

func TestDatabaseGatewayCertificateReconcilerQueuesChangedMaterialOnce(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	endpoint, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.QueueDatabaseGatewayProvision(ctx, endpoint.Engine, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDatabaseGatewayOperation(ctx, initial.ID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDatabaseGatewayEndpointState(ctx, endpoint.Engine, "active", "active", "gateway-container", 1, 1, 1, "", ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.SetDatabaseGatewayCertificate(ctx, endpoint.Engine, "old-fingerprint", now.Add(20*24*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	certificate, privateKey := gatewayTestCertificate(t, endpoint.Hostname, now.Add(30*24*time.Hour))
	directory := t.TempDir()
	certificatePath := filepath.Join(directory, "gateway.crt")
	privateKeyPath := filepath.Join(directory, "gateway.key")
	if err := os.WriteFile(certificatePath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privateKeyPath, privateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{PostgreSQLGatewayCertificateFile: certificatePath, PostgreSQLGatewayKeyFile: privateKeyPath}
	if err := reconcileDatabaseGatewayCertificate(ctx, cfg, store, now); err != nil {
		t.Fatal(err)
	}
	if err := reconcileDatabaseGatewayCertificate(ctx, cfg, store, now); err != nil {
		t.Fatal(err)
	}
	active, err := store.HasActiveDatabaseGatewayOperation(ctx, endpoint.Engine, "provision_gateway")
	if err != nil || !active {
		t.Fatalf("renewal operation active=%t err=%v", active, err)
	}
	claimed, err := store.ClaimNextDatabaseGatewayOperation(ctx, "renewal-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.OperationType != "provision_gateway" || claimed.Actor != "system:certificate-renewal" {
		t.Fatalf("unexpected renewal operation: %+v", claimed)
	}
	if _, err := store.ClaimNextDatabaseGatewayOperation(ctx, "duplicate-worker", time.Minute); err != sql.ErrNoRows {
		t.Fatalf("certificate renewal was queued more than once: %v", err)
	}
}

func TestDatabaseGatewayStartupReconciliationQueuesOnceForActiveEndpoint(t *testing.T) {
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, filepath.Join(t.TempDir(), "hostforge.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := repository.New(db)
	endpoint, err := store.EnsureDatabaseGatewayEndpoint(ctx, "postgresql", "postgres.apps.example.test", "pgbouncer@sha256:test", "1.25.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDatabaseGatewayEndpointState(ctx, endpoint.Engine, "active", "active", "gateway-container", 1, 1, 1, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := queueDatabaseGatewayStartupReconciliation(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := queueDatabaseGatewayStartupReconciliation(ctx, store); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextDatabaseGatewayOperation(ctx, "startup-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.OperationType != "provision_gateway" || claimed.Actor != "system:startup-reconciliation" {
		t.Fatalf("unexpected startup reconciliation: %+v", claimed)
	}
	if _, err := store.ClaimNextDatabaseGatewayOperation(ctx, "duplicate-worker", time.Minute); err != sql.ErrNoRows {
		t.Fatalf("startup reconciliation was queued more than once: %v", err)
	}
}

func TestPgBouncerClientSelectionTargetsOnlyMatchingRoleOrRoute(t *testing.T) {
	output := strings.Join([]string{
		"type,user,database,id",
		"C,hfc_target,hf_other,client-role",
		"C,hfc_other,hf_target,client-route",
		"C,hfc_other,hf_other,client-unrelated",
		"",
	}, "\n")
	clientIDs, err := pgBouncerClientIDs(output, map[string]struct{}{"hfc_target": {}}, "hf_target")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(clientIDs, ",") != "client-role,client-route" {
		t.Fatalf("targeted client IDs=%v", clientIDs)
	}

	unsafe := strings.Join([]string{
		"type,user,database,id",
		"C,hfc_target,hf_other,client;DROP",
		"",
	}, "\n")
	if _, err := pgBouncerClientIDs(unsafe, map[string]struct{}{"hfc_target": {}}, ""); err == nil {
		t.Fatal("unsafe PgBouncer client identifier was accepted")
	}
}

func TestPgBouncerActiveRolesReturnsUniqueExternalCredentialsOnly(t *testing.T) {
	output := strings.Join([]string{
		"type,user,database,id",
		"C,hfc_z,hf_one,client-one",
		"C,hostforge_gateway_admin,pgbouncer,admin",
		"C,hfc_a,hf_two,client-two",
		"C,hfc_z,hf_one,client-three",
		"C,unsafe-role,hf_one,client-four",
		"",
	}, "\n")
	roles, err := pgBouncerActiveRoles(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(roles, ",") != "hfc_a,hfc_z" {
		t.Fatalf("active external roles=%v", roles)
	}
}
