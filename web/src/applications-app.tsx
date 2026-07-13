import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  AppWindowIcon,
  ArrowSquareOutIcon,
  BellIcon,
  BookOpenIcon,
  BracketsCurlyIcon,
  CaretDownIcon,
  CaretRightIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  CubeIcon,
  DotsThreeIcon,
  FunnelSimpleIcon,
  GearSixIcon,
  GitBranchIcon,
  GlobeIcon,
  ListIcon,
  MagnifyingGlassIcon,
  PlusIcon,
  PulseIcon,
  RocketLaunchIcon,
  SignOutIcon,
  SquaresFourIcon,
  StackIcon,
  XIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import { navigateTo } from "@/navigation"
import { CommandSearch } from "@/command-search"
import { ThemeSwitcher } from "@/theme-switcher"
import { ApplicationActivity } from "@/application-activity"
import { CreateApplication } from "@/create-application"
import { DeploymentsList } from "@/deployment-screens"
import { DomainsScreen, EnvironmentScreen } from "@/operations-screens"
import { ObservabilityScreen } from "@/observability-screen"
import { DocumentationScreen, SystemStatusScreen } from "@/platform-screens"
import { ServicesRouter } from "@/services-router"
import { ApplicationSettings, GlobalSettings } from "@/settings-screens"
import "@/applications.css"

type Icon = React.ComponentType<{ className?: string; size?: number; weight?: "regular" | "bold" | "fill" }>

const applications = [
  { id: "taxio", name: "TaxIO", description: "Nigerian personal income tax platform", initials: "TX", services: 3, healthy: 3, status: "Healthy", deployment: "8 minutes ago", domains: 4, updated: "8m ago" },
  { id: "gamenation", name: "GameNation", description: "Competitive gaming and tournament platform", initials: "GN", services: 2, healthy: 1, status: "Degraded", deployment: "2 hours ago", domains: 2, updated: "2h ago" },
  { id: "hostforge-docs", name: "HostForge Docs", description: "Product documentation and operator guides", initials: "HD", services: 1, healthy: 1, status: "Healthy", deployment: "Yesterday", domains: 1, updated: "1d ago" },
  { id: "ledger-api", name: "Ledger API", description: "Internal accounting and reconciliation services", initials: "LA", services: 0, healthy: 0, status: "No services", deployment: "Never", domains: 0, updated: "4d ago" },
]

const services = [
  { name: "web", type: "Web service", branch: "main", status: "Running", deployment: "8m ago", url: "taxio.ng", cpu: "18%", memory: "312 MB" },
  { name: "api", type: "API service", branch: "main", status: "Running", deployment: "24m ago", url: "api.taxio.ng", cpu: "31%", memory: "486 MB" },
  { name: "worker", type: "Worker", branch: "main", status: "Running", deployment: "1h ago", url: null, cpu: "12%", memory: "208 MB" },
]

const deploymentActivity = [
  { service: "web", commit: "8c2af71", message: "Add invoice retry policy", status: "Live", time: "8 minutes ago" },
  { service: "api", commit: "13d298a", message: "Validate taxpayer region codes", status: "Live", time: "24 minutes ago" },
  { service: "worker", commit: "fa72c20", message: "Reduce queue concurrency", status: "Failed", time: "1 hour ago" },
]

const navItems: Array<{ label: string; icon: Icon; href: string }> = [
  { label: "Overview", icon: SquaresFourIcon, href: "/" },
  { label: "Applications", icon: AppWindowIcon, href: "/applications" },
  { label: "Deployments", icon: CloudArrowUpIcon, href: "/deployments" },
  { label: "Observability", icon: PulseIcon, href: "/observability" },
]

