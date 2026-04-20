export type DnsGuidanceRecord = {
  type: string;
  name: string;
  value: string;
  zone_hint?: string;
  note?: string;
};

export type DnsGuidance = {
  ipv4?: string;
  ipv6?: string;
  ipv4_source: string;
  ipv6_source?: string;
  records: DnsGuidanceRecord[];
  steps?: string[];
  message?: string;
};

export type CaddySyncOutcome = {
  attempted: boolean;
  ok: boolean;
  error?: string;
};

export type ApiDeployConfig = {
  runtime: string;
  install_cmd: string;
  build_cmd: string;
  start_cmd: string;
};

export type ApiProject = {
  id: string;
  name: string;
  repo_url: string;
  branch: string;
  deploy: ApiDeployConfig;
  created_at: string;
  updated_at: string;
  latest_deployment?: ApiDeployment;
  domains?: ApiDomain[];
  dns_guidance?: DnsGuidance;
  current_container?: ApiContainer;
};

export type ApiDeployment = {
  id: string;
  project_id: string;
  status: "QUEUED" | "BUILDING" | "SUCCESS" | "FAILED" | string;
  commit_hash: string;
  logs_path: string;
  image_ref: string;
  worktree: string;
  error_message: string;
  created_at: string;
  updated_at: string;
  container?: ApiContainer;
};

export type ApiContainer = {
  id: string;
  deployment_id: string;
  docker_container_id: string;
  internal_port: number;
  host_port: number;
  status: string;
  created_at: string;
  updated_at: string;
};

export type ApiDomain = {
  id: string;
  project_id: string;
  domain_name: string;
  ssl_status: string;
  last_cert_message?: string;
  cert_checked_at?: string;
  registrar_dns_status?: string;
  resolved_ipv4?: string[];
  created_at: string;
  updated_at: string;
};

export type SystemStatusCheck = {
  id: string;
  label: string;
  status: string;
  detail?: string;
  error_code?: string;
};

export type SystemStatus = {
  version: string;
  checks: SystemStatusCheck[];
};

export type ObservabilitySummary = {
  window_hours: number;
  http_request_count: number;
  http_error_count: number;
  http_duration_p50_ms: number;
  http_duration_p95_ms: number;
  deploy_count: number;
  deploy_failed_count: number;
  deploy_duration_p50_ms: number;
  deploy_duration_p95_ms: number;
};

export type DeployStepRow = {
  id: number;
  deployment_id: string;
  project_id: string;
  request_id: string;
  step: string;
  status: string;
  duration_ms: number;
  error_code: string;
  started_at: string;
  ended_at: string;
  project_name?: string;
};

export type HTTPRequestRow = {
  id: number;
  request_id: string;
  method: string;
  path: string;
  status: number;
  duration_ms: number;
  started_at: string;
};

export type CreateProjectRequest = {
  repo_url: string;
  branch: string;
  project_name: string;
  deploy?: {
    runtime?: string;
    install_cmd?: string;
    build_cmd?: string;
    start_cmd?: string;
  };
};

export type RepositoryBranches = {
  repo_url: string;
  branches: string[];
  default_branch: string;
};

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

export async function fetchProjects(): Promise<ApiProject[]> {
  const res = await apiFetch("/api/projects");
  const body = await readJSON<{ projects: ApiProject[] }>(res);
  return body.projects || [];
}

export async function fetchSystemStatus(): Promise<SystemStatus> {
  const res = await apiFetch("/api/system/status");
  return await readJSON<SystemStatus>(res);
}

export async function fetchObservabilitySummary(): Promise<{
  summary: ObservabilitySummary;
  system: SystemStatus;
}> {
  const res = await apiFetch("/api/observability/summary");
  return await readJSON<{ summary: ObservabilitySummary; system: SystemStatus }>(res);
}

