// Package main implements the hostforge command-line interface: deploy (clone, build, run)
// and version. Deploy persists control-plane state to SQLite under the configured data directory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/database"
	"github.com/hostforge/hostforge/internal/dnsops"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/logging"
	"github.com/hostforge/hostforge/internal/repository"
	"github.com/hostforge/hostforge/internal/services"
	"github.com/hostforge/hostforge/internal/version"
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
	case "validate":
		code := runValidate(log, os.Args[2:])
		os.Exit(code)
	case "version":
		fmt.Printf("hostforge %s\n", version.Display())
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
  hostforge domain remove [flags] (--id <domain_id> | --domain <host> <repo_url>)
  hostforge domain edit [flags] --id <domain_id> --domain <new_host>
  hostforge caddy sync [flags]
  hostforge validate docker|preflight
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
	for _, d := range []string{cfg.DataDir, cfg.WorktreesDir(), cfg.BuildsDir(), cfg.LogsDir()} {
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
	var envSealer *envcrypt.Sealer
	if k := strings.TrimSpace(os.Getenv(config.EnvEncryptionKeyEnv)); k != "" {
		sealer, err := envcrypt.NewFromBase64Key(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", config.EnvEncryptionKeyEnv, err)
			return 1
		}
		envSealer = sealer
	}
	resolvedBranch := git.ResolveBranch(ctx, repoURL, strings.TrimSpace(*branch), git.AuthOptions{})
	project, err := store.EnsureProject(ctx, repoURL, resolvedBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
		return 1
	}
	result, err := services.Deploy(ctx, log, cfg, store, services.DeployPrepareInput{
		Project: project,
		RepoURL: repoURL,
		Branch:  resolvedBranch,
	}, envSealer, cliGitAuthResolver(ctx, store, envSealer))
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
		fmt.Fprintln(os.Stderr, "error: domain requires a subcommand (supported: add, remove, edit)")
		return 2
	}
	switch strings.TrimSpace(args[0]) {
	case "add":
		return runDomainAdd(log, args[1:])
	case "remove":
		return runDomainRemove(log, args[1:])
	case "edit":
		return runDomainEdit(log, args[1:])
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
	if err := dnsops.ValidateDomainName(strings.TrimSpace(*domainName)); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

	resolvedBranch := git.ResolveBranch(ctx, repoURL, strings.TrimSpace(*branch), git.AuthOptions{})
	project, err := store.EnsureProject(ctx, repoURL, resolvedBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
		return 1
	}
	domainRec, err := store.CreateDomain(ctx, project.ID, strings.TrimSpace(*domainName))
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateDomain) {
			fmt.Fprintln(os.Stderr, "error: duplicate domain (hostname already registered)")
			return 1
		}
		fmt.Fprintf(os.Stderr, "error: domain state: %v\n", err)
		return 1
	}
	log.Info("domain added", "domain", domainRec.DomainName, "project_id", domainRec.ProjectID)
	fmt.Printf("domain_id=%s\ndomain=%s\nproject_id=%s\nssl_status=%s\n",
		domainRec.ID, domainRec.DomainName, domainRec.ProjectID, domainRec.SSLStatus)
	printDNSGuidance(os.Stdout, cfg, ctx, []string{domainRec.DomainName})
	if err := maybeSyncDomainsCLI(ctx, log, cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "warning: caddy sync after domain change: %v\n", err)
	}
	return 0
}

