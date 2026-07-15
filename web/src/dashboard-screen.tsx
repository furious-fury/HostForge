import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  AppWindowIcon,
  ArrowClockwiseIcon,
  CaretRightIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  CubeIcon,
  HardDrivesIcon,
  RocketLaunchIcon,
  StackIcon,
  WarningCircleIcon,
} from "@phosphor-icons/react"

import { api, queryKeys } from "@/api"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index > 2 ? 1 : 0)} ${units[index]}`
}

function relativeTime(value: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function Panel({ title, subtitle, children, action }: { title: string; subtitle: string; children: React.ReactNode; action?: React.ReactNode }) {
  return <section className="overflow-hidden rounded-xl border bg-card"><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold">{title}</h2><p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p></div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function Loading() {
  return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-10 w-56 rounded bg-muted" /><div className="mt-7 grid grid-cols-2 gap-4 xl:grid-cols-5">{Array.from({ length: 5 }, (_, index) => <div key={index} className="h-28 rounded-xl border bg-card" />)}</div><div className="mt-5 h-80 rounded-xl border bg-card" /></main>
}

export function DashboardScreen() {
  const applicationsQuery = useQuery({ queryKey: queryKeys.applications, queryFn: ({ signal }) => api.applications(signal) })
  const deploymentsQuery = useQuery({ queryKey: queryKeys.deployments(), queryFn: ({ signal }) => api.deployments({}, signal) })
  const hostQuery = useQuery({ queryKey: queryKeys.hostSnapshot, queryFn: ({ signal }) => api.hostSnapshot(signal), refetchInterval: 15_000 })
  const statusQuery = useQuery({ queryKey: queryKeys.systemStatus, queryFn: ({ signal }) => api.systemStatus(signal), refetchInterval: 30_000 })
  const onboardingQuery = useQuery({ queryKey: queryKeys.onboarding, queryFn: ({ signal }) => api.onboarding(signal) })
  const pending = applicationsQuery.isPending || deploymentsQuery.isPending || statusQuery.isPending
  if (pending) return <Loading />
  const failedQuery = [applicationsQuery, deploymentsQuery, statusQuery].find((query) => query.isError)
  if (failedQuery) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><WarningCircleIcon className="mx-auto text-destructive" size={24} /><h1 className="mt-3 text-sm font-semibold">Overview data could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">HostForge will not substitute fixture data when the server is unavailable.</p><Button className="mt-4" variant="outline" onClick={() => { applicationsQuery.refetch(); deploymentsQuery.refetch(); statusQuery.refetch() }}>Retry</Button></section></main>

  const applications = applicationsQuery.data?.applications ?? []
  const deployments = deploymentsQuery.data?.deployments ?? []
  const serviceCount = applications.reduce((sum, application) => sum + (application.service_count || 0), 0)
  const recent = [...deployments].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)).slice(0, 6)
  const successful = deployments.filter((deployment) => deployment.status === "SUCCESS").length
  const failed = deployments.filter((deployment) => deployment.status === "FAILED").length
  const active = deployments.filter((deployment) => deployment.status === "QUEUED" || deployment.status === "BUILDING").length
  const statusChecks = statusQuery.data?.checks ?? []
  const allHealthy = statusChecks.every((check) => ["RUNNING", "READY"].includes(check.status))
  const sample = hostQuery.data?.sample
  const disk = sample?.disks.find((item) => item.mount === "/") || sample?.disks[0]
  const netRate = sample?.net.reduce((sum, item) => sum + item.rx_bps + item.tx_bps, 0) || 0
  const setup = onboardingQuery.data?.onboarding
  const setupSteps = setup ? [true, setup.github_app_complete, setup.permanent_ingress_complete, setup.bootstrap_complete].filter(Boolean).length : 0

  const metrics = [
    { label: "Applications", value: applications.length, detail: applications.length ? "Managed products" : "Create your first", icon: AppWindowIcon },
    { label: "Services", value: serviceCount, detail: serviceCount ? "Deployable components" : "None configured", icon: StackIcon },
    { label: "Deployments", value: deployments.length, detail: `${successful} successful`, icon: RocketLaunchIcon },
    { label: "Active", value: active, detail: active ? "Queued or building" : "Queue is clear", icon: CloudArrowUpIcon },
    { label: "Failed", value: failed, detail: failed ? "Needs review" : "No failures", icon: WarningCircleIcon },
  ]

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><p className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground"><span className={`size-1.5 rounded-full ${allHealthy ? "bg-emerald-500" : "bg-amber-500"}`} />{allHealthy ? "All platform checks responding" : "A platform check needs attention"}</p><h1 className="text-3xl font-semibold tracking-[-0.035em]">Overview</h1><p className="mt-2 text-sm text-muted-foreground">Monitor applications, deployments, and host resources.</p></div><div className="flex gap-2 sm:ml-auto"><Button variant="outline" onClick={() => { hostQuery.refetch(); statusQuery.refetch(); deploymentsQuery.refetch() }}><ArrowClockwiseIcon />Refresh</Button><Button asChild><Link to="/applications/new"><AppWindowIcon />Create application</Link></Button></div></div>
    <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card sm:grid-cols-3 xl:grid-cols-5">{metrics.map((metric) => { const Icon = metric.icon; return <article key={metric.label} className="border-b border-r p-5 last:border-r-0 xl:border-b-0"><div className="flex items-center justify-between"><span className="text-xs font-medium text-muted-foreground">{metric.label}</span><Icon size={16} className="text-muted-foreground" /></div><p className="mt-5 text-3xl font-semibold tabular-nums">{metric.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{metric.detail}</p></article> })}</section>
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,.8fr)]">
      <Panel title="Host resources" subtitle={hostQuery.data?.supported ? "Live utilization from the host sampler" : "Host sampling is unavailable"} action={<span className="text-[11px] text-muted-foreground">{sample ? relativeTime(sample.at) : "Unsupported"}</span>}>
        {sample ? <div className="grid gap-6 p-5 sm:grid-cols-2">{[
          { label: "CPU", value: `${sample.cpu_pct.toFixed(1)}%`, progress: sample.cpu_pct, detail: `${sample.per_core_pct?.length || 0} cores` },
          { label: "Memory", value: `${sample.mem.used_pct.toFixed(1)}%`, progress: sample.mem.used_pct, detail: `${formatBytes(sample.mem.used_bytes)} / ${formatBytes(sample.mem.total_bytes)}` },
          { label: "Root disk", value: disk ? `${disk.used_pct.toFixed(1)}%` : "Unavailable", progress: disk?.used_pct || 0, detail: disk ? `${formatBytes(disk.used_bytes)} / ${formatBytes(disk.total_bytes)}` : "No disk sample" },
          { label: "Network", value: `${formatBytes(netRate)}/s`, progress: Math.min(100, netRate / 1_000_000), detail: sample.rates_ready ? "Combined receive and transmit" : "Rate sample warming up" },
        ].map((resource) => <div key={resource.label}><div className="mb-3 flex items-end justify-between"><div><p className="text-xs font-medium text-muted-foreground">{resource.label}</p><p className="mt-1 text-xl font-semibold tabular-nums">{resource.value}</p></div><span className="text-[10px] text-muted-foreground">{resource.detail}</span></div><div className="h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-accent" style={{ width: `${Math.max(0, Math.min(100, resource.progress))}%` }} /></div></div>)}</div> : <div className="p-8 text-center"><HardDrivesIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No host sample available</p><p className="mt-1 text-xs text-muted-foreground">{hostQuery.data?.error_code || "The sampler has not returned data yet."}</p></div>}
      </Panel>
      <Panel title="System health" subtitle="Read-only platform dependency checks" action={<Link to="/status" className="text-[11px] font-semibold hover:underline">Details</Link>}><div className="divide-y">{statusChecks.map((check) => { const healthy = ["RUNNING", "READY"].includes(check.status); return <div key={check.id} className="flex items-center gap-3 px-5 py-4"><span className="grid size-8 place-items-center rounded-lg border bg-muted"><ActivityIcon size={15} /></span><div className="min-w-0"><p className="text-xs font-semibold">{check.label}</p><p className="truncate text-[10px] text-muted-foreground">{check.detail || "Check completed successfully"}</p></div><StatusBadge className="ml-auto" tone={healthy ? "success" : check.status === "SKIPPED" ? "neutral" : "warning"} dot>{check.status}</StatusBadge></div> })}</div></Panel>
    </div>
    <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.6fr)_minmax(320px,.8fr)]">
      <Panel title="Recent deployments" subtitle="Latest service release activity" action={<Link to="/deployments" className="text-[11px] font-semibold hover:underline">View all</Link>}>{recent.length ? <div className="overflow-x-auto"><Table><TableHeader><TableRow><TableHead>Service</TableHead><TableHead>Commit</TableHead><TableHead>Status</TableHead><TableHead>Started</TableHead></TableRow></TableHeader><TableBody>{recent.map((deployment) => <TableRow key={deployment.id}><TableCell><Link className="font-semibold hover:underline" to={`/deployments/${deployment.id}`}>{deployment.service_name || deployment.service_id.slice(0, 8)}</Link><p className="text-[10px] text-muted-foreground">{deployment.application_name || "Unknown application"}</p></TableCell><TableCell className="font-mono text-[10px]">{deployment.commit_hash?.slice(0, 8) || "Pending"}</TableCell><TableCell><StatusBadge tone={deployment.status === "SUCCESS" ? "success" : deployment.status === "FAILED" ? "error" : "warning"} dot>{deployment.status}</StatusBadge></TableCell><TableCell className="text-xs text-muted-foreground">{relativeTime(deployment.created_at)}</TableCell></TableRow>)}</TableBody></Table></div> : <div className="p-10 text-center"><CubeIcon className="mx-auto text-muted-foreground" size={22} /><p className="mt-3 text-sm font-semibold">No deployments yet</p><p className="mt-1 text-xs text-muted-foreground">Deploy a configured service to see release history here.</p></div>}</Panel>
      <Panel title="Setup progress" subtitle="Permanent control-plane readiness">{setup ? <div className="p-5"><div className="flex items-end justify-between"><p className="text-2xl font-semibold">{setupSteps} of 4</p><span className="text-[11px] text-muted-foreground">{setup.bootstrap_complete ? "Complete" : "In progress"}</span></div><div className="mt-4 h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-accent" style={{ width: `${setupSteps / 4 * 100}%` }} /></div><div className="mt-5 space-y-3">{[
        ["Authenticated operator", true],
        ["GitHub App", setup.github_app_complete],
        ["Permanent ingress", setup.permanent_ingress_complete],
        ["Bootstrap disabled", setup.bootstrap_complete],
      ].map(([label, complete]) => <div key={String(label)} className="flex items-center gap-2 text-xs"><CheckCircleIcon size={15} weight="fill" className={complete ? "text-emerald-600" : "text-muted-foreground/40"} /><span>{String(label)}</span></div>)}</div>{!setup.bootstrap_complete && <Button asChild className="mt-5 w-full" variant="outline"><Link to="/onboarding">Continue setup <CaretRightIcon /></Link></Button>}</div> : <div className="p-8 text-center text-xs text-muted-foreground">Setup state unavailable.</div>}</Panel>
    </div>
  </main>
}
