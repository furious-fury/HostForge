import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
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

function mockDatabaseCatalog() {
  vi.spyOn(api, "databaseEngines").mockResolvedValue({
    engines: [{
      id: "postgresql", name: "PostgreSQL", description: "Relational database", category: "Relational",
      versions: [{ version: "18", default: true, provisioning_available: true }], internal_port: 5432,
      connection_variable: "DATABASE_URL", minimum_memory_bytes: 512 * 1024 * 1024, public_access_available: false,
    }],
    resource_presets: [{ id: "development", name: "Development", description: "Low traffic", cpu_limit_millis: 500, memory_limit_bytes: 512 * 1024 * 1024 }],
    networking: { scope: "hostforge_environment", public_access_available: false },
  })
  vi.spyOn(api, "backupDestinations").mockResolvedValue({ destinations: [] })
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
      }, {
        id: "custom",
        name: "Custom",
        description: "Choose exact limits",
        cpu_limit_millis: 0,
        memory_limit_bytes: 0,
      }],
      resource_capacity: {
        available: true,
        cpu_total_millis: 4000,
        cpu_allocated_millis: 0,
        cpu_reserve_millis: 400,
        cpu_available_millis: 3600,
        memory_total_bytes: 4 * 1024 ** 3,
        memory_allocated_bytes: 0,
        memory_reserve_bytes: 1024 ** 3,
        memory_available_bytes: 3 * 1024 ** 3,
      },
      networking: { scope: "hostforge_environment", public_access_available: false },
    })

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure database/i }))

    expect(await screen.findByText("Environment isolation")).toBeInTheDocument()
    expect(screen.getByRole("img", { name: "PostgreSQL database icon" })).toHaveAttribute("src", "/db/postgresql.png")
    expect(screen.getByText(/own container, volume, credentials/i)).toBeInTheDocument()
    expect(screen.getByText("Private by default")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /create database/i })).toBeEnabled()
    await user.click(screen.getByRole("button", { name: /^custom/i }))
    expect(screen.getByRole("slider", { name: "CPU allocation" })).toHaveAttribute("aria-valuenow", "2")
    expect(screen.getByRole("slider", { name: "CPU allocation" })).toHaveAttribute("aria-valuemax", "3.6")
    expect(screen.getByRole("slider", { name: "Memory allocation" })).toHaveAttribute("aria-valuenow", "2")
    expect(screen.getByRole("slider", { name: "Memory allocation" })).toHaveAttribute("aria-valuemax", "3")
    expect(screen.getByText(/3.6 allocatable vCPU remains/i)).toBeInTheDocument()
    expect(github).not.toHaveBeenCalled()
  })

  it("blocks a known duplicate database service name with actionable guidance", async () => {
    const user = userEvent.setup()
    const existingDatabase: ServiceDTO = { ...service, id: "database-1", service_type: "database", name: "database", repo_url: "", runtime: "database" }
    vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment], services: [existingDatabase], service_bindings: {} })
    mockDatabaseCatalog()

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure database/i }))

    expect(await screen.findByText("A service with this name already exists in this application. Choose a different name.")).toBeInTheDocument()
    expect(screen.getByLabelText("Service name")).toHaveAttribute("aria-invalid", "true")
    expect(screen.getByRole("button", { name: /^create database$/i })).toBeDisabled()
  })

  it("explains a duplicate name returned by the database API", async () => {
    const user = userEvent.setup()
    mockApplication()
    mockDatabaseCatalog()
    vi.spyOn(api, "createDatabaseService").mockRejectedValue(new APIError(409, "database_service_name_conflict"))

    renderScreen()
    await user.click(await screen.findByRole("button", { name: /configure database/i }))
    await user.click(await screen.findByRole("button", { name: /^create database$/i }))

    expect(await screen.findByText("A service named “database” already exists in this application. Choose a different service name.")).toBeInTheDocument()
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

  it("renders distinct database tab views and loads diagnostics on demand", async () => {
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
	expect(screen.getByRole("region", { name: "Database instance overview" })).not.toHaveClass("xl:grid-cols-2")
	expect(screen.getByRole("img", { name: "PostgreSQL database icon" })).toHaveAttribute("src", "/db/postgresql.png")
	const databaseNavigation = screen.getByRole("tablist", { name: "Database service navigation" })
	expect(databaseNavigation).toHaveTextContent("Backups")
	expect(databaseNavigation).toHaveTextContent("Data & connections")
	expect(databaseNavigation).toHaveTextContent("Metrics")
	expect(databaseNavigation).toHaveTextContent("Logs")
	expect(databaseNavigation).toHaveTextContent("Settings")
    expect(logs).not.toHaveBeenCalled()
    expect(metrics).not.toHaveBeenCalled()

    await user.click(within(databaseNavigation).getByRole("tab", { name: "Data & connections" }))
    expect(await screen.findByText("primary_a1b2c3d4")).toBeInTheDocument()
    expect(screen.getByRole("region", { name: "Database data and connections" }).querySelector(".xl\\:grid-cols-2")).toBeNull()
    expect(screen.getByText("hf_primary_a1b2c3d4")).toBeInTheDocument()
    expect(screen.queryByRole("log", { name: "Database logs" })).not.toBeInTheDocument()

    await user.click(within(databaseNavigation).getByRole("tab", { name: "Logs" }))

    expect(await screen.findByText("database system is ready")).toBeInTheDocument()
    expect(logs).toHaveBeenCalledWith("db-instance-1", 200, expect.any(AbortSignal))
	  expect(metrics).not.toHaveBeenCalled()

	  await user.click(within(databaseNavigation).getByRole("tab", { name: "Metrics" }))
	  expect((await screen.findAllByText("128.0 MB")).length).toBeGreaterThan(0)
    expect(screen.getByRole("img", { name: /CPU usage live trend/i })).toBeInTheDocument()
    expect(screen.getByRole("img", { name: /Memory usage live trend/i })).toBeInTheDocument()
    expect(screen.getByRole("img", { name: /Network ingress live trend/i })).toBeInTheDocument()
    expect(screen.getByRole("img", { name: /Network egress live trend/i })).toBeInTheDocument()
    expect(metrics).toHaveBeenCalled()
  })

  it("shows the retained operation error when an older failed provision has no container logs", async () => {
    const user = userEvent.setup()
    const databaseService: ServiceDTO = { ...service, service_type: "database", name: "Failed database", repo_url: "", stack_kind: "postgresql", stack_label: "PostgreSQL" }
    vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment], services: [databaseService], service_bindings: {} })
    vi.spyOn(api, "service").mockResolvedValue({
      service: databaseService,
      database: { service_id: databaseService.id, engine: "postgresql", default_version: "18" },
      database_instances: [{
        id: "db-instance-failed", service_id: databaseService.id, environment_id: environment.id,
        engine_version: "18", docker_container_id: "", network_alias: "failed-production",
        internal_port: 5432, volume_name: "hostforge-db-failed", resource_preset: "development",
        cpu_limit_millis: 500, memory_limit_bytes: 512 * 1024 * 1024, desired_state: "running",
        status: "failed", health_message: "database_engine_configuration_failed", created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T00:00:00Z",
      }],
      database_operations: [{
        id: "operation-failed", service_id: databaseService.id, database_instance_id: "db-instance-failed",
        operation_type: "provision", status: "failed", progress_step: "failed", progress_percent: 90,
        error_code: "database_engine_configuration_failed", error_message: "PostgreSQL application-user setup exited with code 2",
        created_at: "2026-07-01T00:00:00Z", updated_at: "2026-07-01T00:00:00Z",
      }],
      database_bindings: {}, database_credentials: {}, bindings: [], environment_states: [],
    })
    const logs = vi.spyOn(api, "databaseLogs")

    renderOverview()
    const databaseNavigation = await screen.findByRole("tablist", { name: "Database service navigation" })
    await user.click(within(databaseNavigation).getByRole("tab", { name: "Logs" }))

    expect(await screen.findByText("PostgreSQL application-user setup exited with code 2")).toBeInTheDocument()
    expect(screen.getByText(/failed container was removed by the earlier provisioning workflow/i)).toBeInTheDocument()
    expect(logs).not.toHaveBeenCalled()
  })
})

