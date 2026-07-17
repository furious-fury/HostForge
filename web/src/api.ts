export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message?: string,
    public fields?: Record<string, string>,
    public details?: Record<string, unknown>,
  ) {
    super(message || code)
    this.name = "APIError"
  }
}

type APIErrorPayload = {
  status?: "error"
  error?: string
  message?: string
  fields?: Record<string, string>
  [key: string]: unknown
}

export const unauthorizedEvent = "hostforge:unauthorized"

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...init.headers,
    },
  })
  const payload = response.status === 204 ? null : await response.json().catch(() => null)
  if (!response.ok) {
    if (response.status === 401 && typeof window !== "undefined") {
      window.dispatchEvent(new Event(unauthorizedEvent))
    }
    const error = (payload || {}) as APIErrorPayload
    throw new APIError(response.status, error.error || "request_failed", error.message, error.fields, error)
  }
  return payload as T
}

export type ApplicationDTO = {
  id: string
  name: string
  description: string
  archived: boolean
  created_at: string
  updated_at: string
  environment_health?: Array<{ environment_id: string; name: string; kind: string; service_count: number; configured_count?: number; running_count: number; status: "empty" | "healthy" | "degraded" }>
  service_count?: number
  healthy_service_count?: number
  domain_count?: number
  latest_deployment?: DeploymentDTO
}

export type EnvironmentDTO = {
  id: string
  application_id: string
  name: string
  slug: string
  kind: "production" | "staging"
  created_at: string
  updated_at: string
}

export type ServiceDTO = {
  id: string
  application_id: string
  service_type: "application" | "database"
  name: string
  repo_url: string
  stack_kind?: string
  stack_label?: string
  github_installation_id: number
  root_directory: string
  runtime: string
  install_cmd: string
  build_cmd: string
  start_cmd: string
  internal_port: number
  health_check_path: string
  created_at: string
  updated_at: string
}

export type DatabaseEngineDTO = {
  id: "postgresql" | "mysql" | "mariadb" | "mongodb" | "redis" | "valkey"
  name: string
  description: string
  category: "Relational" | "Document" | "Key-value"
  versions: Array<{ version: string; default: boolean; provisioning_available: boolean }>
  internal_port: number
  connection_variable: string
  minimum_memory_bytes: number
  public_access_available: false
}

export type DatabaseResourcePresetDTO = {
  id: "development" | "standard" | "performance" | "custom"
  name: string
  description: string
  cpu_limit_millis: number
  memory_limit_bytes: number
}

export type DatabaseInstanceDTO = {
  id: string
  service_id: string
  environment_id: string
  engine_version: string
  docker_container_id?: string
  network_alias: string
  internal_port: number
  volume_name: string
  resource_preset: string
  cpu_limit_millis: number
  memory_limit_bytes: number
  desired_state: "running" | "stopped" | "deleted"
  status: "provisioning" | "starting" | "healthy" | "unhealthy" | "stopping" | "stopped" | "failed" | "deleted" | "purging"
  health_message?: string
	storage_used_bytes?: number
	storage_checked_at?: string
  purge_after?: string
  created_at: string
  updated_at: string
}

export type DatabaseOperationDTO = {
  id: string
  service_id: string
  database_instance_id?: string
  operation_type: "provision" | "start" | "stop" | "restart" | "backup" | "restore" | "rotate_credentials" | "delete" | "restore_deleted" | "purge"
  status: "queued" | "running" | "success" | "failed" | "cancelled"
  progress_step: string
  progress_percent: number
  error_code?: string
  error_message?: string
  created_at: string
  updated_at: string
}

export type DatabaseCredentialMetadataDTO = {
  database_instance_id: string
  database_name: string
  username: string
  generation: number
  rotated_at?: string
  created_at: string
  updated_at: string
}

export type DatabaseMetricDTO = {
  cpu_percent: number
  memory_bytes: number
  network_rx_bytes: number
  network_tx_bytes: number
  sampled_at: string
}

export type DatabaseUpgradePreflightDTO = {
  available: boolean
  ready: boolean
  reason: string
  engine_version: string
  current_image_ref: string
  target_image_ref: string
  backup_max_age_hours: number
  latest_backup?: DatabaseBackupDTO
}

export type BackupDestinationDTO = {
  id: string
  name: string
  provider: "r2" | "s3"
  endpoint: string
  region: string
  bucket: string
  object_prefix: string
  path_style: boolean
  server_side_encryption?: "" | "AES256" | "aws:kms"
  sse_kms_key_id?: string
  last_test_status: "" | "success" | "failed"
  last_test_message?: string
  last_tested_at?: string
  created_at: string
  updated_at: string
}

export type DatabaseBackupPolicyDTO = {
  database_instance_id: string
  destination_id: string
  enabled: boolean
  schedule: string
  timezone: string
  retention_days: number
  last_scheduled_at?: string
  next_scheduled_at?: string
  created_at: string
  updated_at: string
}

