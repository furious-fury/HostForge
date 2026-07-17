package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const (
	ManagedLabel       = "dev.hostforge.managed"
	ResourceTypeLabel  = "dev.hostforge.resource-type"
	ApplicationIDLabel = "dev.hostforge.application-id"
	EnvironmentIDLabel = "dev.hostforge.environment-id"
	ServiceIDLabel     = "dev.hostforge.service-id"
	InstanceIDLabel    = "dev.hostforge.database-instance-id"
)

func EnvironmentNetworkName(environmentID string) string {
	return "hostforge-env-" + strings.ToLower(strings.TrimSpace(environmentID))
}

func EnsureEnvironmentNetwork(ctx context.Context, cli *client.Client, applicationID, environmentID string) (string, error) {
	name := EnvironmentNetworkName(environmentID)
	if name == "hostforge-env-" {
		return "", fmt.Errorf("environment id required")
	}
	inspected, err := cli.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err == nil {
		if inspected.Network.Labels[ManagedLabel] != "true" ||
			inspected.Network.Labels[EnvironmentIDLabel] != strings.TrimSpace(environmentID) {
			return "", fmt.Errorf("network %s exists without matching HostForge ownership labels", name)
		}
		return inspected.Network.ID, nil
	}
	if !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect environment network %s: %w", name, err)
	}
	result, err := cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			ManagedLabel:       "true",
			ResourceTypeLabel:  "environment-network",
			ApplicationIDLabel: strings.TrimSpace(applicationID),
			EnvironmentIDLabel: strings.TrimSpace(environmentID),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create environment network %s: %w", name, err)
	}
	return result.ID, nil
}

func ValidateEnvironmentNetwork(ctx context.Context, cli *client.Client, applicationID, environmentID string) error {
	name := EnvironmentNetworkName(environmentID)
	inspected, err := cli.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect environment network %s: %w", name, err)
	}
	labels := inspected.Network.Labels
	if labels[ManagedLabel] != "true" || labels[ResourceTypeLabel] != "environment-network" || labels[ApplicationIDLabel] != strings.TrimSpace(applicationID) || labels[EnvironmentIDLabel] != strings.TrimSpace(environmentID) {
		return fmt.Errorf("environment network %s ownership labels do not match", name)
	}
	return nil
}

// RemoveEnvironmentNetworkIfEmpty removes only an owned HostForge environment
// network with no attached containers. Callers must separately prove that no
// retained database instance still reserves the environment identity.
func RemoveEnvironmentNetworkIfEmpty(ctx context.Context, cli *client.Client, environmentID string) (bool, error) {
	name := EnvironmentNetworkName(environmentID)
	inspected, err := cli.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect environment network before removal: %w", err)
	}
	if inspected.Network.Labels[ManagedLabel] != "true" || inspected.Network.Labels[ResourceTypeLabel] != "environment-network" || inspected.Network.Labels[EnvironmentIDLabel] != strings.TrimSpace(environmentID) {
		return false, fmt.Errorf("refusing to remove network %s without matching HostForge ownership labels", name)
	}
	if len(inspected.Network.Containers) > 0 {
		return false, nil
	}
	if _, err := cli.NetworkRemove(ctx, inspected.Network.ID, client.NetworkRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return false, fmt.Errorf("remove environment network %s: %w", name, err)
	}
	return true, nil
}

func EnsureManagedVolume(ctx context.Context, cli *client.Client, name string, labels map[string]string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("volume name required")
	}
	inspected, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err == nil {
		if inspected.Volume.Labels[ManagedLabel] != "true" ||
			inspected.Volume.Labels[ResourceTypeLabel] != "database-volume" {
			return "", fmt.Errorf("volume %s exists without HostForge database ownership labels", name)
		}
		for key, value := range labels {
			if inspected.Volume.Labels[key] != strings.TrimSpace(value) {
				return "", fmt.Errorf("volume %s exists with mismatched ownership label %s", name, key)
			}
		}
		return inspected.Volume.Name, nil
	}
	if !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect volume %s: %w", name, err)
	}
	ownedLabels := map[string]string{ManagedLabel: "true", ResourceTypeLabel: "database-volume"}
	for key, value := range labels {
		ownedLabels[key] = strings.TrimSpace(value)
	}
	result, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: ownedLabels})
	if err != nil {
		return "", fmt.Errorf("create volume %s: %w", name, err)
	}
	return result.Volume.Name, nil
}

