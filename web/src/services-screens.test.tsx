import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { APIError, api, type ApplicationDTO, type EnvironmentDTO, type ServiceDTO } from "@/api"
import { AddService, ServiceOverview, ServicesList } from "@/services-screens"
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
  repo_url: "https://github.com/acme/payments",
  stack_kind: "node_vite",
  stack_label: "Node.js · Vite",
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

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ToastProvider><AddService applicationID={application.id} /></ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function renderOverview() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ToastProvider><ServiceOverview applicationID={application.id} service={service.id} /></ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function renderServicesList() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ToastProvider><ServicesList applicationID={application.id} /></ToastProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function mockApplication() {
  return vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment], services: [], service_bindings: {} })
}

afterEach(() => vi.restoreAllMocks())

describe("add service", () => {
  it("renders a purposeful empty state when no active GitHub installation exists", async () => {
    const user = userEvent.setup()
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [] })

    renderScreen()

    await user.click(await screen.findByRole("button", { name: /configure service/i }))
    expect(await screen.findByText("No active GitHub installation")).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Configure GitHub App" })).toHaveAttribute("href", "/onboarding")
  })

  it("opens the private database wizard without requiring GitHub", async () => {
    const user = userEvent.setup()
    mockApplication()
    const github = vi.spyOn(api, "githubInstallations")
    vi.spyOn(api, "databaseEngines").mockResolvedValue({
      engines: [{
        id: "postgresql",
        name: "PostgreSQL",
        description: "Relational database",
        category: "Relational",
        versions: [{ version: "18", default: true, provisioning_available: true }],
        internal_port: 5432,
        connection_variable: "DATABASE_URL",
        minimum_memory_bytes: 512 * 1024 * 1024,
        public_access_available: false,
      }],
      resource_presets: [{
        id: "development",
        name: "Development",
        description: "Low traffic",
        cpu_limit_millis: 500,
        memory_limit_bytes: 512 * 1024 * 1024,
      }],
      networking: { scope: "hostforge_environment", public_access_available: false },
    })

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure database/i }))

    expect(await screen.findByText("Environment isolation")).toBeInTheDocument()
    expect(screen.getByText(/own container, volume, credentials/i)).toBeInTheDocument()
    expect(screen.getByText("Private by default")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /create database/i })).toBeEnabled()
    expect(github).not.toHaveBeenCalled()
  })

  it("creates a deployable service and starts its first deployment", async () => {
    const user = userEvent.setup()
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [{ installation_id: 42, account_login: "acme", suspended: false }] })
    vi.spyOn(api, "githubRepositories").mockResolvedValue({ repositories: [{ id: 7, full_name: "acme/payments", private: true, default_branch: "main", clone_url: "https://github.com/acme/payments.git" }] })
    vi.spyOn(api, "repositoryBranches").mockResolvedValue({ branches: ["main"], default_branch: "main" })
    const create = vi.spyOn(api, "createService").mockResolvedValue({ service })
    const deploy = vi.spyOn(api, "deploy").mockResolvedValue({ deployment: {
      id: "deployment-1",
      service_id: service.id,
      environment_id: environment.id,
      status: "QUEUED",
      commit_hash: "",
      trigger: "manual",
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
    } })

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure service/i }))
    expect(await screen.findByText(/Ready to deploy/)).toBeInTheDocument()
    expect(screen.getByLabelText("Service name")).toHaveValue("payments")
    await user.click(screen.getByRole("button", { name: "Create and deploy" }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create).toHaveBeenCalledWith(application.id, expect.objectContaining({
      environment_id: environment.id,
      branch: "main",
      auto_deploy: true,
      name: "payments",
    }))
    expect(deploy).toHaveBeenCalledWith(service.id, environment.id)
  })

  it("reports partial success without recreating the service when deployment cannot start", async () => {
    const user = userEvent.setup()
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [{ installation_id: 42, account_login: "acme", suspended: false }] })
    vi.spyOn(api, "githubRepositories").mockResolvedValue({ repositories: [{ id: 7, full_name: "acme/payments", private: true, default_branch: "main", clone_url: "https://github.com/acme/payments.git" }] })
    vi.spyOn(api, "repositoryBranches").mockResolvedValue({ branches: ["main"], default_branch: "main" })
    const create = vi.spyOn(api, "createService").mockResolvedValue({ service })
    vi.spyOn(api, "deploy").mockRejectedValue(new APIError(503, "docker_unavailable"))

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure service/i }))
    await screen.findByText(/Ready to deploy/)
    await user.clear(screen.getByLabelText("Service name"))
    await user.type(screen.getByLabelText("Service name"), "API")
    await user.click(screen.getByRole("button", { name: "Create and deploy" }))

    expect(await screen.findByText(/service and branch were saved, but the first deployment could not be started/i)).toBeInTheDocument()
    expect(create).toHaveBeenCalledTimes(1)
  })

  it("offers the database wizard while keeping cron jobs marked as planned", async () => {
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [{ installation_id: 42, account_login: "acme", suspended: false }] })

    renderScreen()

    expect(await screen.findByText("Application service")).toBeInTheDocument()
    expect(screen.getByText("Database")).toBeInTheDocument()
    expect(screen.getByText("Cron job")).toBeInTheDocument()
    expect(screen.getAllByText("Planned")).toHaveLength(1)
    expect(screen.getByRole("button", { name: /configure database/i })).toBeInTheDocument()
  })
})