export type DatabaseBackupDTO = {
  id: string
  database_instance_id: string
  destination_id?: string
  status: "queued" | "running" | "success" | "failed" | "cancelled" | "deleting"
  trigger_kind: "manual" | "scheduled" | "safety"
  object_key?: string
  archive_format?: string
  checksum?: string
  compressed_size: number
  engine: "postgresql" | "mysql" | "mariadb" | "mongodb" | "redis" | "valkey"
  database_name: string
  engine_version: string
  error_code?: string
  completed_at?: string
  expires_at?: string
  created_at: string
  updated_at: string
}

export type ServiceEnvironmentDTO = {
  service_id: string
  environment_id: string
  branch: string
  auto_deploy: boolean
  active_deployment_id: string
  desired_state: "running" | "stopped"
  created_at: string
  updated_at: string
}

export type ServiceEnvironmentStateDTO = ServiceEnvironmentDTO & {
  environment_name?: string
  environment_kind?: string
  active_deployment?: DeploymentDTO
  current_container?: { id: string; docker_container_id: string; internal_port: number; host_port: number; status: string; updated_at: string }
  public_url?: string
  public_url_status?: "ready" | "platform_domain_required" | "platform_state_unavailable" | "platform_domain_generation_failed" | "route_sync_failed"
  domains?: DomainDTO[]
  container_error?: string
}

export type DeploymentDTO = {
  id: string
  service_id: string
  environment_id: string
  application_id?: string
  application_name?: string
  service_name?: string
  environment_name?: string
  environment_kind?: "production" | "staging"
  is_active?: boolean
  public_url?: string
  urls?: string[]
  branch?: string
  status: "QUEUED" | "BUILDING" | "SUCCESS" | "FAILED" | "CANCELLED"
  commit_hash: string
  logs_path?: string
  image_ref?: string
  error_message?: string
  builder_kind?: string
  stack_kind?: string
  stack_label?: string
  trigger: string
  actor?: string
  cancelled_at?: string
  rollback_of?: string
  created_at: string
  updated_at: string
}

export type DeploymentFilter = {
  applicationID?: string
  serviceID?: string
  environmentID?: string
  status?: string
  trigger?: string
  branch?: string
  dateFrom?: string
  dateTo?: string
  cursor?: string
  limit?: number
}

export type DeploymentStepDTO = {
  id: number
  deployment_id: string
  service_id: string
  environment_id: string
  service_name?: string
  environment_name?: string
  request_id: string
  step: string
  status: string
  duration_ms: number
  error_code: string
  started_at: string
  ended_at: string
}

export type ServiceMetricDTO = {
  id: number
  service_id: string
  environment_id: string
  cpu_percent: number
  memory_bytes: number
  network_rx_bytes: number
  network_tx_bytes: number
  sampled_at: string
}

export type HTTPRequestDTO = {
  id: number
  request_id: string
  application_id?: string
  service_id?: string
  environment_id?: string
  method: string
  path: string
  status: number
  duration_ms: number
  started_at: string
}

export type ObservabilitySummaryDTO = {
  window_hours: number
  http_request_count: number
  http_error_count: number
  http_duration_p50_ms: number
  http_duration_p95_ms: number
  deploy_count: number
  deploy_failed_count: number
  deploy_duration_p50_ms: number
  deploy_duration_p95_ms: number
}

export type HostSampleDTO = {
  at: string
  cpu_pct: number
  per_core_pct?: number[]
  mem: { used_bytes: number; total_bytes: number; available_bytes?: number; used_pct: number }
  net: Array<{ iface: string; rx_bps: number; tx_bps: number }>
  disks: Array<{ mount: string; used_bytes: number; total_bytes: number; used_pct: number }>
  uptime_seconds: number
  rates_ready: boolean
  err?: string
}

export type PlatformEventDTO = {
  id: number
  application_id?: string
  service_id?: string
  environment_id?: string
  deployment_id?: string
  event_type: string
  status?: string
  actor?: string
  message: string
  detail?: string
  created_at: string
}

export type GitHubInstallationDTO = {
  installation_id: number
  account_login: string
  suspended: boolean
}

export type GitHubRepositoryDTO = {
  id: number
  full_name: string
  private: boolean
  default_branch: string
  clone_url: string
}

export type DomainDTO = {
  id: string
  application_id: string
  environment_id: string
  service_id: string
  domain_name: string
  kind?: "custom" | "platform"
  ssl_status: "PENDING" | "ACTIVE" | "ERROR"
  last_cert_message?: string
  cert_checked_at?: string
  created_at: string
  updated_at: string
}

export type CaddySyncOutcomeDTO = {
  attempted: boolean
  ok: boolean
  error?: string
}

export type DNSGuidanceDTO = {
  ipv4?: string
  ipv6?: string
  ipv4_source: "override" | "detected" | "unknown"
  ipv6_source: "override" | "detected" | "unknown" | "omitted"
  records: Array<{ type: "A" | "AAAA"; name: string; value: string; zone_hint?: string; note?: string }>
  checks?: Array<{ hostname: string; status: "ok" | "pending" | "unknown" | "lookup_error"; expected_ipv4?: string; resolved_ipv4: string[] }>
  steps?: string[]
  message?: string
}

