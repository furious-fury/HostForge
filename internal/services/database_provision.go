package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/containerd/errdefs"
	backupstorage "github.com/hostforge/hostforge/internal/backups"
	"github.com/hostforge/hostforge/internal/crypto/envcrypt"
	"github.com/hostforge/hostforge/internal/databases"
	"github.com/hostforge/hostforge/internal/docker"
	"github.com/hostforge/hostforge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

type DatabaseConnectionInput struct {
	ConsumerServiceID string
	VariableKey       string
	ReplaceExisting   bool
}

func processDatabaseUpgrade(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, operation repository.DatabaseOperation, fail func(string, error)) {
	job, err := store.GetDatabaseUpgradeJob(ctx, operation.ID)
	if err != nil {
		fail("database_upgrade_job_missing", err)
		return
	}
	if job.Status == "rolled_back" {
		fail("database_upgrade_failed_rolled_back", errors.New("patch upgrade failed and the previous image was restored"))
		return
	}
	instance, err := store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil || instance.ID != job.DatabaseInstanceID {
		fail("database_instance_lookup_failed", fmt.Errorf("upgrade target instance is unavailable"))
		return
	}
	if instance.ImageRef == job.TargetImageRef && instance.Status == "healthy" {
		_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "success")
		_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "upgrade_complete", 100, "", "")
		return
	}
	if instance.ImageRef != job.PreviousImageRef || instance.EngineVersion != job.EngineVersion {
		fail("database_upgrade_state_changed", errors.New("database image or engine version changed after upgrade was queued"))
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_service_lookup_failed", err)
		return
	}
	engine, version, ok := databases.FindVersion(databaseService.Engine, instance.EngineVersion)
	if !ok || version.ImageRef != job.TargetImageRef {
		fail("database_upgrade_catalog_changed", errors.New("queued patch image is no longer the catalog target"))
		return
	}
	credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		fail("database_credentials_lookup_failed", err)
		return
	}
	sensitive := [][]byte{}
	defer func() {
		for _, secret := range sensitive {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	password, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		fail("database_credentials_decrypt_failed", err)
		return
	}
	sensitive = append(sensitive, password)
	adminPassword, err := sealer.Open(credential.AdminPasswordCT)
	if err != nil {
		fail("database_admin_credentials_decrypt_failed", err)
		return
	}
	sensitive = append(sensitive, adminPassword)
	containerSpec, err := databaseContainerConfiguration(engine.ID, credential, password, adminPassword)
	if err != nil {
		fail("database_engine_provisioning_not_ready", err)
		return
	}
	service, err := store.GetService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_service_identity_lookup_failed", err)
		return
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		fail("docker_unavailable", err)
		return
	}
	defer client.Close()
	_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "running")
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "image_pull", 15, "", "")
	if err := docker.PullImage(ctx, client, job.TargetImageRef); err != nil {
		_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "failed")
		fail("database_upgrade_image_pull_failed", err)
		return
	}
	stoppedConsumers, err := stopDatabaseConsumers(ctx, store, client, instance)
	if err != nil {
		_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "failed")
		fail("database_upgrade_consumer_stop_failed", err)
		return
	}
	defer restartDatabaseConsumers(context.WithoutCancel(ctx), client, stoppedConsumers)
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "replace_container", 40, "", "")
	if instance.DockerContainerID != "" {
		inspection, inspectErr := docker.InspectManagedContainer(ctx, client, instance.DockerContainerID)
		if inspectErr == nil && inspection.Labels[docker.InstanceIDLabel] != instance.ID {
			_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "failed")
			fail("database_container_ownership_mismatch", errors.New("database container does not belong to the upgrade target"))
			return
		}
		if inspectErr != nil && !errdefs.IsNotFound(inspectErr) {
			_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "failed")
			fail("database_container_inspection_failed", inspectErr)
			return
		}
		if err := docker.StopAndRemoveWithTimeout(ctx, client, instance.DockerContainerID, engine.StopTimeoutSeconds); err != nil {
			_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "failed")
			fail("database_upgrade_container_remove_failed", err)
			return
		}
	}
	createContainer := func(runCtx context.Context, imageRef string) (string, error) {
		containerID, createErr := docker.RunManagedContainer(runCtx, client, docker.ManagedContainerOptions{
			ImageRef: imageRef, ContainerName: "hostforge-db-" + instance.ID[:12],
			Env: containerSpec.Env, Command: containerSpec.Command,
			Labels:      map[string]string{docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: instance.EnvironmentID, docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: instance.ID},
			NetworkName: docker.EnvironmentNetworkName(instance.EnvironmentID), NetworkAliases: []string{instance.NetworkAlias},
			VolumeName: instance.VolumeName, VolumeTarget: engine.VolumeTarget,
			CPULimitMillis: instance.CPULimitMillis, MemoryLimitBytes: instance.MemoryLimitBytes,
		})
		if createErr != nil {
			return "", createErr
		}
		_, _ = store.UpdateDatabaseInstanceState(runCtx, instance.ID, repository.UpdateDatabaseInstanceStateInput{DockerContainerID: containerID, DesiredState: "running", Status: "starting"})
		healthCtx, cancel := context.WithTimeout(runCtx, 90*time.Second)
		defer cancel()
		if healthErr := waitForDatabase(healthCtx, client, containerID, engine.ID, credential, password, adminPassword); healthErr != nil {
			_ = docker.StopAndRemoveWithTimeout(context.WithoutCancel(runCtx), client, containerID, engine.StopTimeoutSeconds)
			return "", healthErr
		}
		if configureErr := configureDatabaseAfterStart(runCtx, client, containerID, engine.ID, credential, password, adminPassword); configureErr != nil {
			_ = docker.StopAndRemoveWithTimeout(context.WithoutCancel(runCtx), client, containerID, engine.StopTimeoutSeconds)
			return "", configureErr
		}
		return containerID, nil
	}
	targetContainerID, targetErr := createContainer(ctx, job.TargetImageRef)
	if targetErr == nil {
		_, targetErr = store.CommitDatabaseInstanceUpgrade(ctx, instance.ID, job.PreviousImageRef, job.TargetImageRef, targetContainerID)
	}
	if targetErr == nil {
		_ = store.UpdateDatabaseUpgradeJobStatus(ctx, operation.ID, "success")
		_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "upgrade_complete", 100, "", "")
		if log != nil {
			log.Info("database patch image upgraded", "instance_id", instance.ID, "engine_version", instance.EngineVersion, "previous_image", job.PreviousImageRef, "target_image", job.TargetImageRef)
		}
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "rollback_previous_image", 80, "", "")
	if targetContainerID != "" {
		_ = docker.StopAndRemoveWithTimeout(context.WithoutCancel(ctx), client, targetContainerID, engine.StopTimeoutSeconds)
	}
	rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancelRollback()
	_ = docker.PullImage(rollbackCtx, client, job.PreviousImageRef)
	rollbackContainerID, rollbackErr := createContainer(rollbackCtx, job.PreviousImageRef)
	if rollbackErr != nil {
		_ = store.UpdateDatabaseUpgradeJobStatus(context.WithoutCancel(ctx), operation.ID, "failed")
		_, _ = store.UpdateDatabaseInstanceState(context.WithoutCancel(ctx), instance.ID, repository.UpdateDatabaseInstanceStateInput{ClearContainerID: true, DesiredState: "running", Status: "failed", HealthMessage: "database_upgrade_rollback_failed"})
		fail("database_upgrade_rollback_failed", fmt.Errorf("patch image failed: %v; previous image rollback failed: %w", targetErr, rollbackErr))
		return
	}
	_, _ = store.UpdateDatabaseInstanceState(context.WithoutCancel(ctx), instance.ID, repository.UpdateDatabaseInstanceStateInput{DockerContainerID: rollbackContainerID, DesiredState: "running", Status: "healthy", HealthMessage: "ready", HealthCheckedAt: time.Now().UTC()})
	_ = store.UpdateDatabaseUpgradeJobStatus(context.WithoutCancel(ctx), operation.ID, "rolled_back")
	fail("database_upgrade_failed_rolled_back", fmt.Errorf("patch image failed and the previous image was restored: %w", targetErr))
}

