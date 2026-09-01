package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Operation is one row of the generic operations queue (ADR-0002 §4).
//
// Every database operation also has a database_operations row with the same
// id, kept in step by the writers below. That table remains the read model
// for the existing API; this one is authoritative for queueing state.
type Operation struct {
	ID       string
	Kind     string
	LockKey  string
	Status   string
	Priority int

	ProgressStep    string
	ProgressPercent int

	AvailableAt time.Time
	Attempt     int
	MaxAttempts int

	LeaseOwner        string
	LeaseExpiresAt    time.Time
	CancelRequestedAt time.Time

	ApplicationID string
	ServiceID     string
	EnvironmentID string

	Actor        string
	ErrorCode    string
	ErrorMessage string

	StartedAt   time.Time
	CompletedAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const operationColumns = `id,kind,lock_key,status,priority,progress_step,progress_percent,` +
	`available_at,attempt,max_attempts,lease_owner,lease_expires_at,cancel_requested_at,` +
	`application_id,service_id,environment_id,actor,error_code,error_message,` +
	`started_at,completed_at,created_at,updated_at`

// defaultOperationPriority is the band ordinary background work runs in.
// Interactive work can be enqueued above it without a schema change; nothing
// does yet (ADR-0002 §20.2).
const defaultOperationPriority = 100

// defaultOperationMaxAttempts matches the cap the database operation claim
// already enforced before this table existed.
const defaultOperationMaxAttempts = 5

// NewOperationInput describes a row to enqueue.
type NewOperationInput struct {
	ID      string
	Kind    string
	LockKey string
	Status  string

	Priority     int
	MaxAttempts  int
	ProgressStep string

	ApplicationID string
	ServiceID     string
	EnvironmentID string
	Actor         string
}

// insertOperationTx writes one operations row inside an existing
// transaction. Every enqueue path goes through it so the seven call sites
// that insert a database_operations row cannot drift in how they populate
// the queue side.
func insertOperationTx(ctx context.Context, tx *sql.Tx, in NewOperationInput, now time.Time) error {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Kind) == "" || strings.TrimSpace(in.LockKey) == "" {
		return fmt.Errorf("operation requires an id, a kind, and a lock key")
	}
	status := in.Status
	if status == "" {
		status = "queued"
	}
	priority := in.Priority
	if priority == 0 {
		priority = defaultOperationPriority
	}
	maxAttempts := in.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = defaultOperationMaxAttempts
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	completed := ""
	percent := 0
	// A row inserted already terminal is an audit record, not work: it is
	// never claimed, so it needs its completion stamped here.
	if status == "success" || status == "failed" || status == "cancelled" {
		completed = stamp
		if status == "success" {
			percent = 100
		}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO operations(
			id,kind,lock_key,status,priority,progress_step,progress_percent,
			available_at,attempt,max_attempts,
			application_id,service_id,environment_id,actor,
			completed_at,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,'',0,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(in.ID), strings.TrimSpace(in.Kind), strings.TrimSpace(in.LockKey),
		status, priority, in.ProgressStep, percent, maxAttempts,
		strings.TrimSpace(in.ApplicationID), strings.TrimSpace(in.ServiceID),
		strings.TrimSpace(in.EnvironmentID), strings.TrimSpace(in.Actor),
		completed, stamp, stamp)
	return err
}

// insertDatabaseOperationQueueRow writes the operations row paired with a
// database_operations row inserted earlier in the same transaction. The two
// share a primary key, so this must run in that transaction or the queue
// gains a row the projection does not have.
//
// It derives every column from the database_operations row by the same
// expression migration 0028 uses to backfill, rather than taking them as Go
// arguments. That is deliberate: it means a row enqueued today and a row
// backfilled from before this change are populated identically, and the
// seven insert sites cannot drift from each other or from the migration.
func insertDatabaseOperationQueueRow(ctx context.Context, tx *sql.Tx, operationID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO operations (
			id, kind, lock_key, status, priority, progress_step, progress_percent,
			available_at, attempt, max_attempts,
			application_id, service_id, environment_id, actor,
			started_at, completed_at, created_at, updated_at
		)
		SELECT
			op.id,
			'db_' || op.operation_type,
			CASE
				WHEN op.database_instance_id IS NOT NULL AND op.database_instance_id <> ''
					THEN 'dbi:' || op.database_instance_id
				ELSE 'dbsvc:' || op.service_id
			END,
			op.status, ?, op.progress_step, op.progress_percent,
			'', 0, ?,
			COALESCE(svc.application_id, ''), op.service_id, COALESCE(inst.environment_id, ''),
			op.actor, op.started_at, op.completed_at, op.created_at, op.updated_at
		FROM database_operations op
		LEFT JOIN services svc ON svc.id = op.service_id
		LEFT JOIN database_instances inst ON inst.id = op.database_instance_id
		WHERE op.id = ?`,
		defaultOperationPriority, defaultOperationMaxAttempts, strings.TrimSpace(operationID))
	return err
}

// EnqueueOperation inserts a standalone operations row. Database operations
// enqueue through their own paths, which write both tables in one
// transaction; this exists for tests and for future subsystems that have no
// projection.
func (s *Store) EnqueueOperation(ctx context.Context, in NewOperationInput) (Operation, error) {
	if strings.TrimSpace(in.ID) == "" {
		in.ID = newID()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback()
	if err := insertOperationTx(ctx, tx, in, time.Now()); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Operation{}, err
	}
	return s.GetOperation(ctx, in.ID)
}

func scanOperation(row rowScanner) (Operation, error) {
	var item Operation
	var available, leaseExpires, cancelRequested, started, completed, created, updated string
	if err := row.Scan(&item.ID, &item.Kind, &item.LockKey, &item.Status, &item.Priority,
		&item.ProgressStep, &item.ProgressPercent, &available, &item.Attempt, &item.MaxAttempts,
		&item.LeaseOwner, &leaseExpires, &cancelRequested,
		&item.ApplicationID, &item.ServiceID, &item.EnvironmentID,
		&item.Actor, &item.ErrorCode, &item.ErrorMessage,
		&started, &completed, &created, &updated); err != nil {
		return Operation{}, err
	}
	item.AvailableAt = parseTime(available)
	item.LeaseExpiresAt = parseTime(leaseExpires)
	item.CancelRequestedAt = parseTime(cancelRequested)
	item.StartedAt = parseTime(started)
	item.CompletedAt = parseTime(completed)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Store) GetOperation(ctx context.Context, id string) (Operation, error) {
	return scanOperation(s.db.QueryRowContext(ctx,
		`SELECT `+operationColumns+` FROM operations WHERE id=?`, strings.TrimSpace(id)))
}

// ClaimOptions bounds which operations a worker will pick up.
type ClaimOptions struct {
	// Owner identifies the claiming worker and is what lease renewal and
	// release are checked against. Required.
	Owner string
	// Lease is how long the claim is held before another worker may take
	// over. Required.
	Lease time.Duration
	// MinPriority lets a worker be reserved for high-priority work. Zero
	// means claim anything (ADR-0002 §20.2).
	MinPriority int
	// Now is the clock. Zero means time.Now().
	Now time.Time
}

// ClaimNextOperation atomically claims the highest-priority runnable
// operation and marks it running, returning sql.ErrNoRows when there is
// nothing to do.
//
// An operation is runnable when it is queued, its available_at has passed,
// it has attempts left, and no *running* operation holds its lock_key. That
// last clause is the serialisation guarantee: work sharing a lock key runs
// one at a time, in priority then age order.
//
// The candidate SELECT and the claiming UPDATE are separate statements in
// one transaction rather than a single UPDATE ... RETURNING. The UPDATE
// re-checks status, so a row claimed in between affects zero rows and the
// caller gets ErrNoRows and retries — the same compare-and-swap the database
// operation claim has always used. RETURNING would save one statement out of
// three (the projection write below is required regardless) and appears
// nowhere else in this codebase.
func (s *Store) ClaimNextOperation(ctx context.Context, opts ClaimOptions) (Operation, error) {
	owner := strings.TrimSpace(opts.Owner)
	if owner == "" || opts.Lease < time.Second {
		return Operation{}, fmt.Errorf("operation claim requires an owner and a lease of at least one second")
	}
	nowTime := opts.Now
	if nowTime.IsZero() {
		nowTime = time.Now()
	}
	nowTime = nowTime.UTC()
	now := nowTime.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback()

	var id string
	if err := tx.QueryRowContext(ctx, `
		SELECT o.id FROM operations o
		WHERE o.status='queued'
		  AND (o.available_at='' OR o.available_at<=?)
		  AND o.attempt < o.max_attempts
		  AND o.priority >= ?
		  AND NOT EXISTS (
		    SELECT 1 FROM operations running
		    WHERE running.status='running' AND running.lock_key=o.lock_key
		  )
		ORDER BY o.priority DESC,o.created_at,o.id
		LIMIT 1`, now, opts.MinPriority).Scan(&id); err != nil {
		return Operation{}, err
	}

	leaseExpires := nowTime.Add(opts.Lease).Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status='running',progress_step='starting',progress_percent=1,
		    started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,
		    lease_owner=?,lease_expires_at=?,attempt=attempt+1,updated_at=?
		WHERE id=? AND status='queued'`, now, owner, leaseExpires, now, id)
	if err != nil {
		return Operation{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Operation{}, sql.ErrNoRows
	}
	if err := projectOperationClaim(ctx, tx, id, owner, leaseExpires, now); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Operation{}, err
	}
	return s.GetOperation(ctx, id)
}

