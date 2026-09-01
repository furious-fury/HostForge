package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type BackupDestination struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Provider             string    `json:"provider"`
	Endpoint             string    `json:"endpoint"`
	Region               string    `json:"region"`
	Bucket               string    `json:"bucket"`
	ObjectPrefix         string    `json:"object_prefix"`
	PathStyle            bool      `json:"path_style"`
	ServerSideEncryption string    `json:"server_side_encryption,omitempty"`
	SSEKMSKeyID          string    `json:"sse_kms_key_id,omitempty"`
	LastTestStatus       string    `json:"last_test_status"`
	LastTestMessage      string    `json:"last_test_message,omitempty"`
	LastTestedAt         time.Time `json:"last_tested_at,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type BackupDestinationSealed struct {
	BackupDestination
	AccessKeyCT []byte
	SecretKeyCT []byte
}

type CreateBackupDestinationInput struct {
	Name, Provider, Endpoint, Region, Bucket, ObjectPrefix string
	ServerSideEncryption, SSEKMSKeyID                      string
	PathStyle                                              bool
	AccessKeyCT, SecretKeyCT                               []byte
}

func scanBackupDestination(scanner interface{ Scan(...any) error }, sealed bool) (BackupDestinationSealed, error) {
	var item BackupDestinationSealed
	var pathStyle int
	var tested, created, updated string
	values := []any{&item.ID, &item.Name, &item.Provider, &item.Endpoint, &item.Region, &item.Bucket, &item.ObjectPrefix, &pathStyle, &item.ServerSideEncryption, &item.SSEKMSKeyID}
	if sealed {
		values = append(values, &item.AccessKeyCT, &item.SecretKeyCT)
	}
	values = append(values, &item.LastTestStatus, &item.LastTestMessage, &tested, &created, &updated)
	if err := scanner.Scan(values...); err != nil {
		return BackupDestinationSealed{}, err
	}
	item.PathStyle = pathStyle == 1
	item.LastTestedAt, item.CreatedAt, item.UpdatedAt = parseTime(tested), parseTime(created), parseTime(updated)
	return item, nil
}

func validBackupServerSideEncryption(algorithm, kmsKeyID string) bool {
	switch algorithm {
	case "":
		return kmsKeyID == ""
	case "AES256":
		return kmsKeyID == ""
	case "aws:kms":
		return kmsKeyID != ""
	default:
		return false
	}
}

const backupDestinationPublicColumns = `id,name,provider,endpoint,region,bucket,object_prefix,path_style,server_side_encryption,sse_kms_key_id,last_test_status,last_test_message,last_tested_at,created_at,updated_at`

func (s *Store) CreateBackupDestination(ctx context.Context, in CreateBackupDestinationInput) (BackupDestination, error) {
	in.Name, in.Provider, in.Endpoint = strings.TrimSpace(in.Name), strings.ToLower(strings.TrimSpace(in.Provider)), strings.TrimSpace(in.Endpoint)
	in.Region, in.Bucket, in.ObjectPrefix = strings.TrimSpace(in.Region), strings.TrimSpace(in.Bucket), strings.Trim(strings.TrimSpace(in.ObjectPrefix), "/")
	in.ServerSideEncryption, in.SSEKMSKeyID = strings.TrimSpace(in.ServerSideEncryption), strings.TrimSpace(in.SSEKMSKeyID)
	if in.Name == "" || (in.Provider != "r2" && in.Provider != "s3") || in.Endpoint == "" || in.Region == "" || in.Bucket == "" || len(in.AccessKeyCT) == 0 || len(in.SecretKeyCT) == 0 || !validBackupServerSideEncryption(in.ServerSideEncryption, in.SSEKMSKeyID) {
		return BackupDestination{}, fmt.Errorf("invalid backup destination")
	}
	now, id := time.Now().UTC(), newID()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO backup_destinations(id,name,provider,endpoint,region,bucket,object_prefix,path_style,server_side_encryption,sse_kms_key_id,access_key_ct,secret_key_ct,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, in.Name, in.Provider, in.Endpoint, in.Region, in.Bucket, in.ObjectPrefix, boolInt(in.PathStyle), in.ServerSideEncryption, in.SSEKMSKeyID, in.AccessKeyCT, in.SecretKeyCT, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return BackupDestination{}, err
	}
	return s.GetBackupDestination(ctx, id)
}

func (s *Store) UpdateBackupDestination(ctx context.Context, id string, in CreateBackupDestinationInput) (BackupDestination, error) {
	in.Name, in.Provider, in.Endpoint = strings.TrimSpace(in.Name), strings.ToLower(strings.TrimSpace(in.Provider)), strings.TrimSpace(in.Endpoint)
	in.Region, in.Bucket, in.ObjectPrefix = strings.TrimSpace(in.Region), strings.TrimSpace(in.Bucket), strings.Trim(strings.TrimSpace(in.ObjectPrefix), "/")
	in.ServerSideEncryption, in.SSEKMSKeyID = strings.TrimSpace(in.ServerSideEncryption), strings.TrimSpace(in.SSEKMSKeyID)
	if in.Name == "" || (in.Provider != "r2" && in.Provider != "s3") || in.Endpoint == "" || in.Region == "" || in.Bucket == "" || len(in.AccessKeyCT) == 0 || len(in.SecretKeyCT) == 0 || !validBackupServerSideEncryption(in.ServerSideEncryption, in.SSEKMSKeyID) {
		return BackupDestination{}, fmt.Errorf("invalid backup destination")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE backup_destinations
		SET name=?,provider=?,endpoint=?,region=?,bucket=?,object_prefix=?,path_style=?,server_side_encryption=?,sse_kms_key_id=?,
		    access_key_ct=?,secret_key_ct=?,last_test_status='',last_test_message='',last_tested_at='',updated_at=?
		WHERE id=?`, in.Name, in.Provider, in.Endpoint, in.Region, in.Bucket, in.ObjectPrefix,
		boolInt(in.PathStyle), in.ServerSideEncryption, in.SSEKMSKeyID, in.AccessKeyCT, in.SecretKeyCT, now, strings.TrimSpace(id))
	if err != nil {
		return BackupDestination{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return BackupDestination{}, sql.ErrNoRows
	}
	return s.GetBackupDestination(ctx, id)
}

