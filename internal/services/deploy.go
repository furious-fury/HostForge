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
	"github.com/hostforge/hostforge/internal/repository"
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

// ExecuteDeploy runs clone/build/run/health/cutover for a prepared deployment.
func ExecuteDeploy(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, job DeployJob) (DeployResult, error) {
	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentBuilding, ""); err != nil {
		return DeployResult{}, fmt.Errorf("deployment state: %w", err)
	}

	markFailed := func(stepErr error) {
		if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentFailed, stepErr.Error()); err != nil {
			log.Warn("failed to mark deployment failed", "deployment_id", job.Deployment.ID, "error", err)
		}
	}

	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("docker: %w", err)
	}
	defer dockerClient.Close()

	if err := os.MkdirAll(filepath.Dir(job.LogsPath), 0o755); err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("mkdir logs dir: %w", err)
	}
	logFile, err := os.OpenFile(job.LogsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("open deployment log: %w", err)
	}
	defer logFile.Close()
	combinedOut := io.MultiWriter(os.Stdout, logFile)

	log.Info("cloning", "url", job.RepoURL, "worktree", job.Worktree)
	_, _ = fmt.Fprintf(combinedOut, "hostforge: cloning url=%s worktree=%s\n", job.RepoURL, job.Worktree)
	if err := git.CloneOrUpdate(ctx, job.RepoURL, job.Branch, job.Worktree); err != nil {
		markFailed(err)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: clone failed: %v\n", err)
		return DeployResult{}, fmt.Errorf("clone: %w", err)
	}

	hostPortValue, err := docker.PickHostPort(cfg.HostPort, cfg.PortStart, cfg.PortEnd)
	if err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("host port selection: %w", err)
	}

	log.Info("running nixpacks image build", "dir", job.Worktree, "image", job.ImageRef)
	if err := nixpacks.BuildImageWithWriters(ctx, job.Worktree, job.ImageRef, combinedOut, combinedOut); err != nil {
		markFailed(err)
		_, _ = fmt.Fprintf(combinedOut, "hostforge: nixpacks failed: %v\n", err)
		return DeployResult{}, fmt.Errorf("nixpacks: %w", err)
	}

	containerID, err := docker.RunContainer(ctx, dockerClient, docker.RunOptions{
		ImageRef:      job.ImageRef,
		ContainerName: job.ContainerName,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
	})
	if err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("run container: %w", err)
	}

	candidateContainer, err := store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID:      job.Deployment.ID,
		DockerContainerID: containerID,
		InternalPort:      cfg.ContainerPort,
		HostPort:          hostPortValue,
		Status:            "RUNNING",
	})
	if err != nil {
		markFailed(err)
		return DeployResult{}, fmt.Errorf("container state: %w", err)
	}

	cleanupCandidate := func(reason string) {
		if stopErr := docker.StopAndRemove(ctx, dockerClient, containerID); stopErr != nil {
			log.Warn("failed to remove candidate container", "reason", reason, "container_id", ShortID(containerID), "error", stopErr)
			return
		}
		if err := store.UpdateContainerStatus(ctx, candidateContainer.ID, "REMOVED"); err != nil {
			log.Warn("failed to mark candidate container removed", "container_id", candidateContainer.ID, "error", err)
		}
	}

	if err := WaitForHealthy(ctx, hostPortValue, cfg); err != nil {
		markFailed(err)
		cleanupCandidate("health check failure")
		return DeployResult{}, fmt.Errorf("health check: %w", err)
	}

	shouldSyncCaddy := cfg.SyncCaddy
	if !shouldSyncCaddy {
		projectDomains, err := store.ListDomainsByProject(ctx, job.Project.ID)
		if err != nil {
			markFailed(err)
			cleanupCandidate("domain lookup failure")
			return DeployResult{}, fmt.Errorf("load project domains: %w", err)
		}
		shouldSyncCaddy = len(projectDomains) > 0
	}
	if shouldSyncCaddy {
		if err := SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
			markFailed(err)
			cleanupCandidate("caddy sync failure")
			return DeployResult{}, fmt.Errorf("caddy sync: %w", err)
		}
	}

	if err := store.UpdateDeploymentStatus(ctx, job.Deployment.ID, models.DeploymentSuccess, ""); err != nil {
		log.Warn("failed to mark deployment success", "deployment_id", job.Deployment.ID, "error", err)
	}

	if job.PreviousContainer.DockerContainerID != "" && job.PreviousContainer.DockerContainerID != containerID {
		if err := docker.StopAndRemove(ctx, dockerClient, job.PreviousContainer.DockerContainerID); err != nil {
			log.Warn("old container teardown failed", "container_id", ShortID(job.PreviousContainer.DockerContainerID), "error", err)
		} else if err := store.UpdateContainerStatus(ctx, job.PreviousContainer.ID, "REMOVED"); err != nil {
			log.Warn("failed to mark previous container removed", "container_id", job.PreviousContainer.ID, "error", err)
		}
	}

	result := DeployResult{
		DeploymentID:  job.Deployment.ID,
		ContainerID:   containerID,
		ImageRef:      job.ImageRef,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
		URL:           fmt.Sprintf("http://127.0.0.1:%d", hostPortValue),
	}
	log.Info("deploy finished", "deployment_id", result.DeploymentID, "image", result.ImageRef, "container_id", ShortID(result.ContainerID), "url", result.URL)
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
	domainRoutes, err := store.ListDomainRoutes(ctx)
	if err != nil {
		return fmt.Errorf("load domain routes: %w", err)
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
	if _, err := caddy.Sync(ctx, caddy.SyncOptions{
		CaddyBin:      cfg.CaddyBin,
		GeneratedPath: cfg.CaddyGeneratedPath,
		RootConfig:    cfg.CaddyRootConfig,
		Routes:        routes,
	}); err != nil {
		for _, domainID := range activeDomainIDs {
			if updateErr := store.UpdateDomainSSLStatus(ctx, domainID, models.SSLStatusError); updateErr != nil {
				log.Warn("failed to update domain ssl status", "domain_id", domainID, "status", models.SSLStatusError, "error", updateErr)
			}
		}
		return err
	}
	for _, domainID := range activeDomainIDs {
		if err := store.UpdateDomainSSLStatus(ctx, domainID, models.SSLStatusActive); err != nil {
			log.Warn("failed to update domain ssl status", "domain_id", domainID, "status", models.SSLStatusActive, "error", err)
		}
	}
	log.Info("caddy sync complete", "generated_path", cfg.CaddyGeneratedPath, "root_config", cfg.CaddyRootConfig, "routes", len(routes))
	return nil
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
func WaitForHealthy(ctx context.Context, hostPort int, cfg *config.Config) error {
	path := cfg.HealthPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target := fmt.Sprintf("http://127.0.0.1:%d%s", hostPort, path)
	client := &http.Client{Timeout: time.Duration(cfg.HealthTimeoutMS) * time.Millisecond}
	var lastErr error
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
	return fmt.Errorf("probe %s failed after %d attempts: %w", target, cfg.HealthRetries, lastErr)
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
	return u.String(), nil
}

// ShortID returns a human-readable prefix for container IDs.
func ShortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