// RenewOperationLease extends the lease and reports whether cancellation has
// been requested, in one transaction.
//
// Both facts are read together deliberately: the worker asks this question
// on a timer anyway, so folding the cancellation check into it costs nothing
// and guarantees the two are consistent. Returns sql.ErrNoRows if the lease
// is no longer held by owner, which the caller treats as losing the
// operation.
func (s *Store) RenewOperationLease(ctx context.Context, id, owner string, lease time.Duration) (bool, error) {
	id, owner = strings.TrimSpace(id), strings.TrimSpace(owner)
	now := time.Now().UTC()
	expires := now.Add(lease).Format(time.RFC3339Nano)
	stamp := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE operations SET lease_expires_at=?,updated_at=?
		WHERE id=? AND status='running' AND lease_owner=?`, expires, stamp, id, owner)
	if err != nil {
		return false, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return false, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE database_operations SET lease_expires_at=?,updated_at=?
		WHERE id=?`, expires, stamp, id); err != nil {
		return false, err
	}
	var cancelRequested string
	if err := tx.QueryRowContext(ctx, `SELECT cancel_requested_at FROM operations WHERE id=?`, id).Scan(&cancelRequested); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return strings.TrimSpace(cancelRequested) != "", nil
}

// RequestOperationCancellation asks the worker running an operation to stop
// at its next step boundary. Queued operations are cancelled outright, since
// nothing is running to observe the request.
func (s *Store) RequestOperationCancellation(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE operations SET cancel_requested_at=?,updated_at=?
		WHERE id=? AND status='running' AND cancel_requested_at=''`, now, now, strings.TrimSpace(id))
	return err
}

// RequestServiceOperationCancellation marks every running operation for a
// service as cancel-requested, returning how many were marked.
func (s *Store) RequestServiceOperationCancellation(ctx context.Context, serviceID string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE operations SET cancel_requested_at=?,updated_at=?
		WHERE service_id=? AND status='running' AND cancel_requested_at=''`,
		now, now, strings.TrimSpace(serviceID))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeferOperation returns a claimed operation to the queue, unavailable until
