import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router-dom"
import { useMutation, useQuery } from "@tanstack/react-query"
import {
  ArrowRightIcon,
  CheckCircleIcon,
  CubeIcon,
  GithubLogoIcon,
  GlobeIcon,
  KeyIcon,
  ShieldCheckIcon,
  WarningCircleIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { APIError, api, queryKeys } from "@/api"
import { queryClient } from "@/query-client"
import "@/auth.css"

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label>
}

export function LoginScreen() {
  const [token, setToken] = useState("")
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const login = useMutation({
    mutationFn: api.login,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.session })
      const requested = searchParams.get("returnTo") || "/"
      navigate(requested.startsWith("/") ? requested : "/", { replace: true })
    },
  })
  const message = login.error instanceof APIError && login.error.status === 401 ? "The access token is not valid." : login.isError ? "HostForge could not complete sign in. Check the server and try again." : ""

  return <main className="hf-auth-surface"><section className="w-full max-w-md overflow-hidden rounded-2xl border bg-card shadow-[0_22px_70px_rgb(28_28_24_/_0.09)]"><header className="flex items-center gap-3 border-b bg-muted/75 px-6 py-5"><span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={19} weight="fill" /></span><div><p className="text-sm font-semibold">HostForge</p><p className="mt-0.5 text-[11px] text-muted-foreground">Control plane</p></div></header><form className="space-y-5 p-6" onSubmit={(event) => { event.preventDefault(); if (token.trim()) login.mutate(token) }}><div><h1 className="text-2xl font-semibold tracking-[-0.035em]">Sign in to your control plane</h1><p className="mt-2 text-xs leading-5 text-muted-foreground">Enter the management API token configured on this HostForge server.</p></div><Field label="Access token"><div className="relative"><KeyIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><Input value={token} onChange={(event) => setToken(event.target.value)} className="h-10 w-full bg-background pl-9 text-xs" type="password" autoComplete="current-password" placeholder="HOSTFORGE_API_TOKEN" aria-invalid={Boolean(message)} /></div></Field>{message && <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-[11px] font-medium text-red-700">{message}</p>}<Button className="w-full" type="submit" disabled={!token.trim() || login.isPending}>{login.isPending ? "Signing in..." : "Sign in"} <ArrowRightIcon /></Button></form><footer className="border-t bg-muted/30 px-6 py-3 text-[10px] font-medium text-muted-foreground">Session credentials remain in an HTTP-only cookie.</footer></section></main>
}

function submitManifest(postURL: string, manifest: Record<string, unknown>) {
  const form = document.createElement("form")
  form.method = "POST"
  form.action = postURL
  const input = document.createElement("input")
  input.type = "hidden"
  input.name = "manifest"
  input.value = JSON.stringify(manifest)
  form.append(input)
  document.body.append(form)
  form.submit()
}

