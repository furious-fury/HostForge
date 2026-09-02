package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/builder"
	"github.com/furious-fury/HostForge/internal/caddy"
	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/git"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/obs"
	"github.com/furious-fury/HostForge/internal/railpack"
	"github.com/furious-fury/HostForge/internal/redact"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/furious-fury/HostForge/internal/reqctx"
	mobyclient "github.com/moby/moby/client"
)

// DeployJob contains persisted and computed data for a deployment execution.
type DeployJob struct {
	Target            *DeployTarget
	Deployment        models.Deployment
	PreviousContainer models.Container
	RepoURL           string
	Branch            string
	Worktree          string
	BuildDirectory    string
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

// GitAuthResolver returns credentials for a GitHub App installation.
type GitAuthResolver interface {
	ResolveInstallationAuth(context.Context, int64) (git.AuthOptions, error)
}

func recordDeployObs(ctx context.Context, log *slog.Logger, job DeployJob, step, status string, started time.Time, durMs int64, errCode string) {
	obs.RecordDeployStep(ctx, log, models.DeployStepRecord{
		DeploymentID:  job.Deployment.ID,
		ServiceID:     job.serviceID(),
		EnvironmentID: job.environmentID(),
		RequestID:     reqctx.RequestID(ctx),
		Step:          step,
		Status:        status,
		DurationMS:    durMs,
		ErrorCode:     errCode,
		StartedAt:     started,
		EndedAt:       time.Now().UTC(),
	})
}

// stepBoundary reports whether ctx has already been cancelled. If so it
// tears down cleanup (a candidate container, when one already exists),
// resolves the deployment through markFailed -- which already routes
// ctx.Err() != nil to CancelDeployment rather than a plain failure -- records
// the cancellation on a context detached from ctx (ctx is cancelled here by
// definition, and a write scoped to it would fail before ever reaching the
// database), and returns an error that unwraps to ctx.Err().
//
// Call this before starting any phase a cancel should be able to stop.
// CheckoutCommit and HeadCommit take no context argument at all, so this is
// the only thing that can stop them; even where the next call does respect
// ctx, a cancel landing in the gap between two steps would otherwise be
// silently absorbed by whichever step happens to run next. There are
// deliberately no boundaries once cutover begins (ActivateServiceDeployment
// onward) -- a half-applied cutover that leaves the previous container torn
// down and the new one unrouted is worse than a slow one.
func (job DeployJob) stepBoundary(ctx context.Context, log *slog.Logger, markFailed func(error), cleanup func(string), step string, started time.Time) error {
	if ctx.Err() == nil {
		return nil
	}
	if cleanup != nil {
		cleanup("cancelled before " + step)
	}
	e := ErrCode("deploy_cancelled", fmt.Errorf("deploy cancelled before %s: %w", step, ctx.Err()))
	markFailed(e)
	recordDeployObs(context.WithoutCancel(ctx), log, job, step, "cancelled", started, time.Since(started).Milliseconds(), "deploy_cancelled")
	return e
}

// cleanupCandidateContainer stops and removes a deploy's candidate container
// and marks its row REMOVED. Runs on a context detached from ctx, with its
// own timeout: ctx is frequently already cancelled by the time this is
// called -- that is exactly when a cancelled deploy needs it to run -- and
// StopAndRemove or the status write would otherwise fail before ever
// reaching the daemon or the database, leaving a running container with a
// RUNNING row that nothing ever cleans up.
func cleanupCandidateContainer(ctx context.Context, log *slog.Logger, dockerClient *mobyclient.Client, store *repository.Store, containerID, containerRowID, reason string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if stopErr := docker.StopAndRemove(cleanupCtx, dockerClient, containerID); stopErr != nil {
		log.Warn("failed to remove candidate container", "reason", reason, "docker_container_id", ShortID(containerID), "error", stopErr)
		return
	}
	if err := store.UpdateContainerStatus(cleanupCtx, containerRowID, "REMOVED"); err != nil {
		log.Warn("failed to mark candidate container removed", "container_row_id", containerRowID, "error", err)
	}
}

// ExecuteDeploy runs clone/build/run/health/cutover for a prepared deployment.
// authResolver is required for private GitHub repositories.
func ExecuteDeploy(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, job DeployJob, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client, authResolver GitAuthResolver) (result DeployResult, err error) {
	deployStart := time.Now()
	log = log.With("service_id", job.serviceID(), "environment_id", job.environmentID(), "deployment_id", job.Deployment.ID)
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
		obs.RecordDeployStep(ctx, log, models.DeployStepRecord{
			DeploymentID:  job.Deployment.ID,
			ServiceID:     job.serviceID(),
			EnvironmentID: job.environmentID(),
			RequestID:     reqctx.RequestID(ctx),
			Step:          "deploy_total",
			Status:        status,
			DurationMS:    dur,
			ErrorCode:     code,
			StartedAt:     deployStart.UTC(),
			EndedAt:       time.Now().UTC(),
		})
	}()
	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentBuilding, ""); err != nil {
		return DeployResult{}, ErrCode("deployment_state_update_failed", fmt.Errorf("deployment state: %w", err))
	}

	markFailed := func(stepErr error) {
		if ctx.Err() != nil {
			_, _ = store.CancelDeployment(context.Background(), job.Deployment.ID)
			return
		}
		code := FirstPublicCode(stepErr)
		if code == "internal_error" {
			code = "deploy_failed"
		}
		if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentFailed, code); err != nil {
			log.Warn("failed to mark deployment failed", "deployment_id", job.Deployment.ID, "error", err)
		}
		log.Error("deploy failed", "deployment_id", job.Deployment.ID, "public_code", code, "error", stepErr)
	}

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

	if err := job.stepBoundary(ctx, log, markFailed, nil, "clone", time.Now()); err != nil {
		return DeployResult{}, err
	}

	t0 := time.Now()
	log.Info("deploy step", "step", "clone_start", "repo_url", redact.RepoURLForLog(job.RepoURL), "worktree", job.Worktree)
	_, _ = fmt.Fprintf(combinedOut, "hostforge: cloning url=%s worktree=%s\n", redact.RepoURLForLog(job.RepoURL), job.Worktree)
	gitAuth, err := resolveDeployGitAuth(ctx, job, authResolver)
	if err != nil {
		e := ErrCode("git_auth_prepare_failed", err)
		markFailed(e)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: git auth setup failed: %v\n", err)
		return DeployResult{}, e
	}
	if err := git.CloneOrUpdate(ctx, job.RepoURL, job.Branch, job.Worktree, gitAuth); err != nil {
		e := ErrCode("clone_failed", err)
		markFailed(e)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: clone failed: %v\n", err)
		ms := time.Since(t0).Milliseconds()
		log.Info("deploy step", "step", "clone_end", "status", "failed", "duration_ms", ms)
		recordDeployObs(ctx, log, job, "clone", "failed", t0, ms, FirstPublicCode(e))
		return DeployResult{}, e
	}
	if err := job.stepBoundary(ctx, log, markFailed, nil, "checkout", time.Now()); err != nil {
		return DeployResult{}, err
	}

	if requestedCommit := strings.TrimSpace(job.Deployment.CommitHash); requestedCommit != "" {
		if err := git.CheckoutCommit(job.Worktree, requestedCommit); err != nil {
			e := ErrCode("commit_checkout_failed", err)
			markFailed(e)
			_, _ = fmt.Fprintf(combinedOut, "hostforge: commit checkout failed: %v\n", err)
			return DeployResult{}, e
		}
	}
	checkedOutCommit, err := git.HeadCommit(job.Worktree)
	if err != nil {
		e := ErrCode("commit_lookup_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}
	if err := store.UpdateDeploymentCommitHash(ctx, job.Deployment.ID, checkedOutCommit); err != nil {
		e := ErrCode("deployment_commit_update_failed", err)
		markFailed(e)
		return DeployResult{}, e
	}
	job.Deployment.CommitHash = checkedOutCommit
	msClone := time.Since(t0).Milliseconds()
	log.Info("deploy step", "step", "clone_end", "status", "ok", "duration_ms", msClone)
	recordDeployObs(ctx, log, job, "clone", "ok", t0, msClone, "")

	_, _ = fmt.Fprint(combinedOut, "\nhostforge: Railpack/BuildKit builder selected.\n\n")
	log.Info("deploy step", "step", "railpack_prepare", "status", "selected")

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

	extraEnv, err := buildDockerEnvForJob(ctx, log, store, job, sealer)
	if err != nil {
		markFailed(err)
		return DeployResult{}, err
	}

	if err := job.stepBoundary(ctx, log, markFailed, nil, "railpack_build", time.Now()); err != nil {
		return DeployResult{}, err
	}

	t1 := time.Now()
	buildStep := "railpack_build"
	detectedStackKind := ""
	{
		log.Info("deploy step", "step", "railpack_build_start", "dir", job.BuildDirectory, "image", job.ImageRef)
		railpackBuilder, err := newRailpackAdapter(cfg, dockerClient)
		var dockerfileBuilder builder.Builder
		if err == nil {
			dockerfileBuilder, err = newDockerfileBuilder(cfg, dockerClient)
		}
		var buildResult builder.Result
		if err == nil {
			buildResult, err = (builder.Selection{Railpack: railpackBuilder, Dockerfile: dockerfileBuilder}).Build(ctx, builder.Request{
				Worktree:     job.BuildDirectory,
				ImageRef:     job.ImageRef,
				Platform:     runtime.GOOS + "/" + runtime.GOARCH,
				CacheKey:     job.serviceID(),
				Runtime:      job.Target.Service.DeployRuntime,
				InstallCmd:   job.Target.Service.InstallCmd,
				BuildCmd:     job.Target.Service.BuildCmd,
				StartCmd:     job.Target.Service.StartCmd,
				BuildSecrets: dockerEnvMap(extraEnv),
			}, railpackLogSink(combinedOut))
		}
		if err != nil {
			e := ErrCode("railpack_build_failed", err)
			markFailed(e)
			_, _ = fmt.Fprintf(combinedOut, "hostforge: ===== RAILPACK BUILDKIT IMAGE BUILD FAILED =====\nhostforge: railpack failed: %v\n", err)
			ms := time.Since(t1).Milliseconds()
			log.Info("deploy step", "step", "railpack_build_end", "status", "failed", "duration_ms", ms)
			recordDeployObs(ctx, log, job, buildStep, "failed", t1, ms, FirstPublicCode(e))
			return DeployResult{}, e
		}
		if err := store.UpdateDeploymentBuilder(ctx, job.Deployment.ID, string(buildResult.Kind), buildResult.StackKind, buildResult.StackLabel); err != nil {
			log.Warn("deploy step", "step", "deployment_stack_persist", "status", "failed", "error", err)
		} else {
			log.Info("deploy step", "step", "deployment_stack_persist", "status", "ok", "stack_kind", buildResult.StackKind, "stack_label", buildResult.StackLabel, "builder", buildResult.Kind)
			_, _ = fmt.Fprintf(combinedOut, "hostforge: detected stack kind=%q label=%q builder=%q\n", buildResult.StackKind, buildResult.StackLabel, buildResult.Kind)
		}
		// Provenance only, never fatal to the deploy: a Dockerfile build has
		// nothing to report, and an oversize plan/info file (capped in
		// internal/railpack) already comes back as an empty string, not an
		// error. The size log line is how the real distribution on this
		// operator's repos gets learned — it can't be measured in advance.
		if buildResult.PlanJSON != "" || buildResult.InfoJSON != "" {
			if err := store.UpdateDeploymentRailpackArtifacts(ctx, job.Deployment.ID, buildResult.PlanJSON, buildResult.InfoJSON); err != nil {
				log.Warn("deploy step", "step", "deployment_railpack_artifacts_persist", "status", "failed", "error", err)
			} else {
				log.Info("deploy step", "step", "deployment_railpack_artifacts_persist", "status", "ok", "plan_bytes", len(buildResult.PlanJSON), "info_bytes", len(buildResult.InfoJSON))
			}
		}
		detectedStackKind = buildResult.StackKind
		_, _ = fmt.Fprintf(combinedOut, "\nhostforge: ===== RAILPACK BUILDKIT IMAGE BUILD SUCCEEDED image=%s =====\n\n", job.ImageRef)
		msRailpack := time.Since(t1).Milliseconds()
		log.Info("deploy step", "step", "railpack_build_end", "status", "ok", "duration_ms", msRailpack)
		recordDeployObs(ctx, log, job, buildStep, "ok", t1, msRailpack, "")
	}

	if err := job.stepBoundary(ctx, log, markFailed, nil, "container_start", time.Now()); err != nil {
		return DeployResult{}, err
	}

	tRun := time.Now()
	environmentNetwork := docker.EnvironmentNetworkName(job.environmentID())
	if _, err := docker.EnsureEnvironmentNetwork(ctx, dockerClient, job.Target.Application.ID, job.environmentID()); err != nil {
		e := ErrCode("environment_network_failed", err)
		markFailed(e)
		recordDeployObs(ctx, log, job, "container_start", "failed", tRun, time.Since(tRun).Milliseconds(), FirstPublicCode(e))
		return DeployResult{}, e
	}
	containerID, err := docker.RunContainer(ctx, dockerClient, docker.RunOptions{
		ImageRef:      job.ImageRef,
		ContainerName: job.ContainerName,
		ContainerPort: job.internalPort(cfg),
		HostPort:      hostPortValue,
		NetworkName:   environmentNetwork,
		Labels: map[string]string{
			docker.ManagedLabel:       "true",
			docker.ResourceTypeLabel:  "application-container",
			docker.ApplicationIDLabel: job.Target.Application.ID,
			docker.EnvironmentIDLabel: job.environmentID(),
			docker.ServiceIDLabel:     job.serviceID(),
		},
		Env:              extraEnv,
		MemoryLimitBytes: cfg.AppContainerMemoryLimitBytes,
		CPULimitMillis:   cfg.AppContainerCPULimitMillis,
		PidsLimit:        cfg.AppContainerPidsLimit,
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
		InternalPort:      job.internalPort(cfg),
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
		cleanupCandidateContainer(ctx, log, dockerClient, store, containerID, candidateContainer.ID, reason)
	}

	if err := job.stepBoundary(ctx, log, markFailed, cleanupCandidate, "health_check", time.Now()); err != nil {
		return DeployResult{}, err
	}

	t2 := time.Now()
	if err := WaitForHealthy(ctx, log, hostPortValue, job.healthConfig(cfg, detectedStackKind)); err != nil {
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

	if err := job.stepBoundary(ctx, log, markFailed, cleanupCandidate, "platform_domain", time.Now()); err != nil {
		return DeployResult{}, err
	}

	platformDomain, platformDomainCreated, err := store.EnsurePlatformServiceDomain(ctx, job.Target.Application.ID, job.Target.Environment.ID, job.Target.Service.ID)
	if err != nil {
		e := ErrCode("platform_domain_provision_failed", err)
		markFailed(e)
		cleanupCandidate("platform domain provisioning failure")
		return DeployResult{}, e
	}
	if platformDomainCreated {
		log.Info("generated platform domain", "domain", platformDomain.DomainName, "environment_id", job.Target.Environment.ID)
		_ = store.RecordPlatformEvent(ctx, repository.PlatformEventInput{
			ApplicationID: job.Target.Application.ID,
			ServiceID:     job.Target.Service.ID,
			EnvironmentID: job.Target.Environment.ID,
			DeploymentID:  job.Deployment.ID,
			EventType:     "domain",
			Status:        "created",
			Actor:         "hostforge",
			Message:       "Platform domain generated",
			Detail:        platformDomain.DomainName,
		})
	}

	// This is the last point a cancel can still stop the deploy: once
	// deployments flips to SUCCESS, cutover begins and there are no more
	// boundaries. Without this check a cancel arriving after the health
	// check but before this write was silently overwritten by SUCCESS —
	// CancelDeployment's guard only matches a non-terminal row, so cancel
	// requests processed in this window used to have no effect at all.
	if err := job.stepBoundary(ctx, log, markFailed, cleanupCandidate, "cutover", time.Now()); err != nil {
		return DeployResult{}, err
	}

	// Caddy resolves each domain to the latest SUCCESS deployment. Promote the
	// healthy candidate before rendering routes so the generated upstream is the
	// new container, not the one that is about to be removed after cutover.
	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentSuccess, ""); err != nil {
		e := ErrCode("deployment_state_update_failed", fmt.Errorf("mark deployment success: %w", err))
		markFailed(e)
		cleanupCandidate("deployment success state failure")
		return DeployResult{}, e
	}

	previousActiveDeploymentID := job.Target.Binding.ActiveDeploymentID
	if err := store.ActivateServiceDeployment(ctx, job.Target.Service.ID, job.Target.Environment.ID, job.Deployment.ID); err != nil {
		e := ErrCode("active_deployment_update_failed", err)
		markFailed(e)
		cleanupCandidate("active deployment update failure")
		return DeployResult{}, e
	}

	shouldSyncCaddy := cfg.SyncCaddy
	if !shouldSyncCaddy {
		serviceDomains, err := store.ListServiceDomains(ctx, job.Target.Application.ID, job.Target.Environment.ID, job.Target.Service.ID)
		if err != nil {
			_ = store.ActivateServiceDeployment(ctx, job.Target.Service.ID, job.Target.Environment.ID, previousActiveDeploymentID)
			e := ErrCode("domain_lookup_failed", err)
			markFailed(e)
			cleanupCandidate("domain lookup failure")
			return DeployResult{}, e
		}
		shouldSyncCaddy = len(serviceDomains) > 0
	}
	if shouldSyncCaddy {
		t3 := time.Now()
		if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			if restoreErr := store.ActivateServiceDeployment(ctx, job.Target.Service.ID, job.Target.Environment.ID, previousActiveDeploymentID); restoreErr != nil {
				log.Error("failed to restore previous active deployment", "error", restoreErr)
			}
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
		ContainerPort: job.internalPort(cfg),
		HostPort:      hostPortValue,
		URL:           fmt.Sprintf("http://127.0.0.1:%d", hostPortValue),
	}
	log.Info("deploy finished", "deployment_id", result.DeploymentID, "image", result.ImageRef, "docker_container_id", ShortID(result.ContainerID), "url", result.URL, "duration_ms_total", time.Since(deployStart).Milliseconds())
	return result, nil
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
	var certificateDomains []string
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
	if endpoint, endpointErr := store.GetDatabaseGatewayEndpoint(ctx, "postgresql"); endpointErr == nil && endpoint.DesiredStatus == "active" {
		certificateDomains = append(certificateDomains, endpoint.Hostname)
	}
	syncRes, err := caddy.Sync(ctx, caddy.SyncOptions{
		CaddyBin:           cfg.CaddyBin,
		GeneratedPath:      cfg.CaddyGeneratedPath,
		RootConfig:         cfg.CaddyRootConfig,
		Routes:             routes,
		CertificateDomains: certificateDomains,
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
	if err := ValidateRailpackConfig(cfg); err != nil {
		return err
	}
	if cfg.DatabaseMinFreeDiskBytes <= 0 {
		return fmt.Errorf("database minimum free disk bytes must be > 0")
	}
	return nil
}

func newRailpackAdapter(cfg *config.Config, dockerClient *mobyclient.Client) (*railpack.Adapter, error) {
	planner, err := railpack.NewPlanner(cfg.RailpackBin, cfg.RailpackVersion)
	if err != nil {
		return nil, err
	}
	executor, err := railpack.NewBuildKitExecutor(railpack.BuildKitConfig{
		Binary:          cfg.BuildKitBin,
		Address:         cfg.BuildKitAddress,
		FrontendImage:   cfg.RailpackFrontendImage,
		RailpackVersion: cfg.RailpackVersion,
	}, dockerClient)
	if err != nil {
		return nil, err
	}
	return railpack.NewAdapter(railpack.AdapterConfig{
		Planner:       planner,
		Executor:      executor,
		ArtifactsRoot: cfg.RailpackArtifactsDir,
	})
}

func newDockerfileBuilder(cfg *config.Config, dockerClient *mobyclient.Client) (builder.Builder, error) {
	executor, err := railpack.NewBuildKitExecutor(railpack.BuildKitConfig{
		Binary:          cfg.BuildKitBin,
		Address:         cfg.BuildKitAddress,
		FrontendImage:   cfg.RailpackFrontendImage,
		RailpackVersion: cfg.RailpackVersion,
	}, dockerClient)
	if err != nil {
		return nil, err
	}
	return railpack.NewDockerfileBuilder(executor)
}

func railpackLogSink(out io.Writer) builder.EventSink {
	return func(event builder.Event) {
		if out == nil {
			return
		}
		_, _ = fmt.Fprintf(out, "hostforge: railpack %s: %s\n", event.Phase, event.Message)
	}
}

// ValidateRailpackConfig validates the required Railpack/BuildKit
// configuration without touching host services.
func ValidateRailpackConfig(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("railpack configuration is required")
	}
	if !cfg.RailpackEnabled {
		return fmt.Errorf("railpack is required by the active deployment path")
	}
	if strings.TrimSpace(cfg.RailpackBin) == "" || strings.TrimSpace(cfg.RailpackVersion) == "" {
		return fmt.Errorf("railpack is enabled but helper binary and version are required")
	}
	if !strings.Contains(strings.TrimSpace(cfg.RailpackFrontendImage), "@sha256:") {
		return fmt.Errorf("railpack is enabled but frontend image must be digest-pinned")
	}
	if strings.TrimSpace(cfg.BuildKitBin) == "" || strings.TrimSpace(cfg.BuildKitAddress) == "" {
		return fmt.Errorf("railpack is enabled but BuildKit binary and address are required")
	}
	if strings.TrimSpace(cfg.RailpackArtifactsDir) == "" {
		return fmt.Errorf("railpack is enabled but artifacts directory is required")
	}
	if cfg.RailpackBuildConcurrency <= 0 {
		return fmt.Errorf("railpack build concurrency must be > 0")
	}
	if cfg.RailpackMinFreeDiskBytes <= 0 {
		return fmt.Errorf("railpack minimum free disk bytes must be > 0")
	}
	return nil
}

// ValidateRailpackReadiness verifies the required local Railpack dependencies.
func ValidateRailpackReadiness(ctx context.Context, cfg *config.Config) error {
	if err := ValidateRailpackConfig(cfg); err != nil {
		return err
	}
	for _, binary := range []string{cfg.RailpackBin, cfg.BuildKitBin} {
		if _, err := exec.LookPath(binary); err != nil {
			return fmt.Errorf("railpack readiness: executable %q: %w", binary, err)
		}
	}
	if err := os.MkdirAll(cfg.RailpackArtifactsDir, 0o700); err != nil {
		return fmt.Errorf("railpack readiness: create artifacts directory: %w", err)
	}
	if info, err := os.Stat(cfg.RailpackArtifactsDir); err != nil || !info.IsDir() {
		return fmt.Errorf("railpack readiness: artifacts directory unavailable")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cli, err := docker.NewClient(checkCtx)
	if err != nil {
		return fmt.Errorf("railpack readiness: docker unavailable: %w", err)
	}
	defer cli.Close()
	var output bytes.Buffer
	cmd := exec.CommandContext(checkCtx, cfg.BuildKitBin, "--addr", cfg.BuildKitAddress, "debug", "workers")
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("railpack readiness: BuildKit unavailable: %w", err)
	}
	if strings.TrimSpace(output.String()) == "" {
		return fmt.Errorf("railpack readiness: BuildKit returned no workers")
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
