import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  BracketsCurlyIcon,
  CheckCircleIcon,
  ClipboardIcon,
  CopyIcon,
  CpuIcon,
  DownloadSimpleIcon,
  DotsThreeIcon,
  EyeIcon,
  EyeSlashIcon,
  GlobeIcon,
  HardDrivesIcon,
  KeyIcon,
  MagnifyingGlassIcon,
  MemoryIcon,
  PauseIcon,
  PlayIcon,
  PlusIcon,
  RocketLaunchIcon,
  ShieldCheckIcon,
  TrashIcon,
  WarningCircleIcon,
  WifiHighIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import "@/operations.css"

const runtimeLogs = [
  { time: "13:48:22.105", level: "INFO", source: "http", message: "GET /health 200 3ms", stream: "stdout" },
  { time: "13:48:20.884", level: "INFO", source: "api", message: "Assessment request completed for taxpayer tx_8392", stream: "stdout" },
  { time: "13:48:20.671", level: "DEBUG", source: "database", message: "SELECT assessments WHERE taxpayer_id=$1 duration=12ms", stream: "stdout" },
  { time: "13:48:18.407", level: "WARN", source: "queue", message: "Retrying tax-rate synchronization attempt=2", stream: "stderr" },
  { time: "13:48:17.942", level: "INFO", source: "http", message: "POST /v1/assessments 201 148ms", stream: "stdout" },
  { time: "13:48:14.231", level: "ERROR", source: "upstream", message: "FIRS region endpoint timed out after 5000ms", stream: "stderr" },
  { time: "13:48:12.006", level: "INFO", source: "worker", message: "Queued reconciliation job job_28fb", stream: "stdout" },
  { time: "13:48:09.118", level: "INFO", source: "http", message: "GET /v1/rates?year=2026 200 21ms", stream: "stdout" },
]

const metricSeries = {
  cpu: [18, 22, 20, 24, 31, 28, 34, 29, 26, 38, 42, 31, 27, 32, 36, 30, 25, 29, 33, 28, 31, 35, 30, 31],
  memory: [41, 42, 43, 42, 44, 45, 44, 46, 47, 46, 47, 48, 48, 47, 49, 48, 48, 49, 48, 48, 47, 48, 48, 48],
  ingress: [8, 14, 11, 22, 18, 25, 19, 16, 28, 34, 21, 18, 26, 31, 22, 17, 29, 38, 24, 20, 32, 27, 22, 28],
  egress: [5, 7, 6, 12, 9, 11, 8, 10, 14, 18, 12, 9, 15, 16, 11, 8, 13, 19, 12, 10, 17, 14, 12, 15],
}

const applicationVariables = [
  { key: "DATABASE_URL", value: "••••••••••••", secret: true, scope: "Application", services: "web, api, worker", updated: "2h ago" },
  { key: "REDIS_URL", value: "••••••••••••", secret: true, scope: "Application", services: "api, worker", updated: "2h ago" },
  { key: "APP_ENV", value: "production", secret: false, scope: "Application", services: "All services", updated: "3d ago" },
  { key: "SENTRY_DSN", value: "••••••••••••", secret: true, scope: "Application", services: "web, api", updated: "5d ago" },
]

const serviceVariables = [
  { key: "PORT", value: "3000", secret: false, scope: "Service override", services: "api", updated: "1h ago" },
  { key: "LOG_LEVEL", value: "info", secret: false, scope: "Service", services: "api", updated: "1d ago" },
  { key: "FIRS_API_KEY", value: "••••••••••••", secret: true, scope: "Service", services: "api", updated: "2d ago" },
  { key: "DATABASE_URL", value: "Inherited", secret: true, scope: "Application", services: "api", updated: "2h ago" },
]

const applicationDomains = [
  { domain: "taxio.ng", service: "web", dns: "Verified", tls: "Active", routing: "Active", updated: "2d ago" },
  { domain: "www.taxio.ng", service: "web", dns: "Verified", tls: "Active", routing: "Active", updated: "2d ago" },
  { domain: "api.taxio.ng", service: "api", dns: "Verified", tls: "Active", routing: "Active", updated: "1d ago" },
  { domain: "staging.taxio.ng", service: "web", dns: "Pending DNS", tls: "Waiting", routing: "Inactive", updated: "18m ago" },
]

