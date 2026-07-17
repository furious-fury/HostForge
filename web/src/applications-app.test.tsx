import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api, type ApplicationDTO, type EnvironmentDTO, type ServiceDTO } from "@/api"
import ApplicationsApp from "@/applications-app"
import { ThemeProvider } from "@/theme-provider"
import { ToastProvider } from "@/toast-provider"

const application: ApplicationDTO = {
  id: "c3421358d5f219b18e37bcaec5609a8f",
  name: "test",
  description: "",
  archived: false,
  created_at: "2026-07-15T08:28:02Z",
  updated_at: "2026-07-15T08:28:02Z",
  environment_health: [
    { environment_id: "env-production", name: "Production", kind: "production", service_count: 0, running_count: 0, status: "empty" },
    { environment_id: "env-staging", name: "Staging", kind: "staging", service_count: 0, running_count: 0, status: "empty" },
  ],
  service_count: 0,
  healthy_service_count: 0,
  domain_count: 0,
}

const environments: EnvironmentDTO[] = application.environment_health!.map((environment) => ({
  id: environment.environment_id,
  application_id: application.id,
  name: environment.name,
  slug: environment.kind,
  kind: environment.kind as EnvironmentDTO["kind"],
  created_at: application.created_at,
  updated_at: application.updated_at,
}))

const service: ServiceDTO = {
  service_type: "application",
  id: "service-1",
  application_id: application.id,
  name: "Fundraiser",
  repo_url: "https://github.com/acme/fundraiser",
  stack_kind: "node_vite",
  stack_label: "Node.js · Vite",
  github_installation_id: 42,
  root_directory: "",
  runtime: "auto",
  install_cmd: "",
  build_cmd: "",
  start_cmd: "",
  internal_port: 3000,
  health_check_path: "/",
  created_at: application.created_at,
  updated_at: application.updated_at,
}

function mockShellAPI() {
  vi.spyOn(api, "applications").mockResolvedValue({ applications: [application] })
  vi.spyOn(api, "application").mockResolvedValue({ application, environments, services: [], service_bindings: {} })
  vi.spyOn(api, "deployments").mockResolvedValue({ deployments: [], next_cursor: "" })
  vi.spyOn(api, "hostSnapshot").mockResolvedValue({ supported: false, error_code: "unsupported_host" })
  vi.spyOn(api, "hostHistory").mockResolvedValue({ supported: false, error_code: "unsupported_host", samples: [] })
  vi.spyOn(api, "systemStatus").mockResolvedValue({ version: "v0.8.0", checks: [{ id: "docker", label: "Docker daemon", status: "RUNNING" }] })
  vi.spyOn(api, "onboarding").mockResolvedValue({ onboarding: { bootstrap_complete: false, bootstrap_enabled: false, bootstrap_expires_at: "", bootstrap_https_port: 443, bootstrap_public_ip: "", completed_at: "0001-01-01T00:00:00Z", github_app_complete: false, permanent_ingress_complete: false, platform_domain: "" } })
  vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [] })
  vi.spyOn(api, "service").mockResolvedValue({ service, bindings: [], environment_states: [] })
  vi.spyOn(api, "repositoryBranches").mockResolvedValue({ branches: ["main"], default_branch: "main" })
}