func ValidateManagedVolume(ctx context.Context, cli *client.Client, name string, labels map[string]string) error {
	inspected, err := cli.VolumeInspect(ctx, strings.TrimSpace(name), client.VolumeInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect managed database volume %s: %w", name, err)
	}
	if inspected.Volume.Labels[ManagedLabel] != "true" || inspected.Volume.Labels[ResourceTypeLabel] != "database-volume" {
		return fmt.Errorf("volume %s database ownership labels do not match", name)
	}
	for key, value := range labels {
		if inspected.Volume.Labels[key] != strings.TrimSpace(value) {
			return fmt.Errorf("volume %s ownership label %s does not match", name, key)
		}
	}
	return nil
}

func RemoveManagedVolume(ctx context.Context, cli *client.Client, name string) error {
	return RemoveManagedDatabaseVolume(ctx, cli, name, "")
}

func ManagedVolumeUsage(ctx context.Context, cli *client.Client) (map[string]int64, error) {
	result, err := cli.DiskUsage(ctx, client.DiskUsageOptions{Volumes: true})
	if err != nil {
		return nil, fmt.Errorf("inspect managed volume usage: %w", err)
	}
	out := map[string]int64{}
	for _, item := range result.Volumes.Items {
		if item.Labels[ManagedLabel] == "true" && item.Labels[ResourceTypeLabel] == "database-volume" && item.UsageData != nil && item.UsageData.Size >= 0 {
			out[item.Name] = item.UsageData.Size
		}
	}
	return out, nil
}

// RemoveManagedDatabaseVolume permanently removes a HostForge database volume.
// When instanceID is provided, the ownership label must match before deletion.
func RemoveManagedDatabaseVolume(ctx context.Context, cli *client.Client, name, instanceID string) error {
	inspected, err := cli.VolumeInspect(ctx, strings.TrimSpace(name), client.VolumeInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect volume before removal: %w", err)
	}
	if inspected.Volume.Labels[ManagedLabel] != "true" ||
		inspected.Volume.Labels[ResourceTypeLabel] != "database-volume" {
		return fmt.Errorf("refusing to remove volume %s without database ownership labels", name)
	}
	if strings.TrimSpace(instanceID) != "" && inspected.Volume.Labels[InstanceIDLabel] != strings.TrimSpace(instanceID) {
		return fmt.Errorf("refusing to remove volume %s with mismatched database instance ownership", name)
	}
	if _, err := cli.VolumeRemove(ctx, inspected.Volume.Name, client.VolumeRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove volume %s: %w", name, err)
	}
	return nil
}

type ManagedContainerOptions struct {
	ImageRef         string
	ContainerName    string
	Env              []string
	Command          []string
	Labels           map[string]string
	NetworkName      string
	NetworkAliases   []string
	VolumeName       string
	VolumeTarget     string
	CPULimitMillis   int
	MemoryLimitBytes int64
}

// RunManagedContainer creates a persistent container without publishing any host
// ports. It is intended for environment-private database resources.
func RunManagedContainer(ctx context.Context, cli *client.Client, opts ManagedContainerOptions) (string, error) {
	if strings.TrimSpace(opts.ImageRef) == "" || strings.TrimSpace(opts.ContainerName) == "" {
		return "", fmt.Errorf("image and container name required")
	}
	if strings.TrimSpace(opts.NetworkName) == "" {
		return "", fmt.Errorf("managed container network required")
	}
	if strings.TrimSpace(opts.VolumeName) == "" || strings.TrimSpace(opts.VolumeTarget) == "" {
		return "", fmt.Errorf("managed container volume and target required")
	}
	if opts.CPULimitMillis <= 0 || opts.MemoryLimitBytes <= 0 {
		return "", fmt.Errorf("managed container cpu and memory limits required")
	}
	labels := map[string]string{ManagedLabel: "true", ResourceTypeLabel: "database-container"}
	for key, value := range opts.Labels {
		labels[key] = strings.TrimSpace(value)
	}
	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:  strings.TrimSpace(opts.ImageRef),
			Env:    opts.Env,
			Cmd:    opts.Command,
			Labels: labels,
		},
		HostConfig: &container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			Resources: container.Resources{
				NanoCPUs: int64(opts.CPULimitMillis) * 1_000_000,
				Memory:   opts.MemoryLimitBytes,
			},
			Mounts: []mount.Mount{{
				Type: mount.TypeVolume, Source: strings.TrimSpace(opts.VolumeName),
				Target: strings.TrimSpace(opts.VolumeTarget),
			}},
		},
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			strings.TrimSpace(opts.NetworkName): {Aliases: opts.NetworkAliases},
		}},
		Name: strings.TrimSpace(opts.ContainerName),
	})
	if err != nil {
		return "", fmt.Errorf("create managed container: %w", err)
	}
	if _, err := cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		_, _ = cli.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("start managed container %s: %w", shortID(resp.ID), err)
	}
	return resp.ID, nil
}