function PageHeader({ title, description, back, children }: { title: string; description: string; back: { label: string; href: string }; children?: React.ReactNode }) {
  return <><Link to={back.href} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{back.label}</Link><div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 text-sm text-muted-foreground">{description}</p></div>{children && <div className="flex flex-wrap gap-2 sm:ml-auto">{children}</div>}</div></>
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function ServiceTabs({ active, service }: { active: string; service: string }) {
  const tabs = ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]
  return <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? `/applications/taxio/services/${service}` : `/applications/taxio/services/${service}/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav>
}

function ApplicationTabs({ active }: { active: string }) {
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? "/applications/taxio" : `/applications/taxio/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav>
}

export function ServiceLogs({ service = "api" }: { service?: string }) {
  const [streaming, setStreaming] = useState(true)
  const [query, setQuery] = useState("")
  const [level, setLevel] = useState("All levels")
  const visible = runtimeLogs.filter((log) => (level === "All levels" || log.level === level) && `${log.source} ${log.message}`.toLowerCase().includes(query.toLowerCase()))
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Runtime logs" description={`Live application output from the TaxIO ${service} service.`} back={{ label: `${service} overview`, href: `/applications/taxio/services/${service}` }}><Button variant="outline"><DownloadSimpleIcon />Download</Button><Button onClick={() => setStreaming(!streaming)}>{streaming ? <PauseIcon /> : <PlayIcon />}{streaming ? "Pause stream" : "Resume stream"}</Button></PageHeader><ServiceTabs active="Logs" service={service} />
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/75 p-4 xl:flex-row xl:items-center"><div className="flex items-center gap-2"><span className={`size-2 rounded-full ${streaming ? "bg-emerald-500 animate-pulse" : "bg-amber-500"}`} /><span className="text-xs font-semibold">{streaming ? "Live stream" : "Stream paused"}</span></div><div className="grid flex-1 grid-cols-2 gap-2 sm:grid-cols-4 xl:ml-5"><select className="hf-compact-field"><option>Last 30 minutes</option><option>Last hour</option><option>Last 24 hours</option></select><select value={level} onChange={(event) => setLevel(event.target.value)} className="hf-compact-field"><option>All levels</option><option>INFO</option><option>DEBUG</option><option>WARN</option><option>ERROR</option></select><select className="hf-compact-field"><option>All streams</option><option>stdout</option><option>stderr</option></select><select className="hf-compact-field"><option>hf_taxio_api_01</option></select></div><label className="relative xl:w-64"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="hf-compact-field w-full pl-8" placeholder="Search visible logs" /></label><Button variant="outline" size="sm">Clear visible</Button></header>
      <div className="hf-runtime-log" role="log">{visible.map((log) => <div key={`${log.time}-${log.message}`} className="hf-runtime-log-line"><span className="font-mono text-[10px] text-neutral-500">{log.time}</span><span className={`hf-level hf-level-${log.level.toLowerCase()}`}>{log.level}</span><span className="font-mono text-[10px] text-neutral-500">{log.source}</span><span className="font-mono text-[11px] text-neutral-200">{log.message}</span></div>)}</div><footer className="flex items-center gap-4 border-t bg-muted/30 px-4 py-2.5 text-[10px] text-muted-foreground"><span>{visible.length} visible events</span><span>stdout + stderr</span><span className="ml-auto">Auto-scroll {streaming ? "enabled" : "paused"}</span></footer></section>
  </main>
}

export function ServiceMetrics({ service = "api" }: { service?: string }) {
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Metrics" description={`Container resource performance for the TaxIO ${service} service.`} back={{ label: `${service} overview`, href: `/applications/taxio/services/${service}` }}><select className="hf-compact-field"><option>Last hour</option><option>Last 6 hours</option><option>Last 24 hours</option></select><Button variant="outline"><ActivityIcon />Refresh</Button></PageHeader><ServiceTabs active="Metrics" service={service} />
    <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "CPU", value: "31%", detail: "42% peak", icon: CpuIcon }, { label: "Memory", value: "486 MB", detail: "1 GB limit", icon: MemoryIcon }, { label: "Network ingress", value: "4.8 MB/s", detail: "1.2 GB this hour", icon: WifiHighIcon }, { label: "Restarts", value: "0", detail: "Last 30 days", icon: HardDrivesIcon }].map((item) => { const ItemIcon = item.icon; return <article key={item.label} className="hf-operation-summary"><div className="flex justify-between"><p className="text-xs text-muted-foreground">{item.label}</p><ItemIcon size={16} className="text-muted-foreground" /></div><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{item.detail}</p></article> })}</section>
    <div className="grid gap-5 lg:grid-cols-2"><MetricChart title="CPU usage" subtitle="Percentage across allocated cores" value="31% current" data={metricSeries.cpu} /><MetricChart title="Memory usage" subtitle="Percentage of 1 GB container limit" value="486 MB current" data={metricSeries.memory} /><MetricChart title="Network ingress" subtitle="Megabytes received per interval" value="4.8 MB/s" data={metricSeries.ingress} /><MetricChart title="Network egress" subtitle="Megabytes sent per interval" value="2.1 MB/s" data={metricSeries.egress} /></div>
    <div className="mt-5 flex items-start gap-3 rounded-xl border bg-muted/40 p-4"><ActivityIcon className="mt-0.5 text-muted-foreground" size={18} /><div><p className="text-xs font-semibold">Container metrics only</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Request rate, error rate, and response time are omitted until application-level telemetry is connected.</p></div></div>
  </main>
}

