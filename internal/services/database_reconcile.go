package services

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/databases"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

func StartDatabaseReconciliationLoop(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client) {
	if count, err := store.RequeueExpiredDatabaseOperations(ctx, time.Now().UTC()); err != nil {
		if log != nil {
			log.Error("recover interrupted database operations failed", "error", err)
		}
	} else if count > 0 && log != nil {
		log.Info("requeued expired database operation leases", "count", count)
	}
	failExhaustedDatabaseOperations(ctx, log, store)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			reconcileDatabaseInstances(ctx, log, store, sealer, dockerClient)
			// Also on the ticker, not only at boot: an operation exhausts its
			// attempts while the process is running, and without this it would
			// stay queued — skipped by the claim query, polled by the UI — until
			// the next restart.
			failExhaustedDatabaseOperations(ctx, log, store)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func failExhaustedDatabaseOperations(ctx context.Context, log *slog.Logger, store *repository.Store) {
	count, err := store.FailExhaustedDatabaseOperations(ctx, time.Now().UTC())
	if err != nil {
		if log != nil {
			log.Error("fail exhausted database operations failed", "error", err)
		}
		return
	}
	if count > 0 && log != nil {
		log.Warn("failed database operations that exceeded the retry limit",
			"count", count, "max_attempts", repository.MaxDatabaseOperationAttempts)
	}
}

func reconcileDatabaseInstances(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client) {
	instances, err := store.ListAllDatabaseInstances(ctx)
	if err != nil {
		if log != nil {
			log.Error("list database instances for reconciliation failed", "error", err)
		}
		return
	}
	if len(instances) == 0 {
		return
	}
	client := dockerClient
	volumeUsage, volumeUsageErr := docker.ManagedVolumeUsage(ctx, client)
	if volumeUsageErr != nil && log != nil {
		log.Warn("database volume usage sampling unavailable", "error", volumeUsageErr)
	}
	for _, instance := range instances {
		if !instance.DeletedAt.IsZero() || instance.DesiredState == "deleted" || strings.TrimSpace(instance.DockerContainerID) == "" {
			continue
		}
		service, serviceErr := store.GetService(ctx, instance.ServiceID)
		databaseService, databaseServiceErr := store.GetDatabaseService(ctx, instance.ServiceID)
		engine, engineFound := databases.Find(databaseService.Engine)
		if serviceErr != nil || databaseServiceErr != nil || !engineFound {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "database catalog identity is unavailable"})
			continue
		}
		if err := docker.ValidateEnvironmentNetwork(ctx, client, service.ApplicationID, instance.EnvironmentID); err != nil {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "private environment network is missing or mismatched"})
			continue
		}
		if err := docker.ValidateManagedVolume(ctx, client, instance.VolumeName, map[string]string{docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: instance.EnvironmentID, docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: instance.ID}); err != nil {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "persistent database volume is missing or mismatched"})
			continue
		}
		inspection, err := docker.InspectManagedContainer(ctx, client, instance.DockerContainerID)
		if err != nil {
			state := repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "database container inspection failed"}
			if errdefs.IsNotFound(err) {
				state.ClearContainerID = true
				state.HealthMessage = "managed database container is missing"
			}
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, state)
			continue
		}
		if databaseManagedContainerDrift(inspection, instance, engine) {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "managed database runtime configuration drift detected"})
			continue
		}
		if inspection.Labels[docker.ResourceTypeLabel] != "database-container" || inspection.Labels[docker.InstanceIDLabel] != instance.ID {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: "database container ownership mismatch"})
			continue
		}
		if instance.DesiredState == "stopped" {
			status, message := "stopped", "stopped by operator"
			if inspection.Running {
				status, message = "unhealthy", "container is running despite stopped desired state"
			}
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: status, HealthMessage: message, HealthCheckedAt: time.Now().UTC()})
			continue
		}
		if !inspection.Running {
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "unhealthy", HealthMessage: "database container is not running", HealthCheckedAt: time.Now().UTC()})
			continue
		}
		if sealer == nil {
			continue
		}
		credential, credentialErr := store.GetDatabaseCredentialSealed(ctx, instance.ID)
		if credentialErr != nil {
			continue
		}
		password, passwordErr := sealer.Open(credential.PasswordCT)
		adminPassword, adminErr := sealer.Open(credential.AdminPasswordCT)
		if passwordErr != nil || adminErr != nil {
			for _, secret := range [][]byte{password, adminPassword} {
				for index := range secret {
					secret[index] = 0
				}
			}
			_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{Status: "unhealthy", HealthMessage: "database credentials could not be decrypted", HealthCheckedAt: time.Now().UTC()})
			continue
		}
		healthErr := checkDatabaseHealth(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, password, adminPassword)
		for _, secret := range [][]byte{password, adminPassword} {
			for index := range secret {
				secret[index] = 0
			}
		}
		status, message := "healthy", "ready"
		if healthErr != nil {
			status, message = "unhealthy", databaseService.Engine+" authenticated health check failed"
		}
		state := repository.UpdateDatabaseInstanceStateInput{Status: status, HealthMessage: message, HealthCheckedAt: time.Now().UTC()}
		if used, ok := volumeUsage[instance.VolumeName]; ok {
			state.StorageUsedBytes = &used
			state.StorageCheckedAt = time.Now().UTC()
		}
		_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, state)
	}
}

func databaseRuntimeConfigurationDrift(inspection docker.ManagedContainerInspection, instance repository.DatabaseInstance, engine databases.Engine, aliasFound bool) bool {
	return inspection.ImageRef != instance.ImageRef ||
		inspection.NanoCPUs != int64(instance.CPULimitMillis)*1_000_000 ||
		inspection.MemoryBytes != instance.MemoryLimitBytes ||
		inspection.PublishedPorts ||
		inspection.VolumeMounts[instance.VolumeName] != engine.VolumeTarget ||
		!aliasFound
}

func databaseManagedContainerDrift(inspection docker.ManagedContainerInspection, instance repository.DatabaseInstance, engine databases.Engine) bool {
	if inspection.Labels[docker.ResourceTypeLabel] != "database-container" || inspection.Labels[docker.InstanceIDLabel] != instance.ID || inspection.Labels[docker.ServiceIDLabel] != instance.ServiceID || inspection.Labels[docker.EnvironmentIDLabel] != instance.EnvironmentID {
		return true
	}
	aliasFound := false
	for _, alias := range inspection.NetworkAliases[docker.EnvironmentNetworkName(instance.EnvironmentID)] {
		if alias == instance.NetworkAlias {
			aliasFound = true
			break
		}
	}
	return databaseRuntimeConfigurationDrift(inspection, instance, engine, aliasFound)
}
