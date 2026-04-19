// Package main implements the hostforge command-line interface: deploy (clone, build, run)
// and version. Deploy persists control-plane state to SQLite under the configured data directory.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/caddy"
	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/logging"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/nixpacks"
	"github.com/hostforge/hostforge/internal/repository"
)

func main() {
	log := logging.New()
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "deploy":
		code := runDeploy(log, os.Args[2:])
		os.Exit(code)
	case "domain":
		code := runDomain(log, os.Args[2:])
		os.Exit(code)
	case "caddy":
		code := runCaddy(log, os.Args[2:])
		os.Exit(code)
	case "version":
		fmt.Println("hostforge dev")
		os.Exit(0)
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s

Usage:
  hostforge deploy [flags] <repo_url>
  hostforge domain add [flags] --domain <host> <repo_url>
  hostforge caddy sync [flags]
  hostforge version

deploy clones the repository (HTTPS), runs nixpacks build in the worktree, and streams build logs to stdout/stderr.

Flags for deploy:
  -data-dir string
    	data directory (overrides %s)
  -branch string
    	git branch (default: remote default)
  -host-port int
    	host port mapping: -1 range mode, 0 ephemeral, >0 exact (default from %s)
  -port-start int
    	range start when host-port=-1 (default from %s)
  -port-end int
    	range end when host-port=-1 (default from %s)
  -container-port int
    	app port inside container (default from %s)
  -health-path string
		HTTP path probed before cutover (default from %s)
  -health-timeout-ms int
		per-request health timeout in milliseconds (default from %s)
  -health-retries int
		number of health probe attempts before deploy fails (default from %s)
  -health-interval-ms int
		delay between health probes in milliseconds (default from %s)
  -health-expected-min int
		minimum accepted health status code (default from %s)
  -health-expected-max int
		maximum accepted health status code (default from %s)
  -sync-caddy
		run caddy sync after successful deploy (default from %s)

`, os.Args[0], config.DataDirEnv, config.HostPortEnv, config.PortStartEnv, config.PortEndEnv, config.ContainerPortEnv, config.HealthPathEnv, config.HealthTimeoutMSEnv, config.HealthRetriesEnv, config.HealthIntervalMSEnv, config.HealthExpectedMinEnv, config.HealthExpectedMaxEnv, config.SyncCaddyEnv)
}

// runDeploy clones repoURL, builds a Docker image with Nixpacks, runs a container, and records
// project/deployment/container rows in SQLite. Status flow: QUEUED → BUILDING → SUCCESS or FAILED
// (FAILED also records error_message). On failure after deployment creation, the deployment is
// marked FAILED; no container row is written unless the container started successfully.
func runDeploy(log *slog.Logger, args []string) int {
	defaultHostPort, defaultPortStart, defaultPortEnd, defaultContainerPort, err := config.RuntimeDefaults()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: runtime env defaults: %v\n", err)
		return 2
	}
	defaultHealthPath, defaultHealthTimeoutMS, defaultHealthRetries, defaultHealthIntervalMS, defaultHealthExpectedMin, defaultHealthExpectedMax, err := config.HealthDefaults()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: health env defaults: %v\n", err)
		return 2
	}

	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	branch := fs.String("branch", "", "git branch (default: remote default)")
	hostPort := fs.Int("host-port", defaultHostPort, "host port mapping: -1 range mode, 0 ephemeral, >0 exact")
	portStart := fs.Int("port-start", defaultPortStart, "range start when host-port=-1")
	portEnd := fs.Int("port-end", defaultPortEnd, "range end when host-port=-1")
	containerPort := fs.Int("container-port", defaultContainerPort, "app port inside container")
	healthPath := fs.String("health-path", defaultHealthPath, "HTTP path probed before cutover")
	healthTimeoutMS := fs.Int("health-timeout-ms", defaultHealthTimeoutMS, "per-request health timeout in milliseconds")
	healthRetries := fs.Int("health-retries", defaultHealthRetries, "number of health probe attempts before deploy fails")
	healthIntervalMS := fs.Int("health-interval-ms", defaultHealthIntervalMS, "delay between health probes in milliseconds")
	healthExpectedMin := fs.Int("health-expected-min", defaultHealthExpectedMin, "minimum accepted health status code")
	healthExpectedMax := fs.Int("health-expected-max", defaultHealthExpectedMax, "maximum accepted health status code")
	syncCaddy := fs.Bool("sync-caddy", cfgBoolDefault(config.SyncCaddyEnv, false), "run caddy sync after successful deploy")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintf(os.Stderr, "error: deploy requires exactly one <repo_url>\n\n")
		fs.SetOutput(os.Stderr)
		fs.PrintDefaults()
		return 2
	}
	repoURL := strings.TrimSpace(rest[0])
	if err := validateRepoURL(repoURL); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid repo URL: %v\n", err)
		return 2
	}

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	cfg.HostPort = *hostPort
	cfg.PortStart = *portStart
	cfg.PortEnd = *portEnd
	cfg.ContainerPort = *containerPort
	cfg.HealthPath = *healthPath
	cfg.HealthTimeoutMS = *healthTimeoutMS
	cfg.HealthRetries = *healthRetries
	cfg.HealthIntervalMS = *healthIntervalMS
	cfg.HealthExpectedMin = *healthExpectedMin
	cfg.HealthExpectedMax = *healthExpectedMax
	cfg.SyncCaddy = *syncCaddy
	if err := validateRuntimeConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: runtime config: %v\n", err)
		return 2
	}
	for _, d := range []string{cfg.DataDir, cfg.WorktreesDir(), cfg.BuildsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", d, err)
			return 1
		}
	}

	ctx := context.Background()
	// Control-plane DB: schema + migrations applied on open (see internal/database).
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		return 1
	}
	defer db.Close()
	store := repository.New(db)

	slug := git.WorktreeDir(repoURL, *branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildID := time.Now().UTC().Format("20060102t150405")
	imageRef := fmt.Sprintf("hostforge/%s:%s", slug, buildID)
	containerName := fmt.Sprintf("hostforge-%s-%s", slug[:12], buildID)
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: docker: %v\n", err)
		return 1
	}
	defer dockerClient.Close()

	project, err := store.EnsureProject(ctx, repoURL, strings.TrimSpace(*branch))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
		return 1
	}
	previousDeployment, err := store.GetLatestSuccessfulDeploymentByProjectID(ctx, project.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fmt.Fprintf(os.Stderr, "error: previous deployment state: %v\n", err)
		return 1
	}
	var previousContainer models.Container
	if err == nil {
		previousContainer, err = store.GetContainerByDeploymentID(ctx, previousDeployment.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			fmt.Fprintf(os.Stderr, "error: previous container state: %v\n", err)
			return 1
		}
	}
	deployment, err := store.CreateDeployment(ctx, repository.CreateDeploymentInput{
		ProjectID: project.ID,
		ImageRef:  imageRef,
		Worktree:  worktree,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: deployment state: %v\n", err)
		return 1
	}
	// Best-effort: keep DB aligned with stderr errors for operators and future UI/API.
	markFailed := func(stepErr error) {
		if err := store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentFailed, stepErr.Error()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to mark deployment FAILED: %v\n", err)
		}
	}
	if err := store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentBuilding, ""); err != nil {
		fmt.Fprintf(os.Stderr, "error: deployment state: %v\n", err)
		return 1
	}

	log.Info("cloning", "url", repoURL, "worktree", worktree)
	if err := git.CloneOrUpdate(ctx, repoURL, *branch, worktree); err != nil {
		markFailed(err)
		fmt.Fprintf(os.Stderr, "error: clone: %v\n", err)
		return 1
	}

	hostPortValue, err := docker.PickHostPort(cfg.HostPort, cfg.PortStart, cfg.PortEnd)
	if err != nil {
		markFailed(err)
		fmt.Fprintf(os.Stderr, "error: host port selection: %v\n", err)
		return 1
	}

	log.Info("running nixpacks image build", "dir", worktree, "image", imageRef)
	if err := nixpacks.BuildImage(ctx, worktree, imageRef); err != nil {
		markFailed(err)
		fmt.Fprintf(os.Stderr, "error: nixpacks: %v\n", err)
		return 1
	}
	containerID, err := docker.RunContainer(ctx, dockerClient, docker.RunOptions{
		ImageRef:      imageRef,
		ContainerName: containerName,
		ContainerPort: cfg.ContainerPort,
		HostPort:      hostPortValue,
	})
	if err != nil {
		markFailed(err)
		fmt.Fprintf(os.Stderr, "error: run container: %v\n", err)
		return 1
	}
	candidateContainer, err := store.AttachContainer(ctx, repository.AttachContainerInput{
		DeploymentID:      deployment.ID,
		DockerContainerID: containerID,
		InternalPort:      cfg.ContainerPort,
		HostPort:          hostPortValue,
		Status:            "RUNNING",
	})
	if err != nil {
		markFailed(err)
		fmt.Fprintf(os.Stderr, "error: container state: %v\n", err)
		return 1
	}
	cleanupCandidate := func(reason string) {
		if stopErr := docker.StopAndRemove(ctx, dockerClient, containerID); stopErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove candidate container after %s: %v\n", reason, stopErr)
			return
		}
		if err := store.UpdateContainerStatus(ctx, candidateContainer.ID, "REMOVED"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to mark candidate container removed: %v\n", err)
		}
	}

	if err := waitForHealthy(ctx, hostPortValue, cfg); err != nil {
		markFailed(err)
		cleanupCandidate("health check failure")
		fmt.Fprintf(os.Stderr, "error: health check: %v\n", err)
		return 1
	}

	shouldSyncCaddy := cfg.SyncCaddy
	if !shouldSyncCaddy {
		projectDomains, err := store.ListDomainsByProject(ctx, project.ID)
		if err != nil {
			markFailed(err)
			cleanupCandidate("domain lookup failure")
			fmt.Fprintf(os.Stderr, "error: load project domains: %v\n", err)
			return 1
		}
		shouldSyncCaddy = len(projectDomains) > 0
	}
	if shouldSyncCaddy {
		if err := syncCaddyRoutes(ctx, log, cfg, store); err != nil {
			markFailed(err)
			cleanupCandidate("caddy sync failure")
			fmt.Fprintf(os.Stderr, "error: caddy sync: %v\n", err)
			return 1
		}
	}

	if err := store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentSuccess, ""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to mark deployment SUCCESS: %v\n", err)
	}
	if previousContainer.DockerContainerID != "" && previousContainer.DockerContainerID != containerID {
		if err := docker.StopAndRemove(ctx, dockerClient, previousContainer.DockerContainerID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: old container teardown failed (%s): %v\n", shortID(previousContainer.DockerContainerID), err)
		} else if err := store.UpdateContainerStatus(ctx, previousContainer.ID, "REMOVED"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to mark previous container removed: %v\n", err)
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", hostPortValue)
	log.Info("deploy finished", "image", imageRef, "container_id", shortID(containerID), "url", url)
	fmt.Printf("container_id=%s\nimage=%s\ncontainer_port=%d\nhost_port=%d\nurl=%s\n",
		containerID, imageRef, cfg.ContainerPort, hostPortValue, url)
	return 0
}

func runDomain(log *slog.Logger, args []string) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, "error: domain requires a subcommand (supported: add)")
		return 2
	}
	switch strings.TrimSpace(args[0]) {
	case "add":
		return runDomainAdd(log, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported domain subcommand %q\n", args[0])
		return 2
	}
}

func runDomainAdd(log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("domain add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	branch := fs.String("branch", "", "git branch (default: remote default)")
	domainName := fs.String("domain", "", "domain to route to latest successful deployment")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "error: domain add requires exactly one <repo_url>")
		return 2
	}
	if strings.TrimSpace(*domainName) == "" {
		fmt.Fprintln(os.Stderr, "error: --domain is required")
		return 2
	}
	if err := validateRepoURL(rest[0]); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid repo URL: %v\n", err)
		return 2
	}

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", cfg.DataDir, err)
		return 1
	}
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		return 1
	}
	defer db.Close()
	store := repository.New(db)

	project, err := store.EnsureProject(ctx, strings.TrimSpace(rest[0]), strings.TrimSpace(*branch))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
		return 1
	}
	domainRec, err := store.CreateDomain(ctx, project.ID, strings.TrimSpace(*domainName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: domain state: %v\n", err)
		return 1
	}
	log.Info("domain added", "domain", domainRec.DomainName, "project_id", domainRec.ProjectID)
	fmt.Printf("domain_id=%s\ndomain=%s\nproject_id=%s\nssl_status=%s\n",
		domainRec.ID, domainRec.DomainName, domainRec.ProjectID, domainRec.SSLStatus)
	return 0
}

func runCaddy(log *slog.Logger, args []string) int {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(os.Stderr, "error: caddy requires a subcommand (supported: sync)")
		return 2
	}
	switch strings.TrimSpace(args[0]) {
	case "sync":
		return runCaddySync(log, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported caddy subcommand %q\n", args[0])
		return 2
	}
}

func runCaddySync(log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("caddy sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: config: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", cfg.DataDir, err)
		return 1
	}
	ctx := context.Background()
	db, err := database.OpenSQLite(ctx, cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: sqlite: %v\n", err)
		return 1
	}
	defer db.Close()
	store := repository.New(db)
	if err := syncCaddyRoutes(ctx, log, cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "error: caddy sync: %v\n", err)
		return 1
	}
	fmt.Printf("generated_path=%s\nroot_config=%s\n", cfg.CaddyGeneratedPath, cfg.CaddyRootConfig)
	return 0
}

func syncCaddyRoutes(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store) error {
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

func validateRepoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("only http(s) URLs are supported (got scheme %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

func validateRuntimeConfig(cfg *config.Config) error {
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

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func cfgBoolDefault(envKey string, def bool) bool {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return def
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return val
}

func waitForHealthy(ctx context.Context, hostPort int, cfg *config.Config) error {
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
