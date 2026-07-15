import { lazy, Suspense } from "react"
import { useLocation } from "react-router-dom"

import { ProtectedRoute } from "@/protected-route"

const ApplicationsApp = lazy(() => import("@/applications-app"))
const LoginScreen = lazy(() => import("@/auth-screens").then((module) => ({ default: module.LoginScreen })))
const OnboardingScreen = lazy(() => import("@/auth-screens").then((module) => ({ default: module.OnboardingScreen })))

function RouteLoading() {
  return <main className="min-h-svh bg-background p-6 text-foreground" aria-busy="true" aria-label="Loading page"><div className="mx-auto max-w-[1600px] animate-pulse"><div className="mb-8 h-8 w-48 rounded-md bg-muted" /><div className="h-64 rounded-xl border bg-card" /></div></main>
}

export default function AppRouter() {
  const { pathname } = useLocation()

  if (pathname === "/login") return <Suspense fallback={<RouteLoading />}><LoginScreen /></Suspense>
  if (pathname === "/onboarding") return <ProtectedRoute><Suspense fallback={<RouteLoading />}><OnboardingScreen /></Suspense></ProtectedRoute>

  return <ProtectedRoute><Suspense fallback={<RouteLoading />}><ApplicationsApp /></Suspense></ProtectedRoute>
}
