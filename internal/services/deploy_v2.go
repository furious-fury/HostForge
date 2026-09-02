package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/git"
	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

// DeployLockKey returns the operations queue lock key that serialises
// deploys for one service environment. Also used as the git worktree scope
// (git.WorktreeDir): the two must stay identical, or work that queues one
// at a time behind this key could still share a checkout with something
// running concurrently.
func DeployLockKey(serviceID, environmentID string) string {
	return "svc:" + serviceID + ":" + environmentID
}

// newBuildID returns a build identifier unique even within the same
// second. The timestamp alone collides for two deploys of the same
// repo+branch starting in the same wall-clock second, which corrupts both
// the image tag (the second build silently overwrites the first) and the
// container name (docker.RunContainer fails outright on the clash).
func newBuildID() (string, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate build id suffix: %w", err)
	}
	return time.Now().UTC().Format("20060102t150405") + "-" + hex.EncodeToString(suffix), nil
}

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
	if service.ServiceType != "application" {
		return DeployTarget{}, ErrCode("service_type_not_deployable", fmt.Errorf("service type %s does not use application deployments", service.ServiceType))
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

// deployPriority maps a deploy's trigger to its operations-queue priority
// (ADR-0002 §20.2). A webhook can fan out to several deploys from one push;
// manual, redeploy, and rollback are each one operator action awaiting one
// result, so they go first.
func deployPriority(trigger string) int {
	if trigger == "github_push" {
		return webhookDeployPriority
	}
	return manualDeployPriority
}

const (
	manualDeployPriority  = 200
	webhookDeployPriority = 150
)

// deployContainerNameFromImageRef derives the candidate container name from
// an image ref of the form "hostforge/<slug>:<buildID>". PrepareServiceDeploy
// and LoadDeployJob both need it -- Prepare has slug and buildID as separate
// values already, but computing the name from the image ref they combine
// into is what lets LoadDeployJob reconstruct it later without ContainerName
// ever being persisted.
func deployContainerNameFromImageRef(imageRef string) (string, error) {
	repo, buildID, ok := strings.Cut(imageRef, ":")
	if !ok || buildID == "" {
		return "", fmt.Errorf("image ref %q has no build id tag", imageRef)
	}
	slug := strings.TrimPrefix(repo, "hostforge/")
	if len(slug) < 12 {
		return "", fmt.Errorf("image ref %q has a slug shorter than 12 characters", imageRef)
	}
	return fmt.Sprintf("hostforge-%s-%s", slug[:12], buildID), nil
}

func PrepareServiceDeploy(ctx context.Context, cfg *config.Config, store *repository.Store, target DeployTarget, trigger, actor, commitHash, rollbackOf string) (DeployJob, error) {
	repoURL := strings.TrimSpace(target.Service.RepoURL)
	branch := strings.TrimSpace(target.Binding.Branch)
	slug := git.WorktreeDir(DeployLockKey(target.Service.ID, target.Environment.ID), repoURL, branch)
	worktree := filepath.Join(cfg.WorktreesDir(), slug)
	buildDirectory, err := ResolveServiceBuildDirectory(worktree, target.Service.RootDirectory)
	if err != nil {
		return DeployJob{}, err
	}
	buildID, err := newBuildID()
	if err != nil {
		return DeployJob{}, err
	}
	imageRef := fmt.Sprintf("hostforge/%s:%s", slug, buildID)
	containerName, err := deployContainerNameFromImageRef(imageRef)
	if err != nil {
		return DeployJob{}, err
	}

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

	// Deliberately the last write, and deliberately not in the same
	// transaction as CreateServiceDeployment above: an operation visible
	// before logs_path is written is claimable before logs_path is written,
	// and ExecuteDeploy depends on that file already existing. If this
	// fails, the deployment row is stuck QUEUED with nothing left to claim
	// it -- mark it FAILED so it is never left silently stuck; if the
	// process dies in this exact window instead, the orphan sweep
	// (RecoverOperations, commit 3) catches it on the next boot.
	if _, err := store.EnqueueDeployOperation(ctx, repository.EnqueueDeployOperationInput{
		DeploymentID:  deployment.ID,
		LockKey:       DeployLockKey(target.Service.ID, target.Environment.ID),
		ApplicationID: target.Application.ID,
		ServiceID:     target.Service.ID,
		EnvironmentID: target.Environment.ID,
		Actor:         strings.TrimSpace(actor),
		Priority:      deployPriority(trigger),
	}); err != nil {
		_ = store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentFailed, "enqueue_failed")
		return DeployJob{}, fmt.Errorf("enqueue deploy operation: %w", err)
	}

	return DeployJob{
		Target: &target, Deployment: deployment, PreviousContainer: previousContainer,
		RepoURL: repoURL, Branch: branch, Worktree: worktree, BuildDirectory: buildDirectory, ImageRef: imageRef,
		ContainerName: containerName, LogsPath: logsPath,
	}, nil
}

