package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/furious-fury/HostForge/internal/config"
	"github.com/furious-fury/HostForge/internal/crypto/envcrypt"
	"github.com/furious-fury/HostForge/internal/dnsops"
	"github.com/furious-fury/HostForge/internal/docker"
	"github.com/furious-fury/HostForge/internal/repository"
	mobyclient "github.com/moby/moby/client"
)

// StartDatabaseGatewayOperationLoop starts the gateway operation workers and
// returns a WaitGroup that completes once they have all stopped. See
// StartDatabaseOperationLoop for the drain contract; this loop follows it.
func StartDatabaseGatewayOperationLoop(stopCtx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client, routeNotifier RouteNotifier) *sync.WaitGroup {
	var wg sync.WaitGroup
	ctx := stopCtx
	if cfg == nil || !cfg.DatabaseGatewaysEnabled {
		return &wg
	}
	if sealer == nil {
		if log != nil {
			log.Error("database gateway worker disabled; encryption key is not configured")
		}
		return &wg
	}
	if count, err := store.RequeueExpiredDatabaseGatewayOperations(ctx, time.Now().UTC()); err != nil {
		if log != nil {
			log.Error("recover interrupted gateway operations failed", "error", err)
		}
	} else if count > 0 && log != nil {
		log.Info("requeued expired database gateway operations", "count", count)
	}
	if err := queueDatabaseGatewayStartupReconciliation(ctx, store); err != nil {
		if log != nil {
			log.Error("queue database gateway startup reconciliation failed", "error", safeDatabaseOperationError(err))
		}
	}
	workers := cfg.DatabaseGatewayOperationConcurrency
	if workers < 1 {
		workers = 1
	}
	for index := 0; index < workers; index++ {
		workerID := fmt.Sprintf("hostforge-gateway-%d-%d", os.Getpid(), index)
		wg.Add(1)
		go func() {
			defer wg.Done()
			runDatabaseGatewayWorker(stopCtx, log, cfg, store, sealer, dockerClient, workerID, routeNotifier)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		runDatabaseGatewayClaims(stopCtx, log, cfg, store, dockerClient)
	}()
	return &wg
}

func queueDatabaseGatewayStartupReconciliation(ctx context.Context, store *repository.Store) error {
	if store == nil {
		return nil
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if errors.Is(err, repository.ErrDatabaseGatewayNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if endpoint.DesiredStatus != "active" {
		return nil
	}
	active, err := store.HasActiveDatabaseGatewayOperation(ctx, endpoint.Engine, "provision_gateway")
	if err != nil {
		return err
	}
	if active {
		return nil
	}
	_, err = store.QueueDatabaseGatewayProvision(ctx, endpoint.Engine, "system:startup-reconciliation")
	return err
}

func runDatabaseGatewayClaims(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, dockerClient *mobyclient.Client) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastCertificateExpiryWarning time.Time
	for {
		now := time.Now().UTC()
		if _, err := store.QueueDueDatabaseExternalConnectionExpirations(ctx, now, 50); err != nil && log != nil {
			log.Warn("claim expired database external connections failed", "error", err)
		}
		if _, err := store.QueueDueDatabaseExternalCredentialRetirements(ctx, now, 50); err != nil && log != nil {
			log.Warn("claim expired database external credential grace periods failed", "error", err)
		}
		if err := reconcileDatabaseGatewayCertificate(ctx, cfg, store, now); err != nil {
			if log != nil {
				log.Warn("reconcile database gateway certificate failed", "error", safeDatabaseOperationError(err))
			}
		} else if endpoint, endpointErr := store.GetDatabaseGatewayEndpoint(ctx, "postgresql"); endpointErr == nil && endpoint.DesiredStatus == "active" && !endpoint.CertificateExpiresAt.IsZero() && endpoint.CertificateExpiresAt.Before(now.Add(14*24*time.Hour)) && (lastCertificateExpiryWarning.IsZero() || now.Sub(lastCertificateExpiryWarning) >= 6*time.Hour) {
			if log != nil {
				log.Warn("database gateway certificate expires soon", "hostname", endpoint.Hostname, "expires_at", endpoint.CertificateExpiresAt)
			}
			lastCertificateExpiryWarning = now
		}
		if err := reconcileDatabaseGatewayUsage(ctx, store, dockerClient, now); err != nil && log != nil {
			log.Warn("reconcile database gateway usage metadata failed", "error", safeDatabaseOperationError(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reconcileDatabaseGatewayCertificate(ctx context.Context, cfg *config.Config, store *repository.Store, now time.Time) error {
	if cfg == nil || store == nil {
		return nil
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if errors.Is(err, repository.ErrDatabaseGatewayNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if endpoint.DesiredStatus != "active" {
		return nil
	}
	material, err := LoadDatabaseGatewayTLSMaterial(endpoint.Hostname, cfg.PostgreSQLGatewayCertificateFile, cfg.PostgreSQLGatewayKeyFile, cfg.CaddyStorageRoot, now)
	if err != nil {
		return err
	}
	if material.Fingerprint == endpoint.CertificateFingerprint {
		return nil
	}
	active, err := store.HasActiveDatabaseGatewayOperation(ctx, endpoint.Engine, "provision_gateway")
	if err != nil {
		return err
	}
	if active {
		return nil
	}
	_, err = store.QueueDatabaseGatewayProvision(ctx, endpoint.Engine, "system:certificate-renewal")
	return err
}

func reconcileDatabaseGatewayUsage(ctx context.Context, store *repository.Store, dockerClient *mobyclient.Client, now time.Time) error {
	if store == nil {
		return nil
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if errors.Is(err, repository.ErrDatabaseGatewayNotFound) || (err == nil && (endpoint.DesiredStatus != "active" || endpoint.DockerContainerID == "")) {
		return nil
	}
	if err != nil {
		return err
	}
	runtime := &DockerPostgreSQLGatewayRuntime{Client: dockerClient, GatewayContainerID: endpoint.DockerContainerID}
	roles, err := runtime.ActivePgBouncerRoles(ctx)
	if err != nil {
		return err
	}
	return store.TouchDatabaseExternalCredentialUsage(ctx, roles, now)
}

func runDatabaseGatewayWorker(stopCtx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client, workerID string, routeNotifier RouteNotifier) {
	const leaseDuration = 2 * time.Minute
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if stopCtx.Err() != nil {
			return
		}
		operation, err := store.ClaimNextDatabaseGatewayOperation(stopCtx, workerID, leaseDuration)
		if errors.Is(err, sql.ErrNoRows) {
			select {
			case <-stopCtx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		if err != nil {
			if log != nil {
				log.Error("claim database gateway operation failed", "error", err)
			}
			// Back off on the same ticker the empty-queue branch uses. A bare
			// continue here spins a CPU for as long as the error persists —
			// a closed or unreadable database never returns ErrNoRows, so
			// nothing else in this loop would ever yield or observe stopCtx.
			select {
			case <-stopCtx.Done():
				return
			case <-ticker.C:
			}
			continue
		}
		// Detached from stopCtx so shutdown drains a claimed operation rather
		// than cancelling it mid-step; see runDatabaseOperationWorker.
		operationCtx, cancel := context.WithCancel(context.WithoutCancel(stopCtx))
		leaseDone := make(chan struct{})
		go func() {
			defer close(leaseDone)
			leaseTicker := time.NewTicker(30 * time.Second)
			defer leaseTicker.Stop()
			for {
				select {
				case <-operationCtx.Done():
					return
				case <-leaseTicker.C:
					if err := store.RenewDatabaseGatewayOperationLease(operationCtx, operation.ID, workerID, leaseDuration); err != nil {
						cancel()
						return
					}
				}
			}
		}()
		processDatabaseGatewayOperation(operationCtx, log, cfg, store, sealer, dockerClient, operation, routeNotifier)
		cancel()
		<-leaseDone
	}
}

func processDatabaseGatewayOperation(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, dockerClient *mobyclient.Client, operation repository.DatabaseGatewayOperation, routeNotifier RouteNotifier) {
	fail := func(code string, failure error) {
		failure = safeDatabaseOperationError(failure)
		_, _ = store.UpdateDatabaseGatewayOperation(context.WithoutCancel(ctx), operation.ID, "failed", "failed", operation.ProgressPercent, code, failure.Error())
		if operation.ConnectionID != "" && operation.OperationType != "disable_connection" && operation.OperationType != "expire_connection" && operation.OperationType != "revoke_connection" && operation.OperationType != "retire_credential" && operation.OperationType != "rotate_connection" {
			_ = store.SetDatabaseExternalConnectionStatus(context.WithoutCancel(ctx), operation.ConnectionID, "failed", code, failure.Error())
		}
		if operation.OperationType == "provision_gateway" || operation.OperationType == "teardown_gateway" {
			_ = store.SetDatabaseGatewayEndpointState(context.WithoutCancel(ctx), operation.Engine, "", "failed", "", -1, -1, -1, code, failure.Error())
		}
		if log != nil {
			log.Error("database gateway operation failed", "operation_id", operation.ID, "type", operation.OperationType, "error_code", code, "error", failure)
		}
	}
	client := dockerClient
	complete := func() {
		if _, err := store.UpdateDatabaseGatewayOperation(ctx, operation.ID, "success", "ready", 100, "", ""); err != nil && log != nil {
			log.Error("complete database gateway operation failed", "operation_id", operation.ID, "error", err)
		}
	}
	switch operation.OperationType {
	case "provision_gateway":
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil {
			fail(FirstPublicCode(err), err)
			return
		}
		complete()
	case "teardown_gateway":
		if err := teardownPostgreSQLGateway(ctx, store, client, operation.Engine); err != nil {
			fail("database_gateway_teardown_failed", err)
			return
		}
		complete()
	default:
		if err := processPostgreSQLExternalConnectionOperation(ctx, log, cfg, store, sealer, client, operation, routeNotifier); err != nil {
			fail(FirstPublicCode(err), err)
			return
		}
		complete()
	}
}

func waitForDatabaseGatewayTLSMaterial(ctx context.Context, hostname, certificatePath, privateKeyPath, caddyStorageRoot string, timeout, pollInterval time.Duration) (DatabaseGatewayTLSMaterial, error) {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		material, err := LoadDatabaseGatewayTLSMaterial(hostname, certificatePath, privateKeyPath, caddyStorageRoot, time.Now().UTC())
		if err == nil {
			return material, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return DatabaseGatewayTLSMaterial{}, ctx.Err()
		case <-deadline.C:
			return DatabaseGatewayTLSMaterial{}, ErrCode("database_gateway_tls_unavailable", fmt.Errorf("gateway certificate did not become available before timeout: %w", lastErr))
		case <-ticker.C:
		}
	}
}

func ensurePostgreSQLGatewayDataPlane(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, routeNotifier RouteNotifier, includeConnections, excludeCredentials map[string]bool) error {
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if err != nil {
		return ErrCode("database_gateway_not_found", err)
	}
	if err := ValidatePgBouncerImage(endpoint.ImageRef, endpoint.ImageVersion); err != nil {
		return ErrCode("database_gateway_image_unavailable", err)
	}
	expectedIPv4, _, _ := dnsops.ResolveExpectedIPv4(ctx, cfg)
	if expectedIPv4 == "" {
		return ErrCode("database_gateway_dns_mismatch", errors.New("expected public IPv4 is unavailable"))
	}
	dnsTimeout := time.Duration(cfg.DNSDetectTimeoutMS) * time.Millisecond
	if dnsTimeout <= 0 {
		dnsTimeout = 2500 * time.Millisecond
	}
	if status, _ := dnsops.CheckRegistrarARecord(ctx, endpoint.Hostname, expectedIPv4, dnsTimeout); status != "ok" {
		return ErrCode("database_gateway_dns_mismatch", errors.New("reserved gateway hostname does not resolve to this VPS"))
	}
	if strings.TrimSpace(cfg.CaddyRootConfig) == "" {
		return ErrCode("database_gateway_tls_unavailable", errors.New("Caddy root configuration is required"))
	}
	// The reconciler already includes this hostname as a certificate-only
	// site whenever the gateway endpoint is active (ADR-0002 §6, §5.1) --
	// Notify only has to ask it to converge, not render anything itself.
	// waitForDatabaseGatewayTLSMaterial below already polls disk for up to
	// 90s, which is what actually waits for the reconcile-and-issue round
	// trip to finish; it is not new latency this change introduces.
	if routeNotifier != nil {
		routeNotifier.Notify()
	}
	tlsMaterial, err := waitForDatabaseGatewayTLSMaterial(ctx, endpoint.Hostname, cfg.PostgreSQLGatewayCertificateFile, cfg.PostgreSQLGatewayKeyFile, cfg.CaddyStorageRoot, 90*time.Second, time.Second)
	if err != nil {
		return err
	}
	generationNumber := endpoint.DesiredConfigGeneration
	if generationNumber <= endpoint.RenderedConfigGeneration {
		generationNumber = endpoint.RenderedConfigGeneration + 1
	}
	if generationNumber <= endpoint.AppliedConfigGeneration {
		generationNumber = endpoint.AppliedConfigGeneration + 1
	}
	if generationNumber < 1 {
		generationNumber = 1
	}
	renderRequest, activeRoutes, err := buildPostgreSQLGatewayRenderRequest(ctx, store, sealer, endpoint.Hostname, generationNumber, includeConnections, excludeCredentials)
	if err != nil {
		return err
	}
	adapter := NewPostgreSQLGatewayAdapter(nil)
	generation, err := adapter.Render(ctx, renderRequest)
	if err != nil {
		return ErrCode("database_gateway_config_render_failed", err)
	}
	if err := adapter.Validate(ctx, generation); err != nil {
		return ErrCode("database_gateway_config_validation_failed", err)
	}
	gatewayRoot := filepath.Join(cfg.DataDir, "database-gateways", "postgresql")
	if _, err := WriteInactiveDatabaseGatewayGeneration(gatewayRoot, generation, tlsMaterial.CertificatePEM, tlsMaterial.PrivateKeyPEM); err != nil {
		return ErrCode("database_gateway_config_activation_failed", err)
	}
	previousGeneration, err := ActivateDatabaseGatewayGeneration(gatewayRoot, generation.Generation)
	if err != nil {
		return ErrCode("database_gateway_config_activation_failed", err)
	}
	activationCommitted := false
	containerID := endpoint.DockerContainerID
	newContainer := false
	defer func() {
		if activationCommitted {
			return
		}
		_ = RestoreDatabaseGatewayGeneration(gatewayRoot, previousGeneration)
		if newContainer && containerID != "" {
			_ = docker.RemoveManagedDatabaseGateway(context.WithoutCancel(ctx), client, containerID, endpoint.Engine)
			return
		}
		if containerID != "" && previousGeneration != "" {
			rollbackRuntime := &DockerPostgreSQLGatewayRuntime{Client: client, GatewayContainerID: containerID}
			_ = rollbackRuntime.ReloadPgBouncer(context.WithoutCancel(ctx))
		}
	}()
	if _, err := docker.EnsureDatabaseGatewayIngressNetwork(ctx, client, endpoint.Engine, endpoint.IngressNetworkName); err != nil {
		return ErrCode("database_gateway_network_failed", err)
	}
	if containerID == "" {
		if err := docker.PullImage(ctx, client, endpoint.ImageRef); err != nil {
			return ErrCode("database_gateway_image_pull_failed", err)
		}
		if err := requireDatabaseGatewayPortAvailable(); err != nil {
			return ErrCode("database_gateway_port_occupied", err)
		}
		containerID, err = docker.RunManagedDatabaseGateway(ctx, client, docker.ManagedDatabaseGatewayOptions{Engine: endpoint.Engine, ImageRef: endpoint.ImageRef, ContainerName: endpoint.ContainerName, IngressNetwork: endpoint.IngressNetworkName, ConfigDir: gatewayRoot, HostPort: endpoint.Port})
		if err != nil {
			return ErrCode("database_gateway_container_failed", err)
		}
		newContainer = true
	} else if inspection, inspectErr := docker.InspectManagedContainer(ctx, client, containerID); inspectErr != nil || inspection.Labels[docker.ResourceTypeLabel] != "database-gateway-container" || inspection.Labels[docker.GatewayEngineLabel] != endpoint.Engine {
		return ErrCode("database_gateway_container_drift", errors.New("gateway container is missing or ownership does not match"))
	}
	if err := ValidatePostgreSQLGatewayContainerImage(ctx, client, containerID, endpoint.ImageVersion); err != nil {
		return ErrCode("database_gateway_image_unavailable", err)
	}
	if err := docker.ValidateDatabaseGatewayNetworkMembership(ctx, client, endpoint.IngressNetworkName, "database-gateway-ingress", containerID); err != nil {
		return ErrCode("database_gateway_network_drift", err)
	}
	for _, route := range activeRoutes {
		instance, err := store.GetDatabaseInstance(ctx, route.DatabaseInstanceID)
		if err != nil || instance.DockerContainerID == "" || instance.Status != "healthy" {
			return ErrCode("database_gateway_backend_unhealthy", errors.New("target database instance is not healthy"))
		}
		if _, err := docker.EnsureDatabaseGatewayLinkNetwork(ctx, client, route.Engine, route.ID, route.DatabaseInstanceID, route.LinkNetworkName); err != nil {
			return ErrCode("database_gateway_network_failed", err)
		}
		if err := docker.ConnectDatabaseGatewayLink(ctx, client, route.LinkNetworkName, containerID, nil); err != nil {
			return ErrCode("database_gateway_network_failed", err)
		}
		if err := docker.ConnectDatabaseGatewayLink(ctx, client, route.LinkNetworkName, instance.DockerContainerID, []string{route.BackendAlias}); err != nil {
			return ErrCode("database_gateway_network_failed", err)
		}
		if err := docker.ValidateDatabaseGatewayNetworkMembership(ctx, client, route.LinkNetworkName, "database-gateway-link", containerID, instance.DockerContainerID); err != nil {
			return ErrCode("database_gateway_network_drift", err)
		}
		_ = store.SetDatabaseGatewayRouteState(ctx, route.ID, "active", "active", "", "")
	}
	runtime := &DockerPostgreSQLGatewayRuntime{Client: client, GatewayContainerID: containerID}
	adapter = NewPostgreSQLGatewayAdapter(runtime)
	var reloadErr error
	for attempt := 0; attempt < 20; attempt++ {
		reloadErr = adapter.Reload(ctx, generation)
		if reloadErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if reloadErr != nil {
		return ErrCode("database_gateway_reload_failed", reloadErr)
	}
	if err := store.SetDatabaseGatewayCertificate(ctx, endpoint.Engine, tlsMaterial.Fingerprint, tlsMaterial.NotAfter, time.Now().UTC()); err != nil {
		return err
	}
	if err := store.SetDatabaseGatewayEndpointState(ctx, endpoint.Engine, "active", "active", containerID, generationNumber, generationNumber, generationNumber, "", ""); err != nil {
		return err
	}
	activationCommitted = true
	return nil
}

func buildPostgreSQLGatewayRenderRequest(ctx context.Context, store *repository.Store, sealer *envcrypt.Sealer, hostname string, generation int, includeConnections, excludeCredentials map[string]bool) (GatewayRenderRequest, []repository.DatabaseGatewayRoute, error) {
	routes, err := store.ListDatabaseGatewayRoutes(ctx, "postgresql")
	if err != nil {
		return GatewayRenderRequest{}, nil, err
	}
	request := GatewayRenderRequest{Hostname: hostname, Generation: generation}
	activeRoutes := []repository.DatabaseGatewayRoute{}
	for _, route := range routes {
		if route.DesiredStatus != "active" {
			continue
		}
		instance, err := store.GetDatabaseInstance(ctx, route.DatabaseInstanceID)
		if err != nil || instance.DesiredState != "running" || instance.Status != "healthy" {
			continue
		}
		internalCredential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
		if err != nil {
			return GatewayRenderRequest{}, nil, err
		}
		renderRoute := GatewayRenderRoute{RouteAlias: route.RouteAlias, BackendAlias: route.BackendAlias, BackendPort: instance.InternalPort, DatabaseName: internalCredential.DatabaseName, RoutePoolSize: route.RouteBackendLimit, DesiredStatus: "active", DatabaseRouteID: route.ID}
		connections, err := store.ListDatabaseExternalConnections(ctx, route.ID)
		if err != nil {
			return GatewayRenderRequest{}, nil, err
		}
		for _, connection := range connections {
			included := connection.Status == "active" || connection.Status == "rotating" || (includeConnections != nil && includeConnections[connection.ID])
			if !included {
				continue
			}
			credentials, err := store.ListDatabaseExternalCredentials(ctx, connection.ID, true)
			if err != nil {
				return GatewayRenderRequest{}, nil, err
			}
			for _, credential := range credentials {
				if credential.State == "revoked" || (excludeCredentials != nil && excludeCredentials[credential.ID]) {
					continue
				}
				verifier, err := sealer.Open(credential.SCRAMVerifierCT)
				if err != nil {
					return GatewayRenderRequest{}, nil, errors.New("gateway SCRAM verifier could not be decrypted")
				}
				renderRoute.Credentials = append(renderRoute.Credentials, GatewayRenderCredential{RoleName: credential.RoleName, SCRAMVerifier: string(verifier), CIDRs: connection.CIDRs, BackendPoolSize: route.CredentialBackendLimit, ClientLimit: connection.ClientConnectionLimit, ConnectionID: connection.ID, CredentialID: credential.ID})
				for index := range verifier {
					verifier[index] = 0
				}
			}
		}
		if len(renderRoute.Credentials) > 0 {
			request.Routes = append(request.Routes, renderRoute)
			activeRoutes = append(activeRoutes, route)
		}
	}
	return request, activeRoutes, nil
}

func processPostgreSQLExternalConnectionOperation(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, operation repository.DatabaseGatewayOperation, routeNotifier RouteNotifier) error {
	connection, err := store.GetDatabaseExternalConnection(ctx, operation.ConnectionID)
	if err != nil {
		return err
	}
	route, err := store.GetDatabaseGatewayRoute(ctx, connection.RouteID)
	if err != nil {
		return err
	}
	instance, err := store.GetDatabaseInstance(ctx, route.DatabaseInstanceID)
	if err != nil {
		return err
	}
	internalCredential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		return err
	}
	adminPassword, err := sealer.Open(internalCredential.AdminPasswordCT)
	if err != nil {
		return errors.New("PostgreSQL administrator credential could not be decrypted")
	}
	defer zeroBytes(adminPassword)
	include := map[string]bool{connection.ID: true}
	previousCredential := currentExternalCredential(connection)
	needsNewCredential := operation.OperationType == "create_connection" || operation.OperationType == "rotate_connection" || (operation.OperationType == "enable_connection" && previousCredential == nil)
	if needsNewCredential {
		identity, err := store.NewDatabaseExternalCredentialIdentity(ctx, connection.ID)
		if err != nil {
			return err
		}
		password, err := GenerateDatabaseGatewayPassword()
		if err != nil {
			return err
		}
		passwordBytes := []byte(password)
		defer zeroBytes(passwordBytes)
		runtime := &DockerPostgreSQLGatewayRuntime{Client: client}
		adapter := NewPostgreSQLGatewayAdapter(runtime)
		newCredentialCommitted := false
		rotationCompleted := operation.OperationType != "rotate_connection"
		var createdCredential *repository.DatabaseExternalCredential
		defer func() {
			if createdCredential == nil {
				if !rotationCompleted {
					_ = store.SetDatabaseExternalConnectionStatus(context.WithoutCancel(ctx), connection.ID, "active", "", "")
				}
				return
			}
			if newCredentialCommitted {
				return
			}
			rollbackCtx := context.WithoutCancel(ctx)
			if operation.OperationType == "rotate_connection" && previousCredential != nil {
				_ = store.RollbackDatabaseExternalCredentialRotation(rollbackCtx, connection.ID, createdCredential.ID, previousCredential.Generation)
			} else {
				_ = store.RevokeDatabaseExternalCredential(rollbackCtx, createdCredential.ID)
			}
			_ = adapter.RevokeRole(rollbackCtx, GatewayRevokeRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: createdCredential.RoleName})
			_ = ensurePostgreSQLGatewayDataPlane(rollbackCtx, log, cfg, store, sealer, client, routeNotifier, nil, nil)
		}()
		material, err := adapter.ProvisionRole(ctx, GatewayRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: identity.RoleName, Password: password, PermissionProfile: connection.PermissionProfile})
		if err != nil {
			return ErrCode("database_gateway_role_provision_failed", err)
		}
		passwordCT, err := sealer.Seal(passwordBytes)
		if err != nil {
			return err
		}
		verifierCT, err := sealer.Seal([]byte(material.SCRAMVerifier))
		if err != nil {
			return err
		}
		credential, err := store.CreateDatabaseExternalCredentialWithIdentity(ctx, identity, passwordCT, verifierCT)
		if err != nil {
			_ = adapter.RevokeRole(context.WithoutCancel(ctx), GatewayRevokeRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: identity.RoleName})
			return err
		}
		createdCredential = &credential
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, include, nil); err != nil {
			return err
		}
		endpoint, _ := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
		runtime.GatewayContainerID = endpoint.DockerContainerID
		adapter = NewPostgreSQLGatewayAdapter(runtime)
		if err := adapter.Probe(ctx, GatewayProbeRequest{Hostname: endpoint.Hostname, Port: endpoint.Port, Alias: route.RouteAlias, Username: credential.RoleName, Password: password}); err != nil {
			return ErrCode("database_gateway_probe_failed", err)
		}
		previousID := ""
		if previousCredential != nil {
			previousID = previousCredential.ID
		}
		deadline := time.Now().UTC().Add(time.Duration(operation.RequestedGracePeriodHours) * time.Hour)
		if err := store.CompleteDatabaseExternalCredentialRotation(ctx, connection.ID, previousID, deadline); err != nil {
			return err
		}
		newCredentialCommitted = true
		rotationCompleted = true
		if previousID != "" && operation.RequestedGracePeriodHours == 0 {
			return retirePostgreSQLExternalCredential(ctx, log, cfg, store, sealer, client, connection, route, instance, internalCredential, adminPassword, *previousCredential, routeNotifier)
		}
		return nil
	}
	credentials, err := store.ListDatabaseExternalCredentials(ctx, connection.ID, true)
	if err != nil {
		return err
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if err != nil {
		return err
	}
	runtime := &DockerPostgreSQLGatewayRuntime{Client: client, GatewayContainerID: endpoint.DockerContainerID}
	adapter := NewPostgreSQLGatewayAdapter(runtime)
	switch operation.OperationType {
	case "update_connection", "enable_connection":
		roles := externalCredentialRoles(credentials)
		// Permission and CIDR changes fail closed: deny backend logins and
		// terminate PostgreSQL sessions before activating the new proxy config.
		if err := disablePostgreSQLRoles(ctx, runtime, instance, internalCredential, adminPassword, roles); err != nil {
			return err
		}
		for _, credential := range credentials {
			if credential.State == "revoked" {
				continue
			}
			if err := adapter.ReconcilePermissions(ctx, GatewayPermissionRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: credential.RoleName, PermissionProfile: connection.PermissionProfile}); err != nil {
				return err
			}
		}
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, include, nil); err != nil {
			return err
		}
		if err := adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: roles}); err != nil {
			return err
		}
		for _, credential := range credentials {
			if credential.State == "revoked" {
				continue
			}
			if _, err := runtime.RunSQL(ctx, instance.DockerContainerID, internalCredential.DatabaseName, string(adminPassword), "ALTER ROLE "+quotePostgresIdentifier(credential.RoleName)+" LOGIN;", nil); err != nil {
				return err
			}
		}
		var currentCredential *repository.DatabaseExternalCredential
		for index := range credentials {
			if credentials[index].Generation == connection.CurrentGeneration && credentials[index].State == "active" {
				currentCredential = &credentials[index]
				break
			}
		}
		if currentCredential == nil {
			return errors.New("current external credential is unavailable")
		}
		password, err := sealer.Open(currentCredential.PasswordCT)
		if err != nil {
			return errors.New("current external credential could not be decrypted")
		}
		defer zeroBytes(password)
		endpoint, err = store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
		if err != nil {
			return err
		}
		runtime.GatewayContainerID = endpoint.DockerContainerID
		adapter = NewPostgreSQLGatewayAdapter(runtime)
		if err := adapter.Probe(ctx, GatewayProbeRequest{Hostname: endpoint.Hostname, Port: endpoint.Port, Alias: route.RouteAlias, Username: currentCredential.RoleName, Password: string(password)}); err != nil {
			return ErrCode("database_gateway_probe_failed", err)
		}
		return store.SetDatabaseExternalConnectionStatus(ctx, connection.ID, "active", "", "")
	case "disable_connection", "expire_connection":
		roles := externalCredentialRoles(credentials)
		// Deny backend logins and terminate PostgreSQL sessions before touching
		// DNS, TLS, or gateway config so revocation fails closed when those
		// dependencies are unavailable.
		if err := disablePostgreSQLRoles(ctx, runtime, instance, internalCredential, adminPassword, roles); err != nil {
			return err
		}
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil {
			return err
		}
		if err := adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: roles}); err != nil {
			return err
		}
		return nil
	case "revoke_connection":
		roles := externalCredentialRoles(credentials)
		if err := disablePostgreSQLRoles(ctx, runtime, instance, internalCredential, adminPassword, roles); err != nil {
			return err
		}
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil {
			return err
		}
		if err := adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: roles}); err != nil {
			return err
		}
		for _, credential := range credentials {
			if credential.State == "revoked" {
				continue
			}
			if err := adapter.RevokeRole(ctx, GatewayRevokeRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: credential.RoleName}); err != nil {
				return err
			}
		}
		if err := store.FinalizeDatabaseExternalConnectionRevocation(ctx, connection.ID); err != nil {
			return err
		}
		return cleanupUnusedPostgreSQLGatewayRoute(ctx, store, client, route)
	case "retire_credential":
		credential, err := store.GetDatabaseExternalCredentialSealed(ctx, operation.CredentialID)
		if err != nil {
			return err
		}
		return retirePostgreSQLExternalCredential(ctx, log, cfg, store, sealer, client, connection, route, instance, internalCredential, adminPassword, credential, routeNotifier)
	default:
		return ErrCode("database_gateway_operation_not_implemented", fmt.Errorf("operation type %s", operation.OperationType))
	}
}

