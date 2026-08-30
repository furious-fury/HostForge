// Package docker wraps the Docker Engine API for container lifecycle and port publishing.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
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
	ImageRef       string
	ContainerName  string
	ContainerPort  int
	HostPort       int
	NetworkName    string
	NetworkAliases []string
	Labels         map[string]string
	// Env is extra KEY=value pairs (runtime). PORT is always set from ContainerPort and wins over any duplicate here.
	Env []string
	// MemoryLimitBytes, CPULimitMillis, and PidsLimit are fleet-wide resource
	// limits (ADR-0002 §14.2), not per-service — every app container gets the
	// same values, sourced from config. 0 disables that one limit, matching
	// Docker's own "0 = unlimited" semantics.
	MemoryLimitBytes int64
	CPULimitMillis   int
	PidsLimit        int64
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

	var networkingConfig *network.NetworkingConfig
	if networkName := strings.TrimSpace(opts.NetworkName); networkName != "" {
		networkingConfig = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			networkName: {Aliases: opts.NetworkAliases},
		}}
	}
	resources := container.Resources{
		Memory:   opts.MemoryLimitBytes,
		NanoCPUs: int64(opts.CPULimitMillis) * 1_000_000,
	}
	if opts.PidsLimit > 0 {
		limit := opts.PidsLimit
		resources.PidsLimit = &limit
	}
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        opts.ImageRef,
			ExposedPorts: exposed,
			// PORT matches common PaaS conventions (e.g. Heroku); app must listen on this port
			// for the host→container port mapping to receive traffic.
			Env:    env,
			Labels: opts.Labels,
		},
		HostConfig: &container.HostConfig{
			PortBindings: bindings,
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
			Resources: resources,
			// CapDrop/SecurityOpt/tmpfs mirror the database-gateway hardening
			// profile (ADR-0002 §14.4). ReadonlyRootfs is deliberately not set
			// here — arbitrary application images are not safe under it by
			// default, and there is no per-service opt-in yet to make it safe.
			CapDrop:     []string{"ALL"},
			SecurityOpt: []string{"no-new-privileges"},
			Tmpfs: map[string]string{
				"/tmp": "rw,noexec,nosuid,nodev,size=16m",
			},
		},
		NetworkingConfig: networkingConfig,
		Name:             opts.ContainerName,
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
	return StopAndRemoveWithTimeout(ctx, cli, containerID, 10)
}

func StopAndRemoveWithTimeout(ctx context.Context, cli *client.Client, containerID string, timeout int) error {
	if timeout <= 0 {
		timeout = 10
	}
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "is not running") {
			return fmt.Errorf("stop container %s: %w", shortID(containerID), err)
		}
	}
	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("remove container %s: %w", shortID(containerID), err)
	}
	return nil
}

// StopContainer stops a running container without removing it.
func StopContainer(ctx context.Context, cli *client.Client, containerID string) error {
	return StopContainerWithTimeout(ctx, cli, containerID, 10)
}

func StopContainerWithTimeout(ctx context.Context, cli *client.Client, containerID string, timeout int) error {
	if timeout <= 0 {
		timeout = 10
	}
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "is not running") {
		return fmt.Errorf("stop container %s: %w", shortID(containerID), err)
	}
	return nil
}

// StartContainer starts an existing stopped container.
func StartContainer(ctx context.Context, cli *client.Client, containerID string) error {
	if _, err := cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "already started") {
		return fmt.Errorf("start container %s: %w", shortID(containerID), err)
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

// ReadContainerLogs returns a bounded tail of a container's stdout and stderr.
func ReadContainerLogs(ctx context.Context, cli *client.Client, containerID string, tail int) (string, error) {
	if tail <= 0 || tail > 5000 {
		tail = 200
	}
	var output bytes.Buffer
	if err := StreamContainerLogs(ctx, cli, containerID, LogStreamOptions{
		Tail: fmt.Sprintf("%d", tail), Timestamps: true, ShowStdout: true, ShowStderr: true,
	}, &output); err != nil {
		return "", err
	}
	return output.String(), nil
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

type ContainerMetric struct {
	CPUPercent  float64
	MemoryBytes int64
	NetworkRX   int64
	NetworkTX   int64
	SampledAt   time.Time
}

func CollectContainerMetric(ctx context.Context, cli *client.Client, containerID string) (ContainerMetric, error) {
	result, err := cli.ContainerStats(ctx, containerID, client.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
	if err != nil {
		return ContainerMetric{}, fmt.Errorf("container stats: %w", err)
	}
	defer result.Body.Close()
	var stats container.StatsResponse
	if err := json.NewDecoder(result.Body).Decode(&stats); err != nil {
		return ContainerMetric{}, fmt.Errorf("decode container stats: %w", err)
	}
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	cpus := stats.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = uint32(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta >= 0 && cpus > 0 {
		cpuPercent = cpuDelta / systemDelta * float64(cpus) * 100
	}
	memory := stats.MemoryStats.Usage
	if cache := stats.MemoryStats.Stats["inactive_file"]; cache > 0 && memory >= cache {
		memory -= cache
	}
	var rx, tx uint64
	for _, network := range stats.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}
	at := stats.Read.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return ContainerMetric{CPUPercent: cpuPercent, MemoryBytes: int64(memory), NetworkRX: int64(rx), NetworkTX: int64(tx), SampledAt: at}, nil
}