function renderApp(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <ToastProvider><ApplicationsApp /></ToastProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("application shell with a newly created empty application", () => {
  it("renders the overview", async () => {
    const user = userEvent.setup()
    mockShellAPI()
    renderApp("/")
    expect(await screen.findByRole("heading", { name: "Overview" })).toBeInTheDocument()
    expect(screen.getByText("1")).toBeInTheDocument()
    expect(api.application).not.toHaveBeenCalled()
    await user.click(screen.getByPlaceholderText("Search HostForge..."))
    await waitFor(() => expect(api.application).toHaveBeenCalledWith(application.id, expect.any(AbortSignal)))
    expect(screen.queryByText("This screen could not be rendered")).not.toBeInTheDocument()
    expect(screen.getByText("HostForge Admin")).toBeInTheDocument()
    expect(screen.getByText("Secure control session")).toBeInTheDocument()
  })

  it("renders Task Manager-style history charts for every host resource", async () => {
    mockShellAPI()
    const first = { at: "2026-07-17T08:00:00Z", cpu_pct: 12, per_core_pct: [10, 14], mem: { used_bytes: 2 * 1024 ** 3, total_bytes: 4 * 1024 ** 3, used_pct: 50 }, net: [{ iface: "eth0", rx_bps: 1024, tx_bps: 2048 }], disks: [{ mount: "/", used_bytes: 20 * 1024 ** 3, total_bytes: 40 * 1024 ** 3, used_pct: 50 }], uptime_seconds: 100, rates_ready: true }
    const current = { ...first, at: "2026-07-17T08:00:05Z", cpu_pct: 18, mem: { ...first.mem, used_pct: 52 }, net: [{ iface: "eth0", rx_bps: 4096, tx_bps: 2048 }] }
    vi.spyOn(api, "hostSnapshot").mockResolvedValue({ supported: true, sample: current })
    vi.spyOn(api, "hostHistory").mockResolvedValue({ supported: true, samples: [first, current] })

    renderApp("/")

    expect(await screen.findByRole("img", { name: "CPU host resource history" })).toBeInTheDocument()
    for (const label of ["Memory", "Root disk", "Network"]) expect(screen.getByRole("img", { name: `${label} host resource history` })).toBeInTheDocument()
    expect(api.hostHistory).toHaveBeenCalledWith(60, expect.any(AbortSignal))
  })

  it("uses database engine icons in the application overview service list", async () => {
    mockShellAPI()
    const databaseService: ServiceDTO = {
      ...service,
      id: "database-1",
      service_type: "database",
      name: "Primary database",
      repo_url: "",
      stack_kind: "postgresql",
      stack_label: "PostgreSQL",
      internal_port: 5432,
    }
    vi.spyOn(api, "application").mockResolvedValue({ application, environments, services: [databaseService], service_bindings: {} })

    renderApp(`/applications/${application.id}`)

    expect(await screen.findByRole("img", { name: "PostgreSQL database icon" })).toHaveAttribute("src", "/db/postgresql.png")
    expect(screen.getByText("HostForge managed database")).toBeInTheDocument()
  })

  it("renders the add-service GitHub prerequisite state", async () => {
	const user = userEvent.setup()
    mockShellAPI()
    renderApp(`/applications/${application.id}/services/new`)
	await user.click(await screen.findByRole("button", { name: /application service/i }))
    expect(await screen.findByText("No active GitHub installation")).toBeInTheDocument()
    const breadcrumbs = screen.getByRole("navigation", { name: "Breadcrumb" })
    expect(breadcrumbs).toHaveTextContent("Applications")
    expect(breadcrumbs).toHaveTextContent("Services")
    expect(breadcrumbs).toHaveTextContent("New service")
    expect(breadcrumbs.closest("header")).toBeNull()
  })

  it("offers operational application filters instead of raw deployment states", async () => {
    const user = userEvent.setup()
    mockShellAPI()
    renderApp("/applications")

    expect(await screen.findByRole("heading", { name: "Applications" })).toBeInTheDocument()
    for (const label of ["Production live", "Staging only", "Needs attention", "Setup needed", "Archived"]) {
      expect(screen.getByRole("button", { name: label })).toBeInTheDocument()
    }
    expect(screen.queryByRole("button", { name: "Not deployed" })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "No services" })).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Setup needed" }))
    expect(screen.getByRole("link", { name: /test/i })).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Production live" }))
    expect(screen.getByText("No applications match this view")).toBeInTheDocument()
    expect(screen.getByText("You do not currently have any applications in the production live category.")).toBeInTheDocument()
    expect(screen.queryByText("Create your first application")).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "View all applications" }))
    expect(screen.getByRole("link", { name: /test/i })).toBeInTheDocument()
  })

  it.each([
    ["Deployments", `/applications/${application.id}/deployments`],
    ["Settings", `/applications/${application.id}/settings`],
  ])("keeps application navigation visible on %s", async (activeTab, path) => {
    mockShellAPI()
    renderApp(path)

    expect(await screen.findByRole("heading", { name: activeTab === "Settings" ? "Application settings" : "Deployments" })).toBeInTheDocument()
    const tabs = screen.getByRole("tablist", { name: "Application navigation" })
    for (const label of ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]) {
      expect(within(tabs).getByRole("tab", { name: label })).toBeInTheDocument()
    }
    expect(within(tabs).getByRole("tab", { name: activeTab })).toHaveAttribute("data-state", "active")
  })

  it.each([
    ["Deployments", `/applications/${application.id}/services/${service.id}/deployments`],
    ["Settings", `/applications/${application.id}/services/${service.id}/settings`],
  ])("keeps service navigation visible on %s", async (activeTab, path) => {
    mockShellAPI()
    renderApp(path)

    expect(await screen.findByRole("heading", { name: activeTab === "Settings" ? "Service settings" : "Deployments" })).toBeInTheDocument()
    const tabs = screen.getByRole("tablist", { name: "Service navigation" })
    for (const label of ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]) {
      expect(within(tabs).getByRole("tab", { name: label })).toBeInTheDocument()
    }
    expect(within(tabs).getByRole("tab", { name: activeTab })).toHaveAttribute("data-state", "active")
  })
})
