import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  ArrowClockwiseIcon,
  BookOpenIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  CodeIcon,
  CubeIcon,
  DatabaseIcon,
  StackIcon,
  GlobeIcon,
  HardDrivesIcon,
  MagnifyingGlassIcon,
  ShieldCheckIcon,
  TerminalWindowIcon,
  WarningCircleIcon,
  WrenchIcon,
  ArrowSquareOutIcon,
} from "@phosphor-icons/react"

import { api, queryKeys } from "@/api"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

const guides = [
  { title: "Install HostForge", description: "Provision the server, systemd service, data directory, and initial administrator.", category: "Getting started", time: "8 min", icon: TerminalWindowIcon, path: "site/src/content/docs/01-installation.md" },
  { title: "Create an application", description: "Connect a GitHub repository, configure services, and prepare the first deployment.", category: "Applications", time: "6 min", icon: CubeIcon, path: "site/src/content/docs/02-quickstart.md" },
  { title: "Deployment lifecycle", description: "Understand builds, health checks, Caddy cutover, rollback, and release history.", category: "Deployments", time: "10 min", icon: CloudArrowUpIcon, path: "site/src/content/docs/11-deployments-and-cutover.md" },
  { title: "Domains and HTTPS", description: "Configure DNS, platform hostnames, custom domains, and certificate validation.", category: "Networking", time: "7 min", icon: GlobeIcon, path: "site/src/content/docs/12-domains-and-caddy.md" },
  { title: "Environment and secrets", description: "Manage application and service variables without exposing secret values.", category: "Configuration", time: "5 min", icon: DatabaseIcon, path: "site/src/content/docs/21-environment-variables.md" },
  { title: "Operator troubleshooting", description: "Inspect systemd, Docker, Caddy, deployment logs, and common recovery paths.", category: "Operations", time: "12 min", icon: WrenchIcon, path: "docs/operator-guide.md" },
  { title: "Local development", description: "Run the Go server and Vite UI, test webhooks, and validate your changes.", category: "Development", time: "9 min", icon: CodeIcon, path: "docs/development.md" },
  { title: "Updating a VPS", description: "Safely install a new HostForge build while preserving server configuration.", category: "Operations", time: "6 min", icon: ArrowClockwiseIcon, path: "docs/vps-update.md" },
]

function PageHeader({ title, description, action }: { title: string; description: string; action?: React.ReactNode }) {
  return <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p></div>{action && <div className="sm:ml-auto">{action}</div>}</div>
}

export function DocumentationScreen() {
  const [query, setQuery] = useState("")
  const visible = guides.filter((guide) => `${guide.title} ${guide.description} ${guide.category}`.toLowerCase().includes(query.toLowerCase()))

  return <main className="mx-auto w-full max-w-[1500px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <PageHeader title="Documentation" description="Guides for installing, operating, deploying, and troubleshooting HostForge." />
    <section className="mb-5 overflow-hidden rounded-xl border bg-card">
      <div className="grid gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:p-8">
        <div><span className="grid size-10 place-items-center rounded-lg bg-accent text-accent-foreground"><BookOpenIcon size={20} weight="fill" /></span><h2 className="mt-5 text-xl font-semibold tracking-tight">What do you want to accomplish?</h2><p className="mt-2 max-w-xl text-xs leading-5 text-muted-foreground">Search the operator guides or start with the installation and first-deployment path.</p><label className="relative mt-5 block max-w-2xl"><MagnifyingGlassIcon className="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted-foreground" size={16} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-11 w-full rounded-lg border bg-background pl-10 pr-4 text-xs outline-none focus:border-accent focus:ring-3 focus:ring-ring/20" placeholder="Search guides, operations, deployments..." /></label></div>
        <div className="rounded-xl border bg-muted/55 p-5"><p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">Recommended path</p><ol className="mt-4 space-y-3">{["Install the control plane", "Connect the GitHub App", "Create and deploy a service"].map((item, index) => <li key={item} className="flex items-center gap-3 text-xs font-medium"><span className="grid size-6 place-items-center rounded-full bg-accent text-[10px] font-bold text-accent-foreground">{index + 1}</span>{item}</li>)}</ol></div>
      </div>
    </section>
    <div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">Operator guides</h2><span className="text-[11px] text-muted-foreground">{visible.length} guides</span></div>
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{visible.map((guide) => { const GuideIcon = guide.icon; return <a key={guide.title} href={"https://github.com/furious-fury/HostForge/blob/main/" + guide.path} target="_blank" rel="noreferrer" className="group rounded-xl border bg-card p-5 shadow-[0_5px_18px_rgb(31_35_30_/_0.045)] transition hover:-translate-y-0.5 hover:border-accent"><div className="flex items-start justify-between"><span className="grid size-9 place-items-center rounded-lg border bg-muted"><GuideIcon size={17} /></span><span className="text-[10px] text-muted-foreground">{guide.time}</span></div><p className="mt-4 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">{guide.category}</p><h3 className="mt-2 text-sm font-semibold">{guide.title}</h3><p className="mt-2 text-[11px] leading-5 text-muted-foreground">{guide.description}</p><span className="mt-4 inline-flex items-center gap-1 text-[11px] font-semibold">Open operator reference <ArrowSquareOutIcon size={12} /></span></a> })}</div>
    {!visible.length && <section className="rounded-xl border bg-card p-12 text-center"><MagnifyingGlassIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No guides found</p><p className="mt-1 text-xs text-muted-foreground">Try a broader search term.</p></section>}
  </main>
}

