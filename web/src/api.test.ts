import { afterEach, describe, expect, it, vi } from "vitest"

import { APIError, api, apiRequest, unauthorizedEvent } from "@/api"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("apiRequest", () => {
  it("sends same-origin credentials and parses JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }))
    vi.stubGlobal("fetch", fetchMock)

    await expect(apiRequest<{ ok: boolean }>("/api/test")).resolves.toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledWith("/api/test", expect.objectContaining({ credentials: "same-origin" }))
  })

  it("handles empty successful responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    await expect(apiRequest<null>("/api/test", { method: "DELETE" })).resolves.toBeNull()
  })

  it("normalizes field errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      status: "error", error: "validation_failed", message: "Check the fields", fields: { name: "required" },
    }), { status: 422 })))

    const error = await apiRequest("/api/test").catch((reason) => reason)
    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 422, code: "validation_failed", message: "Check the fields", fields: { name: "required" } })
  })

  it("preserves structured operation details on API errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      status: "error", error: "caddy_sync_failed", caddy_sync: { attempted: true, ok: false, error: "caddy_validate_failed" },
    }), { status: 502 })))

    const error = await apiRequest("/api/test").catch((reason) => reason)
    expect(error).toMatchObject({
      code: "caddy_sync_failed",
      details: { caddy_sync: { attempted: true, ok: false, error: "caddy_validate_failed" } },
    })
  })

  it("announces unauthorized responses", async () => {
    const listener = vi.fn()
    window.addEventListener(unauthorizedEvent, listener)
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 })))

    await expect(apiRequest("/api/private")).rejects.toMatchObject({ status: 401, code: "unauthorized" })
    expect(listener).toHaveBeenCalledOnce()
    window.removeEventListener(unauthorizedEvent, listener)
  })

  it("preserves abort errors", async () => {
    const aborted = new DOMException("Aborted", "AbortError")
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(aborted))
    await expect(apiRequest("/api/test", { signal: AbortSignal.abort() })).rejects.toBe(aborted)
  })

  it("submits login tokens only in the authorization header", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ authenticated: true }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await api.login("  secret-token  ")
    expect(fetchMock).toHaveBeenCalledWith("/auth/session", expect.objectContaining({
      method: "POST",
      headers: expect.objectContaining({ Authorization: "Bearer secret-token" }),
    }))
  })

  it("normalizes nullable application collections from empty server results", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      application: { id: "app", name: "Empty application" },
      environments: null,
      services: null,
      service_bindings: null,
    }), { status: 200 })))

    await expect(api.application("app")).resolves.toMatchObject({
      environments: [],
      services: [],
      service_bindings: {},
    })
  })

  it("normalizes nullable collections across empty server-backed screens", async () => {
    const payloads: Record<string, Record<string, unknown>> = {
      "/api/services/service": { service: { id: "service" }, bindings: null, environment_states: null },
      "/api/services/service/environments/environment/metrics?points=120": { supported: true, samples: null },
      "/api/observability/requests": { requests: null, next_cursor: "" },
      "/api/observability/deploy-steps": { deploy_steps: null, next_cursor: "" },
      "/api/system/host/history?points=120": { supported: true, samples: null },
      "/api/events": { events: null, next_cursor: "" },
      "/api/github/installations": { installations: null },
      "/api/github/installations/sync": { installations: null },
      "/api/github/installations/42/repositories": { repositories: null },
      "/api/repositories/branches?repo_url=https%3A%2F%2Fgithub.com%2Facme%2Fapp.git&installation_id=42": { branches: null, default_branch: "" },
      "/api/applications/application/environments/environment/domains": { domains: null, dns_guidance: { records: [] } },
      "/api/applications/application/environments/environment/variables": { variables: null },
      "/api/deployments": { deployments: null, next_cursor: "" },
      "/api/deployments/deployment/steps": { deployment_id: "deployment", steps: null },
    }
    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: string) => (
      Promise.resolve(new Response(JSON.stringify(payloads[input]), { status: 200 }))
    )))

    const [
      service,
      metrics,
      requests,
      deploySteps,
      history,
      events,
      installations,
      syncedInstallations,
      repositories,
      branches,
      domains,
      variables,
      deployments,
      deploymentSteps,
    ] = await Promise.all([
      api.service("service"),
      api.serviceMetrics("service", "environment"),
      api.observabilityRequests(),
      api.observabilityDeploySteps(),
      api.hostHistory(),
      api.events(),
      api.githubInstallations(),
      api.syncGitHubInstallations(),
      api.githubRepositories(42),
      api.repositoryBranches("https://github.com/acme/app.git", 42),
      api.domains("application", "environment"),
      api.environmentVariables("application", "environment"),
      api.deployments(),
      api.deploymentSteps("deployment"),
    ])

    expect(service).toMatchObject({ bindings: [], environment_states: [] })
    expect(metrics.samples).toEqual([])
    expect(requests.requests).toEqual([])
    expect(deploySteps.deploy_steps).toEqual([])
    expect(history.samples).toEqual([])
    expect(events.events).toEqual([])
    expect(installations.installations).toEqual([])
    expect(syncedInstallations.installations).toEqual([])
    expect(repositories.repositories).toEqual([])
    expect(branches.branches).toEqual([])
    expect(domains.domains).toEqual([])
    expect(variables.variables).toEqual([])
    expect(deployments.deployments).toEqual([])
    expect(deploymentSteps.steps).toEqual([])
  })

  it("requests operator-triggered DNS checks without changing normal domain reads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ domains: [], dns_guidance: { records: [] } }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await api.domains("app one", "production", "service/one", undefined, true)
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/applications/app%20one/environments/production/domains?service_id=service%2Fone&check_dns=1",
      expect.anything(),
    )
  })

  it("serializes the complete deployment filter contract", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ deployments: [], next_cursor: "" }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await api.deployments({ applicationID: "app", serviceID: "svc", environmentID: "env", status: "SUCCESS", trigger: "webhook", branch: "release/v2", dateFrom: "2026-07-01T00:00:00Z", dateTo: "2026-07-15T23:59:59Z", cursor: "cursor", limit: 50 })
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/deployments?application_id=app&service_id=svc&environment_id=env&status=SUCCESS&trigger=webhook&branch=release%2Fv2&date_from=2026-07-01T00%3A00%3A00Z&date_to=2026-07-15T23%3A59%3A59Z&cursor=cursor&limit=50",
      expect.anything(),
    )
  })

  it("disconnects GitHub App credentials with an authenticated DELETE request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "deleted" }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await api.deleteGitHubApp()
    expect(fetchMock).toHaveBeenCalledWith("/api/github/app", expect.objectContaining({ method: "DELETE", credentials: "same-origin" }))
  })

  it("exchanges GitHub manifest codes through the documented v2 route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      app: { configured: true, app_id: 42, slug: "hostforge" },
      install_url: "https://github.com/apps/hostforge/installations/new",
    }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await api.githubManifestExchange("one-time-code")
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/github/app/manifest/exchange",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ code: "one-time-code" }),
        credentials: "same-origin",
      }),
    )
  })

  it("preserves routing warnings from successful resource deletions", async () => {
    const outcome = { status: "deleted" as const, routing_warning: "caddy_reload_failed" }
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(outcome), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await expect(api.deleteApplication("app one")).resolves.toEqual(outcome)
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/applications/app%20one",
      expect.objectContaining({ method: "DELETE", credentials: "same-origin" }),
    )
  })

  it("updates an environment through its application-scoped endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "ok", environment: { id: "env" } }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)

    await api.updateEnvironment("app one", "env/staging", { name: "Preview" })
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/applications/app%20one/environments/env%2Fstaging",
      expect.objectContaining({ method: "PATCH", body: JSON.stringify({ name: "Preview" }), credentials: "same-origin" }),
    )
  })
})
