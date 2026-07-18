package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/repository"
)

// PgBouncer requires trust-authenticated users to exist in auth_file. This
// verifier is only an existence sentinel for the Unix-socket-only admin role;
// public HBA rules reject that role and the pgbouncer database.
const pgBouncerLocalAdminSCRAMVerifier = "SCRAM-SHA-256$4096:aG9zdGZvcmdlLXBnYm91bmNlci1hZG1pbg==$sfio0UzVE4D/9icOzaicsEBcobc4zG5wYy4a8l28Jyw=:/xmwld83xHddQ8XcW/IJMVUZlk/BWPdpuJxpJdleg1M="

var ErrExternalAccessEngineUnsupported = errors.New("external_access_engine_unsupported")

type GatewayEndpointRequest struct {
	PlatformDomain string
}

type GatewayEndpointSpec struct {
	Engine   string
	Hostname string
	Port     int
}

type ConnectionURLRequest struct {
	Username string
	Password string
	Hostname string
	Port     int
	Alias    string
}

type GatewayRoleRequest struct {
	ContainerID          string
	DatabaseName         string
	ApplicationOwnerRole string
	AdminPassword        string
	RoleName             string
	Password             string
	PermissionProfile    string
}

type GatewayRoleMaterial struct {
	SCRAMVerifier string
}

type GatewayPermissionRequest struct {
	ContainerID          string
	DatabaseName         string
	ApplicationOwnerRole string
	AdminPassword        string
	RoleName             string
	PermissionProfile    string
}

type GatewayRevokeRoleRequest struct {
	ContainerID          string
	DatabaseName         string
	ApplicationOwnerRole string
	AdminPassword        string
	RoleName             string
}

type GatewayRenderCredential struct {
	RoleName        string
	SCRAMVerifier   string
	CIDRs           []string
	BackendPoolSize int
	ClientLimit     int
	ConnectionID    string
	CredentialID    string
}

type GatewayRenderRoute struct {
	RouteAlias      string
	BackendAlias    string
	BackendPort     int
	DatabaseName    string
	RoutePoolSize   int
	DesiredStatus   string
	Credentials     []GatewayRenderCredential
	DatabaseRouteID string
}

type GatewayRenderRequest struct {
	Hostname   string
	Generation int
	Routes     []GatewayRenderRoute
}

type GatewayGeneration struct {
	Generation   int
	PgBouncerINI []byte
	HBA          []byte
	Userlist     []byte
	Manifest     []byte
}

type GatewayProbeRequest struct {
	Hostname string
	Port     int
	Alias    string
	Username string
	Password string
}

type GatewayTerminationRequest struct {
	RoleNames  []string
	RouteAlias string
}

type DatabaseGatewayAdapter interface {
	Engine() string
	Endpoint(context.Context, GatewayEndpointRequest) (GatewayEndpointSpec, error)
	ConnectionURL(ConnectionURLRequest) (string, error)
	ProvisionRole(context.Context, GatewayRoleRequest) (GatewayRoleMaterial, error)
	ReconcilePermissions(context.Context, GatewayPermissionRequest) error
	RevokeRole(context.Context, GatewayRevokeRoleRequest) error
	Render(context.Context, GatewayRenderRequest) (GatewayGeneration, error)
	Validate(context.Context, GatewayGeneration) error
	Reload(context.Context, GatewayGeneration) error
	Probe(context.Context, GatewayProbeRequest) error
	Terminate(context.Context, GatewayTerminationRequest) error
}

type GatewayAdapterRegistry struct {
	adapters map[string]DatabaseGatewayAdapter
}

func NewGatewayAdapterRegistry(adapters ...DatabaseGatewayAdapter) *GatewayAdapterRegistry {
	registry := &GatewayAdapterRegistry{adapters: map[string]DatabaseGatewayAdapter{}}
	for _, adapter := range adapters {
		if adapter != nil {
			registry.adapters[adapter.Engine()] = adapter
		}
	}
	return registry
}

// NewDatabaseGatewayAdapterRegistry is the v1 adapter catalog. PostgreSQL is
// intentionally the only registered public data-plane protocol.
func NewDatabaseGatewayAdapterRegistry() *GatewayAdapterRegistry {
	return NewGatewayAdapterRegistry(NewPostgreSQLGatewayAdapter(nil))
}