// LoadDeployJob reconstructs a DeployJob from a persisted deployments row,
// for the queue handler to hand to ExecuteDeploy. Every field either lives on
// deployment directly or is cheap to re-derive from target -- the same
// values PrepareServiceDeploy computed, recomputed the same way, except
// Worktree and ContainerName.
//
// Worktree prefers the persisted column over recomputing it: WorktreeDir's
// scope-keyed hash (commit 4 of this phase) produces a different path than
// the pre-phase-2 hash did, so a deployment prepared before that change and
// claimed after an upgrade must still resolve to the path it was actually
// cloned into, not a path that has never been cloned.
//
// ContainerName is the one DeployJob field with no column of its own; it is
// re-derived from ImageRef through the same function Prepare used to compute
// it, so the two can never drift from each other.
func LoadDeployJob(ctx context.Context, cfg *config.Config, store *repository.Store, deployment models.Deployment) (DeployJob, error) {
	target, err := ResolveDeployTarget(ctx, store, deployment.ServiceID, deployment.EnvironmentID)
	if err != nil {
		return DeployJob{}, fmt.Errorf("resolve deploy target: %w", err)
	}
	repoURL := strings.TrimSpace(target.Service.RepoURL)
	branch := strings.TrimSpace(deployment.Branch)

	worktree := strings.TrimSpace(deployment.Worktree)
	if worktree == "" {
		slug := git.WorktreeDir(DeployLockKey(target.Service.ID, target.Environment.ID), repoURL, branch)
		worktree = filepath.Join(cfg.WorktreesDir(), slug)
	}
	buildDirectory, err := ResolveServiceBuildDirectory(worktree, target.Service.RootDirectory)
	if err != nil {
		return DeployJob{}, err
	}
	containerName, err := deployContainerNameFromImageRef(deployment.ImageRef)
	if err != nil {
		return DeployJob{}, err
	}

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

	return DeployJob{
		Target: &target, Deployment: deployment, PreviousContainer: previousContainer,
		RepoURL: repoURL, Branch: branch, Worktree: worktree, BuildDirectory: buildDirectory, ImageRef: deployment.ImageRef,
		ContainerName: containerName, LogsPath: deployment.LogsPath,
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

func (j DeployJob) healthConfig(cfg *config.Config, detectedStackKind string) *config.Config {
	servicePath := strings.TrimSpace(j.Target.Service.HealthCheckPath)
	if servicePath == "" {
		return cfg
	}
	copy := *cfg
	copy.HealthPath = servicePath
	// Early v2 services stored /health as an automatic default. Standard Next.js
	// applications serve / without defining that route, so use Railpack's stack
	// detection to preserve zero-config deployments for those existing services.
	if strings.EqualFold(j.Target.Service.DeployRuntime, "auto") &&
		servicePath == "/health" &&
		detectedStackKind == "node_next" {
		copy.HealthPath = "/"
	}
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
	managed, err := store.ListDatabaseConnectionBindingsSealed(ctx, serviceID, environmentID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && len(managed) == 0 {
		return nil, nil
	}
	if sealer == nil {
		return nil, ErrCode("env_encryption_key_missing", fmt.Errorf("environment has stored variables but encryption key is not configured"))
	}
	out := make([]string, 0, len(rows)+len(managed))
	keys := make([]string, 0, len(rows)+len(managed))
	seen := map[string]struct{}{}
	positions := map[string]int{}
	for _, row := range rows {
		plaintext, err := sealer.Open(row.ValueCT)
		if err != nil {
			return nil, ErrCode("env_decrypt_failed", fmt.Errorf("%s: %w", row.Key, err))
		}
		keys = append(keys, row.Key)
		seen[row.Key] = struct{}{}
		positions[row.Key] = len(out)
		out = append(out, row.Key+"="+string(plaintext))
	}
	for _, binding := range managed {
		_, conflict := seen[binding.VariableKey]
		if conflict && !binding.ReplaceExisting {
			return nil, ErrCode("database_binding_variable_conflict", fmt.Errorf("%s is already defined", binding.VariableKey))
		}
		password, err := sealer.Open(binding.PasswordCT)
		if err != nil {
			return nil, ErrCode("database_credential_decrypt_failed", fmt.Errorf("%s: %w", binding.DatabaseInstanceID, err))
		}
		scheme := map[string]string{
			"postgresql": "postgresql", "mysql": "mysql", "mariadb": "mysql",
			"mongodb": "mongodb", "redis": "redis", "valkey": "redis",
		}[binding.Engine]
		if scheme == "" {
			return nil, ErrCode("database_engine_unsupported", fmt.Errorf("%s", binding.Engine))
		}
		connection := &url.URL{
			Scheme: scheme,
			User:   url.UserPassword(binding.Username, string(password)),
			Host:   binding.NetworkAlias + ":" + strconv.Itoa(binding.InternalPort),
		}
		if binding.DatabaseName != "" {
			connection.Path = "/" + binding.DatabaseName
		}
		value := binding.VariableKey + "=" + connection.String()
		if conflict {
			out[positions[binding.VariableKey]] = value
		} else {
			keys = append(keys, binding.VariableKey)
			seen[binding.VariableKey] = struct{}{}
			positions[binding.VariableKey] = len(out)
			out = append(out, value)
		}
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

func StopServiceEnvironment(ctx context.Context, store *repository.Store, dockerClient *mobyclient.Client, serviceID, environmentID string) (ServiceRuntimeResult, error) {
	_, deployment, container, err := ResolveActiveServiceContainer(ctx, store, serviceID, environmentID)
	if err != nil {
		return ServiceRuntimeResult{}, err
	}
	client := dockerClient
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

func RestartServiceEnvironment(ctx context.Context, store *repository.Store, dockerClient *mobyclient.Client, serviceID, environmentID string) (ServiceRuntimeResult, error) {
	_, deployment, container, err := ResolveActiveServiceContainer(ctx, store, serviceID, environmentID)
	if err != nil {
		return ServiceRuntimeResult{}, err
	}
	client := dockerClient
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