func (s *Store) GetBackupDestination(ctx context.Context, id string) (BackupDestination, error) {
	item, err := scanBackupDestination(s.db.QueryRowContext(ctx, `SELECT `+backupDestinationPublicColumns+` FROM backup_destinations WHERE id=?`, strings.TrimSpace(id)), false)
	return item.BackupDestination, err
}

func (s *Store) GetBackupDestinationSealed(ctx context.Context, id string) (BackupDestinationSealed, error) {
	return scanBackupDestination(s.db.QueryRowContext(ctx, `
		SELECT id,name,provider,endpoint,region,bucket,object_prefix,path_style,server_side_encryption,sse_kms_key_id,access_key_ct,secret_key_ct,last_test_status,last_test_message,last_tested_at,created_at,updated_at
		FROM backup_destinations WHERE id=?`, strings.TrimSpace(id)), true)
}

func (s *Store) ListBackupDestinations(ctx context.Context) ([]BackupDestination, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+backupDestinationPublicColumns+` FROM backup_destinations ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BackupDestination{}
	for rows.Next() {
		item, err := scanBackupDestination(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, item.BackupDestination)
	}
	return out, rows.Err()
}

func (s *Store) UpdateBackupDestinationTest(ctx context.Context, id, status, message string) (BackupDestination, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "success" && status != "failed" {
		return BackupDestination{}, fmt.Errorf("invalid destination test status")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE backup_destinations SET last_test_status=?,last_test_message=?,last_tested_at=?,updated_at=? WHERE id=?`, status, strings.TrimSpace(message), now, now, strings.TrimSpace(id))
	if err != nil {
		return BackupDestination{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return BackupDestination{}, sql.ErrNoRows
	}
	return s.GetBackupDestination(ctx, id)
}

