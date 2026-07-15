import { useEffect, useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { Navigate, useLocation } from "react-router-dom"

import { APIError, api, queryKeys, unauthorizedEvent } from "@/api"

function SessionLoading() {
  return (
    <main className="grid min-h-svh place-items-center bg-background text-foreground" aria-busy="true">
      <div className="w-full max-w-sm animate-pulse space-y-3 px-6">
        <div className="h-9 w-40 rounded-md bg-muted" />
        <div className="h-28 rounded-xl border bg-card" />
      </div>
    </main>
  )
}

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const location = useLocation()
  const [sessionExpired, setSessionExpired] = useState(false)
  useEffect(() => {
    const handleUnauthorized = () => setSessionExpired(true)
    window.addEventListener(unauthorizedEvent, handleUnauthorized)
    return () => window.removeEventListener(unauthorizedEvent, handleUnauthorized)
  }, [])
  const session = useQuery({
    queryKey: queryKeys.session,
    queryFn: ({ signal }) => api.session(signal),
    retry: false,
    staleTime: 30_000,
  })

  if (session.isPending) return <SessionLoading />

  if (sessionExpired || session.error instanceof APIError && session.error.status === 401) {
    const returnTo = `${location.pathname}${location.search}`
    return <Navigate to={`/login?returnTo=${encodeURIComponent(returnTo)}`} replace />
  }

  if (session.isError) {
    return (
      <main className="grid min-h-svh place-items-center bg-background p-6 text-foreground">
        <section className="max-w-md rounded-xl border bg-card p-6 text-center">
          <h1 className="text-sm font-semibold">Unable to reach HostForge</h1>
          <p className="mt-2 text-xs text-muted-foreground">Check that the server is running, then try again.</p>
          <button className="mt-4 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground" onClick={() => session.refetch()}>
            Retry connection
          </button>
        </section>
      </main>
    )
  }

  return children
}