export function SystemStatusScreen() {
  const statusQuery = useQuery({ queryKey: queryKeys.systemStatus, queryFn: ({ signal }) => api.systemStatus(signal) })
  const hostQuery = useQuery({ queryKey: queryKeys.hostSnapshot, queryFn: ({ signal }) => api.hostSnapshot(signal) })
  const settingsQuery = useQuery({ queryKey: queryKeys.settings, queryFn: ({ signal }) => api.settings(signal) })
  const refresh = () => { statusQuery.refetch(); hostQuery.refetch(); settingsQuery.refetch() }

  if (statusQuery.isPending || settingsQuery.isPending) return <main className="mx-auto w-full max-w-[1500px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-10 w-60 rounded bg-muted" /><div className="mt-7 h-72 rounded-xl border bg-card" /></main>
  if (statusQuery.isError || settingsQuery.isError) return <main className="mx-auto w-full max-w-[1500px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><WarningCircleIcon className="mx-auto text-destructive" size={24} /><h1 className="mt-3 text-sm font-semibold">System diagnostics could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">Check the management server connection and try again.</p><Button className="mt-4" variant="outline" onClick={refresh}><ArrowClockwiseIcon />Retry</Button></section></main>

  const settings = settingsQuery.data
  const sample = hostQuery.data?.sample
  const disk = sample?.disks.find((item) => item.mount === "/") || sample?.disks[0]
  const checks = statusQuery.data.checks
  const attention = checks.filter((check) => !["RUNNING", "READY"].includes(check.status))
  const uptime = settings.build.uptime_seconds
  const uptimeLabel = uptime >= 86400 ? `${Math.floor(uptime / 86400)}d ${Math.floor(uptime % 86400 / 3600)}h` : uptime >= 3600 ? `${Math.floor(uptime / 3600)}h ${Math.floor(uptime % 3600 / 60)}m` : `${Math.floor(uptime / 60)}m`
  const values = [
    { label: "Uptime", value: uptimeLabel, detail: new Date(settings.build.started_at).toLocaleString() },
    { label: "CPU", value: sample ? `${sample.cpu_pct.toFixed(1)}%` : "Unavailable", detail: sample ? `${sample.per_core_pct?.length || 0} cores sampled` : hostQuery.data?.error_code || "No sample" },
    { label: "Memory", value: sample ? `${sample.mem.used_pct.toFixed(1)}%` : "Unavailable", detail: sample ? `${(sample.mem.used_bytes / 1024 ** 3).toFixed(1)} / ${(sample.mem.total_bytes / 1024 ** 3).toFixed(1)} GB` : "No sample" },
    { label: "Root disk", value: disk ? `${disk.used_pct.toFixed(1)}%` : "Unavailable", detail: disk ? `${(disk.used_bytes / 1024 ** 3).toFixed(1)} / ${(disk.total_bytes / 1024 ** 3).toFixed(1)} GB` : "No disk sample" },
  ]

  return <main className="mx-auto w-full max-w-[1500px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <PageHeader title="System status" description="Read-only health and capacity diagnostics for this HostForge installation." action={<Button variant="outline" disabled={statusQuery.isFetching || hostQuery.isFetching} onClick={refresh}><ArrowClockwiseIcon className={statusQuery.isFetching ? "animate-spin" : ""} />Refresh status</Button>} />
    <section className={`mb-5 overflow-hidden rounded-xl border bg-card ${attention.length ? "border-amber-200" : "border-emerald-200"}`}>
      <header className={`flex gap-3 border-b px-5 py-4 ${attention.length ? "border-amber-200 bg-amber-50/80 dark:bg-amber-950/20" : "border-emerald-200 bg-emerald-50/70 dark:bg-emerald-950/20"}`}><span className={`grid size-9 place-items-center rounded-lg ${attention.length ? "bg-amber-100 text-amber-700" : "bg-emerald-100 text-emerald-700"}`}>{attention.length ? <WarningCircleIcon size={19} weight="fill" /> : <CheckCircleIcon size={19} weight="fill" />}</span><div><h2 className="text-sm font-semibold">{attention.length ? `${attention.length} platform ${attention.length === 1 ? "check needs" : "checks need"} attention` : "All platform checks are healthy"}</h2><p className="mt-1 text-[11px] text-muted-foreground">{attention.length ? "Review the diagnostic details below. Runtime controls are intentionally not exposed here." : "Docker, ingress, and webhook routing responded successfully."}</p></div></header>
      <div className="grid grid-cols-2 divide-x divide-y lg:grid-cols-4 lg:divide-y-0">{values.map((item) => <div key={item.label} className="p-5"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{item.label}</p><p className="mt-3 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 truncate text-[10px] text-muted-foreground">{item.detail}</p></div>)}</div>
    </section>
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,.6fr)]">
      <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Platform services</h2><p className="mt-1 text-xs text-muted-foreground">Latest safe, read-only connectivity checks</p></header><div className="divide-y">{checks.map((check) => { const healthy = ["RUNNING", "READY"].includes(check.status); return <div key={check.id} className="flex items-center gap-3 px-5 py-4"><span className="grid size-9 place-items-center rounded-lg border bg-muted"><StackIcon size={17} /></span><div className="min-w-0"><p className="text-xs font-semibold">{check.label}</p><p className="mt-0.5 text-[10px] text-muted-foreground">{check.detail || "Connectivity check completed successfully."}</p>{check.error_code && <p className="mt-1 font-mono text-[9px] text-destructive">{check.error_code}</p>}</div><StatusBadge className="ml-auto shrink-0" tone={healthy ? "success" : check.status === "SKIPPED" ? "neutral" : "warning"} dot>{check.status}</StatusBadge></div> })}</div></section>
      <div className="grid gap-5">
        <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Installation</h2><p className="mt-1 text-xs text-muted-foreground">Version and runtime information</p></header><div className="divide-y">{[
          { label: "HostForge", value: settings.build.version_display },
          { label: "Go runtime", value: settings.build.go_version },
          { label: "Operating system", value: settings.build.os },
          { label: "Architecture", value: settings.build.arch },
          { label: "Commit", value: settings.build.commit || "development" },
        ].map((item) => <div key={item.label} className="flex justify-between gap-4 px-5 py-3.5"><span className="text-[11px] text-muted-foreground">{item.label}</span><span className="truncate font-mono text-[11px] font-medium">{item.value}</span></div>)}</div></section>
        <section className="rounded-xl border bg-card p-5"><div className="flex items-start gap-3"><ShieldCheckIcon className="mt-0.5 text-emerald-600" size={19} /><div><h2 className="text-xs font-semibold">Ingress configuration</h2><p className="mt-1 text-[11px] leading-5 text-muted-foreground">{settings.caddy.root_config ? `Caddy root: ${settings.caddy.root_config}. Application containers remain behind managed routes.` : "Caddy root configuration is not set; HTTPS route validation is unavailable."}</p></div></div></section>
        <section className="rounded-xl border bg-card p-5"><div className="flex items-start gap-3"><HardDrivesIcon className="mt-0.5 text-muted-foreground" size={19} /><div><h2 className="text-xs font-semibold">Database</h2><p className="mt-1 break-all font-mono text-[10px] text-muted-foreground">{settings.paths.db_path}</p><span className="mt-3 flex items-center gap-1.5 text-[10px] font-medium"><DatabaseIcon size={14} />{settings.paths.db_size_bytes >= 0 ? `${(settings.paths.db_size_bytes / 1024 ** 2).toFixed(2)} MB` : "Size unavailable"}</span></div></div></section>
      </div>
    </div>
  </main>
}