export type DomainMutationDTO = {
  status: string
  domain?: DomainDTO
  domain_id?: string
  caddy_sync: CaddySyncOutcomeDTO
}

export type DeleteOutcomeDTO = {
  status: "deleted"
  routing_warning?: string
}

export type EnvironmentVariableDTO = {
  id: string
  application_id: string
  environment_id: string
  service_id?: string
  key: string
  value_last4: string
  created_at: string
  updated_at: string
}

export type OnboardingDTO = {
  bootstrap_enabled: boolean
  bootstrap_public_ip: string
  bootstrap_https_port: number
  bootstrap_expires_at: string
  github_app_complete: boolean
  platform_domain: string
  permanent_ingress_complete: boolean
  bootstrap_complete: boolean
  completed_at: string
}

export type SystemStatusDTO = {
  version: string
  checks: Array<{ id: string; label: string; status: string; detail?: string; error_code?: string }>
}

export type SettingsDTO = {
  auth: { scheme: string; expires_at?: string; subject?: string }
  build: { version: string; version_display: string; commit: string; build_time: string; go_version: string; os: string; arch: string; started_at: string; uptime_seconds: number }
  paths: { data_dir: string; logs_dir: string; db_path: string; db_size_bytes: number; logs_dir_size_bytes: number }
  network: { listen: string; host_port: number; port_start: number; port_end: number; container_port: number }
  caddy: { root_config: string; generated_path: string; control_plane_path: string; sync_caddy: boolean; domain_sync_after_mutate: boolean; admin_url: string }
  webhooks: { base_path: string; async: boolean; rate_limit_per_minute: number; secret_set: boolean }
  dns: { server_ipv4: string; detected_ipv4: string; detected_ipv4_source: string; detected_ipv4_warning: string }
  session: { ttl_minutes: number; cookie_secure: boolean; session_secret_set: boolean; api_token_set: boolean }
  health: { path: string; timeout_ms: number; retries: number; interval_ms: number; expected_min: number; expected_max: number }
  platform: { domain: string; configured: boolean; managed_domain_count: number }
}

export type ObservabilityFilter = {
  applicationID?: string
  serviceID?: string
  environmentID?: string
  dateFrom?: string
  dateTo?: string
  cursor?: string
  limit?: number
}

export type RequestObservabilityFilter = ObservabilityFilter & {
  method?: string
  statusClass?: "success" | "client_error" | "server_error"
}

export type DeployStepObservabilityFilter = ObservabilityFilter & {
  status?: string
}

function observabilityQuery(filters: ObservabilityFilter & { method?: string; statusClass?: string; status?: string }) {
  const params = new URLSearchParams()
  if (filters.applicationID) params.set("application_id", filters.applicationID)
  if (filters.serviceID) params.set("service_id", filters.serviceID)
  if (filters.environmentID) params.set("environment_id", filters.environmentID)
  if (filters.method) params.set("method", filters.method)
  if (filters.statusClass) params.set("status_class", filters.statusClass)
  if (filters.status) params.set("status", filters.status)
  if (filters.dateFrom) params.set("date_from", filters.dateFrom)
  if (filters.dateTo) params.set("date_to", filters.dateTo)
  if (filters.cursor) params.set("cursor", filters.cursor)
  if (filters.limit) params.set("limit", String(filters.limit))
  return params.size ? "?" + params.toString() : ""
}

