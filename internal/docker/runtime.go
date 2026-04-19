package docker

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

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

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        opts.ImageRef,
			ExposedPorts: exposed,
			Env:          []string{fmt.Sprintf("PORT=%d", opts.ContainerPort)},
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

// RemoveImage deletes an image reference (unused now, retained for future cleanup flows).
func RemoveImage(ctx context.Context, cli *client.Client, imageRef string) error {
	_, err := cli.ImageRemove(ctx, imageRef, client.ImageRemoveOptions{Force: true, PruneChildren: true})
	if err != nil {
		return fmt.Errorf("remove image %s: %w", imageRef, err)
	}
	return nil
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}
