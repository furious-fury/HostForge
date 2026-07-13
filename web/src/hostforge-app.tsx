import { useState } from "react"
import {
  ActivityIcon,
  AppWindowIcon,
  BellIcon,
  BookOpenIcon,
  CaretDownIcon,
  CaretRightIcon,
  CheckCircleIcon,
  ClockCounterClockwiseIcon,
  CloudArrowUpIcon,
  CubeIcon,
  GearSixIcon,
  HardDrivesIcon,
  ListIcon,
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
import "@/hostforge.css"

type Icon = React.ComponentType<{ className?: string; size?: number; weight?: "regular" | "bold" | "fill" }>

const workspaceNavigation: Array<{ label: string; icon: Icon; active?: boolean }> = [
  { label: "Overview", icon: SquaresFourIcon, active: true },
  { label: "Applications", icon: AppWindowIcon },
  { label: "Deployments", icon: CloudArrowUpIcon },
  { label: "Observability", icon: PulseIcon },
]

const platformNavigation: Array<{ label: string; icon: Icon }> = [
  { label: "Settings", icon: GearSixIcon },
  { label: "Documentation", icon: BookOpenIcon },
  { label: "System status", icon: ActivityIcon },
]

const metrics = [
  { label: "Applications", value: "12", detail: "All healthy", icon: AppWindowIcon },
  { label: "Active services", value: "28", detail: "26 running", icon: StackIcon },
  { label: "Deploys today", value: "17", detail: "+4 from yesterday", icon: RocketLaunchIcon },
  { label: "Failed", value: "2", detail: "Needs review", icon: ClockCounterClockwiseIcon, attention: true },
  { label: "Containers", value: "34", detail: "2 restarting", icon: CubeIcon },
]

const resources = [
  { label: "CPU", value: "28%", detail: "4 cores", progress: 28 },
  { label: "Memory", value: "61%", detail: "9.8 / 16 GB", progress: 61 },
  { label: "Root disk", value: "43%", detail: "84 / 196 GB", progress: 43 },
  { label: "Network", value: "18.4 MB/s", detail: "5 min average", progress: 36 },
]

const health = [
  { label: "Host", detail: "Ubuntu 24.04", state: "Operational" },
  { label: "Docker", detail: "34 containers", state: "Operational" },
  { label: "Caddy", detail: "19 routes", state: "Operational" },
  { label: "GitHub App", detail: "Token expires soon", state: "Attention", attention: true },
  { label: "Webhook ingress", detail: "Last event 2m ago", state: "Operational" },
]

const deployments = [
  { service: "api", application: "TaxIO", commit: "8c2af71", message: "Add invoice retry policy", status: "Live", started: "4m ago", duration: "1m 42s" },
  { service: "web", application: "GameNation", commit: "e143b8a", message: "Refine tournament lobby", status: "Building", started: "7m ago", duration: "2m 08s" },
  { service: "worker", application: "TaxIO", commit: "fa72c20", message: "Reduce queue concurrency", status: "Failed", started: "31m ago", duration: "48s" },
  { service: "docs", application: "HostForge", commit: "01ad983", message: "Update install guide", status: "Live", started: "2h ago", duration: "1m 11s" },
]

function Brand() {
  return (
    <div className="flex h-16 items-center gap-3 border-b px-4">
      <span className="grid size-8 place-items-center rounded-lg bg-accent text-accent-foreground">
        <CubeIcon size={18} weight="fill" />
      </span>
      <div className="min-w-0">
        <p className="truncate text-sm font-semibold tracking-tight">HostForge</p>
        <p className="truncate text-[11px] text-muted-foreground">Control plane</p>
      </div>
      <CaretDownIcon className="ml-auto text-muted-foreground" size={14} weight="bold" />
    </div>
  )
}

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <>
      {open && <button className="fixed inset-0 z-40 bg-black/30 lg:hidden" aria-label="Close navigation" onClick={onClose} />}
      <aside className={`hf-sidebar ${open ? "translate-x-0" : "-translate-x-full lg:translate-x-0"}`}>
        <div className="flex items-center lg:hidden">
          <div className="flex-1"><Brand /></div>
          <button className="mr-3 rounded-md p-2 text-muted-foreground hover:bg-muted" onClick={onClose} aria-label="Close navigation">
            <XIcon size={18} />
          </button>
        </div>
        <div className="hidden lg:block"><Brand /></div>

        <nav className="flex-1 overflow-y-auto px-3 py-5" aria-label="Primary navigation">
          <NavGroup label="Workspace" items={workspaceNavigation} onNavigate={onClose} />
          <NavGroup label="Platform" items={platformNavigation} onNavigate={onClose} className="mt-7" />
        </nav>

        <div className="border-t p-3">
          <div className="mb-2 flex items-center gap-2 rounded-md px-2 py-2 text-xs text-muted-foreground">
            <span className="size-2 rounded-full bg-emerald-500 shadow-[0_0_0_3px_rgb(16_185_129_/_0.12)]" />
            Server connected
            <span className="ml-auto font-mono text-[10px]">v0.9.4</span>
          </div>
          <button className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left hover:bg-muted">
            <span className="grid size-8 place-items-center rounded-full bg-foreground text-xs font-semibold text-background">MF</span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-xs font-medium">Mr Fury</span>
              <span className="block truncate text-[11px] text-muted-foreground">Administrator</span>
            </span>
            <SignOutIcon size={16} className="text-muted-foreground" />
          </button>
        </div>
      </aside>
    </>
  )
}