function MetricChart({ title, subtitle, value, data }: { title: string; subtitle: string; value: string; data: number[] }) {
  const max = Math.max(...data)
  return <Panel title={title} subtitle={subtitle} action={<span className="text-xs font-semibold tabular-nums">{value}</span>}><div className="p-5"><div className="flex h-44 items-end gap-1 border-b border-dashed pb-1">{data.map((point, index) => <div key={index} className="group relative flex-1 rounded-t-sm bg-accent/80 transition-opacity hover:opacity-65" style={{ height: `${Math.max(5, (point / max) * 100)}%` }}><span className="pointer-events-none absolute bottom-full left-1/2 mb-1 hidden -translate-x-1/2 rounded bg-foreground px-1.5 py-1 text-[9px] text-background group-hover:block">{point}</span></div>)}</div><div className="mt-2 flex justify-between text-[9px] text-muted-foreground"><span>60m ago</span><span>30m ago</span><span>Now</span></div></div></Panel>
}

export function EnvironmentScreen({ scope, service = "api" }: { scope: "application" | "service"; service?: string }) {
  const [revealed, setRevealed] = useState<string[]>([])
  const [changed, setChanged] = useState(false)
  const variables = scope === "application" ? applicationVariables : serviceVariables
  const base = scope === "application" ? "/applications/taxio" : `/applications/taxio/services/${service}`
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Environment" description={scope === "application" ? "Manage variables inherited by TaxIO services." : `Manage variables and overrides for the ${service} service.`} back={{ label: scope === "application" ? "TaxIO overview" : `${service} overview`, href: base }}><Button variant="outline"><DownloadSimpleIcon />Export names</Button><Button variant="outline"><ClipboardIcon />Import .env</Button><Button onClick={() => setChanged(true)}><PlusIcon />Add variable</Button></PageHeader>{scope === "application" ? <ApplicationTabs active="Environment" /> : <ServiceTabs active="Environment" service={service} />}
    {changed && <div className="mb-5 flex flex-col gap-3 rounded-xl border border-amber-300 bg-amber-50 p-4 sm:flex-row sm:items-center"><WarningCircleIcon size={20} className="shrink-0 text-amber-700" /><div><p className="text-xs font-semibold text-amber-900">Configuration changed. A new deployment is required.</p><p className="mt-1 text-[11px] text-amber-700">Running containers still use the previous environment.</p></div><div className="flex gap-2 sm:ml-auto"><Button variant="outline" size="sm" onClick={() => setChanged(false)}>Deploy later</Button><Button size="sm"><RocketLaunchIcon weight="fill" />Deploy now</Button></div></div>}
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex items-center gap-3 border-b bg-muted/70 px-5 py-4"><BracketsCurlyIcon size={17} /><div><h2 className="text-sm font-semibold">{scope === "application" ? "Shared variables" : "Service variables"}</h2><p className="mt-0.5 text-xs text-muted-foreground">Secret values remain encrypted and hidden by default.</p></div><span className="ml-auto text-xs text-muted-foreground">{variables.length} variables</span></header><div className="overflow-x-auto"><table className="w-full min-w-[900px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Key</th><th className="px-4 py-3 font-semibold">Value</th><th className="px-4 py-3 font-semibold">Scope</th><th className="px-4 py-3 font-semibold">Services</th><th className="px-4 py-3 font-semibold">Last updated</th><th className="px-5 py-3 text-right font-semibold">Actions</th></tr></thead><tbody className="divide-y">{variables.map((variable) => { const showing = revealed.includes(variable.key); return <tr key={variable.key} className="hover:bg-muted/35"><td className="px-5 py-4"><span className="flex items-center gap-2 font-mono text-xs font-semibold">{variable.secret ? <KeyIcon size={14} className="text-muted-foreground" /> : <BracketsCurlyIcon size={14} className="text-muted-foreground" />}{variable.key}</span></td><td className="px-4 py-4"><div className="flex items-center gap-2"><span className="font-mono text-xs text-muted-foreground">{variable.secret && showing ? "postgres://hf_user:7f2..." : variable.value}</span>{variable.secret && variable.value !== "Inherited" && <button onClick={() => setRevealed(showing ? revealed.filter((item) => item !== variable.key) : [...revealed, variable.key])} className="text-muted-foreground hover:text-foreground" aria-label={showing ? "Hide value" : "Reveal value"}>{showing ? <EyeSlashIcon size={14} /> : <EyeIcon size={14} />}</button>}</div></td><td className="px-4 py-4"><span className="rounded bg-muted px-2 py-1 text-[10px] font-medium">{variable.scope}</span></td><td className="px-4 py-4 text-xs text-muted-foreground">{variable.services}</td><td className="px-4 py-4 text-xs text-muted-foreground">{variable.updated}</td><td className="px-5 py-4"><div className="flex justify-end gap-1"><Button variant="ghost" size="icon" aria-label="Copy variable"><CopyIcon /></Button><Button variant="ghost" size="icon" aria-label="Delete variable"><TrashIcon /></Button><Button variant="ghost" size="icon" aria-label="More actions"><DotsThreeIcon weight="bold" /></Button></div></td></tr> })}</tbody></table></div></section>
  </main>
}

export function DomainsScreen({ scope, service = "api" }: { scope: "application" | "service"; service?: string }) {
  const [adding, setAdding] = useState(false)
  const [step, setStep] = useState(1)
  const domains = scope === "application" ? applicationDomains : applicationDomains.filter((item) => item.service === service)
  const base = scope === "application" ? "/applications/taxio" : `/applications/taxio/services/${service}`
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Domains" description={scope === "application" ? "Manage public routes across every TaxIO service." : `Manage public routes for the ${service} service.`} back={{ label: scope === "application" ? "TaxIO overview" : `${service} overview`, href: base }}><Button variant="outline"><ActivityIcon />Check all DNS</Button><Button onClick={() => { setAdding(true); setStep(1) }}><PlusIcon />Add domain</Button></PageHeader>{scope === "application" ? <ApplicationTabs active="Domains" /> : <ServiceTabs active="Domains" service={service} />}
    {adding && <Panel title="Add domain" subtitle="Verify ownership, activate routing, and provision TLS" action={<button className="text-xs text-muted-foreground hover:text-foreground" onClick={() => setAdding(false)}>Cancel</button>}><div className="grid border-b sm:grid-cols-6">{["Domain", "Service", "DNS records", "Check DNS", "Activate", "TLS"].map((label, index) => <div key={label} className={`flex items-center gap-2 border-b px-3 py-3 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0 ${step === index + 1 ? "bg-muted/75" : ""}`}><span className={`grid size-5 shrink-0 place-items-center rounded-full text-[9px] font-semibold ${index + 1 <= step ? "bg-accent text-accent-foreground" : "border text-muted-foreground"}`}>{index + 1 < step ? "✓" : index + 1}</span><span className="hidden text-[10px] font-semibold xl:block">{label}</span></div>)}</div><div className="p-5"><div className="grid gap-5 md:grid-cols-[1fr_0.8fr]"><div>{step === 1 && <Field label="Domain name"><input className="hf-field" defaultValue="reports.taxio.ng" /></Field>}{step === 2 && <Field label="Assigned service"><select className="hf-field"><option>web</option><option>api</option><option>worker</option></select></Field>}{step === 3 && <div className="overflow-hidden rounded-lg border"><DnsRow type="CNAME" name="reports" value="routes.hostforge.dev" /><DnsRow type="TXT" name="_hostforge.reports" value="hf_verify=1f829a" /></div>}{step === 4 && <StateCard icon={<ActivityIcon size={20} />} title="Ready to check DNS" description="HostForge will query authoritative nameservers for both required records." />}{step === 5 && <StateCard icon={<GlobeIcon size={20} />} title="DNS verified" description="Activate the Caddy route from reports.taxio.ng to the selected service." />}{step === 6 && <StateCard icon={<ShieldCheckIcon size={20} />} title="Route active · TLS provisioning" description="Caddy is requesting a certificate. This normally completes within one minute." />}</div><div className="rounded-lg border bg-muted/35 p-4"><p className="text-xs font-semibold">Three separate states</p><div className="mt-4 space-y-3"><DomainState icon={<CheckCircleIcon size={16} weight="fill" />} label="Registrar verification" value={step >= 5 ? "Verified" : "Pending"} complete={step >= 5} /><DomainState icon={<GlobeIcon size={16} />} label="HostForge routing" value={step >= 6 ? "Active" : "Inactive"} complete={step >= 6} /><DomainState icon={<ShieldCheckIcon size={16} />} label="TLS certificate" value={step >= 6 ? "Provisioning" : "Waiting"} complete={false} /></div></div></div><div className="mt-5 flex justify-between"><Button variant="outline" disabled={step === 1} onClick={() => setStep(Math.max(1, step - 1))}><ArrowLeftIcon />Back</Button><Button onClick={() => step < 6 ? setStep(step + 1) : setAdding(false)}>{step < 6 ? "Continue" : "Finish"}</Button></div></div></Panel>}
    <section className={`overflow-hidden rounded-xl border bg-card ${adding ? "mt-5" : ""}`}><header className="flex items-center gap-3 border-b bg-muted/70 px-5 py-4"><GlobeIcon size={17} /><div><h2 className="text-sm font-semibold">Configured domains</h2><p className="mt-0.5 text-xs text-muted-foreground">DNS, routing, and certificate health</p></div><span className="ml-auto text-xs text-muted-foreground">{domains.length} domains</span></header><div className="overflow-x-auto"><table className="w-full min-w-[920px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Domain</th><th className="px-4 py-3 font-semibold">Assigned service</th><th className="px-4 py-3 font-semibold">DNS</th><th className="px-4 py-3 font-semibold">TLS</th><th className="px-4 py-3 font-semibold">Routing</th><th className="px-4 py-3 font-semibold">Updated</th><th className="px-5 py-3 text-right font-semibold">Actions</th></tr></thead><tbody className="divide-y">{domains.map((domain) => <tr key={domain.domain} className="hover:bg-muted/35"><td className="px-5 py-4"><Link to={`https://${domain.domain}`} className="flex items-center gap-1.5 text-xs font-semibold hover:underline">{domain.domain}<ArrowSquareOutIcon size={12} /></Link></td><td className="px-4 py-4 text-xs">{domain.service}</td><td className="px-4 py-4"><DomainBadge value={domain.dns} /></td><td className="px-4 py-4"><DomainBadge value={domain.tls} /></td><td className="px-4 py-4"><DomainBadge value={domain.routing} /></td><td className="px-4 py-4 text-xs text-muted-foreground">{domain.updated}</td><td className="px-5 py-4 text-right"><Button variant="ghost" size="icon"><DotsThreeIcon weight="bold" /></Button></td></tr>)}</tbody></table></div></section>
  </main>
}

function DomainBadge({ value }: { value: string }) {
  const active = value === "Verified" || value === "Active"
  return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ring-inset ${active ? "bg-emerald-50 text-emerald-700 ring-emerald-600/15" : "bg-amber-50 text-amber-700 ring-amber-600/15"}`}><span className="size-1.5 rounded-full bg-current" />{value}</span>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label> }
function DnsRow({ type, name, value }: { type: string; name: string; value: string }) { return <div className="grid grid-cols-[70px_1fr_1.4fr_auto] gap-3 border-b p-3 last:border-b-0"><span className="rounded bg-muted px-2 py-1 text-center font-mono text-[10px]">{type}</span><span className="font-mono text-[11px]">{name}</span><span className="truncate font-mono text-[11px] text-muted-foreground">{value}</span><CopyIcon size={14} className="text-muted-foreground" /></div> }
function StateCard({ icon, title, description }: { icon: React.ReactNode; title: string; description: string }) { return <div className="rounded-lg border p-5"><span className="grid size-10 place-items-center rounded-lg bg-accent text-accent-foreground">{icon}</span><h3 className="mt-4 text-sm font-semibold">{title}</h3><p className="mt-2 text-xs leading-5 text-muted-foreground">{description}</p></div> }
function DomainState({ icon, label, value, complete }: { icon: React.ReactNode; label: string; value: string; complete: boolean }) { return <div className="flex items-center gap-2.5 text-xs"><span className={complete ? "text-emerald-600" : "text-muted-foreground"}>{icon}</span><span>{label}</span><span className={`ml-auto text-[10px] font-semibold ${complete ? "text-emerald-700" : "text-muted-foreground"}`}>{value}</span></div> }
