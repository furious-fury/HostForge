import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"

import {
  api,
  type DatabaseExternalAccessDTO,
  type DatabaseExternalConnectionDTO,
  type DatabaseInstanceDTO,
  type EnvironmentDTO,
} from "@/api"
import { DatabaseExternalAccess } from "@/database-external-access"
import { ToastProvider } from "@/toast-provider"

const timestamp = "2026-07-18T00:00:00Z"
const environments: EnvironmentDTO[] = [
  { id: "env-production", application_id: "app", name: "Production", slug: "production", kind: "production", created_at: timestamp, updated_at: timestamp },
  { id: "env-staging", application_id: "app", name: "Staging", slug: "staging", kind: "staging", created_at: timestamp, updated_at: timestamp },
]
const instances: DatabaseInstanceDTO[] = environments.map((environment, index) => ({
  id: index === 0 ? "instance-production" : "instance-staging",
  service_id: "database-service",
  environment_id: environment.id,
  engine_version: "18",
  docker_container_id: `container-${index}`,
  network_alias: `postgres-${index}`,
  internal_port: 5432,
  volume_name: `database-volume-${index}`,
  resource_preset: "standard",
  cpu_limit_millis: 1000,
  memory_limit_bytes: 2 * 1024 * 1024 * 1024,
  desired_state: "running",
  status: "healthy",
  created_at: timestamp,
  updated_at: timestamp,
}))

const activeConnection: DatabaseExternalConnectionDTO = {
  id: "connection-production",
  route_id: "route-production",
  name: "Production reporting",
  permission_profile: "read_only",
  cidrs: ["203.0.113.8/32"],
  status: "active",
  current_generation: 1,
  client_connection_limit: 20,
  credentials: [],
  created_at: timestamp,
  updated_at: timestamp,
}

function externalAccess(instance: DatabaseInstanceDTO, connections: DatabaseExternalConnectionDTO[] = []): DatabaseExternalAccessDTO {
  const suffix = instance.environment_id === "env-production" ? "prod" : "stage"
  return {
    feature_enabled: true,
    adapter_available: true,
    engine: "postgresql",
    client_ip: "203.0.113.8/32",
    external_access: {
      instance,
      route: {
        id: `route-${suffix}`,
        engine: "postgresql",
        database_instance_id: instance.id,
        route_alias: `hf_${suffix}`,
        backend_alias: `hfb_${suffix}`,
        link_network_name: `hostforge-${suffix}`,
        desired_status: "active",
        observed_status: "active",
        route_backend_limit: 25,
        credential_backend_limit: 10,
        created_at: timestamp,
        updated_at: timestamp,
      },
      connections,
    },
  }
}

function renderGateway(engine: string, selectedInstances: DatabaseInstanceDTO[] = instances) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <DatabaseExternalAccess engine={engine} instances={selectedInstances} environments={environments} />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe("database external access", () => {
  it("shows the multi-engine foundation without an unusable form", async () => {
    vi.spyOn(api, "databaseGateway").mockResolvedValue({ engine: "mysql", feature_enabled: true, adapter_available: false, unavailable_reason: "external_access_engine_unsupported" })

    renderGateway("mysql")

    expect(await screen.findByText("Multi-engine foundation ready")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /add external connection/i })).not.toBeInTheDocument()
  })

  it("leaves existing state visible but removes every mutation when the feature is disabled", async () => {
    vi.spyOn(api, "databaseGateway").mockResolvedValue({ engine: "postgresql", feature_enabled: false, adapter_available: true, reserved_hostname: "postgres.apps.example.test" })
    vi.spyOn(api, "databaseExternalAccess").mockResolvedValue(externalAccess(instances[0], [activeConnection]))

    renderGateway("postgresql", [instances[0]])

    expect(await screen.findByText("Production reporting")).toBeInTheDocument()
    expect(screen.getByText(/gateway feature flag is disabled/i)).toBeInTheDocument()
    expect(screen.getByText(/provisioned with first database/i)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /add external connection/i })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /reveal/i })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /rotate/i })).not.toBeInTheDocument()
    expect(screen.queryByRole("button", { name: /revoke/i })).not.toBeInTheDocument()
  })

  it("renders Production and Staging as isolated route cards", async () => {
    vi.spyOn(api, "databaseGateway").mockResolvedValue({ engine: "postgresql", feature_enabled: true, adapter_available: true, reserved_hostname: "postgres.apps.example.test" })
    vi.spyOn(api, "databaseExternalAccess").mockImplementation(async (instanceID) => {
      const instance = instances.find((item) => item.id === instanceID)
      if (!instance) throw new Error("unknown test instance")
      return externalAccess(instance)
    })

    renderGateway("postgresql")

    expect(await screen.findByText("hf_prod")).toBeInTheDocument()
    expect(await screen.findByText("hf_stage")).toBeInTheDocument()
    expect(screen.getByText("Production")).toBeInTheDocument()
    expect(screen.getByText("Staging")).toBeInTheDocument()
    expect(screen.getAllByRole("button", { name: /add external connection/i })).toHaveLength(2)
  })

  it("shows the previous credential grace deadline as a countdown", async () => {
    vi.spyOn(api, "databaseGateway").mockResolvedValue({ engine: "postgresql", feature_enabled: true, adapter_available: true, reserved_hostname: "postgres.apps.example.test" })
    const rotating: DatabaseExternalConnectionDTO = {
      ...activeConnection,
      status: "rotating",
      current_generation: 2,
      credentials: [{
        id: "credential-one",
        connection_id: activeConnection.id,
        username: "hfc_one",
        generation: 1,
        state: "grace",
        grace_deadline: new Date(Date.now() + 25 * 60 * 60 * 1000).toISOString(),
        created_at: timestamp,
        updated_at: timestamp,
      }],
    }
    vi.spyOn(api, "databaseExternalAccess").mockResolvedValue(externalAccess(instances[0], [rotating]))

    renderGateway("postgresql", [instances[0]])

    expect(await screen.findByText(/Grace countdown:/)).toBeInTheDocument()
    expect(screen.getByText(/hours remaining/)).toBeInTheDocument()
  })

  it("requires the typed warning before an open CIDR can be submitted", async () => {
    const user = userEvent.setup()
    vi.spyOn(api, "databaseGateway").mockResolvedValue({ engine: "postgresql", feature_enabled: true, adapter_available: true, reserved_hostname: "postgres.apps.example.test" })
    vi.spyOn(api, "databaseExternalAccess").mockResolvedValue(externalAccess(instances[0]))

    renderGateway("postgresql", [instances[0]])
    await user.click(await screen.findByRole("button", { name: /add external connection/i }))
    await user.type(screen.getByLabelText("Connection name"), "Developer laptop")
    await user.type(screen.getByLabelText(/Allowed source CIDRs/), "0.0.0.0/0")

    const submit = screen.getByRole("button", { name: "Create connection" })
    expect(submit).toBeDisabled()
    const warning = screen.getByText("This allows connection attempts from the entire internet.").closest("label")
    expect(warning).not.toBeNull()
    await user.type(within(warning!).getByRole("textbox"), "ALLOW PUBLIC ACCESS")
    expect(submit).toBeEnabled()
  })
})
