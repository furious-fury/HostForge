import { act, render, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider, useLocation } from "react-router-dom"
import { afterEach, describe, expect, it, vi } from "vitest"

import { RouteAwareAppErrorBoundary } from "@/app-error-boundary"

function RouteContent() {
  const location = useLocation()
  if (location.pathname === "/broken") throw new Error("broken route")
  return <p>Healthy route</p>
}

afterEach(() => vi.restoreAllMocks())

describe("route-aware application error boundary", () => {
  it("recovers when navigation leaves a failed screen", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined)
    const router = createMemoryRouter([{
      path: "*",
      element: <RouteAwareAppErrorBoundary><RouteContent /></RouteAwareAppErrorBoundary>,
    }], { initialEntries: ["/broken"] })

    render(<RouterProvider router={router} />)
    expect(screen.getByText("This screen could not be rendered")).toBeInTheDocument()

    await act(() => router.navigate("/"))
    expect(await screen.findByText("Healthy route")).toBeInTheDocument()
  })
})
