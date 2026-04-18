package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/logging"
	"github.com/hostforge/hostforge/internal/nixpacks"
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
  hostforge version

deploy clones the repository (HTTPS), runs nixpacks build in the worktree, and streams build logs to stdout/stderr.

Flags for deploy:
  -data-dir string
    	data directory (overrides %s)
  -branch string
    	git branch (default: remote default)

`, os.Args[0], config.DataDirEnv)
}

func runDeploy(log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("deploy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data directory (overrides "+config.DataDirEnv+")")
	branch := fs.String("branch", "", "git branch (default: remote default)")
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
	for _, d := range []string{cfg.DataDir, cfg.WorktreesDir(), cfg.BuildsDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: mkdir %s: %v\n", d, err)
			return 1
		}
	}

	slug := git.WorktreeDir(repoURL, *branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildOut := filepath.Join(cfg.BuildsDir(), slug)

	ctx := context.Background()
	log.Info("cloning", "url", repoURL, "worktree", worktree)
	if err := git.CloneOrUpdate(ctx, repoURL, *branch, worktree); err != nil {
		fmt.Fprintf(os.Stderr, "error: clone: %v\n", err)
		return 1
	}

	log.Info("running nixpacks", "dir", worktree, "output", buildOut)
	if err := nixpacks.Build(ctx, worktree, buildOut, "hostforge-build"); err != nil {
		fmt.Fprintf(os.Stderr, "error: nixpacks: %v\n", err)
		return 1
	}
	log.Info("build finished", "output", buildOut)
	return 0
}

func validateRepoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("only http(s) URLs are supported in phase 0 (got scheme %q)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}
