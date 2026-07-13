import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ArrowClockwiseIcon,
  ArrowRightIcon,
  BookOpenIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  CodeIcon,
  CubeIcon,
  DatabaseIcon,
  StackIcon,
  GitBranchIcon,
  GlobeIcon,
  HardDrivesIcon,
  MagnifyingGlassIcon,
  ShieldCheckIcon,
  TerminalWindowIcon,
  WarningCircleIcon,
  WrenchIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"

const guides = [
  { title: "Install HostForge", description: "Provision the server, systemd service, data directory, and initial administrator.", category: "Getting started", time: "8 min", icon: TerminalWindowIcon },
  { title: "Create an application", description: "Connect a GitHub repository, configure services, and prepare the first deployment.", category: "Applications", time: "6 min", icon: CubeIcon },
  { title: "Deployment lifecycle", description: "Understand builds, health checks, Caddy cutover, rollback, and release history.", category: "Deployments", time: "10 min", icon: CloudArrowUpIcon },
  { title: "Domains and HTTPS", description: "Configure DNS, platform hostnames, custom domains, and certificate validation.", category: "Networking", time: "7 min", icon: GlobeIcon },
  { title: "Environment and secrets", description: "Manage application and service variables without exposing secret values.", category: "Configuration", time: "5 min", icon: DatabaseIcon },
  { title: "Operator troubleshooting", description: "Inspect systemd, Docker, Caddy, deployment logs, and common recovery paths.", category: "Operations", time: "12 min", icon: WrenchIcon },
  { title: "Local development", description: "Run the Go server and Vite UI, test webhooks, and validate your changes.", category: "Development", time: "9 min", icon: CodeIcon },
  { title: "Updating a VPS", description: "Safely install a new HostForge build while preserving server configuration.", category: "Operations", time: "6 min", icon: ArrowClockwiseIcon },
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
        <div><span className="grid size-10 place-items-center rounded-lg bg-accent text-accent-foreground"><BookOpenIcon size={20} weight="fill" /></span><h2 className="mt-5 text-xl font-semibold tracking-tight">What do you want to accomplish?</h2><p className="mt-2 max-w-xl text-xs leading-5 text-muted-foreground">Search the operator guides or start with the installation and first-deployment path.</p><label className="relative mt-5 block max-w-2xl"><MagnifyingGlassIcon className="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted-foreground" size={16} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="h-11 w-full rounded-lg border bg-background pl-10 pr-4 text-xs outline-none focus:border-accent focus:ring-3 focus:ring-ring/20" placeholder="Search guides, operations, deployments..." /></label></div>
        <div className="rounded-xl border bg-muted/55 p-5"><p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">Recommended path</p><ol className="mt-4 space-y-3">{["Install the control plane", "Connect the GitHub App", "Create and deploy a service"].map((item, index) => <li key={item} className="flex items-center gap-3 text-xs font-medium"><span className="grid size-6 place-items-center rounded-full bg-accent text-[10px] font-bold text-accent-foreground">{index + 1}</span>{item}</li>)}</ol></div>
      </div>
    </section>
    <div className="mb-3 flex items-center justify-between"><h2 className="text-sm font-semibold">Operator guides</h2><span className="text-[11px] text-muted-foreground">{visible.length} guides</span></div>
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">{visible.map((guide) => { const GuideIcon = guide.icon; return <Link key={guide.title} to="#" className="group rounded-xl border bg-card p-5 shadow-[0_5px_18px_rgb(31_35_30_/_0.045)] transition-[transform,box-shadow,border-color] hover:-translate-y-0.5 hover:border-foreground/20 hover:shadow-[0_10px_26px_rgb(31_35_30_/_0.08)]"><div className="flex items-start justify-between"><span className="grid size-9 place-items-center rounded-lg border bg-muted"><GuideIcon size={17} /></span><span className="text-[10px] text-muted-foreground">{guide.time}</span></div><p className="mt-4 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">{guide.category}</p><h3 className="mt-2 text-sm font-semibold">{guide.title}</h3><p className="mt-2 text-[11px] leading-5 text-muted-foreground">{guide.description}</p><span className="mt-4 flex items-center gap-1 text-[11px] font-semibold">Read guide <ArrowRightIcon className="transition-transform group-hover:translate-x-0.5" size={13} /></span></Link> })}</div>
    {!visible.length && <section className="rounded-xl border bg-card p-12 text-center"><MagnifyingGlassIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No guides found</p><p className="mt-1 text-xs text-muted-foreground">Try a broader search term.</p></section>}
  </main>
}

