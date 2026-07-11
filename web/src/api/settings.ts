import type { CaddySyncOutcome, SystemStatus } from "../api";

function apiFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  return fetch(input, { credentials: "same-origin", ...init });
}

async function readJSON<T>(res: Response): Promise<T> {
  const text = await res.text();
  let parsed: unknown = null;
  if (text.trim() !== "") {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }
  if (!res.ok) {
    if (parsed && typeof parsed === "object" && "error" in parsed) {
      const msg = String((parsed as Record<string, unknown>).error || "").trim();
      if (msg) throw new Error(msg);
    }
    if (text.trim()) {
      throw new Error(text.trim());
    }
    throw new Error(`request failed: ${res.status}`);
  }
  if (parsed === null) {
    return {} as T;
  }
  return parsed as T;
}

export type SettingsAuth = {
  scheme: string;
  expires_at?: string;
  subject?: string;
};

export type SettingsBuild = {
  version: string;
  version_display: string;
  commit: string;
  build_time: string;
  go_version: string;
  os: string;
  arch: string;
  pid: number;
  started_at: string;
  uptime_seconds: number;
};

export type SettingsPaths = {
  data_dir: string;
  data_dir_env: string;
  logs_dir: string;
  logs_dir_env: string;
  db_path: string;
  db_size_bytes: number;
  logs_dir_size_bytes: number;
};

export type SettingsNetwork = {
  listen: string;
  listen_env: string;
  host_port: number;
  host_port_env: string;
  port_start: number;
  port_start_env: string;
  port_end: number;
  port_end_env: string;
  container_port: number;
  container_port_env: string;
};

export type SettingsHealth = {
  path: string;
  path_env: string;
  timeout_ms: number;
  timeout_ms_env: string;
  retries: number;
  retries_env: string;
  interval_ms: number;
  interval_ms_env: string;
  expected_min: number;
  expected_min_env: string;
  expected_max: number;
  expected_max_env: string;
};

export type SettingsCaddy = {
  bin: string;
  bin_env: string;
  generated_path: string;
  generated_path_env: string;
  root_config: string;
  root_config_env: string;
  sync_caddy: boolean;
  sync_caddy_env: string;
  domain_sync_after_mutate: boolean;
  domain_sync_after_mutate_env: string;
  cert_poll_interval_sec: number;
  cert_poll_interval_sec_env: string;
  admin_url: string;
  admin_url_env: string;
  storage_root: string;
  storage_root_env: string;
};

export type SettingsWebhooks = {
  base_path: string;
  base_path_env: string;
  max_body_bytes: number;
  max_body_bytes_env: string;
  async: boolean;
  async_env: string;
  rate_limit_per_minute: number;
  rate_limit_per_minute_env: string;
  secret_set: boolean;
  secret_env: string;
};

export type SettingsDNS = {
  server_ipv4: string;
  server_ipv4_env: string;
  server_ipv6: string;
  server_ipv6_env: string;
  detect_url: string;
  detect_url_env: string;
  detect_ipv6_url: string;
  detect_ipv6_url_env: string;
  detect_timeout_ms: number;
  detect_timeout_ms_env: string;
  detected_ipv4: string;
  detected_ipv4_source: string;
  detected_ipv4_warning: string;
};

export type SettingsSession = {
  cookie_name: string;
  cookie_name_env: string;
  ttl_minutes: number;
  ttl_minutes_env: string;
  cookie_secure: boolean;
  cookie_secure_env: string;
  session_secret_set: boolean;
  session_secret_env: string;
  api_token_set: boolean;
  api_token_env: string;
};

export type HostForgeSettings = {
  auth: SettingsAuth;
  build: SettingsBuild;
  paths: SettingsPaths;
  network: SettingsNetwork;
  health: SettingsHealth;
  caddy: SettingsCaddy;
  webhooks: SettingsWebhooks;
  dns: SettingsDNS;
  session: SettingsSession;
};

export async function fetchSettings(): Promise<HostForgeSettings> {
  const res = await apiFetch("/api/settings");
  return readJSON<HostForgeSettings>(res);
}

export type CaddyValidateResult = {
  ok: boolean;
  stdout: string;
  stderr: string;
  took_ms: number;
  root?: string;
  error?: string;
  detail?: string;
};

export async function postCaddyValidate(): Promise<CaddyValidateResult> {
  const res = await apiFetch("/api/settings/actions/caddy-validate", { method: "POST" });
  return readJSON<CaddyValidateResult>(res);
}

export async function postCaddySync(): Promise<{ caddy_sync: CaddySyncOutcome; duration_ms: number }> {
  const res = await apiFetch("/api/settings/actions/caddy-sync", { method: "POST" });
  return readJSON<{ caddy_sync: CaddySyncOutcome; duration_ms: number }>(res);
}

export async function postRefreshSystemStatus(): Promise<SystemStatus> {
  const res = await apiFetch("/api/settings/actions/refresh-status", { method: "POST" });
  return readJSON<SystemStatus>(res);
}

export type DetectIPv4Result = {
  ipv4: string;
  source: string;
  warning: string;
};

export async function postDetectPublicIPv4(): Promise<DetectIPv4Result> {
  const res = await apiFetch("/api/settings/actions/detect-public-ipv4", { method: "POST" });
  return readJSON<DetectIPv4Result>(res);
}
