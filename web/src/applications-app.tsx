import { lazy, Suspense, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api, queryKeys } from "@/api"
import { Link, useLocation, useNavigate } from "react-router-dom"
import { RouteTabs } from "@/components/route-tabs"
import {
  ActivityIcon,
  AppWindowIcon,
  BookOpenIcon,
  CaretDownIcon,
  CaretRightIcon,
  CloudArrowUpIcon,
  CubeIcon,
  GearSixIcon,
  GlobeIcon,
  ListIcon,
  MagnifyingGlassIcon,
  PlusIcon,
  PulseIcon,
  SignOutIcon,
  SquaresFourIcon,
  XIcon,
} from "@phosphor-icons/react"

import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Input } from "@/components/ui/input"
import { CommandSearch } from "@/command-search"
import { ThemeSwitcher } from "@/theme-switcher"
import "@/applications.css"

const DashboardScreen = lazy(() => import("@/dashboard-screen").then((module) => ({ default: module.DashboardScreen })))
const ApplicationActivity = lazy(() => import("@/application-activity").then((module) => ({ default: module.ApplicationActivity })))
const CreateApplication = lazy(() => import("@/create-application").then((module) => ({ default: module.CreateApplication })))
const DeploymentsList = lazy(() => import("@/deployment-screens").then((module) => ({ default: module.DeploymentsList })))
const DeploymentDetail = lazy(() => import("@/deployment-screens").then((module) => ({ default: module.DeploymentDetail })))
const DomainsScreen = lazy(() => import("@/operations-screens").then((module) => ({ default: module.DomainsScreen })))
const EnvironmentScreen = lazy(() => import("@/operations-screens").then((module) => ({ default: module.EnvironmentScreen })))
const ObservabilityScreen = lazy(() => import("@/observability-screen").then((module) => ({ default: module.ObservabilityScreen })))
const DocumentationScreen = lazy(() => import("@/platform-screens").then((module) => ({ default: module.DocumentationScreen })))
const SystemStatusScreen = lazy(() => import("@/platform-screens").then((module) => ({ default: module.SystemStatusScreen })))
const ServicesRouter = lazy(() => import("@/services-router").then((module) => ({ default: module.ServicesRouter })))
const ApplicationSettings = lazy(() => import("@/settings-screens").then((module) => ({ default: module.ApplicationSettings })))
const GlobalSettings = lazy(() => import("@/settings-screens").then((module) => ({ default: module.GlobalSettings })))

type Icon = React.ComponentType<{ className?: string; size?: number; weight?: "regular" | "bold" | "fill" }>

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

