package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrDatabaseServiceNotFound  = errors.New("database_service_not_found")
	ErrDatabaseInstanceNotFound = errors.New("database_instance_not_found")
	ErrInvalidDatabaseEngine    = errors.New("invalid_database_engine")
	ErrInvalidDatabaseBinding   = errors.New("invalid_database_binding")
	ErrDatabaseBindingConflict  = errors.New("database_binding_variable_conflict")
	ErrDatabaseTransferLimited  = errors.New("database_transfer_rate_limited")
)

type DatabaseService struct {
	ServiceID      string    `json:"service_id"`
	Engine         string    `json:"engine"`
	DefaultVersion string    `json:"default_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DatabaseInstance struct {
	ID                string    `json:"id"`
	ServiceID         string    `json:"service_id"`
	EnvironmentID     string    `json:"environment_id"`
	EngineVersion     string    `json:"engine_version"`
	ImageRef          string    `json:"image_ref"`
	DockerContainerID string    `json:"docker_container_id,omitempty"`
	NetworkAlias      string    `json:"network_alias"`
	InternalPort      int       `json:"internal_port"`
	VolumeName        string    `json:"volume_name"`
	ResourcePreset    string    `json:"resource_preset"`
	CPULimitMillis    int       `json:"cpu_limit_millis"`
	MemoryLimitBytes  int64     `json:"memory_limit_bytes"`
	DesiredState      string    `json:"desired_state"`
	Status            string    `json:"status"`
	HealthMessage     string    `json:"health_message,omitempty"`
	HealthCheckedAt   time.Time `json:"health_checked_at,omitempty"`
	StorageUsedBytes  int64     `json:"storage_used_bytes"`
	StorageCheckedAt  time.Time `json:"storage_checked_at,omitempty"`
	DeletedAt         time.Time `json:"deleted_at,omitempty"`
	PurgeAfter        time.Time `json:"purge_after,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type DatabaseCredential struct {
	DatabaseInstanceID string    `json:"database_instance_id"`
	DatabaseName       string    `json:"database_name"`
	Username           string    `json:"username"`
	PasswordCT         []byte    `json:"-"`
	AdminPasswordCT    []byte    `json:"-"`
	PendingPasswordCT  []byte    `json:"-"`
	PendingCreatedAt   time.Time `json:"-"`
	Generation         int       `json:"generation"`
	RotatedAt          time.Time `json:"rotated_at,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DatabaseBinding struct {
	ID                 string    `json:"id"`
	DatabaseInstanceID string    `json:"database_instance_id"`
	EnvironmentID      string    `json:"environment_id"`
	ConsumerServiceID  string    `json:"consumer_service_id"`
	VariableKey        string    `json:"variable_key"`
	ReplaceExisting    bool      `json:"replace_existing"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DatabaseConnectionBindingSealed struct {
	DatabaseInstanceID string
	Engine             string
	NetworkAlias       string
	InternalPort       int
	DatabaseName       string
	Username           string
	PasswordCT         []byte
	VariableKey        string
	ReplaceExisting    bool
}

// MaxDatabaseOperationAttempts caps how many times one operation may be
// claimed. attempt_count rises on every claim, and the claim query also
// matches running rows whose lease has expired, so an operation that
// reliably wedges its worker — a hung image pull, a deadlock that never
// panics — was re-claimed every two minutes forever while the UI polled it
// every two seconds (ADR-0002 §4.3).
//
// This is a constant rather than a column because every operation type wants
// the same cap today: a column would have one writer, a hardcoded default at
// each insert site, and no API to set it. ADR Phase 1 introduces a per-kind
// operations.max_attempts, which is where a real per-operation limit belongs.
const MaxDatabaseOperationAttempts = 5

type DatabaseOperation struct {
	ID        string `json:"id"`
	ServiceID string `json:"service_id"`
	// DatabaseInstanceID is empty for service-scoped audit rows; see the note
	// on OperationType.
	DatabaseInstanceID string `json:"database_instance_id,omitempty"`
	// OperationType covers more values than processDatabaseOperation
	// dispatches on. FinalizeDatabaseServiceDeletion writes a terminal
	// 'delete' row as an audit record, which the claim query can never
	// select; 'purge' is permitted by the 0021 CHECK constraint but has no
	// writer at all.
	OperationType   string    `json:"operation_type"`
	Status          string    `json:"status"`
	ProgressStep    string    `json:"progress_step"`
	ProgressPercent int       `json:"progress_percent"`
	Actor           string    `json:"actor,omitempty"`
	ErrorCode       string    `json:"error_code,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	AttemptCount    int       `json:"attempt_count"`
	LeaseOwner      string    `json:"-"`
	LeaseExpiresAt  time.Time `json:"-"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DatabaseUpgradeJob struct {
	OperationID        string    `json:"operation_id"`
	DatabaseInstanceID string    `json:"database_instance_id"`
	EngineVersion      string    `json:"engine_version"`
	PreviousImageRef   string    `json:"previous_image_ref"`
	TargetImageRef     string    `json:"target_image_ref"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CreateDatabaseBindingInput struct {
	ConsumerServiceID string
	VariableKey       string
	ReplaceExisting   bool
}

type CreateDatabaseInstanceInput struct {
	EnvironmentID    string
	EngineVersion    string
	ImageRef         string
	NetworkAlias     string
	InternalPort     int
	VolumeName       string
	ResourcePreset   string
	CPULimitMillis   int
	MemoryLimitBytes int64
	DatabaseName     string
	Username         string
	PasswordCT       []byte
	AdminPasswordCT  []byte
	Bindings         []CreateDatabaseBindingInput
}

type CreateDatabaseServiceInput struct {
	ApplicationID  string
	Name           string
	Engine         string
	DefaultVersion string
	Actor          string
	Instances      []CreateDatabaseInstanceInput
}

type CreatedDatabaseService struct {
	Service    Service             `json:"service"`
	Database   DatabaseService     `json:"database"`
	Instances  []DatabaseInstance  `json:"instances"`
	Bindings   []DatabaseBinding   `json:"bindings"`
	Operations []DatabaseOperation `json:"operations"`
}

type UpdateDatabaseInstanceStateInput struct {
	DockerContainerID string
	ClearContainerID  bool
	DesiredState      string
	Status            string
	HealthMessage     string
	HealthCheckedAt   time.Time
	StorageUsedBytes  *int64
	StorageCheckedAt  time.Time
}

func validDatabaseEngine(engine string) bool {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "postgresql", "mysql", "mariadb", "mongodb", "redis", "valkey":
		return true
	default:
		return false
	}
}

func normalizeDatabaseBindingVariableKey(value string) (string, bool) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", false
	}
	for index, char := range value {
		if (char < 'A' || char > 'Z') && char != '_' && (index == 0 || char < '0' || char > '9') {
			return "", false
		}
	}
	return value, true
}

func (s *Store) CreateDatabaseService(ctx context.Context, in CreateDatabaseServiceInput) (CreatedDatabaseService, error) {
	applicationID := strings.TrimSpace(in.ApplicationID)
	name := strings.TrimSpace(in.Name)
	engine := strings.ToLower(strings.TrimSpace(in.Engine))
	defaultVersion := strings.TrimSpace(in.DefaultVersion)
	if name == "" || defaultVersion == "" || len(in.Instances) == 0 {
		return CreatedDatabaseService{}, fmt.Errorf("database service name, version, and instances required")
	}
	if !validDatabaseEngine(engine) {
		return CreatedDatabaseService{}, ErrInvalidDatabaseEngine
	}
	if _, err := s.GetApplication(ctx, applicationID); errors.Is(err, sql.ErrNoRows) {
		return CreatedDatabaseService{}, ErrApplicationNotFound
	} else if err != nil {
		return CreatedDatabaseService{}, err
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339)
	serviceID := newID()
	service := Service{
		ID: serviceID, ApplicationID: applicationID, ServiceType: "database",
		Name: name, RepoURL: "", DeployRuntime: "database",
		InternalPort: in.Instances[0].InternalPort, HealthCheckPath: "",
		CreatedAt: now, UpdatedAt: now,
	}
	database := DatabaseService{
		ServiceID: serviceID, Engine: engine, DefaultVersion: defaultVersion,
		CreatedAt: now, UpdatedAt: now,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreatedDatabaseService{}, err
	}
	defer tx.Rollback()
	var duplicateName int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM services WHERE application_id=? AND lower(name)=lower(?)`, applicationID, name).Scan(&duplicateName); err != nil {
		return CreatedDatabaseService{}, err
	}
	if duplicateName > 0 {
		return CreatedDatabaseService{}, ErrDuplicateService
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO services(
			id,application_id,service_type,name,repo_url,github_installation_id,root_directory,
			deploy_runtime,deploy_install_cmd,deploy_build_cmd,deploy_start_cmd,internal_port,
			health_check_path,created_at,updated_at
		) VALUES(?,?, 'database', ?, '', 0, '', 'database', '', '', '', ?, '', ?, ?)`,
		service.ID, service.ApplicationID, service.Name, service.InternalPort, stamp, stamp)
	if err != nil {
		if isUniqueConstraint(err) {
			return CreatedDatabaseService{}, ErrDuplicateService
		}
		return CreatedDatabaseService{}, fmt.Errorf("insert database service identity: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO database_services(service_id,engine,default_version,created_at,updated_at) VALUES(?,?,?,?,?)`,
		database.ServiceID, database.Engine, database.DefaultVersion, stamp, stamp); err != nil {
		return CreatedDatabaseService{}, fmt.Errorf("insert database service: %w", err)
	}

	result := CreatedDatabaseService{Service: service, Database: database}
	seenEnvironments := map[string]struct{}{}
	for _, candidate := range in.Instances {
		environmentID := strings.TrimSpace(candidate.EnvironmentID)
		if _, duplicate := seenEnvironments[environmentID]; duplicate {
			return CreatedDatabaseService{}, fmt.Errorf("duplicate database environment %s", environmentID)
		}
		seenEnvironments[environmentID] = struct{}{}
		var environmentApplicationID string
		if err := tx.QueryRowContext(ctx, `SELECT application_id FROM environments WHERE id=?`, environmentID).Scan(&environmentApplicationID); errors.Is(err, sql.ErrNoRows) {
			return CreatedDatabaseService{}, ErrEnvironmentNotFound
		} else if err != nil {
			return CreatedDatabaseService{}, err
		}
		if environmentApplicationID != applicationID {
			return CreatedDatabaseService{}, ErrEnvironmentNotFound
		}
		if strings.TrimSpace(candidate.EngineVersion) == "" || strings.TrimSpace(candidate.ImageRef) == "" ||
			strings.TrimSpace(candidate.NetworkAlias) == "" || strings.TrimSpace(candidate.VolumeName) == "" ||
			candidate.InternalPort < 1 || candidate.InternalPort > 65535 ||
			candidate.CPULimitMillis <= 0 || candidate.MemoryLimitBytes <= 0 ||
			strings.TrimSpace(candidate.DatabaseName) == "" || strings.TrimSpace(candidate.Username) == "" ||
			len(candidate.PasswordCT) == 0 {
			return CreatedDatabaseService{}, fmt.Errorf("invalid database instance configuration")
		}

		instance := DatabaseInstance{
			ID: newID(), ServiceID: serviceID, EnvironmentID: environmentID,
			EngineVersion: strings.TrimSpace(candidate.EngineVersion), ImageRef: strings.TrimSpace(candidate.ImageRef),
			NetworkAlias: strings.TrimSpace(candidate.NetworkAlias), InternalPort: candidate.InternalPort,
			VolumeName: strings.TrimSpace(candidate.VolumeName), ResourcePreset: strings.ToLower(strings.TrimSpace(candidate.ResourcePreset)),
			CPULimitMillis: candidate.CPULimitMillis, MemoryLimitBytes: candidate.MemoryLimitBytes,
			DesiredState: "running", Status: "provisioning", CreatedAt: now, UpdatedAt: now,
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO database_instances(
				id,service_id,environment_id,engine_version,image_ref,network_alias,internal_port,
				volume_name,resource_preset,cpu_limit_millis,memory_limit_bytes,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			instance.ID, instance.ServiceID, instance.EnvironmentID, instance.EngineVersion, instance.ImageRef,
			instance.NetworkAlias, instance.InternalPort, instance.VolumeName, instance.ResourcePreset,
			instance.CPULimitMillis, instance.MemoryLimitBytes, stamp, stamp); err != nil {
			return CreatedDatabaseService{}, fmt.Errorf("insert database instance: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO database_credentials(
				database_instance_id,database_name,username,password_ct,admin_password_ct,generation,created_at,updated_at
			) VALUES(?,?,?,?,?,1,?,?)`,
			instance.ID, strings.TrimSpace(candidate.DatabaseName), strings.TrimSpace(candidate.Username),
			candidate.PasswordCT, nonNilBytes(candidate.AdminPasswordCT), stamp, stamp); err != nil {
			return CreatedDatabaseService{}, fmt.Errorf("insert database credentials: %w", err)
		}

		for _, bindingInput := range candidate.Bindings {
			consumerID := strings.TrimSpace(bindingInput.ConsumerServiceID)
			var consumerApplicationID, consumerType string
			err := tx.QueryRowContext(ctx,
				`SELECT application_id,service_type FROM services WHERE id=?`, consumerID,
			).Scan(&consumerApplicationID, &consumerType)
			if errors.Is(err, sql.ErrNoRows) || consumerApplicationID != applicationID || consumerType != "application" {
				return CreatedDatabaseService{}, ErrInvalidDatabaseBinding
			}
			if err != nil {
				return CreatedDatabaseService{}, err
			}
			var environmentBindingCount int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(1) FROM service_environments WHERE service_id=? AND environment_id=?`,
				consumerID, environmentID,
			).Scan(&environmentBindingCount); err != nil {
				return CreatedDatabaseService{}, err
			}
			variableKey, validVariableKey := normalizeDatabaseBindingVariableKey(bindingInput.VariableKey)
			if environmentBindingCount != 1 || !validVariableKey {
				return CreatedDatabaseService{}, ErrInvalidDatabaseBinding
			}
			var environmentVariableConflicts int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(1) FROM environment_variables
				WHERE environment_id=? AND key=? AND (service_id IS NULL OR service_id=?)`,
				environmentID, variableKey, consumerID,
			).Scan(&environmentVariableConflicts); err != nil {
				return CreatedDatabaseService{}, err
			}
			if environmentVariableConflicts > 0 && !bindingInput.ReplaceExisting {
				return CreatedDatabaseService{}, ErrDatabaseBindingConflict
			}
			var managedBindingConflicts int
			if err := tx.QueryRowContext(ctx, `
				SELECT COUNT(1) FROM database_bindings
				WHERE environment_id=? AND consumer_service_id=? AND variable_key=?`,
				environmentID, consumerID, variableKey,
			).Scan(&managedBindingConflicts); err != nil {
				return CreatedDatabaseService{}, err
			}
			if managedBindingConflicts > 0 {
				return CreatedDatabaseService{}, ErrDatabaseBindingConflict
			}
			binding := DatabaseBinding{
				ID: newID(), DatabaseInstanceID: instance.ID, EnvironmentID: environmentID,
				ConsumerServiceID: consumerID, VariableKey: variableKey,
				ReplaceExisting: bindingInput.ReplaceExisting, CreatedAt: now, UpdatedAt: now,
			}
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO database_bindings(id,database_instance_id,environment_id,consumer_service_id,variable_key,replace_existing,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
				binding.ID, binding.DatabaseInstanceID, binding.EnvironmentID, binding.ConsumerServiceID,
				binding.VariableKey, boolInt(binding.ReplaceExisting), stamp, stamp); err != nil {
				return CreatedDatabaseService{}, fmt.Errorf("insert database binding: %w", err)
			}
			result.Bindings = append(result.Bindings, binding)
		}

		operation := DatabaseOperation{
			ID: newID(), ServiceID: serviceID, DatabaseInstanceID: instance.ID,
			OperationType: "provision", Status: "queued", ProgressStep: "reserved",
			Actor: strings.TrimSpace(in.Actor), CreatedAt: now, UpdatedAt: now,
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO database_operations(
				id,service_id,database_instance_id,operation_type,status,progress_step,
				progress_percent,actor,created_at,updated_at
			) VALUES(?,?,?,'provision','queued','reserved',0,?,?,?)`,
			operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.Actor, stamp, stamp); err != nil {
			return CreatedDatabaseService{}, fmt.Errorf("insert database operation: %w", err)
		}
		if err = insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
			return CreatedDatabaseService{}, fmt.Errorf("enqueue database operation: %w", err)
		}
		result.Instances = append(result.Instances, instance)
		result.Operations = append(result.Operations, operation)
	}

	if err = tx.Commit(); err != nil {
		return CreatedDatabaseService{}, err
	}
	return result, nil
}

