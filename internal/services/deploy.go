package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/caddy"
	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/nixpacks"
	"github.com/hostforge/hostforge/internal/obs"
	"github.com/hostforge/hostforge/internal/redact"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/reqctx"
)

// DeployPrepareInput defines values needed to create a queued deployment.
type DeployPrepareInput struct {
	Project    models.Project
	RepoURL    string
	Branch     string
	CommitHash string
}

// DeployJob contains persisted and computed data for a deployment execution.
type DeployJob struct {
	Project           models.Project
	Deployment        models.Deployment
	PreviousContainer models.Container
	RepoURL           string
	Branch            string
	Worktree          string
	ImageRef          string
	ContainerName     string
	LogsPath          string
}

// DeployResult captures output values from a successful deployment.
type DeployResult struct {
	DeploymentID  string
	ContainerID   string
	ImageRef      string
	ContainerPort int
	HostPort      int
	URL           string
}

// RollbackResult captures values from a successful rollback action.
type RollbackResult struct {
	FromDeploymentID string
	ToDeploymentID   string
	ContainerID      string
	HostPort         int
	URL              string
}

// PrepareDeploy creates a queued deployment row and returns execution metadata.
func PrepareDeploy(ctx context.Context, cfg *config.Config, store *repository.Store, in DeployPrepareInput) (DeployJob, error) {
	repoURL := strings.TrimSpace(in.RepoURL)
	branch := strings.TrimSpace(in.Branch)
	if repoURL == "" {
		repoURL = strings.TrimSpace(in.Project.RepoURL)
	}
	if branch == "" {
		branch = strings.TrimSpace(in.Project.Branch)
	}
	slug := git.WorktreeDir(repoURL, branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildID := time.Now().UTC().Format("20060102t150405")
	imageRef := fmt.Sprintf("hostforge/%s:%s", slug, buildID)
	containerName := fmt.Sprintf("hostforge-%s-%s", slug[:12], buildID)

	previousDeployment, err := store.GetLatestSuccessfulDeploymentByProjectID(ctx, in.Project.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DeployJob{}, fmt.Errorf("previous deployment state: %w", err)
	}
	var previousContainer models.Container
	if err == nil {
		previousContainer, err = store.GetContainerByDeploymentID(ctx, previousDeployment.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return DeployJob{}, fmt.Errorf("previous container state: %w", err)
		}
	}

	deployment, err := store.CreateDeployment(ctx, repository.CreateDeploymentInput{
		ProjectID:  in.Project.ID,
		CommitHash: strings.TrimSpace(in.CommitHash),
		ImageRef:   imageRef,
		Worktree:   worktree,
	})
	if err != nil {
		return DeployJob{}, fmt.Errorf("deployment state: %w", err)
	}
	logsPath := filepath.Join(cfg.LogsDir(), deployment.ID+".log")
	if err := store.UpdateDeploymentLogsPath(ctx, deployment.ID, logsPath); err != nil {
		return DeployJob{}, fmt.Errorf("deployment log path state: %w", err)
	}
	deployment.LogsPath = logsPath

	// Pre-create the log file so async log subscribers (e.g. the wizard's WebSocket)
	// can attach immediately after PrepareDeploy returns, before ExecuteDeploy opens
	// the file for writing. Without this, an early subscriber would hit ErrNotExist
	// and the stream would close prematurely.
	if err := os.MkdirAll(filepath.Dir(logsPath), 0o755); err != nil {
		return DeployJob{}, fmt.Errorf("create logs dir: %w", err)
	}
	if f, err := os.OpenFile(logsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		_ = f.Close()
	}

	return DeployJob{
		Project:           in.Project,
		Deployment:        deployment,
		PreviousContainer: previousContainer,
		RepoURL:           repoURL,
		Branch:            branch,
		Worktree:          worktree,
		ImageRef:          imageRef,
		ContainerName:     containerName,
		LogsPath:          logsPath,
	}, nil
}

func recordDeployObs(ctx context.Context, log *slog.Logger, job DeployJob, step, status string, started time.Time, durMs int64, errCode string) {
	obs.RecordDeployStep(ctx, log, repository.DeployStepRecord{
		DeploymentID: job.Deployment.ID,
		ProjectID:      job.Project.ID,
		RequestID:      reqctx.RequestID(ctx),
		Step:           step,
		Status:         status,
		DurationMS:     durMs,
		ErrorCode:      errCode,
		StartedAt:      started,
		EndedAt:        time.Now().UTC(),
	})
}

// ExecuteDeploy runs clone/build/run/health/cutover for a prepared deployment.
func ExecuteDeploy(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, job DeployJob) (result DeployResult, err error) {
	deployStart := time.Now()
	log = log.With("project_id", job.Project.ID, "deployment_id", job.Deployment.ID)
	defer func() {
		dur := time.Since(deployStart).Milliseconds()
		status := "ok"
		code := ""
		if err != nil {
			status = "failed"
			code = FirstPublicCode(err)
			if code == "" || code == "internal_error" {
				code = "deploy_failed"
			}
		}
		obs.RecordDeployStep(ctx, log, repository.DeployStepRecord{
			DeploymentID: job.Deployment.ID,
			ProjectID:    job.Project.ID,
			RequestID:    reqctx.RequestID(ctx),
			Step:         "deploy_total",
			Status:       status,
			DurationMS:   dur,
			ErrorCode:    code,
			StartedAt:    deployStart.UTC(),
			EndedAt:      time.Now().UTC(),
		})
	}()
	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentBuilding, ""); err != nil {
		return DeployResult{}, ErrCode("deployment_state_update_failed", fmt.Errorf("deployment state: %w", err))
	}

	markFailed := func(stepErr error) {
		code := FirstPublicCode(stepErr)
		if code == "internal_error" {
			code = "deploy_failed"
		}
		if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentFailed, code); err != nil {
			log.Warn("failed to mark deployment failed", "deployment_id", job.Deployment.ID, "error", err)
		}
		log.Error("deploy failed", "deployment_id", job.Deployment.ID, "public_code", code, "error", stepErr)
	}

	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		e := ErrCode("docker_unavailable", err)
		markFailed(e)
		return DeployResult{}, e
	}
	defer dockerClient.Close()

	if err := os.MkdirAll(filepath.Dir(job.LogsPath), 0o755); err != nil {
		e := ErrCode("deploy_mkdir_logs_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}
	logFile, err := os.OpenFile(job.LogsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		e := ErrCode("deploy_log_open_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}
	defer logFile.Close()
	combinedOut := io.MultiWriter(os.Stdout, logFile)

	t0 := time.Now()
	log.Info("deploy step", "step", "clone_start", "repo_url", redact.RepoURLForLog(job.RepoURL), "worktree", job.Worktree)
	_, _ = fmt.Fprintf(combinedOut, "hostforge: cloning url=%s worktree=%s\n", redact.RepoURLForLog(job.RepoURL), job.Worktree)
	if err := git.CloneOrUpdate(ctx, job.RepoURL, job.Branch, job.Worktree); err != nil {
		e := ErrCode("clone_failed", err)
		markFailed(e)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: clone failed: %v\n", err)
		ms := time.Since(t0).Milliseconds()
		log.Info("deploy step", "step", "clone_end", "status", "failed", "duration_ms", ms)
		recordDeployObs(ctx, log, job, "clone", "failed", t0, ms, FirstPublicCode(e))
		return DeployResult{}, e
	}
	msClone := time.Since(t0).Milliseconds()
	log.Info("deploy step", "step", "clone_end", "status", "ok", "duration_ms", msClone)
	recordDeployObs(ctx, log, job, "clone", "ok", t0, msClone, "")

	reservedPorts, err := store.ListAllocatedHostPorts(ctx, "")
	if err != nil {
		e := ErrCode("reserved_ports_lookup_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}
	hostPortValue, err := docker.PickHostPortAvoiding(cfg.HostPort, cfg.PortStart, cfg.PortEnd, reservedPorts)
	if err != nil {
		e := ErrCode("host_port_selection_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}

	t1 := time.Now()
	log.Info("deploy step", "step", "nixpacks_build_start", "dir", job.Worktree, "image", job.ImageRef)
	if err := nixpacks.BuildImageWithWriters(ctx, job.Worktree, job.ImageRef, combinedOut, combinedOut); err != nil {
		e := ErrCode("nixpacks_build_failed", err)
		markFailed(e)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: ===== NIXPACKS IMAGE BUILD FAILED =====\nhostforge: nixpacks failed: %v\n", err)
		ms := time.Since(t1).Milliseconds()
		log.Info("deploy step", "step", "nixpacks_build_end", "status", "failed", "duration_ms", ms)
		recordDeployObs(ctx, log, job, "nixpacks_build", "failed", t1, ms, FirstPublicCode(e))
		return DeployResult{}, e
	}
	_, _ = fmt.Fprintf(combinedOut, "\nhostforge: ===== NIXPACKS IMAGE BUILD SUCCEEDED image=%s =====\n\n", job.ImageRef)
	msNix := time.Since(t1).Milliseconds()
	log.Info("deploy step", "step", "nixpacks_build_end", "status", "ok", "duration_ms", msNix)
	recordDeployObs(ctx, log, job, "nixpacks_build", "ok", t1, msNix, "")

	tRun := time.Now()
	containerID, err := docker.RunContainer(ctx, dockerClient, docker.RunOptions{
		ImageRef:      job.ImageRef,
		ContainerName: job.ContainerName,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
	})
	if err != nil {
		e := ErrCode("run_container_failed", err)
		markFailed(e)
		recordDeployObs(ctx, log, job, "container_start", "failed", tRun, time.Since(tRun).Milliseconds(), FirstPublicCode(e))
		return DeployResult{}, e
	}

	candidateContainer, err := store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID:      job.Deployment.ID,
		DockerContainerID: containerID,
		InternalPort:      cfg.ContainerPort,
		HostPort:          hostPortValue,
		Status:            "RUNNING",
	})
	if err != nil {
		e := ErrCode("container_attach_failed", err)
		markFailed(e)
		recordDeployObs(ctx, log, job, "container_start", "failed", tRun, time.Since(tRun).Milliseconds(), FirstPublicCode(e))
		return DeployResult{}, e
	}
	recordDeployObs(ctx, log, job, "container_start", "ok", tRun, time.Since(tRun).Milliseconds(), "")

	cleanupCandidate := func(reason string) {
		if stopErr := docker.StopAndRemove(ctx, dockerClient, containerID); stopErr != nil {
			log.Warn("failed to remove candidate container", "reason", reason, "docker_container_id", ShortID(containerID), "error", stopErr)
			return
		}
		if err := store.UpdateContainerStatus(ctx, candidateContainer.ID, "REMOVED"); err != nil {
			log.Warn("failed to mark candidate container removed", "container_row_id", candidateContainer.ID, "error", err)
		}
	}

	t2 := time.Now()
	if err := WaitForHealthy(ctx, log, hostPortValue, cfg); err != nil {
		e := ErrCode("health_check_failed", err)
		markFailed(e)
		cleanupCandidate("health check failure")
		ms := time.Since(t2).Milliseconds()
		log.Info("deploy step", "step", "health_check_end", "status", "failed", "host_port", hostPortValue, "duration_ms", ms)
		recordDeployObs(ctx, log, job, "health_check", "failed", t2, ms, FirstPublicCode(e))
		return DeployResult{}, e
	}
	msHealth := time.Since(t2).Milliseconds()
	log.Info("deploy step", "step", "health_check_end", "status", "ok", "host_port", hostPortValue, "duration_ms", msHealth)
	recordDeployObs(ctx, log, job, "health_check", "ok", t2, msHealth, "")

	shouldSyncCaddy := cfg.SyncCaddy
	if !shouldSyncCaddy {
		projectDomains, err := store.ListDomainsByProject(ctx, job.Project.ID)
		if err != nil {
			e := ErrCode("domain_lookup_failed", err)
			markFailed(e)
			cleanupCandidate("domain lookup failure")
			return DeployResult{}, e
		}
		shouldSyncCaddy = len(projectDomains) > 0
	}
	if shouldSyncCaddy {
		t3 := time.Now()
		if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			e := ErrCode("caddy_sync_failed", err)
			markFailed(e)
			cleanupCandidate("caddy sync failure")
			ms := time.Since(t3).Milliseconds()
			log.Info("deploy step", "step", "caddy_sync_end", "status", "failed", "duration_ms", ms)
			recordDeployObs(ctx, log, job, "caddy_sync", "failed", t3, ms, FirstPublicCode(e))
			return DeployResult{}, e
		}
		msCaddy := time.Since(t3).Milliseconds()
		log.Info("deploy step", "step", "caddy_sync_end", "status", "ok", "duration_ms", msCaddy)
		recordDeployObs(ctx, log, job, "caddy_sync", "ok", t3, msCaddy, "")
	}

	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentSuccess, ""); err != nil {
		log.Warn("failed to mark deployment success", "deployment_id", job.Deployment.ID, "error", err)
	}

	if job.PreviousContainer.DockerContainerID != "" && job.PreviousContainer.DockerContainerID != containerID {
		if err := docker.StopAndRemove(ctx, dockerClient, job.PreviousContainer.DockerContainerID); err != nil {
			log.Warn("old container teardown failed", "docker_container_id", ShortID(job.PreviousContainer.DockerContainerID), "error", err)
		} else if err := store.UpdateContainerStatus(ctx, job.PreviousContainer.ID, "REMOVED"); err != nil {
			log.Warn("failed to mark previous container removed", "container_row_id", job.PreviousContainer.ID, "error", err)
		}
	}

	result = DeployResult{
		DeploymentID:  job.Deployment.ID,
		ContainerID:   containerID,
		ImageRef:      job.ImageRef,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
		URL:           fmt.Sprintf("http://127.0.0.1:%d", hostPortValue),
	}
	log.Info("deploy finished", "deployment_id", result.DeploymentID, "image", result.ImageRef, "docker_container_id", ShortID(result.ContainerID), "url", result.URL, "duration_ms_total", time.Since(deployStart).Milliseconds())
	return result, nil
}

// Deploy runs a deployment end-to-end from persisted acceptance to cutover.
func Deploy(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, in DeployPrepareInput) (DeployResult, error) {
	job, err := PrepareDeploy(ctx, cfg, store, in)
	if err != nil {
		return DeployResult{}, err
	}
	return ExecuteDeploy(ctx, log, cfg, store, job)
}

// SyncCaddyRoutes regenerates HostForge-managed routes and updates ssl_status per outcome.
func SyncCaddyRoutes(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store) error {
	syncStart := time.Now()
	domainRoutes, err := store.ListDomainRoutes(ctx)
	if err != nil {
		return ErrCode("caddy_domain_routes_load_failed", err)
	}
	var routes []caddy.Route
	var activeDomainIDs []string
	for _, domainRoute := range domainRoutes {
		if domainRoute.HostPort <= 0 {
			log.Warn("domain has no successful deployment upstream yet", "domain", domainRoute.DomainName)
			if err := store.UpdateDomainSSLStatus(ctx, domainRoute.ID, models.SSLStatusError); err != nil {
				log.Warn("failed to update domain ssl status", "domain_id", domainRoute.ID, "status", models.SSLStatusError, "error", err)
			}
			continue
		}
		routes = append(routes, caddy.Route{
			Domain:   domainRoute.DomainName,
			HostPort: domainRoute.HostPort,
		})
		activeDomainIDs = append(activeDomainIDs, domainRoute.ID)
	}
	syncRes, err := caddy.Sync(ctx, caddy.SyncOptions{
		CaddyBin:      cfg.CaddyBin,
		GeneratedPath: cfg.CaddyGeneratedPath,
		RootConfig:    cfg.CaddyRootConfig,
		Routes:        routes,
	})
	if err != nil {
		for _, domainID := range activeDomainIDs {
			if updateErr := store.UpdateDomainSSLStatus(ctx, domainID, models.SSLStatusError); updateErr != nil {
				log.Warn("failed to update domain ssl status", "domain_id", domainID, "status", models.SSLStatusError, "error", updateErr)
			}
		}
		return ErrCode("caddy_sync_failed", err)
	}
	if !syncRes.Applied {
		log.Warn("caddy reload skipped (admin API unreachable); snippet written and validated. Start Caddy if it is stopped, or run caddy sync again after it is running to live-reload.")
	}
	for _, domainID := range activeDomainIDs {
		if err := store.UpdateDomainSSLStatus(ctx, domainID, models.SSLStatusActive); err != nil {
			log.Warn("failed to update domain ssl status", "domain_id", domainID, "status", models.SSLStatusActive, "error", err)
		}
	}
	log.Info("caddy sync complete", "generated_path", cfg.CaddyGeneratedPath, "root_config", cfg.CaddyRootConfig, "routes", len(routes), "duration_ms", time.Since(syncStart).Milliseconds())
	return nil
}

// RollbackProject rolls traffic back to the previous successful deployment for a project.
//
// Since normal deploy cutover removes the previously running container, rollback creates a
// fresh container from the previous deployment image, marks the current deployment FAILED so
// route resolution picks the previous deployment, syncs Caddy, then removes the superseded
// active container.
func RollbackProject(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, project models.Project) (RollbackResult, error) {
	activeDeployment, err := store.GetLatestSuccessfulDeploymentByProjectID(ctx, project.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RollbackResult{}, ErrCode("rollback_no_active_deployment", err)
		}
		return RollbackResult{}, ErrCode("rollback_active_deployment_lookup_failed", err)
	}
	previousDeployment, err := store.GetPreviousSuccessfulDeploymentByProjectID(ctx, project.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RollbackResult{}, ErrCode("rollback_no_previous_deployment", err)
		}
		return RollbackResult{}, ErrCode("rollback_previous_deployment_lookup_failed", err)
	}
	if strings.TrimSpace(previousDeployment.ImageRef) == "" {
		return RollbackResult{}, ErrCode("rollback_previous_image_missing", fmt.Errorf("previous deployment has no image reference"))
	}

	activeContainer, err := store.GetContainerByDeploymentID(ctx, activeDeployment.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RollbackResult{}, ErrCode("rollback_active_container_lookup_failed", err)
	}

	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		return RollbackResult{}, ErrCode("docker_unavailable", err)
	}
	defer dockerClient.Close()

	reservedPorts, err := store.ListAllocatedHostPorts(ctx, "")
	if err != nil {
		return RollbackResult{}, ErrCode("rollback_reserved_ports_lookup_failed", err)
	}
	hostPortValue, err := docker.PickHostPortAvoiding(cfg.HostPort, cfg.PortStart, cfg.PortEnd, reservedPorts)
	if err != nil {
		return RollbackResult{}, ErrCode("rollback_host_port_selection_failed", err)
	}
	containerName := fmt.Sprintf(
		"hostforge-rb-%s-%s",
		ShortID(previousDeployment.ID),
		time.Now().UTC().Format("20060102t150405"),
	)
	rollbackContainerID, err := docker.RunContainer(ctx, dockerClient, docker.RunOptions{
		ImageRef:      previousDeployment.ImageRef,
		ContainerName: containerName,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
	})
	if err != nil {
		return RollbackResult{}, ErrCode("rollback_run_container_failed", err)
	}

	rollbackContainerRec, err := store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID:      previousDeployment.ID,
		DockerContainerID: rollbackContainerID,
		InternalPort:      cfg.ContainerPort,
		HostPort:          hostPortValue,
		Status:            "RUNNING",
	})
	if err != nil {
		_ = docker.StopAndRemove(ctx, dockerClient, rollbackContainerID)
		return RollbackResult{}, ErrCode("rollback_container_attach_failed", err)
	}

	cleanupRollbackContainer := func(reason string) {
		if stopErr := docker.StopAndRemove(ctx, dockerClient, rollbackContainerID); stopErr != nil {
			log.Warn("failed to cleanup rollback candidate", "reason", reason, "container_id", ShortID(rollbackContainerID), "error", stopErr)
		}
		if statusErr := store.UpdateContainerStatus(ctx, rollbackContainerRec.ID, "REMOVED"); statusErr != nil {
			log.Warn("failed to mark rollback candidate removed", "container_id", rollbackContainerRec.ID, "error", statusErr)
		}
	}

	if err := WaitForHealthy(ctx, log, hostPortValue, cfg); err != nil {
		cleanupRollbackContainer("health check failure")
		return RollbackResult{}, ErrCode("rollback_health_check_failed", err)
	}

	rollbackMessage := fmt.Sprintf("rolled back to deployment %s", previousDeployment.ID)
	if err := store.UpdateDeploymentStatus(ctx, activeDeployment.ID, models.DeploymentFailed, rollbackMessage); err != nil {
		cleanupRollbackContainer("failed to mark active deployment failed")
		return RollbackResult{}, ErrCode("rollback_mark_active_failed", err)
	}

	if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
		_ = store.UpdateDeploymentStatus(ctx, activeDeployment.ID, models.DeploymentSuccess, "")
		cleanupRollbackContainer("caddy sync failure")
		return RollbackResult{}, ErrCode("rollback_caddy_sync_failed", err)
	}

	if activeContainer.DockerContainerID != "" {
		if err := docker.StopAndRemove(ctx, dockerClient, activeContainer.DockerContainerID); err != nil {
			log.Warn("active container teardown failed after rollback", "container_id", ShortID(activeContainer.DockerContainerID), "error", err)
		} else if err := store.UpdateContainerStatus(ctx, activeContainer.ID, "REMOVED"); err != nil {
			log.Warn("failed to mark old active container removed", "container_id", activeContainer.ID, "error", err)
		}
	}

	return RollbackResult{
		FromDeploymentID: activeDeployment.ID,
		ToDeploymentID:   previousDeployment.ID,
		ContainerID:      rollbackContainerID,
		HostPort:         hostPortValue,
		URL:              fmt.Sprintf("http://127.0.0.1:%d", hostPortValue),
	}, nil
}

