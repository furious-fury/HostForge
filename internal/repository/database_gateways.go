package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

var (
	ErrDatabaseGatewayNotFound        = errors.New("database_gateway_not_found")
	ErrDatabaseGatewayRouteNotFound   = errors.New("database_gateway_route_not_found")
	ErrExternalConnectionNotFound     = errors.New("database_external_connection_not_found")
	ErrInvalidExternalAccessCIDR      = errors.New("invalid_external_access_cidr")
	ErrInvalidExternalAccessProfile   = errors.New("invalid_external_access_profile")
	ErrInvalidExternalAccessExpiry    = errors.New("invalid_external_access_expiry")
	ErrOpenAccessConfirmationRequired = errors.New("external_access_open_confirmation_required")
	ErrInvalidExternalConnectionState = errors.New("invalid_external_connection_state")
	ErrGatewayHasActiveConnections    = errors.New("database_gateway_has_active_connections")
	ErrGatewayTeardownRequired        = errors.New("database_gateway_teardown_required")
	ErrPlatformDomainChanged          = errors.New("platform_domain_changed_concurrently")
)

const (
	DefaultGatewayPort              = 5432
	DefaultGatewayClientLimit       = 200
	DefaultCredentialClientLimit    = 20
	DefaultRotationGracePeriodHours = 24
	MaximumRotationGracePeriodHours = 168
)

type DatabaseGatewayEndpoint struct {
	Engine                   string    `json:"engine"`
	Hostname                 string    `json:"hostname"`
	Port                     int       `json:"port"`
	ImageRef                 string    `json:"image_ref,omitempty"`
	ImageVersion             string    `json:"image_version,omitempty"`
	ContainerName            string    `json:"container_name"`
	DockerContainerID        string    `json:"docker_container_id,omitempty"`
	IngressNetworkName       string    `json:"ingress_network_name"`
	DesiredStatus            string    `json:"desired_status"`
	ObservedStatus           string    `json:"observed_status"`
	CertificateFingerprint   string    `json:"certificate_fingerprint,omitempty"`
	CertificateExpiresAt     time.Time `json:"certificate_expires_at,omitempty"`
	CertificateSyncedAt      time.Time `json:"certificate_synced_at,omitempty"`
	DesiredConfigGeneration  int       `json:"desired_config_generation"`
	RenderedConfigGeneration int       `json:"rendered_config_generation"`
	AppliedConfigGeneration  int       `json:"applied_config_generation"`
	LastErrorCode            string    `json:"last_error_code,omitempty"`
	LastErrorMessage         string    `json:"last_error_message,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type DatabaseGatewayRoute struct {
	ID                     string    `json:"id"`
	Engine                 string    `json:"engine"`
	DatabaseInstanceID     string    `json:"database_instance_id"`
	RouteAlias             string    `json:"route_alias"`
	BackendAlias           string    `json:"backend_alias"`
	LinkNetworkName        string    `json:"link_network_name"`
	DesiredStatus          string    `json:"desired_status"`
	ObservedStatus         string    `json:"observed_status"`
	RouteBackendLimit      int       `json:"route_backend_limit"`
	CredentialBackendLimit int       `json:"credential_backend_limit"`
	LastErrorCode          string    `json:"last_error_code,omitempty"`
	LastErrorMessage       string    `json:"last_error_message,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type DatabaseExternalConnection struct {
	ID                    string                       `json:"id"`
	RouteID               string                       `json:"route_id"`
	Name                  string                       `json:"name"`
	PermissionProfile     string                       `json:"permission_profile"`
	CIDRs                 []string                     `json:"cidrs"`
	ExpiresAt             time.Time                    `json:"expires_at,omitempty"`
	Status                string                       `json:"status"`
	CurrentGeneration     int                          `json:"current_generation"`
	ClientConnectionLimit int                          `json:"client_connection_limit"`
	LastUsedAt            time.Time                    `json:"last_used_at,omitempty"`
	LastErrorCode         string                       `json:"last_error_code,omitempty"`
	LastErrorMessage      string                       `json:"last_error_message,omitempty"`
	Credentials           []DatabaseExternalCredential `json:"credentials,omitempty"`
	CreatedAt             time.Time                    `json:"created_at"`
	UpdatedAt             time.Time                    `json:"updated_at"`
}

type DatabaseExternalCredential struct {
	ID              string    `json:"id"`
	ConnectionID    string    `json:"connection_id"`
	RoleName        string    `json:"username"`
	PasswordCT      []byte    `json:"-"`
	SCRAMVerifierCT []byte    `json:"-"`
	Generation      int       `json:"generation"`
	State           string    `json:"state"`
	GraceDeadline   time.Time `json:"grace_deadline,omitempty"`
	LastUsedAt      time.Time `json:"last_used_at,omitempty"`
	RevokedAt       time.Time `json:"revoked_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DatabaseGatewayOperation struct {
	ID                        string    `json:"id"`
	Engine                    string    `json:"engine"`
	RouteID                   string    `json:"route_id,omitempty"`
	ConnectionID              string    `json:"connection_id,omitempty"`
	CredentialID              string    `json:"credential_id,omitempty"`
	OperationType             string    `json:"operation_type"`
	Status                    string    `json:"status"`
	ProgressStep              string    `json:"progress_step"`
	ProgressPercent           int       `json:"progress_percent"`
	RequestedGracePeriodHours int       `json:"requested_grace_period_hours"`
	Actor                     string    `json:"actor,omitempty"`
	ErrorCode                 string    `json:"error_code,omitempty"`
	ErrorMessage              string    `json:"error_message,omitempty"`
	StartedAt                 time.Time `json:"started_at,omitempty"`
	CompletedAt               time.Time `json:"completed_at,omitempty"`
	LeaseOwner                string    `json:"-"`
	LeaseExpiresAt            time.Time `json:"-"`
	AttemptCount              int       `json:"attempt_count"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type DatabaseExternalAccess struct {
	Instance    DatabaseInstance             `json:"instance"`
	Endpoint    *DatabaseGatewayEndpoint     `json:"gateway,omitempty"`
	Route       *DatabaseGatewayRoute        `json:"route,omitempty"`
	Connections []DatabaseExternalConnection `json:"connections"`
}

type CreateExternalConnectionInput struct {
	Name              string
	PermissionProfile string
	CIDRs             []string
	ExpiresAt         time.Time
	ConfirmOpenAccess bool
	Actor             string
}

type UpdateExternalConnectionInput struct {
	Name              string
	PermissionProfile string
	CIDRs             []string
	ExpiresAt         time.Time
	ClearExpiry       bool
	ConfirmOpenAccess bool
	Actor             string
}

func validExternalAccessProfile(profile string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "read_only", "read_write", "migration":
		return true
	default:
		return false
	}
}

