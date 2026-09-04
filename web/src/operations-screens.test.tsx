import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api, type ApplicationDTO, type EnvironmentDTO, type ServiceDTO } from "@/api"
import { DomainsScreen, EnvironmentScreen, ServiceMetrics } from "@/operations-screens"
import { formatRuntimeLogLine } from "@/runtime-log-format"
import { ToastProvider } from "@/toast-provider"

const application: ApplicationDTO = {
  id: "app-1",
  name: "Payments",
  description: "Payment services",
  archived: false,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
}

const environment: EnvironmentDTO = {
  id: "env-production",
  application_id: application.id,
  name: "Production",
  slug: "production",
  kind: "production",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
}

const service: ServiceDTO = {
  service_type: "application",
  id: "service-api",
  application_id: application.id,
  name: "API",
  repo_url: "https://github.com/acme/payments.git",
  github_installation_id: 42,
  root_directory: "",
  runtime: "auto",
  install_cmd: "",
  build_cmd: "",
  start_cmd: "",
  internal_port: 3000,
  health_check_path: "/health",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
}

function mockApplication() {
  return vi.spyOn(api, "application").mockResolvedValue({
    application,
    environments: [environment],
    services: [service],
    service_bindings: {},
  })
}

function renderScreen(node: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ToastProvider>{node}</ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("operations screens", () => {
  it("renders an empty variable scope and creates an encrypted service override", async () => {
    const user = userEvent.setup()
    mockApplication()
    vi.spyOn(api, "environmentVariables").mockResolvedValue({ variables: [] })
    const upsert = vi.spyOn(api, "upsertEnvironmentVariable").mockResolvedValue({
      variable: {
        id: "variable-1",
        application_id: application.id,
        environment_id: environment.id,
        service_id: service.id,
        key: "DATABASE_URL",
        value_last4: "prod",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    })

    renderScreen(<EnvironmentScreen scope="service" applicationID={application.id} service={service.id} />)
    expect(await screen.findByText("No variables in this scope")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Add variable" }))
    await user.type(screen.getByLabelText("Key"), "database_url")
    await user.type(screen.getByLabelText("Secret value"), "postgres://prod")
    await user.click(screen.getByRole("button", { name: "Save variable" }))

    expect(upsert).toHaveBeenCalledWith(application.id, environment.id, {
      key: "DATABASE_URL",
      value: "postgres://prod",
      service_id: service.id,
    })
    expect(await screen.findByText("DATABASE_URL saved.")).toBeInTheDocument()
  })

  it("shows a retryable domain error without substituting data", async () => {
    mockApplication()
    vi.spyOn(api, "domains").mockRejectedValue(new Error("offline"))

    renderScreen(<DomainsScreen scope="application" applicationID={application.id} />)

    expect(await screen.findByText("Domains could not be loaded")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument()
    expect(screen.queryByText("payments.example.test")).not.toBeInTheDocument()
  })

  it("creates a service-targeted domain from the empty state", async () => {
    const user = userEvent.setup()
    mockApplication()
    vi.spyOn(api, "domains").mockResolvedValue({
      domains: [],
      dns_guidance: { ipv4_source: "unknown", ipv6_source: "omitted", records: [] },
    })
    const create = vi.spyOn(api, "createDomain").mockResolvedValue({
      status: "created",
      domain: {
        id: "domain-1",
        application_id: application.id,
        environment_id: environment.id,
        service_id: service.id,
        domain_name: "payments.example.test",
        ssl_status: "PENDING",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      },
    })

    renderScreen(<DomainsScreen scope="service" applicationID={application.id} service={service.id} />)
    expect(await screen.findByText("No domains configured")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Add domain" }))
    await user.type(screen.getByLabelText("Hostname"), "payments.example.test")
    await user.click(screen.getByRole("button", { name: "Add route" }))

    expect(create).toHaveBeenCalledWith(application.id, environment.id, {
      domain_name: "payments.example.test",
      service_id: service.id,
    })
    expect(await screen.findAllByText(/payments\.example\.test added/)).toHaveLength(2)
  })

  it("does not show registrar guidance for managed platform domains", async () => {
    mockApplication()
    vi.spyOn(api, "domains").mockResolvedValue({
      domains: [{
        id: "managed-domain",
        application_id: application.id,
        environment_id: environment.id,
        service_id: service.id,
        domain_name: "clear-river.hostforge.example.com",
        kind: "platform",
        ssl_status: "ACTIVE",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      }],
      dns_guidance: {
        ipv4: "203.0.113.10",
        ipv4_source: "override",
        ipv6_source: "omitted",
        records: [{ type: "A", name: "clear-river", value: "203.0.113.10", zone_hint: "example.com" }],
      },
    })

    renderScreen(<DomainsScreen scope="application" applicationID={application.id} />)

    expect(await screen.findByText("clear-river.hostforge.example.com")).toBeInTheDocument()
    expect(screen.queryByText("DNS and routing guidance")).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Check custom DNS" })).toBeDisabled()
  })

  it("formats Caddy access JSON as readable runtime activity", () => {
    const line = JSON.stringify({
      logger: "http.log.access.log0",
      msg: "handled request",
      duration: 0.00124,
      size: 30040,
      status: 200,
      request: { method: "GET", uri: "/img/children.jpg", client_ip: "143.105.174.165" },
    })

    expect(formatRuntimeLogLine(line)).toBe("[access] 200 GET /img/children.jpg · 1 ms · 29.3 KB · 143.105.174.165")
    expect(formatRuntimeLogLine("server started")).toBe("server started")
  })

  it("requests persisted service metric ranges from the server", async () => {
    const user = userEvent.setup()
    mockApplication()
    const sample = {
      id: 1,
      service_id: service.id,
      environment_id: environment.id,
      cpu_percent: 12.5,
      memory_bytes: 64 * 1024 * 1024,
      network_rx_bytes: 1024,
      network_tx_bytes: 2048,
      sampled_at: "2026-07-15T12:00:00Z",
    }
    const metrics = vi.spyOn(api, "serviceMetrics").mockResolvedValue({
      supported: true,
      stale: false,
      sample_interval_seconds: 10,
      sample,
      samples: [sample],
    })

    renderScreen(<ServiceMetrics applicationID={application.id} service={service.id} />)
    expect(await screen.findByText("Persisted Docker resource samples for the active container.")).toBeInTheDocument()
    expect(metrics).toHaveBeenCalledWith(service.id, environment.id, 360, expect.any(AbortSignal))

    await user.click(screen.getByRole("combobox", { name: "Metric time range" }))
    await user.click(await screen.findByRole("option", { name: "2 hours" }))
    await waitFor(() => expect(metrics).toHaveBeenCalledWith(service.id, environment.id, 720, expect.any(AbortSignal)))
  })
})