export async function fetchObservabilityRequests(limit = 100): Promise<HTTPRequestRow[]> {
  const res = await apiFetch(`/api/observability/requests?limit=${encodeURIComponent(String(limit))}`);
  const body = await readJSON<{ requests?: HTTPRequestRow[] }>(res);
  return body.requests || [];
}

export async function fetchObservabilityDeploySteps(limit = 200): Promise<DeployStepRow[]> {
  const res = await apiFetch(`/api/observability/deploy-steps?limit=${encodeURIComponent(String(limit))}`);
  const body = await readJSON<{ deploy_steps?: DeployStepRow[] }>(res);
  return body.deploy_steps || [];
}

export async function fetchDeploymentSteps(deploymentID: string, limit = 200): Promise<DeployStepRow[]> {
  const res = await apiFetch(
    `/api/deployments/${encodeURIComponent(deploymentID)}/steps?limit=${encodeURIComponent(String(limit))}`,
  );
  const body = await readJSON<{ steps?: DeployStepRow[] }>(res);
  return body.steps || [];
}

export async function fetchAllDeployments(limit = 100): Promise<ApiDeployment[]> {
  const res = await apiFetch(`/api/deployments?limit=${limit}`);
  const body = await readJSON<{ deployments: ApiDeployment[] }>(res);
  return body.deployments || [];
}

export async function fetchProject(projectID: string): Promise<ApiProject> {
  const res = await apiFetch(`/api/projects/${projectID}`);
  const body = await readJSON<{ project: ApiProject }>(res);
  return body.project;
}

export async function deleteProject(projectID: string): Promise<void> {
  const res = await apiFetch(`/api/projects/${projectID}`, { method: "DELETE" });
  await readJSON<Record<string, unknown>>(res);
}

export async function fetchProjectDeployments(projectID: string): Promise<ApiDeployment[]> {
  const res = await apiFetch(`/api/projects/${projectID}/deployments?limit=100`);
  const body = await readJSON<{ deployments: ApiDeployment[] }>(res);
  return body.deployments || [];
}

export async function fetchProjectDomains(projectID: string): Promise<{
  domains: ApiDomain[];
  dns_guidance?: DnsGuidance;
}> {
  const res = await apiFetch(`/api/projects/${projectID}/domains`);
  const body = await readJSON<{ domains: ApiDomain[]; dns_guidance?: DnsGuidance }>(res);
  return { domains: body.domains || [], dns_guidance: body.dns_guidance };
}

export async function createProjectDomain(
  projectID: string,
  domainName: string,
): Promise<{ domain: ApiDomain; dns_guidance?: DnsGuidance; caddy_sync?: CaddySyncOutcome }> {
  const res = await apiFetch(`/api/projects/${projectID}/domains`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ domain_name: domainName }),
  });
  return await readJSON<{
    domain: ApiDomain;
    dns_guidance?: DnsGuidance;
    caddy_sync?: CaddySyncOutcome;
  }>(res);
}

export async function updateProjectDomain(
  projectID: string,
  domainID: string,
  domainName: string,
): Promise<{ domain: ApiDomain; dns_guidance?: DnsGuidance; caddy_sync?: CaddySyncOutcome }> {
  const res = await apiFetch(`/api/projects/${projectID}/domains/${encodeURIComponent(domainID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ domain_name: domainName }),
  });
  return await readJSON<{
    domain: ApiDomain;
    dns_guidance?: DnsGuidance;
    caddy_sync?: CaddySyncOutcome;
  }>(res);
}

export async function deleteProjectDomain(
  projectID: string,
  domainID: string,
): Promise<{ caddy_sync?: CaddySyncOutcome }> {
  const res = await apiFetch(`/api/projects/${projectID}/domains/${encodeURIComponent(domainID)}`, {
    method: "DELETE",
  });
  return await readJSON<{ caddy_sync?: CaddySyncOutcome }>(res);
}

export async function createProject(input: CreateProjectRequest): Promise<ApiProject> {
  const res = await apiFetch("/api/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const body = await readJSON<{ project: ApiProject }>(res);
  return body.project;
}

export async function updateProjectDeploy(projectID: string, deploy: ApiDeployConfig): Promise<ApiProject> {
  const res = await apiFetch(`/api/projects/${projectID}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ deploy }),
  });
  const body = await readJSON<{ project: ApiProject }>(res);
  return body.project;
}