// NormalizeExternalAccessCIDRs parses, masks, de-duplicates, and sorts network
// prefixes so repository state and rendered HBA rules remain deterministic.
func NormalizeExternalAccessCIDRs(values []string, confirmOpenAccess bool) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	open := false
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, ErrInvalidExternalAccessCIDR
		}
		prefix = prefix.Masked()
		canonical := prefix.String()
		if canonical == "0.0.0.0/0" || canonical == "::/0" {
			open = true
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		result = append(result, canonical)
	}
	if len(result) == 0 {
		return nil, ErrInvalidExternalAccessCIDR
	}
	if open && !confirmOpenAccess {
		return nil, ErrOpenAccessConfirmationRequired
	}
	sort.Slice(result, func(i, j int) bool {
		left, _ := netip.ParsePrefix(result[i])
		right, _ := netip.ParsePrefix(result[j])
		if left.Addr().BitLen() != right.Addr().BitLen() {
			return left.Addr().BitLen() < right.Addr().BitLen()
		}
		return result[i] < result[j]
	})
	return result, nil
}

func gatewaySafeID(prefix, id string) (string, error) {
	var builder strings.Builder
	builder.WriteString(prefix)
	for _, character := range strings.ToLower(strings.TrimSpace(id)) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	value := builder.String()
	if value == prefix || len(value) > 63 {
		return "", fmt.Errorf("unsafe gateway identifier")
	}
	return value, nil
}

func GatewayRouteAlias(instanceID string) (string, error) {
	return gatewaySafeID("hf_", instanceID)
}

func GatewayBackendAlias(instanceID string) (string, error) {
	return gatewaySafeID("hfb_", instanceID)
}

func GatewayCredentialRole(credentialID string) (string, error) {
	return gatewaySafeID("hfc_", credentialID)
}

func GatewayPoolBudget(preset string, memoryBytes int64) (route, credential int) {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "development":
		return 10, 5
	case "standard":
		return 25, 10
	case "performance":
		return 50, 20
	default:
		route = int((memoryBytes * 10) / (1024 * 1024 * 1024))
		if route < 10 {
			route = 10
		}
		if route > 50 {
			route = 50
		}
		credential = route
		if credential > 10 {
			credential = 10
		}
		return route, credential
	}
}

func (s *Store) EnsureDatabaseGatewayEndpoint(ctx context.Context, engine, hostname, imageRef, imageVersion string) (DatabaseGatewayEndpoint, error) {
	return s.ensureDatabaseGatewayEndpoint(ctx, engine, hostname, imageRef, imageVersion, "")
}

// EnsureDatabaseGatewayEndpointForPlatformDomain atomically rejects a stale
// endpoint request if the platform domain changed after endpoint discovery.
// This closes the race between the API's onboarding read and endpoint write.
func (s *Store) EnsureDatabaseGatewayEndpointForPlatformDomain(ctx context.Context, engine, hostname, imageRef, imageVersion, platformDomain string) (DatabaseGatewayEndpoint, error) {
	platformDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(platformDomain), "."))
	if platformDomain == "" {
		return DatabaseGatewayEndpoint{}, fmt.Errorf("platform domain required")
	}
	return s.ensureDatabaseGatewayEndpoint(ctx, engine, hostname, imageRef, imageVersion, platformDomain)
}

func (s *Store) ensureDatabaseGatewayEndpoint(ctx context.Context, engine, hostname, imageRef, imageVersion, expectedPlatformDomain string) (DatabaseGatewayEndpoint, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	hostname = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), "."))
	if !validDatabaseEngine(engine) || hostname == "" {
		return DatabaseGatewayEndpoint{}, fmt.Errorf("gateway engine and hostname required")
	}
	containerName := "hostforge-database-gateway-" + engine
	ingressNetwork := containerName + "-ingress"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var err error
	if expectedPlatformDomain == "" {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO database_gateway_endpoints(
				engine,hostname,port,image_ref,image_version,container_name,ingress_network_name,
				desired_status,observed_status,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,'absent','absent',?,?)
			ON CONFLICT(engine) DO UPDATE SET
				hostname=excluded.hostname,image_ref=excluded.image_ref,image_version=excluded.image_version,updated_at=excluded.updated_at
			WHERE database_gateway_endpoints.desired_status='absent'`,
			engine, hostname, DefaultGatewayPort, strings.TrimSpace(imageRef), strings.TrimSpace(imageVersion),
			containerName, ingressNetwork, now, now)
	} else {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO database_gateway_endpoints(
				engine,hostname,port,image_ref,image_version,container_name,ingress_network_name,
				desired_status,observed_status,created_at,updated_at
			)
			SELECT ?,?,?,?,?,?,?,'absent','absent',?,?
			FROM onboarding_state WHERE id=1 AND platform_domain=?
			ON CONFLICT(engine) DO UPDATE SET
				hostname=excluded.hostname,image_ref=excluded.image_ref,image_version=excluded.image_version,updated_at=excluded.updated_at
			WHERE database_gateway_endpoints.desired_status='absent'`,
			engine, hostname, DefaultGatewayPort, strings.TrimSpace(imageRef), strings.TrimSpace(imageVersion),
			containerName, ingressNetwork, now, now, expectedPlatformDomain)
	}
	if err != nil {
		return DatabaseGatewayEndpoint{}, err
	}
	endpoint, err := s.GetDatabaseGatewayEndpoint(ctx, engine)
	if err != nil {
		if expectedPlatformDomain != "" && errors.Is(err, ErrDatabaseGatewayNotFound) {
			return DatabaseGatewayEndpoint{}, ErrPlatformDomainChanged
		}
		return DatabaseGatewayEndpoint{}, err
	}
	if expectedPlatformDomain != "" {
		var currentDomain string
		if err := s.db.QueryRowContext(ctx, `SELECT platform_domain FROM onboarding_state WHERE id=1`).Scan(&currentDomain); err != nil {
			return DatabaseGatewayEndpoint{}, err
		}
		if !strings.EqualFold(strings.TrimSpace(currentDomain), expectedPlatformDomain) || endpoint.Hostname != hostname {
			return DatabaseGatewayEndpoint{}, ErrPlatformDomainChanged
		}
	}
	return endpoint, nil
}