function Brand() {
  return (
    <Link to="/" className="flex h-16 items-center gap-3 border-b px-4">
      <span className="grid size-8 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={18} weight="fill" /></span>
      <span className="min-w-0">
        <span className="block truncate text-sm font-semibold tracking-tight">HostForge</span>
        <span className="block truncate text-[11px] text-muted-foreground">Control plane</span>
      </span>
      <CaretDownIcon className="ml-auto text-muted-foreground" size={14} weight="bold" />
    </Link>
  )
}

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <>
      {open && <button className="fixed inset-0 z-40 bg-black/30 lg:hidden" aria-label="Close navigation" onClick={onClose} />}
      <aside className={`hf-sidebar ${open ? "translate-x-0" : "-translate-x-full lg:translate-x-0"}`}>
        <div className="flex items-center lg:hidden">
          <div className="flex-1"><Brand /></div>
          <button className="mr-3 rounded-md p-2 text-muted-foreground hover:bg-muted" onClick={onClose} aria-label="Close navigation"><XIcon size={18} /></button>
        </div>
        <div className="hidden lg:block"><Brand /></div>
        <nav className="flex-1 overflow-y-auto px-3 py-5" aria-label="Primary navigation">
          <p className="mb-2 px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">Workspace</p>
          <div className="space-y-1">
            {navItems.map((item) => {
              const ItemIcon = item.icon
              const active = window.location.pathname === "/observability" ? item.label === "Observability" : window.location.pathname === "/deployments" ? item.label === "Deployments" : window.location.pathname.startsWith("/applications") ? item.label === "Applications" : false
              return <Link key={item.label} to={item.href} onClick={onClose} className={`hf-nav-item ${active ? "hf-nav-item-active" : ""}`}><ItemIcon size={17} weight={active ? "fill" : "regular"} />{item.label}</Link>
            })}
          </div>
          <p className="mb-2 mt-7 px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">Platform</p>
          <div className="space-y-1">
            <Link to="/settings" className={window.location.pathname === "/settings" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><GearSixIcon size={17} />Settings</Link>
            <Link to="/docs" className={window.location.pathname === "/docs" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><BookOpenIcon size={17} />Documentation</Link>
            <Link to="/status" className={window.location.pathname === "/status" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><ActivityIcon size={17} />System status<span className="ml-auto size-1.5 rounded-full bg-amber-500" /></Link>
          </div>
        </nav>
        <div className="border-t p-3">
          <div className="mb-2 flex items-center gap-2 rounded-md px-2 py-2 text-xs text-muted-foreground"><span className="size-2 rounded-full bg-emerald-500" />Server connected<span className="ml-auto font-mono text-[10px]">v0.9.4</span></div>
          <button className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-muted">
            <span className="grid size-8 place-items-center rounded-full bg-foreground text-xs font-semibold text-background">MF</span>
            <span className="min-w-0 flex-1"><span className="block text-xs font-medium">Mr Fury</span><span className="block text-[11px] text-muted-foreground">Administrator</span></span>
            <SignOutIcon size={16} className="text-muted-foreground" />
          </button>
        </div>
      </aside>
    </>
  )
}

function Topbar({ onOpenNavigation, application, section }: { onOpenNavigation: () => void; application?: string; section?: string }) {
  return (
    <header className="hf-topbar sticky top-0 z-30 flex h-16 items-center border-b bg-background/90 px-4 backdrop-blur-md sm:px-6 lg:px-8">
      <button className="mr-3 rounded-md p-2 hover:bg-muted lg:hidden" onClick={onOpenNavigation} aria-label="Open navigation"><ListIcon size={20} /></button>
      <div className="flex min-w-0 items-center gap-2 text-sm">
        {section ? <span className="font-medium">{section}</span> : <>
          <Link to="/applications" className={application ? "text-muted-foreground hover:text-foreground" : "font-medium"}>Applications</Link>
          {application && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><span className="truncate font-medium">{application}</span></>}
        </>}
      </div>
      <div className="ml-auto flex items-center gap-2">
        <CommandSearch />
        <ThemeSwitcher />
        <button className="relative grid size-9 place-items-center rounded-md border bg-card text-muted-foreground hover:text-foreground" aria-label="Notifications"><BellIcon size={17} /><span className="absolute right-2 top-2 size-1.5 rounded-full bg-amber-500 ring-2 ring-card" /></button>
        <Button size="sm" className="hidden sm:inline-flex"><PlusIcon /> Create</Button>
      </div>
    </header>
  )
}

function PageHeader({ eyebrow, title, description, children }: { eyebrow?: React.ReactNode; title: string; description: string; children?: React.ReactNode }) {
  return (
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end">
      <div className="min-w-0">{eyebrow}<h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p></div>
      {children && <div className="flex flex-wrap gap-2 sm:ml-auto">{children}</div>}
    </div>
  )
}

function StatusPill({ status }: { status: string }) {
  const style = status === "Healthy" || status === "Running" || status === "Live" ? "bg-emerald-50 text-emerald-700 ring-emerald-600/15" : status === "Degraded" || status === "Failed" ? "bg-amber-50 text-amber-700 ring-amber-600/15" : "bg-neutral-100 text-neutral-600 ring-neutral-500/15"
  return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ring-inset ${style}`}><span className="size-1.5 rounded-full bg-current" />{status}</span>
}

function ApplicationsList() {
  const [filter, setFilter] = useState("All")
  const [query, setQuery] = useState("")
  const visibleApplications = applications.filter((application) => (filter === "All" || application.status === filter) && `${application.name} ${application.description}`.toLowerCase().includes(query.toLowerCase()))

  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <PageHeader title="Applications" description="Organize related services, deployments, domains and configuration.">
        <Button variant="outline"><GitBranchIcon /> Import from GitHub</Button>
        <Button onClick={() => navigateTo("/applications/new")}><PlusIcon /> Create application</Button>
      </PageHeader>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/70 p-4 sm:flex-row sm:items-center">
          <div className="flex flex-wrap gap-1 rounded-lg border bg-card p-1">
            {["All", "Healthy", "Degraded", "No services"].map((item) => <button key={item} onClick={() => setFilter(item)} className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${filter === item ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{item}</button>)}
          </div>
          <label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full rounded-md border bg-card pl-9 pr-3 text-xs outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/20" placeholder="Search applications or services" /></label>
          <Button variant="outline" size="icon" aria-label="Filter applications"><FunnelSimpleIcon /></Button>
        </header>

        <div className="overflow-x-auto">
          <table className="w-full min-w-[880px] text-left">
            <thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Application</th><th className="px-4 py-3 font-semibold">Services</th><th className="px-4 py-3 font-semibold">Production</th><th className="px-4 py-3 font-semibold">Latest deployment</th><th className="px-4 py-3 font-semibold">Domains</th><th className="px-4 py-3 font-semibold">Updated</th><th className="px-5 py-3 text-right font-semibold">Actions</th></tr></thead>
            <tbody className="divide-y">
              {visibleApplications.map((application) => (
                <tr key={application.id} className="group hover:bg-muted/35">
                  <td className="px-5 py-4"><Link to={`/applications/${application.id}`} className="flex items-center gap-3"><span className="grid size-9 place-items-center rounded-lg bg-accent text-[11px] font-bold text-accent-foreground">{application.initials}</span><span><span className="block text-xs font-semibold group-hover:underline">{application.name}</span><span className="mt-1 block max-w-72 truncate text-[11px] text-muted-foreground">{application.description}</span></span></Link></td>
                  <td className="px-4 py-4"><p className="text-xs font-medium tabular-nums">{application.services}</p><p className="mt-1 text-[11px] text-muted-foreground">{application.services ? `${application.healthy} healthy` : "None added"}</p></td>
                  <td className="px-4 py-4"><StatusPill status={application.status} /></td>
                  <td className="px-4 py-4 text-xs text-muted-foreground">{application.deployment}</td>
                  <td className="px-4 py-4"><span className="flex items-center gap-1.5 text-xs"><GlobeIcon size={14} className="text-muted-foreground" />{application.domains}</span></td>
                  <td className="px-4 py-4 text-xs text-muted-foreground">{application.updated}</td>
                  <td className="px-5 py-4 text-right"><Button variant="ghost" size="icon" aria-label={`Actions for ${application.name}`}><DotsThreeIcon weight="bold" /></Button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!visibleApplications.length && <div className="grid place-items-center px-6 py-16 text-center"><span className="grid size-11 place-items-center rounded-xl border bg-muted"><MagnifyingGlassIcon size={20} /></span><p className="mt-4 text-sm font-semibold">Create your first application</p><p className="mt-1 text-xs text-muted-foreground">Applications group the services that make up a product. Clear filters if you expected an existing application.</p><div className="mt-4 flex gap-2"><Button size="sm" onClick={() => navigateTo("/applications/new")}><PlusIcon />Create application</Button><Button variant="outline" size="sm"><GitBranchIcon />Import from GitHub</Button></div></div>}
        <footer className="flex items-center justify-between border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground"><span>{visibleApplications.length} of {applications.length} applications</span><span>Updated just now</span></footer>
      </section>
    </main>
  )
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function ApplicationOverview() {
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <PageHeader eyebrow={<p className="mb-2 flex items-center gap-2 text-xs font-medium text-emerald-700"><span className="size-1.5 rounded-full bg-emerald-500" />Production healthy</p>} title="TaxIO" description="Nigerian personal income tax platform">
        <Button variant="outline" onClick={() => navigateTo("/applications/taxio/settings")}><GearSixIcon /> Settings</Button><Button variant="outline"><PlusIcon /> Add service</Button><Button><RocketLaunchIcon weight="fill" /> Deploy all</Button><Button variant="outline" size="icon" aria-label="More application actions"><DotsThreeIcon weight="bold" /></Button>
      </PageHeader>

      <div className="mb-5 flex flex-col gap-3 rounded-xl border bg-card p-4 sm:flex-row sm:items-center">
        <span className="grid size-11 place-items-center rounded-xl bg-accent text-sm font-bold text-accent-foreground">TX</span>
        <div><p className="text-xs font-medium">Production URL</p><Link to="https://taxio.ng" className="mt-1 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">taxio.ng <ArrowSquareOutIcon size={13} /></Link></div>
        <div className="sm:ml-auto sm:text-right"><p className="text-xs font-medium">Last updated</p><p className="mt-1 text-xs text-muted-foreground">8 minutes ago by deployment</p></div>
      </div>

      <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1" aria-label="Application navigation"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? "/applications/taxio" : `/applications/taxio/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === "Overview" ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav>

      <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">
        {[{ label: "Services", value: "3", detail: "All deployed", icon: StackIcon }, { label: "Healthy services", value: "3", detail: "100% operational", icon: CheckCircleIcon }, { label: "Failed deploys", value: "1", detail: "Last 30 days", icon: CloudArrowUpIcon }, { label: "Domains", value: "4", detail: "All secured", icon: GlobeIcon }].map((item) => { const ItemIcon = item.icon; return <article key={item.label} className="hf-app-summary"><div className="flex items-center justify-between"><p className="text-xs text-muted-foreground">{item.label}</p><ItemIcon size={16} className="text-muted-foreground" /></div><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{item.detail}</p></article> })}
      </section>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.55fr)_minmax(320px,0.75fr)]">
        <Panel title="Services" subtitle="Deployable components in this application" action={<Button variant="ghost" size="sm">View all <CaretRightIcon /></Button>}>
          <div className="overflow-x-auto"><table className="w-full min-w-[740px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Service</th><th className="px-4 py-3 font-semibold">Branch</th><th className="px-4 py-3 font-semibold">Status</th><th className="px-4 py-3 font-semibold">URL</th><th className="px-5 py-3 text-right font-semibold">Resources</th></tr></thead><tbody className="divide-y">{services.map((service) => <tr key={service.name} className="hover:bg-muted/35"><td className="px-5 py-4"><Link to={`/applications/taxio/services/${service.name}`} className="flex items-center gap-3"><span className="grid size-8 place-items-center rounded-md border bg-muted"><CubeIcon size={15} /></span><span><span className="block text-xs font-semibold">{service.name}</span><span className="mt-0.5 block text-[11px] text-muted-foreground">{service.type} · deployed {service.deployment}</span></span></Link></td><td className="px-4 py-4"><span className="flex items-center gap-1.5 font-mono text-[11px]"><GitBranchIcon size={13} />{service.branch}</span></td><td className="px-4 py-4"><StatusPill status={service.status} /></td><td className="px-4 py-4">{service.url ? <Link to={`https://${service.url}`} className="flex items-center gap-1 text-xs hover:underline">{service.url}<ArrowSquareOutIcon size={12} /></Link> : <span className="text-xs text-muted-foreground">Internal</span>}</td><td className="px-5 py-4 text-right text-[11px] text-muted-foreground">{service.cpu} CPU · {service.memory}</td></tr>)}</tbody></table></div>
        </Panel>

        <Panel title="Application domains" subtitle="Public routes and TLS status" action={<Button variant="ghost" size="sm">Manage</Button>}>
          <div className="divide-y">{[{ domain: "taxio.ng", target: "web", primary: true }, { domain: "www.taxio.ng", target: "web" }, { domain: "api.taxio.ng", target: "api" }].map((item) => <div key={item.domain} className="flex items-center gap-3 px-5 py-4"><span className="grid size-8 place-items-center rounded-md border bg-muted"><GlobeIcon size={15} /></span><div className="min-w-0"><p className="truncate text-xs font-medium">{item.domain}</p><p className="mt-0.5 text-[11px] text-muted-foreground">Routes to {item.target}</p></div><span className="ml-auto text-[10px] font-medium text-emerald-700">{item.primary ? "Primary · TLS" : "TLS"}</span></div>)}</div>
        </Panel>

        <Panel title="Deployment activity" subtitle="Recent releases across all services">
          <div className="divide-y">{deploymentActivity.map((deployment) => <div key={deployment.commit} className="flex gap-4 px-5 py-4"><div className="relative"><span className={`mt-1 block size-2.5 rounded-full ring-4 ring-card ${deployment.status === "Failed" ? "bg-red-500" : "bg-emerald-500"}`} /></div><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><p className="text-xs font-semibold">{deployment.service}</p><StatusPill status={deployment.status} /><span className="ml-auto text-[11px] text-muted-foreground">{deployment.time}</span></div><p className="mt-1 text-xs text-muted-foreground"><span className="font-mono text-foreground">{deployment.commit}</span> · {deployment.message}</p></div></div>)}</div>
        </Panel>

        <Panel title="Shared environment" subtitle="Variable names available to services" action={<Button variant="ghost" size="sm">Configure</Button>}>
          <div className="divide-y">{[{ name: "DATABASE_URL", scope: "All services" }, { name: "REDIS_URL", scope: "api, worker" }, { name: "APP_ENV", scope: "All services" }, { name: "SENTRY_DSN", scope: "web, api" }].map((variable) => <div key={variable.name} className="flex items-center gap-3 px-5 py-3.5"><BracketsCurlyIcon size={16} className="text-muted-foreground" /><div><p className="font-mono text-[11px] font-medium">{variable.name}</p><p className="mt-0.5 text-[10px] text-muted-foreground">{variable.scope}</p></div><span className="ml-auto rounded bg-muted px-1.5 py-1 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground">Secret</span></div>)}</div>
        </Panel>
      </div>
    </main>
  )
}