type ManagedContainerInspection struct {
	ID             string
	Running        bool
	Status         string
	Labels         map[string]string
	ImageRef       string
	NanoCPUs       int64
	MemoryBytes    int64
	PublishedPorts bool
	VolumeMounts   map[string]string
	NetworkAliases map[string][]string
}

func InspectManagedContainer(ctx context.Context, cli *client.Client, containerID string) (ManagedContainerInspection, error) {
	result, err := cli.ContainerInspect(ctx, strings.TrimSpace(containerID), client.ContainerInspectOptions{})
	if err != nil {
		return ManagedContainerInspection{}, err
	}
	if result.Container.Config == nil || result.Container.Config.Labels[ManagedLabel] != "true" {
		return ManagedContainerInspection{}, fmt.Errorf("container %s is not HostForge managed", shortID(containerID))
	}
	inspection := ManagedContainerInspection{ID: result.Container.ID, Labels: result.Container.Config.Labels, ImageRef: result.Container.Config.Image, VolumeMounts: map[string]string{}, NetworkAliases: map[string][]string{}}
	if result.Container.HostConfig != nil {
		inspection.NanoCPUs = result.Container.HostConfig.NanoCPUs
		inspection.MemoryBytes = result.Container.HostConfig.Memory
		inspection.PublishedPorts = len(result.Container.HostConfig.PortBindings) > 0
	}
	for _, mounted := range result.Container.Mounts {
		if mounted.Name != "" {
			inspection.VolumeMounts[mounted.Name] = mounted.Destination
		}
	}
	if result.Container.NetworkSettings != nil {
		for name, endpoint := range result.Container.NetworkSettings.Networks {
			if endpoint != nil {
				inspection.NetworkAliases[name] = append([]string(nil), endpoint.Aliases...)
			}
		}
	}
	if result.Container.State != nil {
		inspection.Running = result.Container.State.Running
		inspection.Status = string(result.Container.State.Status)
	}
	return inspection, nil
}

func PullImage(ctx context.Context, cli *client.Client, imageRef string) error {
	response, err := cli.ImagePull(ctx, strings.TrimSpace(imageRef), client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", imageRef, err)
	}
	defer response.Close()
	if _, err := io.Copy(io.Discard, response); err != nil {
		return fmt.Errorf("read image pull response: %w", err)
	}
	return nil
}

func ExecExitCode(ctx context.Context, cli *client.Client, containerID string, command []string, env []string) (int, error) {
	if strings.TrimSpace(containerID) == "" || len(command) == 0 {
		return -1, fmt.Errorf("container and command required")
	}
	exec, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{Cmd: command, Env: env})
	if err != nil {
		return -1, fmt.Errorf("create container exec: %w", err)
	}
	if _, err := cli.ExecStart(ctx, exec.ID, client.ExecStartOptions{Detach: true}); err != nil {
		return -1, fmt.Errorf("start container exec: %w", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := cli.ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
		if err != nil {
			return -1, fmt.Errorf("inspect container exec: %w", err)
		}
		if !state.Running {
			return state.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
		}
	}
}

type ManagedJobOptions struct {
	ImageRef, ContainerName, NetworkName string
	VolumeName, VolumeTarget             string
	Command, Env                         []string
	Labels                               map[string]string
}

