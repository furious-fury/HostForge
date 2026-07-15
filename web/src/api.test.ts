import { afterEach, describe, expect, it, vi } from "vitest"

import { APIError, api, apiRequest, unauthorizedEvent } from "@/api"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("apiRequest", () => {
  it("sends same-origin credentials and parses JSON", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }))
    vi.stubGlobal("fetch", fetchMock)

    await expect(apiRequest<{ ok: boolean }>("/api/test")).resolves.toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledWith("/api/test", expect.objectContaining({ credentials: "same-origin" }))
  })

  it("handles empty successful responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    await expect(apiRequest<null>("/api/test", { method: "DELETE" })).resolves.toBeNull()
  })

  it("normalizes field errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      status: "error", error: "validation_failed", message: "Check the fields", fields: { name: "required" },
    }), { status: 422 })))

    const error = await apiRequest("/api/test").catch((reason) => reason)
    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({ status: 422, code: "validation_failed", message: "Check the fields", fields: { name: "required" } })
  })

  it("announces unauthorized responses", async () => {
    const listener = vi.fn()
    window.addEventListener(unauthorizedEvent, listener)
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 })))

    await expect(apiRequest("/api/private")).rejects.toMatchObject({ status: 401, code: "unauthorized" })
    expect(listener).toHaveBeenCalledOnce()
    window.removeEventListener(unauthorizedEvent, listener)
  })

  it("preserves abort errors", async () => {
    const aborted = new DOMException("Aborted", "AbortError")
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(aborted))
    await expect(apiRequest("/api/test", { signal: AbortSignal.abort() })).rejects.toBe(aborted)
  })

  it("submits login tokens only in the authorization header", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ authenticated: true }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await api.login("  secret-token  ")
    expect(fetchMock).toHaveBeenCalledWith("/auth/session", expect.objectContaining({
      method: "POST",
      headers: expect.objectContaining({ Authorization: "Bearer secret-token" }),
    }))
  })
})