func (s *Store) GetDatabaseGatewayEndpoint(ctx context.Context, engine string) (DatabaseGatewayEndpoint, error) {
	var item DatabaseGatewayEndpoint
	var certificateExpires, certificateSynced, created, updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT engine,hostname,port,image_ref,image_version,container_name,docker_container_id,
		       ingress_network_name,desired_status,observed_status,certificate_fingerprint,
		       certificate_expires_at,certificate_synced_at,desired_config_generation,
		       rendered_config_generation,applied_config_generation,last_error_code,last_error_message,
		       created_at,updated_at
		FROM database_gateway_endpoints WHERE engine=?`, strings.ToLower(strings.TrimSpace(engine))).Scan(
		&item.Engine, &item.Hostname, &item.Port, &item.ImageRef, &item.ImageVersion, &item.ContainerName,
		&item.DockerContainerID, &item.IngressNetworkName, &item.DesiredStatus, &item.ObservedStatus,
		&item.CertificateFingerprint, &certificateExpires, &certificateSynced,
		&item.DesiredConfigGeneration, &item.RenderedConfigGeneration, &item.AppliedConfigGeneration,
		&item.LastErrorCode, &item.LastErrorMessage, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseGatewayEndpoint{}, ErrDatabaseGatewayNotFound
	}
	item.CertificateExpiresAt = parseTime(certificateExpires)
	item.CertificateSyncedAt = parseTime(certificateSynced)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, err
}

func (s *Store) QueueDatabaseGatewayProvision(ctx context.Context, engine, actor string) (DatabaseGatewayOperation, error) {
	endpoint, err := s.GetDatabaseGatewayEndpoint(ctx, engine)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	if endpoint.DesiredStatus == "deleting" {
		return DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
	}
	return s.queueGatewayOperation(ctx, endpoint.Engine, "", "", "", "provision_gateway", DefaultRotationGracePeriodHours, actor, func(tx *sql.Tx, stamp string) error {
		result, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_status='active',observed_status=CASE WHEN observed_status='active' THEN 'active' ELSE 'provisioning' END,last_error_code='',last_error_message='',updated_at=? WHERE engine=? AND desired_status<>'deleting'`, stamp, endpoint.Engine)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return ErrInvalidExternalConnectionState
		}
		return nil
	})
}