type CreateManagedDatabaseInput struct {
	ApplicationID     string
	Name              string
	Engine            string
	Version           string
	EnvironmentIDs    []string
	ResourcePreset    string
	CustomCPUMillis   int
	CustomMemoryBytes int64
	Connections       []DatabaseConnectionInput
	Actor             string
}

func safeDatabaseSlug(value string) string {
	var out strings.Builder
	lastSeparator := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			out.WriteRune(char)
			lastSeparator = false
			continue
		}
		if !lastSeparator && out.Len() > 0 {
			out.WriteByte('_')
			lastSeparator = true
		}
	}
	return strings.Trim(out.String(), "_")
}

func randomDatabaseIdentifierSuffix() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func randomDatabaseToken(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("token length must be positive")
	}
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(raw), "="), nil
}

func PrepareManagedDatabase(ctx context.Context, store *repository.Store, sealer *envcrypt.Sealer, in CreateManagedDatabaseInput) (repository.CreatedDatabaseService, error) {
	if sealer == nil {
		return repository.CreatedDatabaseService{}, ErrCode("env_encryption_key_missing", errors.New("database credentials require encryption"))
	}
	engine, version, ok := databases.FindVersion(strings.ToLower(strings.TrimSpace(in.Engine)), strings.TrimSpace(in.Version))
	if !ok {
		return repository.CreatedDatabaseService{}, ErrCode("database_version_not_supported", errors.New("database engine or version not in catalog"))
	}
	if strings.TrimSpace(version.ImageRef) == "" {
		return repository.CreatedDatabaseService{}, ErrCode("database_engine_provisioning_not_ready", fmt.Errorf("%s %s does not have a pinned runtime image", engine.ID, version.Version))
	}
	preset, ok := databases.FindResourcePreset(strings.ToLower(strings.TrimSpace(in.ResourcePreset)))
	if !ok {
		return repository.CreatedDatabaseService{}, ErrCode("database_resource_preset_invalid", errors.New("unknown database resource preset"))
	}
	if preset.ID == "custom" {
		if in.CustomCPUMillis < 100 || in.CustomCPUMillis > 32000 || in.CustomMemoryBytes < engine.MinimumMemoryBytes || in.CustomMemoryBytes > 256*1024*1024*1024 {
			return repository.CreatedDatabaseService{}, ErrCode("database_custom_resources_invalid", fmt.Errorf("custom resources must provide 100-32000 CPU millis and at least %d bytes of memory", engine.MinimumMemoryBytes))
		}
		preset.CPULimitMillis, preset.MemoryLimitBytes = in.CustomCPUMillis, in.CustomMemoryBytes
	}
	if preset.MemoryLimitBytes < engine.MinimumMemoryBytes {
		return repository.CreatedDatabaseService{}, ErrCode("database_resource_preset_too_small", fmt.Errorf("%s requires at least %d bytes", engine.ID, engine.MinimumMemoryBytes))
	}
	nameSlug := safeDatabaseSlug(in.Name)
	if nameSlug == "" || len(in.EnvironmentIDs) == 0 {
		return repository.CreatedDatabaseService{}, ErrCode("database_configuration_invalid", errors.New("database name and environments required"))
	}
	if len(nameSlug) > 32 {
		nameSlug = nameSlug[:32]
	}
	instances := make([]repository.CreateDatabaseInstanceInput, 0, len(in.EnvironmentIDs))
	for _, environmentID := range in.EnvironmentIDs {
		environment, err := store.GetEnvironment(ctx, environmentID)
		if err != nil || environment.ApplicationID != strings.TrimSpace(in.ApplicationID) {
			return repository.CreatedDatabaseService{}, repository.ErrEnvironmentNotFound
		}
		suffix, err := randomDatabaseIdentifierSuffix()
		if err != nil {
			return repository.CreatedDatabaseService{}, err
		}
		password, err := randomDatabaseToken(32)
		if err != nil {
			return repository.CreatedDatabaseService{}, err
		}
		passwordCT, err := sealer.Seal([]byte(password))
		if err != nil {
			return repository.CreatedDatabaseService{}, err
		}
		adminPassword, err := randomDatabaseToken(32)
		if err != nil {
			return repository.CreatedDatabaseService{}, err
		}
		adminPasswordCT, err := sealer.Seal([]byte(adminPassword))
		if err != nil {
			return repository.CreatedDatabaseService{}, err
		}
		identityEntropy := suffix[:8]
		alias := strings.ReplaceAll(nameSlug, "_", "-") + "-" + environment.Slug + "-" + identityEntropy
		databaseName := nameSlug + "_" + identityEntropy
		usernameLimit := 48
		if engine.ID == "mysql" || engine.ID == "mariadb" {
			usernameLimit = 32
		}
		usernameBaseLimit := usernameLimit - len("hf__") - len(identityEntropy)
		usernameBase := nameSlug
		if len(usernameBase) > usernameBaseLimit {
			usernameBase = usernameBase[:usernameBaseLimit]
		}
		username := "hf_" + usernameBase + "_" + identityEntropy
		if engine.ID == "redis" || engine.ID == "valkey" {
			databaseName, username = "0", "default"
		}
		bindings := make([]repository.CreateDatabaseBindingInput, 0, len(in.Connections))
		for _, connection := range in.Connections {
			variableKey := strings.ToUpper(strings.TrimSpace(connection.VariableKey))
			if variableKey == "" {
				variableKey = engine.ConnectionVariable
			}
			bindings = append(bindings, repository.CreateDatabaseBindingInput{
				ConsumerServiceID: connection.ConsumerServiceID, VariableKey: variableKey,
				ReplaceExisting: connection.ReplaceExisting,
			})
		}
		instances = append(instances, repository.CreateDatabaseInstanceInput{
			EnvironmentID: environment.ID, EngineVersion: version.Version, ImageRef: version.ImageRef,
			NetworkAlias: alias, InternalPort: engine.InternalPort,
			VolumeName:     "hostforge-db-" + strings.ToLower(suffix),
			ResourcePreset: preset.ID, CPULimitMillis: preset.CPULimitMillis,
			MemoryLimitBytes: preset.MemoryLimitBytes, DatabaseName: databaseName,
			Username: username, PasswordCT: passwordCT, AdminPasswordCT: adminPasswordCT, Bindings: bindings,
		})
	}
	created, err := store.CreateDatabaseService(ctx, repository.CreateDatabaseServiceInput{
		ApplicationID: strings.TrimSpace(in.ApplicationID), Name: strings.TrimSpace(in.Name),
		Engine: engine.ID, DefaultVersion: version.Version, Actor: strings.TrimSpace(in.Actor),
		Instances: instances,
	})
	if errors.Is(err, repository.ErrDatabaseBindingConflict) {
		return repository.CreatedDatabaseService{}, ErrCode("database_binding_variable_conflict", err)
	}
	if errors.Is(err, repository.ErrInvalidDatabaseBinding) {
		return repository.CreatedDatabaseService{}, ErrCode("database_binding_invalid", err)
	}
	if errors.Is(err, repository.ErrDuplicateService) {
		return repository.CreatedDatabaseService{}, ErrCode("database_service_name_conflict", err)
	}
	return created, err
}

