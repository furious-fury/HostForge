package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	backupstorage "github.com/furious-fury/HostForge/internal/backups"
	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/repository"
)

// StartControlPlaneSnapshotLoop periodically takes a VACUUM INTO snapshot of
// hostforge.db, optionally uploads it to a configured backup_destinations
// row, and purges snapshots past the retention window (ADR-0002 §17.2).
// Distinct from and independent of the pre-migration snapshot taken by
// internal/database.OpenSQLite.
func StartControlPlaneSnapshotLoop(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer) {
	if cfg.ControlPlaneSnapshotIntervalMinutes <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.ControlPlaneSnapshotIntervalMinutes) * time.Minute)
		defer ticker.Stop()
		for {
			now := time.Now().UTC()
			maybeRunControlPlaneSnapshot(ctx, log, cfg, store, sealer, now)
			purgeExpiredControlPlaneSnapshots(ctx, log, cfg, store, sealer, now)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// maybeRunControlPlaneSnapshot runs a snapshot only if the configured
// interval has elapsed since the most recent attempt (of any status). This
// is what makes it safe to check immediately at loop start on every boot,
// rather than always waiting a full interval before the first snapshot —
// without it, a restart shortly after the last attempt would spam a
// redundant snapshot every time the server restarts.
func maybeRunControlPlaneSnapshot(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, now time.Time) {
	last, err := store.LatestControlPlaneSnapshot(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		if log != nil {
			log.Error("read latest control-plane snapshot failed", "error", err)
		}
		return
	}
	if err == nil && now.Sub(last.CreatedAt) < time.Duration(cfg.ControlPlaneSnapshotIntervalMinutes)*time.Minute {
		return
	}
	runControlPlaneSnapshot(ctx, log, cfg, store, sealer, now)
}

func runControlPlaneSnapshot(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, now time.Time) {
	if err := os.MkdirAll(cfg.ControlPlaneSnapshotDir, 0o755); err != nil {
		if log != nil {
			log.Error("create control-plane snapshot dir failed", "error", err)
		}
		return
	}
	row, err := store.CreateControlPlaneSnapshot(ctx)
	if err != nil {
		if log != nil {
			log.Error("create control-plane snapshot record failed", "error", err)
		}
		return
	}
	filename := "control-plane-" + now.Format("20060102T150405Z") + ".sqlite"
	path := filepath.Join(cfg.ControlPlaneSnapshotDir, filename)

	if err := store.VacuumInto(ctx, path); err != nil {
		if _, completeErr := store.CompleteControlPlaneSnapshot(ctx, row.ID, repository.CompleteControlPlaneSnapshotInput{Status: "failed", ErrorMessage: err.Error()}); completeErr != nil && log != nil {
			log.Error("record failed control-plane snapshot failed", "snapshot_id", row.ID, "error", completeErr)
		}
		if log != nil {
			log.Error("control-plane snapshot vacuum failed", "snapshot_id", row.ID, "error", err)
		}
		return
	}
	var size int64
	if info, statErr := os.Stat(path); statErr == nil {
		size = info.Size()
	}

	remoteKey := uploadControlPlaneSnapshot(ctx, log, store, sealer, cfg.ControlPlaneSnapshotDestinationID, path, filename)

	if _, err := store.CompleteControlPlaneSnapshot(ctx, row.ID, repository.CompleteControlPlaneSnapshotInput{
		Status: "success", SnapshotPath: path, RemoteKey: remoteKey, SizeBytes: size,
	}); err != nil && log != nil {
		log.Error("record control-plane snapshot completion failed", "snapshot_id", row.ID, "error", err)
	}
}

// uploadControlPlaneSnapshot mirrors purgeExpiredDatabaseBackups's
// destination-resolution shape (database_backup_schedule.go): look up the
// sealed destination, open the sealer, build an S3 client, zero key
// material immediately after building the client. Returns "" (local-only)
// on any failure — upload is best-effort; the local file is still a valid,
// already-recorded-success snapshot either way.
func uploadControlPlaneSnapshot(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, destinationID, localPath, filename string) string {
	destinationID = strings.TrimSpace(destinationID)
	if destinationID == "" || sealer == nil {
		return ""
	}
	destination, err := store.GetBackupDestinationSealed(ctx, destinationID)
	if err != nil {
		if log != nil {
			log.Warn("control-plane snapshot destination lookup failed; keeping local copy only", "error", err)
		}
		return ""
	}
	access, accessErr := sealer.Open(destination.AccessKeyCT)
	secret, secretErr := sealer.Open(destination.SecretKeyCT)
	if accessErr != nil || secretErr != nil {
		zeroBytes(access)
		zeroBytes(secret)
		return ""
	}
	client, err := backupstorage.NewClient(ctx, backupstorage.Destination{
		Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket,
		PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption,
		SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(access), SecretKey: string(secret),
	})
	zeroBytes(access)
	zeroBytes(secret)
	if err != nil {
		return ""
	}
	f, err := os.Open(localPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	key := strings.Trim(strings.Join([]string{destination.ObjectPrefix, "control-plane", filename}, "/"), "/")
	if err := client.Put(ctx, key, f, "application/vnd.sqlite3"); err != nil {
		if log != nil {
			log.Warn("control-plane snapshot upload failed; local copy retained", "error", err)
		}
		return ""
	}
	return key
}

func purgeExpiredControlPlaneSnapshots(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, now time.Time) {
	cutoff := now.AddDate(0, 0, -cfg.ControlPlaneSnapshotRetentionDays)
	items, err := store.ListExpiredControlPlaneSnapshots(ctx, cutoff, 100)
	if err != nil {
		if log != nil {
			log.Error("list expired control-plane snapshots failed", "error", err)
		}
		return
	}
	for _, snap := range items {
		if snap.SnapshotPath != "" {
			if err := os.Remove(snap.SnapshotPath); err != nil && !os.IsNotExist(err) && log != nil {
				log.Warn("delete expired control-plane snapshot file failed", "snapshot_id", snap.ID, "path", snap.SnapshotPath, "error", err)
			}
		}
		if snap.RemoteKey != "" && strings.TrimSpace(cfg.ControlPlaneSnapshotDestinationID) != "" && sealer != nil {
			deleteRemoteControlPlaneSnapshot(ctx, log, store, sealer, cfg.ControlPlaneSnapshotDestinationID, snap.RemoteKey)
		}
		if err := store.DeleteControlPlaneSnapshotRecord(ctx, snap.ID); err != nil && log != nil {
			log.Error("delete expired control-plane snapshot record failed", "snapshot_id", snap.ID, "error", err)
		}
	}
}

func deleteRemoteControlPlaneSnapshot(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, destinationID, key string) {
	destination, err := store.GetBackupDestinationSealed(ctx, strings.TrimSpace(destinationID))
	if err != nil {
		return
	}
	access, accessErr := sealer.Open(destination.AccessKeyCT)
	secret, secretErr := sealer.Open(destination.SecretKeyCT)
	if accessErr != nil || secretErr != nil {
		zeroBytes(access)
		zeroBytes(secret)
		return
	}
	client, err := backupstorage.NewClient(ctx, backupstorage.Destination{
		Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket,
		PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption,
		SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(access), SecretKey: string(secret),
	})
	zeroBytes(access)
	zeroBytes(secret)
	if err != nil {
		return
	}
	if err := client.Delete(ctx, key); err != nil && log != nil {
		log.Warn("delete expired remote control-plane snapshot failed", "key", key, "error", err)
	}
}