func (s *Store) QueueDatabaseGatewayTeardown(ctx context.Context, engine, actor string) (DatabaseGatewayOperation, error) {
	endpoint, err := s.GetDatabaseGatewayEndpoint(ctx, engine)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	return s.queueGatewayOperation(ctx, endpoint.Engine, "", "", "", "teardown_gateway", DefaultRotationGracePeriodHours, actor, func(tx *sql.Tx, stamp string) error {
		result, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_status='deleting',observed_status='deleting',updated_at=? WHERE engine=?`, stamp, endpoint.Engine)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return ErrDatabaseGatewayNotFound
		}
		var activeTeardowns int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_gateway_operations WHERE engine=? AND operation_type='teardown_gateway' AND status IN ('queued','running')`, endpoint.Engine).Scan(&activeTeardowns); err != nil {
			return err
		}
		if activeTeardowns > 0 {
			return ErrInvalidExternalConnectionState
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_external_connections c JOIN database_gateway_routes r ON r.id=c.route_id WHERE r.engine=? AND c.status<>'revoked'`, endpoint.Engine).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return ErrGatewayHasActiveConnections
		}
		return nil
	})
}

func (s *Store) CreateDatabaseExternalConnection(ctx context.Context, instanceID string, in CreateExternalConnectionInput) (DatabaseExternalConnection, DatabaseGatewayOperation, error) {
	name := strings.TrimSpace(in.Name)
	profile := strings.ToLower(strings.TrimSpace(in.PermissionProfile))
	if name == "" || len(name) > 120 {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, fmt.Errorf("external connection name required")
	}
	if !validExternalAccessProfile(profile) {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalAccessProfile
	}
	cidrs, err := NormalizeExternalAccessCIDRs(in.CIDRs, in.ConfirmOpenAccess)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if !in.ExpiresAt.IsZero() && !in.ExpiresAt.After(time.Now().UTC()) {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalAccessExpiry
	}
	instance, err := s.GetDatabaseInstance(ctx, strings.TrimSpace(instanceID))
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	databaseService, err := s.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if instance.DesiredState != "running" || !instance.DeletedAt.IsZero() {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
	}
	endpoint, err := s.GetDatabaseGatewayEndpoint(ctx, databaseService.Engine)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	routeAlias, err := GatewayRouteAlias(instance.ID)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	backendAlias, err := GatewayBackendAlias(instance.ID)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	routeLimit, credentialLimit := GatewayPoolBudget(instance.ResourcePreset, instance.MemoryLimitBytes)
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	route := DatabaseGatewayRoute{
		ID: newID(), Engine: databaseService.Engine, DatabaseInstanceID: instance.ID,
		RouteAlias: routeAlias, BackendAlias: backendAlias,
		LinkNetworkName: "hostforge-database-gateway-link-" + strings.TrimPrefix(routeAlias, "hf_"),
		DesiredStatus:   "active", ObservedStatus: "pending", RouteBackendLimit: routeLimit,
		CredentialBackendLimit: credentialLimit, CreatedAt: now, UpdatedAt: now,
	}
	connection := DatabaseExternalConnection{
		ID: newID(), Name: name, PermissionProfile: profile, CIDRs: cidrs, Status: "pending",
		ClientConnectionLimit: DefaultCredentialClientLimit, ExpiresAt: in.ExpiresAt.UTC(),
		CreatedAt: now, UpdatedAt: now,
	}
	operation := DatabaseGatewayOperation{
		ID: newID(), Engine: databaseService.Engine, OperationType: "create_connection", Status: "queued",
		ProgressStep: "queued", RequestedGracePeriodHours: DefaultRotationGracePeriodHours,
		Actor: strings.TrimSpace(in.Actor), CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	defer tx.Rollback()
	// Acquire the endpoint write reservation and validate teardown state before
	// creating any route or grant. A concurrent teardown therefore observes the
	// new connection, or this creation observes `deleting`; neither can slip by.
	guard, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET updated_at=updated_at WHERE engine=? AND desired_status<>'deleting'`, endpoint.Engine)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	guarded, err := guard.RowsAffected()
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if guarded != 1 {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO database_gateway_routes(
			id,engine,database_instance_id,route_alias,backend_alias,link_network_name,
			desired_status,observed_status,route_backend_limit,credential_backend_limit,created_at,updated_at
		) VALUES(?,?,?,?,?,?,'active','pending',?,?,?,?)
		ON CONFLICT(database_instance_id) DO UPDATE SET desired_status='active',updated_at=excluded.updated_at`,
		route.ID, route.Engine, route.DatabaseInstanceID, route.RouteAlias, route.BackendAlias, route.LinkNetworkName,
		route.RouteBackendLimit, route.CredentialBackendLimit, stamp, stamp)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM database_gateway_routes WHERE database_instance_id=?`, instance.ID).Scan(&connection.RouteID); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	operation.RouteID = connection.RouteID
	operation.ConnectionID = connection.ID
	expires := ""
	if !connection.ExpiresAt.IsZero() {
		expires = connection.ExpiresAt.Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_external_connections(id,route_id,name,permission_profile,expires_at,status,current_generation,client_connection_limit,created_at,updated_at) VALUES(?,?,?,?,?,'pending',0,?,?,?)`, connection.ID, connection.RouteID, connection.Name, connection.PermissionProfile, expires, connection.ClientConnectionLimit, stamp, stamp); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := replaceExternalConnectionCIDRs(ctx, tx, connection.ID, cidrs, stamp); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := insertGatewayOperation(ctx, tx, operation, stamp); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_status='active',observed_status=CASE WHEN observed_status='active' THEN 'active' ELSE 'provisioning' END,desired_config_generation=desired_config_generation+1,updated_at=? WHERE engine=?`, stamp, endpoint.Engine); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	return connection, operation, nil
}

func replaceExternalConnectionCIDRs(ctx context.Context, tx *sql.Tx, connectionID string, cidrs []string, stamp string) error {
	if len(cidrs) == 0 {
		return ErrInvalidExternalAccessCIDR
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_external_connection_cidrs WHERE connection_id=?`, connectionID); err != nil {
		return err
	}
	for _, cidr := range cidrs {
		prefix, _ := netip.ParsePrefix(cidr)
		family := 6
		if prefix.Addr().Is4() {
			family = 4
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO database_external_connection_cidrs(connection_id,cidr,address_family,created_at) VALUES(?,?,?,?)`, connectionID, cidr, family, stamp); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetDatabaseExternalAccess(ctx context.Context, instanceID string) (DatabaseExternalAccess, error) {
	instance, err := s.GetDatabaseInstance(ctx, strings.TrimSpace(instanceID))
	if err != nil {
		return DatabaseExternalAccess{}, err
	}
	databaseService, err := s.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		return DatabaseExternalAccess{}, err
	}
	result := DatabaseExternalAccess{Instance: instance, Connections: []DatabaseExternalConnection{}}
	if endpoint, endpointErr := s.GetDatabaseGatewayEndpoint(ctx, databaseService.Engine); endpointErr == nil {
		result.Endpoint = &endpoint
	} else if !errors.Is(endpointErr, ErrDatabaseGatewayNotFound) {
		return DatabaseExternalAccess{}, endpointErr
	}
	route, err := s.GetDatabaseGatewayRouteByInstance(ctx, instance.ID)
	if errors.Is(err, ErrDatabaseGatewayRouteNotFound) {
		return result, nil
	}
	if err != nil {
		return DatabaseExternalAccess{}, err
	}
	result.Route = &route
	result.Connections, err = s.ListDatabaseExternalConnections(ctx, route.ID)
	return result, err
}

func (s *Store) GetDatabaseGatewayRouteByInstance(ctx context.Context, instanceID string) (DatabaseGatewayRoute, error) {
	return s.scanDatabaseGatewayRoute(s.db.QueryRowContext(ctx, `SELECT id,engine,database_instance_id,route_alias,backend_alias,link_network_name,desired_status,observed_status,route_backend_limit,credential_backend_limit,last_error_code,last_error_message,created_at,updated_at FROM database_gateway_routes WHERE database_instance_id=?`, strings.TrimSpace(instanceID)))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanDatabaseGatewayRoute(row rowScanner) (DatabaseGatewayRoute, error) {
	var item DatabaseGatewayRoute
	var created, updated string
	err := row.Scan(&item.ID, &item.Engine, &item.DatabaseInstanceID, &item.RouteAlias, &item.BackendAlias,
		&item.LinkNetworkName, &item.DesiredStatus, &item.ObservedStatus, &item.RouteBackendLimit,
		&item.CredentialBackendLimit, &item.LastErrorCode, &item.LastErrorMessage, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseGatewayRoute{}, ErrDatabaseGatewayRouteNotFound
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) ListDatabaseExternalConnections(ctx context.Context, routeID string) ([]DatabaseExternalConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM database_external_connections WHERE route_id=? ORDER BY created_at DESC,id DESC`, strings.TrimSpace(routeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]DatabaseExternalConnection, 0, len(ids))
	for _, id := range ids {
		item, err := s.GetDatabaseExternalConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Store) GetDatabaseExternalConnection(ctx context.Context, connectionID string) (DatabaseExternalConnection, error) {
	var item DatabaseExternalConnection
	var expires, lastUsed, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,route_id,name,permission_profile,expires_at,status,current_generation,client_connection_limit,last_used_at,last_error_code,last_error_message,created_at,updated_at FROM database_external_connections WHERE id=?`, strings.TrimSpace(connectionID)).Scan(&item.ID, &item.RouteID, &item.Name, &item.PermissionProfile, &expires, &item.Status, &item.CurrentGeneration, &item.ClientConnectionLimit, &lastUsed, &item.LastErrorCode, &item.LastErrorMessage, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseExternalConnection{}, ErrExternalConnectionNotFound
	}
	if err != nil {
		return DatabaseExternalConnection{}, err
	}
	item.ExpiresAt, item.LastUsedAt = parseTime(expires), parseTime(lastUsed)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	item.CIDRs, err = s.listExternalConnectionCIDRs(ctx, item.ID)
	if err != nil {
		return DatabaseExternalConnection{}, err
	}
	item.Credentials, err = s.ListDatabaseExternalCredentials(ctx, item.ID, false)
	return item, err
}

func (s *Store) listExternalConnectionCIDRs(ctx context.Context, connectionID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cidr FROM database_external_connection_cidrs WHERE connection_id=? ORDER BY address_family,cidr`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return nil, err
		}
		result = append(result, cidr)
	}
	return result, rows.Err()
}

func (s *Store) UpdateDatabaseExternalConnection(ctx context.Context, connectionID string, in UpdateExternalConnectionInput) (DatabaseExternalConnection, DatabaseGatewayOperation, error) {
	current, err := s.GetDatabaseExternalConnection(ctx, connectionID)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if current.Status == "revoked" || current.Status == "revoking" {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = current.Name
	}
	profile := strings.ToLower(strings.TrimSpace(in.PermissionProfile))
	if profile == "" {
		profile = current.PermissionProfile
	}
	if !validExternalAccessProfile(profile) {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalAccessProfile
	}
	cidrs := current.CIDRs
	if in.CIDRs != nil {
		cidrs, err = NormalizeExternalAccessCIDRs(in.CIDRs, in.ConfirmOpenAccess)
		if err != nil {
			return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
		}
	}
	expires := current.ExpiresAt
	if in.ClearExpiry {
		expires = time.Time{}
	} else if !in.ExpiresAt.IsZero() {
		if !in.ExpiresAt.After(time.Now().UTC()) {
			return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, ErrInvalidExternalAccessExpiry
		}
		expires = in.ExpiresAt.UTC()
	}
	route, err := s.getDatabaseGatewayRouteByID(ctx, current.RouteID)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	operation := DatabaseGatewayOperation{ID: newID(), Engine: route.Engine, RouteID: route.ID, ConnectionID: current.ID, OperationType: "update_connection", Status: "queued", ProgressStep: "queued", RequestedGracePeriodHours: DefaultRotationGracePeriodHours, Actor: strings.TrimSpace(in.Actor), CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	defer tx.Rollback()
	expiresRaw := ""
	if !expires.IsZero() {
		expiresRaw = expires.Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET name=?,permission_profile=?,expires_at=?,status=CASE WHEN status='failed' THEN 'pending' ELSE status END,last_error_code='',last_error_message='',updated_at=? WHERE id=?`, name, profile, expiresRaw, stamp, current.ID); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := replaceExternalConnectionCIDRs(ctx, tx, current.ID, cidrs, stamp); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := insertGatewayOperation(ctx, tx, operation, stamp); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_config_generation=desired_config_generation+1,updated_at=? WHERE engine=?`, stamp, route.Engine); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatabaseExternalConnection{}, DatabaseGatewayOperation{}, err
	}
	updated, err := s.GetDatabaseExternalConnection(ctx, current.ID)
	return updated, operation, err
}

func (s *Store) QueueDatabaseExternalConnectionAction(ctx context.Context, connectionID, action string, gracePeriodHours int, actor string) (DatabaseGatewayOperation, error) {
	connection, err := s.GetDatabaseExternalConnection(ctx, connectionID)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	route, err := s.getDatabaseGatewayRouteByID(ctx, connection.RouteID)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	operationType, nextStatus := "", ""
	switch action {
	case "disable":
		if connection.Status != "active" && connection.Status != "rotating" {
			return DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
		}
		operationType, nextStatus = "disable_connection", "disabled"
	case "enable":
		if connection.Status != "disabled" && connection.Status != "expired" && connection.Status != "failed" {
			return DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
		}
		if !connection.ExpiresAt.IsZero() && !connection.ExpiresAt.After(time.Now().UTC()) {
			return DatabaseGatewayOperation{}, ErrInvalidExternalAccessExpiry
		}
		operationType, nextStatus = "enable_connection", "pending"
	case "rotate":
		if connection.Status != "active" {
			return DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
		}
		if gracePeriodHours < 0 || gracePeriodHours > MaximumRotationGracePeriodHours {
			return DatabaseGatewayOperation{}, fmt.Errorf("invalid rotation grace period")
		}
		operationType, nextStatus = "rotate_connection", "rotating"
	case "revoke":
		if connection.Status == "revoked" {
			return DatabaseGatewayOperation{}, ErrInvalidExternalConnectionState
		}
		operationType, nextStatus = "revoke_connection", "revoking"
	default:
		return DatabaseGatewayOperation{}, fmt.Errorf("unsupported external connection action")
	}
	return s.queueGatewayOperation(ctx, route.Engine, route.ID, connection.ID, "", operationType, gracePeriodHours, actor, func(tx *sql.Tx, stamp string) error {
		if _, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET status=?,last_error_code='',last_error_message='',updated_at=? WHERE id=?`, nextStatus, stamp, connection.ID); err != nil {
			return err
		}
		if operationType == "revoke_connection" {
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_gateway_operations WHERE connection_id=? AND operation_type='revoke_connection' AND status IN ('queued','running')`, connection.ID).Scan(&active); err != nil {
				return err
			}
			if active > 0 {
				return ErrInvalidExternalConnectionState
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_config_generation=desired_config_generation+1,updated_at=? WHERE engine=?`, stamp, route.Engine)
		return err
	})
}

