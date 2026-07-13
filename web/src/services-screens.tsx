import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  ArrowSquareOutIcon,
  BracketsCurlyIcon,
  CheckCircleIcon,
  CheckIcon,
  CloudArrowUpIcon,
  CodeIcon,
  CopyIcon,
  CpuIcon,
  CubeIcon,
  DotsThreeIcon,
  GithubLogoIcon,
  GitBranchIcon,
  GlobeIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  LinkIcon,
  MagnifyingGlassIcon,
  MemoryIcon,
  PauseIcon,
  PlusIcon,
  RocketLaunchIcon,
  TerminalWindowIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import { navigateTo } from "@/navigation"
import "@/services.css"

const serviceRows = [
  { name: "web", type: "Web service", source: "mr-fury/taxio-web", branch: "main", status: "Running", deployment: "8m ago", url: "taxio.ng" },
  { name: "api", type: "API service", source: "mr-fury/taxio-api", branch: "main", status: "Running", deployment: "24m ago", url: "api.taxio.ng" },
  { name: "worker", type: "Worker", source: "mr-fury/taxio", branch: "main", status: "Running", deployment: "1h ago", url: null },
  { name: "scheduler", type: "Scheduled job", source: "mr-fury/taxio", branch: "jobs", status: "Awaiting deployment", deployment: "Never", url: null },
]

const addServiceSteps = ["Source", "Configuration", "Environment", "Networking", "Deploy"]

function StatusPill({ status }: { status: string }) {
  const style = status === "Running" || status === "Healthy" || status === "Live" ? "bg-emerald-50 text-emerald-700 ring-emerald-600/15" : status === "Deploying" || status === "Building" ? "bg-blue-50 text-blue-700 ring-blue-600/15" : status === "Failed" ? "bg-red-50 text-red-700 ring-red-600/15" : "bg-neutral-100 text-neutral-600 ring-neutral-500/15"
  return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ring-inset ${style}`}><span className="size-1.5 rounded-full bg-current" />{status}</span>
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div className="min-w-0"><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function ApplicationTabs({ active }: { active: string }) {
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1" aria-label="Application navigation"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? "/applications/taxio" : `/applications/taxio/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav>
}

export function ServicesList() {
  const [query, setQuery] = useState("")
  const visibleRows = serviceRows.filter((service) => `${service.name} ${service.type} ${service.source}`.toLowerCase().includes(query.toLowerCase()))
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to="/applications/taxio" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />TaxIO overview</Link>
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Services</h1><p className="mt-2 text-sm text-muted-foreground">Deploy and manage the components of TaxIO.</p></div><Button className="sm:ml-auto" onClick={() => navigateTo("/applications/taxio/services/new")}><PlusIcon />Add service</Button></div>
      <ApplicationTabs active="Services" />

      <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">
        {[{ label: "Total services", value: "4", detail: "Across 3 repositories" }, { label: "Running", value: "3", detail: "All checks passing" }, { label: "Deploying", value: "0", detail: "No active builds" }, { label: "Needs action", value: "1", detail: "Awaiting first deploy" }].map((item) => <article key={item.label} className="hf-service-summary"><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{item.detail}</p></article>)}
      </section>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/70 p-4 sm:flex-row sm:items-center"><div><h2 className="text-sm font-semibold">All services</h2><p className="mt-0.5 text-xs text-muted-foreground">Deployable units within TaxIO</p></div><label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full rounded-md border bg-card pl-9 pr-3 text-xs outline-none focus:ring-2 focus:ring-ring/20" placeholder="Search services" /></label></header>
        <div className="overflow-x-auto"><table className="w-full min-w-[960px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Service</th><th className="px-4 py-3 font-semibold">Type</th><th className="px-4 py-3 font-semibold">Source</th><th className="px-4 py-3 font-semibold">Branch</th><th className="px-4 py-3 font-semibold">Status</th><th className="px-4 py-3 font-semibold">Latest deployment</th><th className="px-4 py-3 font-semibold">URL</th><th className="px-5 py-3 text-right font-semibold">Actions</th></tr></thead><tbody className="divide-y">{visibleRows.map((service) => <tr key={service.name} className="group hover:bg-muted/35"><td className="px-5 py-4"><Link to={`/applications/taxio/services/${service.name}`} className="flex items-center gap-3"><span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={17} weight="fill" /></span><span className="text-xs font-semibold group-hover:underline">{service.name}</span></Link></td><td className="px-4 py-4 text-xs text-muted-foreground">{service.type}</td><td className="px-4 py-4"><span className="flex items-center gap-1.5 text-[11px]"><GithubLogoIcon size={14} />{service.source}</span></td><td className="px-4 py-4"><span className="flex items-center gap-1.5 font-mono text-[11px]"><GitBranchIcon size={13} />{service.branch}</span></td><td className="px-4 py-4"><StatusPill status={service.status} /></td><td className="px-4 py-4 text-xs text-muted-foreground">{service.deployment}</td><td className="px-4 py-4">{service.url ? <Link to={`https://${service.url}`} className="flex items-center gap-1 text-xs hover:underline">{service.url}<ArrowSquareOutIcon size={12} /></Link> : <span className="text-xs text-muted-foreground">Internal</span>}</td><td className="px-5 py-4 text-right"><Button variant="ghost" size="icon" aria-label={`Actions for ${service.name}`}><DotsThreeIcon weight="bold" /></Button></td></tr>)}</tbody></table></div>
        <footer className="flex justify-between border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground"><span>{visibleRows.length} services</span><span>Synced from runtime</span></footer>
      </section>
    </main>
  )
}