func currentExternalCredential(connection repository.DatabaseExternalConnection) *repository.DatabaseExternalCredential {
	for index := range connection.Credentials {
		if connection.Credentials[index].Generation == connection.CurrentGeneration && connection.Credentials[index].State == "active" {
			return &connection.Credentials[index]
		}
	}
	return nil
}

func retirePostgreSQLExternalCredential(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, connection repository.DatabaseExternalConnection, route repository.DatabaseGatewayRoute, instance repository.DatabaseInstance, internalCredential repository.DatabaseCredential, adminPassword []byte, credential repository.DatabaseExternalCredential, routeNotifier RouteNotifier) error {
	runtime := &DockerPostgreSQLGatewayRuntime{Client: client}
	if err := disablePostgreSQLRoles(ctx, runtime, instance, internalCredential, adminPassword, []string{credential.RoleName}); err != nil {
		return err
	}
	if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, map[string]bool{connection.ID: true}, map[string]bool{credential.ID: true}); err != nil {
		return err
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, "postgresql")
	if err != nil {
		return err
	}
	runtime.GatewayContainerID = endpoint.DockerContainerID
	adapter := NewPostgreSQLGatewayAdapter(runtime)
	if err := adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: []string{credential.RoleName}}); err != nil {
		return err
	}
	if err := adapter.RevokeRole(ctx, GatewayRevokeRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: credential.RoleName}); err != nil {
		return err
	}
	return store.RevokeDatabaseExternalCredential(ctx, credential.ID)
}