const services = [
  { name: "HostForge server", detail: "API and control plane", status: "Operational", latency: "18 ms", icon: CubeIcon },
  { name: "Docker Engine", detail: "34 containers · v27.2.0", status: "Operational", latency: "12 ms", icon: StackIcon },
  { name: "Caddy", detail: "19 active routes · HTTPS", status: "Operational", latency: "31 ms", icon: GlobeIcon },
  { name: "Build engine", detail: "BuildKit · Railpack ready", status: "Operational", latency: "24 ms", icon: WrenchIcon },
  { name: "GitHub App", detail: "Token renewal required soon", status: "Attention", latency: "142 ms", icon: GitBranchIcon },
  { name: "SQLite", detail: "hostforge.db · 48.2 MB", status: "Operational", latency: "4 ms", icon: DatabaseIcon },
]

export function SystemStatusScreen() {
  return <main className="mx-auto w-full max-w-[1500px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <PageHeader title="System status" description="Read-only health and capacity diagnostics for this HostForge installation." action={<Button variant="outline"><ArrowClockwiseIcon />Refresh status</Button>} />
    <section className="mb-5 overflow-hidden rounded-xl border border-amber-200 bg-card">
      <header className="flex flex-col gap-3 border-b border-amber-200 bg-amber-50/80 px-5 py-4 sm:flex-row sm:items-center"><span className="grid size-9 place-items-center rounded-lg bg-amber-100 text-amber-700"><WarningCircleIcon size={19} weight="fill" /></span><div><h2 className="text-sm font-semibold text-amber-950">One integration needs attention</h2><p className="mt-1 text-[11px] text-amber-800">GitHub App credentials expire soon. Existing deployments and running services are unaffected.</p></div><Button variant="outline" size="sm" className="sm:ml-auto">Review integration</Button></header>
      <div className="grid grid-cols-2 divide-x divide-y lg:grid-cols-4 lg:divide-y-0">{[{ label: "Uptime", value: "18d 7h", detail: "No restarts" }, { label: "CPU", value: "28%", detail: "4 cores" }, { label: "Memory", value: "61%", detail: "9.8 / 16 GB" }, { label: "Root disk", value: "43%", detail: "84 / 196 GB" }].map((item) => <div key={item.label} className="p-5"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{item.label}</p><p className="mt-3 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{item.detail}</p></div>)}</div>
    </section>
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.6fr)]">
      <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Platform services</h2><p className="mt-1 text-xs text-muted-foreground">Latest safe, read-only connectivity checks</p></header><div className="divide-y">{services.map((service) => { const ServiceIcon = service.icon; const attention = service.status === "Attention"; return <div key={service.name} className="flex items-center gap-3 px-5 py-4"><span className="grid size-9 place-items-center rounded-lg border bg-muted"><ServiceIcon size={17} /></span><div className="min-w-0"><p className="text-xs font-semibold">{service.name}</p><p className="mt-0.5 truncate text-[10px] text-muted-foreground">{service.detail}</p></div><span className="ml-auto hidden font-mono text-[10px] text-muted-foreground sm:block">{service.latency}</span><span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ${attention ? "bg-amber-50 text-amber-700" : "bg-emerald-50 text-emerald-700"}`}><span className="size-1.5 rounded-full bg-current" />{service.status}</span></div> })}</div></section>
      <div className="grid gap-5">
        <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">Installation</h2><p className="mt-1 text-xs text-muted-foreground">Version and runtime information</p></header><div className="divide-y">{[{ label: "HostForge", value: "v0.9.4" }, { label: "Go runtime", value: "1.24.2" }, { label: "Operating system", value: "Ubuntu 24.04 LTS" }, { label: "Architecture", value: "linux / amd64" }].map((item) => <div key={item.label} className="flex justify-between gap-4 px-5 py-3.5"><span className="text-[11px] text-muted-foreground">{item.label}</span><span className="font-mono text-[11px] font-medium">{item.value}</span></div>)}</div></section>
        <section className="rounded-xl border bg-card p-5"><div className="flex items-start gap-3"><ShieldCheckIcon className="mt-0.5 text-emerald-600" size={19} /><div><h2 className="text-xs font-semibold">Ingress protected</h2><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Application containers bind to loopback ports. Public traffic is served through Caddy HTTPS routes.</p></div></div></section>
        <section className="rounded-xl border bg-card p-5"><div className="flex items-start gap-3"><HardDrivesIcon className="mt-0.5 text-muted-foreground" size={19} /><div><h2 className="text-xs font-semibold">Last status refresh</h2><p className="mt-1 text-[11px] text-muted-foreground">A few seconds ago · all checks completed</p><span className="mt-3 flex items-center gap-1.5 text-[10px] font-medium text-emerald-700"><CheckCircleIcon size={14} weight="fill" />Sampler operational</span></div></div></section>
      </div>
    </div>
  </main>
}