export function AddService() {
  const [step, setStep] = useState(1)
  const [source, setSource] = useState("github")
  const [variables, setVariables] = useState([{ key: "NODE_ENV", value: "production", secret: false }, { key: "DATABASE_URL", value: "••••••••••••", secret: true }])
  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to="/applications/taxio/services" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />Back to services</Link>
      <div className="mb-8"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Add service</h1><p className="mt-2 text-sm text-muted-foreground">Connect a source and configure a deployable component for TaxIO.</p></div>

      <div className="mb-5 grid overflow-hidden rounded-xl border bg-card sm:grid-cols-5">{addServiceSteps.map((label, index) => { const number = index + 1; const complete = number < step; const active = number === step; return <button key={label} onClick={() => number < step && setStep(number)} className={`flex items-center gap-2 border-b px-3 py-3.5 text-left last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0 ${active ? "bg-muted/75" : ""}`}><span className={`grid size-6 shrink-0 place-items-center rounded-full text-[10px] font-semibold ${active || complete ? "bg-accent text-accent-foreground" : "border text-muted-foreground"}`}>{complete ? <CheckIcon size={11} weight="bold" /> : number}</span><span className={`hidden text-[11px] font-semibold lg:block ${!active && !complete ? "text-muted-foreground" : ""}`}>{label}</span></button> })}</div>

      <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-6 py-4"><h2 className="text-sm font-semibold">{addServiceSteps[step - 1]}</h2><p className="mt-1 text-xs text-muted-foreground">{["Choose how HostForge should access the source repository.", "Configure build, runtime, and health-check behavior.", "Add service variables without exposing secret values.", "Configure the public route and internal port.", "Review the service and start its first deployment."][step - 1]}</p></header>
        {step === 1 && <div className="p-6"><div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">{[{ id: "github", title: "GitHub App", description: "Select an installed repository.", icon: <GithubLogoIcon size={21} /> }, { id: "public", title: "Public URL", description: "Clone without credentials.", icon: <LinkIcon size={21} /> }, { id: "token", title: "Access token", description: "Use a personal access token.", icon: <CodeIcon size={21} /> }, { id: "ssh", title: "SSH deploy key", description: "Authenticate with an SSH key.", icon: <TerminalWindowIcon size={21} /> }].map((option) => <button key={option.id} onClick={() => setSource(option.id)} className={`relative min-h-36 rounded-lg border p-4 text-left ${source === option.id ? "border-foreground bg-muted/60 ring-1 ring-foreground" : "hover:bg-muted/40"}`}><span className="text-muted-foreground">{option.icon}</span><span className="mt-4 block text-xs font-semibold">{option.title}</span><span className="mt-1.5 block text-[11px] leading-5 text-muted-foreground">{option.description}</span>{source === option.id && <CheckCircleIcon className="absolute right-3 top-3" size={17} weight="fill" />}</button>)}</div><div className="mt-5 grid gap-4 rounded-lg border bg-muted/30 p-5 sm:grid-cols-3"><Field label="Installation"><select className="hf-field"><option>Mr Fury</option></select></Field><Field label="Repository"><select className="hf-field"><option>mr-fury/taxio</option></select></Field><Field label="Branch"><div className="relative"><GitBranchIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><input className="hf-field pl-9" defaultValue="main" /></div></Field></div></div>}
        {step === 2 && <div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Service name"><input className="hf-field" defaultValue="api" /></Field><Field label="Service type"><select className="hf-field"><option>Web service</option><option>Worker</option><option>Scheduled job</option><option>Static site</option></select></Field><Field label="Root directory"><input className="hf-field font-mono" defaultValue="apps/api" /></Field><Field label="Build method"><select className="hf-field"><option>Auto-detected · Nixpacks</option><option>Dockerfile</option><option>Custom commands</option></select></Field><Field label="Build command"><input className="hf-field font-mono" defaultValue="npm run build" /></Field><Field label="Start command"><input className="hf-field font-mono" defaultValue="npm run start" /></Field><Field label="Internal port"><input className="hf-field font-mono" defaultValue="3000" /></Field><Field label="Health-check path"><input className="hf-field font-mono" defaultValue="/health" /></Field></div>}
        {step === 3 && <div className="p-6"><div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center"><div><h3 className="text-xs font-semibold">Environment variables</h3><p className="mt-1 text-[11px] text-muted-foreground">Secret values are encrypted and never shown again.</p></div><div className="flex gap-2 sm:ml-auto"><Button variant="outline" size="sm">Import .env</Button><Button variant="outline" size="sm" onClick={() => setVariables([...variables, { key: "", value: "", secret: false }])}><PlusIcon />Add variable</Button></div></div><div className="overflow-hidden rounded-lg border"><div className="grid grid-cols-[1fr_1fr_90px] gap-3 border-b bg-muted/70 px-4 py-2.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground"><span>Key</span><span>Value</span><span>Secret</span></div>{variables.map((variable, index) => <div key={index} className="grid grid-cols-[1fr_1fr_90px] gap-3 border-b p-3 last:border-b-0"><input className="hf-field font-mono" value={variable.key} onChange={(event) => setVariables(variables.map((item, itemIndex) => itemIndex === index ? { ...item, key: event.target.value } : item))} /><input className="hf-field font-mono" value={variable.value} onChange={(event) => setVariables(variables.map((item, itemIndex) => itemIndex === index ? { ...item, value: event.target.value } : item))} /><button onClick={() => setVariables(variables.map((item, itemIndex) => itemIndex === index ? { ...item, secret: !item.secret } : item))} className={`rounded-md border text-[10px] font-semibold ${variable.secret ? "bg-accent text-accent-foreground" : "bg-card text-muted-foreground"}`}>{variable.secret ? "Secret" : "Plain"}</button></div>)}</div></div>}
        {step === 4 && <div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Platform hostname"><div className="flex"><input className="hf-field rounded-r-none font-mono" defaultValue="api" /><span className="flex items-center rounded-r-md border border-l-0 bg-muted px-3 text-xs text-muted-foreground">.hostforge.dev</span></div></Field><Field label="Internal port"><input className="hf-field font-mono" defaultValue="3000" /></Field><Field label="Health-check path"><input className="hf-field font-mono" defaultValue="/health" /></Field><Field label="Health-check interval"><select className="hf-field"><option>Every 30 seconds</option><option>Every minute</option></select></Field><div className="rounded-lg border bg-muted/35 p-4 sm:col-span-2"><div className="flex items-start gap-3"><GlobeIcon className="mt-0.5 text-muted-foreground" size={18} /><div><p className="text-xs font-semibold">Custom domains can be added after deployment</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">HostForge first provisions a platform URL so the service can be verified before DNS changes.</p></div></div></div></div>}
        {step === 5 && <div className="grid gap-5 p-6 md:grid-cols-[1fr_0.8fr]"><div className="overflow-hidden rounded-lg border"><ReviewRow label="Service" value="api · Web service" /><ReviewRow label="Source" value="mr-fury/taxio · main" /><ReviewRow label="Build" value="Nixpacks · npm run build" /><ReviewRow label="Runtime" value="npm run start · port 3000" /><ReviewRow label="Environment" value={`${variables.length} variables · ${variables.filter((item) => item.secret).length} secret`} /><ReviewRow label="URL" value="api.hostforge.dev" /></div><div className="rounded-lg border bg-muted/40 p-5"><span className="grid size-10 place-items-center rounded-lg bg-accent text-accent-foreground"><RocketLaunchIcon size={19} weight="fill" /></span><h3 className="mt-4 text-sm font-semibold">Ready for first deployment</h3><p className="mt-2 text-xs leading-5 text-muted-foreground">HostForge will clone the repository, build the service, create its container, and stream each deployment phase live.</p></div></div>}
        <footer className="flex items-center justify-between border-t bg-muted/30 px-6 py-4"><Button variant="outline" disabled={step === 1} onClick={() => setStep(Math.max(1, step - 1))}><ArrowLeftIcon />Back</Button>{step < 5 ? <Button onClick={() => setStep(Math.min(5, step + 1))}>Continue <ArrowRightIcon /></Button> : <Button onClick={() => navigateTo("/applications/taxio/services/api/deployments/dep_BUILD01")}><RocketLaunchIcon weight="fill" />Create and deploy</Button>}</footer>
      </section>
    </main>
  )
}

export function ServiceOverview({ service = "api" }: { service?: string }) {
  const tabs = ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to="/applications/taxio/services" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />TaxIO services</Link>
      <div className="mb-6 flex flex-col gap-4 xl:flex-row xl:items-end"><div className="flex items-start gap-4"><span className="grid size-12 place-items-center rounded-xl bg-accent text-accent-foreground"><CubeIcon size={22} weight="fill" /></span><div><p className="mb-1 flex items-center gap-2 text-xs font-medium text-emerald-700"><span className="size-1.5 rounded-full bg-emerald-500" />Running</p><h1 className="text-3xl font-semibold tracking-[-0.035em]">{service}</h1><p className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"><span>TaxIO</span><span>Web service</span><span className="flex items-center gap-1"><GithubLogoIcon size={13} />mr-fury/taxio</span><span className="flex items-center gap-1"><GitBranchIcon size={13} />main</span></p></div></div><div className="flex flex-wrap gap-2 xl:ml-auto"><Button variant="outline"><PauseIcon />Stop</Button><Button variant="outline"><ActivityIcon />Restart</Button><Button><RocketLaunchIcon weight="fill" />Deploy</Button><Button variant="outline" size="icon" aria-label="More service actions"><DotsThreeIcon weight="bold" /></Button></div></div>

      <div className="mb-5 flex flex-col gap-3 rounded-xl border bg-card p-4 sm:flex-row sm:items-center"><div><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Public URL</p><Link to="https://api.taxio.ng" className="mt-1.5 flex items-center gap-1 text-xs font-medium hover:underline">api.taxio.ng<ArrowSquareOutIcon size={12} /></Link></div><div className="sm:ml-auto sm:text-right"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Latest commit</p><p className="mt-1.5 font-mono text-xs">13d298a · Validate taxpayer region codes</p></div><Button variant="ghost" size="icon" aria-label="Copy service URL"><CopyIcon /></Button></div>
      <nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? `/applications/taxio/services/${service}` : `/applications/taxio/services/${service}/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === "Overview" ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.75fr)]">
        <Panel title="Runtime" subtitle="Current container and process state" action={<StatusPill status="Running" />}><div className="grid grid-cols-2 gap-px bg-border md:grid-cols-3">{[{ label: "Container", value: "hf_taxio_api_01" }, { label: "Internal port", value: "3000" }, { label: "Started", value: "Today, 09:42" }, { label: "Uptime", value: "3h 18m" }, { label: "Deployment", value: "dep_7H3KD9" }, { label: "Health checks", value: "128 passed" }].map((item) => <div key={item.label} className="bg-card p-4"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{item.label}</p><p className="mt-2 font-mono text-xs font-medium">{item.value}</p></div>)}</div></Panel>

        <Panel title="Resource usage" subtitle="Last 15 minutes"><div className="space-y-5 p-5">{[{ label: "CPU", value: "31%", progress: 31, icon: CpuIcon }, { label: "Memory", value: "486 MB / 1 GB", progress: 48, icon: MemoryIcon }, { label: "Network", value: "4.8 MB/s", progress: 38, icon: ActivityIcon }, { label: "Container health", value: "Healthy", progress: 100, icon: HeartbeatIcon }].map((metric) => { const MetricIcon = metric.icon; return <div key={metric.label}><div className="mb-2 flex items-center gap-2"><MetricIcon size={14} className="text-muted-foreground" /><span className="text-xs font-medium">{metric.label}</span><span className="ml-auto text-[11px] text-muted-foreground">{metric.value}</span></div><div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-accent" style={{ width: `${metric.progress}%` }} /></div></div> })}</div></Panel>

        <Panel title="Latest deployment" subtitle="Production · deployed 24 minutes ago" action={<Link to={`/applications/taxio/services/${service}/deployments/dep_7H3KD9`} className="text-xs font-medium hover:underline">View deployment</Link>}><div className="grid gap-5 p-5 sm:grid-cols-[auto_1fr]"><span className="grid size-10 place-items-center rounded-lg bg-emerald-50 text-emerald-700"><CheckCircleIcon size={20} weight="fill" /></span><div><div className="flex flex-wrap items-center gap-2"><StatusPill status="Healthy" /><span className="font-mono text-xs">dep_7H3KD9</span></div><p className="mt-3 text-sm font-medium">13d298a · Validate taxpayer region codes</p><div className="mt-4 grid grid-cols-2 gap-4 text-[11px] sm:grid-cols-4"><div><p className="text-muted-foreground">Author</p><p className="mt-1 font-medium">Mr Fury</p></div><div><p className="text-muted-foreground">Trigger</p><p className="mt-1 font-medium">Git push</p></div><div><p className="text-muted-foreground">Duration</p><p className="mt-1 font-medium">1m 26s</p></div><div><p className="text-muted-foreground">Started</p><p className="mt-1 font-medium">10:36</p></div></div></div></div></Panel>

        <Panel title="Source configuration" subtitle="Repository and build settings" action={<Button variant="ghost" size="sm">Edit</Button>}><div className="divide-y">{[{ icon: GithubLogoIcon, label: "Repository", value: "mr-fury/taxio" }, { icon: GitBranchIcon, label: "Branch", value: "main · auto-deploy on" }, { icon: CodeIcon, label: "Build method", value: "Nixpacks · Node.js" }, { icon: HardDrivesIcon, label: "Root directory", value: "apps/api" }].map((item) => { const ItemIcon = item.icon; return <div key={item.label} className="flex items-center gap-3 px-5 py-3.5"><ItemIcon size={16} className="text-muted-foreground" /><div><p className="text-[10px] text-muted-foreground">{item.label}</p><p className="mt-0.5 text-xs font-medium">{item.value}</p></div></div> })}</div></Panel>

        <Panel title="Domains" subtitle="Public routes and certificate state"><div className="divide-y">{[{ domain: "api.taxio.ng", type: "Custom domain", state: "DNS verified · TLS active" }, { domain: "api.hostforge.dev", type: "Platform URL", state: "TLS active" }].map((domain) => <div key={domain.domain} className="flex items-center gap-3 px-5 py-4"><span className="grid size-8 place-items-center rounded-md border bg-muted"><GlobeIcon size={15} /></span><div><p className="text-xs font-medium">{domain.domain}</p><p className="mt-0.5 text-[10px] text-muted-foreground">{domain.type}</p></div><span className="ml-auto text-[10px] font-medium text-emerald-700">{domain.state}</span></div>)}</div></Panel>

        <Panel title="Recent activity" subtitle="Configuration and runtime events"><div className="divide-y">{[{ icon: CloudArrowUpIcon, title: "Deployment completed", detail: "dep_7H3KD9 is healthy", time: "24m" }, { icon: BracketsCurlyIcon, title: "Environment updated", detail: "SENTRY_DSN changed", time: "2h" }, { icon: GlobeIcon, title: "Domain verified", detail: "api.taxio.ng is active", time: "1d" }, { icon: ActivityIcon, title: "Service restarted", detail: "Manual restart by Mr Fury", time: "2d" }].map((event) => { const EventIcon = event.icon; return <div key={event.title} className="flex items-center gap-3 px-5 py-3.5"><span className="grid size-8 place-items-center rounded-md border bg-muted"><EventIcon size={14} /></span><div><p className="text-xs font-medium">{event.title}</p><p className="mt-0.5 text-[10px] text-muted-foreground">{event.detail}</p></div><span className="ml-auto text-[10px] text-muted-foreground">{event.time}</span></div> })}</div></Panel>
      </div>
    </main>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label>
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return <div className="flex items-start gap-4 border-b px-4 py-3.5 last:border-b-0"><span className="w-28 shrink-0 text-[11px] text-muted-foreground">{label}</span><span className="text-xs font-medium">{value}</span></div>
}