func (r *GatewayAdapterRegistry) Get(engine string) (DatabaseGatewayAdapter, error) {
	if r == nil {
		return nil, ErrExternalAccessEngineUnsupported
	}
	adapter, ok := r.adapters[strings.ToLower(strings.TrimSpace(engine))]
	if !ok {
		return nil, ErrExternalAccessEngineUnsupported
	}
	return adapter, nil
}

func (r *GatewayAdapterRegistry) Available(engine string) bool {
	_, err := r.Get(engine)
	return err == nil
}

type PostgreSQLGatewayRuntime interface {
	RunSQL(context.Context, string, string, string, string, []string) (string, error)
	ReloadPgBouncer(context.Context) error
	ProbePostgreSQL(context.Context, GatewayProbeRequest) error
	TerminatePgBouncer(context.Context, GatewayTerminationRequest) error
}

type PostgreSQLGatewayAdapter struct {
	runtime PostgreSQLGatewayRuntime
}

func NewPostgreSQLGatewayAdapter(runtime PostgreSQLGatewayRuntime) *PostgreSQLGatewayAdapter {
	return &PostgreSQLGatewayAdapter{runtime: runtime}
}

func (a *PostgreSQLGatewayAdapter) Engine() string { return "postgresql" }

func (a *PostgreSQLGatewayAdapter) Endpoint(_ context.Context, request GatewayEndpointRequest) (GatewayEndpointSpec, error) {
	base := strings.ToLower(strings.Trim(strings.TrimSpace(request.PlatformDomain), "."))
	if base == "" {
		return GatewayEndpointSpec{}, ErrCode("database_gateway_platform_domain_required", errors.New("platform domain is required"))
	}
	return GatewayEndpointSpec{Engine: a.Engine(), Hostname: "postgres." + base, Port: repository.DefaultGatewayPort}, nil
}

func (a *PostgreSQLGatewayAdapter) ConnectionURL(request ConnectionURLRequest) (string, error) {
	if !gatewayIdentifier(request.Username) || !gatewayIdentifier(request.Alias) || strings.TrimSpace(request.Password) == "" || strings.TrimSpace(request.Hostname) == "" || request.Port < 1 || request.Port > 65535 {
		return "", fmt.Errorf("valid PostgreSQL connection material required")
	}
	value := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(request.Username, request.Password),
		Host:     net.JoinHostPort(strings.TrimSpace(request.Hostname), strconv.Itoa(request.Port)),
		Path:     "/" + request.Alias,
		RawQuery: "sslmode=verify-full",
	}
	return value.String(), nil
}

func (a *PostgreSQLGatewayAdapter) ProvisionRole(ctx context.Context, request GatewayRoleRequest) (GatewayRoleMaterial, error) {
	if a.runtime == nil {
		return GatewayRoleMaterial{}, errors.New("PostgreSQL gateway runtime is unavailable")
	}
	if err := validateGatewayRoleRequest(request.RoleName, request.DatabaseName, request.ApplicationOwnerRole, request.PermissionProfile); err != nil {
		return GatewayRoleMaterial{}, err
	}
	if strings.TrimSpace(request.Password) == "" || strings.ContainsAny(request.Password, "\x00\r\n") || strings.TrimSpace(request.AdminPassword) == "" {
		return GatewayRoleMaterial{}, errors.New("safe PostgreSQL password and administrator credential required")
	}
	script := PostgreSQLProvisionRoleSQL(request.RoleName)
	output, err := a.runtime.RunSQL(ctx, request.ContainerID, request.DatabaseName, request.AdminPassword, script, []string{"PGOPTIONS=-c hostforge.gateway_password=" + request.Password})
	if err != nil {
		return GatewayRoleMaterial{}, safeDatabaseOperationError(err, []byte(request.Password), []byte(request.AdminPassword))
	}
	verifier := strings.TrimSpace(output)
	if !validSCRAMVerifier(verifier) {
		return GatewayRoleMaterial{}, errors.New("PostgreSQL did not return an exact SCRAM verifier")
	}
	if err := a.ReconcilePermissions(ctx, GatewayPermissionRequest{ContainerID: request.ContainerID, DatabaseName: request.DatabaseName, ApplicationOwnerRole: request.ApplicationOwnerRole, AdminPassword: request.AdminPassword, RoleName: request.RoleName, PermissionProfile: request.PermissionProfile}); err != nil {
		return GatewayRoleMaterial{}, err
	}
	return GatewayRoleMaterial{SCRAMVerifier: verifier}, nil
}

