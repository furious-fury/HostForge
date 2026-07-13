import { useLocation } from "react-router-dom"

import ApplicationsApp from "@/applications-app"
import { LoginScreen, OnboardingScreen } from "@/auth-screens"
import HostForgeApp from "@/hostforge-app"

export default function AppRouter() {
  const { pathname } = useLocation()

  if (pathname === "/login") return <LoginScreen />
  if (pathname === "/onboarding") return <OnboardingScreen />

  const managedPath = pathname.startsWith("/applications") || ["/deployments", "/observability", "/settings", "/docs", "/status"].includes(pathname)
  return managedPath ? <ApplicationsApp /> : <HostForgeApp />
}
