export type ApiProject = {
  id: string;
  name: string;
  repo_url: string;
  branch: string;
  created_at: string;
  updated_at: string;
  latest_deployment?: ApiDeployment;
  domains?: ApiDomain[];
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
  created_at: string;
  updated_at: string;
};

type CreateProjectRequest = {
  repo_url: string;
  branch: string;
  project_name: string;
};

export type RepositoryBranches = {
  repo_url: string;
  branches: string[];
  default_branch: string;
};

async function readJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `request failed: ${res.status}`);
  }
  return (await res.json()) as T;
}

export async function fetchProjects(): Promise<ApiProject[]> {
  const res = await fetch("/api/projects");
  const body = await readJSON<{ projects: ApiProject[] }>(res);
  return body.projects || [];
}

export async function fetchAllDeployments(limit = 100): Promise<ApiDeployment[]> {
  const res = await fetch(`/api/deployments?limit=${limit}`);
  const body = await readJSON<{ deployments: ApiDeployment[] }>(res);
  return body.deployments || [];
}

export async function fetchProject(projectID: string): Promise<ApiProject> {
  const res = await fetch(`/api/projects/${projectID}`);
  const body = await readJSON<{ project: ApiProject }>(res);
  return body.project;
}

export async function deleteProject(projectID: string): Promise<void> {
  const res = await fetch(`/api/projects/${projectID}`, { method: "DELETE" });
  await readJSON<Record<string, unknown>>(res);
}

export async function fetchProjectDeployments(projectID: string): Promise<ApiDeployment[]> {
  const res = await fetch(`/api/projects/${projectID}/deployments?limit=100`);
  const body = await readJSON<{ deployments: ApiDeployment[] }>(res);
  return body.deployments || [];
}

export async function fetchProjectDomains(projectID: string): Promise<ApiDomain[]> {
  const res = await fetch(`/api/projects/${projectID}/domains`);
  const body = await readJSON<{ domains: ApiDomain[] }>(res);
  return body.domains || [];
}

export async function createProject(input: CreateProjectRequest): Promise<ApiProject> {
  const res = await fetch("/api/projects", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const body = await readJSON<{ project: ApiProject }>(res);
  return body.project;
}

export async function fetchRepositoryBranches(repoURL: string): Promise<RepositoryBranches> {
  const qs = new URLSearchParams({ repo_url: repoURL }).toString();
  const res = await fetch(`/api/repositories/branches?${qs}`);
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
  const res = await fetch(`/api/projects/${projectID}/deploy${qs}`, { method: "POST" });
  return await readJSON<{ deployment_id?: string; status?: string; mode?: string; url?: string; error?: string }>(res);
}

export async function restartProject(projectID: string): Promise<void> {
  const res = await fetch(`/api/projects/${projectID}/restart`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

export async function rollbackProject(projectID: string): Promise<void> {
  const res = await fetch(`/api/projects/${projectID}/rollback`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

export async function stopProject(projectID: string): Promise<void> {
  const res = await fetch(`/api/projects/${projectID}/stop`, { method: "POST" });
  await readJSON<Record<string, unknown>>(res);
}

export async function fetchDeploymentLogs(deploymentID: string, source: "build" | "container"): Promise<string> {
  const params = source === "build" ? "" : "?tail_lines=200";
  const res = await fetch(`/api/deployments/${deploymentID}/logs${params}`);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `logs request failed: ${res.status}`);
  }
  return await res.text();
}