func (s *Store) DeleteBackupDestination(ctx context.Context, id string) error {
	var retained int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_backups WHERE destination_id=? AND status IN ('queued','running','success','deleting')`, strings.TrimSpace(id)).Scan(&retained); err != nil {
		return err
	}
	if retained > 0 {
		return fmt.Errorf("backup destination still owns retained backup objects")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM backup_destinations WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type DatabaseBackupPolicy struct {
	DatabaseInstanceID string    `json:"database_instance_id"`
	DestinationID      string    `json:"destination_id"`
	Enabled            bool      `json:"enabled"`
	Schedule           string    `json:"schedule"`
	Timezone           string    `json:"timezone"`
	RetentionDays      int       `json:"retention_days"`
	LastScheduledAt    time.Time `json:"last_scheduled_at,omitempty"`
	NextScheduledAt    time.Time `json:"next_scheduled_at,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (s *Store) GetDatabaseBackupPolicy(ctx context.Context, instanceID string) (DatabaseBackupPolicy, error) {
	var item DatabaseBackupPolicy
	var enabled int
	var last, next, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT database_instance_id,destination_id,enabled,schedule,timezone,retention_days,last_scheduled_at,next_scheduled_at,created_at,updated_at FROM database_backup_policies WHERE database_instance_id=?`, strings.TrimSpace(instanceID)).Scan(
		&item.DatabaseInstanceID, &item.DestinationID, &enabled, &item.Schedule, &item.Timezone, &item.RetentionDays, &last, &next, &created, &updated)
	item.Enabled = enabled == 1
	item.LastScheduledAt, item.NextScheduledAt, item.CreatedAt, item.UpdatedAt = parseTime(last), parseTime(next), parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) UpsertDatabaseBackupPolicy(ctx context.Context, instanceID, destinationID string, enabled bool, schedule, timezone string, retentionDays int, next time.Time) (DatabaseBackupPolicy, error) {
	if _, err := s.GetDatabaseInstance(ctx, instanceID); err != nil {
		return DatabaseBackupPolicy{}, err
	}
	if _, err := s.GetBackupDestination(ctx, destinationID); err != nil {
		return DatabaseBackupPolicy{}, err
	}
	if strings.TrimSpace(schedule) == "" || strings.TrimSpace(timezone) == "" || retentionDays < 1 || retentionDays > 3650 {
		return DatabaseBackupPolicy{}, fmt.Errorf("invalid backup policy")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO database_backup_policies(database_instance_id,destination_id,enabled,schedule,timezone,retention_days,next_scheduled_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(database_instance_id) DO UPDATE SET destination_id=excluded.destination_id,enabled=excluded.enabled,schedule=excluded.schedule,timezone=excluded.timezone,retention_days=excluded.retention_days,next_scheduled_at=excluded.next_scheduled_at,updated_at=excluded.updated_at`,
		strings.TrimSpace(instanceID), strings.TrimSpace(destinationID), boolInt(enabled), strings.TrimSpace(schedule), strings.TrimSpace(timezone), retentionDays, formatOptionalTime(next), now, now)
	if err != nil {
		return DatabaseBackupPolicy{}, err
	}
	return s.GetDatabaseBackupPolicy(ctx, instanceID)
}