func StartDatabaseOperationLoop(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, dataDir string, minFreeDiskBytes int64, concurrency int) {
	if sealer == nil {
		if log != nil {
			log.Warn("database operation worker disabled; encryption key is not configured")
		}
		return
	}
	if concurrency < 1 {
		if log != nil {
			log.Error("database operation worker disabled; concurrency must be positive")
		}
		return
	}
	for workerIndex := 0; workerIndex < concurrency; workerIndex++ {
		workerToken, err := randomDatabaseToken(12)
		if err != nil {
			if log != nil {
				log.Error("database operation worker identity generation failed", "worker", workerIndex, "error", err)
			}
			return
		}
		workerID := fmt.Sprintf("hostforge-%d-%d-%s", os.Getpid(), workerIndex, workerToken)
		go runDatabaseOperationWorker(ctx, log, store, sealer, dataDir, minFreeDiskBytes, workerID)
	}
}

func runDatabaseOperationWorker(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, dataDir string, minFreeDiskBytes int64, workerID string) {
	const leaseDuration = 2 * time.Minute
	const leaseRefresh = 30 * time.Second
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		for {
			operation, err := store.ClaimNextDatabaseOperation(ctx, workerID, leaseDuration)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				if log != nil {
					log.Error("claim database operation failed", "worker_id", workerID, "error", err)
				}
				break
			}
			operationCtx, cancelOperation := context.WithCancel(ctx)
			leaseDone := make(chan struct{})
			go func(operationID string) {
				defer close(leaseDone)
				leaseTicker := time.NewTicker(leaseRefresh)
				defer leaseTicker.Stop()
				for {
					select {
					case <-operationCtx.Done():
						return
					case <-leaseTicker.C:
						if err := store.RenewDatabaseOperationLease(operationCtx, operationID, workerID, leaseDuration); err != nil {
							if log != nil {
								log.Error("database operation lease lost; cancelling worker", "operation_id", operationID, "error", err)
							}
							cancelOperation()
							return
						}
					}
				}
			}(operation.ID)
			processDatabaseOperation(operationCtx, log, store, sealer, operation, dataDir, minFreeDiskBytes)
			cancelOperation()
			<-leaseDone
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processDatabaseOperation(ctx context.Context, log *slog.Logger, store *repository.Store, sealer *envcrypt.Sealer, operation repository.DatabaseOperation, dataDir string, minFreeDiskBytes int64) {
	defer func() {
		completed, err := store.GetDatabaseOperation(context.WithoutCancel(ctx), operation.ID)
		if err != nil || (completed.Status != "success" && completed.Status != "failed" && completed.Status != "cancelled") {
			return
		}
		service, err := store.GetService(context.WithoutCancel(ctx), completed.ServiceID)
		if err != nil {
			return
		}
		environmentID := ""
		if completed.DatabaseInstanceID != "" {
			if instance, lookupErr := store.GetDatabaseInstance(context.WithoutCancel(ctx), completed.DatabaseInstanceID); lookupErr == nil {
				environmentID = instance.EnvironmentID
			}
		}
		_ = store.RecordPlatformEvent(context.WithoutCancel(ctx), repository.PlatformEventInput{ApplicationID: service.ApplicationID, ServiceID: service.ID, EnvironmentID: environmentID, EventType: "database_operation", Status: completed.Status, Actor: completed.Actor, Message: "Database " + strings.ReplaceAll(completed.OperationType, "_", " ") + " " + completed.Status, Detail: completed.ID})
	}()
	fail := func(code string, err error) {
		err = safeDatabaseOperationError(err)
		_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "failed", "failed", operation.ProgressPercent, code, err.Error())
		if operation.DatabaseInstanceID != "" {
			state := repository.UpdateDatabaseInstanceStateInput{Status: "failed", HealthMessage: code}
			if operation.OperationType == "rotate_credentials" || operation.OperationType == "backup" || operation.OperationType == "restore" || operation.OperationType == "upgrade" {
				state.Status = ""
			}
			_, _ = store.UpdateDatabaseInstanceState(ctx, operation.DatabaseInstanceID, state)
		}
		if log != nil {
			log.Error("database operation failed", "operation_id", operation.ID, "type", operation.OperationType, "error_code", code, "error", err)
		}
	}
	if operation.OperationType == "provision" || operation.OperationType == "backup" || operation.OperationType == "restore" || operation.OperationType == "upgrade" {
		if err := requireDatabaseDiskReserve(ctx, dataDir, minFreeDiskBytes); err != nil {
			if operation.OperationType == "backup" {
				if backup, lookupErr := store.GetDatabaseBackupByOperationID(ctx, operation.ID); lookupErr == nil {
					_, _ = store.CompleteDatabaseBackup(ctx, backup.ID, repository.CompleteDatabaseBackupInput{Status: "failed", ErrorCode: "database_disk_pressure", ErrorMessage: err.Error()})
				}
			}
			if operation.OperationType == "restore" {
				_ = store.UpdateDatabaseRestoreJobStatus(ctx, operation.ID, "failed")
			}
			fail("database_disk_pressure", err)
			return
		}
	}
	switch operation.OperationType {
	case "backup":
		processDatabaseBackup(ctx, log, store, sealer, operation, fail)
		return
	case "restore":
		processDatabaseRestore(ctx, log, store, sealer, operation, fail)
		return
	case "rotate_credentials":
		processDatabaseCredentialRotation(ctx, log, store, sealer, operation, fail)
		return
	case "upgrade":
		processDatabaseUpgrade(ctx, log, store, sealer, operation, fail)
		return
	case "start", "stop", "restart":
		processDatabaseRuntimeOperation(ctx, log, store, sealer, operation, fail)
		return
	case "provision", "restore_deleted":
	default:
		fail("database_operation_not_implemented", fmt.Errorf("operation type %s", operation.OperationType))
		return
	}
	instance, err := store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil {
		fail("database_instance_lookup_failed", err)
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_service_lookup_failed", err)
		return
	}
	engine, version, ok := databases.FindVersion(databaseService.Engine, instance.EngineVersion)
	if !ok || version.ImageRef == "" || version.ImageRef != instance.ImageRef {
		fail("database_runtime_image_invalid", errors.New("instance image is not the catalog-pinned image"))
		return
	}
	credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		fail("database_credentials_lookup_failed", err)
		return
	}
	provisionSecrets := [][]byte{}
	defer func() {
		for _, secret := range provisionSecrets {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	password, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		fail("database_credentials_decrypt_failed", err)
		return
	}
	provisionSecrets = append(provisionSecrets, password)
	adminPassword, err := sealer.Open(credential.AdminPasswordCT)
	if err != nil {
		fail("database_admin_credentials_decrypt_failed", err)
		return
	}
	provisionSecrets = append(provisionSecrets, adminPassword)
	containerSpec, err := databaseContainerConfiguration(engine.ID, credential, password, adminPassword)
	if err != nil {
		fail("database_engine_provisioning_not_ready", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "docker_connect", 5, "", "")
	client, err := docker.NewClient(ctx)
	if err != nil {
		fail("docker_unavailable", err)
		return
	}
	defer client.Close()
	service, err := store.GetService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_service_identity_lookup_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "network", 15, "", "")
	networkName := docker.EnvironmentNetworkName(instance.EnvironmentID)
	if _, err := docker.EnsureEnvironmentNetwork(ctx, client, service.ApplicationID, instance.EnvironmentID); err != nil {
		fail("database_network_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "volume", 25, "", "")
	if _, err := docker.EnsureManagedVolume(ctx, client, instance.VolumeName, map[string]string{
		docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: instance.EnvironmentID,
		docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: instance.ID,
	}); err != nil {
		fail("database_volume_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "image_pull", 40, "", "")
	if err := docker.PullImage(ctx, client, instance.ImageRef); err != nil {
		fail("database_image_pull_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "container_start", 65, "", "")
	containerID := strings.TrimSpace(instance.DockerContainerID)
	containerName := "hostforge-db-" + instance.ID[:12]
	if containerID != "" {
		inspection, inspectErr := docker.InspectManagedContainer(ctx, client, containerID)
		if inspectErr != nil || databaseManagedContainerDrift(inspection, instance, engine) {
			fail("database_container_recovery_failed", errors.New("persisted container cannot be safely recovered"))
			return
		}
		if !inspection.Running {
			if err := docker.StartContainer(ctx, client, containerID); err != nil {
				fail("database_container_start_failed", err)
				return
			}
		}
	} else if inspection, inspectErr := docker.InspectManagedContainer(ctx, client, containerName); inspectErr == nil {
		if databaseManagedContainerDrift(inspection, instance, engine) {
			fail("database_container_recovery_failed", errors.New("deterministic container name is occupied by mismatched runtime state"))
			return
		}
		containerID = inspection.ID
		if !inspection.Running {
			if err := docker.StartContainer(ctx, client, containerID); err != nil {
				fail("database_container_start_failed", err)
				return
			}
		}
	} else if !errdefs.IsNotFound(inspectErr) {
		fail("database_container_recovery_failed", inspectErr)
		return
	} else {
		containerID, err = docker.RunManagedContainer(ctx, client, docker.ManagedContainerOptions{
			ImageRef: instance.ImageRef, ContainerName: containerName,
			Env: containerSpec.Env, Command: containerSpec.Command,
			Labels: map[string]string{
				docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: instance.EnvironmentID,
				docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: instance.ID,
			},
			NetworkName: networkName, NetworkAliases: []string{instance.NetworkAlias},
			VolumeName: instance.VolumeName, VolumeTarget: engine.VolumeTarget,
			CPULimitMillis: instance.CPULimitMillis, MemoryLimitBytes: instance.MemoryLimitBytes,
		})
		if err != nil {
			fail("database_container_start_failed", err)
			return
		}
	}
	_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{
		DockerContainerID: containerID, DesiredState: "running", Status: "starting",
	})
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "readiness", 80, "", "")
	healthContext, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := waitForDatabaseReadiness(healthContext, client, containerID, engine.ID, credential, password, adminPassword); err != nil {
		// Keep the stopped container and its ID so operators can inspect the
		// startup output after a failed provision attempt.
		_ = docker.StopContainerWithTimeout(context.WithoutCancel(ctx), client, containerID, engine.StopTimeoutSeconds)
		fail("database_readiness_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "engine_configuration", 90, "", "")
	if err := configureDatabaseAfterStart(ctx, client, containerID, engine.ID, credential, password, adminPassword); err != nil {
		_ = docker.StopContainerWithTimeout(context.WithoutCancel(ctx), client, containerID, engine.StopTimeoutSeconds)
		fail("database_engine_configuration_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "application_credentials", 95, "", "")
	credentialContext, cancelCredentialCheck := context.WithTimeout(ctx, 30*time.Second)
	defer cancelCredentialCheck()
	if err := waitForDatabase(credentialContext, client, containerID, engine.ID, credential, password, adminPassword); err != nil {
		_ = docker.StopContainerWithTimeout(context.WithoutCancel(ctx), client, containerID, engine.StopTimeoutSeconds)
		fail("database_application_credentials_failed", err)
		return
	}
	now := time.Now().UTC()
	if _, err := store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{
		DockerContainerID: containerID, DesiredState: "running", Status: "healthy",
		HealthMessage: "ready", HealthCheckedAt: now,
	}); err != nil {
		_ = docker.StopAndRemoveWithTimeout(ctx, client, containerID, engine.StopTimeoutSeconds)
		_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{ClearContainerID: true})
		fail("database_state_update_failed", err)
		return
	}
	if _, err := store.UpdateDatabaseOperation(ctx, operation.ID, "success", "ready", 100, "", ""); err != nil && log != nil {
		log.Error("complete database operation state failed", "operation_id", operation.ID, "error", err)
	}
	if log != nil {
		log.Info("database instance provisioned", "service_id", service.ID, "instance_id", instance.ID, "engine", engine.ID, "version", version.Version)
	}
}

func requireDatabaseDiskReserve(ctx context.Context, dataDir string, minFreeBytes int64) error {
	client, err := docker.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("inspect Docker storage root: %w", err)
	}
	defer client.Close()
	info, err := client.Info(ctx, mobyclient.InfoOptions{})
	if err != nil {
		return fmt.Errorf("inspect Docker storage root: %w", err)
	}
	path := strings.TrimSpace(info.Info.DockerRootDir)
	if path == "" {
		path = strings.TrimSpace(dataDir)
	}
	return requireDatabaseDiskReserveAtPath(path, minFreeBytes)
}

func requireDatabaseDiskReserveAtPath(path string, minFreeBytes int64) error {
	if minFreeBytes <= 0 {
		return fmt.Errorf("database minimum free disk bytes must be greater than zero")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("database storage filesystem path is unavailable")
	}
	available, err := availableFilesystemBytes(path)
	if err != nil {
		return fmt.Errorf("inspect database storage filesystem: %w", err)
	}
	if available < minFreeBytes {
		return fmt.Errorf("database operation requires %d free bytes; only %d are available", minFreeBytes, available)
	}
	return nil
}

func processDatabaseRestore(
	ctx context.Context,
	log *slog.Logger,
	store *repository.Store,
	sealer *envcrypt.Sealer,
	operation repository.DatabaseOperation,
	fail func(string, error),
) {
	job, err := store.GetDatabaseRestoreJob(ctx, operation.ID)
	if err != nil {
		fail("database_restore_job_missing", err)
		return
	}
	failRestore := func(code string, cause error) {
		_ = store.UpdateDatabaseRestoreJobStatus(ctx, operation.ID, "failed")
		fail(code, cause)
	}
	backup, err := store.GetDatabaseBackup(ctx, job.BackupID)
	if err != nil || backup.Status != "success" {
		failRestore("database_restore_backup_unavailable", fmt.Errorf("source backup is unavailable"))
		return
	}
	if job.Mode == "replace_current" {
		safety, safetyErr := store.GetDatabaseBackup(ctx, job.SafetyBackupID)
		if safetyErr != nil || safety.Status != "success" {
			failRestore("database_restore_safety_backup_failed", fmt.Errorf("safety backup did not complete successfully"))
			return
		}
	}
	target, err := store.GetDatabaseInstance(ctx, job.TargetInstanceID)
	if err != nil || target.Status != "healthy" || target.DesiredState != "running" {
		failRestore("database_restore_target_unavailable", fmt.Errorf("target database is not healthy"))
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, target.ServiceID)
	if err != nil || databaseService.Engine != backup.Engine || target.EngineVersion != backup.EngineVersion {
		failRestore("database_restore_incompatible", fmt.Errorf("backup and target database versions are incompatible"))
		return
	}
	service, err := store.GetService(ctx, target.ServiceID)
	if err != nil {
		failRestore("database_service_identity_lookup_failed", err)
		return
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		failRestore("docker_unavailable", err)
		return
	}
	defer client.Close()
	stoppedConsumers := []string{}
	if job.Mode == "replace_current" {
		stoppedConsumers, err = stopDatabaseConsumers(ctx, store, client, target)
		if err != nil {
			failRestore("database_restore_consumer_stop_failed", err)
			return
		}
		defer restartDatabaseConsumers(context.WithoutCancel(ctx), client, stoppedConsumers)
	}
	_ = store.UpdateDatabaseRestoreJobStatus(ctx, operation.ID, "running")
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "download_and_decrypt", 25, "", "")
	err = restoreDatabaseBackupPayload(ctx, store, sealer, client, backup, target, service)
	if err != nil && job.Mode == "replace_current" {
		_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "rollback_safety_backup", 80, "", "")
		safety, safetyErr := store.GetDatabaseBackup(ctx, job.SafetyBackupID)
		if safetyErr == nil {
			safetyErr = restoreDatabaseBackupPayload(ctx, store, sealer, client, safety, target, service)
		}
		if safetyErr != nil {
			failRestore("database_restore_rollback_failed", fmt.Errorf("restore failed: %v; safety rollback failed: %w", err, safetyErr))
			return
		}
		failRestore("database_restore_failed_rolled_back", fmt.Errorf("restore failed and the safety backup was restored: %w", err))
		return
	}
	if err != nil {
		failRestore("database_restore_stream_failed", err)
		return
	}
	_ = store.UpdateDatabaseRestoreJobStatus(ctx, operation.ID, "success")
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "restore_complete", 100, "", "")
	if log != nil {
		log.Info("database restore completed", "operation_id", operation.ID, "backup_id", backup.ID, "target_instance_id", target.ID, "mode", job.Mode)
	}
}

func stopDatabaseConsumers(ctx context.Context, store *repository.Store, client *mobyclient.Client, instance repository.DatabaseInstance) ([]string, error) {
	bindings, err := store.ListDatabaseBindings(ctx, instance.ID)
	if err != nil {
		return nil, err
	}
	stopped := []string{}
	seen := map[string]bool{}
	for _, binding := range bindings {
		environment, err := store.GetServiceEnvironment(ctx, binding.ConsumerServiceID, instance.EnvironmentID)
		if err != nil || strings.TrimSpace(environment.ActiveDeploymentID) == "" {
			continue
		}
		container, err := store.GetContainerByDeploymentID(ctx, environment.ActiveDeploymentID)
		if err != nil || strings.TrimSpace(container.DockerContainerID) == "" || seen[container.DockerContainerID] {
			continue
		}
		if err := docker.StopContainer(ctx, client, container.DockerContainerID); err != nil {
			restartDatabaseConsumers(context.WithoutCancel(ctx), client, stopped)
			return nil, err
		}
		seen[container.DockerContainerID] = true
		stopped = append(stopped, container.DockerContainerID)
	}
	return stopped, nil
}

func restartDatabaseConsumers(ctx context.Context, client *mobyclient.Client, containerIDs []string) {
	for _, containerID := range containerIDs {
		_ = docker.StartContainer(ctx, client, containerID)
	}
}

func restoreDatabaseBackupPayload(ctx context.Context, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, backup repository.DatabaseBackup, target repository.DatabaseInstance, service repository.Service) (returnErr error) {
	sensitive := [][]byte{}
	defer func() {
		if returnErr != nil {
			returnErr = safeDatabaseOperationError(returnErr, sensitive...)
		}
		for _, secret := range sensitive {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	destination, err := store.GetBackupDestinationSealed(ctx, backup.DestinationID)
	if err != nil {
		return err
	}
	accessKey, err := sealer.Open(destination.AccessKeyCT)
	if err != nil {
		return err
	}
	sensitive = append(sensitive, accessKey)
	secretKey, err := sealer.Open(destination.SecretKeyCT)
	if err != nil {
		return err
	}
	sensitive = append(sensitive, secretKey)
	dataKey, err := sealer.Open(backup.EncryptedDataKey)
	if err != nil {
		return err
	}
	sensitive = append(sensitive, dataKey)
	credential, err := store.GetDatabaseCredentialSealed(ctx, target.ID)
	if err != nil {
		return err
	}
	password, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		return err
	}
	sensitive = append(sensitive, password)
	storage, err := backupstorage.NewClient(ctx, backupstorage.Destination{Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket, PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption, SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(accessKey), SecretKey: string(secretKey)})
	if err != nil {
		return err
	}
	body, err := storage.Get(ctx, backup.ObjectKey)
	if err != nil {
		return err
	}
	defer body.Close()
	command, env, err := databaseRestoreCommand(backup.Engine, target, credential, password, backup.DatabaseName)
	if err != nil {
		return err
	}
	hash := sha256.New()
	plainReader, plainWriter := io.Pipe()
	decryptDone := make(chan error, 1)
	go func() {
		decryptErr := backupstorage.DecryptAndDecompress(ctx, io.TeeReader(body, hash), plainWriter, dataKey)
		_ = plainWriter.CloseWithError(decryptErr)
		decryptDone <- decryptErr
	}()
	jobOptions := docker.ManagedJobOptions{ImageRef: target.ImageRef, ContainerName: "hostforge-restore-" + backup.ID[:12], NetworkName: docker.EnvironmentNetworkName(target.EnvironmentID), Command: command, Env: env, Labels: map[string]string{docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: target.EnvironmentID, docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: target.ID}}
	offline := backup.Engine == "redis" || backup.Engine == "valkey"
	if offline {
		engine, found := databases.Find(backup.Engine)
		if !found {
			_ = plainReader.CloseWithError(fmt.Errorf("database engine is unavailable"))
			return fmt.Errorf("database engine is unavailable")
		}
		if err := docker.StopContainerWithTimeout(ctx, client, target.DockerContainerID, engine.StopTimeoutSeconds); err != nil {
			_ = plainReader.CloseWithError(err)
			return err
		}
		defer docker.StartContainer(context.WithoutCancel(ctx), client, target.DockerContainerID)
		jobOptions.VolumeName, jobOptions.VolumeTarget = target.VolumeName, "/data"
	}
	jobErr := docker.RunManagedJobWithInput(ctx, client, jobOptions, plainReader)
	_ = plainReader.CloseWithError(jobErr)
	decryptErr := <-decryptDone
	if jobErr != nil {
		return jobErr
	}
	if decryptErr != nil {
		return decryptErr
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), backup.Checksum) {
		return fmt.Errorf("backup checksum mismatch")
	}
	if offline {
		if err := docker.StartContainer(ctx, client, target.DockerContainerID); err != nil {
			return err
		}
		databaseService, err := store.GetDatabaseService(ctx, target.ServiceID)
		if err != nil {
			return err
		}
		healthCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err := waitForDatabase(healthCtx, client, target.DockerContainerID, databaseService.Engine, credential, password, nil); err != nil {
			return err
		}
	}
	return nil
}

func processDatabaseCredentialRotation(
	ctx context.Context,
	log *slog.Logger,
	store *repository.Store,
	sealer *envcrypt.Sealer,
	operation repository.DatabaseOperation,
	fail func(string, error),
) {
	instance, err := store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil {
		fail("database_instance_lookup_failed", err)
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_engine_rotation_not_ready", fmt.Errorf("credential rotation is unavailable for this engine"))
		return
	}
	credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		fail("database_credentials_lookup_failed", err)
		return
	}
	rotationSecrets := [][]byte{}
	defer func() {
		for _, secret := range rotationSecrets {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	oldPassword, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		fail("database_credentials_decrypt_failed", err)
		return
	}
	rotationSecrets = append(rotationSecrets, oldPassword)
	adminPassword, err := sealer.Open(credential.AdminPasswordCT)
	if err != nil {
		fail("database_admin_credentials_decrypt_failed", err)
		return
	}
	rotationSecrets = append(rotationSecrets, adminPassword)
	if len(credential.PendingPasswordCT) == 0 {
		generated, generateErr := randomDatabaseToken(32)
		if generateErr != nil {
			fail("database_password_generation_failed", generateErr)
			return
		}
		pendingCT, sealErr := sealer.Seal([]byte(generated))
		generatedBytes := []byte(generated)
		for index := range generatedBytes {
			generatedBytes[index] = 0
		}
		if sealErr != nil {
			fail("database_credentials_encrypt_failed", sealErr)
			return
		}
		credential, err = store.StageDatabaseCredentialRotation(ctx, instance.ID, pendingCT)
		if err != nil {
			fail("database_credential_rotation_stage_failed", err)
			return
		}
	}
	newPassword, err := sealer.Open(credential.PendingPasswordCT)
	if err != nil {
		fail("database_pending_credentials_decrypt_failed", err)
		return
	}
	rotationSecrets = append(rotationSecrets, newPassword)
	client, err := docker.NewClient(ctx)
	if err != nil {
		fail("docker_unavailable", err)
		return
	}
	defer client.Close()
	inspection, err := docker.InspectManagedContainer(ctx, client, instance.DockerContainerID)
	if err != nil || inspection.Labels[docker.InstanceIDLabel] != instance.ID {
		fail("database_container_ownership_mismatch", errors.New("container ownership labels do not match database instance"))
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "database_password", 50, "", "")
	oldPasswordValid := checkDatabaseApplicationCredentials(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, oldPassword) == nil
	newPasswordValid := checkDatabaseApplicationCredentials(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, newPassword) == nil
	switch {
	case newPasswordValid:
		// A previous worker changed the engine and stopped before committing the
		// sealed record. Finalize the already-effective staged generation.
	case oldPasswordValid:
		if err := alterDatabasePassword(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, oldPassword, newPassword, adminPassword); err != nil {
			fail("database_password_change_failed", err)
			return
		}
		if err := checkDatabaseApplicationCredentials(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, newPassword); err != nil {
			rollbackErr := alterDatabasePassword(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, newPassword, oldPassword, adminPassword)
			if rollbackErr == nil {
				_ = store.ClearStagedDatabaseCredentialRotation(ctx, instance.ID)
			}
			fail("database_rotated_credentials_verification_failed", err)
			return
		}
	default:
		fail("database_credential_rotation_state_unknown", errors.New("neither current nor staged application credentials authenticate"))
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "encrypted_state", 80, "", "")
	rotated, err := store.CommitStagedDatabaseCredentialRotation(ctx, instance.ID)
	if err != nil {
		rollbackErr := alterDatabasePassword(ctx, client, instance.DockerContainerID, databaseService.Engine, credential, newPassword, oldPassword, adminPassword)
		if rollbackErr != nil {
			fail("database_credential_rotation_compensation_failed", fmt.Errorf("persist encrypted credential: %v; restore previous database password: %w", err, rollbackErr))
		} else {
			_ = store.ClearStagedDatabaseCredentialRotation(ctx, instance.ID)
			fail("database_credential_rotation_persist_failed", err)
		}
		return
	}
	_, _ = store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{HealthMessage: "credentials rotated; redeploy bound applications"})
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "credentials_rotated", 100, "", "")
	if log != nil {
		log.Info("database credentials rotated", "service_id", instance.ServiceID, "instance_id", instance.ID, "generation", rotated.Generation)
	}
}