func nonNilBytes(value []byte) []byte {
	if value == nil {
		return []byte{}
	}
	return value
}

func (s *Store) GetDatabaseService(ctx context.Context, serviceID string) (DatabaseService, error) {
	var item DatabaseService
	var created, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT service_id,engine,default_version,created_at,updated_at FROM database_services WHERE service_id=?`,
		strings.TrimSpace(serviceID),
	).Scan(&item.ServiceID, &item.Engine, &item.DefaultVersion, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseService{}, ErrDatabaseServiceNotFound
	}
	if err != nil {
		return DatabaseService{}, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func scanDatabaseInstance(scanner interface{ Scan(...any) error }) (DatabaseInstance, error) {
	var item DatabaseInstance
	var healthChecked, storageChecked, deletedAt, purgeAfter, created, updated string
	err := scanner.Scan(
		&item.ID, &item.ServiceID, &item.EnvironmentID, &item.EngineVersion, &item.ImageRef,
		&item.DockerContainerID, &item.NetworkAlias, &item.InternalPort, &item.VolumeName,
		&item.ResourcePreset, &item.CPULimitMillis, &item.MemoryLimitBytes, &item.DesiredState,
		&item.Status, &item.HealthMessage, &healthChecked, &item.StorageUsedBytes, &storageChecked,
		&deletedAt, &purgeAfter, &created, &updated,
	)
	if err != nil {
		return DatabaseInstance{}, err
	}
	item.HealthCheckedAt = parseTime(healthChecked)
	item.StorageCheckedAt = parseTime(storageChecked)
	item.DeletedAt = parseTime(deletedAt)
	item.PurgeAfter = parseTime(purgeAfter)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

const databaseInstanceColumns = `
	id,service_id,environment_id,engine_version,image_ref,docker_container_id,network_alias,
	internal_port,volume_name,resource_preset,cpu_limit_millis,memory_limit_bytes,desired_state,
	status,health_message,health_checked_at,storage_used_bytes,storage_checked_at,deleted_at,
	purge_after,created_at,updated_at`

func (s *Store) GetDatabaseInstance(ctx context.Context, instanceID string) (DatabaseInstance, error) {
	item, err := scanDatabaseInstance(s.db.QueryRowContext(ctx,
		`SELECT `+databaseInstanceColumns+` FROM database_instances WHERE id=?`, strings.TrimSpace(instanceID)))
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseInstance{}, ErrDatabaseInstanceNotFound
	}
	return item, err
}

func (s *Store) ListDatabaseInstances(ctx context.Context, serviceID string) ([]DatabaseInstance, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+databaseInstanceColumns+` FROM database_instances WHERE service_id=? ORDER BY created_at,id`,
		strings.TrimSpace(serviceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseInstance{}
	for rows.Next() {
		item, err := scanDatabaseInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAllDatabaseInstances(ctx context.Context) ([]DatabaseInstance, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+databaseInstanceColumns+` FROM database_instances ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseInstance{}
	for rows.Next() {
		item, err := scanDatabaseInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetDatabaseCredentialSealed(ctx context.Context, instanceID string) (DatabaseCredential, error) {
	var item DatabaseCredential
	var pendingCreated, rotated, created, updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT database_instance_id,database_name,username,password_ct,admin_password_ct,
		       pending_password_ct,pending_created_at,generation,rotated_at,created_at,updated_at
		FROM database_credentials WHERE database_instance_id=?`, strings.TrimSpace(instanceID),
	).Scan(&item.DatabaseInstanceID, &item.DatabaseName, &item.Username, &item.PasswordCT,
		&item.AdminPasswordCT, &item.PendingPasswordCT, &pendingCreated, &item.Generation, &rotated, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseCredential{}, ErrDatabaseInstanceNotFound
	}
	if err != nil {
		return DatabaseCredential{}, err
	}
	item.RotatedAt = parseTime(rotated)
	item.PendingCreatedAt = parseTime(pendingCreated)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) StageDatabaseCredentialRotation(ctx context.Context, instanceID string, pendingPasswordCT []byte) (DatabaseCredential, error) {
	if len(pendingPasswordCT) == 0 {
		return DatabaseCredential{}, fmt.Errorf("encrypted pending database password required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE database_credentials SET pending_password_ct=?,pending_created_at=?,updated_at=?
		WHERE database_instance_id=? AND length(pending_password_ct)=0`,
		pendingPasswordCT, now, now, strings.TrimSpace(instanceID))
	if err != nil {
		return DatabaseCredential{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseCredential{}, fmt.Errorf("database credential rotation is already staged")
	}
	return s.GetDatabaseCredentialSealed(ctx, instanceID)
}

func (s *Store) CommitStagedDatabaseCredentialRotation(ctx context.Context, instanceID string) (DatabaseCredential, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE database_credentials
		SET password_ct=pending_password_ct,pending_password_ct=X'',pending_created_at='',
		    generation=generation+1,rotated_at=?,updated_at=?
		WHERE database_instance_id=? AND length(pending_password_ct)>0`, now, now, strings.TrimSpace(instanceID))
	if err != nil {
		return DatabaseCredential{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseCredential{}, fmt.Errorf("database credential rotation is not staged")
	}
	return s.GetDatabaseCredentialSealed(ctx, instanceID)
}

func (s *Store) ClearStagedDatabaseCredentialRotation(ctx context.Context, instanceID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE database_credentials SET pending_password_ct=X'',pending_created_at='',updated_at=?
		WHERE database_instance_id=?`, time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(instanceID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrDatabaseInstanceNotFound
	}
	return nil
}

func (s *Store) ListDatabaseBindings(ctx context.Context, instanceID string) ([]DatabaseBinding, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,database_instance_id,environment_id,consumer_service_id,variable_key,replace_existing,created_at,updated_at
		FROM database_bindings WHERE database_instance_id=? ORDER BY variable_key,consumer_service_id`,
		strings.TrimSpace(instanceID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseBinding{}
	for rows.Next() {
		var item DatabaseBinding
		var created, updated string
		if err := rows.Scan(&item.ID, &item.DatabaseInstanceID, &item.EnvironmentID,
			&item.ConsumerServiceID, &item.VariableKey, &item.ReplaceExisting, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetDatabaseBinding(ctx context.Context, id string) (DatabaseBinding, error) {
	var item DatabaseBinding
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,database_instance_id,environment_id,consumer_service_id,variable_key,replace_existing,created_at,updated_at FROM database_bindings WHERE id=?`, strings.TrimSpace(id)).Scan(&item.ID, &item.DatabaseInstanceID, &item.EnvironmentID, &item.ConsumerServiceID, &item.VariableKey, &item.ReplaceExisting, &created, &updated)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) CreateDatabaseBinding(ctx context.Context, instanceID, consumerServiceID, variableKey string, replaceExisting bool) (DatabaseBinding, error) {
	instance, err := s.GetDatabaseInstance(ctx, instanceID)
	if err != nil || !instance.DeletedAt.IsZero() {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	databaseService, err := s.GetService(ctx, instance.ServiceID)
	if err != nil {
		return DatabaseBinding{}, err
	}
	consumer, err := s.GetService(ctx, strings.TrimSpace(consumerServiceID))
	if err != nil || consumer.ServiceType != "application" || consumer.ApplicationID != databaseService.ApplicationID {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	if _, err := s.GetServiceEnvironment(ctx, consumer.ID, instance.EnvironmentID); err != nil {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	variableKey, validVariableKey := normalizeDatabaseBindingVariableKey(variableKey)
	if !validVariableKey {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	var environmentVariableConflicts int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM environment_variables WHERE environment_id=? AND key=? AND (service_id IS NULL OR service_id=?)`, instance.EnvironmentID, variableKey, consumer.ID).Scan(&environmentVariableConflicts); err != nil {
		return DatabaseBinding{}, err
	}
	if environmentVariableConflicts > 0 && !replaceExisting {
		return DatabaseBinding{}, ErrDatabaseBindingConflict
	}
	now := time.Now().UTC()
	item := DatabaseBinding{ID: newID(), DatabaseInstanceID: instance.ID, EnvironmentID: instance.EnvironmentID, ConsumerServiceID: consumer.ID, VariableKey: variableKey, ReplaceExisting: replaceExisting, CreatedAt: now, UpdatedAt: now}
	_, err = s.db.ExecContext(ctx, `INSERT INTO database_bindings(id,database_instance_id,environment_id,consumer_service_id,variable_key,replace_existing,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, item.ID, item.DatabaseInstanceID, item.EnvironmentID, item.ConsumerServiceID, item.VariableKey, boolInt(item.ReplaceExisting), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return DatabaseBinding{}, ErrDatabaseBindingConflict
	}
	return item, nil
}

func (s *Store) UpdateDatabaseBinding(ctx context.Context, id, consumerServiceID, variableKey string, replaceExisting bool) (DatabaseBinding, error) {
	id = strings.TrimSpace(id)
	var current DatabaseBinding
	var created, updated string
	err := s.db.QueryRowContext(ctx, `
		SELECT id,database_instance_id,environment_id,consumer_service_id,variable_key,replace_existing,created_at,updated_at
		FROM database_bindings WHERE id=?`, id).Scan(&current.ID, &current.DatabaseInstanceID,
		&current.EnvironmentID, &current.ConsumerServiceID, &current.VariableKey, &current.ReplaceExisting, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseBinding{}, sql.ErrNoRows
	}
	if err != nil {
		return DatabaseBinding{}, err
	}
	instance, err := s.GetDatabaseInstance(ctx, current.DatabaseInstanceID)
	if err != nil || !instance.DeletedAt.IsZero() {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	databaseService, err := s.GetService(ctx, instance.ServiceID)
	if err != nil {
		return DatabaseBinding{}, err
	}
	consumer, err := s.GetService(ctx, strings.TrimSpace(consumerServiceID))
	if err != nil || consumer.ServiceType != "application" || consumer.ApplicationID != databaseService.ApplicationID {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	if _, err := s.GetServiceEnvironment(ctx, consumer.ID, instance.EnvironmentID); err != nil {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	variableKey, validVariableKey := normalizeDatabaseBindingVariableKey(variableKey)
	if !validVariableKey {
		return DatabaseBinding{}, ErrInvalidDatabaseBinding
	}
	var environmentVariableConflicts int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM environment_variables
		WHERE environment_id=? AND key=? AND (service_id IS NULL OR service_id=?)`,
		instance.EnvironmentID, variableKey, consumer.ID).Scan(&environmentVariableConflicts); err != nil {
		return DatabaseBinding{}, err
	}
	if environmentVariableConflicts > 0 && !replaceExisting {
		return DatabaseBinding{}, ErrDatabaseBindingConflict
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE database_bindings SET environment_id=?,consumer_service_id=?,variable_key=?,replace_existing=?,updated_at=? WHERE id=?`,
		instance.EnvironmentID, consumer.ID, variableKey, boolInt(replaceExisting), now.Format(time.RFC3339Nano), id)
	if err != nil {
		return DatabaseBinding{}, ErrDatabaseBindingConflict
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseBinding{}, sql.ErrNoRows
	}
	current.EnvironmentID, current.ConsumerServiceID, current.VariableKey = instance.EnvironmentID, consumer.ID, variableKey
	current.ReplaceExisting, current.UpdatedAt = replaceExisting, now
	current.CreatedAt = parseTime(created)
	return current, nil
}

func (s *Store) DeleteDatabaseBinding(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM database_bindings WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListDatabaseConnectionBindingsSealed(ctx context.Context, consumerServiceID, environmentID string) ([]DatabaseConnectionBindingSealed, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT di.id,ds.engine,di.network_alias,di.internal_port,dc.database_name,dc.username,
		       dc.password_ct,db.variable_key,db.replace_existing
		FROM database_bindings db
		JOIN database_instances di ON di.id=db.database_instance_id
		JOIN database_services ds ON ds.service_id=di.service_id
		JOIN database_credentials dc ON dc.database_instance_id=di.id
		WHERE db.consumer_service_id=? AND di.environment_id=?
		  AND di.deleted_at='' AND di.desired_state='running' AND di.status='healthy'
		ORDER BY db.variable_key,di.id`,
		strings.TrimSpace(consumerServiceID), strings.TrimSpace(environmentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseConnectionBindingSealed{}
	for rows.Next() {
		var item DatabaseConnectionBindingSealed
		if err := rows.Scan(&item.DatabaseInstanceID, &item.Engine, &item.NetworkAlias,
			&item.InternalPort, &item.DatabaseName, &item.Username, &item.PasswordCT,
			&item.VariableKey, &item.ReplaceExisting); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// databaseOperationColumns is the one place the column list lives, so the
// single-row and list reads cannot drift into a positional-scan mismatch.
const databaseOperationColumns = `id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,` +
	`actor,error_code,error_message,attempt_count,lease_owner,lease_expires_at,` +
	`started_at,completed_at,created_at,updated_at`

func scanDatabaseOperation(row rowScanner) (DatabaseOperation, error) {
	var item DatabaseOperation
	var instanceID sql.NullString
	var started, completed, leaseExpires, created, updated string
	if err := row.Scan(&item.ID, &item.ServiceID, &instanceID, &item.OperationType, &item.Status,
		&item.ProgressStep, &item.ProgressPercent, &item.Actor, &item.ErrorCode, &item.ErrorMessage,
		&item.AttemptCount, &item.LeaseOwner, &leaseExpires, &started, &completed, &created, &updated); err != nil {
		return DatabaseOperation{}, err
	}
	if instanceID.Valid {
		item.DatabaseInstanceID = instanceID.String
	}
	item.StartedAt = parseTime(started)
	item.CompletedAt = parseTime(completed)
	item.LeaseExpiresAt = parseTime(leaseExpires)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) GetDatabaseOperation(ctx context.Context, operationID string) (DatabaseOperation, error) {
	item, err := scanDatabaseOperation(s.db.QueryRowContext(ctx,
		`SELECT `+databaseOperationColumns+` FROM database_operations WHERE id=?`,
		strings.TrimSpace(operationID)))
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseOperation{}, sql.ErrNoRows
	}
	if err != nil {
		return DatabaseOperation{}, err
	}
	return item, nil
}

// ListDatabaseOperations reads full rows in one query. It previously selected
// ids and then called GetDatabaseOperation per id — up to 51 round trips for
// a single call, all serialised behind the one connection SetMaxOpenConns(1)
// allows, on a list the database detail screen polls every 2 seconds.
func (s *Store) ListDatabaseOperations(ctx context.Context, serviceID string, limit int) ([]DatabaseOperation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+databaseOperationColumns+` FROM database_operations
		WHERE service_id=?
		ORDER BY created_at DESC,id DESC
		LIMIT ?`, strings.TrimSpace(serviceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseOperation{}
	for rows.Next() {
		item, err := scanDatabaseOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) QueueDatabaseInstanceOperation(ctx context.Context, instanceID, operationType, actor string) (DatabaseOperation, error) {
	operationType = strings.ToLower(strings.TrimSpace(operationType))
	if operationType != "start" && operationType != "stop" && operationType != "restart" && operationType != "rotate_credentials" {
		return DatabaseOperation{}, fmt.Errorf("unsupported database runtime operation")
	}
	instance, err := s.GetDatabaseInstance(ctx, instanceID)
	if err != nil {
		return DatabaseOperation{}, err
	}
	if !instance.DeletedAt.IsZero() || instance.DesiredState == "deleted" {
		return DatabaseOperation{}, fmt.Errorf("deleted database must be restored before runtime actions")
	}
	if strings.TrimSpace(instance.DockerContainerID) == "" {
		return DatabaseOperation{}, fmt.Errorf("database container is not provisioned")
	}
	now := time.Now().UTC()
	desiredState, instanceStatus := "running", "starting"
	if operationType == "stop" {
		desiredState, instanceStatus = "stopped", "stopping"
	} else if operationType == "rotate_credentials" {
		desiredState, instanceStatus = instance.DesiredState, instance.Status
	}
	operation := DatabaseOperation{
		ID: newID(), ServiceID: instance.ServiceID, DatabaseInstanceID: instance.ID,
		OperationType: operationType, Status: "queued", ProgressStep: "queued",
		Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseOperation{}, err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM database_operations
		WHERE database_instance_id=? AND status IN ('queued','running')`, instance.ID).Scan(&active); err != nil {
		return DatabaseOperation{}, err
	}
	if active > 0 {
		return DatabaseOperation{}, fmt.Errorf("database operation already in progress")
	}
	if operationType != "rotate_credentials" {
		if _, err = tx.ExecContext(ctx, `
			UPDATE database_instances SET desired_state=?,status=?,updated_at=? WHERE id=?`,
			desiredState, instanceStatus, now.Format(time.RFC3339), instance.ID); err != nil {
			return DatabaseOperation{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO database_operations(
			id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,
			actor,created_at,updated_at
		) VALUES(?,?,?,?,'queued','queued',0,?,?,?)`,
		operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.OperationType,
		operation.Actor, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return DatabaseOperation{}, err
	}
	if err = insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
		return DatabaseOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return DatabaseOperation{}, err
	}
	return operation, nil
}

func (s *Store) QueueDatabaseUpgrade(ctx context.Context, instanceID, engineVersion, targetImageRef, actor string) (DatabaseOperation, error) {
	instance, err := s.GetDatabaseInstance(ctx, instanceID)
	if err != nil {
		return DatabaseOperation{}, err
	}
	engineVersion, targetImageRef = strings.TrimSpace(engineVersion), strings.TrimSpace(targetImageRef)
	if instance.DeletedAt.IsZero() == false || instance.DesiredState != "running" || instance.Status != "healthy" || instance.DockerContainerID == "" {
		return DatabaseOperation{}, fmt.Errorf("database instance must be healthy and running")
	}
	if engineVersion == "" || engineVersion != instance.EngineVersion || targetImageRef == "" || targetImageRef == instance.ImageRef {
		return DatabaseOperation{}, fmt.Errorf("database patch upgrade is not available")
	}
	now := time.Now().UTC()
	operation := DatabaseOperation{ID: newID(), ServiceID: instance.ServiceID, DatabaseInstanceID: instance.ID, OperationType: "upgrade", Status: "queued", ProgressStep: "queued", Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseOperation{}, err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE database_instance_id=? AND status IN ('queued','running')`, instance.ID).Scan(&active); err != nil {
		return DatabaseOperation{}, err
	}
	if active > 0 {
		return DatabaseOperation{}, fmt.Errorf("database operation already in progress")
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_operations(id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,actor,created_at,updated_at) VALUES(?,?,?,'upgrade','queued','queued',0,?,?,?)`, operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.Actor, stamp, stamp); err != nil {
		return DatabaseOperation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_upgrade_jobs(operation_id,database_instance_id,engine_version,previous_image_ref,target_image_ref,status,created_at,updated_at) VALUES(?,?,?,?,?,'queued',?,?)`, operation.ID, instance.ID, engineVersion, instance.ImageRef, targetImageRef, stamp, stamp); err != nil {
		return DatabaseOperation{}, err
	}
	if err := insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
		return DatabaseOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatabaseOperation{}, err
	}
	return operation, nil
}

func (s *Store) GetDatabaseUpgradeJob(ctx context.Context, operationID string) (DatabaseUpgradeJob, error) {
	var item DatabaseUpgradeJob
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,database_instance_id,engine_version,previous_image_ref,target_image_ref,status,created_at,updated_at FROM database_upgrade_jobs WHERE operation_id=?`, strings.TrimSpace(operationID)).Scan(&item.OperationID, &item.DatabaseInstanceID, &item.EngineVersion, &item.PreviousImageRef, &item.TargetImageRef, &item.Status, &created, &updated)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) UpdateDatabaseUpgradeJobStatus(ctx context.Context, operationID, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "running" && status != "success" && status != "failed" && status != "rolled_back" {
		return fmt.Errorf("invalid database upgrade status")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE database_upgrade_jobs SET status=?,updated_at=? WHERE operation_id=?`, status, time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(operationID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CommitDatabaseInstanceUpgrade(ctx context.Context, instanceID, previousImageRef, targetImageRef, containerID string) (DatabaseInstance, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE database_instances SET image_ref=?,docker_container_id=?,desired_state='running',status='healthy',health_message='ready',health_checked_at=?,updated_at=? WHERE id=? AND image_ref=? AND deleted_at=''`, strings.TrimSpace(targetImageRef), strings.TrimSpace(containerID), now, now, strings.TrimSpace(instanceID), strings.TrimSpace(previousImageRef))
	if err != nil {
		return DatabaseInstance{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseInstance{}, fmt.Errorf("database upgrade state changed concurrently")
	}
	return s.GetDatabaseInstance(ctx, instanceID)
}

// UpdateDatabaseOperation records progress or a terminal status against an
// operation, writing both the database_operations row and its operations
// counterpart in one transaction.
//
// Keeping this method — rather than replacing its callers with a queue-level
// equivalent — is what keeps the port small: the roughly forty call sites
// across the provisioning, backup, restore and upgrade paths compile and
// behave unchanged, and the projection cannot drift because the same
// transaction writes both.
func (s *Store) UpdateDatabaseOperation(ctx context.Context, operationID, status, step string, progress int, errorCode, errorMessage string) (DatabaseOperation, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if progress < 0 || progress > 100 {
		return DatabaseOperation{}, fmt.Errorf("invalid operation progress")
	}
	now := time.Now().UTC()
	startedAt := ""
	completedAt := ""
	if status == "running" {
		startedAt = now.Format(time.RFC3339)
	}
	if status == "success" || status == "failed" || status == "cancelled" {
		completedAt = now.Format(time.RFC3339)
	}
	step, errorCode, errorMessage = strings.TrimSpace(step), strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage)
	stamp := now.Format(time.RFC3339)
	operationID = strings.TrimSpace(operationID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseOperation{}, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status=?,progress_step=?,progress_percent=?,error_code=?,error_message=?,
		    started_at=CASE WHEN started_at='' AND ?<>'' THEN ? ELSE started_at END,
		    completed_at=CASE WHEN ?<>'' THEN ? ELSE completed_at END,
		    lease_owner=CASE WHEN ?<>'' THEN '' ELSE lease_owner END,
		    lease_expires_at=CASE WHEN ?<>'' THEN '' ELSE lease_expires_at END,
		    updated_at=?
		WHERE id=?`,
		status, step, progress, errorCode, errorMessage,
		startedAt, startedAt, completedAt, completedAt, completedAt, completedAt,
		stamp, operationID)
	if err != nil {
		return DatabaseOperation{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return DatabaseOperation{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status=?,progress_step=?,progress_percent=?,error_code=?,error_message=?,
		    started_at=CASE WHEN started_at='' AND ?<>'' THEN ? ELSE started_at END,
		    completed_at=CASE WHEN ?<>'' THEN ? ELSE completed_at END,
		    lease_owner=CASE WHEN ?<>'' THEN '' ELSE lease_owner END,
		    lease_expires_at=CASE WHEN ?<>'' THEN '' ELSE lease_expires_at END,
		    updated_at=?
		WHERE id=?`,
		status, step, progress, errorCode, errorMessage,
		startedAt, startedAt, completedAt, completedAt, completedAt, completedAt,
		stamp, operationID); err != nil {
		return DatabaseOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatabaseOperation{}, err
	}
	return s.GetDatabaseOperation(ctx, operationID)
}

func (s *Store) ClaimNextDatabaseOperation(ctx context.Context, leaseOwner string, leaseDuration time.Duration) (DatabaseOperation, error) {
	leaseOwner = strings.TrimSpace(leaseOwner)
	if leaseOwner == "" || leaseDuration < time.Second {
		return DatabaseOperation{}, fmt.Errorf("database operation lease owner and duration required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseOperation{}, err
	}
	defer tx.Rollback()
	var operationID string
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	err = tx.QueryRowContext(ctx, `
		SELECT op.id FROM database_operations op
		WHERE (op.status='queued' OR (op.status='running' AND (op.lease_expires_at='' OR op.lease_expires_at<=?)))
		  AND op.attempt_count < `+strconv.Itoa(MaxDatabaseOperationAttempts)+`
		  AND (op.database_instance_id IS NULL OR EXISTS(
		    SELECT 1 FROM database_instances active_instance
		    WHERE active_instance.id=op.database_instance_id AND active_instance.desired_state<>'deleted'
		  ))
		  AND (op.operation_type<>'restore' OR (
		    EXISTS(SELECT 1 FROM database_instances di WHERE di.id=op.database_instance_id AND di.status='healthy' AND di.desired_state='running')
		    AND EXISTS(
		      SELECT 1 FROM database_restore_jobs rj
		      LEFT JOIN database_backups safety ON safety.id=rj.safety_backup_id
		      WHERE rj.operation_id=op.id AND (rj.safety_backup_id IS NULL OR safety.status IN ('success','failed','cancelled'))
		    )
		  ))
		ORDER BY op.created_at,op.id
		LIMIT 1`, now).Scan(&operationID)
	if err != nil {
		return DatabaseOperation{}, err
	}
	leaseExpires := nowTime.Add(leaseDuration).Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='running',progress_step='starting',progress_percent=1,
		    started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,
		    lease_owner=?,lease_expires_at=?,attempt_count=attempt_count+1,updated_at=?
		WHERE id=? AND (status='queued' OR (status='running' AND (lease_expires_at='' OR lease_expires_at<=?)))`,
		now, leaseOwner, leaseExpires, now, operationID, now)
	if err != nil {
		return DatabaseOperation{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return DatabaseOperation{}, sql.ErrNoRows
	}
	if err = tx.Commit(); err != nil {
		return DatabaseOperation{}, err
	}
	return s.GetDatabaseOperation(ctx, operationID)
}

func (s *Store) RenewDatabaseOperationLease(ctx context.Context, operationID, leaseOwner string, leaseDuration time.Duration) error {
	if strings.TrimSpace(operationID) == "" || strings.TrimSpace(leaseOwner) == "" || leaseDuration < time.Second {
		return fmt.Errorf("database operation lease id, owner, and duration required")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE database_operations SET lease_expires_at=?,updated_at=?
		WHERE id=? AND status='running' AND lease_owner=?`,
		now.Add(leaseDuration).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		strings.TrimSpace(operationID), strings.TrimSpace(leaseOwner))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// FailExhaustedDatabaseOperations marks operations that have used up their
// claim budget as failed, so a request that reliably wedges its worker
// surfaces as a failure instead of sitting queued forever while the claim
// query skips it (ADR-0002 §4.3).
//
// Companion rows are failed before the parent, mirroring
// RequeueExpiredDatabaseOperations, so a caller reading the child row never
// sees it queued under a terminal parent.
func (s *Store) FailExhaustedDatabaseOperations(ctx context.Context, at time.Time) (int64, error) {
	now := at.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	exhausted := `SELECT id FROM database_operations WHERE status IN ('queued','running') AND attempt_count>=` +
		strconv.Itoa(MaxDatabaseOperationAttempts)
	for _, table := range []string{"database_backups", "database_restore_jobs", "database_upgrade_jobs"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+
			` SET status='failed',updated_at=? WHERE status IN ('queued','running') AND operation_id IN (`+exhausted+`)`, now); err != nil {
			return 0, err
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='failed',progress_step='interrupted',
		    error_code='interrupted',
		    error_message='operation exceeded the retry limit',
		    completed_at=?,lease_owner='',lease_expires_at='',updated_at=?
		WHERE status IN ('queued','running') AND attempt_count>=?`,
		now, now, MaxDatabaseOperationAttempts)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) RequeueExpiredDatabaseOperations(ctx context.Context, at time.Time) (int64, error) {
	now := at.UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	expired := `SELECT id FROM database_operations WHERE status='running' AND (lease_expires_at='' OR lease_expires_at<=?)`
	if _, err = tx.ExecContext(ctx, `UPDATE database_backups SET status='queued',updated_at=? WHERE status='running' AND operation_id IN (`+expired+`)`, now, now); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE database_restore_jobs SET status='queued',updated_at=? WHERE status='running' AND operation_id IN (`+expired+`)`, now, now); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE database_upgrade_jobs SET status='queued',updated_at=? WHERE status='running' AND operation_id IN (`+expired+`)`, now, now); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='queued',progress_step='recovery',progress_percent=0,
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE status='running' AND (lease_expires_at='' OR lease_expires_at<=?)`, now, now)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) UpdateDatabaseInstanceState(ctx context.Context, instanceID string, in UpdateDatabaseInstanceStateInput) (DatabaseInstance, error) {
	current, err := s.GetDatabaseInstance(ctx, instanceID)
	if err != nil {
		return DatabaseInstance{}, err
	}
	if in.ClearContainerID {
		current.DockerContainerID = ""
	} else if strings.TrimSpace(in.DockerContainerID) != "" {
		current.DockerContainerID = strings.TrimSpace(in.DockerContainerID)
	}
	if strings.TrimSpace(in.DesiredState) != "" {
		current.DesiredState = strings.ToLower(strings.TrimSpace(in.DesiredState))
	}
	if strings.TrimSpace(in.Status) != "" {
		current.Status = strings.ToLower(strings.TrimSpace(in.Status))
	}
	current.HealthMessage = strings.TrimSpace(in.HealthMessage)
	healthCheckedAt := ""
	if !in.HealthCheckedAt.IsZero() {
		healthCheckedAt = in.HealthCheckedAt.UTC().Format(time.RFC3339)
	}
	storageCheckedAt := ""
	if !in.StorageCheckedAt.IsZero() {
		storageCheckedAt = in.StorageCheckedAt.UTC().Format(time.RFC3339)
	}
	storageUsedBytes := current.StorageUsedBytes
	if in.StorageUsedBytes != nil && *in.StorageUsedBytes >= 0 {
		storageUsedBytes = *in.StorageUsedBytes
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE database_instances
		SET docker_container_id=?,desired_state=?,status=?,health_message=?,
		    health_checked_at=CASE WHEN ?<>'' THEN ? ELSE health_checked_at END,
		    storage_used_bytes=?,storage_checked_at=CASE WHEN ?<>'' THEN ? ELSE storage_checked_at END,updated_at=?
		WHERE id=?`,
		current.DockerContainerID, current.DesiredState, current.Status, current.HealthMessage,
		healthCheckedAt, healthCheckedAt, storageUsedBytes, storageCheckedAt, storageCheckedAt, time.Now().UTC().Format(time.RFC3339), current.ID)
	if err != nil {
		return DatabaseInstance{}, err
	}
	return s.GetDatabaseInstance(ctx, current.ID)
}

// MarkDatabaseServiceDeleted removes automatic consumer bindings and starts the
// retained-volume window. Docker container cleanup is performed by the service
// layer before this transaction is committed.
func (s *Store) EnsureDatabaseServiceDeletionReady(ctx context.Context, serviceID string) error {
	var running int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE service_id=? AND status='running'`, strings.TrimSpace(serviceID)).Scan(&running)
	if err != nil {
		return err
	}
	if running > 0 {
		return fmt.Errorf("database operation is currently running")
	}
	return nil
}

func (s *Store) EnsureDatabaseServicePurgeReady(ctx context.Context, serviceID string) error {
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE service_id=? AND status IN ('queued','running')`, strings.TrimSpace(serviceID)).Scan(&active)
	if err != nil {
		return err
	}
	if active > 0 {
		return fmt.Errorf("database operation is active")
	}
	return nil
}

// BeginDatabaseServiceDeletion atomically prevents new instance work and
// cancels queued operations while preserving container IDs for retryable
// runtime cleanup. The retention clock starts only after cleanup succeeds.
func (s *Store) BeginDatabaseServiceDeletion(ctx context.Context, serviceID string) ([]DatabaseInstance, error) {
	if _, err := s.GetDatabaseService(ctx, serviceID); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var running int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE service_id=? AND status='running'`, strings.TrimSpace(serviceID)).Scan(&running); err != nil {
		return nil, err
	}
	if running > 0 {
		return nil, fmt.Errorf("database operation is currently running")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='cancelled',progress_step='cancelled_by_delete',completed_at=?,lease_owner='',lease_expires_at='',updated_at=?
		WHERE service_id=? AND status='queued'`, now, now, strings.TrimSpace(serviceID)); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM database_bindings WHERE database_instance_id IN (SELECT id FROM database_instances WHERE service_id=?)`, strings.TrimSpace(serviceID)); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE database_instances SET desired_state='deleted',status='stopping',updated_at=? WHERE service_id=? AND deleted_at=''`, now, strings.TrimSpace(serviceID))
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil, ErrDatabaseServiceNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListDatabaseInstances(ctx, serviceID)
}

func (s *Store) FinalizeDatabaseServiceDeletion(ctx context.Context, serviceID string, retention time.Duration, actor string) ([]DatabaseInstance, error) {
	if retention <= 0 {
		return nil, fmt.Errorf("database retention must be positive")
	}
	now := time.Now().UTC()
	purgeAfter := now.Add(retention)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE service_id=? AND status IN ('queued','running')`, strings.TrimSpace(serviceID)).Scan(&active); err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, fmt.Errorf("database operation became active during deletion")
	}
	stamp := now.Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE database_instances
		SET docker_container_id='',desired_state='deleted',status='deleted',deleted_at=?,purge_after=?,updated_at=?
		WHERE service_id=? AND deleted_at='' AND desired_state='deleted'`, stamp, purgeAfter.Format(time.RFC3339Nano), stamp, strings.TrimSpace(serviceID))
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return nil, ErrDatabaseServiceNotFound
	}
	// A terminal audit record, not work: it is never claimed. It still gets
	// an operations row so the two tables stay one-to-one, and its lock_key
	// falls back to the service because there is no instance.
	auditOperationID := newID()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO database_operations(
			id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,
			actor,started_at,completed_at,created_at,updated_at
		) VALUES(?,?,NULL,'delete','success','volume_retained',100,?,?,?,?,?)`,
		auditOperationID, strings.TrimSpace(serviceID), strings.TrimSpace(actor), stamp, stamp, stamp, stamp); err != nil {
		return nil, err
	}
	if err = insertDatabaseOperationQueueRow(ctx, tx, auditOperationID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListDatabaseInstances(ctx, serviceID)
}

func (s *Store) MarkDatabaseServiceDeleted(ctx context.Context, serviceID string, retention time.Duration, actor string) ([]DatabaseInstance, error) {
	if _, err := s.BeginDatabaseServiceDeletion(ctx, serviceID); err != nil {
		return nil, err
	}
	return s.FinalizeDatabaseServiceDeletion(ctx, serviceID, retention, actor)
}

func (s *Store) RestoreDeletedDatabaseService(ctx context.Context, serviceID, actor string) ([]DatabaseOperation, error) {
	instances, err := s.ListDatabaseInstances(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, ErrDatabaseServiceNotFound
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	operations := make([]DatabaseOperation, 0, len(instances))
	for _, instance := range instances {
		if instance.DeletedAt.IsZero() || !instance.PurgeAfter.After(now) {
			return nil, fmt.Errorf("database retention window expired")
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE database_instances
			SET desired_state='running',status='provisioning',deleted_at='',purge_after='',updated_at=?
			WHERE id=?`, now.Format(time.RFC3339), instance.ID); err != nil {
			return nil, err
		}
		operation := DatabaseOperation{
			ID: newID(), ServiceID: serviceID, DatabaseInstanceID: instance.ID,
			OperationType: "restore_deleted", Status: "queued", ProgressStep: "volume_reserved",
			Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now,
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO database_operations(
				id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,
				actor,created_at,updated_at
			) VALUES(?,?,?,'restore_deleted','queued','volume_reserved',0,?,?,?)`,
			operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.Actor,
			now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
			return nil, err
		}
		if err = insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return operations, nil
}

func (s *Store) ListDatabaseInstancesDueForPurge(ctx context.Context, at time.Time, limit int) ([]DatabaseInstance, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+databaseInstanceColumns+`
		FROM database_instances
		WHERE deleted_at<>'' AND purge_after<>'' AND purge_after<=?
		ORDER BY purge_after,id LIMIT ?`, at.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseInstance{}
	for rows.Next() {
		item, err := scanDatabaseInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) EnvironmentHasRetainedDatabaseInstances(ctx context.Context, environmentID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_instances WHERE environment_id=? AND deleted_at<>''`, strings.TrimSpace(environmentID)).Scan(&count)
	return count > 0, err
}

// PurgeDatabaseServiceRecords permanently removes the shared service identity
// only after every retained instance is past its purge deadline. The service
// layer must remove each labelled Docker volume before calling this method.
func (s *Store) PurgeDatabaseServiceRecords(ctx context.Context, serviceID string, at time.Time) error {
	instances, err := s.ListDatabaseInstances(ctx, serviceID)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return ErrDatabaseServiceNotFound
	}
	for _, instance := range instances {
		if instance.DeletedAt.IsZero() || instance.PurgeAfter.IsZero() || instance.PurgeAfter.After(at.UTC()) {
			return fmt.Errorf("database instance %s is not eligible for purge", instance.ID)
		}
	}
	return s.DeleteService(ctx, serviceID)
}