export default function ApplicationsApp() {
  const [navigationOpen, setNavigationOpen] = useState(false)
  const path = window.location.pathname
  const creatingApplication = path === "/applications/new"
  const globalDeployments = path === "/deployments"
  const observability = path === "/observability"
  const globalSettings = path === "/settings"
  const documentation = path === "/docs"
  const systemStatus = path === "/status"
  const applicationSettings = path === "/applications/taxio/settings"
  const applicationDeployments = path === "/applications/taxio/deployments"
  const applicationActivity = path === "/applications/taxio/activity"
  const applicationEnvironment = path === "/applications/taxio/environment"
  const applicationDomains = path === "/applications/taxio/domains"
  const serviceArea = path.startsWith("/applications/taxio/services")
  const applicationOverview = /^\/applications\/[^/]+\/?$/.test(path) && path !== "/applications/new"
  return <div className="min-h-svh bg-background text-foreground"><Sidebar open={navigationOpen} onClose={() => setNavigationOpen(false)} /><div className="lg:pl-60"><Topbar application={path.startsWith("/applications/taxio") ? "TaxIO" : undefined} section={globalSettings ? "Settings" : documentation ? "Documentation" : systemStatus ? "System status" : observability ? "Observability" : globalDeployments ? "Deployments" : undefined} onOpenNavigation={() => setNavigationOpen(true)} />{documentation ? <DocumentationScreen /> : systemStatus ? <SystemStatusScreen /> : globalSettings ? <GlobalSettings /> : applicationSettings ? <ApplicationSettings /> : observability ? <ObservabilityScreen /> : globalDeployments ? <DeploymentsList /> : applicationDeployments ? <DeploymentsList scope="application" /> : applicationActivity ? <ApplicationActivity /> : applicationEnvironment ? <EnvironmentScreen scope="application" /> : applicationDomains ? <DomainsScreen scope="application" /> : serviceArea ? <ServicesRouter path={path} /> : creatingApplication ? <CreateApplication /> : applicationOverview ? <ApplicationOverview /> : <ApplicationsList />}</div></div>
}