// RestartResult captures values from a successful restart action.
type RestartResult struct {
	ProjectID    string
	DeploymentID string
	ContainerID  string
	HostPort     int
	URL          string
	Recreated    bool
}

// RestartProject restarts the active container for a project. If the previously bound
// host port is now claimed by a different container (or the in-place restart fails
// because the port is no longer available), the container is recreated from the same
// deployment image on a freshly picked port and the database row is updated.
func RestartProject(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, project models.Project) (RestartResult, error) {
	deployment, err := store.GetLatestSuccessfulDeploymentByProjectID(ctx, project.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RestartResult{}, ErrCode("restart_no_active_deployment", err)
		}
		return RestartResult{}, ErrCode("restart_active_deployment_lookup_failed", err)
	}
	containerRec, err := store.GetContainerByDeploymentID(ctx, deployment.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RestartResult{}, ErrCode("restart_no_active_container", err)
		}
		return RestartResult{}, ErrCode("restart_active_container_lookup_failed", err)
	}

	cli, err := docker.NewClient(ctx)
	if err != nil {
		return RestartResult{}, ErrCode("docker_unavailable", err)
	}
	defer cli.Close()

	reservedPorts, err := store.ListAllocatedHostPorts(ctx, containerRec.ID)
	if err != nil {
		return RestartResult{}, ErrCode("restart_reserved_ports_lookup_failed", err)
	}
	_, portStolen := reservedPorts[containerRec.HostPort]

	if !portStolen {
		if err := docker.RestartContainer(ctx, cli, containerRec.DockerContainerID, 10); err == nil {
			if err := store.UpdateContainerStatus(ctx, containerRec.ID, "RUNNING"); err != nil {
				log.Warn("failed to mark container running", "container_id", containerRec.ID, "error", err)
			}
			return RestartResult{
				ProjectID:    project.ID,
				DeploymentID: deployment.ID,
				ContainerID:  containerRec.DockerContainerID,
				HostPort:     containerRec.HostPort,
				URL:          fmt.Sprintf("http://127.0.0.1:%d", containerRec.HostPort),
				Recreated:    false,
			}, nil
		} else {
			log.Warn("in-place restart failed; will recreate", "container_id", ShortID(containerRec.DockerContainerID), "error", err)
		}
	} else {
		log.Info("host port reused by another container; recreating", "project_id", project.ID, "old_host_port", containerRec.HostPort)
	}

	if strings.TrimSpace(deployment.ImageRef) == "" {
		return RestartResult{}, ErrCode("restart_image_ref_missing", fmt.Errorf("deployment %s has no image reference; cannot recreate", deployment.ID))
	}

	if containerRec.DockerContainerID != "" {
		if err := docker.StopAndRemove(ctx, cli, containerRec.DockerContainerID); err != nil {
			log.Warn("failed to stop+remove old container before recreate", "container_id", ShortID(containerRec.DockerContainerID), "error", err)
		}
	}

	newPort, err := docker.PickHostPortAvoiding(cfg.HostPort, cfg.PortStart, cfg.PortEnd, reservedPorts)
	if err != nil {
		return RestartResult{}, ErrCode("restart_host_port_selection_failed", err)
	}

	internalPort := containerRec.InternalPort
	if internalPort <= 0 {
		internalPort = cfg.ContainerPort
	}

	containerName := fmt.Sprintf(
		"hostforge-rs-%s-%s",
		ShortID(deployment.ID),
		time.Now().UTC().Format("20060102t150405"),
	)
	newContainerID, err := docker.RunContainer(ctx, cli, docker.RunOptions{
		ImageRef:      deployment.ImageRef,
		ContainerName: containerName,
		ContainerPort: internalPort,
		HostPort:      newPort,
	})
	if err != nil {
		return RestartResult{}, ErrCode("restart_run_container_failed", err)
	}

	if err := WaitForHealthy(ctx, log, newPort, cfg); err != nil {
		if stopErr := docker.StopAndRemove(ctx, cli, newContainerID); stopErr != nil {
			log.Warn("cleanup of unhealthy recreated container failed", "docker_container_id", ShortID(newContainerID), "error", stopErr)
		}
		return RestartResult{}, ErrCode("restart_health_check_failed", err)
	}

	if err := store.UpdateContainerHostBinding(ctx, containerRec.ID, newContainerID, newPort, "RUNNING"); err != nil {
		if stopErr := docker.StopAndRemove(ctx, cli, newContainerID); stopErr != nil {
			log.Warn("cleanup after binding update failure", "docker_container_id", ShortID(newContainerID), "error", stopErr)
		}
		return RestartResult{}, ErrCode("restart_binding_update_failed", err)
	}

	shouldSyncCaddy := cfg.SyncCaddy
	if !shouldSyncCaddy {
		domains, err := store.ListDomainsByProject(ctx, project.ID)
		if err != nil {
			log.Warn("domain lookup after restart-recreate failed", "project_id", project.ID, "error", err)
		} else {
			shouldSyncCaddy = len(domains) > 0
		}
	}
	if shouldSyncCaddy {
		if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			log.Warn("caddy sync after restart-recreate failed", "project_id", project.ID, "error", err)
		}
	}

	return RestartResult{
		ProjectID:    project.ID,
		DeploymentID: deployment.ID,
		ContainerID:  newContainerID,
		HostPort:     newPort,
		URL:          fmt.Sprintf("http://127.0.0.1:%d", newPort),
		Recreated:    true,
	}, nil
}

