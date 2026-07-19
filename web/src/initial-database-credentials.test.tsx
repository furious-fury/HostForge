import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { afterEach, expect, it, vi } from "vitest"

import { api, type InitialDatabaseExternalConnectionDTO } from "@/api"
import { InitialDatabaseCredentials } from "@/initial-database-credentials"
import { ToastProvider } from "@/toast-provider"

afterEach(() => {
  vi.restoreAllMocks()
})

it("waits for initial public access and automatically reveals no-store credentials", async () => {
  const timestamp = "2026-07-19T00:00:00Z"
  const entry: InitialDatabaseExternalConnectionDTO = {
    environment_id: "environment-production",
    environment_name: "Production",
    connection: {
      id: "connection-production",
      route_id: "route-production",
      name: "Production public access",
      permission_profile: "read_write",
      cidrs: ["198.51.100.42/32"],
      status: "pending",
      current_generation: 0,
      client_connection_limit: 20,
      created_at: timestamp,
      updated_at: timestamp,
    },
    operation: {
      id: "operation-production",
      engine: "postgresql",
      route_id: "route-production",
      connection_id: "connection-production",
      operation_type: "create_connection",
      status: "queued",
      progress_step: "queued",
      progress_percent: 0,
      requested_grace_period_hours: 24,
      attempt_count: 0,
      created_at: timestamp,
      updated_at: timestamp,
    },
  }
  vi.spyOn(api, "databaseGatewayOperation").mockResolvedValue({
    operation: { ...entry.operation, status: "success", progress_step: "ready", progress_percent: 100 },
  })
  const reveal = vi.spyOn(api, "revealDatabaseExternalCredentials").mockResolvedValue({
    username: "hfc_credential",
    password: "secret-password",
    database_alias: "hf_instance",
    hostname: "postgres.hostforge.example.test",
    port: 5432,
    sslmode: "verify-full",
    url: "postgresql://hfc_credential:secret-password@postgres.hostforge.example.test:5432/hf_instance?sslmode=verify-full",
    generation: 1,
  })
  const done = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })

  render(
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <InitialDatabaseCredentials entries={[entry]} onDone={done} />
      </ToastProvider>
    </QueryClientProvider>,
  )

  expect(await screen.findByText("Database credentials are ready")).toBeInTheDocument()
  expect(screen.getByText("Production")).toBeInTheDocument()
  expect(screen.getByText("secret-password")).toBeInTheDocument()
  expect(screen.getByText(/postgresql:\/\/hfc_credential/)).toBeInTheDocument()
  expect(reveal).toHaveBeenCalledWith("connection-production")
})
