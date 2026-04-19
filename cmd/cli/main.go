// Package main implements the hostforge command-line interface: deploy (clone, build, run)
// and version. Deploy persists control-plane state to SQLite under the configured data directory.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/logging"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
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
	repoURL, err := services.CanonicalRepoURL(rest[0])
	if err != nil {
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
	if err := services.ValidateRuntimeConfig(cfg); err != nil {
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
	project, err := store.EnsureProject(ctx, repoURL, strings.TrimSpace(*branch))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
		return 1
	}
	result, err := services.Deploy(ctx, log, cfg, store, services.DeployPrepareInput{
		Project: project,
		RepoURL: repoURL,
		Branch:  strings.TrimSpace(*branch),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: deploy: %v\n", err)
		return 1
	}
	fmt.Printf("deployment_id=%s\ncontainer_id=%s\nimage=%s\ncontainer_port=%d\nhost_port=%d\nurl=%s\n",
		result.DeploymentID, result.ContainerID, result.ImageRef, result.ContainerPort, result.HostPort, result.URL)
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
	repoURL, err := services.CanonicalRepoURL(rest[0])
	if err != nil {
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

	project, err := store.EnsureProject(ctx, repoURL, strings.TrimSpace(*branch))
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
	if err := services.SyncCaddyRoutes(ctx, log, cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "error: caddy sync: %v\n", err)
		return 1
	}
	fmt.Printf("generated_path=%s\nroot_config=%s\n", cfg.CaddyGeneratedPath, cfg.CaddyRootConfig)
	return 0
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
