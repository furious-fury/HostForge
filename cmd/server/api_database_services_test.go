package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hostforge/hostforge/internal/databases"
	"github.com/hostforge/hostforge/internal/repository"
)

func TestCreatePostgreSQLServiceQueuesIsolatedProvisioning(t *testing.T) {
	s := newAPITestServer(t)
	app, err := s.store.CreateApplication(context.Background(), "Database API", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(context.Background(), app.ID)
	if err != nil || len(environments) != 2 {
		t.Fatalf("environments=%d err=%v", len(environments), err)
	}
	body, err := json.Marshal(map[string]any{
		"name": "primary", "engine": "postgresql", "version": "18",
		"environment_ids": []string{environments[0].ID, environments[1].ID},
		"resource_preset": "development", "connections": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost,
		"/api/applications/"+app.ID+"/database-services", strings.NewReader(string(body))))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeResponse(t, recorder)
	service := payload["service"].(map[string]any)
	if service["service_type"] != "database" || service["repo_url"] != "" {
		t.Fatalf("unexpected service identity: %+v", service)
	}
	instances := payload["instances"].([]any)
	operations := payload["operations"].([]any)
	if len(instances) != 2 || len(operations) != 2 {
		t.Fatalf("instances=%d operations=%d", len(instances), len(operations))
	}
	operationID := operations[0].(map[string]any)["id"].(string)
	operationRecorder := httptest.NewRecorder()
	s.handleDatabaseOperations(operationRecorder, httptest.NewRequest(http.MethodGet,
		"/api/database-operations/"+operationID, nil))
	if operationRecorder.Code != http.StatusOK {
		t.Fatalf("operation status=%d body=%s", operationRecorder.Code, operationRecorder.Body.String())
	}
	operation := decodeResponse(t, operationRecorder)["operation"].(map[string]any)
	if operation["status"] != "queued" || operation["operation_type"] != "provision" {
		t.Fatalf("unexpected operation: %+v", operation)
	}
	instanceID := instances[0].(map[string]any)["id"].(string)
	credential, err := s.store.GetDatabaseCredentialSealed(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	password, err := s.envSealer.Open(credential.PasswordCT)
	if err != nil {
		t.Fatal(err)
	}
	detailRecorder := httptest.NewRecorder()
	s.handleServices(detailRecorder, httptest.NewRequest(http.MethodGet, "/api/services/"+service["id"].(string), nil))
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}
	detailBody := detailRecorder.Body.String()
	if !strings.Contains(detailBody, credential.DatabaseName) || !strings.Contains(detailBody, credential.Username) {
		t.Fatalf("database connection metadata missing from service detail: %s", detailBody)
	}
	for _, forbidden := range []string{string(password), "password_ct", "admin_password_ct", "lease_owner", "lease_expires_at"} {
		if strings.Contains(detailBody, forbidden) {
			t.Fatalf("database service detail exposed %q: %s", forbidden, detailBody)
		}
	}
}

func TestDatabaseCreationRejectsVersionOutsidePinnedCatalog(t *testing.T) {
	s := newAPITestServer(t)
	app, _ := s.store.CreateApplication(context.Background(), "Database API", "")
	environments, _ := s.store.ListApplicationEnvironments(context.Background(), app.ID)
	body := `{"name":"cache","engine":"redis","version":"latest","environment_ids":["` + environments[0].ID + `"],"resource_preset":"development","connections":[]}`
	recorder := httptest.NewRecorder()
	s.handleApplications(recorder, httptest.NewRequest(http.MethodPost,
		"/api/applications/"+app.ID+"/database-services", strings.NewReader(body)))
	assertAPIError(t, recorder, http.StatusUnprocessableEntity, "database_version_not_supported")
}

func TestRestoreDeletedDatabaseQueuesRetainedInstances(t *testing.T) {
	s := newAPITestServer(t)
	app, _ := s.store.CreateApplication(context.Background(), "Restore API", "")
	environments, _ := s.store.ListApplicationEnvironments(context.Background(), app.ID)
	body := `{"name":"primary","engine":"postgresql","version":"18","environment_ids":["` + environments[0].ID + `"],"resource_preset":"development","connections":[]}`
	createRecorder := httptest.NewRecorder()
	s.handleApplications(createRecorder, httptest.NewRequest(http.MethodPost,
		"/api/applications/"+app.ID+"/database-services", strings.NewReader(body)))
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	serviceID := decodeResponse(t, createRecorder)["service"].(map[string]any)["id"].(string)
	detailRecorder := httptest.NewRecorder()
	s.handleDatabaseServices(detailRecorder, httptest.NewRequest(http.MethodGet, "/api/database-services/"+serviceID, nil))
	if detailRecorder.Code != http.StatusOK || decodeResponse(t, detailRecorder)["database"].(map[string]any)["engine"] != "postgresql" {
		t.Fatalf("database-specific detail endpoint failed: status=%d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}
	if _, err := s.store.MarkDatabaseServiceDeleted(context.Background(), serviceID, 7*24*time.Hour, "operator"); err != nil {
		t.Fatal(err)
	}
	purgeRecorder := httptest.NewRecorder()
	s.handleDatabaseServices(purgeRecorder, httptest.NewRequest(http.MethodDelete, "/api/database-services/"+serviceID+"/purge", strings.NewReader(`{"confirmation":"primary"}`)))
	if purgeRecorder.Code != http.StatusConflict {
		t.Fatalf("retained database purged before deadline: status=%d body=%s", purgeRecorder.Code, purgeRecorder.Body.String())
	}

	restoreRecorder := httptest.NewRecorder()
	s.handleDatabaseServices(restoreRecorder, httptest.NewRequest(http.MethodPost,
		"/api/database-services/"+serviceID+"/restore-deleted", nil))
	if restoreRecorder.Code != http.StatusAccepted {
		t.Fatalf("restore status=%d body=%s", restoreRecorder.Code, restoreRecorder.Body.String())
	}
	operations := decodeResponse(t, restoreRecorder)["operations"].([]any)
	if len(operations) != 1 || operations[0].(map[string]any)["operation_type"] != "restore_deleted" {
		t.Fatalf("unexpected restore operations: %+v", operations)
	}
	instance := decodeResponse(t, createRecorder)["instances"].([]any)[0].(map[string]any)
	restored, err := s.store.GetDatabaseInstance(context.Background(), instance["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != "provisioning" || !restored.DeletedAt.IsZero() || !restored.PurgeAfter.IsZero() {
		t.Fatalf("instance was not restored to provisioning: %+v", restored)
	}
}

func TestDatabaseRuntimeActionQueuesWithoutRunningDockerInline(t *testing.T) {
	s := newAPITestServer(t)
	app, _ := s.store.CreateApplication(context.Background(), "Runtime API", "")
	environments, _ := s.store.ListApplicationEnvironments(context.Background(), app.ID)
	body := `{"name":"primary","engine":"postgresql","version":"18","environment_ids":["` + environments[0].ID + `"],"resource_preset":"development","connections":[]}`
	createRecorder := httptest.NewRecorder()
	s.handleApplications(createRecorder, httptest.NewRequest(http.MethodPost,
		"/api/applications/"+app.ID+"/database-services", strings.NewReader(body)))
	payload := decodeResponse(t, createRecorder)
	instanceID := payload["instances"].([]any)[0].(map[string]any)["id"].(string)
	operations := payload["operations"].([]any)
	provisionOperationID := operations[0].(map[string]any)["id"].(string)
	if _, err := s.store.UpdateDatabaseOperation(context.Background(), provisionOperationID, "success", "ready", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.UpdateDatabaseInstanceState(context.Background(), instanceID, repository.UpdateDatabaseInstanceStateInput{
		DockerContainerID: "managed-container", DesiredState: "running", Status: "healthy",
	}); err != nil {
		t.Fatal(err)
	}

	actionRecorder := httptest.NewRecorder()
	s.handleDatabaseInstances(actionRecorder, httptest.NewRequest(http.MethodPost,
		"/api/database-instances/"+instanceID+"/stop", nil))
	if actionRecorder.Code != http.StatusAccepted {
		t.Fatalf("runtime status=%d body=%s", actionRecorder.Code, actionRecorder.Body.String())
	}
	operation := decodeResponse(t, actionRecorder)["operation"].(map[string]any)
	if operation["operation_type"] != "stop" || operation["status"] != "queued" {
		t.Fatalf("unexpected runtime operation: %+v", operation)
	}
	instance, _ := s.store.GetDatabaseInstance(context.Background(), instanceID)
	if instance.DesiredState != "stopped" || instance.Status != "stopping" {
		t.Fatalf("runtime intent not persisted: %+v", instance)
	}
	if _, err := s.store.UpdateDatabaseOperation(context.Background(), operation["id"].(string), "success", "stopped", 100, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.UpdateDatabaseInstanceState(context.Background(), instanceID, repository.UpdateDatabaseInstanceStateInput{
		DesiredState: "running", Status: "healthy",
	}); err != nil {
		t.Fatal(err)
	}
	rotateRecorder := httptest.NewRecorder()
	s.handleDatabaseInstances(rotateRecorder, httptest.NewRequest(http.MethodPost,
		"/api/database-instances/"+instanceID+"/rotate-credentials", nil))
	if rotateRecorder.Code != http.StatusAccepted {
		t.Fatalf("rotate status=%d body=%s", rotateRecorder.Code, rotateRecorder.Body.String())
	}
	rotation := decodeResponse(t, rotateRecorder)["operation"].(map[string]any)
	if rotation["operation_type"] != "rotate_credentials" {
		t.Fatalf("unexpected rotation operation: %+v", rotation)
	}
}

func TestDatabaseBindingCanBeUpdatedThroughAPI(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	app, _ := s.store.CreateApplication(ctx, "Binding API", "")
	environments, _ := s.store.ListApplicationEnvironments(ctx, app.ID)
	apiService, _ := s.store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "api", RepoURL: "https://github.com/acme/api.git", InternalPort: 3000, HealthCheckPath: "/"})
	workerService, _ := s.store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: app.ID, Name: "worker", RepoURL: "https://github.com/acme/worker.git", InternalPort: 3001, HealthCheckPath: "/"})
	createRecorder := httptest.NewRecorder()
	body := `{"name":"primary","engine":"postgresql","version":"18","environment_ids":["` + environments[0].ID + `"],"resource_preset":"development","connections":[{"service_id":"` + apiService.ID + `","variable_key":"DATABASE_URL"}]}`
	s.handleApplications(createRecorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+app.ID+"/database-services", strings.NewReader(body)))
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	instanceID := decodeResponse(t, createRecorder)["instances"].([]any)[0].(map[string]any)["id"].(string)
	bindings, err := s.store.ListDatabaseBindings(ctx, instanceID)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("initial database binding missing: %+v err=%v", bindings, err)
	}
	bindingID := bindings[0].ID
	patchRecorder := httptest.NewRecorder()
	s.handleDatabaseBindings(patchRecorder, httptest.NewRequest(http.MethodPatch, "/api/database-bindings/"+bindingID, strings.NewReader(`{"consumer_service_id":"`+workerService.ID+`","variable_key":"WORKER_DATABASE_URL"}`)))
	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patchRecorder.Code, patchRecorder.Body.String())
	}
	binding := decodeResponse(t, patchRecorder)["binding"].(map[string]any)
	if binding["consumer_service_id"] != workerService.ID || binding["variable_key"] != "WORKER_DATABASE_URL" {
		t.Fatalf("unexpected updated binding: %+v", binding)
	}
}

