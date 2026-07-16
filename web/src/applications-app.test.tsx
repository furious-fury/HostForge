import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { MemoryRouter } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { api, type ApplicationDTO, type EnvironmentDTO } from "@/api"
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

function mockShellAPI() {
  vi.spyOn(api, "applications").mockResolvedValue({ applications: [application] })
  vi.spyOn(api, "application").mockResolvedValue({ application, environments, services: [], service_bindings: {} })
  vi.spyOn(api, "deployments").mockResolvedValue({ deployments: [], next_cursor: "" })
  vi.spyOn(api, "hostSnapshot").mockResolvedValue({ supported: false, error_code: "unsupported_host" })
  vi.spyOn(api, "systemStatus").mockResolvedValue({ version: "v0.8.0", checks: [{ id: "docker", label: "Docker daemon", status: "RUNNING" }] })
  vi.spyOn(api, "onboarding").mockResolvedValue({ onboarding: { bootstrap_complete: false, bootstrap_enabled: false, bootstrap_expires_at: "", bootstrap_https_port: 443, bootstrap_public_ip: "", completed_at: "0001-01-01T00:00:00Z", github_app_complete: false, permanent_ingress_complete: false, platform_domain: "" } })
  vi.spyOn(api, "githubInstallations").mockResolvedValue({ installations: [] })
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
  })

  it("renders the add-service GitHub prerequisite state", async () => {
    mockShellAPI()
    renderApp(`/applications/${application.id}/services/new`)
    expect(await screen.findByText("No active GitHub installation")).toBeInTheDocument()
    const breadcrumbs = screen.getByRole("navigation", { name: "Breadcrumb" })
    expect(breadcrumbs).toHaveTextContent("Applications")
    expect(breadcrumbs).toHaveTextContent("Services")
    expect(breadcrumbs).toHaveTextContent("New service")
    expect(breadcrumbs.closest("header")).toBeNull()
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
})
