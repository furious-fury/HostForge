import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  CircleNotchIcon,
  CloudArrowUpIcon,
  CodeIcon,
  CopyIcon,
  DownloadSimpleIcon,
  FunnelSimpleIcon,
  GitBranchIcon,
  GithubLogoIcon,
  MagnifyingGlassIcon,
  RocketLaunchIcon,
  TerminalWindowIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import "@/deployments.css"

const deployments = [
  { id: "dep_7H3KD9", application: "TaxIO", service: "api", commit: "13d298a", message: "Validate taxpayer region codes", status: "Healthy", trigger: "Git push", branch: "main", started: "24m ago", duration: "1m 26s" },
  { id: "dep_2QM8LA", application: "GameNation", service: "web", commit: "e143b8a", message: "Refine tournament lobby", status: "Building", trigger: "Manual", branch: "main", started: "7m ago", duration: "2m 08s" },
  { id: "dep_9BK4TR", application: "TaxIO", service: "worker", commit: "fa72c20", message: "Reduce queue concurrency", status: "Failed", trigger: "Git push", branch: "main", started: "1h ago", duration: "48s" },
  { id: "dep_4CJ1WP", application: "HostForge Docs", service: "docs", commit: "01ad983", message: "Update install guide", status: "Healthy", trigger: "Git push", branch: "main", started: "2h ago", duration: "1m 11s" },
  { id: "dep_6VN7HS", application: "TaxIO", service: "api", commit: "bc72e10", message: "Normalize assessment periods", status: "Rolled back", trigger: "Manual", branch: "main", started: "1d ago", duration: "1m 34s" },
  { id: "dep_1FX5DE", application: "GameNation", service: "api", commit: "732fe2a", message: "Add leaderboard pagination", status: "Cancelled", trigger: "Git push", branch: "develop", started: "2d ago", duration: "19s" },
]

const phases = [
  { name: "Queued", time: "10:36:02", duration: "2s", detail: "Deployment accepted by the build scheduler" },
  { name: "Source fetched", time: "10:36:04", duration: "4s", detail: "Cloned mr-fury/taxio at 13d298a" },
  { name: "Build configuration detected", time: "10:36:08", duration: "1s", detail: "Nixpacks detected Node.js 22" },
  { name: "Image built", time: "10:36:09", duration: "54s", detail: "Created image hf-taxio-api:13d298a" },
  { name: "Container created", time: "10:37:03", duration: "6s", detail: "Started hf_taxio_api_01 on port 3000" },
  { name: "Health check passed", time: "10:37:09", duration: "15s", detail: "GET /health returned 200" },
  { name: "Route activated", time: "10:37:24", duration: "4s", detail: "api.taxio.ng now targets the new container" },
]

const logs = [
  ["10:36:02.114", "system", "Deployment dep_7H3KD9 queued"],
  ["10:36:04.328", "source", "Cloning https://github.com/mr-fury/taxio.git"],
  ["10:36:08.042", "source", "Checked out commit 13d298a on branch main"],
  ["10:36:09.117", "build", "Nixpacks 1.36.0 · detecting application plan"],
  ["10:36:10.381", "build", "Node.js 22 and npm workspace detected"],
  ["10:36:15.904", "build", "Running npm ci --include=dev"],
  ["10:36:41.722", "build", "Running npm run build --workspace apps/api"],
  ["10:36:58.265", "build", "Compiled server bundle in 15.8s"],
  ["10:37:03.510", "release", "Image sha256:91fb4a2c8d1e created"],
  ["10:37:09.891", "health", "Waiting for GET http://container:3000/health"],
  ["10:37:24.104", "health", "Health check passed · status 200"],
  ["10:37:28.006", "route", "Caddy route api.taxio.ng activated"],
]