export async function fetchRepositoryBranches(repoURL: string): Promise<RepositoryBranches> {
  const qs = new URLSearchParams({ repo_url: repoURL }).toString();
  const res = await apiFetch(`/api/repositories/branches?${qs}`);
  const body = await readJSON<{
    repo_url: string;
    branches?: string[];
    default_branch?: string;
  }>(res);
  return {
    repo_url: body.repo_url,
    branches: body.branches || [],
    default_branch: body.default_branch || "main",
  };
}

export async function deployProject(
  projectID: string,
  options: { async?: boolean } = {},
): Promise<{ deployment_id?: string; status?: string; mode?: string; url?: string; error?: string }> {
  const qs = options.async ? "?async=true" : "";
  const res = await apiFetch(`/api/projects/${projectID}/deploy${qs}`, { method: "POST" });
  return await readJSON<{ deployment_id?: string; status?: string; mode?: string; url?: string; error?: string }>(res);
}

export async function restartProject(projectID: string): Promise<void> {
  const res = await apiFetch(`/api/projects/${projectID}/restart`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

export async function rollbackProject(projectID: string): Promise<void> {
  const res = await apiFetch(`/api/projects/${projectID}/rollback`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

export async function stopProject(projectID: string): Promise<void> {
  const res = await apiFetch(`/api/projects/${projectID}/stop`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

/** HTTP tail of deployment logs plus `X-Log-EOF-Offset` for WebSocket resume (build source). */
export type DeploymentLogTail = { text: string; eofOffset: number };

export async function fetchDeploymentLogs(
  deploymentID: string,
  source: "build" | "container",
): Promise<DeploymentLogTail> {
  const qs = new URLSearchParams();
  qs.set("eof_meta", "1");
  if (source !== "build") {
    qs.set("tail_lines", "200");
  }
  const res = await apiFetch(`/api/deployments/${deploymentID}/logs?${qs.toString()}`);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `logs request failed: ${res.status}`);
  }
  const rawBody = await res.text();
  try {
    const j = JSON.parse(rawBody) as { eof?: unknown; text?: unknown };
    if (j && typeof j === "object" && typeof j.text === "string") {
      let eofOffset = 0;
      if (typeof j.eof === "number" && Number.isFinite(j.eof) && j.eof >= 0) {
        eofOffset = j.eof;
      }
      return { text: j.text, eofOffset };
    }
  } catch {
    /* fall through: legacy plain tail */
  }
  const rawEof = res.headers.get("X-Log-EOF-Offset");
  let eofOffset = 0;
  if (rawEof != null && rawEof !== "") {
    const n = parseInt(rawEof, 10);
    if (Number.isFinite(n) && n >= 0) {
      eofOffset = n;
    }
  }
  return { text: rawBody, eofOffset };
}

export async function createSession(token: string): Promise<void> {
  const res = await apiFetch("/auth/session", {
    method: "POST",
    headers: { Authorization: `Bearer ${token.trim()}` },
  });
  await readJSON<Record<string, unknown>>(res);
}

export async function getSessionStatus(): Promise<boolean> {
  const res = await apiFetch("/auth/session");
  if (res.status === 401) {
    return false;
  }
  const body = await readJSON<{ authenticated?: boolean }>(res);
  return Boolean(body.authenticated);
}

export async function deleteSession(): Promise<void> {
  const res = await apiFetch("/auth/session", { method: "DELETE" });
  if (res.status === 401) {
    return;
  }
  await readJSON<Record<string, unknown>>(res);
}
