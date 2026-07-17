package services

import (
	"context"
	"log/slog"
	"time"

	backupstorage "github.com/hostforge/hostforge/internal/backups"
	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/robfig/cron/v3"
)

func NextDatabaseBackupSchedule(schedule, timezone string, after time.Time) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.Next(after.In(location)).UTC(), nil
}

func StartDatabaseBackupRetentionLoop(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer) {
	if sealer == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			purgeExpiredDatabaseBackups(ctx, log, store, sealer, time.Now().UTC())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func purgeExpiredDatabaseBackups(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, now time.Time) {
	items, err := store.ListExpiredDatabaseBackups(ctx, now, 100)
	if err != nil {
		if log != nil {
			log.Error("list expired database backups failed", "error", err)
		}
		return
	}
	for _, backup := range items {
		destination, err := store.GetBackupDestinationSealed(ctx, backup.DestinationID)
		if err != nil {
			continue
		}
		access, accessErr := sealer.Open(destination.AccessKeyCT)
		secret, secretErr := sealer.Open(destination.SecretKeyCT)
		if accessErr != nil || secretErr != nil {
			for _, value := range [][]byte{access, secret} {
				for index := range value {
					value[index] = 0
				}
			}
			continue
		}
		client, err := backupstorage.NewClient(ctx, backupstorage.Destination{Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket, PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption, SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(access), SecretKey: string(secret)})
		for _, value := range [][]byte{access, secret} {
			for index := range value {
				value[index] = 0
			}
		}
		if err != nil {
			continue
		}
		_ = store.MarkDatabaseBackupDeleting(ctx, backup.ID)
		if err := client.Delete(ctx, backup.ObjectKey); err != nil {
			if log != nil {
				log.Warn("delete expired database backup failed; will retry", "backup_id", backup.ID, "error", err)
			}
			continue
		}
		if err := store.DeleteDatabaseBackupRecord(ctx, backup.ID); err != nil && log != nil {
			log.Error("delete expired database backup record failed", "backup_id", backup.ID, "error", err)
		}
	}
}

func StartDatabaseBackupScheduleLoop(ctx context.Context, log *slog.Logger, store *repository.Store, maxTransfersPerHour int) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			queueDueDatabaseBackups(ctx, log, store, time.Now().UTC(), maxTransfersPerHour)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func queueDueDatabaseBackups(ctx context.Context, log *slog.Logger, store *repository.Store, now time.Time, maxTransfersPerHour int) {
	policies, err := store.ListDueDatabaseBackupPolicies(ctx, now, 100)
	if err != nil {
		if log != nil {
			log.Error("list scheduled database backups failed", "error", err)
		}
		return
	}
	for _, policy := range policies {
		next, err := NextDatabaseBackupSchedule(policy.Schedule, policy.Timezone, now)
		if err != nil {
			if log != nil {
				log.Error("database backup schedule became invalid", "instance_id", policy.DatabaseInstanceID, "error", err)
			}
			continue
		}
		if _, _, err := store.QueueDatabaseBackup(ctx, policy.DatabaseInstanceID, policy.DestinationID, "scheduled", "scheduler", policy.RetentionDays, maxTransfersPerHour); err != nil {
			if log != nil {
				log.Warn("scheduled database backup could not be queued; will retry", "instance_id", policy.DatabaseInstanceID, "error", err)
			}
			continue
		}
		if err := store.AdvanceDatabaseBackupPolicySchedule(ctx, policy.DatabaseInstanceID, now, next); err != nil && log != nil {
			log.Error("advance database backup schedule failed", "instance_id", policy.DatabaseInstanceID, "error", err)
		}
	}
}
