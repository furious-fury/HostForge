package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hostforge/hostforge/internal/config"
	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/git"
	"github.com/hostforge/hostforge/internal/models"
	"github.com/hostforge/hostforge/internal/repository"
)

type DeployTarget struct {
	Application repository.Application
	Service     repository.Service
	Environment repository.Environment
	Binding     repository.ServiceEnvironment
}

func ResolveDeployTarget(ctx context.Context, store *repository.Store, serviceID, environmentID string) (DeployTarget, error) {
	service, err := store.GetService(ctx, serviceID)
	if err != nil {
		return DeployTarget{}, err
	}
	environment, err := store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return DeployTarget{}, err
	}
	if environment.ApplicationID != service.ApplicationID {
		return DeployTarget{}, repository.ErrEnvironmentNotFound
	}
	binding, err := store.GetServiceEnvironment(ctx, service.ID, environment.ID)
	if err != nil {
		return DeployTarget{}, err
	}
	if strings.TrimSpace(binding.Branch) == "" {
		return DeployTarget{}, ErrCode("service_environment_branch_required", fmt.Errorf("service environment has no branch"))
	}
	application, err := store.GetApplication(ctx, service.ApplicationID)
	if err != nil {
		return DeployTarget{}, err
	}
	return DeployTarget{Application: application, Service: service, Environment: environment, Binding: binding}, nil
}

func PrepareServiceDeploy(ctx context.Context, cfg *config.Config, store *repository.Store, target DeployTarget, trigger, actor, commitHash, rollbackOf string) (DeployJob, error) {
	repoURL := strings.TrimSpace(target.Service.RepoURL)
	branch := strings.TrimSpace(target.Binding.Branch)
	slug := git.WorktreeDir(repoURL, branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildDirectory, err := ResolveServiceBuildDirectory(worktree, target.Service.RootDirectory)
	if err != nil {
		return DeployJob{}, err
	}
	buildID := time.Now().UTC().Format("20060102t150405")
	imageRef := fmt.Sprintf("hostforge/%s:%s", slug, buildID)
	containerName := fmt.Sprintf("hostforge-%s-%s", slug[:12], buildID)

	previousDeployment, err := store.GetLatestSuccessfulServiceDeployment(ctx, target.Service.ID, target.Environment.ID)
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

	deployment, err := store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{
		ServiceID: target.Service.ID, EnvironmentID: target.Environment.ID,
		CommitHash: commitHash, ImageRef: imageRef, Worktree: worktree,
		TriggerKind: trigger, Actor: actor, RollbackOf: rollbackOf,
		Branch: branch,
	})
	if err != nil {
		return DeployJob{}, fmt.Errorf("deployment state: %w", err)
	}
	logsPath := filepath.Join(cfg.LogsDir(), deployment.ID+".log")
	if err := store.UpdateDeploymentLogsPath(ctx, deployment.ID, logsPath); err != nil {
		return DeployJob{}, fmt.Errorf("deployment log path state: %w", err)
	}
	deployment.LogsPath = logsPath
	if err := os.MkdirAll(filepath.Dir(logsPath), 0o755); err != nil {
		return DeployJob{}, fmt.Errorf("create logs dir: %w", err)
	}
	if file, err := os.OpenFile(logsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		_ = file.Close()
	}
	return DeployJob{
		Target: &target, Deployment: deployment, PreviousContainer: previousContainer,
		RepoURL: repoURL, Branch: branch, Worktree: worktree, BuildDirectory: buildDirectory, ImageRef: imageRef,
		ContainerName: containerName, LogsPath: logsPath,
	}, nil
}

func (j DeployJob) serviceID() string { return j.Target.Service.ID }

func (j DeployJob) environmentID() string { return j.Target.Environment.ID }

func (j DeployJob) internalPort(cfg *config.Config) int {
	if j.Target.Service.InternalPort > 0 {
		return j.Target.Service.InternalPort
	}
	return cfg.ContainerPort
}

func resolveDeployGitAuth(ctx context.Context, job DeployJob, resolver GitAuthResolver) (git.AuthOptions, error) {
	if resolver == nil || job.Target == nil || job.Target.Service.GitHubInstallationID <= 0 {
		return git.AuthOptions{}, ErrCode("github_app_required", fmt.Errorf("service must be linked to a GitHub App installation"))
	}
	return resolver.ResolveInstallationAuth(ctx, job.Target.Service.GitHubInstallationID)
}

func (j DeployJob) healthConfig(cfg *config.Config) *config.Config {
	if strings.TrimSpace(j.Target.Service.HealthCheckPath) == "" {
		return cfg
	}
	copy := *cfg
	copy.HealthPath = j.Target.Service.HealthCheckPath
	return &copy
}

