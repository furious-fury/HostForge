export class APIError extends Error {
  constructor(public status: number, public code: string, message?: string, public fields?: Record<string, string>) {
    super(message || code)
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(init.body ? { "Content-Type": "application/json" } : {}), ...init.headers },
  })
  const payload = response.status === 204 ? null : await response.json().catch(() => null)
  if (!response.ok) {
    if (response.status === 401 && window.location.pathname !== "/login") {
      const returnTo = `${window.location.pathname}${window.location.search}`
      window.location.assign(`/login?returnTo=${encodeURIComponent(returnTo)}`)
    }
    throw new APIError(response.status, payload?.error || "request_failed", payload?.message, payload?.fields)
  }
  return payload as T
}

export type ApplicationDTO = { ID: string; Name: string; Description: string; Archived: boolean; CreatedAt: string; UpdatedAt: string }
export type EnvironmentDTO = { ID: string; ApplicationID: string; Name: string; Slug: string; Kind: "production" | "staging"; CreatedAt: string; UpdatedAt: string }
export type ServiceDTO = { ID: string; ApplicationID: string; Name: string; RepoURL: string; GitHubInstallationID: number; RootDirectory: string; DeployRuntime: string; InternalPort: number; HealthCheckPath: string }

export const api = {
  session: () => apiRequest<{ authenticated: boolean }>("/auth/session"),
  applications: () => apiRequest<{ applications: ApplicationDTO[] }>("/api/applications"),
  application: (id: string) => apiRequest<{ application: ApplicationDTO; environments: EnvironmentDTO[]; services: ServiceDTO[] }>(`/api/applications/${id}`),
  createApplication: (input: { name: string; description: string }) => apiRequest<{ application: ApplicationDTO }>("/api/applications", { method: "POST", body: JSON.stringify(input) }),
}

export const queryKeys = {
  session: ["session"] as const,
  applications: ["applications"] as const,
  application: (id: string) => ["applications", id] as const,
}