func disablePostgreSQLRoles(ctx context.Context, runtime *DockerPostgreSQLGatewayRuntime, instance repository.DatabaseInstance, internalCredential repository.DatabaseCredential, adminPassword []byte, roles []string) error {
	for _, role := range roles {
		script := "ALTER ROLE " + quotePostgresIdentifier(role) + " NOLOGIN; SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=" + quotePostgresLiteral(role) + " AND pid<>pg_backend_pid();"
		if _, err := runtime.RunSQL(ctx, instance.DockerContainerID, internalCredential.DatabaseName, string(adminPassword), script, nil); err != nil {
			return err
		}
	}
	return nil
}

func externalCredentialRoles(credentials []repository.DatabaseExternalCredential) []string {
	roles := []string{}
	for _, credential := range credentials {
		if credential.State != "revoked" {
			roles = append(roles, credential.RoleName)
		}
	}
	return roles
}

func cleanupUnusedPostgreSQLGatewayRoute(ctx context.Context, store *repository.Store, client *mobyclient.Client, route repository.DatabaseGatewayRoute) error {
	connections, err := store.ListDatabaseExternalConnections(ctx, route.ID)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Status != "revoked" {
			return nil
		}
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, route.Engine)
	if err != nil {
		return err
	}
	instance, _ := store.GetDatabaseInstance(ctx, route.DatabaseInstanceID)
	if endpoint.DockerContainerID != "" {
		if err := docker.DisconnectDatabaseGatewayLink(ctx, client, route.LinkNetworkName, endpoint.DockerContainerID); err != nil {
			return err
		}
	}
	if instance.DockerContainerID != "" {
		if err := docker.DisconnectDatabaseGatewayLink(ctx, client, route.LinkNetworkName, instance.DockerContainerID); err != nil {
			return err
		}
	}
	removed, err := docker.RemoveDatabaseGatewayLinkNetworkIfEmpty(ctx, client, route.LinkNetworkName, route.ID)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("database gateway link network still has attached containers")
	}
	return store.SetDatabaseGatewayRouteState(ctx, route.ID, "disabled", "disabled", "", "")
}