func (s *Store) ListDueDatabaseBackupPolicies(ctx context.Context, at time.Time, limit int) ([]DatabaseBackupPolicy, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT database_instance_id FROM database_backup_policies WHERE enabled=1 AND next_scheduled_at<>'' AND next_scheduled_at<=? ORDER BY next_scheduled_at,database_instance_id LIMIT ?`, at.UTC().Format(time.RFC3339), limit)
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
	out := make([]DatabaseBackupPolicy, 0, len(ids))
	for _, id := range ids {
		item, err := s.GetDatabaseBackupPolicy(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) AdvanceDatabaseBackupPolicySchedule(ctx context.Context, instanceID string, last, next time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE database_backup_policies SET last_scheduled_at=?,next_scheduled_at=?,updated_at=? WHERE database_instance_id=?`, formatOptionalTime(last), formatOptionalTime(next), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(instanceID))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type DatabaseBackup struct {
	ID                  string    `json:"id"`
	OperationID         string    `json:"operation_id,omitempty"`
	DatabaseInstanceID  string    `json:"database_instance_id"`
	DestinationID       string    `json:"destination_id,omitempty"`
	Status              string    `json:"status"`
	TriggerKind         string    `json:"trigger_kind"`
	ObjectKey           string    `json:"object_key,omitempty"`
	ArchiveFormat       string    `json:"archive_format,omitempty"`
	Checksum            string    `json:"checksum,omitempty"`
	CompressedSize      int64     `json:"compressed_size"`
	Engine              string    `json:"engine"`
	DatabaseName        string    `json:"database_name"`
	EngineVersion       string    `json:"engine_version"`
	EncryptionAlgorithm string    `json:"encryption_algorithm,omitempty"`
	EncryptedDataKey    []byte    `json:"-"`
	ErrorCode           string    `json:"error_code,omitempty"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	StartedAt           time.Time `json:"started_at,omitempty"`
	CompletedAt         time.Time `json:"completed_at,omitempty"`
	ExpiresAt           time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func scanDatabaseBackup(scanner interface{ Scan(...any) error }) (DatabaseBackup, error) {
	var item DatabaseBackup
	var operationID, instanceID, destination sql.NullString
	var started, completed, expires, created, updated string
	err := scanner.Scan(&item.ID, &operationID, &instanceID, &destination, &item.Status, &item.TriggerKind, &item.ObjectKey, &item.ArchiveFormat, &item.Checksum, &item.CompressedSize, &item.Engine, &item.DatabaseName, &item.EngineVersion, &item.EncryptionAlgorithm, &item.EncryptedDataKey, &item.ErrorCode, &item.ErrorMessage, &started, &completed, &expires, &created, &updated)
	item.OperationID = operationID.String
	item.DatabaseInstanceID = instanceID.String
	item.DestinationID = destination.String
	item.StartedAt, item.CompletedAt, item.ExpiresAt = parseTime(started), parseTime(completed), parseTime(expires)
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

const databaseBackupColumns = `id,operation_id,database_instance_id,destination_id,status,trigger_kind,object_key,archive_format,checksum,compressed_size,engine,database_name,engine_version,encryption_algorithm,encrypted_data_key,error_code,error_message,started_at,completed_at,expires_at,created_at,updated_at`

func (s *Store) QueueDatabaseBackup(ctx context.Context, instanceID, destinationID, triggerKind, actor string, retentionDays, maxTransfersPerHour int) (DatabaseBackup, DatabaseOperation, error) {
	instance, err := s.GetDatabaseInstance(ctx, instanceID)
	if err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if instance.Status != "healthy" || instance.DesiredState != "running" || !instance.DeletedAt.IsZero() {
		return DatabaseBackup{}, DatabaseOperation{}, fmt.Errorf("database instance is not available for backup")
	}
	if _, err := s.GetBackupDestination(ctx, destinationID); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	databaseService, err := s.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	credential, err := s.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if triggerKind != "manual" && triggerKind != "scheduled" && triggerKind != "safety" {
		return DatabaseBackup{}, DatabaseOperation{}, fmt.Errorf("invalid backup trigger")
	}
	if retentionDays < 1 || retentionDays > 3650 {
		return DatabaseBackup{}, DatabaseOperation{}, fmt.Errorf("invalid backup retention")
	}
	now := time.Now().UTC()
	operation := DatabaseOperation{ID: newID(), ServiceID: instance.ServiceID, DatabaseInstanceID: instance.ID, OperationType: "backup", Status: "queued", ProgressStep: "queued", Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now}
	backup := DatabaseBackup{ID: newID(), OperationID: operation.ID, DatabaseInstanceID: instance.ID, DestinationID: destinationID, Status: "queued", TriggerKind: triggerKind, Engine: databaseService.Engine, DatabaseName: credential.DatabaseName, EngineVersion: instance.EngineVersion, ExpiresAt: now.Add(time.Duration(retentionDays) * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	defer tx.Rollback()
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_operations WHERE database_instance_id=? AND status IN ('queued','running')`, instance.ID).Scan(&active); err != nil || active > 0 {
		if err == nil {
			err = fmt.Errorf("database operation already in progress")
		}
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if err = enforceDatabaseTransferRateLimit(ctx, tx, now, maxTransfersPerHour); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	stamp := now.Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `INSERT INTO database_operations(id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,actor,created_at,updated_at) VALUES(?,?,?,'backup','queued','queued',0,?,?,?)`, operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.Actor, stamp, stamp); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO database_backups(id,operation_id,database_instance_id,destination_id,status,trigger_kind,engine,database_name,engine_version,expires_at,created_at,updated_at) VALUES(?,?,?,?, 'queued',?,?,?,?,?,?,?)`, backup.ID, backup.OperationID, backup.DatabaseInstanceID, backup.DestinationID, backup.TriggerKind, backup.Engine, backup.DatabaseName, backup.EngineVersion, backup.ExpiresAt.Format(time.RFC3339), stamp, stamp); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if err = insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return DatabaseBackup{}, DatabaseOperation{}, err
	}
	return backup, operation, nil
}

func (s *Store) GetQueuedDatabaseBackup(ctx context.Context, instanceID string) (DatabaseBackup, error) {
	return scanDatabaseBackup(s.db.QueryRowContext(ctx, `SELECT `+databaseBackupColumns+` FROM database_backups WHERE database_instance_id=? AND status='queued' ORDER BY created_at,id LIMIT 1`, strings.TrimSpace(instanceID)))
}

func (s *Store) GetDatabaseBackupByOperationID(ctx context.Context, operationID string) (DatabaseBackup, error) {
	return scanDatabaseBackup(s.db.QueryRowContext(ctx, `SELECT `+databaseBackupColumns+` FROM database_backups WHERE operation_id=?`, strings.TrimSpace(operationID)))
}

func (s *Store) GetDatabaseBackup(ctx context.Context, id string) (DatabaseBackup, error) {
	return scanDatabaseBackup(s.db.QueryRowContext(ctx, `SELECT `+databaseBackupColumns+` FROM database_backups WHERE id=?`, strings.TrimSpace(id)))
}

func (s *Store) ListDatabaseBackups(ctx context.Context, instanceID string, limit int) ([]DatabaseBackup, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+databaseBackupColumns+` FROM database_backups WHERE database_instance_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, strings.TrimSpace(instanceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseBackup{}
	for rows.Next() {
		item, err := scanDatabaseBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) LatestSuccessfulDatabaseBackup(ctx context.Context, instanceID string) (DatabaseBackup, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM database_backups
		WHERE database_instance_id=? AND status='success'
		ORDER BY completed_at DESC,created_at DESC,id DESC LIMIT 1`, strings.TrimSpace(instanceID)).Scan(&id)
	if err != nil {
		return DatabaseBackup{}, err
	}
	return s.GetDatabaseBackup(ctx, id)
}

type CompleteDatabaseBackupInput struct {
	Status, ObjectKey, ArchiveFormat, Checksum, EncryptionAlgorithm, ErrorCode, ErrorMessage string
	CompressedSize                                                                           int64
	EncryptedDataKey                                                                         []byte
}

func (s *Store) CompleteDatabaseBackup(ctx context.Context, id string, in CompleteDatabaseBackupInput) (DatabaseBackup, error) {
	if in.Status != "success" && in.Status != "failed" && in.Status != "cancelled" {
		return DatabaseBackup{}, fmt.Errorf("invalid backup completion status")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE database_backups SET status=?,object_key=?,archive_format=?,checksum=?,compressed_size=?,encryption_algorithm=?,encrypted_data_key=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=?`, in.Status, strings.TrimSpace(in.ObjectKey), strings.TrimSpace(in.ArchiveFormat), strings.TrimSpace(in.Checksum), in.CompressedSize, strings.TrimSpace(in.EncryptionAlgorithm), nonNilBytes(in.EncryptedDataKey), strings.TrimSpace(in.ErrorCode), strings.TrimSpace(in.ErrorMessage), now, now, strings.TrimSpace(id))
	if err != nil {
		return DatabaseBackup{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return DatabaseBackup{}, sql.ErrNoRows
	}
	return s.GetDatabaseBackup(ctx, id)
}

func (s *Store) MarkDatabaseBackupRunning(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE database_backups SET status='running',started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,updated_at=? WHERE id=? AND status='queued'`, now, now, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListExpiredDatabaseBackups(ctx context.Context, at time.Time, limit int) ([]DatabaseBackup, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+databaseBackupColumns+` FROM database_backups b WHERE status IN ('success','deleting') AND expires_at<>'' AND expires_at<=? AND NOT EXISTS (SELECT 1 FROM database_restore_jobs r WHERE r.backup_id=b.id OR r.safety_backup_id=b.id) ORDER BY expires_at,id LIMIT ?`, at.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DatabaseBackup{}
	for rows.Next() {
		item, err := scanDatabaseBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkDatabaseBackupDeleting(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE database_backups SET status='deleting',updated_at=? WHERE id=? AND status IN ('success','deleting')`, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(id))
	return err
}

