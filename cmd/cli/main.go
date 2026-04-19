package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/docker"
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
  -host-port int
    	host port mapping: -1 range mode, 0 ephemeral, >0 exact (default from %s)
  -port-start int
    	range start when host-port=-1 (default from %s)
  -port-end int
    	range end when host-port=-1 (default from %s)
  -container-port int
    	app port inside container (default from %s)

`, os.Args[0], config.DataDirEnv, config.HostPortEnv, config.PortStartEnv, config.PortEndEnv, config.ContainerPortEnv)
}

func runDeploy(log *slog.Logger, args []string) int {
	defaultHostPort, defaultPortStart, defaultPortEnd, defaultContainerPort, err := config.RuntimeDefaults()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: runtime env defaults: %v\n", err)
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

	slug := git.WorktreeDir(repoURL, *branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildID := time.Now().UTC().Format("20060102t150405")
	imageRef := fmt.Sprintf("hostforge/%s:%s", slug, buildID)
	containerName := fmt.Sprintf("hostforge-%s-%s", slug[:12], buildID)

	ctx := context.Background()
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: docker: %v\n", err)
		return 1
	}
	defer dockerClient.Close()

	log.Info("cloning", "url", repoURL, "worktree", worktree)
	if err := git.CloneOrUpdate(ctx, repoURL, *branch, worktree); err != nil {
		fmt.Fprintf(os.Stderr, "error: clone: %v\n", err)
		return 1
	}

	hostPortValue, err := docker.PickHostPort(cfg.HostPort, cfg.PortStart, cfg.PortEnd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: host port selection: %v\n", err)
		return 1
	}

	log.Info("running nixpacks image build", "dir", worktree, "image", imageRef)
	if err := nixpacks.BuildImage(ctx, worktree, imageRef); err != nil {
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
		fmt.Fprintf(os.Stderr, "error: run container: %v\n", err)
		return 1
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", hostPortValue)
	log.Info("deploy finished", "image", imageRef, "container_id", shortID(containerID), "url", url)
	fmt.Printf("container_id=%s\nimage=%s\ncontainer_port=%d\nhost_port=%d\nurl=%s\n",
		containerID, imageRef, cfg.ContainerPort, hostPortValue, url)
	if err := writeLastDeploy(filepath.Join(cfg.DataDir, "last-deploy.json"), deployState{
		ContainerID:   containerID,
		ImageRef:      imageRef,
		RepoURL:       repoURL,
		Branch:        strings.TrimSpace(*branch),
		HostPort:      hostPortValue,
		ContainerPort: cfg.ContainerPort,
		Worktree:      worktree,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write last-deploy.json: %v\n", err)
	}
	return 0
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
	return nil
}

type deployState struct {
	ContainerID   string `json:"container_id"`
	ImageRef      string `json:"image_ref"`
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch,omitempty"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Worktree      string `json:"worktree"`
	CreatedAt     string `json:"created_at"`
}

func writeLastDeploy(path string, state deployState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