// ValidateRuntimeConfig checks deploy-time runtime options loaded from env/flags.
func ValidateRuntimeConfig(cfg *config.Config) error {
	if cfg.HostPort < -1 {
		return fmt.Errorf("host port must be -1, 0, or >0")
	}
	if cfg.ContainerPort <= 0 {
		return fmt.Errorf("container port must be > 0")
	}
	if cfg.HostPort == -1 {
		if cfg.PortStart <= 0 || cfg.PortEnd <= 0 || cfg.PortStart > cfg.PortEnd {
			return fmt.Errorf("invalid host port range %d..%d", cfg.PortStart, cfg.PortEnd)
		}
	}
	if cfg.HealthPath == "" {
		return fmt.Errorf("health path must not be empty")
	}
	if cfg.HealthTimeoutMS <= 0 {
		return fmt.Errorf("health timeout must be > 0")
	}
	if cfg.HealthRetries <= 0 {
		return fmt.Errorf("health retries must be > 0")
	}
	if cfg.HealthIntervalMS < 0 {
		return fmt.Errorf("health interval must be >= 0")
	}
	if cfg.HealthExpectedMin <= 0 || cfg.HealthExpectedMax <= 0 || cfg.HealthExpectedMin > cfg.HealthExpectedMax {
		return fmt.Errorf("invalid health expected status range %d..%d", cfg.HealthExpectedMin, cfg.HealthExpectedMax)
	}
	return nil
}