func TestDatabaseBackupRestoreDefaultsToNewService(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	app, _ := s.store.CreateApplication(ctx, "Backup Restore API", "")
	environments, _ := s.store.ListApplicationEnvironments(ctx, app.ID)
	createRecorder := httptest.NewRecorder()
	s.handleApplications(createRecorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+app.ID+"/database-services", strings.NewReader(`{"name":"primary","engine":"postgresql","version":"18","environment_ids":["`+environments[0].ID+`"],"resource_preset":"development","connections":[]}`)))
	payload := decodeResponse(t, createRecorder)
	instanceID := payload["instances"].([]any)[0].(map[string]any)["id"].(string)
	provisionID := payload["operations"].([]any)[0].(map[string]any)["id"].(string)
	_, _ = s.store.UpdateDatabaseOperation(ctx, provisionID, "success", "ready", 100, "", "")
	_, _ = s.store.UpdateDatabaseInstanceState(ctx, instanceID, repository.UpdateDatabaseInstanceStateInput{DockerContainerID: "source-container", DesiredState: "running", Status: "healthy"})
	access, _ := s.envSealer.Seal([]byte("access"))
	secret, _ := s.envSealer.Seal([]byte("secret"))
	destination, err := s.store.CreateBackupDestination(ctx, repository.CreateBackupDestinationInput{Name: "R2", Provider: "r2", Endpoint: "https://account.r2.cloudflarestorage.com", Region: "auto", Bucket: "backups", AccessKeyCT: access, SecretKeyCT: secret})
	if err != nil {
		t.Fatal(err)
	}
	backup, operation, err := s.store.QueueDatabaseBackup(ctx, instanceID, destination.ID, "manual", "operator", 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.store.UpdateDatabaseOperation(ctx, operation.ID, "success", "complete", 100, "", "")
	wrapped, _ := s.envSealer.Seal([]byte("01234567890123456789012345678901"))
	_, err = s.store.CompleteDatabaseBackup(ctx, backup.ID, repository.CompleteDatabaseBackupInput{Status: "success", ObjectKey: "backup.hfbk", ArchiveFormat: "postgresql-custom+gzip+aead", Checksum: "checksum", CompressedSize: 10, EncryptionAlgorithm: "AES-256-GCM-CHUNKED", EncryptedDataKey: wrapped})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	s.handleDatabaseBackups(recorder, httptest.NewRequest(http.MethodPost, "/api/database-backups/"+backup.ID+"/restore", strings.NewReader(`{"mode":"new_service"}`)))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("restore status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	restored := decodeResponse(t, recorder)
	if restored["operation"].(map[string]any)["operation_type"] != "restore" || restored["database_service"].(map[string]any)["id"] == payload["service"].(map[string]any)["id"] {
		t.Fatalf("restore did not create an isolated database service: %+v", restored)
	}
}

func TestDatabasePatchUpgradeRequiresAndUsesRecentBackup(t *testing.T) {
	s := newAPITestServer(t)
	ctx := context.Background()
	app, _ := s.store.CreateApplication(ctx, "Upgrade API", "")
	environments, _ := s.store.ListApplicationEnvironments(ctx, app.ID)
	createRecorder := httptest.NewRecorder()
	s.handleApplications(createRecorder, httptest.NewRequest(http.MethodPost, "/api/applications/"+app.ID+"/database-services", strings.NewReader(`{"name":"primary","engine":"postgresql","version":"18","environment_ids":["`+environments[0].ID+`"],"resource_preset":"development","connections":[]}`)))
	payload := decodeResponse(t, createRecorder)
	instanceID := payload["instances"].([]any)[0].(map[string]any)["id"].(string)
	provisionID := payload["operations"].([]any)[0].(map[string]any)["id"].(string)
	_, _ = s.store.UpdateDatabaseOperation(ctx, provisionID, "success", "ready", 100, "", "")
	instance, _ := s.store.GetDatabaseInstance(ctx, instanceID)
	_, _ = s.store.UpdateDatabaseInstanceState(ctx, instanceID, repository.UpdateDatabaseInstanceStateInput{DockerContainerID: "old-container", DesiredState: "running", Status: "healthy"})
	_, version, _ := databases.FindVersion("postgresql", "18")
	oldImage := "postgres@sha256:previous-patch"
	if _, err := s.store.CommitDatabaseInstanceUpgrade(ctx, instanceID, instance.ImageRef, oldImage, "old-container"); err != nil {
		t.Fatal(err)
	}
	preflight := httptest.NewRecorder()
	s.handleDatabaseInstances(preflight, httptest.NewRequest(http.MethodGet, "/api/database-instances/"+instanceID+"/upgrade", nil))
	if preflight.Code != http.StatusOK || decodeResponse(t, preflight)["reason"] != "recent_backup_required" {
		t.Fatalf("upgrade without backup was not blocked: status=%d body=%s", preflight.Code, preflight.Body.String())
	}
	access, _ := s.envSealer.Seal([]byte("access"))
	secret, _ := s.envSealer.Seal([]byte("secret"))
	destination, _ := s.store.CreateBackupDestination(ctx, repository.CreateBackupDestinationInput{Name: "Upgrade backup", Provider: "r2", Endpoint: "https://account.r2.cloudflarestorage.com", Region: "auto", Bucket: "backups", AccessKeyCT: access, SecretKeyCT: secret})
	backup, backupOperation, err := s.store.QueueDatabaseBackup(ctx, instanceID, destination.ID, "manual", "operator", 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.store.UpdateDatabaseOperation(ctx, backupOperation.ID, "success", "complete", 100, "", "")
	wrapped, _ := s.envSealer.Seal([]byte("01234567890123456789012345678901"))
	if _, err := s.store.CompleteDatabaseBackup(ctx, backup.ID, repository.CompleteDatabaseBackupInput{Status: "success", ObjectKey: "upgrade.hfbk", ArchiveFormat: "postgresql-custom+gzip+aead", Checksum: "checksum", CompressedSize: 10, EncryptionAlgorithm: "AES-256-GCM-CHUNKED", EncryptedDataKey: wrapped}); err != nil {
		t.Fatal(err)
	}
	readyRecorder := httptest.NewRecorder()
	s.handleDatabaseInstances(readyRecorder, httptest.NewRequest(http.MethodGet, "/api/database-instances/"+instanceID+"/upgrade", nil))
	ready := decodeResponse(t, readyRecorder)
	if ready["ready"] != true || ready["target_image_ref"] != version.ImageRef {
		t.Fatalf("unexpected upgrade preflight: %s", readyRecorder.Body.String())
	}
	queueRecorder := httptest.NewRecorder()
	s.handleDatabaseInstances(queueRecorder, httptest.NewRequest(http.MethodPost, "/api/database-instances/"+instanceID+"/upgrade", nil))
	if queueRecorder.Code != http.StatusAccepted || decodeResponse(t, queueRecorder)["operation"].(map[string]any)["operation_type"] != "upgrade" {
		t.Fatalf("upgrade was not queued: status=%d body=%s", queueRecorder.Code, queueRecorder.Body.String())
	}
}
