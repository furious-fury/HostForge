import { afterEach, describe, expect, it, vi } from "vitest"

import { api } from "@/api"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("database gateway API client", () => {
  it("reveals credentials with POST and an explicit no-store cache policy", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      username: "hfc_credential",
      password: "secret",
      database_alias: "hf_instance",
      hostname: "postgres.apps.example.test",
      port: 5432,
      sslmode: "verify-full",
      url: "postgresql://example",
      generation: 1,
    }), { status: 200, headers: { "Content-Type": "application/json" } }))
    vi.stubGlobal("fetch", fetchMock)

    await api.revealDatabaseExternalCredentials("connection/one")

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/database-external-connections/connection%2Fone/credentials",
      expect.objectContaining({ method: "POST", cache: "no-store", credentials: "same-origin" }),
    )
  })

  it("serializes CIDR confirmation and rotation grace inputs", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "queued", operation: { id: "operation" } }), {
      status: 202,
      headers: { "Content-Type": "application/json" },
    }))
    vi.stubGlobal("fetch", fetchMock)

    await api.createDatabaseExternalConnection("instance", {
      name: "Laptop",
      profile: "read_only",
      cidrs: ["0.0.0.0/0"],
      confirm_open_access: true,
    })
    await api.databaseExternalConnectionAction("connection", "rotate", { grace_period_hours: 48 })

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/database-instances/instance/external-connections", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ name: "Laptop", profile: "read_only", cidrs: ["0.0.0.0/0"], confirm_open_access: true }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/database-external-connections/connection/rotate", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ grace_period_hours: 48 }),
    }))
  })
})