// revokePostgreSQLGatewayRouteForDeletion removes every public grant before a
// retained database container is removed. Role denial and removal happen first,
// so stale proxy configuration cannot preserve database access if reconciliation
// is temporarily blocked by DNS or certificate availability.
func revokePostgreSQLGatewayRouteForDeletion(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, instance repository.DatabaseInstance, routeNotifier RouteNotifier) error {
	route, err := store.GetDatabaseGatewayRouteByInstance(ctx, instance.ID)
	if errors.Is(err, repository.ErrDatabaseGatewayRouteNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	internalCredential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		return err
	}
	adminPassword, err := sealer.Open(internalCredential.AdminPasswordCT)
	if err != nil {
		return errors.New("PostgreSQL administrator credential could not be decrypted")
	}
	defer zeroBytes(adminPassword)
	connections, err := store.ListDatabaseExternalConnections(ctx, route.ID)
	if err != nil {
		return err
	}
	endpoint, endpointErr := store.GetDatabaseGatewayEndpoint(ctx, route.Engine)
	runtime := &DockerPostgreSQLGatewayRuntime{Client: client}
	if endpointErr == nil {
		runtime.GatewayContainerID = endpoint.DockerContainerID
	}
	adapter := NewPostgreSQLGatewayAdapter(runtime)
	if err := store.SetDatabaseGatewayRouteState(ctx, route.ID, "disabled", "disabled", "", ""); err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Status == "revoked" {
			continue
		}
		if err := store.SetDatabaseExternalConnectionStatus(ctx, connection.ID, "revoking", "", ""); err != nil {
			return err
		}
		credentials, err := store.ListDatabaseExternalCredentials(ctx, connection.ID, true)
		if err != nil {
			return err
		}
		roles := externalCredentialRoles(credentials)
		if err := disablePostgreSQLRoles(ctx, runtime, instance, internalCredential, adminPassword, roles); err != nil {
			return err
		}
		if runtime.GatewayContainerID != "" {
			_ = adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: roles, RouteAlias: route.RouteAlias})
		}
		for _, credential := range credentials {
			if credential.State == "revoked" {
				continue
			}
			if err := adapter.RevokeRole(ctx, GatewayRevokeRoleRequest{ContainerID: instance.DockerContainerID, DatabaseName: internalCredential.DatabaseName, ApplicationOwnerRole: internalCredential.Username, AdminPassword: string(adminPassword), RoleName: credential.RoleName}); err != nil {
				return err
			}
		}
		if err := store.FinalizeDatabaseExternalConnectionRevocation(ctx, connection.ID); err != nil {
			return err
		}
	}
	if endpointErr == nil {
		if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil && log != nil {
			log.Warn("gateway config reconciliation deferred after fail-closed database deletion revocation", "instance_id", instance.ID, "error", err)
		}
	}
	return cleanupUnusedPostgreSQLGatewayRoute(ctx, store, client, route)
}

