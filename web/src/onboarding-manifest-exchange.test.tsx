import { QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"
import { afterEach, expect, it, vi } from "vitest"

import { api, type OnboardingDTO, type SettingsDTO, type SystemStatusDTO } from "@/api"
import { OnboardingScreen } from "@/auth-screens"
import { queryClient } from "@/query-client"

// GitHub returns the manifest code as a query parameter, and it can be spent
// exactly once. It used to sit there until the operator noticed a second
// button and clicked it, so returning from GitHub looked like nothing had
// happened -- and navigating away or refreshing discarded the code silently,
// leaving an App on GitHub that HostForge had no credentials for.

const onboarding: OnboardingDTO = {
  bootstrap_enabled: true,
  bootstrap_public_ip: "203.0.113.10",
  bootstrap_https_port: 443,
  bootstrap_expires_at: "2026-09-04T00:00:00Z",
  github_app_complete: false,
  platform_domain: "",
  permanent_ingress_complete: false,
  bootstrap_complete: false,
  completed_at: "",
}

const settings = {
  auth: { scheme: "session" },
  build: { version: "0.9.0", version_display: "v0.9.0", commit: "abc", build_time: "", go_version: "go1.25.0", os: "linux", arch: "amd64", started_at: "", uptime_seconds: 1 },
  paths: { data_dir: "/var/lib/hostforge", logs_dir: "", db_path: "", db_size_bytes: 0, logs_dir_size_bytes: 0 },
  network: { listen: "127.0.0.1:8080", host_port: -1, port_start: 40000, port_end: 41000, container_port: 3000 },
  caddy: { root_config: "", generated_path: "", control_plane_path: "", sync_caddy: true, domain_sync_after_mutate: true, admin_url: "" },
  webhooks: { base_path: "/hooks/github", async: false, rate_limit_per_minute: 60, secret_set: true },
  dns: { server_ipv4: "203.0.113.10", detected_ipv4: "203.0.113.10", detected_ipv4_source: "config", detected_ipv4_warning: "" },
  session: { ttl_minutes: 60, cookie_secure: true, session_secret_set: true, api_token_set: true },
  health: { path: "/", timeout_ms: 1000, retries: 3, interval_ms: 500, expected_min: 200, expected_max: 399 },
  platform: { domain: "", configured: false, managed_domain_count: 0 },
} as SettingsDTO

const systemStatus: SystemStatusDTO = {
  version: "v0.9.0",
  checks: [{ id: "docker", label: "Docker", status: "READY" }],
}

function stubQueries() {
  vi.spyOn(api, "onboarding").mockResolvedValue({ onboarding })
  vi.spyOn(api, "settings").mockResolvedValue(settings)
  vi.spyOn(api, "systemStatus").mockResolvedValue(systemStatus)
  vi.spyOn(api, "applications").mockResolvedValue({ applications: [] })
  vi.spyOn(api, "githubApp").mockResolvedValue({ app: { configured: false } })
  vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [] })
}

function renderAt(path: string) {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <OnboardingScreen />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
  queryClient.clear()
})

it("exchanges the manifest code on arrival without waiting for a click", async () => {
  stubQueries()
  const exchange = vi.spyOn(api, "githubManifestExchange").mockResolvedValue({
    app: { configured: true, app_id: 1, slug: "hostforge" },
    install_url: "",
  })

  renderAt("/onboarding?code=manifest-code-123")

  // React Query passes a context object as a second argument, so assert on
  // the code itself rather than the whole call signature.
  await waitFor(() => expect(exchange).toHaveBeenCalled())
  expect(exchange.mock.calls[0][0]).toBe("manifest-code-123")
})

it("does not exchange when there is no code to spend", async () => {
  stubQueries()
  const exchange = vi.spyOn(api, "githubManifestExchange")

  renderAt("/onboarding")

  await screen.findByText(/Register the permanent platform domain/i)
  expect(exchange).not.toHaveBeenCalled()
})

// A manifest code is single-use: a second attempt always fails. Re-renders
// and React's double-invoked effects in development must not spend it twice.
it("exchanges a given code only once", async () => {
  stubQueries()
  const exchange = vi.spyOn(api, "githubManifestExchange").mockResolvedValue({
    app: { configured: true, app_id: 1, slug: "hostforge" },
    install_url: "",
  })

  const view = renderAt("/onboarding?code=manifest-code-123")
  await waitFor(() => expect(exchange).toHaveBeenCalledTimes(1))
  view.rerender(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/onboarding?code=manifest-code-123"]}>
        <OnboardingScreen />
      </MemoryRouter>
    </QueryClientProvider>,
  )

  await waitFor(() => expect(exchange).toHaveBeenCalledTimes(1))
})

it("offers a retry, and explains why, when the exchange fails", async () => {
  stubQueries()
  vi.spyOn(api, "githubManifestExchange").mockRejectedValue(new Error("exchange failed"))

  renderAt("/onboarding?code=manifest-code-123")

  await screen.findByText(/GitHub App exchange failed/i)
  expect(await screen.findByRole("button", { name: /Retry exchange/i })).toBeInTheDocument()
})