// retryAfter has elapsed, for a handler that is not ready to run yet.
//
// The attempt taken by the claim is given back. A deferral means "not yet",
// not "tried and failed", and without this compensation an operation that
// waits on a dependency would burn through max_attempts and fail for being
// patient. The claim is the only place attempt is incremented, so undoing it
// here keeps the counter meaning "times actually started", which is what
// crash recovery needs it to mean.
func (s *Store) DeferOperation(ctx context.Context, id, owner string, retryAfter time.Duration) error {
	id, owner = strings.TrimSpace(id), strings.TrimSpace(owner)
	if retryAfter < 0 {
		retryAfter = 0
	}
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	available := now.Add(retryAfter).Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status='queued',progress_step='waiting',progress_percent=0,
		    available_at=?,attempt=MAX(attempt-1,0),
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE id=? AND status='running' AND lease_owner=?`, available, stamp, id, owner)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='queued',progress_step='waiting',progress_percent=0,
		    attempt_count=MAX(attempt_count-1,0),
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE id=?`, stamp, id); err != nil {
		return err
	}
	return tx.Commit()
}

// CompleteOperationInput carries a terminal transition.
type CompleteOperationInput struct {
	ID           string
	Owner        string
	Status       string
	ProgressStep string
	ErrorCode    string
	ErrorMessage string
}