func (a *PostgreSQLGatewayAdapter) ReconcilePermissions(ctx context.Context, request GatewayPermissionRequest) error {
	if a.runtime == nil {
		return errors.New("PostgreSQL gateway runtime is unavailable")
	}
	if err := validateGatewayRoleRequest(request.RoleName, request.DatabaseName, request.ApplicationOwnerRole, request.PermissionProfile); err != nil {
		return err
	}
	if strings.TrimSpace(request.AdminPassword) == "" {
		return errors.New("PostgreSQL administrator credential required")
	}
	_, err := a.runtime.RunSQL(ctx, request.ContainerID, request.DatabaseName, request.AdminPassword, PostgreSQLPermissionSQL(request.DatabaseName, request.ApplicationOwnerRole, request.RoleName, request.PermissionProfile), nil)
	return safeNilDatabaseGatewayError(err)
}

func (a *PostgreSQLGatewayAdapter) RevokeRole(ctx context.Context, request GatewayRevokeRoleRequest) error {
	if a.runtime == nil {
		return errors.New("PostgreSQL gateway runtime is unavailable")
	}
	if !gatewayIdentifier(request.RoleName) || !postgresIdentifier(request.DatabaseName) || !postgresIdentifier(request.ApplicationOwnerRole) {
		return errors.New("safe PostgreSQL identifiers required")
	}
	if strings.TrimSpace(request.AdminPassword) == "" {
		return errors.New("PostgreSQL administrator credential required")
	}
	_, err := a.runtime.RunSQL(ctx, request.ContainerID, request.DatabaseName, request.AdminPassword, PostgreSQLRevokeRoleSQL(request.DatabaseName, request.ApplicationOwnerRole, request.RoleName), nil)
	return safeNilDatabaseGatewayError(err)
}