func (s *Store) PrepareDatabaseBackupDeletion(ctx context.Context, id string) (DatabaseBackup, error) {
	backup, err := s.GetDatabaseBackup(ctx, id)
	if err != nil {
		return DatabaseBackup{}, err
	}
	if backup.Status == "queued" || backup.Status == "running" {
		return DatabaseBackup{}, fmt.Errorf("active database backup cannot be deleted")
	}
	var references int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM database_restore_jobs WHERE backup_id=? OR safety_backup_id=?`,
		backup.ID, backup.ID).Scan(&references); err != nil {
		return DatabaseBackup{}, err
	}
	if references > 0 {
		return DatabaseBackup{}, fmt.Errorf("database backup is referenced by restore history")
	}
	if backup.Status == "success" {
		if err := s.MarkDatabaseBackupDeleting(ctx, backup.ID); err != nil {
			return DatabaseBackup{}, err
		}
		backup.Status = "deleting"
	}
	if backup.Status != "deleting" && backup.Status != "failed" && backup.Status != "cancelled" {
		return DatabaseBackup{}, fmt.Errorf("database backup status cannot be deleted")
	}
	return backup, nil
}

func (s *Store) DeleteDatabaseBackupRecord(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM database_backups WHERE id=? AND status IN ('deleting','failed','cancelled')`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

type DatabaseRestoreJob struct {
	OperationID, BackupID, TargetInstanceID, SafetyBackupID, Mode, Status string
	CreatedAt, UpdatedAt                                                  time.Time
}

func (s *Store) QueueDatabaseRestore(ctx context.Context, backupID, targetInstanceID, safetyBackupID, mode, actor string, maxTransfersPerHour int) (DatabaseOperation, error) {
	backup, err := s.GetDatabaseBackup(ctx, backupID)
	if err != nil || backup.Status != "success" || len(backup.EncryptedDataKey) == 0 || backup.ObjectKey == "" {
		return DatabaseOperation{}, fmt.Errorf("backup is not restorable")
	}
	target, err := s.GetDatabaseInstance(ctx, targetInstanceID)
	if err != nil {
		return DatabaseOperation{}, err
	}
	if !target.DeletedAt.IsZero() || target.DesiredState != "running" {
		return DatabaseOperation{}, fmt.Errorf("restore target is being deleted")
	}
	if mode == "replace_current" && (target.Status != "healthy" || strings.TrimSpace(target.DockerContainerID) == "") {
		return DatabaseOperation{}, fmt.Errorf("replace-current target must be healthy and running")
	}
	if mode == "new_service" && target.Status != "provisioning" && target.Status != "starting" && target.Status != "healthy" {
		return DatabaseOperation{}, fmt.Errorf("new-service restore target is unavailable")
	}
	databaseService, err := s.GetDatabaseService(ctx, target.ServiceID)
	if err != nil || databaseService.Engine != backup.Engine || target.EngineVersion != backup.EngineVersion {
		return DatabaseOperation{}, fmt.Errorf("backup and target database versions are incompatible")
	}
	if mode != "new_service" && mode != "replace_current" {
		return DatabaseOperation{}, fmt.Errorf("invalid restore mode")
	}
	var safetyBackupValue any
	if mode == "replace_current" {
		safety, safetyErr := s.GetDatabaseBackup(ctx, safetyBackupID)
		if safetyErr != nil || safety.DatabaseInstanceID != target.ID || safety.TriggerKind != "safety" {
			return DatabaseOperation{}, fmt.Errorf("replace-current restore requires a queued safety backup")
		}
		safetyBackupValue = safety.ID
	}
	now := time.Now().UTC()
	operation := DatabaseOperation{ID: newID(), ServiceID: target.ServiceID, DatabaseInstanceID: target.ID, OperationType: "restore", Status: "queued", ProgressStep: "waiting_for_target", Actor: strings.TrimSpace(actor), CreatedAt: now, UpdatedAt: now}
	stamp := now.Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatabaseOperation{}, err
	}
	defer tx.Rollback()
	if err = enforceDatabaseTransferRateLimit(ctx, tx, now, maxTransfersPerHour); err != nil {
		return DatabaseOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO database_operations(id,service_id,database_instance_id,operation_type,status,progress_step,progress_percent,actor,created_at,updated_at) VALUES(?,?,?,'restore','queued','waiting_for_target',0,?,?,?)`, operation.ID, operation.ServiceID, operation.DatabaseInstanceID, operation.Actor, stamp, stamp); err != nil {
		return DatabaseOperation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO database_restore_jobs(operation_id,backup_id,target_instance_id,safety_backup_id,mode,status,created_at,updated_at) VALUES(?,?,?,?,?, 'queued',?,?)`, operation.ID, backup.ID, target.ID, safetyBackupValue, mode, stamp, stamp); err != nil {
		return DatabaseOperation{}, err
	}
	// This is the site that has always queued a restore alongside its own
	// still-running safety backup on the same instance — no admission guard,
	// deliberately. Sharing a lock_key is what now orders the two.
	if err = insertDatabaseOperationQueueRow(ctx, tx, operation.ID); err != nil {
		return DatabaseOperation{}, err
	}
	if err = tx.Commit(); err != nil {
		return DatabaseOperation{}, err
	}
	return operation, nil
}

