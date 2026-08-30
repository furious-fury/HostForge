package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

type DeleteRuntimeResult struct {
	CaddySyncError string
}

func cleanupServiceRuntime(ctx context.Context, log *slog.Logger, store *repository.Store, dockerClient *mobyclient.Client, serviceID string) error {
	service, err := store.GetService(ctx, serviceID)
	if err != nil {
		return err
	}
	if service.ServiceType != "application" {
		return ErrCode("database_service_delete_requires_retention", errors.New("database services require the retained-volume deletion workflow"))
	}
	deployments, err := store.ListServiceDeployments(ctx, serviceID, "", 500)
	if err != nil {
		return ErrCode("delete_deployments_lookup_failed", err)
	}
	type runtimeArtifact struct {
		containerID string
		imageRef    string
	}
	artifacts := make([]runtimeArtifact, 0, len(deployments))
	seenContainers := map[string]struct{}{}
	for _, deployment := range deployments {
		container, err := store.GetContainerByDeploymentID(ctx, deployment.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return ErrCode("delete_container_lookup_failed", err)
		}
		containerID := strings.TrimSpace(container.DockerContainerID)
		if containerID == "" || strings.EqualFold(strings.TrimSpace(container.Status), "REMOVED") {
			continue
		}
		if _, seen := seenContainers[containerID]; seen {
			continue
		}
		seenContainers[containerID] = struct{}{}
		artifacts = append(artifacts, runtimeArtifact{containerID: containerID, imageRef: strings.TrimSpace(deployment.ImageRef)})
	}
	if len(artifacts) == 0 {
		return nil
	}
	client := dockerClient
	for _, artifact := range artifacts {
		if err := docker.StopAndRemove(ctx, client, artifact.containerID); err != nil {
			return ErrCode("delete_container_failed", err)
		}
		if artifact.imageRef != "" {
			if err := docker.RemoveImage(ctx, client, artifact.imageRef); err != nil && log != nil {
				log.Warn("failed to remove service image during delete", "service_id", serviceID, "image", artifact.imageRef, "error", err)
			}
		}
	}
	return nil
}

func syncCaddyAfterDelete(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store) string {
	if !cfg.SyncCaddy && !cfg.DomainSyncAfterMutate {
		return ""
	}
	if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
		if log != nil {
			log.Error("caddy sync after delete failed", "error", err)
		}
		return FirstPublicCode(err)
	}
	return ""
}

func DeleteServiceAndRuntime(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, dockerClient *mobyclient.Client, serviceID string) (DeleteRuntimeResult, error) {
	service, err := store.GetService(ctx, serviceID)
	if err != nil {
		return DeleteRuntimeResult{}, err
	}
	if service.ServiceType != "application" {
		return DeleteRuntimeResult{}, ErrCode("database_service_delete_requires_retention", errors.New("database services require the retained-volume deletion workflow"))
	}
	if err := cleanupServiceRuntime(ctx, log, store, dockerClient, serviceID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	if err := store.DeleteService(ctx, serviceID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	return DeleteRuntimeResult{CaddySyncError: syncCaddyAfterDelete(ctx, log, cfg, store)}, nil
}

func DeleteApplicationAndRuntime(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, dockerClient *mobyclient.Client, applicationID string) (DeleteRuntimeResult, error) {
	environments, err := store.ListApplicationEnvironments(ctx, applicationID)
	if err != nil {
		return DeleteRuntimeResult{}, err
	}
	services, err := store.ListApplicationServices(ctx, applicationID)
	if err != nil {
		return DeleteRuntimeResult{}, err
	}
	for _, service := range services {
		if service.ServiceType != "application" {
			return DeleteRuntimeResult{}, ErrCode("application_has_database_services", errors.New("delete database services through their retained-volume workflow first"))
		}
	}
	for _, service := range services {
		if err := cleanupServiceRuntime(ctx, log, store, dockerClient, service.ID); err != nil {
			return DeleteRuntimeResult{}, err
		}
	}
	if err := store.DeleteApplication(ctx, applicationID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	// Network cleanup is best-effort: an orphaned empty network is a minor leak,
	// not a reason to fail a delete that already committed. dockerClient is nil
	// only in tests; production always constructs one at startup.
	if dockerClient != nil {
		for _, environment := range environments {
			if _, cleanupErr := docker.RemoveEnvironmentNetworkIfEmpty(ctx, dockerClient, environment.ID); cleanupErr != nil && log != nil {
				log.Warn("application environment network cleanup failed", "environment_id", environment.ID, "error", cleanupErr)
			}
		}
	}
	return DeleteRuntimeResult{CaddySyncError: syncCaddyAfterDelete(ctx, log, cfg, store)}, nil
}