func (a *PostgreSQLGatewayAdapter) Render(_ context.Context, request GatewayRenderRequest) (GatewayGeneration, error) {
	if request.Generation < 1 || strings.TrimSpace(request.Hostname) == "" {
		return GatewayGeneration{}, errors.New("gateway hostname and positive generation required")
	}
	routes := append([]GatewayRenderRoute(nil), request.Routes...)
	sort.Slice(routes, func(i, j int) bool { return routes[i].RouteAlias < routes[j].RouteAlias })
	var databases, users, hba, userlist strings.Builder
	databases.WriteString("[databases]\n")
	users.WriteString("\n[users]\n")
	hba.WriteString("# Generated by HostForge. Unmatched traffic is denied.\n")
	hba.WriteString("local pgbouncer hostforge_gateway_admin trust\n")
	hba.WriteString("hostnossl all all 0.0.0.0/0 reject\n")
	hba.WriteString("hostnossl all all ::/0 reject\n")
	hba.WriteString("hostssl pgbouncer all 0.0.0.0/0 reject\n")
	hba.WriteString("hostssl pgbouncer all ::/0 reject\n")
	credentials := []GatewayRenderCredential{}
	seenRoles := map[string]struct{}{}
	for _, route := range routes {
		if route.DesiredStatus != "active" {
			continue
		}
		if !gatewayIdentifier(route.RouteAlias) || !gatewayIdentifier(route.BackendAlias) || !postgresIdentifier(route.DatabaseName) || route.BackendPort < 1 || route.BackendPort > 65535 || route.RoutePoolSize < 1 || route.RoutePoolSize > 50 {
			return GatewayGeneration{}, errors.New("invalid PostgreSQL gateway route")
		}
		fmt.Fprintf(&databases, "%s = host=%s port=%d dbname=%s pool_size=%d max_db_connections=%d\n", route.RouteAlias, route.BackendAlias, route.BackendPort, route.DatabaseName, route.RoutePoolSize, route.RoutePoolSize)
		for _, credential := range route.Credentials {
			if !gatewayIdentifier(credential.RoleName) || !validSCRAMVerifier(credential.SCRAMVerifier) || credential.BackendPoolSize < 1 || credential.BackendPoolSize > route.RoutePoolSize || credential.ClientLimit < 1 || credential.ClientLimit > repository.DefaultCredentialClientLimit {
				return GatewayGeneration{}, errors.New("invalid PostgreSQL gateway credential")
			}
			if _, duplicate := seenRoles[credential.RoleName]; duplicate {
				return GatewayGeneration{}, errors.New("duplicate PostgreSQL gateway role")
			}
			seenRoles[credential.RoleName] = struct{}{}
			credential.CIDRs, _ = repository.NormalizeExternalAccessCIDRs(credential.CIDRs, true)
			if len(credential.CIDRs) == 0 {
				return GatewayGeneration{}, errors.New("gateway credential requires at least one CIDR")
			}
			for _, cidr := range credential.CIDRs {
				fmt.Fprintf(&hba, "hostssl %s %s %s scram-sha-256\n", route.RouteAlias, credential.RoleName, cidr)
			}
			credentials = append(credentials, credential)
		}
	}
	sort.Slice(credentials, func(i, j int) bool { return credentials[i].RoleName < credentials[j].RoleName })
	fmt.Fprintf(&userlist, "\"hostforge_gateway_admin\" \"%s\"\n", pgBouncerLocalAdminSCRAMVerifier)
	for _, credential := range credentials {
		fmt.Fprintf(&users, "%s = pool_size=%d max_user_connections=%d max_user_client_connections=%d\n", credential.RoleName, credential.BackendPoolSize, credential.BackendPoolSize, credential.ClientLimit)
		fmt.Fprintf(&userlist, "\"%s\" \"%s\"\n", credential.RoleName, credential.SCRAMVerifier)
	}
	hba.WriteString("hostssl all all 0.0.0.0/0 reject\n")
	hba.WriteString("hostssl all all ::/0 reject\n")
	ini := databases.String() + users.String() + `
[pgbouncer]
listen_addr = 0.0.0.0,::
listen_port = 5432
unix_socket_dir = /run/pgbouncer
unix_socket_mode = 0770
admin_users = hostforge_gateway_admin
stats_users = hostforge_gateway_admin
pool_mode = session
auth_type = hba
auth_file = /etc/hostforge-gateway/current/userlist.txt
auth_hba_file = /etc/hostforge-gateway/current/pg_hba.conf
client_tls_sslmode = require
client_tls_key_file = /etc/hostforge-gateway/current/server.key
client_tls_cert_file = /etc/hostforge-gateway/current/server.crt
client_tls_protocols = secure
server_tls_sslmode = disable
max_client_conn = 200
ignore_startup_parameters = extra_float_digits
`
	manifest := fmt.Sprintf("engine=postgresql\nhostname=%s\ngeneration=%d\nroutes=%d\ncredentials=%d\n", strings.ToLower(request.Hostname), request.Generation, len(routes), len(credentials))
	return GatewayGeneration{Generation: request.Generation, PgBouncerINI: []byte(ini), HBA: []byte(hba.String()), Userlist: []byte(userlist.String()), Manifest: []byte(manifest)}, nil
}

func (a *PostgreSQLGatewayAdapter) Validate(_ context.Context, generation GatewayGeneration) error {
	if generation.Generation < 1 || len(generation.PgBouncerINI) == 0 || len(generation.HBA) == 0 || len(generation.Manifest) == 0 {
		return errors.New("incomplete PostgreSQL gateway generation")
	}
	ini, hba := string(generation.PgBouncerINI), string(generation.HBA)
	for _, required := range []string{"pool_mode = session", "auth_type = hba", "client_tls_sslmode = require", "client_tls_protocols = secure", "max_client_conn = 200", "unix_socket_dir = /run/pgbouncer"} {
		if !strings.Contains(ini, required) {
			return fmt.Errorf("gateway configuration missing %s", required)
		}
	}
	if !strings.HasSuffix(hba, "hostssl all all 0.0.0.0/0 reject\nhostssl all all ::/0 reject\n") {
		return errors.New("gateway HBA does not end in IPv4/IPv6 rejection")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(generation.Userlist)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\" \"", 2)
		if len(parts) != 2 || !validSCRAMVerifier(strings.TrimSuffix(parts[1], "\"")) {
			return errors.New("gateway userlist contains a non-SCRAM secret")
		}
	}
	return nil
}

