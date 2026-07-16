import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { APIError, api, type ApplicationDTO, type EnvironmentDTO, type ServiceDTO } from "@/api"
import { AddService, ServiceOverview } from "@/services-screens"
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
  id: "service-api",
  application_id: application.id,
  name: "API",
  repo_url: "https://github.com/acme/payments",
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

function mockApplication() {
  return vi.spyOn(api, "application").mockResolvedValue({ application, environments: [environment], services: [], service_bindings: {} })
}

afterEach(() => vi.restoreAllMocks())

describe("add service", () => {
  it("renders a purposeful empty state when no active GitHub installation exists", async () => {
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [] })

    renderScreen()

    expect(await screen.findByText("No active GitHub installation")).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "Configure GitHub App" })).toHaveAttribute("href", "/onboarding")
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
    await user.type(screen.getByLabelText("Service name"), "API")
    await user.click(screen.getByRole("button", { name: "Create and deploy" }))

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    expect(create).toHaveBeenCalledWith(application.id, expect.objectContaining({
      environment_id: environment.id,
      branch: "main",
      auto_deploy: true,
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
    await user.type(screen.getByLabelText("Service name"), "API")
    await user.click(screen.getByRole("button", { name: "Create and deploy" }))

    expect(await screen.findByText(/service and branch were saved, but the first deployment could not be started/i)).toBeInTheDocument()
    expect(create).toHaveBeenCalledTimes(1)
  })

  it("shows database and cron as planned service types", async () => {
    mockApplication()
    vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [{ installation_id: 42, account_login: "acme", suspended: false }] })

    renderScreen()

    expect(await screen.findByText("Application service")).toBeInTheDocument()
    expect(screen.getByText("Database")).toBeInTheDocument()
    expect(screen.getByText("Cron job")).toBeInTheDocument()
    expect(screen.getAllByText("Planned")).toHaveLength(2)
    expect(screen.queryByRole("button", { name: /database/i })).not.toBeInTheDocument()
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
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument()
  })
})
