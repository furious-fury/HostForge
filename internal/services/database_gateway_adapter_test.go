package services

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/furious-fury/HostForge/internal/repository"
)

const testSCRAMVerifier = "SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVk:a2V5"

type fakePostgreSQLGatewayRuntime struct {
	output       string
	err          error
	scripts      []string
	environments [][]string
	reloads      int
	probes       int
	terminations []GatewayTerminationRequest
}

func (runtime *fakePostgreSQLGatewayRuntime) RunSQL(_ context.Context, _, _, _, script string, environment []string) (string, error) {
	runtime.scripts = append(runtime.scripts, script)
	runtime.environments = append(runtime.environments, append([]string(nil), environment...))
	return runtime.output, runtime.err
}

func (runtime *fakePostgreSQLGatewayRuntime) ReloadPgBouncer(context.Context) error {
	runtime.reloads++
	return runtime.err
}

func (runtime *fakePostgreSQLGatewayRuntime) ProbePostgreSQL(context.Context, GatewayProbeRequest) error {
	runtime.probes++
	return runtime.err
}

func (runtime *fakePostgreSQLGatewayRuntime) TerminatePgBouncer(_ context.Context, request GatewayTerminationRequest) error {
	runtime.terminations = append(runtime.terminations, request)
	return runtime.err
}