func pausePostgreSQLGatewayRoute(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, instance repository.DatabaseInstance, routeNotifier RouteNotifier) error {
	route, err := store.GetDatabaseGatewayRouteByInstance(ctx, instance.ID)
	if errors.Is(err, repository.ErrDatabaseGatewayRouteNotFound) {
		return nil
	}
	if err != nil || route.DesiredStatus == "disabled" {
		return err
	}
	connections, err := store.ListDatabaseExternalConnections(ctx, route.ID)
	if err != nil {
		return err
	}
	roles := []string{}
	for _, connection := range connections {
		credentials, err := store.ListDatabaseExternalCredentials(ctx, connection.ID, false)
		if err != nil {
			return err
		}
		roles = append(roles, externalCredentialRoles(credentials)...)
	}
	if err := store.SetDatabaseGatewayRouteState(ctx, route.ID, "disabled", "disabled", "", ""); err != nil {
		return err
	}
	if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil {
		_ = store.SetDatabaseGatewayRouteState(context.WithoutCancel(ctx), route.ID, "active", "failed", "database_gateway_route_pause_failed", err.Error())
		return err
	}
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, route.Engine)
	if err != nil {
		return err
	}
	runtime := &DockerPostgreSQLGatewayRuntime{Client: client, GatewayContainerID: endpoint.DockerContainerID}
	adapter := NewPostgreSQLGatewayAdapter(runtime)
	if err := adapter.Terminate(ctx, GatewayTerminationRequest{RoleNames: roles, RouteAlias: route.RouteAlias}); err != nil {
		return err
	}
	internalCredential, err := store.GetDatabaseCredentialSealed(ctx, instance.ID)
	if err != nil {
		return err
	}
	adminPassword, err := sealer.Open(internalCredential.AdminPasswordCT)
	if err != nil {
		return err
	}
	defer zeroBytes(adminPassword)
	for _, role := range roles {
		script := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename=" + quotePostgresLiteral(role) + " AND pid<>pg_backend_pid();"
		if _, err := runtime.RunSQL(ctx, instance.DockerContainerID, internalCredential.DatabaseName, string(adminPassword), script, nil); err != nil {
			return err
		}
	}
	return nil
}