func enforceDatabaseTransferRateLimit(ctx context.Context, tx *sql.Tx, now time.Time, maxPerHour int) error {
	if maxPerHour < 1 {
		return fmt.Errorf("database transfer rate limit must be positive")
	}
	var recent int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM database_operations
		WHERE operation_type IN ('backup','restore') AND created_at>=?`,
		now.Add(-time.Hour).UTC().Format(time.RFC3339)).Scan(&recent); err != nil {
		return err
	}
	if recent >= maxPerHour {
		return ErrDatabaseTransferLimited
	}
	return nil
}

func (s *Store) GetDatabaseRestoreJob(ctx context.Context, operationID string) (DatabaseRestoreJob, error) {
	var item DatabaseRestoreJob
	var safetyBackupID sql.NullString
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,backup_id,target_instance_id,safety_backup_id,mode,status,created_at,updated_at FROM database_restore_jobs WHERE operation_id=?`, strings.TrimSpace(operationID)).Scan(&item.OperationID, &item.BackupID, &item.TargetInstanceID, &safetyBackupID, &item.Mode, &item.Status, &created, &updated)
	item.SafetyBackupID = safetyBackupID.String
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return item, err
}

func (s *Store) UpdateDatabaseRestoreJobStatus(ctx context.Context, operationID, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE database_restore_jobs SET status=?,updated_at=? WHERE operation_id=?`, strings.TrimSpace(status), time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(operationID))
	return err
}