describe("service overview", () => {
  it("does not present a failed deployment request as an empty state", async () => {
    mockApplication()
    vi.spyOn(api, "service").mockResolvedValue({
      service,
      bindings: [{
        service_id: service.id,
        environment_id: environment.id,
        branch: "main",
        auto_deploy: true,
        active_deployment_id: "",
        desired_state: "running",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      }],
      environment_states: [],
    })
    vi.spyOn(api, "deployments").mockRejectedValue(new APIError(503, "deployments_unavailable"))

    renderOverview()

    expect(await screen.findByText("Deployment history could not be loaded.")).toBeInTheDocument()
    expect(screen.queryByText("Deploy this environment to create its first release.")).not.toBeInTheDocument()
  })

  it("shows active deployments and URLs for every environment without a selector", async () => {
    const staging = { ...environment, id: "env-staging", name: "Staging", slug: "staging", kind: "staging" as const }
    vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment, staging], services: [service], service_bindings: {} })
    vi.spyOn(api, "service").mockResolvedValue({
      service,
      bindings: [environment, staging].map((item) => ({
        service_id: service.id,
        environment_id: item.id,
        branch: item.kind === "staging" ? "develop" : "main",
        auto_deploy: true,
        active_deployment_id: item.kind === "staging" ? "deployment-staging" : "",
        desired_state: "running" as const,
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      })),
      environment_states: [{
        service_id: service.id,
        environment_id: staging.id,
        branch: "develop",
        auto_deploy: true,
        active_deployment_id: "deployment-staging",
        desired_state: "running",
        environment_name: "Staging",
        environment_kind: "staging",
        public_url: "https://staging.payments.example.com",
        created_at: "2026-07-01T00:00:00Z",
        updated_at: "2026-07-01T00:00:00Z",
      }],
    })
    vi.spyOn(api, "deployments").mockResolvedValue({ deployments: [], next_cursor: "" })

    renderOverview()

    expect(await screen.findByRole("link", { name: /staging\.payments\.example\.com/i })).toHaveAttribute("href", "https://staging.payments.example.com")
    expect(screen.getByText("Production")).toBeInTheDocument()
    expect(screen.getByText("Staging")).toBeInTheDocument()
    expect(screen.getAllByRole("img", { name: "Node.js · Vite stack icon" }).length).toBeGreaterThan(0)
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument()
  })

  it("loads database logs and metrics only when diagnostics are opened", async () => {
    const user = userEvent.setup()
    const databaseService: ServiceDTO = { ...service, service_type: "database", name: "Primary database", repo_url: "", stack_kind: "postgresql", stack_label: "PostgreSQL" }
    vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment], services: [databaseService], service_bindings: {} })
    vi.spyOn(api, "service").mockResolvedValue({
      service: databaseService,
      database: { service_id: databaseService.id, engine: "postgresql", default_version: "18" },
      database_instances: [{
        id: "db-instance-1", service_id: databaseService.id, environment_id: environment.id,
        engine_version: "18", docker_container_id: "container-1", network_alias: "primary-production",
        internal_port: 5432, volume_name: "hostforge-db-primary", resource_preset: "development",
        cpu_limit_millis: 500, memory_limit_bytes: 512 * 1024 * 1024, desired_state: "running",
        status: "healthy", health_message: "ready", created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T00:00:00Z",
      }],
      database_bindings: {}, database_credentials: { "db-instance-1": { database_instance_id: "db-instance-1", database_name: "primary_a1b2c3d4", username: "hf_primary_a1b2c3d4", generation: 1, created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T00:00:00Z" } }, database_operations: [], bindings: [], environment_states: [],
    })
    const logs = vi.spyOn(api, "databaseLogs").mockResolvedValue({ instance_id: "db-instance-1", logs: "database system is ready\ncheckpoint complete\n" })
    const metrics = vi.spyOn(api, "databaseMetrics").mockResolvedValue({ instance_id: "db-instance-1", metric: { cpu_percent: 2.5, memory_bytes: 128 * 1024 * 1024, network_rx_bytes: 1024, network_tx_bytes: 2048, sampled_at: "2026-07-01T00:00:00Z" } })
    vi.spyOn(api, "databaseUpgradePreflight").mockResolvedValue({ available: false, ready: false, reason: "catalog_image_current", engine_version: "18", current_image_ref: "postgres@sha256:current", target_image_ref: "postgres@sha256:current", backup_max_age_hours: 24 })

    renderOverview()
    expect(await screen.findByText("Primary database")).toBeInTheDocument()
	expect(screen.getByText("primary_a1b2c3d4")).toBeInTheDocument()
	expect(screen.getByText("hf_primary_a1b2c3d4")).toBeInTheDocument()
	const databaseNavigation = screen.getByRole("tablist", { name: "Database service navigation" })
	expect(databaseNavigation).toHaveTextContent("Backups")
	expect(databaseNavigation).toHaveTextContent("Data & connections")
	expect(databaseNavigation).toHaveTextContent("Metrics")
	expect(databaseNavigation).toHaveTextContent("Logs")
	expect(databaseNavigation).toHaveTextContent("Settings")
    expect(logs).not.toHaveBeenCalled()
    await user.click(screen.getByRole("button", { name: /logs and resource usage/i }))

    expect(await screen.findByText("database system is ready")).toBeInTheDocument()
    expect(await screen.findByText("128.0 MB")).toBeInTheDocument()
    expect(logs).toHaveBeenCalledWith("db-instance-1", 200, expect.any(AbortSignal))
    expect(metrics).toHaveBeenCalled()
  })
})