function Sidebar({ open, onClose, applicationID }: { open: boolean; onClose: () => void; applicationID?: string }) {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const queryClient = useQueryClient()
  const applicationBase = applicationID ? "/applications/" + applicationID : ""
  const applicationQuery = useQuery({
    queryKey: queryKeys.application(applicationID || ""),
    queryFn: ({ signal }) => api.application(applicationID!, signal),
    enabled: Boolean(applicationID),
  })
  const statusQuery = useQuery({ queryKey: queryKeys.systemStatus, queryFn: ({ signal }) => api.systemStatus(signal), refetchInterval: 30_000 })
  const logout = useMutation({ mutationFn: api.logout, onSuccess: async () => { queryClient.clear(); navigate("/login", { replace: true }) } })
  const connected = statusQuery.isSuccess
  const healthy = statusQuery.data?.checks.every((check) => ["RUNNING", "READY"].includes(check.status)) ?? false
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
              const active = pathname === "/" ? item.label === "Overview" : pathname === "/observability" ? item.label === "Observability" : pathname.startsWith("/deployments") ? item.label === "Deployments" : pathname.startsWith("/applications") ? item.label === "Applications" : false
              return <Link key={item.label} to={item.href} onClick={onClose} className={`hf-nav-item ${active ? "hf-nav-item-active" : ""}`}><ItemIcon size={17} weight={active ? "fill" : "regular"} />{item.label}</Link>
            })}
          </div>
          {applicationID && <div className="mt-7">
            <p className="mb-2 truncate px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">{applicationQuery.data?.application.name || "Current application"}</p>
            <div className="space-y-1">
              <Link to={applicationBase} onClick={onClose} className={pathname === applicationBase || pathname === applicationBase + "/" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><SquaresFourIcon size={17} />Overview</Link>
              <Link to={applicationBase + "/services"} onClick={onClose} className={pathname.startsWith(applicationBase + "/services") ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><CubeIcon size={17} />Services</Link>
              <Link to={applicationBase + "/deployments"} onClick={onClose} className={pathname === applicationBase + "/deployments" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><CloudArrowUpIcon size={17} />Deployments</Link>
            </div>
          </div>}
          <p className="mb-2 mt-7 px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">Platform</p>
          <div className="space-y-1">
            <Link to="/settings" className={pathname === "/settings" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><GearSixIcon size={17} />Settings</Link>
            <Link to="/docs" className={pathname === "/docs" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><BookOpenIcon size={17} />Documentation</Link>
            <Link to="/status" className={pathname === "/status" ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}><ActivityIcon size={17} />System status<span className={"ml-auto size-1.5 rounded-full " + (healthy ? "bg-emerald-500" : "bg-amber-500")} /></Link>
          </div>
        </nav>
        <div className="border-t p-3">
          <div className="mb-2 flex items-center gap-2 rounded-md px-2 py-2 text-xs text-muted-foreground"><span className={"size-2 rounded-full " + (connected ? "bg-emerald-500" : "bg-red-500")} />{connected ? "Server connected" : statusQuery.isPending ? "Connecting..." : "Server unavailable"}<span className="ml-auto font-mono text-[10px]">{statusQuery.data?.version || ""}</span></div>
          <button disabled={logout.isPending} onClick={() => logout.mutate()} className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-muted disabled:opacity-60">
            <span className="grid size-8 place-items-center rounded-full bg-foreground text-xs font-semibold text-background">OP</span>
            <span className="min-w-0 flex-1"><span className="block text-xs font-medium">Operator</span><span className="block text-[11px] text-muted-foreground">{logout.isPending ? "Signing out..." : "Authenticated session"}</span></span>
            <SignOutIcon size={16} className="text-muted-foreground" />
          </button>
        </div>
      </aside>
    </>
  )
}

function Topbar({ onOpenNavigation, applicationID, section }: { onOpenNavigation: () => void; applicationID?: string; section?: string }) {
  const { pathname } = useLocation()
  const applicationQuery = useQuery({
    queryKey: queryKeys.application(applicationID || ""),
    queryFn: ({ signal }) => api.application(applicationID!, signal),
    enabled: Boolean(applicationID),
  })
  const serviceID = pathname.match(/^\/applications\/[^/]+\/services\/([^/]+)/)?.[1]
  const serviceQuery = useQuery({
    queryKey: queryKeys.service(serviceID || ""),
    queryFn: ({ signal }) => api.service(serviceID!, signal),
    enabled: Boolean(serviceID && serviceID !== "new"),
  })
  const applicationName = applicationQuery.data?.application.name || (applicationQuery.isPending ? "Loading application..." : "Application")
  const applicationBase = applicationID ? "/applications/" + applicationID : ""
  const serviceBase = serviceID && serviceID !== "new" ? applicationBase + "/services/" + serviceID : ""
  const serviceSection = serviceID === "new" ? "New service" : pathname === serviceBase ? "" : serviceBase ? pathname.slice(serviceBase.length + 1).split("/")[0] : ""
  const applicationSection = applicationID && !pathname.startsWith(applicationBase + "/services") && pathname !== applicationBase && pathname !== applicationBase + "/" ? pathname.slice(applicationBase.length + 1).split("/")[0] : ""
  const globalDeploymentID = pathname.match(/^\/deployments\/([^/]+)\/?$/)?.[1]
  return (
    <header className="hf-topbar sticky top-0 z-30 flex h-16 items-center border-b px-4 backdrop-blur-md sm:px-6 lg:px-8">
      <button className="mr-3 rounded-md p-2 hover:bg-muted lg:hidden" onClick={onOpenNavigation} aria-label="Open navigation"><ListIcon size={20} /></button>
      <div className="flex min-w-0 items-center gap-2 text-sm">
        {globalDeploymentID ? <><Link to="/deployments" className="text-muted-foreground hover:text-foreground">Deployments</Link><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><span className="max-w-56 truncate font-mono font-medium">{globalDeploymentID}</span></> : section ? <span className="font-medium">{section}</span> : <>
          <Link to="/applications" className={applicationID ? "text-muted-foreground hover:text-foreground" : "font-medium"}>Applications</Link>
          {applicationID && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><Link to={applicationBase} className={pathname === applicationBase ? "truncate font-medium" : "truncate text-muted-foreground hover:text-foreground"}>{applicationName}</Link></>}
          {applicationSection && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><span className="truncate font-medium capitalize">{applicationSection}</span></>}
          {pathname.startsWith(applicationBase + "/services") && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><Link to={applicationBase + "/services"} className={serviceID ? "text-muted-foreground hover:text-foreground" : "font-medium"}>Services</Link></>}
          {serviceID && serviceID !== "new" && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><Link to={serviceBase} className={serviceSection ? "max-w-40 truncate text-muted-foreground hover:text-foreground" : "max-w-40 truncate font-medium"}>{serviceQuery.data?.service.name || "Service"}</Link></>}
          {(serviceSection || serviceID === "new") && <><CaretRightIcon size={12} className="shrink-0 text-muted-foreground" /><span className="truncate font-medium capitalize">{serviceSection || "New service"}</span></>}
        </>}
      </div>
      <div className="absolute left-1/2 top-1/2 hidden w-[clamp(20rem,38vw,36rem)] -translate-x-1/2 -translate-y-1/2 lg:block">
        <CommandSearch />
      </div>
      <div className="ml-auto flex items-center gap-2">
        <ThemeSwitcher />
        <Button asChild size="sm" className="hidden sm:inline-flex"><Link to="/applications/new"><PlusIcon /> Create</Link></Button>
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
  const tone = status === "Healthy" || status === "Running" || status === "Live" ? "success" : status === "Degraded" || status === "Failed" ? "warning" : "neutral"
  return <StatusBadge tone={tone} dot>{status}</StatusBadge>
}

function ScreenLoading() {
  return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-7 sm:px-6 lg:px-8 lg:py-9" aria-busy="true" aria-label="Loading page"><div className="mb-7 h-8 w-52 rounded-md bg-muted" /><div className="h-72 rounded-xl border bg-card" /></main>
}

function NotFoundScreen() {
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><h1 className="text-lg font-semibold">Page not found</h1><p className="mt-2 text-xs text-muted-foreground">The requested HostForge screen does not exist.</p><Button asChild className="mt-5"><Link to="/">Return to overview</Link></Button></section></main>
}

function ApplicationsList() {
  const navigate = useNavigate()
  const [filter, setFilter] = useState("All")
  const [query, setQuery] = useState("")
  const applicationsQuery = useQuery({ queryKey: queryKeys.applications, queryFn: ({ signal }) => api.applications(signal) })
  if (applicationsQuery.isPending) return <ScreenLoading />
  if (applicationsQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Applications could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">HostForge returned an error while loading application data.</p><Button className="mt-4" variant="outline" onClick={() => applicationsQuery.refetch()}>Retry</Button></section></main>
  const applications = applicationsQuery.data.applications.map((application) => {
    const production = application.environment_health?.find((environment) => environment.kind === "production")
    const services = application.service_count || 0
    const healthy = application.healthy_service_count || 0
    const status = services === 0 ? "No services" : production?.status === "healthy" ? "Healthy" : "Degraded"
    return { id: application.id, name: application.name, description: application.description, initials: application.name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase(), services, healthy, domains: application.domain_count || 0, status, deployment: application.latest_deployment ? new Date(application.latest_deployment.created_at).toLocaleString() : "Never", updated: new Date(application.updated_at).toLocaleDateString() }
  })
  const visibleApplications = applications.filter((application) => (filter === "All" || application.status === filter) && [application.name, application.description].join(" ").toLowerCase().includes(query.toLowerCase()))

  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <PageHeader title="Applications" description="Organize related services, deployments, domains and configuration.">
        <Button onClick={() => navigate("/applications/new")}><PlusIcon /> Create application</Button>
      </PageHeader>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/70 p-4 sm:flex-row sm:items-center">
          <div className="flex flex-wrap gap-1 rounded-lg border bg-card p-1">
            {["All", "Healthy", "Degraded", "No services"].map((item) => <button key={item} onClick={() => setFilter(item)} className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${filter === item ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{item}</button>)}
          </div>
          <label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full rounded-md border bg-card pl-9 pr-3 text-xs outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/20" placeholder="Search applications" /></label>
        </header>

        <div className="overflow-x-auto">
          <Table className="w-full min-w-[880px] text-left">
            <TableHeader className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><TableRow><TableHead className="px-5 py-3 font-semibold">Application</TableHead><TableHead className="px-4 py-3 font-semibold">Services</TableHead><TableHead className="px-4 py-3 font-semibold">Production</TableHead><TableHead className="px-4 py-3 font-semibold">Latest deployment</TableHead><TableHead className="px-4 py-3 font-semibold">Domains</TableHead><TableHead className="px-4 py-3 font-semibold">Updated</TableHead><TableHead className="px-5 py-3 text-right font-semibold">Actions</TableHead></TableRow></TableHeader>
            <TableBody className="divide-y">
              {visibleApplications.map((application) => (
                <TableRow key={application.id} className="group hover:bg-muted/35">
                  <TableCell className="px-5 py-4"><Link to={`/applications/${application.id}`} className="flex items-center gap-3"><span className="grid size-9 place-items-center rounded-lg bg-accent text-[11px] font-bold text-accent-foreground">{application.initials}</span><span><span className="block text-xs font-semibold group-hover:underline">{application.name}</span><span className="mt-1 block max-w-72 truncate text-[11px] text-muted-foreground">{application.description}</span></span></Link></TableCell>
                  <TableCell className="px-4 py-4"><p className="text-xs font-medium tabular-nums">{application.services}</p><p className="mt-1 text-[11px] text-muted-foreground">{application.services ? `${application.healthy} healthy` : "None added"}</p></TableCell>
                  <TableCell className="px-4 py-4"><StatusPill status={application.status} /></TableCell>
                  <TableCell className="px-4 py-4 text-xs text-muted-foreground">{application.deployment}</TableCell>
                  <TableCell className="px-4 py-4"><Link to={"/applications/" + application.id + "/domains"} className="flex items-center gap-1.5 text-xs font-medium hover:underline"><GlobeIcon size={14} className="text-muted-foreground" />{application.domains}</Link></TableCell>
                  <TableCell className="px-4 py-4 text-xs text-muted-foreground">{application.updated}</TableCell>
                  <TableCell className="px-5 py-4 text-right"><Button asChild variant="ghost" size="sm"><Link to={"/applications/" + application.id + "/settings"}>Settings</Link></Button></TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        {!visibleApplications.length && <div className="grid place-items-center px-6 py-16 text-center"><span className="grid size-11 place-items-center rounded-xl border bg-muted"><MagnifyingGlassIcon size={20} /></span><p className="mt-4 text-sm font-semibold">Create your first application</p><p className="mt-1 text-xs text-muted-foreground">Applications group the services that make up a product. Clear filters if you expected an existing application.</p><Button className="mt-4" size="sm" onClick={() => navigate("/applications/new")}><PlusIcon />Create application</Button></div>}
        <footer className="flex items-center justify-between border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground"><span>{visibleApplications.length} of {applications.length} applications</span><span>Updated just now</span></footer>
      </section>
    </main>
  )
}

function ApplicationOverview({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  if (applicationQuery.isPending) return <ScreenLoading />
  if (applicationQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Application could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">It may have been removed or the server is unavailable.</p><Button className="mt-4" variant="outline" onClick={() => applicationQuery.refetch()}>Retry</Button></section></main>
  const { application, environments, services } = applicationQuery.data
  const initials = application.name.split(/\s+/).map((part) => part[0]).join("").slice(0, 2).toUpperCase()
  const base = "/applications/" + application.id
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <PageHeader eyebrow={<p className="mb-2 text-xs font-medium text-muted-foreground">{application.archived ? "Archived" : "Active application"}</p>} title={application.name} description={application.description || "No description provided."}>
        <Button variant="outline" onClick={() => navigate(base + "/settings")}><GearSixIcon /> Settings</Button>
        <Button onClick={() => navigate(base + "/services/new")}><PlusIcon /> Add service</Button>
      </PageHeader>
      <div className="mb-5 flex flex-col gap-3 rounded-xl border bg-card p-4 sm:flex-row sm:items-center">
        <span className="grid size-11 place-items-center rounded-xl bg-accent text-sm font-bold text-accent-foreground">{initials}</span>
        <div><p className="text-xs font-medium">{services.length} {services.length === 1 ? "service" : "services"}</p><p className="mt-1 text-xs text-muted-foreground">{environments.map((environment) => environment.name).join(" · ")}</p></div>
        <div className="sm:ml-auto sm:text-right"><p className="text-xs font-medium">Last updated</p><p className="mt-1 text-xs text-muted-foreground">{new Date(application.updated_at).toLocaleString()}</p></div>
      </div>
      <RouteTabs active="Overview" label="Application navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? base : base + "/" + tab.toLowerCase() }))} />
      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex min-h-14 items-center border-b bg-muted/75 px-5"><div><h2 className="text-sm font-semibold">Services</h2><p className="mt-0.5 text-xs text-muted-foreground">Deployable components in this application</p></div><Button className="ml-auto" variant="ghost" size="sm" onClick={() => navigate(base + "/services")}>View all <CaretRightIcon /></Button></header>
        {services.length ? <div className="divide-y">{services.map((service) => <Link key={service.id} to={base + "/services/" + service.id} className="flex items-center gap-3 px-5 py-4 hover:bg-muted/35"><span className="grid size-9 place-items-center rounded-lg border bg-muted"><CubeIcon size={16} /></span><span className="min-w-0"><span className="block truncate text-xs font-semibold">{service.name}</span><span className="mt-1 block truncate text-[11px] text-muted-foreground">{service.repo_url} · {service.runtime}</span></span><span className="ml-auto font-mono text-[10px] text-muted-foreground">:{service.internal_port}</span></Link>)}</div> : <div className="px-6 py-14 text-center"><CubeIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No services yet</p><p className="mt-1 text-xs text-muted-foreground">Connect a repository to create the first deployable service.</p><Button className="mt-4" onClick={() => navigate(base + "/services/new")}><PlusIcon /> Add service</Button></div>}
      </section>
    </main>
  )
}

export default function ApplicationsApp() {
  const [navigationOpen, setNavigationOpen] = useState(false)
  const { pathname: path } = useLocation()
  const dashboard = path === "/"
  const applicationsList = path === "/applications" || path === "/applications/"
  const creatingApplication = path === "/applications/new"
  const globalDeployments = path === "/deployments"
  const deploymentDetailMatch = path.match(/^\/deployments\/([^/]+)\/?$/)
  const observability = path === "/observability"
  const globalSettings = path === "/settings"
  const documentation = path === "/docs"
  const systemStatus = path === "/status"
  const applicationMatch = path.match(/^\/applications\/([^/]+)/)
  const applicationID = applicationMatch?.[1]
  const applicationBase = applicationID ? "/applications/" + applicationID : ""
  const applicationSettings = path === applicationBase + "/settings"
  const applicationDeployments = path === applicationBase + "/deployments"
  const applicationActivity = path === applicationBase + "/activity"
  const applicationEnvironment = path === applicationBase + "/environment"
  const applicationDomains = path === applicationBase + "/domains"
  const serviceArea = Boolean(applicationBase) && path.startsWith(applicationBase + "/services")
  const applicationOverview = /^\/applications\/[^/]+\/?$/.test(path) && path !== "/applications/new"
  return <div className="min-h-svh bg-background text-foreground"><Sidebar open={navigationOpen} onClose={() => setNavigationOpen(false)} applicationID={applicationID} /><div className="lg:pl-60"><Topbar applicationID={applicationID} section={dashboard ? "Overview" : globalSettings ? "Settings" : documentation ? "Documentation" : systemStatus ? "System status" : observability ? "Observability" : globalDeployments ? "Deployments" : undefined} onOpenNavigation={() => setNavigationOpen(true)} /><Suspense fallback={<ScreenLoading />}>{dashboard ? <DashboardScreen /> : documentation ? <DocumentationScreen /> : systemStatus ? <SystemStatusScreen /> : globalSettings ? <GlobalSettings /> : applicationSettings ? <ApplicationSettings applicationID={applicationID!} /> : observability ? <ObservabilityScreen /> : deploymentDetailMatch ? <DeploymentDetail deploymentID={deploymentDetailMatch[1]} /> : globalDeployments ? <DeploymentsList /> : applicationDeployments ? <DeploymentsList scope="application" applicationID={applicationID!} /> : applicationActivity ? <ApplicationActivity applicationID={applicationID!} /> : applicationEnvironment ? <EnvironmentScreen scope="application" applicationID={applicationID!} /> : applicationDomains ? <DomainsScreen scope="application" applicationID={applicationID!} /> : serviceArea ? <ServicesRouter path={path} /> : creatingApplication ? <CreateApplication /> : applicationOverview && applicationID ? <ApplicationOverview applicationID={applicationID} /> : applicationsList ? <ApplicationsList /> : <NotFoundScreen />}</Suspense></div></div>
}
