package services

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
)

type DeleteRuntimeResult struct {
	CaddySyncError string
}

func cleanupServiceRuntime(ctx context.Context, log *slog.Logger, store *repository.Store, serviceID string) error {
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
	client, err := docker.NewClient(ctx)
	if err != nil {
		return ErrCode("docker_unavailable", err)
	}
	defer client.Close()
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

func DeleteServiceAndRuntime(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, serviceID string) (DeleteRuntimeResult, error) {
	if err := cleanupServiceRuntime(ctx, log, store, serviceID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	if err := store.DeleteService(ctx, serviceID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	return DeleteRuntimeResult{CaddySyncError: syncCaddyAfterDelete(ctx, log, cfg, store)}, nil
}

func DeleteApplicationAndRuntime(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, applicationID string) (DeleteRuntimeResult, error) {
	services, err := store.ListApplicationServices(ctx, applicationID)
	if err != nil {
		return DeleteRuntimeResult{}, err
	}
	for _, service := range services {
		if err := cleanupServiceRuntime(ctx, log, store, service.ID); err != nil {
			return DeleteRuntimeResult{}, err
		}
	}
	if err := store.DeleteApplication(ctx, applicationID); err != nil {
		return DeleteRuntimeResult{}, err
	}
	return DeleteRuntimeResult{CaddySyncError: syncCaddyAfterDelete(ctx, log, cfg, store)}, nil
}