func (a *PostgreSQLGatewayAdapter) Reload(ctx context.Context, _ GatewayGeneration) error {
	if a.runtime == nil {
		return errors.New("PostgreSQL gateway runtime is unavailable")
	}
	return safeNilDatabaseGatewayError(a.runtime.ReloadPgBouncer(ctx))
}

func (a *PostgreSQLGatewayAdapter) Probe(ctx context.Context, request GatewayProbeRequest) error {
	if a.runtime == nil {
		return errors.New("PostgreSQL gateway runtime is unavailable")
	}
	return safeNilDatabaseGatewayError(a.runtime.ProbePostgreSQL(ctx, request))
}

func (a *PostgreSQLGatewayAdapter) Terminate(ctx context.Context, request GatewayTerminationRequest) error {
	if a.runtime == nil {
		return errors.New("PostgreSQL gateway runtime is unavailable")
	}
	for _, role := range request.RoleNames {
		if !gatewayIdentifier(role) {
			return errors.New("unsafe PostgreSQL role in termination request")
		}
	}
	if request.RouteAlias != "" && !gatewayIdentifier(request.RouteAlias) {
		return errors.New("unsafe PostgreSQL alias in termination request")
	}
	return safeNilDatabaseGatewayError(a.runtime.TerminatePgBouncer(ctx, request))
}

func safeNilDatabaseGatewayError(err error) error {
	if err == nil {
		return nil
	}
	return safeDatabaseOperationError(err)
}

func GenerateDatabaseGatewayPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate gateway credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ValidatePgBouncerImage(imageRef, version string) error {
	if !strings.Contains(strings.TrimSpace(imageRef), "@sha256:") {
		return errors.New("PgBouncer image must be digest pinned")
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(parts) < 3 {
		return errors.New("PgBouncer version must include major, minor, and patch")
	}
	values := make([]int, 3)
	for index := range values {
		value, err := strconv.Atoi(parts[index])
		if err != nil {
			return errors.New("invalid PgBouncer version")
		}
		values[index] = value
	}
	if values[0] < 1 || (values[0] == 1 && values[1] < 25) || (values[0] == 1 && values[1] == 25 && values[2] < 2) {
		return errors.New("PgBouncer 1.25.2 or newer is required")
	}
	return nil
}

func PostgreSQLProvisionRoleSQL(roleName string) string {
	quoted := quotePostgresIdentifier(roleName)
	literal := quotePostgresLiteral(roleName)
	return fmt.Sprintf(`DO $hostforge$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = %s) THEN
    EXECUTE format('ALTER ROLE %%I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %%L', %s, current_setting('hostforge.gateway_password'));
  ELSE
    EXECUTE format('CREATE ROLE %%I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %%L', %s, current_setting('hostforge.gateway_password'));
  END IF;
END
$hostforge$;
SELECT rolpassword FROM pg_authid WHERE rolname = %s;
-- role identifier: %s
`, literal, literal, literal, literal, quoted)
}

func PostgreSQLPermissionSQL(databaseName, ownerRole, roleName, profile string) string {
	database, owner, role := quotePostgresIdentifier(databaseName), quotePostgresIdentifier(ownerRole), quotePostgresIdentifier(roleName)
	var grants strings.Builder
	fmt.Fprintf(&grants, "ALTER ROLE %s IN DATABASE %s RESET role;\n", role, database)
	fmt.Fprintf(&grants, "REASSIGN OWNED BY %s TO %s;\n", role, owner)
	fmt.Fprintf(&grants, "DROP OWNED BY %s;\n", role)
	fmt.Fprintf(&grants, "REVOKE %s FROM %s;\n", owner, role)
	fmt.Fprintf(&grants, "REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s;\n", database, role)
	grants.WriteString(`DO $hostforge$
DECLARE schema_name text;
BEGIN
  FOR schema_name IN SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg_%' AND nspname <> 'information_schema' LOOP
`)
	fmt.Fprintf(&grants, "    EXECUTE format('REVOKE ALL PRIVILEGES ON SCHEMA %%I FROM %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	fmt.Fprintf(&grants, "    EXECUTE format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA %%I FROM %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	fmt.Fprintf(&grants, "    EXECUTE format('REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %%I FROM %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	grants.WriteString("  END LOOP;\nEND\n$hostforge$;\n")
	fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s REVOKE ALL ON TABLES FROM %s;\n", owner, role)
	fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s REVOKE ALL ON SEQUENCES FROM %s;\n", owner, role)
	fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s REVOKE ALL ON SCHEMAS FROM %s;\n", owner, role)
	fmt.Fprintf(&grants, "GRANT CONNECT ON DATABASE %s TO %s;\n", database, role)
	if profile == "migration" {
		fmt.Fprintf(&grants, "GRANT %s TO %s;\n", owner, role)
		fmt.Fprintf(&grants, "ALTER ROLE %s IN DATABASE %s SET role TO %s;\n", role, database, quotePostgresLiteral(ownerRole))
		return grants.String()
	}
	grants.WriteString(`DO $hostforge$
DECLARE schema_name text;
BEGIN
  FOR schema_name IN SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg_%' AND nspname <> 'information_schema' LOOP
`)
	fmt.Fprintf(&grants, "    EXECUTE format('GRANT USAGE ON SCHEMA %%I TO %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	if profile == "read_only" {
		fmt.Fprintf(&grants, "    EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %%I TO %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
		fmt.Fprintf(&grants, "    EXECUTE format('GRANT SELECT ON ALL SEQUENCES IN SCHEMA %%I TO %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	} else {
		fmt.Fprintf(&grants, "    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %%I TO %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
		fmt.Fprintf(&grants, "    EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %%I TO %%I', schema_name, %s);\n", quotePostgresLiteral(roleName))
	}
	grants.WriteString("  END LOOP;\nEND\n$hostforge$;\n")
	fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT USAGE ON SCHEMAS TO %s;\n", owner, role)
	if profile == "read_only" {
		fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT SELECT ON TABLES TO %s;\n", owner, role)
		fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT SELECT ON SEQUENCES TO %s;\n", owner, role)
	} else {
		fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s;\n", owner, role)
		fmt.Fprintf(&grants, "ALTER DEFAULT PRIVILEGES FOR ROLE %s GRANT USAGE, SELECT ON SEQUENCES TO %s;\n", owner, role)
	}
	return grants.String()
}

func PostgreSQLRevokeRoleSQL(databaseName, ownerRole, roleName string) string {
	base := PostgreSQLPermissionSQL(databaseName, ownerRole, roleName, "read_only")
	marker := fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s;", quotePostgresIdentifier(databaseName), quotePostgresIdentifier(roleName))
	if index := strings.Index(base, marker); index >= 0 {
		base = base[:index]
	}
	return base + fmt.Sprintf("ALTER ROLE %s NOLOGIN;\nDROP ROLE IF EXISTS %s;\n", quotePostgresIdentifier(roleName), quotePostgresIdentifier(roleName))
}

func validateGatewayRoleRequest(role, database, owner, profile string) error {
	if !gatewayIdentifier(role) || !postgresIdentifier(database) || !postgresIdentifier(owner) || !validExternalGatewayProfile(profile) {
		return errors.New("safe PostgreSQL role, database, owner, and permission profile required")
	}
	return nil
}

func validExternalGatewayProfile(profile string) bool {
	return profile == "read_only" || profile == "read_write" || profile == "migration"
}

func gatewayIdentifier(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, character := range value {
		if (character < 'a' || character > 'z') && character != '_' && (index == 0 || character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func postgresIdentifier(value string) bool {
	if value == "" || len(value) > 63 || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	return true
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quotePostgresLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

func validSCRAMVerifier(value string) bool {
	if !strings.HasPrefix(value, "SCRAM-SHA-256$") || strings.ContainsAny(value, "\x00\r\n\"") {
		return false
	}
	parts := strings.Split(value, "$")
	return len(parts) == 3 && strings.Contains(parts[1], ":") && strings.Contains(parts[2], ":")
}

// WriteDatabaseGatewayGeneration writes a complete private generation and
// activates it through an atomic current symlink replacement.
func WriteDatabaseGatewayGeneration(root string, generation GatewayGeneration, certificatePEM, privateKeyPEM []byte) (string, error) {
	final, err := WriteInactiveDatabaseGatewayGeneration(root, generation, certificatePEM, privateKeyPEM)
	if err != nil {
		return "", err
	}
	if _, err := ActivateDatabaseGatewayGeneration(root, generation.Generation); err != nil {
		return "", err
	}
	return final, nil
}

// WriteInactiveDatabaseGatewayGeneration persists an immutable generation
// without changing the configuration currently observed by PgBouncer.
func WriteInactiveDatabaseGatewayGeneration(root string, generation GatewayGeneration, certificatePEM, privateKeyPEM []byte) (string, error) {
	if strings.TrimSpace(root) == "" || generation.Generation < 1 || len(certificatePEM) == 0 || len(privateKeyPEM) == 0 {
		return "", errors.New("gateway generation root, files, and TLS material required")
	}
	root = filepath.Clean(root)
	generationsDir := filepath.Join(root, "generations")
	if err := os.MkdirAll(generationsDir, 0o700); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(generationsDir, ".staging-")
	if err != nil {
		return "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	files := map[string][]byte{
		"pgbouncer.ini": generation.PgBouncerINI,
		"pg_hba.conf":   generation.HBA,
		"userlist.txt":  generation.Userlist,
		"manifest.txt":  generation.Manifest,
		"server.crt":    certificatePEM,
		"server.key":    privateKeyPEM,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(staging, name), contents, 0o600); err != nil {
			return "", err
		}
	}
	final := filepath.Join(generationsDir, strconv.Itoa(generation.Generation))
	if _, err := os.Stat(final); err == nil {
		return "", fmt.Errorf("gateway generation %d already exists", generation.Generation)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(staging, final); err != nil {
		return "", err
	}
	cleanup = false
	return final, nil
}

// ActivateDatabaseGatewayGeneration swaps current atomically and returns the
// previous relative target so callers can restore it when reload fails.
func ActivateDatabaseGatewayGeneration(root string, generation int) (string, error) {
	if strings.TrimSpace(root) == "" || generation < 1 {
		return "", errors.New("gateway generation root and number required")
	}
	root = filepath.Clean(root)
	final := filepath.Join(root, "generations", strconv.Itoa(generation))
	if info, err := os.Stat(final); err != nil || !info.IsDir() {
		if err != nil {
			return "", err
		}
		return "", errors.New("gateway generation is not a directory")
	}
	current := filepath.Join(root, "current")
	previous, err := os.Readlink(current)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	temporaryLink := filepath.Join(root, fmt.Sprintf(".current-%d", time.Now().UTC().UnixNano()))
	if err := os.Symlink(filepath.Join("generations", strconv.Itoa(generation)), temporaryLink); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryLink, current); err != nil {
		_ = os.Remove(temporaryLink)
		return "", err
	}
	return previous, nil
}

// RestoreDatabaseGatewayGeneration restores a previous relative current target.
// An empty target removes current, which is the correct rollback for first use.
func RestoreDatabaseGatewayGeneration(root, target string) error {
	root = filepath.Clean(root)
	current := filepath.Join(root, "current")
	if strings.TrimSpace(target) == "" {
		if err := os.Remove(current); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if filepath.IsAbs(target) || strings.Contains(filepath.Clean(target), "..") {
		return errors.New("unsafe gateway generation rollback target")
	}
	temporaryLink := filepath.Join(root, fmt.Sprintf(".current-rollback-%d", time.Now().UTC().UnixNano()))
	if err := os.Symlink(target, temporaryLink); err != nil {
		return err
	}
	if err := os.Rename(temporaryLink, current); err != nil {
		_ = os.Remove(temporaryLink)
		return err
	}
	return nil
}

func GatewayGenerationFingerprint(generation GatewayGeneration) string {
	hash := sha256.New()
	_, _ = hash.Write(generation.PgBouncerINI)
	_, _ = hash.Write(generation.HBA)
	_, _ = hash.Write(generation.Userlist)
	_, _ = hash.Write(generation.Manifest)
	return hex.EncodeToString(hash.Sum(nil))
}