describe("services list", () => {
  it("separates retained databases from active services", async () => {
    const user = userEvent.setup()
    const deletedDatabase: ServiceDTO = { ...service, id: "database-deleted", service_type: "database", name: "Old Redis", repo_url: "", runtime: "database", stack_kind: "redis", stack_label: "Redis", internal_port: 6379 }
    const failedDatabase: ServiceDTO = { ...service, id: "database-failed", service_type: "database", name: "Failed MySQL", repo_url: "", runtime: "database", stack_kind: "mysql", stack_label: "MySQL", internal_port: 3306 }
    vi.spyOn(api, "application").mockResolvedValue({
      application,
      environments: [environment],
      services: [service, failedDatabase, deletedDatabase],
      service_bindings: { [service.id]: [] },
      database_instances: {
        [failedDatabase.id]: [{
          id: "instance-failed", service_id: failedDatabase.id, environment_id: environment.id, engine_version: "8.4",
          network_alias: "failed-mysql-production", internal_port: 3306, volume_name: "hostforge-db-failed-mysql", resource_preset: "development",
          cpu_limit_millis: 500, memory_limit_bytes: 512 * 1024 * 1024, desired_state: "running", status: "failed",
          created_at: "2026-07-17T09:00:00Z", updated_at: "2026-07-17T09:00:00Z",
        }],
        [deletedDatabase.id]: [{
          id: "instance-deleted", service_id: deletedDatabase.id, environment_id: environment.id, engine_version: "8",
          network_alias: "old-redis-production", internal_port: 6379, volume_name: "hostforge-db-old-redis", resource_preset: "development",
          cpu_limit_millis: 500, memory_limit_bytes: 512 * 1024 * 1024, desired_state: "deleted", status: "deleted",
          purge_after: "2026-07-24T09:00:00Z", created_at: "2026-07-17T09:00:00Z", updated_at: "2026-07-17T09:00:00Z",
        }],
      },
    })

    renderServicesList()

    expect(await screen.findByRole("link", { name: /API/ })).toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /Failed MySQL/ })).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /Old Redis/ })).not.toBeInTheDocument()
    const categories = screen.getByRole("tablist", { name: "Service categories" })
    await user.click(within(categories).getByRole("tab", { name: /Failed/ }))
    expect(await screen.findByRole("link", { name: /Failed MySQL/ })).toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /API/ })).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /Old Redis/ })).not.toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Failed services" })).toBeInTheDocument()
    await user.click(within(categories).getByRole("tab", { name: /Deleted/ }))
    expect(await screen.findByRole("link", { name: /Old Redis/ })).toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /API/ })).not.toBeInTheDocument()
    expect(screen.queryByRole("link", { name: /Failed MySQL/ })).not.toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Deleted databases" })).toBeInTheDocument()
  })

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