describe("services list", () => {
  it("shows an active staging binding instead of an empty production binding", async () => {
    const staging = { ...environment, id: "env-staging", name: "Staging", slug: "staging", kind: "staging" as const }
    vi.spyOn(api, "application").mockResolvedValue({
      application,
      environments: [environment, staging],
      services: [service],
      service_bindings: {
        [service.id]: [{
          service_id: service.id,
          environment_id: environment.id,
          branch: "",
          auto_deploy: false,
          active_deployment_id: "",
          desired_state: "running",
          created_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        }, {
          service_id: service.id,
          environment_id: staging.id,
          branch: "develop",
          auto_deploy: true,
          active_deployment_id: "deployment-staging",
          desired_state: "running",
          created_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        }],
      },
    })

    renderServicesList()

    expect(await screen.findByText("develop")).toBeInTheDocument()
    expect(screen.getByText("deployment-staging")).toBeInTheDocument()
    expect(screen.getAllByText("Staging")).toHaveLength(2)
    expect(screen.getAllByText("Running").length).toBeGreaterThan(0)
    expect(screen.queryByText("Configuration required")).not.toBeInTheDocument()
    expect(screen.getAllByRole("img", { name: "Node.js · Vite stack icon" }).length).toBeGreaterThan(0)
  })
})
