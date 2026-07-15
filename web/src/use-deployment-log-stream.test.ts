import { act, renderHook } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useDeploymentLogStream } from "@/use-deployment-log-stream"

class FakeWebSocket {
  static instances: FakeWebSocket[] = []

  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null

  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }

  close() {}
}

afterEach(() => {
  FakeWebSocket.instances = []
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("useDeploymentLogStream", () => {
  it("reconnects build logs from the last byte cursor", () => {
    vi.useFakeTimers()
    vi.stubGlobal("WebSocket", FakeWebSocket)
    const { result } = renderHook(() => useDeploymentLogStream("deploy one", true))
    const first = FakeWebSocket.instances[0]

    act(() => {
      first.onopen?.()
      first.onmessage?.({ data: JSON.stringify({ t: "chunk", d: "hello", end: 5 }) })
      first.onclose?.()
      vi.advanceTimersByTime(1_100)
    })

    expect(result.current.text).toBe("hello")
    expect(FakeWebSocket.instances[1].url).toContain("source=build")
    expect(FakeWebSocket.instances[1].url).toContain("cursor=5")
  })

  it("replaces non-resumable runtime catch-up after reconnect", () => {
    vi.useFakeTimers()
    vi.stubGlobal("WebSocket", FakeWebSocket)
    const { result } = renderHook(() => useDeploymentLogStream("deploy-runtime", true, "container"))
    const first = FakeWebSocket.instances[0]

    act(() => {
      first.onopen?.()
      first.onmessage?.({ data: JSON.stringify({ t: "chunk", d: "old output", seq: 1 }) })
      first.onclose?.()
      vi.advanceTimersByTime(1_100)
    })
    const second = FakeWebSocket.instances[1]
    act(() => {
      second.onopen?.()
      second.onmessage?.({ data: JSON.stringify({ t: "chunk", d: "fresh output", seq: 1 }) })
    })

    expect(result.current.text).toBe("fresh output")
    expect(second.url).toContain("source=container")
  })
})
