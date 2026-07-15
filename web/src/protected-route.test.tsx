import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom"
import { describe, expect, it, vi } from "vitest"

import { APIError, api, unauthorizedEvent } from "@/api"
import { ProtectedRoute } from "@/protected-route"

function Location() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname + location.search}</div>
}

function renderProtected(initial = "/applications/app-1/services/service-1?environment=staging") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initial]}>
        <Routes>
          <Route path="/login" element={<Location />} />
          <Route path="*" element={<ProtectedRoute><div>Protected content</div></ProtectedRoute>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe("ProtectedRoute", () => {
  it("renders authenticated routes", async () => {
    vi.spyOn(api, "session").mockResolvedValue({ authenticated: true })
    renderProtected("/")
    expect(await screen.findByText("Protected content")).toBeInTheDocument()
  })

  it("redirects an unauthorized direct URL and preserves its destination", async () => {
    vi.spyOn(api, "session").mockRejectedValue(new APIError(401, "unauthorized"))
    renderProtected()
    const location = await screen.findByTestId("location")
    expect(location.textContent).toContain("/login?returnTo=")
    expect(decodeURIComponent(location.textContent || "")).toContain("/applications/app-1/services/service-1?environment=staging")
  })

  it("redirects when a later API request reports session expiry", async () => {
    vi.spyOn(api, "session").mockResolvedValue({ authenticated: true })
    renderProtected("/deployments/deploy-1")
    expect(await screen.findByText("Protected content")).toBeInTheDocument()
    act(() => window.dispatchEvent(new Event(unauthorizedEvent)))
    expect(await screen.findByTestId("location")).toHaveTextContent("/login?returnTo=%2Fdeployments%2Fdeploy-1")
  })
})
