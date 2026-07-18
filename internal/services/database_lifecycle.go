package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/databases"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
)

const DatabaseVolumeRetention = 7 * 24 * time.Hour

type DeleteDatabaseRuntimeResult struct {
	Instances  []repository.DatabaseInstance
	PurgeAfter time.Time
}

// DeleteDatabaseServiceAndRuntime removes only the database containers and then
// starts the retained-volume window. Named volumes are deliberately untouched.
func DeleteDatabaseServiceAndRuntime(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, serviceID, actor string) (DeleteDatabaseRuntimeResult, error) {
	service, err := store.GetService(ctx, serviceID)
	if err != nil {
		return DeleteDatabaseRuntimeResult{}, err
	}
	if service.ServiceType != "database" {
		return DeleteDatabaseRuntimeResult{}, ErrCode("service_type_not_database", fmt.Errorf("service is %s", service.ServiceType))
	}
	databaseService, err := store.GetDatabaseService(ctx, serviceID)
	if err != nil {
		return DeleteDatabaseRuntimeResult{}, ErrCode("database_service_not_found", err)
	}
	engine, ok := databases.Find(databaseService.Engine)
	if !ok {
		return DeleteDatabaseRuntimeResult{}, ErrCode("database_engine_unsupported", fmt.Errorf("engine %s is unavailable", databaseService.Engine))
	}
	instances, err := store.BeginDatabaseServiceDeletion(ctx, serviceID)
	if err != nil {
		return DeleteDatabaseRuntimeResult{}, ErrCode("database_operation_in_progress", err)
	}
	needsDocker := false
	hasGatewayRoute := false
	for _, instance := range instances {
		if strings.TrimSpace(instance.DockerContainerID) != "" {
			needsDocker = true
		}
		if _, routeErr := store.GetDatabaseGatewayRouteByInstance(ctx, instance.ID); routeErr == nil {
			hasGatewayRoute = true
		} else if !errors.Is(routeErr, repository.ErrDatabaseGatewayRouteNotFound) {
			return DeleteDatabaseRuntimeResult{}, ErrCode("database_gateway_route_lookup_failed", routeErr)
		}
	}
	if needsDocker || hasGatewayRoute {
		client, err := docker.NewClient(ctx)
		if err != nil {
			return DeleteDatabaseRuntimeResult{}, ErrCode("docker_unavailable", err)
		}
		defer client.Close()
		if hasGatewayRoute {
			if cfg == nil || sealer == nil {
				return DeleteDatabaseRuntimeResult{}, ErrCode("database_gateway_revocation_unavailable", errors.New("gateway configuration and encryption key are required to revoke external access"))
			}
			for _, instance := range instances {
				if err := revokePostgreSQLGatewayRouteForDeletion(ctx, log, cfg, store, sealer, client, instance); err != nil {
					return DeleteDatabaseRuntimeResult{}, ErrCode("database_gateway_revocation_failed", err)
				}
			}
		}
		for _, instance := range instances {
			containerID := strings.TrimSpace(instance.DockerContainerID)
			if containerID == "" {
				continue
			}
			inspection, err := docker.InspectManagedContainer(ctx, client, containerID)
			if err != nil {
				if docker.IsNotFound(err) {
					if log != nil {
						log.Warn("database container already absent during retained deletion", "service_id", serviceID, "instance_id", instance.ID, "container_id", containerID)
					}
					continue
				}
				return DeleteDatabaseRuntimeResult{}, ErrCode("database_container_inspection_failed", err)
			}
			if inspection.Labels[docker.ResourceTypeLabel] != "database-container" ||
				inspection.Labels[docker.InstanceIDLabel] != instance.ID {
				return DeleteDatabaseRuntimeResult{}, ErrCode("database_container_ownership_mismatch", fmt.Errorf("container %s does not belong to instance %s", containerID, instance.ID))
			}
			if err := docker.StopAndRemoveWithTimeout(ctx, client, containerID, engine.StopTimeoutSeconds); err != nil {
				return DeleteDatabaseRuntimeResult{}, ErrCode("database_container_remove_failed", err)
			}
			if log != nil {
				log.Info("database container removed; volume retained", "service_id", serviceID, "instance_id", instance.ID, "volume", instance.VolumeName)
			}
		}
	}
	retained, err := store.FinalizeDatabaseServiceDeletion(ctx, serviceID, DatabaseVolumeRetention, actor)
	if err != nil {
		return DeleteDatabaseRuntimeResult{}, ErrCode("database_retention_state_failed", err)
	}
	result := DeleteDatabaseRuntimeResult{Instances: retained}
	if len(retained) > 0 {
		result.PurgeAfter = retained[0].PurgeAfter
	}
	return result, nil
}

