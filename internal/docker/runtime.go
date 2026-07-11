// Package docker wraps the Docker Engine API for container lifecycle and port publishing.
package docker

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// NewClient creates a Docker client from env and validates connectivity.
func NewClient(ctx context.Context) (*client.Client, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	pong, err := cli.Ping(ctx, client.PingOptions{})
	if err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("ping docker daemon: %w", err)
	}
	_ = pong
	return cli, nil
}

// RunOptions configures container creation and startup.
type RunOptions struct {
	ImageRef      string
	ContainerName string
	ContainerPort int
	HostPort      int
	// Env is extra KEY=value pairs (runtime). PORT is always set from ContainerPort and wins over any duplicate here.
	Env []string
}

// RunContainer creates and starts a container with host->container port mapping.
func RunContainer(ctx context.Context, cli *client.Client, opts RunOptions) (string, error) {
	if opts.ContainerPort <= 0 {
		return "", fmt.Errorf("container port must be > 0")
	}
	if opts.HostPort <= 0 {
		return "", fmt.Errorf("host port must be > 0")
	}

	port, err := network.ParsePort(strconv.Itoa(opts.ContainerPort) + "/tcp")
	if err != nil {
		return "", fmt.Errorf("container port format: %w", err)
	}
	exposed := network.PortSet{port: struct{}{}}
	hostIP := netip.MustParseAddr("127.0.0.1")
	bindings := network.PortMap{
		port: []network.PortBinding{{
			HostIP:   hostIP,
			HostPort: strconv.Itoa(opts.HostPort),
		}},
	}

	portEnv := fmt.Sprintf("PORT=%d", opts.ContainerPort)
	env := []string{portEnv}
	for _, e := range opts.Env {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		// Never let callers override the bound app port.
		if strings.HasPrefix(strings.ToUpper(e), "PORT=") {
			continue
		}
		env = append(env, e)
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        opts.ImageRef,
			ExposedPorts: exposed,
			// PORT matches common PaaS conventions (e.g. Heroku); app must listen on this port
			// for the host→container port mapping to receive traffic.
			Env: env,
		},
		HostConfig: &container.HostConfig{
			PortBindings: bindings,
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
		},
		Name: opts.ContainerName,
	})
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container %s: %w", shortID(resp.ID), err)
	}
	return resp.ID, nil
}

// StopAndRemove stops a running container (best effort) and removes it.
func StopAndRemove(ctx context.Context, cli *client.Client, containerID string) error {
	timeout := 10
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "is not running") {
		return fmt.Errorf("stop container %s: %w", shortID(containerID), err)
	}
	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container %s: %w", shortID(containerID), err)
	}
	return nil
}

// StopContainer stops a running container without removing it.
func StopContainer(ctx context.Context, cli *client.Client, containerID string) error {
	timeout := 10
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "is not running") {
		return fmt.Errorf("stop container %s: %w", shortID(containerID), err)
	}
	return nil
}

// RestartContainer restarts a container and waits up to timeout seconds.
func RestartContainer(ctx context.Context, cli *client.Client, containerID string, timeoutSeconds int) error {
	timeout := timeoutSeconds
	if timeout <= 0 {
		timeout = 10
	}
	if _, err := cli.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("restart container %s: %w", shortID(containerID), err)
	}
	return nil
}

// RemoveImage deletes an image reference (unused now, retained for future cleanup flows).
func RemoveImage(ctx context.Context, cli *client.Client, imageRef string) error {
	_, err := cli.ImageRemove(ctx, imageRef, client.ImageRemoveOptions{Force: true, PruneChildren: true})
	if err != nil {
		return fmt.Errorf("remove image %s: %w", imageRef, err)
	}
	return nil
}

// LogStreamOptions controls container log streaming behavior.
type LogStreamOptions struct {
	Follow     bool
	Tail       string
	Since      string
	Timestamps bool
	ShowStdout bool
	ShowStderr bool
}

// StreamContainerLogs streams Docker container logs to out and demultiplexes stdout/stderr.
func StreamContainerLogs(ctx context.Context, cli *client.Client, containerID string, opts LogStreamOptions, out io.Writer) error {
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container id must not be empty")
	}
	if out == nil {
		return fmt.Errorf("output writer must not be nil")
	}
	showStdout := opts.ShowStdout
	showStderr := opts.ShowStderr
	if !showStdout && !showStderr {
		showStdout = true
		showStderr = true
	}
	reader, err := cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: showStdout,
		ShowStderr: showStderr,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Tail:       opts.Tail,
		Since:      opts.Since,
	})
	if err != nil {
		return fmt.Errorf("container logs %s: %w", shortID(containerID), err)
	}
	defer reader.Close()
	if _, err := stdcopy.StdCopy(out, out, reader); err != nil {
		return fmt.Errorf("stream container logs %s: %w", shortID(containerID), err)
	}
	return nil
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
