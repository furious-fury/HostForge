import { useState } from "react"
import {
  ActivityIcon,
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  ClockIcon,
  CloudArrowUpIcon,
  CpuIcon,
  CubeIcon,
  FunnelSimpleIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  MagnifyingGlassIcon,
  MemoryIcon,
  PulseIcon,
  WarningCircleIcon,
  WifiHighIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import "@/observability.css"

const requests = [
  { time: "13:52:18.405", method: "GET", path: "/v1/assessments/tx_8392", status: 200, duration: "42ms", application: "TaxIO", service: "api" },
  { time: "13:52:17.918", method: "POST", path: "/v1/tournaments/register", status: 201, duration: "186ms", application: "GameNation", service: "api" },
  { time: "13:52:16.774", method: "GET", path: "/assets/app-shell.css", status: 304, duration: "8ms", application: "TaxIO", service: "web" },
  { time: "13:52:15.201", method: "POST", path: "/v1/assessments", status: 500, duration: "5.02s", application: "TaxIO", service: "api" },
  { time: "13:52:12.630", method: "GET", path: "/docs/deployment", status: 200, duration: "17ms", application: "HostForge Docs", service: "docs" },
  { time: "13:52:09.118", method: "GET", path: "/health", status: 200, duration: "3ms", application: "TaxIO", service: "api" },
]

const deploySteps = [
  { time: "13:49:28", deployment: "dep_2QM8LA", service: "web", step: "Image built", status: "Completed", duration: "54s" },
  { time: "13:49:22", deployment: "dep_2QM8LA", service: "web", step: "Build configuration detected", status: "Completed", duration: "1s" },
  { time: "13:49:18", deployment: "dep_2QM8LA", service: "web", step: "Source fetched", status: "Completed", duration: "4s" },
  { time: "13:48:57", deployment: "dep_9BK4TR", service: "worker", step: "Image built", status: "Failed", duration: "31s" },
  { time: "13:48:26", deployment: "dep_9BK4TR", service: "worker", step: "Build configuration detected", status: "Completed", duration: "2s" },
]

const events = [
  { time: "13:51:44", type: "Domain", title: "Route activated", detail: "staging.taxio.ng now targets TaxIO/web", state: "success" },
  { time: "13:49:28", type: "Deployment", title: "Build entered release phase", detail: "dep_2QM8LA · GameNation/web", state: "active" },
  { time: "13:48:57", type: "Deployment", title: "Build step failed", detail: "dep_9BK4TR · TaxIO/worker · Image built", state: "error" },
  { time: "13:44:12", type: "Configuration", title: "Environment variable updated", detail: "TaxIO/api · SENTRY_DSN", state: "neutral" },
  { time: "13:41:05", type: "Runtime", title: "Container restarted", detail: "GameNation/api · automatic restart policy", state: "warning" },
]

const hostSeries = {
  cpu: [24, 28, 22, 31, 35, 29, 27, 38, 41, 36, 32, 29, 34, 28, 31, 37, 33, 30, 28, 32, 35, 31, 29, 28],
  memory: [57, 58, 58, 59, 59, 60, 60, 61, 61, 61, 62, 61, 61, 62, 62, 61, 61, 61, 62, 61, 61, 61, 61, 61],
  disk: [42, 42, 42, 42, 42, 42, 42, 42, 42, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43, 43],
  network: [12, 18, 14, 25, 20, 32, 24, 19, 35, 42, 28, 21, 31, 26, 22, 38, 45, 29, 25, 36, 32, 27, 41, 34],
}

type Tab = "Overview" | "Requests" | "Deploy steps" | "Host" | "Events"

export function ObservabilityScreen() {
  const [tab, setTab] = useState<Tab>("Overview")
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><p className="mb-2 flex items-center gap-2 text-xs font-medium text-emerald-700"><span className="size-1.5 rounded-full bg-emerald-500" />Telemetry stream connected</p><h1 className="text-3xl font-semibold tracking-[-0.035em]">Observability</h1><p className="mt-2 text-sm text-muted-foreground">Platform-wide request, deployment and host telemetry.</p></div><div className="flex gap-2 sm:ml-auto"><select className="hf-compact-field"><option>Last hour</option><option>Last 6 hours</option><option>Last 24 hours</option></select><Button variant="outline"><ArrowsClockwiseIcon />Refresh</Button></div></div>
    <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1" aria-label="Observability navigation"><div className="flex min-w-max gap-1">{(["Overview", "Requests", "Deploy steps", "Host", "Events"] as Tab[]).map((item) => <button key={item} onClick={() => setTab(item)} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === item ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{item}</button>)}</div></nav>
    {(tab === "Overview" || tab === "Host") && <SummaryCards />}
    {tab === "Overview" && <OverviewContent onOpen={setTab} />}
    {tab === "Requests" && <RequestRecords />}
    {tab === "Deploy steps" && <DeployStepRecords />}
    {tab === "Host" && <HostMetrics />}
    {tab === "Events" && <EventRecords />}
  </main>
}

function SummaryCards() {
  const cards = [
    { label: "Request rate", value: "184 req/s", detail: "+12% from previous hour", icon: PulseIcon },
    { label: "Error rate", value: "0.38%", detail: "7 errors in 1,842 requests", icon: WarningCircleIcon, attention: true },
    { label: "Avg response", value: "86 ms", detail: "p95 · 241 ms", icon: ClockIcon },
    { label: "Active deploys", value: "1", detail: "GameNation/web · building", icon: CloudArrowUpIcon },
    { label: "Failed steps", value: "2", detail: "Last 24 hours", icon: ActivityIcon, attention: true },
    { label: "Host health", value: "Healthy", detail: "34 containers running", icon: HeartbeatIcon },
  ]
  return <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card md:grid-cols-3 xl:grid-cols-6">{cards.map((card) => { const CardIcon = card.icon; return <article key={card.label} className="hf-observe-summary"><div className="flex justify-between"><p className="text-[11px] text-muted-foreground">{card.label}</p><CardIcon size={15} className={card.attention ? "text-amber-600" : "text-muted-foreground"} /></div><p className="mt-4 text-xl font-semibold tracking-tight">{card.value}</p><p className={`mt-1 text-[10px] ${card.attention ? "text-amber-700" : "text-muted-foreground"}`}>{card.detail}</p></article> })}</section>
}

function OverviewContent({ onOpen }: { onOpen: (tab: Tab) => void }) {
  return <div className="grid gap-5 xl:grid-cols-[minmax(0,1.45fr)_minmax(340px,0.7fr)]">
    <Panel title="Request traffic" subtitle="Requests per minute across all services" action={<button onClick={() => onOpen("Requests")} className="text-xs font-medium hover:underline">View requests</button>}><TrafficChart /></Panel>
    <Panel title="Host utilization" subtitle="Current primary host saturation" action={<button onClick={() => onOpen("Host")} className="text-xs font-medium hover:underline">View host</button>}><div className="space-y-5 p-5">{[{ label: "CPU", value: "28%", progress: 28, icon: CpuIcon }, { label: "Memory", value: "61%", progress: 61, icon: MemoryIcon }, { label: "Disk", value: "43%", progress: 43, icon: HardDrivesIcon }, { label: "Network", value: "18.4 MB/s", progress: 36, icon: WifiHighIcon }].map((item) => { const ItemIcon = item.icon; return <div key={item.label}><div className="mb-2 flex items-center gap-2"><ItemIcon size={14} className="text-muted-foreground" /><span className="text-xs font-medium">{item.label}</span><span className="ml-auto text-[11px] text-muted-foreground">{item.value}</span></div><div className="h-1.5 rounded-full bg-muted"><div className="h-full rounded-full bg-accent" style={{ width: `${item.progress}%` }} /></div></div> })}<div className="flex items-center gap-3 rounded-lg border bg-muted/30 p-3"><CubeIcon size={17} /><div><p className="text-xs font-medium">34 containers</p><p className="mt-0.5 text-[10px] text-muted-foreground">31 healthy · 2 restarting · 1 stopped</p></div></div></div></Panel>
    <Panel title="Recent requests" subtitle="Latest traffic across applications" action={<button onClick={() => onOpen("Requests")} className="text-xs font-medium hover:underline">View all</button>}><RequestTable rows={requests.slice(0, 5)} compact /></Panel>
    <Panel title="Recent events" subtitle="Platform and configuration activity" action={<button onClick={() => onOpen("Events")} className="text-xs font-medium hover:underline">View all</button>}><EventList rows={events.slice(0, 4)} /></Panel>
  </div>
}

function TrafficChart() {
  const data = [42, 48, 45, 56, 61, 58, 67, 63, 72, 78, 69, 74, 82, 77, 86, 91, 84, 89, 95, 88, 98, 93, 96, 100, 94, 91, 97, 92, 99, 96, 100, 98]
  return <div className="p-5"><div className="flex h-52 items-end gap-1 border-b border-dashed pb-1">{data.map((value, index) => <div key={index} className="flex-1 rounded-t-sm bg-accent/80" style={{ height: `${value}%` }} />)}</div><div className="mt-2 flex justify-between text-[9px] text-muted-foreground"><span>60m ago</span><span>45m</span><span>30m</span><span>15m</span><span>Now</span></div></div>
}

function RequestRecords() {
  const [query, setQuery] = useState("")
  const visible = requests.filter((request) => `${request.method} ${request.path} ${request.application} ${request.service}`.toLowerCase().includes(query.toLowerCase()))
  return <Panel title="Request records" subtitle="Ingress records captured by the HostForge proxy" action={<span className="text-xs text-muted-foreground">{visible.length} records</span>}><FilterBar query={query} setQuery={setQuery}><select className="hf-compact-field"><option>All applications</option><option>TaxIO</option><option>GameNation</option></select><select className="hf-compact-field"><option>All services</option><option>api</option><option>web</option></select><select className="hf-compact-field"><option>All status codes</option><option>2xx</option><option>4xx</option><option>5xx</option></select></FilterBar><RequestTable rows={visible} /></Panel>
}

function RequestTable({ rows, compact = false }: { rows: typeof requests; compact?: boolean }) {
  return <div className="overflow-x-auto"><table className={`w-full text-left ${compact ? "min-w-[760px]" : "min-w-[940px]"}`}><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Time</th><th className="px-4 py-3 font-semibold">Method</th><th className="px-4 py-3 font-semibold">Path</th><th className="px-4 py-3 font-semibold">Status</th><th className="px-4 py-3 font-semibold">Duration</th><th className="px-4 py-3 font-semibold">Application</th><th className="px-5 py-3 font-semibold">Service</th></tr></thead><tbody className="divide-y">{rows.map((request) => <tr key={`${request.time}-${request.path}`} className="hover:bg-muted/35"><td className="px-5 py-3.5 font-mono text-[10px] text-muted-foreground">{request.time}</td><td className="px-4 py-3.5"><span className="rounded bg-muted px-1.5 py-1 font-mono text-[10px] font-semibold">{request.method}</span></td><td className="max-w-72 truncate px-4 py-3.5 font-mono text-[11px]">{request.path}</td><td className="px-4 py-3.5"><StatusCode code={request.status} /></td><td className="px-4 py-3.5 text-xs text-muted-foreground">{request.duration}</td><td className="px-4 py-3.5 text-xs">{request.application}</td><td className="px-5 py-3.5 text-xs">{request.service}</td></tr>)}</tbody></table></div>
}

function DeployStepRecords() {
  return <Panel title="Deployment-step records" subtitle="Timing and outcome for every build and release stage"><FilterBar query="" setQuery={() => {}}><select className="hf-compact-field"><option>All deployments</option></select><select className="hf-compact-field"><option>All steps</option><option>Source fetched</option><option>Image built</option></select><select className="hf-compact-field"><option>All statuses</option><option>Completed</option><option>Failed</option></select></FilterBar><div className="overflow-x-auto"><table className="w-full min-w-[820px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Time</th><th className="px-4 py-3 font-semibold">Deployment</th><th className="px-4 py-3 font-semibold">Service</th><th className="px-4 py-3 font-semibold">Step</th><th className="px-4 py-3 font-semibold">Status</th><th className="px-5 py-3 text-right font-semibold">Duration</th></tr></thead><tbody className="divide-y">{deploySteps.map((step) => <tr key={`${step.time}-${step.step}`} className="hover:bg-muted/35"><td className="px-5 py-4 font-mono text-[10px] text-muted-foreground">{step.time}</td><td className="px-4 py-4 font-mono text-xs font-semibold">{step.deployment}</td><td className="px-4 py-4 text-xs">{step.service}</td><td className="px-4 py-4 text-xs">{step.step}</td><td className="px-4 py-4"><StateBadge value={step.status} /></td><td className="px-5 py-4 text-right text-xs text-muted-foreground">{step.duration}</td></tr>)}</tbody></table></div></Panel>
}

function HostMetrics() {
  return <div className="grid gap-5 lg:grid-cols-2"><HostChart title="CPU utilization" value="28%" detail="4 cores · 41% peak" icon={CpuIcon} data={hostSeries.cpu} /><HostChart title="Memory utilization" value="61%" detail="9.8 GB of 16 GB" icon={MemoryIcon} data={hostSeries.memory} /><HostChart title="Root disk utilization" value="43%" detail="84 GB of 196 GB" icon={HardDrivesIcon} data={hostSeries.disk} /><HostChart title="Network throughput" value="18.4 MB/s" detail="12.1 in · 6.3 out" icon={WifiHighIcon} data={hostSeries.network} /><Panel title="Container inventory" subtitle="Docker runtime status" className="lg:col-span-2"><div className="grid grid-cols-2 gap-px bg-border sm:grid-cols-4">{[{ label: "Running", value: "31", tone: "text-emerald-700" }, { label: "Restarting", value: "2", tone: "text-amber-700" }, { label: "Stopped", value: "1", tone: "text-muted-foreground" }, { label: "Total", value: "34", tone: "text-foreground" }].map((item) => <div key={item.label} className="bg-card p-5"><p className="text-xs text-muted-foreground">{item.label}</p><p className={`mt-3 text-2xl font-semibold ${item.tone}`}>{item.value}</p></div>)}</div></Panel></div>
}

function HostChart({ title, value, detail, icon: Icon, data }: { title: string; value: string; detail: string; icon: React.ComponentType<{ size?: number; className?: string }>; data: number[] }) {
  const max = Math.max(...data)
  return <Panel title={title} subtitle={detail} action={<div className="flex items-center gap-2"><Icon size={15} className="text-muted-foreground" /><span className="text-xs font-semibold">{value}</span></div>}><div className="p-5"><div className="flex h-40 items-end gap-1 border-b border-dashed pb-1">{data.map((point, index) => <div key={index} className="flex-1 rounded-t-sm bg-accent/80" style={{ height: `${Math.max(4, point / max * 100)}%` }} />)}</div><div className="mt-2 flex justify-between text-[9px] text-muted-foreground"><span>60m ago</span><span>30m ago</span><span>Now</span></div></div></Panel>
}

function EventRecords() { return <Panel title="Platform events" subtitle="Deployment, routing, configuration, and runtime activity" action={<span className="text-xs text-muted-foreground">Live</span>}><FilterBar query="" setQuery={() => {}}><select className="hf-compact-field"><option>All event types</option><option>Deployment</option><option>Domain</option><option>Runtime</option></select><select className="hf-compact-field"><option>All applications</option><option>TaxIO</option><option>GameNation</option></select></FilterBar><EventList rows={events} /></Panel> }

function EventList({ rows }: { rows: typeof events }) { return <div className="divide-y">{rows.map((event) => <div key={`${event.time}-${event.title}`} className="flex items-start gap-4 px-5 py-4"><span className={`mt-1 size-2.5 shrink-0 rounded-full ${event.state === "success" ? "bg-emerald-500" : event.state === "active" ? "bg-blue-500" : event.state === "error" ? "bg-red-500" : event.state === "warning" ? "bg-amber-500" : "bg-neutral-400"}`} /><div className="min-w-0"><div className="flex items-center gap-2"><p className="text-xs font-semibold">{event.title}</p><span className="rounded bg-muted px-1.5 py-0.5 text-[9px] text-muted-foreground">{event.type}</span></div><p className="mt-1 text-[11px] text-muted-foreground">{event.detail}</p></div><span className="ml-auto font-mono text-[10px] text-muted-foreground">{event.time}</span></div>)}</div> }

function FilterBar({ query, setQuery, children }: { query: string; setQuery: (value: string) => void; children: React.ReactNode }) { return <div className="flex flex-col gap-2 border-b bg-muted/30 p-4 sm:flex-row sm:items-center"><FunnelSimpleIcon size={15} className="text-muted-foreground" />{children}<label className="relative sm:ml-auto sm:w-64"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="hf-compact-field w-full pl-8" placeholder="Search records" /></label></div> }
function StatusCode({ code }: { code: number }) { return <span className={`rounded-full px-2 py-1 font-mono text-[10px] font-semibold ${code >= 500 ? "bg-red-50 text-red-700" : code >= 400 ? "bg-amber-50 text-amber-700" : "bg-emerald-50 text-emerald-700"}`}>{code}</span> }
function StateBadge({ value }: { value: string }) { const good = value === "Completed"; return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ${good ? "bg-emerald-50 text-emerald-700" : "bg-red-50 text-red-700"}`}>{good ? <CheckCircleIcon size={12} weight="fill" /> : <WarningCircleIcon size={12} weight="fill" />}{value}</span> }
function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) { return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section> }