func (s *Store) getDatabaseGatewayRouteByID(ctx context.Context, routeID string) (DatabaseGatewayRoute, error) {
	return s.scanDatabaseGatewayRoute(s.db.QueryRowContext(ctx, `SELECT id,engine,database_instance_id,route_alias,backend_alias,link_network_name,desired_status,observed_status,route_backend_limit,credential_backend_limit,last_error_code,last_error_message,created_at,updated_at FROM database_gateway_routes WHERE id=?`, strings.TrimSpace(routeID)))
}

func (s *Store) GetDatabaseGatewayRoute(ctx context.Context, routeID string) (DatabaseGatewayRoute, error) {
	return s.getDatabaseGatewayRouteByID(ctx, routeID)
}

func (s *Store) CreateDatabaseExternalCredential(ctx context.Context, connectionID string, passwordCT, verifierCT []byte) (DatabaseExternalCredential, error) {
	if len(passwordCT) == 0 || len(verifierCT) == 0 {
		return DatabaseExternalCredential{}, fmt.Errorf("encrypted credential material required")
	}
	connection, err := s.GetDatabaseExternalConnection(ctx, connectionID)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	identity, err := s.NewDatabaseExternalCredentialIdentity(ctx, connection.ID)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	return s.CreateDatabaseExternalCredentialWithIdentity(ctx, identity, passwordCT, verifierCT)
}

func (s *Store) NewDatabaseExternalCredentialIdentity(ctx context.Context, connectionID string) (DatabaseExternalCredential, error) {
	connection, err := s.GetDatabaseExternalConnection(ctx, connectionID)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	credentialID := newID()
	roleName, err := GatewayCredentialRole(credentialID)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	return DatabaseExternalCredential{ID: credentialID, ConnectionID: connection.ID, RoleName: roleName, Generation: connection.CurrentGeneration + 1, State: "active"}, nil
}