function StatusPill({ status }: { status: string }) {
  const style = status === "Healthy" ? "bg-emerald-50 text-emerald-700 ring-emerald-600/15" : status === "Building" || status === "Releasing" || status === "Queued" ? "bg-blue-50 text-blue-700 ring-blue-600/15" : status === "Failed" ? "bg-red-50 text-red-700 ring-red-600/15" : "bg-neutral-100 text-neutral-600 ring-neutral-500/15"
  return <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-semibold ring-1 ring-inset ${style}`}><span className={`size-1.5 rounded-full bg-current ${status === "Building" ? "animate-pulse" : ""}`} />{status}</span>
}

export function DeploymentsList({ scope = "global", service = "api" }: { scope?: "global" | "application" | "service"; service?: string }) {
  const [status, setStatus] = useState("All statuses")
  const [query, setQuery] = useState("")
  const scoped = deployments.filter((item) => (scope === "global" || (scope === "application" && item.application === "TaxIO") || (scope === "service" && item.service === service)) && (status === "All statuses" || item.status === status) && `${item.id} ${item.application} ${item.service} ${item.commit} ${item.message}`.toLowerCase().includes(query.toLowerCase()))
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      {scope !== "global" && <Link to={scope === "application" ? "/applications/taxio" : "/applications/taxio/services/" + service} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{scope === "application" ? "TaxIO overview" : service + " overview"}</Link>}
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Deployments</h1><p className="mt-2 text-sm text-muted-foreground">{scope === "global" ? "Track build and release activity across every application." : scope === "application" ? "Build and release activity across every TaxIO service." : "Build and release history for the TaxIO " + service + " service."}</p></div><div className="flex gap-2 sm:ml-auto"><Button variant="outline"><ArrowsClockwiseIcon />Refresh</Button><Button><RocketLaunchIcon weight="fill" />Deploy service</Button></div></div>

      <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "Total", value: scope === "global" ? "184" : "42", detail: "Last 30 days" }, { label: "Healthy", value: scope === "global" ? "169" : "39", detail: "91.8% success rate" }, { label: "In progress", value: "1", detail: "Build stage" }, { label: "Failed", value: scope === "global" ? "8" : "2", detail: "Needs review" }].map((item) => <article key={item.label} className="hf-deployment-summary"><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{item.detail}</p></article>)}</section>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/70 p-3 sm:flex-row sm:items-center"><details className="hf-filter-menu"><summary className="hf-log-toggle"><FunnelSimpleIcon size={14} />More filters</summary><div className="hf-filter-popover">{scope === "global" && <select className="hf-compact-field"><option>All applications</option><option>TaxIO</option><option>GameNation</option></select>}<select className="hf-compact-field"><option>All services</option><option>api</option><option>web</option><option>worker</option></select><select className="hf-compact-field"><option>All branches</option><option>main</option><option>develop</option></select><select className="hf-compact-field"><option>All triggers</option><option>Git push</option><option>Manual</option><option>Rollback</option></select><select className="hf-compact-field"><option>Last 30 days</option><option>Last 7 days</option><option>Today</option></select></div></details><select value={status} onChange={(event) => setStatus(event.target.value)} className="hf-compact-field sm:w-36"><option>All statuses</option><option>Queued</option><option>Building</option><option>Releasing</option><option>Healthy</option><option>Failed</option><option>Cancelled</option><option>Rolled back</option></select><label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="hf-compact-field hf-deployment-search w-full" placeholder="Search deployments" /></label></header>
        <div className="overflow-x-auto"><table className="w-full min-w-[1050px] text-left"><thead className="border-b text-[10px] uppercase tracking-[0.12em] text-muted-foreground"><tr><th className="px-5 py-3 font-semibold">Deployment</th>{scope === "global" && <th className="px-4 py-3 font-semibold">Application</th>}<th className="px-4 py-3 font-semibold">Service</th><th className="px-4 py-3 font-semibold">Commit</th><th className="px-4 py-3 font-semibold">Status</th><th className="px-4 py-3 font-semibold">Trigger</th><th className="px-4 py-3 font-semibold">Started</th><th className="px-5 py-3 text-right font-semibold">Duration</th></tr></thead><tbody className="divide-y">{scoped.map((deployment) => <tr key={deployment.id} className="group hover:bg-muted/35"><td className="px-5 py-4"><Link to={`/applications/taxio/services/${deployment.service}/deployments/${deployment.id}`} className="font-mono text-xs font-semibold group-hover:underline">{deployment.id}</Link></td>{scope === "global" && <td className="px-4 py-4 text-xs font-medium">{deployment.application}</td>}<td className="px-4 py-4"><span className="flex items-center gap-2 text-xs"><span className="grid size-7 place-items-center rounded-md border bg-muted"><CloudArrowUpIcon size={13} /></span>{deployment.service}</span></td><td className="px-4 py-4"><p className="font-mono text-[11px]">{deployment.commit}</p><p className="mt-0.5 max-w-48 truncate text-[10px] text-muted-foreground">{deployment.message}</p></td><td className="px-4 py-4"><StatusPill status={deployment.status} /></td><td className="px-4 py-4 text-xs text-muted-foreground">{deployment.trigger}</td><td className="px-4 py-4 text-xs text-muted-foreground">{deployment.started}</td><td className="px-5 py-4 text-right text-xs text-muted-foreground">{deployment.duration}</td></tr>)}</tbody></table></div>
        <footer className="flex justify-between border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground"><span>Showing {scoped.length} deployments</span><span>Updated a few seconds ago</span></footer>
      </section>
    </main>
  )
}

export function DeploymentDetail({ service = "api", deploymentID = "dep_7H3KD9" }: { service?: string; deploymentID?: string }) {
  const [follow, setFollow] = useState(true)
  const [wrap, setWrap] = useState(false)
  const [logQuery, setLogQuery] = useState("")
  const visibleLogs = logs.filter((line) => line.join(" ").toLowerCase().includes(logQuery.toLowerCase()))
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to={`/applications/taxio/services/${service}/deployments`} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{service} deployments</Link>
      <div className="mb-7 flex flex-col gap-4 xl:flex-row xl:items-end"><div><p className="mb-2 flex items-center gap-2"><StatusPill status="Healthy" /><span className="text-xs text-muted-foreground">Production</span></p><h1 className="font-mono text-3xl font-semibold tracking-[-0.04em]">{deploymentID}</h1><p className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"><span>TaxIO / {service}</span><span className="flex items-center gap-1"><GitBranchIcon size={13} />main</span><span>Git push</span><span>Today at 10:36</span><span>1m 26s</span></p></div><div className="flex flex-wrap gap-2 xl:ml-auto"><Button variant="outline"><GithubLogoIcon />Open commit</Button><Button variant="outline"><DownloadSimpleIcon />Download logs</Button><Button variant="outline"><ArrowsClockwiseIcon />Redeploy</Button><Button><ArrowLeftIcon />Roll back to this version</Button></div></div>

      <div className="mb-5 grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.65fr)]">
        <Panel title="Deployment timeline" subtitle="Seven stages completed successfully" action={<span className="text-xs font-medium text-emerald-700">Completed in 1m 26s</span>}>
          <div className="p-5">{phases.map((phase, index) => <div key={phase.name} className="relative flex gap-4 pb-6 last:pb-0"><div className="relative z-10 grid size-7 shrink-0 place-items-center rounded-full bg-emerald-50 text-emerald-700 ring-1 ring-emerald-600/15"><CheckCircleIcon size={15} weight="fill" /></div>{index < phases.length - 1 && <span className="absolute left-[13px] top-7 h-[calc(100%-1.75rem)] w-px bg-emerald-200" />}<div className="min-w-0 flex-1 pt-0.5"><div className="flex flex-wrap items-center gap-2"><p className="text-xs font-semibold">{phase.name}</p><span className="ml-auto font-mono text-[10px] text-muted-foreground">{phase.time} · {phase.duration}</span></div><p className="mt-1 text-[11px] text-muted-foreground">{phase.detail}</p></div></div>)}</div>
        </Panel>

        <Panel title="Deployment information" subtitle="Source, image, and runtime identifiers">
          <div className="divide-y">{[{ label: "Commit", value: "13d298a · Validate taxpayer region codes", icon: CodeIcon }, { label: "Repository", value: "mr-fury/taxio", icon: GithubLogoIcon }, { label: "Branch", value: "main", icon: GitBranchIcon }, { label: "Triggered by", value: "Mr Fury · Git push", icon: CloudArrowUpIcon }, { label: "Build method", value: "Nixpacks · Node.js 22", icon: TerminalWindowIcon }, { label: "Image", value: "sha256:91fb4a2c8d1e", icon: CodeIcon }, { label: "Container", value: "hf_taxio_api_01", icon: CloudArrowUpIcon }, { label: "Public URL", value: "api.taxio.ng", icon: ArrowSquareOutIcon }].map((item) => { const ItemIcon = item.icon; return <div key={item.label} className="flex items-start gap-3 px-5 py-3.5"><ItemIcon size={15} className="mt-0.5 shrink-0 text-muted-foreground" /><div className="min-w-0"><p className="text-[10px] text-muted-foreground">{item.label}</p><p className="mt-0.5 truncate font-mono text-[11px] font-medium">{item.value}</p></div><CopyIcon size={13} className="ml-auto shrink-0 text-muted-foreground" /></div> })}</div>
        </Panel>
      </div>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/75 p-4 xl:flex-row xl:items-center"><div className="flex items-center gap-2"><TerminalWindowIcon size={16} /><div><h2 className="text-sm font-semibold">Deployment logs</h2><p className="mt-0.5 text-[11px] text-muted-foreground">Build and release output</p></div></div><div className="flex flex-wrap gap-2 xl:ml-auto"><select className="hf-compact-field"><option>All stages</option><option>Build</option><option>Release</option><option>Health check</option></select><label className="relative"><MagnifyingGlassIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" size={13} /><input value={logQuery} onChange={(event) => setLogQuery(event.target.value)} className="hf-compact-field w-48 pl-8" placeholder="Search logs" /></label><button onClick={() => setFollow(!follow)} className={`hf-log-toggle ${follow ? "hf-log-toggle-active" : ""}`}><CircleNotchIcon size={13} className={follow ? "animate-spin" : ""} />Follow</button><button onClick={() => setWrap(!wrap)} className={`hf-log-toggle ${wrap ? "hf-log-toggle-active" : ""}`}>Wrap</button><Button variant="outline" size="sm">Jump to error</Button><Button variant="outline" size="icon" aria-label="Copy log output"><CopyIcon /></Button></div></header>
        <div className={`hf-terminal ${wrap ? "whitespace-pre-wrap" : "whitespace-pre"}`} role="log" aria-label="Deployment log output">{visibleLogs.map(([time, stage, message], index) => <div key={`${time}-${index}`} className="hf-log-line"><span className="hf-log-time">{time}</span><span className={`hf-log-stage hf-log-stage-${stage}`}>{stage.padEnd(7)}</span><span className="hf-log-message">{message}</span></div>)}<div className="hf-log-line"><span className="hf-log-time">10:37:28.019</span><span className="hf-log-stage hf-log-stage-system">system </span><span className="text-emerald-400">Deployment completed successfully</span></div></div>
        <footer className="flex flex-wrap items-center gap-4 border-t bg-muted/30 px-4 py-2.5 text-[10px] text-muted-foreground"><span className="flex items-center gap-1.5"><span className="size-1.5 rounded-full bg-emerald-500" />Stream connected</span><span>{visibleLogs.length + 1} lines</span><span className="ml-auto">UTF-8 · timestamps enabled</span></footer>
      </section>
    </main>
  )
}

function Panel({ title, subtitle, action, children }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode }) {
  return <section className="overflow-hidden rounded-xl border bg-card"><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}