// WaitForHealthy polls localhost until the candidate container is ready.
func WaitForHealthy(ctx context.Context, log *slog.Logger, hostPort int, cfg *config.Config) error {
	probeStart := time.Now()
	path := cfg.HealthPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target := fmt.Sprintf("http://127.0.0.1:%d%s", hostPort, path)
	client := &http.Client{Timeout: time.Duration(cfg.HealthTimeoutMS) * time.Millisecond}
	var lastErr error
	if log != nil {
		log.Info("health_check start", "host_port", hostPort, "health_path", path, "retries", cfg.HealthRetries)
	}
	for attempt := 1; attempt <= cfg.HealthRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return fmt.Errorf("build health request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= cfg.HealthExpectedMin && resp.StatusCode <= cfg.HealthExpectedMax {
				if log != nil {
					log.Info("health_check end", "host_port", hostPort, "status", "ok", "attempt", attempt, "http_status", resp.StatusCode, "duration_ms", time.Since(probeStart).Milliseconds())
				}
				return nil
			}
			lastErr = fmt.Errorf("unexpected status code %d (expected %d..%d)", resp.StatusCode, cfg.HealthExpectedMin, cfg.HealthExpectedMax)
		} else {
			lastErr = err
		}
		if attempt == cfg.HealthRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(cfg.HealthIntervalMS) * time.Millisecond):
		}
	}
	if log != nil {
		log.Info("health_check end", "host_port", hostPort, "status", "failed", "attempts", cfg.HealthRetries, "duration_ms", time.Since(probeStart).Milliseconds())
	}
	return fmt.Errorf("probe %s failed after %d attempts: %w", target, cfg.HealthRetries, lastErr)
}