// RunManagedJobAndStream executes a short-lived, network-private job and
// streams only stdout to out. The container has no mounts or published ports.
func RunManagedJobAndStream(ctx context.Context, cli *client.Client, opts ManagedJobOptions, out io.Writer) error {
	if strings.TrimSpace(opts.ImageRef) == "" || strings.TrimSpace(opts.ContainerName) == "" || strings.TrimSpace(opts.NetworkName) == "" || len(opts.Command) == 0 || out == nil {
		return fmt.Errorf("managed job image, name, network, command, and output required")
	}
	labels := map[string]string{ManagedLabel: "true", ResourceTypeLabel: "database-backup-job"}
	for key, value := range opts.Labels {
		labels[key] = strings.TrimSpace(value)
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           &container.Config{Image: strings.TrimSpace(opts.ImageRef), Cmd: opts.Command, Env: opts.Env, Labels: labels, AttachStdout: true, AttachStderr: true},
		HostConfig:       &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}, Tmpfs: map[string]string{"/tmp": "rw,noexec,nosuid,size=256m"}},
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{strings.TrimSpace(opts.NetworkName): {}}},
		Name:             strings.TrimSpace(opts.ContainerName),
	})
	if err != nil {
		return fmt.Errorf("create managed database job: %w", err)
	}
	defer func() {
		_, _ = cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	}()
	attached, err := cli.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdout: true, Stderr: true})
	if err != nil {
		return fmt.Errorf("attach managed database job: %w", err)
	}
	defer attached.Close()
	wait := cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start managed database job: %w", err)
	}
	var stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(out, &stderr, attached.Reader); err != nil {
		return fmt.Errorf("stream managed database job: %w", err)
	}
	select {
	case err := <-wait.Error:
		if err != nil {
			return fmt.Errorf("wait for managed database job: %w", err)
		}
	case result := <-wait.Result:
		if result.StatusCode != 0 {
			return fmt.Errorf("managed database job exited with code %d", result.StatusCode)
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// RunManagedJobWithInput executes a private restore job and streams input into
// its stdin without persisting the payload in container logs or a host file.
func RunManagedJobWithInput(ctx context.Context, cli *client.Client, opts ManagedJobOptions, input io.Reader) error {
	if strings.TrimSpace(opts.ImageRef) == "" || strings.TrimSpace(opts.ContainerName) == "" || strings.TrimSpace(opts.NetworkName) == "" || len(opts.Command) == 0 || input == nil {
		return fmt.Errorf("managed restore job image, name, network, command, and input required")
	}
	labels := map[string]string{ManagedLabel: "true", ResourceTypeLabel: "database-restore-job"}
	for key, value := range opts.Labels {
		labels[key] = strings.TrimSpace(value)
	}
	hostConfig := &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}, Tmpfs: map[string]string{"/tmp": "rw,noexec,nosuid,size=256m"}}
	if strings.TrimSpace(opts.VolumeName) != "" || strings.TrimSpace(opts.VolumeTarget) != "" {
		if strings.TrimSpace(opts.VolumeName) == "" || !strings.HasPrefix(strings.TrimSpace(opts.VolumeTarget), "/") {
			return fmt.Errorf("managed restore job volume name and absolute target must be provided together")
		}
		hostConfig.Binds = []string{strings.TrimSpace(opts.VolumeName) + ":" + strings.TrimSpace(opts.VolumeTarget)}
	}
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           &container.Config{Image: strings.TrimSpace(opts.ImageRef), Cmd: opts.Command, Env: opts.Env, Labels: labels, AttachStdin: true, OpenStdin: true, StdinOnce: true, AttachStdout: true, AttachStderr: true},
		HostConfig:       hostConfig,
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{strings.TrimSpace(opts.NetworkName): {}}},
		Name:             strings.TrimSpace(opts.ContainerName),
	})
	if err != nil {
		return fmt.Errorf("create managed database restore job: %w", err)
	}
	defer func() {
		_, _ = cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	}()
	attached, err := cli.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
	if err != nil {
		return fmt.Errorf("attach managed database restore job: %w", err)
	}
	defer attached.Close()
	wait := cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("start managed database restore job: %w", err)
	}
	outputDone := make(chan error, 1)
	go func() {
		var stderr bytes.Buffer
		_, streamErr := stdcopy.StdCopy(io.Discard, &stderr, attached.Reader)
		outputDone <- streamErr
	}()
	if _, err := io.Copy(attached.Conn, input); err != nil {
		return fmt.Errorf("stream managed database restore input: %w", err)
	}
	_ = attached.CloseWrite()
	if err := <-outputDone; err != nil {
		return fmt.Errorf("read managed database restore output: %w", err)
	}
	select {
	case err := <-wait.Error:
		if err != nil {
			return fmt.Errorf("wait for managed database restore job: %w", err)
		}
	case result := <-wait.Result:
		if result.StatusCode != 0 {
			return fmt.Errorf("managed database restore job exited with code %d", result.StatusCode)
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