export function OnboardingScreen() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [domain, setDomain] = useState(() => suggestedPlatformDomain(window.location.hostname))
  const onboardingQuery = useQuery({ queryKey: queryKeys.onboarding, queryFn: ({ signal }) => api.onboarding(signal) })
  const settingsQuery = useQuery({ queryKey: queryKeys.settings, queryFn: ({ signal }) => api.settings(signal) })
  const statusQuery = useQuery({ queryKey: queryKeys.systemStatus, queryFn: ({ signal }) => api.systemStatus(signal) })
  const applicationsQuery = useQuery({ queryKey: queryKeys.applications, queryFn: ({ signal }) => api.applications(signal) })
  const appQuery = useQuery({ queryKey: queryKeys.githubApp, queryFn: ({ signal }) => api.githubApp(signal) })
  const installationsQuery = useQuery({ queryKey: queryKeys.githubInstallations, queryFn: ({ signal }) => api.githubInstallations(signal), enabled: appQuery.data?.app.configured })
  const manifest = useMutation({
    mutationFn: () => {
      const origin = window.location.origin
      return api.githubManifest({ name: "HostForge", url: origin, callback_url: origin + "/onboarding" })
    },
    onSuccess: (result) => submitManifest(result.post_url, result.manifest),
  })
  const exchange = useMutation({
    mutationFn: api.githubManifestExchange,
    onSuccess: async (result) => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: queryKeys.githubApp }), queryClient.invalidateQueries({ queryKey: queryKeys.githubInstallations }), queryClient.invalidateQueries({ queryKey: queryKeys.onboarding })])
      if (result.install_url) window.location.assign(result.install_url)
      else navigate("/onboarding", { replace: true })
    },
  })
  const complete = useMutation({
    mutationFn: () => api.completeOnboarding(domain),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: queryKeys.onboarding }); navigate("/", { replace: true }) },
  })
  const code = searchParams.get("code")

  if (onboardingQuery.isPending || settingsQuery.isPending || statusQuery.isPending || applicationsQuery.isPending || appQuery.isPending) return <main className="grid min-h-svh place-items-center bg-background"><div className="h-64 w-full max-w-3xl animate-pulse rounded-xl border bg-card" /></main>
  const failed = onboardingQuery.isError || settingsQuery.isError || statusQuery.isError || applicationsQuery.isError || appQuery.isError
  if (failed) return <main className="grid min-h-svh place-items-center bg-background p-5"><section className="w-full max-w-lg rounded-xl border bg-card p-8 text-center"><WarningCircleIcon className="mx-auto text-destructive" size={24} /><h1 className="mt-3 text-sm font-semibold">Setup state could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">The onboarding screen only uses server-reported state.</p><Button className="mt-4" variant="outline" onClick={() => { onboardingQuery.refetch(); settingsQuery.refetch(); statusQuery.refetch(); applicationsQuery.refetch(); appQuery.refetch() }}>Retry</Button></section></main>

  const state = onboardingQuery.data.onboarding
  const settings = settingsQuery.data
  const checksReady = statusQuery.data.checks.every((check) => ["RUNNING", "READY"].includes(check.status))
  const githubConfigured = appQuery.data.app.configured || state.github_app_complete
  const hasInstallation = Boolean(installationsQuery.data?.installations.length)
  const hasApplication = applicationsQuery.data.applications.length > 0
  const currentDomainDetected = Boolean(domain && domain === suggestedPlatformDomain(window.location.hostname))
  const rows = [
    { title: "Management authentication", detail: settings.auth.scheme === "session" ? "Authenticated session active" : "API token authentication active", complete: true, icon: KeyIcon, action: null },
    { title: "GitHub App credentials", detail: githubConfigured ? `${appQuery.data.app.slug || "GitHub App"} configured` : "Create the GitHub App and seal its credentials", complete: githubConfigured, icon: GithubLogoIcon, action: !githubConfigured ? <Button size="sm" disabled={manifest.isPending} onClick={() => manifest.mutate()}>{manifest.isPending ? "Preparing..." : "Create GitHub App"}</Button> : null },
    { title: "GitHub installation", detail: hasInstallation ? `${installationsQuery.data?.installations.length} installation(s) connected` : "Install the configured App on an account or organization", complete: hasInstallation, icon: GithubLogoIcon, action: githubConfigured && !hasInstallation && appQuery.data.app.html_url ? <Button asChild size="sm" variant="outline"><a href={appQuery.data.app.html_url + "/installations/new"}>Install App</a></Button> : null },
    { title: "Docker, Caddy, and webhooks", detail: checksReady ? "All dependency checks ready" : "Review diagnostics before permanent cutover", complete: checksReady, icon: ShieldCheckIcon, action: <Button asChild size="sm" variant="outline"><Link to="/status">Diagnostics</Link></Button> },
    { title: "First application", detail: hasApplication ? `${applicationsQuery.data.applications.length} application(s) created` : "Create the first product and its production/staging environments", complete: hasApplication, icon: CubeIcon, action: !hasApplication ? <Button asChild size="sm"><Link to="/applications/new">Create application</Link></Button> : null },
    { title: "Permanent platform ingress", detail: state.bootstrap_complete ? `Active at ${state.platform_domain}` : currentDomainDetected ? `${domain} detected; register it as the platform domain` : `Point an A record to ${state.bootstrap_public_ip || settings.dns.detected_ipv4 || "this host"}`, complete: state.bootstrap_complete, icon: GlobeIcon, action: null },
  ]

  return <main className="min-h-svh bg-background text-foreground"><header className="flex h-16 items-center border-b bg-card px-5 sm:px-8"><Link to="/" className="flex items-center gap-3"><span className="grid size-8 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={17} weight="fill" /></span><span className="text-sm font-semibold">HostForge</span></Link><Link to="/" className="ml-auto text-xs font-medium text-muted-foreground hover:text-foreground">Finish later</Link></header><div className="mx-auto w-full max-w-4xl px-4 py-8 sm:px-6 lg:px-8 lg:py-12">
    <div className="mb-7"><p className="text-[10px] font-semibold uppercase tracking-[.15em] text-muted-foreground">Server-guided setup</p><h1 className="mt-2 text-3xl font-semibold tracking-[-.035em]">Set up HostForge</h1><p className="mt-2 text-sm text-muted-foreground">Complete only the steps the server reports as pending. There are no local fixture states.</p></div>
    {code && !githubConfigured && <section className="mb-5 rounded-xl border bg-card p-5"><h2 className="text-sm font-semibold">Complete GitHub App exchange</h2><p className="mt-1 text-xs text-muted-foreground">GitHub returned a one-time manifest code. Exchange it to store encrypted App credentials.</p><Button className="mt-4" disabled={exchange.isPending} onClick={() => exchange.mutate(code)}>{exchange.isPending ? "Connecting..." : "Connect GitHub App"}</Button>{exchange.isError && <p className="mt-3 text-xs text-destructive">The manifest exchange failed. Generate a new manifest and try again.</p>}</section>}
    <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Readiness checklist</h2><p className="mt-1 text-xs text-muted-foreground">{rows.filter((row) => row.complete).length} of {rows.length} completed</p></header><div className="divide-y">{rows.map((row) => { const Icon = row.icon; return <div key={row.title} className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center"><span className={`grid size-9 shrink-0 place-items-center rounded-lg ${row.complete ? "bg-emerald-50 text-emerald-700" : "border bg-muted text-muted-foreground"}`}>{row.complete ? <CheckCircleIcon size={18} weight="fill" /> : <Icon size={18} />}</span><div><p className="text-xs font-semibold">{row.title}</p><p className="mt-1 text-[11px] text-muted-foreground">{row.detail}</p></div>{row.action && <div className="sm:ml-auto">{row.action}</div>}</div> })}</div></section>
    {!state.bootstrap_complete && <section className="mt-5 overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Register the permanent platform domain</h2><p className="mt-1 text-xs text-muted-foreground">{currentDomainDetected ? `HostForge detected the domain already serving this dashboard. Confirm it to enable managed deployment URLs.` : "HostForge verifies DNS and Caddy before recording the permanent platform address."}</p></header><form className="space-y-4 p-5" onSubmit={(event) => { event.preventDefault(); if (domain.trim()) complete.mutate() }}><Field label="Platform domain"><div className="flex"><span className="flex items-center rounded-l-md border border-r-0 bg-muted px-3 text-xs text-muted-foreground">https://</span><Input value={domain} onChange={(event) => setDomain(event.target.value)} className="h-10 rounded-l-none bg-background text-xs" placeholder="hostforge.example.com" /></div></Field>{currentDomainDetected && <p className="rounded-md border border-emerald-200 bg-emerald-50 p-3 text-[11px] text-emerald-800">You are already using https://{domain}. This final check records it as the platform domain and configures generated deployment share URLs.</p>}<div className="space-y-1 text-[11px] text-muted-foreground"><p>Control-plane A record: <span className="font-mono">{domain || "your-domain"} -&gt; {state.bootstrap_public_ip || settings.dns.detected_ipv4 || "server IPv4"}</span></p><p>Deployment share URLs require a wildcard A record: <span className="font-mono">*.{domain || "your-domain"} -&gt; {state.bootstrap_public_ip || settings.dns.detected_ipv4 || "server IPv4"}</span></p></div><Button disabled={!domain.trim() || complete.isPending || !githubConfigured}>{complete.isPending ? "Verifying DNS and Caddy..." : currentDomainDetected ? "Verify and register domain" : "Verify and complete setup"}</Button>{complete.isError && <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">{onboardingDomainError(complete.error)}</div>}</form></section>}
    {state.bootstrap_complete && <section className="mt-5 rounded-xl border border-emerald-200 bg-emerald-50 p-5 text-emerald-900"><div className="flex items-start gap-3"><CheckCircleIcon size={20} weight="fill" /><div><h2 className="text-sm font-semibold">Setup complete</h2><p className="mt-1 text-xs">Permanent ingress is active at {state.platform_domain}. Bootstrap access is disabled.</p><Button className="mt-4" onClick={() => navigate("/")}>Open overview</Button></div></div></section>}
    {(manifest.isError || settings.dns.detected_ipv4_warning) && <p className="mt-4 text-xs text-amber-700">{manifest.isError ? "GitHub manifest preparation failed. Verify the public URL and encryption configuration." : settings.dns.detected_ipv4_warning}</p>}
  </div></main>
}

function suggestedPlatformDomain(hostname: string) {
  const host = hostname.trim().toLowerCase()
  if (!host || host === "localhost" || !host.includes(".") || /^[\d.:]+$/.test(host)) return ""
  return host
}

function onboardingDomainError(error: unknown) {
  if (!(error instanceof APIError)) return "Setup completion failed."
  if (error.code === "expected_public_ipv4_unavailable") return "HostForge cannot determine the server IPv4. Configure HOSTFORGE_DNS_SERVER_IPV4 and try again."
  if (error.code === "permanent_https_provision_failed") return error.message
  if (error.code !== "platform_dns_not_ready") return error.message.replaceAll("_", " ")
  const checks = error.details?.checks as Record<string, string> | undefined
  const hostnames = error.details?.hostnames as Record<string, string> | undefined
  const expected = typeof error.details?.expected_ipv4 === "string" ? error.details.expected_ipv4 : "the server IPv4"
  const failures = [
    checks?.apex !== "ok" ? `${hostnames?.apex || "The control-plane hostname"} does not resolve to ${expected}` : "",
    checks?.wildcard !== "ok" ? `${hostnames?.wildcard || "The wildcard hostname"} does not resolve to ${expected}` : "",
  ].filter(Boolean)
  return failures.length ? `${failures.join(". ")}. Add or correct the required A record, wait for DNS propagation, then retry.` : "Platform DNS is not ready yet."
}