function NavGroup({ label, items, className = "", onNavigate }: { label: string; items: Array<{ label: string; icon: Icon; active?: boolean }>; className?: string; onNavigate: () => void }) {
  return (
    <div className={className}>
      <p className="mb-2 px-2 text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">{label}</p>
      <div className="space-y-1">
        {items.map((item) => {
          const ItemIcon = item.icon
          return (
            <button key={item.label} onClick={() => { onNavigate(); if (item.label === "Applications") navigateTo("/applications"); if (item.label === "Deployments") navigateTo("/deployments"); if (item.label === "Observability") navigateTo("/observability"); if (item.label === "Settings") navigateTo("/settings"); if (item.label === "Overview") navigateTo("/") }} className={item.active ? "hf-nav-item hf-nav-item-active" : "hf-nav-item"}>
              <ItemIcon size={17} weight={item.active ? "fill" : "regular"} />
              {item.label}
              {item.label === "System status" && <span className="ml-auto size-1.5 rounded-full bg-amber-500" />}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function Topbar({ onOpenNavigation }: { onOpenNavigation: () => void }) {
  return (
    <header className="hf-topbar sticky top-0 z-30 flex h-16 items-center border-b bg-background/90 px-4 backdrop-blur-md sm:px-6 lg:px-8">
      <button className="mr-3 rounded-md p-2 hover:bg-muted lg:hidden" onClick={onOpenNavigation} aria-label="Open navigation">
        <ListIcon size={20} />
      </button>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">Workspace</span>
        <CaretRightIcon size={12} className="text-muted-foreground" />
        <span className="font-medium">Overview</span>
      </div>

      <div className="ml-auto flex items-center gap-2">
        <CommandSearch />
        <ThemeSwitcher />
        <button className="relative grid size-9 place-items-center rounded-md border bg-card text-muted-foreground hover:text-foreground" aria-label="Notifications">
          <BellIcon size={17} />
          <span className="absolute right-2 top-2 size-1.5 rounded-full bg-amber-500 ring-2 ring-card" />
        </button>
        <Button size="sm" className="hidden sm:inline-flex">
          <PlusIcon /> Create
        </Button>
      </div>
    </header>
  )
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return (
    <section className={`overflow-hidden rounded-xl border bg-card ${className}`}>
      <header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold tracking-tight">{title}</h2>
          {subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}
        </div>
        {action && <div className="ml-auto">{action}</div>}
      </header>
      {children}
    </section>
  )
}

function OverviewPage() {
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end">
        <div>
          <p className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <span className="size-1.5 rounded-full bg-emerald-500" /> All systems responding
          </p>
          <h1 className="text-3xl font-semibold tracking-[-0.035em]">Overview</h1>
          <p className="mt-2 text-sm text-muted-foreground">Monitor applications, deployments and host resources.</p>
        </div>
        <div className="flex flex-wrap gap-2 sm:ml-auto">
          <Button variant="outline">Refresh</Button>
          <Button variant="outline" onClick={() => navigateTo("/applications/new")}><AppWindowIcon /> Create application</Button>
          <Button><RocketLaunchIcon weight="fill" /> Deploy</Button>
        </div>
      </div>

      <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card sm:grid-cols-3 xl:grid-cols-5">
        {metrics.map((metric) => {
          const MetricIcon = metric.icon
          return (
            <article key={metric.label} className="hf-metric-card">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">{metric.label}</span>
                <MetricIcon size={16} className={metric.attention ? "text-amber-600" : "text-muted-foreground"} />
              </div>
              <p className="mt-5 text-3xl font-semibold tracking-[-0.05em] tabular-nums">{metric.value}</p>
              <p className={`mt-1 text-[11px] ${metric.attention ? "text-amber-700" : "text-muted-foreground"}`}>{metric.detail}</p>
            </article>
          )
        })}
      </section>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.65fr)_minmax(320px,0.75fr)]">
        <Panel title="Host resources" subtitle="Live utilization across the primary host" action={<span className="flex items-center gap-1.5 text-[11px] text-muted-foreground"><span className="size-1.5 rounded-full bg-emerald-500" /> Live</span>}>
          <div className="grid gap-x-8 gap-y-7 p-5 sm:grid-cols-2 sm:p-6">
            {resources.map((resource) => (
              <div key={resource.label}>
                <div className="mb-3 flex items-end justify-between gap-3">
                  <div>
                    <p className="text-xs font-medium text-muted-foreground">{resource.label}</p>
                    <p className="mt-1 text-xl font-semibold tracking-tight tabular-nums">{resource.value}</p>
                  </div>
                  <span className="text-[11px] text-muted-foreground">{resource.detail}</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  <div className="h-full rounded-full bg-accent transition-[width] duration-700" style={{ width: `${resource.progress}%` }} />
                </div>
              </div>
            ))}
            <div className="flex items-center gap-4 rounded-lg border bg-muted/35 p-4 sm:col-span-2">
              <span className="grid size-10 place-items-center rounded-lg border bg-card"><HardDrivesIcon size={20} /></span>
              <div>
                <p className="text-sm font-medium">34 containers running</p>
                <p className="mt-0.5 text-xs text-muted-foreground">31 healthy · 2 restarting · 1 stopped</p>
              </div>
              <Button variant="ghost" size="sm" className="ml-auto">Inspect <CaretRightIcon /></Button>
            </div>
          </div>
        </Panel>

        <Panel title="System health" subtitle="Platform dependencies and connections">
          <div className="divide-y">
            {health.map((item) => (
              <div key={item.label} className="flex items-center gap-3 px-5 py-3.5">
                <span className={`size-2 rounded-full ${item.attention ? "bg-amber-500" : "bg-emerald-500"}`} />
                <div className="min-w-0">
                  <p className="text-xs font-medium">{item.label}</p>
                  <p className="mt-0.5 truncate text-[11px] text-muted-foreground">{item.detail}</p>
                </div>
                <span className={`ml-auto text-[11px] font-medium ${item.attention ? "text-amber-700" : "text-emerald-700"}`}>{item.state}</span>
              </div>
            ))}
          </div>
          <button className="flex w-full items-center justify-between border-t px-5 py-3 text-xs font-medium hover:bg-muted/50">
            View system status <CaretRightIcon size={13} />
          </button>
        </Panel>

        <Panel title="Recent deployments" subtitle="Latest build and release activity" action={<Button variant="ghost" size="sm">View all <CaretRightIcon /></Button>}>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] text-left">
              <thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground">
                <tr>
                  <th className="px-5 py-3 font-semibold">Service</th>
                  <th className="px-4 py-3 font-semibold">Commit</th>
                  <th className="px-4 py-3 font-semibold">Status</th>
                  <th className="px-4 py-3 font-semibold">Started</th>
                  <th className="px-5 py-3 text-right font-semibold">Duration</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {deployments.map((deployment) => (
                  <tr key={`${deployment.application}-${deployment.commit}`} className="group hover:bg-muted/35">
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-3">
                        <span className="grid size-8 place-items-center rounded-md border bg-muted/50"><CubeIcon size={15} /></span>
                        <div>
                          <p className="text-xs font-medium">{deployment.service}</p>
                          <p className="mt-0.5 text-[11px] text-muted-foreground">{deployment.application}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <p className="font-mono text-[11px]">{deployment.commit}</p>
                      <p className="mt-0.5 max-w-48 truncate text-[11px] text-muted-foreground">{deployment.message}</p>
                    </td>
                    <td className="px-4 py-3.5"><StatusBadge status={deployment.status} /></td>
                    <td className="px-4 py-3.5 text-xs text-muted-foreground">{deployment.started}</td>
                    <td className="px-5 py-3.5 text-right text-xs text-muted-foreground">{deployment.duration}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title="Setup progress" subtitle="One step left before production" action={<span className="text-xs font-semibold tabular-nums">4 / 5</span>}>
          <div className="p-5">
            <div className="mb-5 h-2 overflow-hidden rounded-full bg-muted"><div className="h-full w-4/5 rounded-full bg-accent" /></div>
            <div className="space-y-3">
              {["Platform domain configured", "GitHub App connected", "Webhook ingress verified", "Caddy routing validated"].map((item) => (
                <div key={item} className="flex items-center gap-2.5 text-xs">
                  <CheckCircleIcon size={17} weight="fill" className="text-emerald-600" /> {item}
                </div>
              ))}
              <div className="flex items-center gap-2.5 text-xs font-medium">
                <span className="grid size-[17px] place-items-center rounded-full border-2 border-accent text-[9px]">5</span>
                Deploy your first service
              </div>
            </div>
            <Button className="mt-6 w-full"><RocketLaunchIcon weight="fill" /> Deploy a service</Button>
          </div>
        </Panel>
      </div>
    </main>
  )
}

function StatusBadge({ status }: { status: string }) {
  const styles = status === "Live" ? "bg-emerald-50 text-emerald-700 ring-emerald-600/15" : status === "Building" ? "bg-blue-50 text-blue-700 ring-blue-600/15" : "bg-red-50 text-red-700 ring-red-600/15"
  return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ring-inset ${styles}`}><span className="size-1.5 rounded-full bg-current" />{status}</span>
}

export default function HostForgeApp() {
  const [navigationOpen, setNavigationOpen] = useState(false)

  return (
    <div className="min-h-svh bg-background text-foreground">
      <Sidebar open={navigationOpen} onClose={() => setNavigationOpen(false)} />
      <div className="lg:pl-60">
        <Topbar onOpenNavigation={() => setNavigationOpen(true)} />
        <OverviewPage />
      </div>
    </div>
  )
}