func StartDatabasePurgeLoop(ctx context.Context, log *slog.Logger, store *repository.Store) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			purgeDueDatabaseServices(ctx, log, store, time.Now().UTC())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func purgeDueDatabaseServices(ctx context.Context, log *slog.Logger, store *repository.Store, at time.Time) {
	due, err := store.ListDatabaseInstancesDueForPurge(ctx, at, 100)
	if err != nil {
		if log != nil {
			log.Error("list retained databases for purge failed", "error", err)
		}
		return
	}
	serviceIDs := map[string]struct{}{}
	for _, instance := range due {
		serviceIDs[instance.ServiceID] = struct{}{}
	}
	for serviceID := range serviceIDs {
		if err := PurgeDatabaseServiceAndRuntime(ctx, log, store, serviceID, at, "system"); err != nil && log != nil {
			log.Error("purge retained database failed", "service_id", serviceID, "error", err)
		}
	}
}

func PurgeDatabaseServiceAndRuntime(ctx context.Context, log *slog.Logger, store *repository.Store, serviceID string, at time.Time, actor string) error {
	service, err := store.GetService(ctx, serviceID)
	if err != nil {
		return ErrCode("database_service_not_found", err)
	}
	instances, err := store.ListDatabaseInstances(ctx, serviceID)
	if err != nil {
		return ErrCode("database_instances_lookup_failed", err)
	}
	if len(instances) == 0 {
		return ErrCode("database_service_not_found", repository.ErrDatabaseServiceNotFound)
	}
	for _, instance := range instances {
		if instance.DeletedAt.IsZero() || instance.PurgeAfter.IsZero() || instance.PurgeAfter.After(at.UTC()) {
			return ErrCode("database_retention_active", fmt.Errorf("instance %s is not eligible for purge", instance.ID))
		}
	}
	if err := store.EnsureDatabaseServicePurgeReady(ctx, serviceID); err != nil {
		return ErrCode("database_purge_operation_active", err)
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		return ErrCode("docker_unavailable", err)
	}
	defer client.Close()
	for _, instance := range instances {
		if err := docker.RemoveManagedDatabaseVolume(ctx, client, instance.VolumeName, instance.ID); err != nil {
			return ErrCode("database_volume_purge_failed", err)
		}
		if log != nil {
			log.Info("database retained volume purged", "service_id", serviceID, "instance_id", instance.ID, "volume", instance.VolumeName)
		}
	}
	if err := store.RecordPlatformEvent(ctx, repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EventType: "database", Status: "purged", Actor: strings.TrimSpace(actor), Message: "Database volumes permanently purged", Detail: service.Name}); err != nil {
		return ErrCode("database_purge_audit_failed", err)
	}
	if err := store.PurgeDatabaseServiceRecords(ctx, serviceID, at); err != nil {
		return ErrCode("database_record_purge_failed", err)
	}
	for _, instance := range instances {
		retained, retainedErr := store.EnvironmentHasRetainedDatabaseInstances(ctx, instance.EnvironmentID)
		if retainedErr != nil {
			if log != nil {
				log.Warn("database environment network cleanup skipped", "environment_id", instance.EnvironmentID, "error", retainedErr)
			}
			continue
		}
		if retained {
			continue
		}
		if _, cleanupErr := docker.RemoveEnvironmentNetworkIfEmpty(ctx, client, instance.EnvironmentID); cleanupErr != nil && log != nil {
			log.Warn("database environment network cleanup failed", "environment_id", instance.EnvironmentID, "error", cleanupErr)
		}
	}
	return nil
}