func (s *Store) CreateDatabaseExternalCredentialWithIdentity(ctx context.Context, identity DatabaseExternalCredential, passwordCT, verifierCT []byte) (DatabaseExternalCredential, error) {
	connection, err := s.GetDatabaseExternalConnection(ctx, identity.ConnectionID)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	expectedRole, err := GatewayCredentialRole(identity.ID)
	if err != nil || expectedRole != identity.RoleName || identity.Generation != connection.CurrentGeneration+1 {
		return DatabaseExternalCredential{}, fmt.Errorf("invalid external credential identity")
	}
	if len(passwordCT) == 0 || len(verifierCT) == 0 {
		return DatabaseExternalCredential{}, fmt.Errorf("encrypted credential material required")
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	credential := DatabaseExternalCredential{ID: identity.ID, ConnectionID: connection.ID, RoleName: identity.RoleName, PasswordCT: passwordCT, SCRAMVerifierCT: verifierCT, Generation: identity.Generation, State: "active", CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_external_credentials(id,connection_id,role_name,password_ct,scram_verifier_ct,generation,state,created_at,updated_at) VALUES(?,?,?,?,?,?,'active',?,?)`, credential.ID, credential.ConnectionID, credential.RoleName, credential.PasswordCT, credential.SCRAMVerifierCT, credential.Generation, stamp, stamp); err != nil {
		return DatabaseExternalCredential{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET current_generation=?,updated_at=? WHERE id=? AND current_generation=?`, credential.Generation, stamp, connection.ID, connection.CurrentGeneration)
	if err != nil {
		return DatabaseExternalCredential{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseExternalCredential{}, fmt.Errorf("external credential generation changed concurrently")
	}
	if err := tx.Commit(); err != nil {
		return DatabaseExternalCredential{}, err
	}
	return credential, nil
}

func (s *Store) GetDatabaseExternalCredentialSealed(ctx context.Context, credentialID string) (DatabaseExternalCredential, error) {
	return scanExternalCredential(s.db.QueryRowContext(ctx, `SELECT id,connection_id,role_name,password_ct,scram_verifier_ct,generation,state,grace_deadline,last_used_at,revoked_at,created_at,updated_at FROM database_external_credentials WHERE id=?`, strings.TrimSpace(credentialID)))
}

func scanExternalCredential(row rowScanner) (DatabaseExternalCredential, error) {
	var item DatabaseExternalCredential
	var grace, lastUsed, revoked, created, updated string
	err := row.Scan(&item.ID, &item.ConnectionID, &item.RoleName, &item.PasswordCT, &item.SCRAMVerifierCT, &item.Generation, &item.State, &grace, &lastUsed, &revoked, &created, &updated)
	item.GraceDeadline, item.LastUsedAt, item.RevokedAt = parseTime(grace), parseTime(lastUsed), parseTime(revoked)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) ListDatabaseExternalCredentials(ctx context.Context, connectionID string, includeCiphertext bool) ([]DatabaseExternalCredential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,connection_id,role_name,password_ct,scram_verifier_ct,generation,state,grace_deadline,last_used_at,revoked_at,created_at,updated_at FROM database_external_credentials WHERE connection_id=? ORDER BY generation DESC`, strings.TrimSpace(connectionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DatabaseExternalCredential{}
	for rows.Next() {
		item, err := scanExternalCredential(rows)
		if err != nil {
			return nil, err
		}
		if !includeCiphertext {
			item.PasswordCT, item.SCRAMVerifierCT = nil, nil
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SetDatabaseExternalCredentialGrace(ctx context.Context, credentialID string, deadline time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_external_credentials SET state='grace',grace_deadline=?,updated_at=? WHERE id=? AND state='active'`, deadline.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(credentialID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RevokeDatabaseExternalCredential(ctx context.Context, credentialID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE database_external_credentials SET state='revoked',password_ct=X'',scram_verifier_ct=X'',grace_deadline='',revoked_at=?,updated_at=? WHERE id=? AND state<>'revoked'`, now, now, strings.TrimSpace(credentialID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchDatabaseExternalCredentialUsage(ctx context.Context, roleNames []string, at time.Time) error {
	unique := map[string]struct{}{}
	for _, roleName := range roleNames {
		if roleName = strings.TrimSpace(roleName); roleName != "" {
			unique[roleName] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil
	}
	if len(unique) > DefaultGatewayClientLimit {
		return fmt.Errorf("too many gateway usage roles")
	}
	placeholders := make([]string, 0, len(unique))
	roles := make([]any, 0, len(unique)+1)
	stamp := at.UTC().Format(time.RFC3339Nano)
	roles = append(roles, stamp)
	for roleName := range unique {
		placeholders = append(placeholders, "?")
		roles = append(roles, roleName)
	}
	querySuffix := strings.Join(placeholders, ",")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE database_external_credentials SET last_used_at=? WHERE state IN ('active','grace') AND role_name IN (`+querySuffix+`)`, roles...); err != nil {
		return err
	}
	connectionArgs := append([]any(nil), roles...)
	if _, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET last_used_at=? WHERE id IN (
		SELECT connection_id FROM database_external_credentials WHERE state IN ('active','grace') AND role_name IN (`+querySuffix+`)
	)`, connectionArgs...); err != nil {
		return err
	}
	return tx.Commit()
}

// RollbackDatabaseExternalCredentialRotation restores the previous generation
// after N+1 provisioning fails. The failed secret is erased but its audit row
// is retained, while the previously working generation remains current.
func (s *Store) RollbackDatabaseExternalCredentialRotation(ctx context.Context, connectionID, failedCredentialID string, previousGeneration int) error {
	if strings.TrimSpace(connectionID) == "" || strings.TrimSpace(failedCredentialID) == "" || previousGeneration < 1 {
		return fmt.Errorf("rotation rollback scope and previous generation required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE database_external_credentials
		SET state='revoked',password_ct=X'',scram_verifier_ct=X'',grace_deadline='',
		    revoked_at=CASE WHEN revoked_at='' THEN ? ELSE revoked_at END,updated_at=?
		WHERE id=? AND connection_id=?`, stamp, stamp, strings.TrimSpace(failedCredentialID), strings.TrimSpace(connectionID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	result, err = tx.ExecContext(ctx, `UPDATE database_external_connections
		SET current_generation=?,status='active',last_error_code='',last_error_message='',updated_at=?
		WHERE id=? AND current_generation>?`, previousGeneration, stamp, strings.TrimSpace(connectionID), previousGeneration)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrInvalidExternalConnectionState
	}
	return tx.Commit()
}

func (s *Store) SetDatabaseExternalConnectionStatus(ctx context.Context, connectionID, status, errorCode, errorMessage string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_external_connections SET status=?,last_error_code=?,last_error_message=?,updated_at=? WHERE id=?`, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(connectionID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrExternalConnectionNotFound
	}
	return nil
}

func (s *Store) SetDatabaseGatewayEndpointState(ctx context.Context, engine, desired, observed, containerID string, desiredGeneration, renderedGeneration, appliedGeneration int, errorCode, errorMessage string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_status=CASE WHEN ?='' THEN desired_status ELSE ? END,observed_status=CASE WHEN ?='' THEN observed_status ELSE ? END,docker_container_id=CASE WHEN ?='' THEN docker_container_id ELSE ? END,desired_config_generation=CASE WHEN ?<0 THEN desired_config_generation ELSE ? END,rendered_config_generation=CASE WHEN ?<0 THEN rendered_config_generation ELSE ? END,applied_config_generation=CASE WHEN ?<0 THEN applied_config_generation ELSE ? END,last_error_code=?,last_error_message=?,updated_at=? WHERE engine=?`, desired, desired, observed, observed, containerID, containerID, desiredGeneration, desiredGeneration, renderedGeneration, renderedGeneration, appliedGeneration, appliedGeneration, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), time.Now().UTC().Format(time.RFC3339Nano), strings.ToLower(strings.TrimSpace(engine)))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrDatabaseGatewayNotFound
	}
	return nil
}