func TestPostgreSQLGatewayRenderIsDeterministicDenyByDefaultAndSCRAMOnly(t *testing.T) {
	adapter := NewPostgreSQLGatewayAdapter(nil)
	request := GatewayRenderRequest{Hostname: "postgres.apps.example.test", Generation: 3, Routes: []GatewayRenderRoute{
		{RouteAlias: "hf_b", BackendAlias: "hfb_b", BackendPort: 5432, DatabaseName: "app_b", RoutePoolSize: 25, DesiredStatus: "active", Credentials: []GatewayRenderCredential{{RoleName: "hfc_b", SCRAMVerifier: testSCRAMVerifier, CIDRs: []string{"2001:db8::4/64", "203.0.113.9/24"}, BackendPoolSize: 10, ClientLimit: 20}}},
		{RouteAlias: "hf_a", BackendAlias: "hfb_a", BackendPort: 5432, DatabaseName: "app_a", RoutePoolSize: 10, DesiredStatus: "active", Credentials: []GatewayRenderCredential{{RoleName: "hfc_a", SCRAMVerifier: testSCRAMVerifier, CIDRs: []string{"198.51.100.4/32"}, BackendPoolSize: 5, ClientLimit: 20}}},
	}}
	first, err := adapter.Render(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Render(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.PgBouncerINI) != string(second.PgBouncerINI) || string(first.HBA) != string(second.HBA) || GatewayGenerationFingerprint(first) != GatewayGenerationFingerprint(second) {
		t.Fatal("same desired state did not render deterministically")
	}
	ini, hba, auth := string(first.PgBouncerINI), string(first.HBA), string(first.Userlist)
	if strings.Index(ini, "hf_a =") > strings.Index(ini, "hf_b =") || !strings.Contains(ini, "pool_mode = session") || !strings.Contains(ini, "auth_type = hba") || !strings.Contains(ini, "client_tls_sslmode = require") {
		t.Fatalf("unsafe or unstable PgBouncer config:\n%s", ini)
	}
	if !strings.Contains(hba, "hostssl hf_b hfc_b 203.0.113.0/24 scram-sha-256") || !strings.Contains(hba, "hostssl hf_b hfc_b 2001:db8::/64 scram-sha-256") || !strings.HasSuffix(hba, "hostssl all all 0.0.0.0/0 reject\nhostssl all all ::/0 reject\n") {
		t.Fatalf("unsafe HBA:\n%s", hba)
	}
	if !strings.Contains(hba, "local pgbouncer hostforge_gateway_admin trust") || !strings.Contains(hba, "hostssl pgbouncer all 0.0.0.0/0 reject") {
		t.Fatalf("socket-only admin HBA contract is missing:\n%s", hba)
	}
	if strings.Contains(auth, "password") || !strings.Contains(auth, testSCRAMVerifier) || !strings.Contains(auth, `"hostforge_gateway_admin" "`+pgBouncerLocalAdminSCRAMVerifier+`"`) {
		t.Fatalf("auth file does not contain exact SCRAM-only material: %s", auth)
	}
	if err := adapter.Validate(context.Background(), first); err != nil {
		t.Fatalf("valid generation rejected: %v", err)
	}
}

func TestPostgreSQLGatewayRejectsNonSCRAMAndUnsafeImageCatalog(t *testing.T) {
	adapter := NewPostgreSQLGatewayAdapter(nil)
	_, err := adapter.Render(context.Background(), GatewayRenderRequest{Hostname: "postgres.example.test", Generation: 1, Routes: []GatewayRenderRoute{{RouteAlias: "hf_a", BackendAlias: "hfb_a", BackendPort: 5432, DatabaseName: "app", RoutePoolSize: 10, DesiredStatus: "active", Credentials: []GatewayRenderCredential{{RoleName: "hfc_a", SCRAMVerifier: "plaintext", CIDRs: []string{"127.0.0.1/32"}, BackendPoolSize: 5, ClientLimit: 20}}}}})
	if err == nil {
		t.Fatal("plaintext auth material was accepted")
	}
	for _, test := range []struct {
		image   string
		version string
		valid   bool
	}{{"pgbouncer:latest", "1.25.2", false}, {"pgbouncer@sha256:abc", "1.25.1", false}, {"pgbouncer@sha256:abc", "1.25.2", true}, {"pgbouncer@sha256:abc", "1.26.0", true}} {
		err := ValidatePgBouncerImage(test.image, test.version)
		if (err == nil) != test.valid {
			t.Fatalf("image=%q version=%q valid=%t err=%v", test.image, test.version, test.valid, err)
		}
	}
}

func TestPostgreSQLPermissionProfilesRevokeBeforeGrantAndCoverFutureObjects(t *testing.T) {
	for _, profile := range []string{"read_only", "read_write", "migration"} {
		sql := PostgreSQLPermissionSQL("app", "app_owner", "hfc_credential", profile)
		if !strings.Contains(sql, `ALTER ROLE "hfc_credential" IN DATABASE "app" RESET role;`) {
			t.Fatalf("%s does not clear stale login-time owner activation:\n%s", profile, sql)
		}
		if !strings.Contains(sql, `REVOKE "app_owner" FROM "hfc_credential"`) || !strings.Contains(sql, `ALTER DEFAULT PRIVILEGES FOR ROLE "app_owner" REVOKE ALL`) {
			t.Fatalf("%s does not revoke obsolete permissions first:\n%s", profile, sql)
		}
		switch profile {
		case "read_only":
			if !strings.Contains(sql, "GRANT SELECT ON TABLES") || strings.Contains(sql, "GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES") || strings.Contains(sql, "SET role TO") {
				t.Fatalf("read-only future grants are incorrect:\n%s", sql)
			}
		case "read_write":
			if !strings.Contains(sql, "GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES") || strings.Contains(sql, "CREATE ON SCHEMA") || strings.Contains(sql, "SET role TO") {
				t.Fatalf("read-write future grants are incorrect:\n%s", sql)
			}
		case "migration":
			if !strings.Contains(sql, `GRANT "app_owner" TO "hfc_credential"`) || !strings.Contains(sql, `ALTER ROLE "hfc_credential" IN DATABASE "app" SET role TO 'app_owner';`) || strings.Contains(sql, "SUPERUSER") {
				t.Fatalf("migration owner membership is incorrect:\n%s", sql)
			}
		}
	}
}

func TestPostgreSQLRoleProvisionUsesExactVerifierAndRedactsPasswordFailures(t *testing.T) {
	runtime := &fakePostgreSQLGatewayRuntime{output: testSCRAMVerifier + "\n"}
	adapter := NewPostgreSQLGatewayAdapter(runtime)
	password := "A_very-secret-generated-password"
	material, err := adapter.ProvisionRole(context.Background(), GatewayRoleRequest{ContainerID: "container", DatabaseName: "app", ApplicationOwnerRole: "app_owner", AdminPassword: "admin-secret", RoleName: "hfc_credential", Password: password, PermissionProfile: "read_only"})
	if err != nil || material.SCRAMVerifier != testSCRAMVerifier {
		t.Fatalf("material=%+v err=%v", material, err)
	}
	if len(runtime.scripts) != 2 || strings.Contains(runtime.scripts[0], password) || len(runtime.environments) == 0 || !strings.Contains(strings.Join(runtime.environments[0], " "), password) {
		t.Fatalf("password command transport is incorrect: scripts=%d env=%v", len(runtime.scripts), runtime.environments)
	}
	runtime = &fakePostgreSQLGatewayRuntime{err: errors.New("database rejected " + password)}
	adapter = NewPostgreSQLGatewayAdapter(runtime)
	_, err = adapter.ProvisionRole(context.Background(), GatewayRoleRequest{ContainerID: "container", DatabaseName: "app", ApplicationOwnerRole: "app_owner", AdminPassword: "admin-secret", RoleName: "hfc_credential", Password: password, PermissionProfile: "read_only"})
	if err == nil || strings.Contains(err.Error(), password) {
		t.Fatalf("password leaked through adapter error: %v", err)
	}
}

func TestPostgreSQLConnectionURLPercentEncodesSecrets(t *testing.T) {
	adapter := NewPostgreSQLGatewayAdapter(nil)
	connectionURL, err := adapter.ConnectionURL(ConnectionURLRequest{Username: "hfc_credential", Password: "a/b:c@d?#%", Hostname: "postgres.example.test", Port: 5432, Alias: "hf_instance"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(connectionURL)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.User.Username() != "hfc_credential" || password != "a/b:c@d?#%" || parsed.Path != "/hf_instance" || parsed.Query().Get("sslmode") != "verify-full" || strings.Contains(connectionURL, "a/b:c@d?#%") {
		t.Fatalf("unsafe URL=%q parsed=%+v", connectionURL, parsed)
	}
}

func TestWriteDatabaseGatewayGenerationUsesPrivateAtomicGenerations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("production activation uses an atomic Unix symlink")
	}
	adapter := NewPostgreSQLGatewayAdapter(nil)
	generation, err := adapter.Render(context.Background(), GatewayRenderRequest{Hostname: "postgres.example.test", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "postgresql")
	path, err := WriteDatabaseGatewayGeneration(root, generation, []byte("certificate"), []byte("private-key"))
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("symlink activation is not available on this development host: %v", err)
		}
		t.Fatal(err)
	}
	for _, name := range []string{"pgbouncer.ini", "pg_hba.conf", "userlist.txt", "manifest.txt", "server.crt", "server.key"} {
		info, err := os.Stat(filepath.Join(path, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions=%o", name, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(root, "current", "pgbouncer.ini")); err != nil {
		t.Fatalf("current generation is not active: %v", err)
	}
	if _, err := WriteDatabaseGatewayGeneration(root, generation, []byte("certificate"), []byte("private-key")); err == nil {
		t.Fatal("existing immutable generation was overwritten")
	}
}

func TestDatabaseGatewayGenerationCanRollbackAfterFailedReload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("production activation uses an atomic Unix symlink")
	}
	adapter := NewPostgreSQLGatewayAdapter(nil)
	root := filepath.Join(t.TempDir(), "postgresql")
	first, err := adapter.Render(context.Background(), GatewayRenderRequest{Hostname: "postgres.example.test", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDatabaseGatewayGeneration(root, first, []byte("certificate"), []byte("private-key")); err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Render(context.Background(), GatewayRenderRequest{Hostname: "postgres.example.test", Generation: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteInactiveDatabaseGatewayGeneration(root, second, []byte("certificate"), []byte("private-key")); err != nil {
		t.Fatal(err)
	}
	previous, err := ActivateDatabaseGatewayGeneration(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if previous != filepath.Join("generations", "1") {
		t.Fatalf("previous generation=%q", previous)
	}
	if err := RestoreDatabaseGatewayGeneration(root, previous); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join("generations", "1") {
		t.Fatalf("rollback target=%q", target)
	}
}

func TestGatewayRegistryRegistersOnlyPostgreSQLV1(t *testing.T) {
	registry := NewGatewayAdapterRegistry(NewPostgreSQLGatewayAdapter(nil))
	if !registry.Available("postgresql") {
		t.Fatal("PostgreSQL adapter is unavailable")
	}
	for _, engine := range []string{"mysql", "mariadb", "mongodb", "redis", "valkey"} {
		if _, err := registry.Get(engine); !errors.Is(err, ErrExternalAccessEngineUnsupported) {
			t.Fatalf("%s adapter error=%v", engine, err)
		}
	}
	route, credential := repository.GatewayPoolBudget("custom", 6*1024*1024*1024)
	if route != 50 || credential != 10 {
		t.Fatalf("custom pool clamp=%d/%d", route, credential)
	}
}