func buildDockerEnvForJob(ctx context.Context, log *slog.Logger, store *repository.Store, job DeployJob, sealer *envcrypt.Sealer) ([]string, error) {
	return buildDockerEnvFromEnvironment(ctx, log, store, job.Target.Service.ApplicationID, job.Target.Environment.ID, job.Target.Service.ID, sealer)
}

func buildDockerEnvFromEnvironment(ctx context.Context, log *slog.Logger, store *repository.Store, applicationID, environmentID, serviceID string, sealer *envcrypt.Sealer) ([]string, error) {
	rows, err := store.ListEnvironmentVariablesSealed(ctx, applicationID, environmentID, serviceID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if sealer == nil {
		return nil, ErrCode("env_encryption_key_missing", fmt.Errorf("environment has stored variables but encryption key is not configured"))
	}
	out := make([]string, 0, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		plaintext, err := sealer.Open(row.ValueCT)
		if err != nil {
			return nil, ErrCode("env_decrypt_failed", fmt.Errorf("%s: %w", row.Key, err))
		}
		keys = append(keys, row.Key)
		out = append(out, row.Key+"="+string(plaintext))
	}
	log.Info("deploy step", "step", "container_env_inject", "env_count", len(keys), "env_keys", strings.Join(keys, ","))
	return out, nil
}

type ServiceRuntimeResult struct {
	DeploymentID string
	ContainerID  string
	Status       string
}

func ResolveActiveServiceContainer(ctx context.Context, store *repository.Store, serviceID, environmentID string) (DeployTarget, models.Deployment, models.Container, error) {
	target, err := ResolveDeployTarget(ctx, store, serviceID, environmentID)
	if err != nil {
		return DeployTarget{}, models.Deployment{}, models.Container{}, err
	}
	if strings.TrimSpace(target.Binding.ActiveDeploymentID) == "" {
		return DeployTarget{}, models.Deployment{}, models.Container{}, ErrCode("runtime_no_active_deployment", sql.ErrNoRows)
	}
	deployment, err := store.GetServiceDeployment(ctx, target.Binding.ActiveDeploymentID)
	if err != nil {
		return DeployTarget{}, models.Deployment{}, models.Container{}, ErrCode("runtime_active_deployment_lookup_failed", err)
	}
	container, err := store.GetContainerByDeploymentID(ctx, deployment.ID)
	if err != nil {
		return DeployTarget{}, models.Deployment{}, models.Container{}, ErrCode("runtime_active_container_lookup_failed", err)
	}
	return target, deployment, container, nil
}

func StopServiceEnvironment(ctx context.Context, store *repository.Store, serviceID, environmentID string) (ServiceRuntimeResult, error) {
	_, deployment, container, err := ResolveActiveServiceContainer(ctx, store, serviceID, environmentID)
	if err != nil {
		return ServiceRuntimeResult{}, err
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		return ServiceRuntimeResult{}, ErrCode("docker_unavailable", err)
	}
	defer client.Close()
	if err := docker.StopContainer(ctx, client, container.DockerContainerID); err != nil {
		return ServiceRuntimeResult{}, ErrCode("stop_container_failed", err)
	}
	if err := store.UpdateContainerStatus(ctx, container.ID, "STOPPED"); err != nil {
		return ServiceRuntimeResult{}, ErrCode("stop_container_state_failed", err)
	}
	if err := store.SetServiceDesiredState(ctx, serviceID, environmentID, "stopped"); err != nil {
		return ServiceRuntimeResult{}, ErrCode("stop_binding_state_failed", err)
	}
	return ServiceRuntimeResult{DeploymentID: deployment.ID, ContainerID: container.DockerContainerID, Status: "stopped"}, nil
}

func RestartServiceEnvironment(ctx context.Context, store *repository.Store, serviceID, environmentID string) (ServiceRuntimeResult, error) {
	_, deployment, container, err := ResolveActiveServiceContainer(ctx, store, serviceID, environmentID)
	if err != nil {
		return ServiceRuntimeResult{}, err
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		return ServiceRuntimeResult{}, ErrCode("docker_unavailable", err)
	}
	defer client.Close()
	if err := docker.RestartContainer(ctx, client, container.DockerContainerID, 10); err != nil {
		return ServiceRuntimeResult{}, ErrCode("restart_container_failed", err)
	}
	if err := store.UpdateContainerStatus(ctx, container.ID, "RUNNING"); err != nil {
		return ServiceRuntimeResult{}, ErrCode("restart_container_state_failed", err)
	}
	if err := store.SetServiceDesiredState(ctx, serviceID, environmentID, "running"); err != nil {
		return ServiceRuntimeResult{}, ErrCode("restart_binding_state_failed", err)
	}
	return ServiceRuntimeResult{DeploymentID: deployment.ID, ContainerID: container.DockerContainerID, Status: "running"}, nil
}

func dockerEnvMap(values []string) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}
