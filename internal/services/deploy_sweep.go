package services

import (
	"context"
	"log/slog"

	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

// SweepOrphanedDeployContainers removes application containers a killed deploy
// left running. It runs once at startup, after operation recovery has settled
// the deployment states and before the deploy runtime starts claiming work --
// see the call site in cmd/server for why that ordering is load-bearing.
//
// It mirrors reconcileDatabaseInstances: iterate the rows the store selects,
// inspect each container, and act only on what is positively identified as
// owned. The store already filters to RUNNING rows whose deployment is
// terminal and not active; this adds the label check that reconcile uses,
// because a leaked container costs a port and some memory while a wrongly
// removed one is an outage. Anything that cannot be confirmed is left alone
// and logged.
//
// It is best-effort: a failure on one container is logged and the sweep moves
// on, and a failure to list is returned but never fatal to startup.
func SweepOrphanedDeployContainers(ctx context.Context, log *slog.Logger, store *repository.Store, dockerClient *mobyclient.Client) (removed int, err error) {
	candidates, err := store.ListSweepableDeployContainers(ctx)
	if err != nil {
		return 0, err
	}
	for _, candidate := range candidates {
		inspection, inspectErr := docker.InspectManagedContainer(ctx, dockerClient, candidate.DockerContainerID)
		if inspectErr != nil {
			// Already gone: the container gets no reprieve, but the row is
			// stale and should stop appearing in this sweep and in port
			// accounting.
			if docker.IsNotFound(inspectErr) {
				if statusErr := store.UpdateContainerStatus(ctx, candidate.ContainerRowID, "REMOVED"); statusErr != nil {
					log.Warn("failed to mark missing orphan container removed", "container_row_id", candidate.ContainerRowID, "error", statusErr)
				}
				continue
			}
			log.Warn("orphan container inspection failed; leaving it alone", "docker_container_id", ShortID(candidate.DockerContainerID), "error", inspectErr)
			continue
		}
		if inspection.Labels[docker.ResourceTypeLabel] != "application-container" ||
			inspection.Labels[docker.ServiceIDLabel] != candidate.ServiceID ||
			inspection.Labels[docker.EnvironmentIDLabel] != candidate.EnvironmentID {
			log.Warn("orphan container ownership labels do not match its row; leaving it alone",
				"docker_container_id", ShortID(candidate.DockerContainerID),
				"container_row_id", candidate.ContainerRowID)
			continue
		}
		if stopErr := docker.StopAndRemove(ctx, dockerClient, candidate.DockerContainerID); stopErr != nil {
			log.Warn("failed to remove orphan container", "docker_container_id", ShortID(candidate.DockerContainerID), "error", stopErr)
			continue
		}
		if statusErr := store.UpdateContainerStatus(ctx, candidate.ContainerRowID, "REMOVED"); statusErr != nil {
			log.Warn("removed orphan container but failed to mark its row", "container_row_id", candidate.ContainerRowID, "error", statusErr)
		}
		log.Info("swept orphaned deploy container",
			"docker_container_id", ShortID(candidate.DockerContainerID),
			"deployment_id", candidate.DeploymentID,
			"service_id", candidate.ServiceID,
			"environment_id", candidate.EnvironmentID)
		removed++
	}
	return removed, nil
}