func runDomainRemove(log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("domain remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	id := fs.String("id", "", "domain row id (from domain add / UI)")
	domainName := fs.String("domain", "", "hostname to remove (requires <repo_url>)")
	branch := fs.String("branch", "", "git branch when resolving project by repo (default: remote default)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if strings.TrimSpace(*id) == "" && (strings.TrimSpace(*domainName) == "" || len(rest) != 1) {
		fmt.Fprintln(os.Stderr, "error: domain remove requires either --id <domain_id> OR (--domain <host> and exactly one <repo_url>)")
		return 2
	}
	if strings.TrimSpace(*id) != "" && (strings.TrimSpace(*domainName) != "" || len(rest) != 0) {
		fmt.Fprintln(os.Stderr, "error: when using --id, do not pass --domain or <repo_url>")
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

	var removedID string
	if strings.TrimSpace(*id) != "" {
		if _, err := store.GetDomainByID(ctx, *id); err != nil {
			if errors.Is(err, repository.ErrDomainNotFound) {
				fmt.Fprintln(os.Stderr, "error: domain not found")
				return 1
			}
			fmt.Fprintf(os.Stderr, "error: lookup domain: %v\n", err)
			return 1
		}
		if err := store.DeleteDomain(ctx, *id); err != nil {
			fmt.Fprintf(os.Stderr, "error: delete domain: %v\n", err)
			return 1
		}
		removedID = strings.TrimSpace(*id)
	} else {
		repoURL, err := services.CanonicalRepoURL(rest[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid repo URL: %v\n", err)
			return 2
		}
		resolvedBranch := git.ResolveBranch(ctx, repoURL, strings.TrimSpace(*branch), git.AuthOptions{})
		project, err := store.EnsureProject(ctx, repoURL, resolvedBranch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: project state: %v\n", err)
			return 1
		}
		d, err := store.GetDomainByProjectAndName(ctx, project.ID, strings.TrimSpace(*domainName))
		if err != nil {
			if errors.Is(err, repository.ErrDomainNotFound) {
				fmt.Fprintln(os.Stderr, "error: domain not found for this project")
				return 1
			}
			fmt.Fprintf(os.Stderr, "error: lookup domain: %v\n", err)
			return 1
		}
		if err := store.DeleteDomain(ctx, d.ID); err != nil {
			fmt.Fprintf(os.Stderr, "error: delete domain: %v\n", err)
			return 1
		}
		removedID = d.ID
	}
	log.Info("domain removed", "domain_id", removedID)
	fmt.Printf("removed_domain_id=%s\n", removedID)
	if err := maybeSyncDomainsCLI(ctx, log, cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "warning: caddy sync after domain change: %v\n", err)
	}
	return 0
}

func runDomainEdit(log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("domain edit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	id := fs.String("id", "", "domain row id")
	newName := fs.String("domain", "", "new hostname (FQDN)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*id) == "" || strings.TrimSpace(*newName) == "" {
		fmt.Fprintln(os.Stderr, "error: domain edit requires --id and --domain <new_host>")
		return 2
	}
	if err := dnsops.ValidateDomainName(strings.TrimSpace(*newName)); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

	existing, err := store.GetDomainByID(ctx, *id)
	if err != nil {
		if errors.Is(err, repository.ErrDomainNotFound) {
			fmt.Fprintln(os.Stderr, "error: domain not found")
			return 1
		}
		fmt.Fprintf(os.Stderr, "error: lookup domain: %v\n", err)
		return 1
	}
	updated, err := store.UpdateDomainName(ctx, existing.ProjectID, existing.ID, strings.TrimSpace(*newName))
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateDomain) {
			fmt.Fprintln(os.Stderr, "error: duplicate domain (hostname already registered)")
			return 1
		}
		fmt.Fprintf(os.Stderr, "error: update domain: %v\n", err)
		return 1
	}
	log.Info("domain updated", "domain_id", updated.ID, "domain", updated.DomainName)
	fmt.Printf("domain_id=%s\ndomain=%s\nproject_id=%s\nssl_status=%s\n",
		updated.ID, updated.DomainName, updated.ProjectID, updated.SSLStatus)
	printDNSGuidance(os.Stdout, cfg, ctx, []string{updated.DomainName})
	if err := maybeSyncDomainsCLI(ctx, log, cfg, store); err != nil {
		fmt.Fprintf(os.Stderr, "warning: caddy sync after domain change: %v\n", err)
	}
	return 0
}

func maybeSyncDomainsCLI(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store) error {
	if !cfg.DomainSyncAfterMutate {
		return nil
	}
	if strings.TrimSpace(cfg.CaddyRootConfig) == "" {
		return nil
	}
	return services.SyncCaddyRoutes(ctx, log, cfg, store)
}

func printDNSGuidance(w io.Writer, cfg *config.Config, ctx context.Context, hostnames []string) {
	g := dnsops.BuildGuidance(ctx, cfg, hostnames)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "# --- DNS records (add at your DNS provider) ---")
	fmt.Fprintf(w, "# IPv4 source: %s\n", g.IPv4Source)
	if g.IPv4 != "" {
		fmt.Fprintf(w, "# Suggested IPv4 target: %s\n", g.IPv4)
	}
	if g.IPv6 != "" {
		fmt.Fprintf(w, "# IPv6 source: %s\n", g.IPv6Source)
		fmt.Fprintf(w, "# Suggested IPv6 target: %s\n", g.IPv6)
	}
	for _, step := range g.Steps {
		fmt.Fprintf(w, "# %s\n", step)
	}
	if len(g.Steps) > 0 {
		fmt.Fprintln(w, "#")
	}
	for _, r := range g.Records {
		if strings.TrimSpace(r.Value) == "" {
			continue
		}
		fmt.Fprintf(w, "# %-5s name=%-8s value=%-40s zone=%s\n", r.Type, r.Name, r.Value, r.ZoneHint)
		if r.Note != "" {
			fmt.Fprintf(w, "#       %s\n", r.Note)
		}
	}
	if g.Message != "" {
		fmt.Fprintf(w, "# Note: %s\n", g.Message)
	}
	fmt.Fprintln(w, "# --- end DNS ---")
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
	fmt.Println("caddy sync: ok (snippet written and root Caddyfile validated).")
	fmt.Println("If Caddy was not running, reload was skipped; start it with: sudo systemctl start caddy")
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