func processDatabaseBackup(
	ctx context.Context,
	log *slog.Logger,
	store *repository.Store,
	sealer *envcrypt.Sealer,
	operation repository.DatabaseOperation,
	fail func(string, error),
) {
	backup, err := store.GetDatabaseBackupByOperationID(ctx, operation.ID)
	if err != nil {
		fail("database_backup_record_missing", err)
		return
	}
	sensitive := [][]byte{}
	defer func() {
		for _, secret := range sensitive {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	failBackup := func(code string, cause error) {
		cause = safeDatabaseOperationError(cause, sensitive...)
		_, _ = store.CompleteDatabaseBackup(ctx, backup.ID, repository.CompleteDatabaseBackupInput{Status: "failed", ErrorCode: code, ErrorMessage: cause.Error()})
		fail(code, cause)
	}
	if err := store.MarkDatabaseBackupRunning(ctx, backup.ID); err != nil {
		failBackup("database_backup_state_failed", err)
		return
	}
	instance, err := store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil {
		failBackup("database_instance_lookup_failed", err)
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		failBackup("database_service_lookup_failed", err)
		return
	}
	credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		failBackup("database_credentials_lookup_failed", err)
		return
	}
	password, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		failBackup("database_credentials_decrypt_failed", err)
		return
	}
	sensitive = append(sensitive, password)
	destination, err := store.GetBackupDestinationSealed(ctx, backup.DestinationID)
	if err != nil {
		failBackup("backup_destination_lookup_failed", err)
		return
	}
	accessKey, err := sealer.Open(destination.AccessKeyCT)
	if err != nil {
		failBackup("backup_destination_decrypt_failed", err)
		return
	}
	sensitive = append(sensitive, accessKey)
	secretKey, err := sealer.Open(destination.SecretKeyCT)
	if err != nil {
		failBackup("backup_destination_decrypt_failed", err)
		return
	}
	sensitive = append(sensitive, secretKey)
	storage, err := backupstorage.NewClient(ctx, backupstorage.Destination{Endpoint: destination.Endpoint, Region: destination.Region, Bucket: destination.Bucket, PathStyle: destination.PathStyle, ServerSideEncryption: destination.ServerSideEncryption, SSEKMSKeyID: destination.SSEKMSKeyID, AccessKey: string(accessKey), SecretKey: string(secretKey)})
	if err != nil {
		failBackup("backup_destination_client_failed", err)
		return
	}
	command, env, archiveFormat, err := databaseBackupCommand(databaseService.Engine, instance, credential, password)
	if err != nil {
		failBackup("database_backup_adapter_unavailable", err)
		return
	}
	dataKey, err := backupstorage.GenerateDataKey()
	if err != nil {
		failBackup("database_backup_key_generation_failed", err)
		return
	}
	sensitive = append(sensitive, dataKey)
	encryptedDataKey, err := sealer.Seal(dataKey)
	if err != nil {
		failBackup("database_backup_key_wrap_failed", err)
		return
	}
	service, err := store.GetService(ctx, instance.ServiceID)
	if err != nil {
		failBackup("database_service_identity_lookup_failed", err)
		return
	}
	datePath := time.Now().UTC().Format("2006/01/02")
	objectKey := strings.Trim(strings.Join([]string{destination.ObjectPrefix, databaseService.Engine, service.ApplicationID, instance.EnvironmentID, instance.ID, datePath, backup.ID + ".hfbk"}, "/"), "/")
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "dump_stream", 25, "", "")
	dockerClient, err := docker.NewClient(ctx)
	if err != nil {
		failBackup("docker_unavailable", err)
		return
	}
	defer dockerClient.Close()

	backupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	rawReader, rawWriter := io.Pipe()
	encryptedReader, encryptedWriter := io.Pipe()
	type encryptionResult struct {
		checksum string
		size     int64
		err      error
	}
	encryptionDone := make(chan encryptionResult, 1)
	go func() {
		checksum, size, encryptErr := backupstorage.CompressAndEncrypt(backupCtx, rawReader, encryptedWriter, dataKey)
		_ = encryptedWriter.CloseWithError(encryptErr)
		encryptionDone <- encryptionResult{checksum: checksum, size: size, err: encryptErr}
	}()
	uploadDone := make(chan error, 1)
	go func() {
		uploadErr := storage.Put(backupCtx, objectKey, encryptedReader, "application/vnd.hostforge.database-backup")
		_ = encryptedReader.CloseWithError(uploadErr)
		uploadDone <- uploadErr
	}()
	jobErr := docker.RunManagedJobAndStream(backupCtx, dockerClient, docker.ManagedJobOptions{
		ImageRef: instance.ImageRef, ContainerName: "hostforge-backup-" + backup.ID[:12],
		NetworkName: docker.EnvironmentNetworkName(instance.EnvironmentID), Command: command, Env: env,
		Labels: map[string]string{docker.ApplicationIDLabel: service.ApplicationID, docker.EnvironmentIDLabel: instance.EnvironmentID, docker.ServiceIDLabel: service.ID, docker.InstanceIDLabel: instance.ID},
	}, rawWriter)
	_ = rawWriter.CloseWithError(jobErr)
	encryption := <-encryptionDone
	uploadErr := <-uploadDone
	if jobErr != nil || encryption.err != nil || uploadErr != nil {
		cancel()
		_ = storage.Delete(context.WithoutCancel(ctx), objectKey)
		cause := jobErr
		if cause == nil {
			cause = encryption.err
		}
		if cause == nil {
			cause = uploadErr
		}
		failBackup("database_backup_stream_failed", cause)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "verify_object", 90, "", "")
	remoteSize, err := storage.Size(ctx, objectKey)
	if err != nil || remoteSize != encryption.size {
		_ = storage.Delete(context.WithoutCancel(ctx), objectKey)
		if err == nil {
			err = fmt.Errorf("uploaded backup size mismatch: expected %d, got %d", encryption.size, remoteSize)
		}
		failBackup("database_backup_verification_failed", err)
		return
	}
	if _, err := store.CompleteDatabaseBackup(ctx, backup.ID, repository.CompleteDatabaseBackupInput{Status: "success", ObjectKey: objectKey, ArchiveFormat: archiveFormat, Checksum: encryption.checksum, CompressedSize: encryption.size, EncryptionAlgorithm: "AES-256-GCM-CHUNKED", EncryptedDataKey: encryptedDataKey}); err != nil {
		_ = storage.Delete(context.WithoutCancel(ctx), objectKey)
		fail("database_backup_state_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "backup_complete", 100, "", "")
	if log != nil {
		log.Info("database backup completed", "backup_id", backup.ID, "instance_id", instance.ID, "engine", databaseService.Engine, "size", encryption.size)
	}
}

func alterPostgreSQLPassword(ctx context.Context, client *mobyclient.Client, containerID, username, databaseName string, password, adminPassword []byte) error {
	for _, value := range []string{username, databaseName} {
		if value == "" {
			return errors.New("database credential identifier is empty")
		}
		for _, char := range value {
			if !(unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_') {
				return errors.New("database credential identifier contains unsafe characters")
			}
		}
	}
	if strings.ContainsAny(string(password), "'\\\n\r\x00") {
		return errors.New("generated database password contains unsafe characters")
	}
	statement := fmt.Sprintf(`ALTER ROLE "%s" WITH PASSWORD '%%s';`, username)
	exitCode, err := docker.ExecExitCode(ctx, client, containerID, []string{"sh", "-c", `printf "$HF_SQL" "$HF_NEW_PASSWORD" | PGPASSWORD="$HF_ADMIN_PASSWORD" psql --host 127.0.0.1 --username hostforge_admin --dbname "$HF_DATABASE" --set ON_ERROR_STOP=1`}, []string{"HF_SQL=" + statement, "HF_NEW_PASSWORD=" + string(password), "HF_ADMIN_PASSWORD=" + string(adminPassword), "HF_DATABASE=" + databaseName})
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("postgres password command exited with code %d", exitCode)
	}
	return nil
}