func (s *Store) SetDatabaseGatewayCertificate(ctx context.Context, engine, fingerprint string, expiresAt, syncedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_endpoints SET certificate_fingerprint=?,certificate_expires_at=?,certificate_synced_at=?,updated_at=? WHERE engine=?`, strings.TrimSpace(fingerprint), expiresAt.UTC().Format(time.RFC3339Nano), syncedAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), strings.ToLower(strings.TrimSpace(engine)))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrDatabaseGatewayNotFound
	}
	return nil
}

func (s *Store) getGatewayOperationScope(ctx context.Context, connectionID string) (DatabaseGatewayRoute, error) {
	connection, err := s.GetDatabaseExternalConnection(ctx, connectionID)
	if err != nil {
		return DatabaseGatewayRoute{}, err
	}
	return s.getDatabaseGatewayRouteByID(ctx, connection.RouteID)
}

func (s *Store) queueGatewayOperation(ctx context.Context, engine, routeID, connectionID, credentialID, operationType string, graceHours int, actor string, mutate func(*sql.Tx, string) error) (DatabaseGatewayOperation, error) {
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	operation := DatabaseGatewayOperation{ID: newID(), Engine: engine, RouteID: routeID, ConnectionID: connectionID, CredentialID: credentialID, OperationType: operationType, Status: "queued", ProgressStep: "queued", RequestedGracePeriodHours: graceHours, Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	defer tx.Rollback()
	if mutate != nil {
		if err := mutate(tx, stamp); err != nil {
			return DatabaseGatewayOperation{}, err
		}
	}
	if err := insertGatewayOperation(ctx, tx, operation, stamp); err != nil {
		return DatabaseGatewayOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatabaseGatewayOperation{}, err
	}
	return operation, nil
}

func insertGatewayOperation(ctx context.Context, tx *sql.Tx, operation DatabaseGatewayOperation, stamp string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO database_gateway_operations(id,engine,route_id,connection_id,credential_id,operation_type,status,progress_step,progress_percent,requested_grace_period_hours,actor,created_at,updated_at) VALUES(?,?,?,?,?,?,'queued','queued',0,?,?,?,?)`, operation.ID, operation.Engine, nullString(operation.RouteID), nullString(operation.ConnectionID), nullString(operation.CredentialID), operation.OperationType, operation.RequestedGracePeriodHours, operation.Actor, stamp, stamp)
	return err
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func (s *Store) GetDatabaseGatewayOperation(ctx context.Context, operationID string) (DatabaseGatewayOperation, error) {
	var item DatabaseGatewayOperation
	var routeID, connectionID, credentialID sql.NullString
	var started, completed, leaseExpires, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,engine,route_id,connection_id,credential_id,operation_type,status,progress_step,progress_percent,requested_grace_period_hours,actor,error_code,error_message,started_at,completed_at,lease_owner,lease_expires_at,attempt_count,created_at,updated_at FROM database_gateway_operations WHERE id=?`, strings.TrimSpace(operationID)).Scan(&item.ID, &item.Engine, &routeID, &connectionID, &credentialID, &item.OperationType, &item.Status, &item.ProgressStep, &item.ProgressPercent, &item.RequestedGracePeriodHours, &item.Actor, &item.ErrorCode, &item.ErrorMessage, &started, &completed, &item.LeaseOwner, &leaseExpires, &item.AttemptCount, &created, &updated)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	if routeID.Valid {
		item.RouteID = routeID.String
	}
	if connectionID.Valid {
		item.ConnectionID = connectionID.String
	}
	if credentialID.Valid {
		item.CredentialID = credentialID.String
	}
	item.StartedAt, item.CompletedAt, item.LeaseExpiresAt = parseTime(started), parseTime(completed), parseTime(leaseExpires)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, nil
}

func (s *Store) HasActiveDatabaseGatewayOperation(ctx context.Context, engine, operationType string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_gateway_operations WHERE engine=? AND operation_type=? AND status IN ('queued','running')`, strings.ToLower(strings.TrimSpace(engine)), strings.TrimSpace(operationType)).Scan(&count)
	return count > 0, err
}

