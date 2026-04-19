import { FormEvent, useEffect, useState } from "react";
import { Route, Routes } from "react-router-dom";
import { createSession, deleteSession, getSessionStatus } from "./api";
import { Button } from "./components/Button";
import { Shell } from "./components/Shell";
import { ToastProvider } from "./components/ToastProvider";
import { UIPrefsProvider } from "./hooks/useUIPrefs";
import { DeploymentsPage } from "./pages/DeploymentsPage";
import { DeploymentPage } from "./pages/DeploymentPage";
import { ObservabilityPage } from "./pages/ObservabilityPage";
import { NewProjectPage } from "./pages/NewProjectPage";
import { ProjectPage } from "./pages/ProjectPage";
import { ProjectsPage } from "./pages/ProjectsPage";
import { SettingsPage } from "./pages/SettingsPage";
import { DefaultLandingRoute } from "./pages/DefaultLandingRoute";

export default function App() {
  const [authChecking, setAuthChecking] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);
  const [loginUsername, setLoginUsername] = useState("admin");
  const [loginPassword, setLoginPassword] = useState("");
  const [authError, setAuthError] = useState("");
  const [loginBusy, setLoginBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const ok = await getSessionStatus();
        if (!cancelled) {
          setAuthenticated(ok);
          setAuthError("");
        }
      } catch (err) {
        if (!cancelled) {
          setAuthenticated(false);
          setAuthError(err instanceof Error ? err.message : "session_check_failed");
        }
      } finally {
        if (!cancelled) setAuthChecking(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleLogin(e: FormEvent) {
    e.preventDefault();
    setLoginBusy(true);
    setAuthError("");
    try {
      if (loginUsername.trim().toLowerCase() !== "admin") {
        throw new Error("invalid username");
      }
      await createSession(loginPassword);
      setAuthenticated(true);
      setLoginPassword("");
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : "invalid credentials");
    } finally {
      setLoginBusy(false);
    }
  }

  async function handleLogout() {
    try {
      await deleteSession();
    } finally {
      setAuthenticated(false);
      setAuthError("");
    }
  }

  if (authChecking) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg text-text">
        <div className="mono border border-border bg-surface px-6 py-4 text-sm">Checking session…</div>
      </div>
    );
  }

  if (!authenticated) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg p-6 text-text">
        <form className="flex w-full max-w-md flex-col gap-4 border border-border bg-surface p-6" onSubmit={handleLogin}>
          <div>
            <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">HostForge</div>
            <h1 className="text-2xl font-semibold tracking-tight">Admin Login</h1>
            <p className="mt-1 text-sm text-muted">Sign in to start an authenticated admin session.</p>
          </div>
          <label className="flex flex-col gap-1.5">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Username</span>
            <input
              type="text"
              value={loginUsername}
              onChange={(e) => setLoginUsername(e.target.value)}
              className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              placeholder="admin"
              required
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Password</span>
            <input
              type="password"
              value={loginPassword}
              onChange={(e) => setLoginPassword(e.target.value)}
              className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              placeholder="••••••••"
              required
            />
          </label>
          {authError && <div className="border border-danger bg-danger/10 p-3 text-sm text-danger">{authError}</div>}
          <div className="w-full">
            <Button type="submit" variant="primary" disabled={loginBusy} className="w-full">
              {loginBusy ? "Logging in…" : "Login"}
            </Button>
          </div>
        </form>
      </div>
    );
  }

  return (
    <ToastProvider>
      <UIPrefsProvider>
        <Shell onLogout={handleLogout}>
          <Routes>
            <Route path="/" element={<DefaultLandingRoute />} />
            <Route path="/deployments" element={<DeploymentsPage />} />
            <Route path="/observability" element={<ObservabilityPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/projects/new" element={<NewProjectPage />} />
            <Route path="/projects/:projectID" element={<ProjectPage />} />
            <Route path="/projects/:projectID/deployments/:deploymentID" element={<DeploymentPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<DefaultLandingRoute />} />
          </Routes>
        </Shell>
      </UIPrefsProvider>
    </ToastProvider>
  );
}