func processDatabaseRuntimeOperation(
	ctx context.Context,
	log *slog.Logger,
	store *repository.Store,
	sealer *envcrypt.Sealer,
	operation repository.DatabaseOperation,
	fail func(string, error),
) {
	instance, err := store.GetDatabaseInstance(ctx, operation.DatabaseInstanceID)
	if err != nil {
		fail("database_instance_lookup_failed", err)
		return
	}
	if strings.TrimSpace(instance.DockerContainerID) == "" {
		fail("database_container_not_provisioned", errors.New("database instance has no container"))
		return
	}
	client, err := docker.NewClient(ctx)
	if err != nil {
		fail("docker_unavailable", err)
		return
	}
	defer client.Close()
	inspection, err := docker.InspectManagedContainer(ctx, client, instance.DockerContainerID)
	if err != nil {
		fail("database_container_inspection_failed", err)
		return
	}
	if inspection.Labels[docker.ResourceTypeLabel] != "database-container" ||
		inspection.Labels[docker.InstanceIDLabel] != instance.ID {
		fail("database_container_ownership_mismatch", errors.New("container ownership labels do not match database instance"))
		return
	}
	databaseService, err := store.GetDatabaseService(ctx, instance.ServiceID)
	if err != nil {
		fail("database_service_lookup_failed", err)
		return
	}
	engine, engineFound := databases.Find(databaseService.Engine)
	if !engineFound {
		fail("database_engine_unsupported", fmt.Errorf("engine %s is unavailable", databaseService.Engine))
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", operation.OperationType, 50, "", "")
	if operation.OperationType == "stop" {
		if err := docker.StopContainerWithTimeout(ctx, client, instance.DockerContainerID, engine.StopTimeoutSeconds); err != nil {
			fail("database_container_stop_failed", err)
			return
		}
		if _, err := store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{
			DesiredState: "stopped", Status: "stopped", HealthMessage: "stopped by operator",
		}); err != nil {
			fail("database_state_update_failed", err)
			return
		}
		_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "stopped", 100, "", "")
		return
	}
	if operation.OperationType == "start" {
		err = docker.StartContainer(ctx, client, instance.DockerContainerID)
	} else {
		err = docker.RestartContainer(ctx, client, instance.DockerContainerID, engine.StopTimeoutSeconds)
	}
	if err != nil {
		fail("database_container_"+operation.OperationType+"_failed", err)
		return
	}
	credential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		fail("database_credentials_lookup_failed", err)
		return
	}
	runtimeSecrets := [][]byte{}
	defer func() {
		for _, secret := range runtimeSecrets {
			for index := range secret {
				secret[index] = 0
			}
		}
	}()
	password, err := sealer.Open(credential.PasswordCT)
	if err != nil {
		fail("database_credentials_decrypt_failed", err)
		return
	}
	runtimeSecrets = append(runtimeSecrets, password)
	adminPassword, err := sealer.Open(credential.AdminPasswordCT)
	if err != nil {
		fail("database_admin_credentials_decrypt_failed", err)
		return
	}
	runtimeSecrets = append(runtimeSecrets, adminPassword)
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "running", "readiness", 80, "", "")
	healthContext, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := waitForDatabase(healthContext, client, instance.DockerContainerID, databaseService.Engine, credential, password, adminPassword); err != nil {
		fail("database_readiness_failed", err)
		return
	}
	now := time.Now().UTC()
	if _, err := store.UpdateDatabaseInstanceState(ctx, instance.ID, repository.UpdateDatabaseInstanceStateInput{
		DesiredState: "running", Status: "healthy", HealthMessage: "ready", HealthCheckedAt: now,
	}); err != nil {
		fail("database_state_update_failed", err)
		return
	}
	_, _ = store.UpdateDatabaseOperation(ctx, operation.ID, "success", "ready", 100, "", "")
	if log != nil {
		log.Info("database runtime action completed", "instance_id", instance.ID, "operation", operation.OperationType)
	}
}

func waitForDatabase(ctx context.Context, dockerClient *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password, adminPassword []byte) error {
	return waitForDatabaseCheck(ctx, engine, func() error {
		return checkDatabaseHealth(ctx, dockerClient, containerID, engine, credential, password, adminPassword)
	})
}

func waitForDatabaseReadiness(ctx context.Context, dockerClient *mobyclient.Client, containerID, engine string, credential repository.DatabaseCredential, password, adminPassword []byte) error {
	return waitForDatabaseCheck(ctx, engine, func() error {
		return checkDatabaseReadiness(ctx, dockerClient, containerID, engine, credential, password, adminPassword)
	})
}

func waitForDatabaseCheck(ctx context.Context, engine string, check func() error) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastError error
	for {
		err := check()
		if err == nil {
			return nil
		}
		lastError = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s readiness timed out (%v): %w", engine, lastError, ctx.Err())
		case <-ticker.C:
		}
	}
}
