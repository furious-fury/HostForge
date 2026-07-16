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
    await screen.findByText(/Ready to deploy/)
    await user.type(screen.getByLabelText("Service name"), "API")
    await user.click(screen.getByRole("button", { name: "Create and deploy" }))

    expect(await screen.findByText(/service and branch were saved, but the first deployment could not be started/i)).toBeInTheDocument()
    expect(create).toHaveBeenCalledTimes(1)
  })
})

describe("service overview", () => {
  it("does not present failed deployment and domain requests as empty states", async () => {
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
    vi.spyOn(api, "domains").mockRejectedValue(new APIError(503, "domains_unavailable"))

    renderOverview()

    expect(await screen.findByText("Deployment history could not be loaded.")).toBeInTheDocument()
    expect(screen.getByText("Domain routes could not be loaded.")).toBeInTheDocument()
    expect(screen.queryByText("Deploy this environment to create its first release.")).not.toBeInTheDocument()
    expect(screen.queryByText("No public routes configured.")).not.toBeInTheDocument()
  })
})