func resumePostgreSQLGatewayRoute(ctx context.Context, log *slog.Logger, cfg *config.Config, store *repository.Store, sealer *envcrypt.Sealer, client *mobyclient.Client, instanceID string, routeNotifier RouteNotifier) error {
	route, err := store.GetDatabaseGatewayRouteByInstance(ctx, instanceID)
	if errors.Is(err, repository.ErrDatabaseGatewayRouteNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	connections, err := store.ListDatabaseExternalConnections(ctx, route.ID)
	if err != nil {
		return err
	}
	hasUsableConnection := false
	for _, connection := range connections {
		if connection.Status == "active" || connection.Status == "rotating" {
			hasUsableConnection = true
			break
		}
	}
	if !hasUsableConnection {
		return nil
	}
	if err := store.SetDatabaseGatewayRouteState(ctx, route.ID, "active", "pending", "", ""); err != nil {
		return err
	}
	if err := ensurePostgreSQLGatewayDataPlane(ctx, log, cfg, store, sealer, client, routeNotifier, nil, nil); err != nil {
		_ = store.SetDatabaseGatewayRouteState(context.WithoutCancel(ctx), route.ID, "disabled", "failed", "database_gateway_route_resume_failed", err.Error())
		return err
	}
	return nil
}

func teardownPostgreSQLGateway(ctx context.Context, store *repository.Store, client *mobyclient.Client, engine string) error {
	endpoint, err := store.GetDatabaseGatewayEndpoint(ctx, engine)
	if err != nil {
		return err
	}
	if endpoint.DockerContainerID != "" {
		if err := docker.RemoveManagedDatabaseGateway(ctx, client, endpoint.DockerContainerID, engine); err != nil {
			return err
		}
	}
	if _, err := docker.RemoveDatabaseGatewayIngressNetworkIfEmpty(ctx, client, endpoint.IngressNetworkName, engine); err != nil {
		return err
	}
	return store.ClearDatabaseGatewayEndpointRuntime(ctx, engine)
}

func requireDatabaseGatewayPortAvailable() error {
	return requireDatabaseGatewayAddressAvailable(":5432")
}

func requireDatabaseGatewayAddressAvailable(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("TCP address %s is already occupied", address)
	}
	return listener.Close()
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