export const api = {
  session: (signal?: AbortSignal) => apiRequest<{ authenticated: boolean }>("/auth/session", { signal }),
  login: (token: string) =>
    apiRequest<{ authenticated: boolean }>("/auth/session", {
      method: "POST",
      headers: { Authorization: `Bearer ${token.trim()}` },
    }),
  logout: () => apiRequest<{ authenticated: boolean }>("/auth/session", { method: "DELETE" }),
  onboarding: (signal?: AbortSignal) => apiRequest<{ onboarding: OnboardingDTO }>("/api/onboarding", { signal }),
  completeOnboarding: (platformDomain: string) => apiRequest<{ status: "ok"; bootstrap_disabled: boolean; platform_domain: string }>("/api/onboarding", { method: "PATCH", body: JSON.stringify({ platform_domain: platformDomain }) }),
  systemStatus: (signal?: AbortSignal) => apiRequest<SystemStatusDTO>("/api/system/status", { signal }),
  settings: (signal?: AbortSignal) => apiRequest<SettingsDTO>("/api/settings", { signal }),
  settingsAction: (action: "caddy-validate" | "caddy-sync" | "refresh-status" | "detect-public-ipv4") => apiRequest<Record<string, unknown>>("/api/settings/actions/" + action, { method: "POST" }),
  updatePlatformDomain: (domain: string) => apiRequest<{ status: string; platform_domain: string }>("/api/settings/platform-domain", { method: "PATCH", body: JSON.stringify({ domain }) }),

  applications: async (signal?: AbortSignal) => {
    const payload = await apiRequest<{ applications: ApplicationDTO[] | null }>("/api/applications", { signal })
    return { ...payload, applications: Array.isArray(payload.applications) ? payload.applications : [] }
  },
  application: async (id: string, signal?: AbortSignal) => {
    const payload = await apiRequest<{ application: ApplicationDTO; environments: EnvironmentDTO[] | null; services: ServiceDTO[] | null; service_bindings: Record<string, ServiceEnvironmentDTO[]> | null; database_instances?: Record<string, DatabaseInstanceDTO[]> | null }>(
      `/api/applications/${encodeURIComponent(id)}`,
      { signal },
    )
    return {
      ...payload,
      environments: Array.isArray(payload.environments) ? payload.environments : [],
      services: Array.isArray(payload.services) ? payload.services : [],
      service_bindings: payload.service_bindings && typeof payload.service_bindings === "object" ? payload.service_bindings : {},
      ...(payload.database_instances && typeof payload.database_instances === "object" ? { database_instances: payload.database_instances } : {}),
    }
  },
  createApplication: (input: { name: string; description: string }) =>
    apiRequest<{ application: ApplicationDTO }>("/api/applications", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  createEnvironment: (applicationID: string, input: { name: string; slug: string; kind: "production" | "staging" }) =>
    apiRequest<{ environment: EnvironmentDTO }>(`/api/applications/${encodeURIComponent(applicationID)}/environments`, { method: "POST", body: JSON.stringify(input) }),
  updateEnvironment: (applicationID: string, environmentID: string, input: { name: string }) =>
    apiRequest<{ status: "ok"; environment: EnvironmentDTO }>(`/api/applications/${encodeURIComponent(applicationID)}/environments/${encodeURIComponent(environmentID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  updateApplication: (id: string, input: Partial<Pick<ApplicationDTO, "name" | "description" | "archived">>) =>
    apiRequest<{ application: ApplicationDTO }>(`/api/applications/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteApplication: (id: string) =>
    apiRequest<DeleteOutcomeDTO>(`/api/applications/${encodeURIComponent(id)}`, { method: "DELETE" }),
  databaseEngines: (signal?: AbortSignal) =>
    apiRequest<{
      engines: DatabaseEngineDTO[]
      resource_presets: DatabaseResourcePresetDTO[]
      resource_capacity?: {
        available: boolean
        cpu_total_millis: number
        cpu_allocated_millis: number
        cpu_reserve_millis: number
        cpu_available_millis: number
        memory_total_bytes: number
        memory_allocated_bytes: number
        memory_reserve_bytes: number
        memory_available_bytes: number
      }
      networking: { scope: "hostforge_environment"; public_access_available: false }
    }>("/api/database-engines", { signal }),
  createDatabaseService: (
    applicationID: string,
    input: {
      name: string
      engine: DatabaseEngineDTO["id"]
      version: string
      environment_ids: string[]
      resource_preset: DatabaseResourcePresetDTO["id"]
	  custom_cpu_millis?: number
	  custom_memory_bytes?: number
	  backup_enabled?: boolean
	  backup_destination_id?: string
      connections: Array<{ service_id: string; variable_key: string; replace_existing?: boolean }>
    },
  ) => apiRequest<{
    status: "queued"
    service: ServiceDTO
    database: { service_id: string; engine: DatabaseEngineDTO["id"]; default_version: string }
    instances: DatabaseInstanceDTO[]
    bindings: Array<{ id: string; database_instance_id: string; environment_id: string; consumer_service_id: string; variable_key: string; replace_existing: boolean }>
    operations: DatabaseOperationDTO[]
  }>(`/api/applications/${encodeURIComponent(applicationID)}/database-services`, { method: "POST", body: JSON.stringify(input) }),
  databaseOperation: (operationID: string, signal?: AbortSignal) =>
    apiRequest<{ operation: DatabaseOperationDTO }>(`/api/database-operations/${encodeURIComponent(operationID)}`, { signal }),

  service: async (id: string, signal?: AbortSignal) => {
    const payload = await apiRequest<{
      service: ServiceDTO
      bindings: ServiceEnvironmentDTO[] | null
      environment_states: ServiceEnvironmentStateDTO[] | null
      database?: { service_id: string; engine: DatabaseEngineDTO["id"]; default_version: string }
      database_instances?: DatabaseInstanceDTO[] | null
      database_bindings?: Record<string, Array<{ id: string; database_instance_id: string; environment_id: string; consumer_service_id: string; variable_key: string; replace_existing: boolean }>> | null
      database_credentials?: Record<string, DatabaseCredentialMetadataDTO> | null
      database_operations?: DatabaseOperationDTO[] | null
    }>(
      `/api/services/${encodeURIComponent(id)}`,
      { signal },
    )
    return {
      ...payload,
      bindings: payload.bindings ?? [],
      environment_states: payload.environment_states ?? [],
    }
  },
  createService: (
    applicationID: string,
    input: Omit<ServiceDTO, "id" | "application_id" | "service_type" | "created_at" | "updated_at"> & {
      environment_id?: string
      branch?: string
      auto_deploy?: boolean
    },
  ) =>
    apiRequest<{ service: ServiceDTO; binding?: ServiceEnvironmentDTO }>(`/api/applications/${encodeURIComponent(applicationID)}/services`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateService: (id: string, input: Partial<Omit<ServiceDTO, "id" | "application_id" | "service_type" | "created_at" | "updated_at">>) =>
    apiRequest<{ service: ServiceDTO }>("/api/services/" + encodeURIComponent(id), { method: "PATCH", body: JSON.stringify(input) }),
  deleteService: (id: string, confirmation?: string) => apiRequest<DeleteOutcomeDTO>("/api/services/" + encodeURIComponent(id), { method: "DELETE", ...(confirmation ? { body: JSON.stringify({ confirmation }) } : {}) }),
  restoreDeletedDatabase: (id: string) =>
    apiRequest<{ status: "queued"; operations: DatabaseOperationDTO[] }>(
      `/api/database-services/${encodeURIComponent(id)}/restore-deleted`,
      { method: "POST" },
    ),
  purgeDatabaseService: (id: string, confirmation: string) =>
    apiRequest<{ status: "purged" }>(`/api/database-services/${encodeURIComponent(id)}/purge`, { method: "DELETE", body: JSON.stringify({ confirmation }) }),
  databaseRuntimeAction: (instanceID: string, action: "start" | "stop" | "restart") =>
    apiRequest<{ status: "queued"; operation: DatabaseOperationDTO }>(
      `/api/database-instances/${encodeURIComponent(instanceID)}/${action}`,
      { method: "POST" },
    ),
  rotateDatabaseCredentials: (instanceID: string) =>
    apiRequest<{ status: "queued"; operation: DatabaseOperationDTO }>(
      `/api/database-instances/${encodeURIComponent(instanceID)}/rotate-credentials`,
      { method: "POST" },
    ),
  databaseUpgradePreflight: (instanceID: string, signal?: AbortSignal) =>
    apiRequest<DatabaseUpgradePreflightDTO>(`/api/database-instances/${encodeURIComponent(instanceID)}/upgrade`, { signal }),
  upgradeDatabaseInstance: (instanceID: string) =>
    apiRequest<{ status: "queued"; operation: DatabaseOperationDTO }>(`/api/database-instances/${encodeURIComponent(instanceID)}/upgrade`, { method: "POST" }),
  databaseLogs: (instanceID: string, tail = 200, signal?: AbortSignal) =>
    apiRequest<{ instance_id: string; logs: string }>(
      `/api/database-instances/${encodeURIComponent(instanceID)}/logs?tail=${tail}`,
      { signal },
    ),
  databaseMetrics: (instanceID: string, signal?: AbortSignal) =>
    apiRequest<{ instance_id: string; metric: DatabaseMetricDTO }>(
      `/api/database-instances/${encodeURIComponent(instanceID)}/metrics`,
      { signal },
    ),
  backupDestinations: (signal?: AbortSignal) =>
    apiRequest<{ destinations: BackupDestinationDTO[] }>("/api/backup-destinations", { signal }),
  createBackupDestination: (input: {
    name: string; provider: "r2" | "s3"; account_id?: string; endpoint?: string; region?: string;
    bucket: string; object_prefix?: string; path_style?: boolean; server_side_encryption?: "" | "AES256" | "aws:kms"; sse_kms_key_id?: string; access_key_id: string; secret_access_key: string
  }) => apiRequest<{ destination: BackupDestinationDTO }>("/api/backup-destinations", { method: "POST", body: JSON.stringify(input) }),
  updateBackupDestination: (id: string, input: Partial<{
    name: string; provider: "r2" | "s3"; account_id: string; endpoint: string; region: string;
    bucket: string; object_prefix: string; path_style: boolean; server_side_encryption: "" | "AES256" | "aws:kms"; sse_kms_key_id: string; access_key_id: string; secret_access_key: string
  }>) => apiRequest<{ destination: BackupDestinationDTO }>(`/api/backup-destinations/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(input) }),
  testBackupDestination: (id: string) => apiRequest<{ destination: BackupDestinationDTO }>(`/api/backup-destinations/${encodeURIComponent(id)}/test`, { method: "POST" }),
  deleteBackupDestination: (id: string) => apiRequest<{ status: "deleted" }>(`/api/backup-destinations/${encodeURIComponent(id)}`, { method: "DELETE" }),
  databaseBackupPolicy: (instanceID: string, signal?: AbortSignal) => apiRequest<{ policy: DatabaseBackupPolicyDTO | null }>(`/api/database-instances/${encodeURIComponent(instanceID)}/backup-policy`, { signal }),
  saveDatabaseBackupPolicy: (instanceID: string, input: { destination_id: string; enabled: boolean; schedule: string; timezone: string; retention_days: number }) => apiRequest<{ policy: DatabaseBackupPolicyDTO }>(`/api/database-instances/${encodeURIComponent(instanceID)}/backup-policy`, { method: "PUT", body: JSON.stringify(input) }),
  databaseBackups: (instanceID: string, signal?: AbortSignal) => apiRequest<{ backups: DatabaseBackupDTO[] }>(`/api/database-instances/${encodeURIComponent(instanceID)}/backups`, { signal }),
  createDatabaseBackup: (instanceID: string) => apiRequest<{ backup: DatabaseBackupDTO; operation: DatabaseOperationDTO }>(`/api/database-instances/${encodeURIComponent(instanceID)}/backups`, { method: "POST" }),
  restoreDatabaseBackup: (backupID: string, input: { mode?: "new_service" | "replace_current"; target_instance_id?: string; confirmation?: string }) =>
    apiRequest<{ status: "queued"; operation: DatabaseOperationDTO }>(`/api/database-backups/${encodeURIComponent(backupID)}/restore`, { method: "POST", body: JSON.stringify(input) }),
  deleteDatabaseBackup: (backupID: string) => apiRequest<{ status: "deleted" }>(`/api/database-backups/${encodeURIComponent(backupID)}`, { method: "DELETE" }),
  createDatabaseBinding: (instanceID: string, input: { consumer_service_id: string; variable_key: string; replace_existing?: boolean }) => apiRequest<{ binding: { id: string } }>(`/api/database-instances/${encodeURIComponent(instanceID)}/bindings`, { method: "POST", body: JSON.stringify(input) }),
  updateDatabaseBinding: (bindingID: string, input: { consumer_service_id: string; variable_key: string; replace_existing?: boolean }) => apiRequest<{ binding: { id: string; consumer_service_id: string; variable_key: string; replace_existing: boolean } }>(`/api/database-bindings/${encodeURIComponent(bindingID)}`, { method: "PATCH", body: JSON.stringify(input) }),
  deleteDatabaseBinding: (bindingID: string) => apiRequest<{ status: "deleted" }>(`/api/database-bindings/${encodeURIComponent(bindingID)}`, { method: "DELETE" }),
  updateServiceBinding: (
    serviceID: string,
    environmentID: string,
    input: { branch: string; auto_deploy: boolean },
  ) =>
    apiRequest<{ binding: ServiceEnvironmentDTO }>(
      `/api/services/${encodeURIComponent(serviceID)}/environments/${encodeURIComponent(environmentID)}`,
      { method: "PATCH", body: JSON.stringify(input) },
    ),
  serviceMetrics: async (serviceID: string, environmentID: string, points = 120, signal?: AbortSignal) => {
    const payload = await apiRequest<{ supported: boolean; stale?: boolean; stale_reason?: "service_stopped" | "collector_delayed"; error_code?: string; deployment_id?: string; sample?: ServiceMetricDTO; samples: ServiceMetricDTO[] | null; sample_interval_seconds?: number }>("/api/services/" + encodeURIComponent(serviceID) + "/environments/" + encodeURIComponent(environmentID) + "/metrics?points=" + points, { signal })
    return { ...payload, samples: payload.samples ?? [] }
  },
  observabilitySummary: (signal?: AbortSignal) => apiRequest<{ summary: ObservabilitySummaryDTO; system: unknown }>("/api/observability/summary", { signal }),
  observabilityRequests: async (filters: RequestObservabilityFilter = {}, signal?: AbortSignal) => {
    const payload = await apiRequest<{ requests: HTTPRequestDTO[] | null; next_cursor: string }>("/api/observability/requests" + observabilityQuery(filters), { signal })
    return { ...payload, requests: payload.requests ?? [] }
  },
  observabilityDeploySteps: async (filters: DeployStepObservabilityFilter = {}, signal?: AbortSignal) => {
    const payload = await apiRequest<{ deploy_steps: DeploymentStepDTO[] | null; next_cursor: string }>("/api/observability/deploy-steps" + observabilityQuery(filters), { signal })
    return { ...payload, deploy_steps: payload.deploy_steps ?? [] }
  },
  hostSnapshot: (signal?: AbortSignal) => apiRequest<{ supported: boolean; error_code?: string; sample?: HostSampleDTO }>("/api/system/host/snapshot", { signal }),
  hostHistory: async (points = 120, signal?: AbortSignal) => {
    const payload = await apiRequest<{ supported: boolean; error_code?: string; samples: HostSampleDTO[] | null }>("/api/system/host/history?points=" + points, { signal })
    return { ...payload, samples: payload.samples ?? [] }
  },
  events: (filters: { applicationID?: string; serviceID?: string; type?: string; dateFrom?: string; dateTo?: string; cursor?: string; limit?: number } = {}, signal?: AbortSignal) => {
    const params = new URLSearchParams()
    if (filters.applicationID) params.set("application_id", filters.applicationID)
    if (filters.serviceID) params.set("service_id", filters.serviceID)
    if (filters.type) params.set("type", filters.type)
    if (filters.dateFrom) params.set("date_from", filters.dateFrom)
    if (filters.dateTo) params.set("date_to", filters.dateTo)
    if (filters.cursor) params.set("cursor", filters.cursor)
    if (filters.limit) params.set("limit", String(filters.limit))
    const query = params.size ? "?" + params.toString() : ""
    return apiRequest<{ events: PlatformEventDTO[] | null; next_cursor: string }>("/api/events" + query, { signal }).then((payload) => ({ ...payload, events: payload.events ?? [] }))
  },
  githubApp: (signal?: AbortSignal) => apiRequest<{ app: { configured: boolean; app_id?: number; slug?: string; html_url?: string; updated_at?: string } }>("/api/github/app", { signal }),
  deleteGitHubApp: () => apiRequest<{ status: "deleted" }>("/api/github/app", { method: "DELETE" }),
  githubManifest: (input: { name: string; url: string; callback_url: string; webhook_url?: string }) => apiRequest<{ manifest: Record<string, unknown>; post_url: string; callback_url: string; webhook_url: string }>("/api/github/app/manifest", { method: "POST", body: JSON.stringify(input) }),
  githubManifestExchange: (code: string) => apiRequest<{ app: { configured: boolean; app_id: number; slug: string }; install_url: string }>("/api/github/app/manifest/exchange", { method: "POST", body: JSON.stringify({ code }) }),
  githubInstallations: (signal?: AbortSignal) => apiRequest<{ installations: GitHubInstallationDTO[] | null }>("/api/github/installations", { signal }).then((payload) => ({ ...payload, installations: payload.installations ?? [] })),
  syncGitHubInstallations: () => apiRequest<{ installations: GitHubInstallationDTO[] | null }>("/api/github/installations/sync", { method: "POST" }).then((payload) => ({ ...payload, installations: payload.installations ?? [] })),
  githubRepositories: (installationID: number, signal?: AbortSignal) => apiRequest<{ repositories: GitHubRepositoryDTO[] | null }>("/api/github/installations/" + installationID + "/repositories", { signal }).then((payload) => ({ ...payload, repositories: payload.repositories ?? [] })),
  repositoryBranches: (repoURL: string, installationID: number, signal?: AbortSignal) => {
    const params = new URLSearchParams({ repo_url: repoURL, installation_id: String(installationID) })
    return apiRequest<{ branches: string[] | null; default_branch: string }>("/api/repositories/branches?" + params.toString(), { signal }).then((payload) => ({ ...payload, branches: payload.branches ?? [] }))
  },
  domains: (applicationID: string, environmentID: string, serviceID = "", signal?: AbortSignal, checkDNS = false) => {
    const params = new URLSearchParams()
    if (serviceID) params.set("service_id", serviceID)
    if (checkDNS) params.set("check_dns", "1")
    const query = params.size ? "?" + params.toString() : ""
    return apiRequest<{ domains: DomainDTO[] | null; dns_guidance: DNSGuidanceDTO }>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/domains" + query, { signal }).then((payload) => ({ ...payload, domains: payload.domains ?? [] }))
  },
  createDomain: (applicationID: string, environmentID: string, input: { domain_name: string; service_id: string }) =>
    apiRequest<DomainMutationDTO>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/domains", { method: "POST", body: JSON.stringify(input) }),
  updateDomain: (applicationID: string, environmentID: string, domainID: string, input: { domain_name: string; service_id: string }) =>
    apiRequest<DomainMutationDTO>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/domains/" + encodeURIComponent(domainID), { method: "PATCH", body: JSON.stringify(input) }),
  deleteDomain: (applicationID: string, environmentID: string, domainID: string) =>
    apiRequest<DomainMutationDTO>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/domains/" + encodeURIComponent(domainID), { method: "DELETE" }),
  environmentVariables: (applicationID: string, environmentID: string, serviceID = "", signal?: AbortSignal) => {
    const query = serviceID ? "?service_id=" + encodeURIComponent(serviceID) : ""
    return apiRequest<{ variables: EnvironmentVariableDTO[] | null }>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/variables" + query, { signal }).then((payload) => ({ ...payload, variables: payload.variables ?? [] }))
  },
  upsertEnvironmentVariable: (applicationID: string, environmentID: string, input: { key: string; value: string; service_id?: string }) =>
    apiRequest<{ variable: EnvironmentVariableDTO }>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/variables", { method: "POST", body: JSON.stringify(input) }),
  updateEnvironmentVariable: (applicationID: string, environmentID: string, variableID: string, value: string) =>
    apiRequest<{ variable: EnvironmentVariableDTO }>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/variables/" + encodeURIComponent(variableID), { method: "PATCH", body: JSON.stringify({ value }) }),
  deleteEnvironmentVariable: (applicationID: string, environmentID: string, variableID: string) =>
    apiRequest<{ status: "deleted" }>("/api/applications/" + encodeURIComponent(applicationID) + "/environments/" + encodeURIComponent(environmentID) + "/variables/" + encodeURIComponent(variableID), { method: "DELETE" }),
  stopService: (serviceID: string, environmentID: string) =>
    apiRequest<{ status: "stopped"; deployment_id: string; container_id: string }>("/api/services/" + encodeURIComponent(serviceID) + "/environments/" + encodeURIComponent(environmentID) + "/stop", { method: "POST" }),
  restartService: (serviceID: string, environmentID: string) =>
    apiRequest<{ status: "running"; deployment_id: string; container_id: string }>("/api/services/" + encodeURIComponent(serviceID) + "/environments/" + encodeURIComponent(environmentID) + "/restart", { method: "POST" }),
  deployments: (filters: DeploymentFilter = {}, signal?: AbortSignal) => {
    const params = new URLSearchParams()
    if (filters.applicationID) params.set("application_id", filters.applicationID)
    if (filters.serviceID) params.set("service_id", filters.serviceID)
    if (filters.environmentID) params.set("environment_id", filters.environmentID)
    if (filters.status) params.set("status", filters.status)
    if (filters.trigger) params.set("trigger", filters.trigger)
    if (filters.branch) params.set("branch", filters.branch)
    if (filters.dateFrom) params.set("date_from", filters.dateFrom)
    if (filters.dateTo) params.set("date_to", filters.dateTo)
    if (filters.cursor) params.set("cursor", filters.cursor)
    if (filters.limit) params.set("limit", String(filters.limit))
    const query = params.size ? "?" + params.toString() : ""
    return apiRequest<{ deployments: DeploymentDTO[] | null; next_cursor: string }>("/api/deployments" + query, { signal }).then((payload) => ({ ...payload, deployments: payload.deployments ?? [] }))
  },
  deployment: (id: string, signal?: AbortSignal) => apiRequest<{ deployment: DeploymentDTO }>("/api/deployments/" + encodeURIComponent(id), { signal }),
  deploymentSteps: (id: string, signal?: AbortSignal) => apiRequest<{ deployment_id: string; steps: DeploymentStepDTO[] | null }>("/api/deployments/" + encodeURIComponent(id) + "/steps", { signal }).then((payload) => ({ ...payload, steps: payload.steps ?? [] })),
  deploymentLogs: (id: string, signal?: AbortSignal) => apiRequest<{ eof: number; text: string }>("/api/deployments/" + encodeURIComponent(id) + "/logs?eof_meta=1&tail_lines=1000", { signal }),
  deploy: (serviceID: string, environmentID: string) => apiRequest<{ deployment: DeploymentDTO }>("/api/services/" + encodeURIComponent(serviceID) + "/environments/" + encodeURIComponent(environmentID) + "/deployments", { method: "POST" }),
  redeploy: (id: string) => apiRequest<{ deployment: DeploymentDTO }>("/api/deployments/" + encodeURIComponent(id) + "/redeploy", { method: "POST" }),
  rollback: (id: string) => apiRequest<{ deployment: DeploymentDTO }>("/api/deployments/" + encodeURIComponent(id) + "/rollback", { method: "POST" }),
  cancelDeployment: (id: string) => apiRequest<{ status: "cancelled" }>("/api/deployments/" + encodeURIComponent(id) + "/cancel", { method: "POST" }),
}

export const queryKeys = {
  session: ["session"] as const,
  onboarding: ["onboarding"] as const,
  systemStatus: ["system-status"] as const,
  settings: ["settings"] as const,
  applications: ["applications"] as const,
  databaseEngines: ["database-engines"] as const,
  backupDestinations: ["backup-destinations"] as const,
  databaseOperation: (id: string) => ["database-operations", id] as const,
  application: (id: string) => ["applications", id] as const,
  serviceMetrics: (serviceID: string, environmentID: string, points = 120) => ["service-metrics", serviceID, environmentID, points] as const,
  observabilitySummary: ["observability-summary"] as const,
  observabilityRequests: (filters: RequestObservabilityFilter = {}) => ["observability-requests", filters] as const,
  observabilityDeploySteps: (filters: DeployStepObservabilityFilter = {}) => ["observability-deploy-steps", filters] as const,
  hostSnapshot: ["host-snapshot"] as const,
  hostHistory: (points = 120) => ["host-history", points] as const,
  events: (applicationID = "", serviceID = "", type = "") => ["events", applicationID, serviceID, type] as const,
  githubApp: ["github-app"] as const,
  githubInstallations: ["github-installations"] as const,
  githubRepositories: (installationID: number) => ["github-repositories", installationID] as const,
  githubRepositoriesRoot: ["github-repositories"] as const,
  repositoryBranches: (repoURL: string, installationID: number) => ["repository-branches", repoURL, installationID] as const,
  repositoryBranchesRoot: ["repository-branches"] as const,
  service: (id: string) => ["services", id] as const,
  domains: (applicationID: string, environmentID: string, serviceID = "") => ["domains", applicationID, environmentID, serviceID] as const,
  variables: (applicationID: string, environmentID: string, serviceID = "") => ["variables", applicationID, environmentID, serviceID] as const,
  deployments: (serviceID = "", environmentID = "", applicationID = "") => ["deployments", { serviceID, environmentID, applicationID }] as const,
  deploymentList: (filters: DeploymentFilter = {}) => ["deployments", "list", filters] as const,
  deployment: (id: string) => ["deployments", id] as const,
}