// DeleteProject stops and removes Docker containers tied to the project's deployments,
// deletes all related database rows (containers, deployments, domains, project),
// and syncs Caddy when the project had domains or when SyncCaddy is enabled.
func DeleteProject(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, projectID string) error {
	if _, err := store.GetProjectByID(ctx, projectID); err != nil {
		return fmt.Errorf("project lookup: %w", err)
	}

	domains, err := store.ListDomainsByProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	deployments, err := store.ListDeploymentsByProjectID(ctx, projectID, 500)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	cli, err := docker.NewClient(ctx)
	if err != nil {
		log.Warn("docker unavailable during project delete; database rows will still be removed", "project_id", projectID, "error", err)
	} else {
		defer cli.Close()
		removed := map[string]struct{}{}
		for _, dep := range deployments {
			c, err := store.GetContainerByDeploymentID(ctx, dep.ID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				log.Warn("container lookup failed", "deployment_id", dep.ID, "error", err)
				continue
			}
			did := strings.TrimSpace(c.DockerContainerID)
			if did == "" {
				continue
			}
			if _, ok := removed[did]; ok {
				continue
			}
			if err := docker.StopAndRemove(ctx, cli, did); err != nil {
				log.Warn("docker stop/remove failed", "docker_id", did, "error", err)
			} else {
				removed[did] = struct{}{}
			}
		}
	}

	if err := store.DeleteProjectCascade(ctx, projectID); err != nil {
		return fmt.Errorf("delete project from db: %w", err)
	}

	if cfg.SyncCaddy || len(domains) > 0 {
		if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			log.Warn("caddy sync after project delete failed", "project_id", projectID, "error", err)
		}
	}

	return nil
}

// CanonicalRepoURL normalizes repository URLs for consistent project matching.
func CanonicalRepoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http(s) URLs are supported (got scheme %q)", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("missing host")
	}
	u.Host = strings.ToLower(u.Host)
	cleanPath := strings.TrimSuffix(strings.TrimSpace(u.Path), "/")
	cleanPath = strings.TrimSuffix(cleanPath, ".git")
	if cleanPath == "" {
		cleanPath = "/"
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	u.Path = cleanPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String(), nil
}

// ShortID returns a human-readable prefix for container IDs.
func ShortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