func (s *Store) ClaimNextDatabaseGatewayOperation(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (DatabaseGatewayOperation, error) {
	leaseOwner = strings.TrimSpace(leaseOwner)
	if leaseOwner == "" || leaseDuration < time.Second {
		return DatabaseGatewayOperation{}, fmt.Errorf("gateway operation lease owner and duration required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	defer tx.Rollback()
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	var operationID string
	if err := tx.QueryRowContext(ctx, `
		SELECT op.id FROM database_gateway_operations op
		WHERE (op.status='queued' OR (op.status='running' AND (op.lease_expires_at='' OR op.lease_expires_at<=?)))
		  AND (
		    op.operation_type<>'create_connection'
		    OR EXISTS(
		      SELECT 1 FROM database_external_connections c
		      JOIN database_gateway_routes r ON r.id=c.route_id
		      JOIN database_instances i ON i.id=r.database_instance_id
		      WHERE c.id=op.connection_id AND i.desired_state='running'
		        AND i.status='healthy' AND i.deleted_at=''
		    )
		  )
		ORDER BY op.created_at,op.id LIMIT 1`, now).Scan(&operationID); err != nil {
		return DatabaseGatewayOperation{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE database_gateway_operations SET status='running',progress_step='starting',progress_percent=1,started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,lease_owner=?,lease_expires_at=?,attempt_count=attempt_count+1,updated_at=? WHERE id=? AND (status='queued' OR (status='running' AND (lease_expires_at='' OR lease_expires_at<=?)))`, now, leaseOwner, nowTime.Add(leaseDuration).Format(time.RFC3339Nano), now, operationID, now)
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseGatewayOperation{}, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return DatabaseGatewayOperation{}, err
	}
	return s.GetDatabaseGatewayOperation(ctx, operationID)
}

func (s *Store) FailQueuedInitialDatabaseExternalConnections(ctx context.Context, instanceID, errorCode, errorMessage string) error {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return ErrDatabaseInstanceNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	connectionIDs := `SELECT c.id FROM database_external_connections c JOIN database_gateway_routes r ON r.id=c.route_id WHERE r.database_instance_id=? AND c.status='pending'`
	if _, err := tx.ExecContext(ctx, `UPDATE database_gateway_operations SET status='failed',progress_step='database_provision_failed',error_code=?,error_message=?,completed_at=?,lease_owner='',lease_expires_at='',updated_at=? WHERE operation_type='create_connection' AND status='queued' AND connection_id IN (`+connectionIDs+`)`, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), now, now, instanceID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET status='failed',last_error_code=?,last_error_message=?,updated_at=? WHERE id IN (`+connectionIDs+`)`, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), now, instanceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RenewDatabaseGatewayOperationLease(ctx context.Context, operationID, leaseOwner string, leaseDuration time.Duration) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_operations SET lease_expires_at=?,updated_at=? WHERE id=? AND status='running' AND lease_owner=?`, now.Add(leaseDuration).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), strings.TrimSpace(operationID), strings.TrimSpace(leaseOwner))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateDatabaseGatewayOperation(ctx context.Context, operationID, status, step string, progress int, errorCode, errorMessage string) (DatabaseGatewayOperation, error) {
	if progress < 0 || progress > 100 {
		return DatabaseGatewayOperation{}, fmt.Errorf("invalid gateway operation progress")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	completed := ""
	if status == "success" || status == "failed" || status == "cancelled" {
		completed = now
	}
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_operations SET status=?,progress_step=?,progress_percent=?,error_code=?,error_message=?,completed_at=CASE WHEN ?<>'' THEN ? ELSE completed_at END,lease_owner=CASE WHEN ?<>'' THEN '' ELSE lease_owner END,lease_expires_at=CASE WHEN ?<>'' THEN '' ELSE lease_expires_at END,updated_at=? WHERE id=?`, strings.ToLower(strings.TrimSpace(status)), strings.TrimSpace(step), progress, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), completed, completed, completed, completed, now, strings.TrimSpace(operationID))
	if err != nil {
		return DatabaseGatewayOperation{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseGatewayOperation{}, sql.ErrNoRows
	}
	return s.GetDatabaseGatewayOperation(ctx, operationID)
}

func (s *Store) RequeueExpiredDatabaseGatewayOperations(ctx context.Context, at time.Time) (int64, error) {
	now := at.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_operations SET status='queued',progress_step='recovery',progress_percent=0,lease_owner='',lease_expires_at='',updated_at=? WHERE status='running' AND (lease_expires_at='' OR lease_expires_at<=?)`, now, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) QueueDueDatabaseExternalConnectionExpirations(ctx context.Context, at time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id FROM database_external_connections c WHERE c.status='active' AND c.expires_at<>'' AND c.expires_at<=? AND NOT EXISTS(SELECT 1 FROM database_gateway_operations op WHERE op.connection_id=c.id AND op.operation_type='expire_connection' AND op.status IN ('queued','running')) ORDER BY c.expires_at,c.id LIMIT ?`, at.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	queued := 0
	for _, id := range ids {
		route, err := s.getGatewayOperationScope(ctx, id)
		if err != nil {
			return queued, err
		}
		_, err = s.queueGatewayOperation(ctx, route.Engine, route.ID, id, "", "expire_connection", DefaultRotationGracePeriodHours, "system", func(tx *sql.Tx, stamp string) error {
			_, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET status='expired',updated_at=? WHERE id=? AND status='active'`, stamp, id)
			return err
		})
		if err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

func (s *Store) CountActiveDatabaseGatewayConnections(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_external_connections WHERE status NOT IN ('revoked')`).Scan(&count)
	return count, err
}

type databaseGatewayDomainQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func checkDatabaseGatewayDomainChangeAllowed(ctx context.Context, queryer databaseGatewayDomainQueryer) error {
	var connections int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_external_connections WHERE status<>'revoked'`).Scan(&connections); err != nil {
		return err
	}
	if connections > 0 {
		return ErrGatewayHasActiveConnections
	}
	var blockers int
	if err := queryer.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM database_gateway_endpoints
			 WHERE desired_status<>'absent' OR observed_status<>'absent' OR docker_container_id<>''
			    OR certificate_fingerprint<>'' OR desired_config_generation<>0
			    OR rendered_config_generation<>0 OR applied_config_generation<>0)
			+
			(SELECT COUNT(*) FROM database_gateway_operations WHERE status IN ('queued','running'))`).Scan(&blockers); err != nil {
		return err
	}
	if blockers > 0 {
		return ErrGatewayTeardownRequired
	}
	return nil
}

// CheckDatabaseGatewayDomainChangeAllowed is an advisory API preflight. The
// authoritative check is repeated under UpdatePlatformDomain's write lock.
func (s *Store) CheckDatabaseGatewayDomainChangeAllowed(ctx context.Context) error {
	return checkDatabaseGatewayDomainChangeAllowed(ctx, s.db)
}

func (s *Store) ListDatabaseGatewayRoutes(ctx context.Context, engine string) ([]DatabaseGatewayRoute, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,engine,database_instance_id,route_alias,backend_alias,link_network_name,desired_status,observed_status,route_backend_limit,credential_backend_limit,last_error_code,last_error_message,created_at,updated_at FROM database_gateway_routes WHERE engine=? AND desired_status<>'deleted' ORDER BY route_alias,id`, strings.ToLower(strings.TrimSpace(engine)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DatabaseGatewayRoute{}
	for rows.Next() {
		item, err := s.scanDatabaseGatewayRoute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SetDatabaseGatewayRouteState(ctx context.Context, routeID, desired, observed, errorCode, errorMessage string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_routes SET desired_status=CASE WHEN ?='' THEN desired_status ELSE ? END,observed_status=CASE WHEN ?='' THEN observed_status ELSE ? END,last_error_code=?,last_error_message=?,updated_at=? WHERE id=?`, desired, desired, observed, observed, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage), time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(routeID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrDatabaseGatewayRouteNotFound
	}
	return nil
}

func (s *Store) CompleteDatabaseExternalCredentialRotation(ctx context.Context, connectionID, previousCredentialID string, graceDeadline time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(previousCredentialID) != "" {
		deadline := graceDeadline.UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE database_external_credentials SET state='grace',grace_deadline=?,updated_at=? WHERE id=? AND connection_id=? AND state='active'`, deadline, stamp, strings.TrimSpace(previousCredentialID), strings.TrimSpace(connectionID)); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET status='active',last_error_code='',last_error_message='',updated_at=? WHERE id=? AND status IN ('pending','rotating','failed')`, stamp, strings.TrimSpace(connectionID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrInvalidExternalConnectionState
	}
	return tx.Commit()
}

func (s *Store) FinalizeDatabaseExternalConnectionRevocation(ctx context.Context, connectionID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE database_external_credentials SET state='revoked',password_ct=X'',scram_verifier_ct=X'',grace_deadline='',revoked_at=CASE WHEN revoked_at='' THEN ? ELSE revoked_at END,updated_at=? WHERE connection_id=?`, stamp, stamp, strings.TrimSpace(connectionID)); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE database_external_connections SET status='revoked',last_error_code='',last_error_message='',updated_at=? WHERE id=?`, stamp, strings.TrimSpace(connectionID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrExternalConnectionNotFound
	}
	return tx.Commit()
}

func (s *Store) ClearDatabaseGatewayEndpointRuntime(ctx context.Context, engine string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_gateway_endpoints SET desired_status='absent',observed_status='absent',docker_container_id='',certificate_fingerprint='',certificate_expires_at='',certificate_synced_at='',desired_config_generation=0,rendered_config_generation=0,applied_config_generation=0,last_error_code='',last_error_message='',updated_at=? WHERE engine=?`, time.Now().UTC().Format(time.RFC3339Nano), strings.ToLower(strings.TrimSpace(engine)))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrDatabaseGatewayNotFound
	}
	return nil
}

func (s *Store) QueueDueDatabaseExternalCredentialRetirements(ctx context.Context, at time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cr.id,cr.connection_id,r.id,r.engine FROM database_external_credentials cr JOIN database_external_connections c ON c.id=cr.connection_id JOIN database_gateway_routes r ON r.id=c.route_id WHERE cr.state='grace' AND cr.grace_deadline<>'' AND cr.grace_deadline<=? AND NOT EXISTS(SELECT 1 FROM database_gateway_operations op WHERE op.credential_id=cr.id AND op.operation_type='retire_credential' AND op.status IN ('queued','running')) ORDER BY cr.grace_deadline,cr.id LIMIT ?`, at.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return 0, err
	}
	type dueCredential struct{ credentialID, connectionID, routeID, engine string }
	due := []dueCredential{}
	for rows.Next() {
		var item dueCredential
		if err := rows.Scan(&item.credentialID, &item.connectionID, &item.routeID, &item.engine); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for index, item := range due {
		if _, err := s.queueGatewayOperation(ctx, item.engine, item.routeID, item.connectionID, item.credentialID, "retire_credential", 0, "system", nil); err != nil {
			return index, err
		}
	}
	return len(due), nil
}