// CompleteOperation records a terminal status and releases the lease.
//
// ProgressStep empty preserves whatever the handler last reported, so a
// cancelled operation still shows where it stopped.
func (s *Store) CompleteOperation(ctx context.Context, in CompleteOperationInput) error {
	if in.Status != "success" && in.Status != "failed" && in.Status != "cancelled" {
		return fmt.Errorf("invalid operation completion status %q", in.Status)
	}
	id, owner := strings.TrimSpace(in.ID), strings.TrimSpace(in.Owner)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	percent := 100
	if in.Status != "success" {
		percent = 0
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status=?,
		    progress_step=CASE WHEN ?='' THEN progress_step ELSE ? END,
		    progress_percent=CASE WHEN ?='success' THEN ? ELSE progress_percent END,
		    error_code=?,error_message=?,completed_at=?,
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE id=? AND status='running' AND lease_owner=?`,
		in.Status, in.ProgressStep, in.ProgressStep, in.Status, percent,
		strings.TrimSpace(in.ErrorCode), strings.TrimSpace(in.ErrorMessage), now, now, id, owner)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status=?,
		    progress_step=CASE WHEN ?='' THEN progress_step ELSE ? END,
		    progress_percent=CASE WHEN ?='success' THEN ? ELSE progress_percent END,
		    error_code=?,error_message=?,completed_at=?,
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE id=?`,
		in.Status, in.ProgressStep, in.ProgressStep, in.Status, percent,
		strings.TrimSpace(in.ErrorCode), strings.TrimSpace(in.ErrorMessage), now, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverOperations returns operations abandoned by a dead worker to the
// queue, and fails those with no attempts left. It reports how many of each.
//
// Unlike the lease-expiry sweep it replaces, this requeues every running
// operation regardless of whether its lease has expired. It is called once,
// at startup, before any worker begins: a single process against a single
// SQLite file means there is definitionally no live lease holder at that
// moment. Waiting out the remaining lease would strand an operation for up
// to two minutes after a restart for no benefit.
func (s *Store) RecoverOperations(ctx context.Context, at time.Time) (requeued int64, failed int64, err error) {
	now := at.UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// Each phase writes the projection and the companion rows *before*
	// mutating operations, while these subqueries still select the intended
	// set. Doing it the other way round would mean identifying the rows
	// after the fact by the timestamp just written to them.
	exhausted := `SELECT id FROM operations WHERE status='running' AND attempt>=max_attempts`

	// Exhausted first, so an operation at its limit fails rather than being
	// requeued into a claim that would skip it forever.
	for _, table := range []string{"database_backups", "database_restore_jobs", "database_upgrade_jobs"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+
			` SET status='failed',updated_at=? WHERE status IN ('queued','running') AND operation_id IN (`+exhausted+`)`, now); err != nil {
			return 0, 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='failed',progress_step='interrupted',
		    error_code='interrupted',error_message='operation exceeded the retry limit',
		    completed_at=?,lease_owner='',lease_expires_at='',updated_at=?
		WHERE id IN (`+exhausted+`)`, now, now); err != nil {
		return 0, 0, err
	}
	failResult, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status='failed',progress_step='interrupted',
		    error_code='interrupted',error_message='operation exceeded the retry limit',
		    completed_at=?,lease_owner='',lease_expires_at='',updated_at=?
		WHERE status='running' AND attempt>=max_attempts`, now, now)
	if err != nil {
		return 0, 0, err
	}
	failed, _ = failResult.RowsAffected()

	// Whatever is still running was abandoned rather than exhausted.
	// Companion rows before the parent, so a caller reading a child never
	// sees it queued under a terminal parent.
	running := `SELECT id FROM operations WHERE status='running'`
	for _, table := range []string{"database_backups", "database_restore_jobs", "database_upgrade_jobs"} {
		if _, err = tx.ExecContext(ctx, `UPDATE `+table+
			` SET status='queued',updated_at=? WHERE status='running' AND operation_id IN (`+running+`)`, now); err != nil {
			return 0, 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='queued',progress_step='recovery',progress_percent=0,
		    lease_owner='',lease_expires_at='',updated_at=?
		WHERE id IN (`+running+`)`, now); err != nil {
		return 0, 0, err
	}
	requeueResult, err := tx.ExecContext(ctx, `
		UPDATE operations
		SET status='queued',progress_step='recovery',progress_percent=0,
		    available_at='',lease_owner='',lease_expires_at='',updated_at=?
		WHERE status='running'`, now)
	if err != nil {
		return 0, 0, err
	}
	requeued, _ = requeueResult.RowsAffected()

	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return requeued, failed, nil
}

// projectOperationClaim mirrors a claim onto the database_operations row so
// the existing read model reports the same state.
func projectOperationClaim(ctx context.Context, tx *sql.Tx, id, owner, leaseExpires, now string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE database_operations
		SET status='running',progress_step='starting',progress_percent=1,
		    started_at=CASE WHEN started_at='' THEN ? ELSE started_at END,
		    lease_owner=?,lease_expires_at=?,attempt_count=attempt_count+1,updated_at=?
		WHERE id=?`, now, owner, leaseExpires, now, id)
	return err
}
