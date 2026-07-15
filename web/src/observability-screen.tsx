import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { ArrowsClockwiseIcon, CpuIcon, HardDrivesIcon, MagnifyingGlassIcon, MemoryIcon, PulseIcon, WarningCircleIcon, WifiHighIcon } from "@phosphor-icons/react"

import { api, queryKeys, type DeploymentStepDTO, type HostSampleDTO, type HTTPRequestDTO, type PlatformEventDTO } from "@/api"
import { AppSelect } from "@/components/app-select"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import "@/observability.css"

type Tab = "Overview" | "Requests" | "Deploy steps" | "Host" | "Events"

export function ObservabilityScreen() {
  const [tab, setTab] = useState<Tab>("Overview")
  const summary = useQuery({ queryKey: queryKeys.observabilitySummary, queryFn: ({ signal }) => api.observabilitySummary(signal), refetchInterval: 15000 })
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Observability</h1><p className="mt-2 text-sm text-muted-foreground">Persisted request, deployment, event, and host telemetry.</p></div><Button className="sm:ml-auto" variant="outline" onClick={() => summary.refetch()}><ArrowsClockwiseIcon />Refresh</Button></div>
    <Tabs value={tab} onValueChange={(value) => setTab(value as Tab)} className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><TabsList className="h-auto min-w-max justify-start bg-transparent p-0">{(["Overview", "Requests", "Deploy steps", "Host", "Events"] as Tab[]).map((item) => <TabsTrigger key={item} value={item} className="px-3.5 py-2 text-xs data-[state=active]:bg-accent data-[state=active]:text-accent-foreground">{item}</TabsTrigger>)}</TabsList></Tabs>
    {tab === "Overview" && <Overview summary={summary} onOpen={setTab} />}
    {tab === "Requests" && <Requests />}
    {tab === "Deploy steps" && <DeploySteps />}
    {tab === "Host" && <Host />}
    {tab === "Events" && <Events />}
  </main>
}

function Overview({ summary, onOpen }: { summary: ReturnType<typeof useQuery<Awaited<ReturnType<typeof api.observabilitySummary>>>>; onOpen: (tab: Tab) => void }) {
  if (summary.isPending) return <Loading />
  if (summary.isError) return <ErrorState title="Observability summary could not be loaded" retry={() => summary.refetch()} />
  const item = summary.data.summary
  const cards = [
    { label: "Requests", value: item.http_request_count, detail: item.window_hours + " hour window" },
    { label: "HTTP errors", value: item.http_error_count, detail: "Persisted 4xx/5xx records" },
    { label: "Request p50", value: item.http_duration_p50_ms + " ms", detail: "p95 " + item.http_duration_p95_ms + " ms" },
    { label: "Deployments", value: item.deploy_count, detail: item.deploy_failed_count + " failed" },
    { label: "Deploy p50", value: item.deploy_duration_p50_ms + " ms", detail: "p95 " + item.deploy_duration_p95_ms + " ms" },
  ]
  return <><section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card md:grid-cols-5">{cards.map((card) => <article key={card.label} className="hf-observe-summary"><p className="text-[11px] text-muted-foreground">{card.label}</p><p className="mt-4 text-xl font-semibold">{card.value}</p><p className="mt-1 text-[10px] text-muted-foreground">{card.detail}</p></article>)}</section><div className="grid gap-5 md:grid-cols-2"><Panel title="Request records" subtitle="Inspect persisted management and ingress requests"><button onClick={() => onOpen("Requests")} className="p-6 text-left text-sm font-semibold hover:bg-muted/30">Open request records</button></Panel><Panel title="Deployment stages" subtitle="Inspect timing and failure codes"><button onClick={() => onOpen("Deploy steps")} className="p-6 text-left text-sm font-semibold hover:bg-muted/30">Open deployment steps</button></Panel><Panel title="Host samples" subtitle="Available only on supported hosts"><button onClick={() => onOpen("Host")} className="p-6 text-left text-sm font-semibold hover:bg-muted/30">Open host telemetry</button></Panel><Panel title="Platform events" subtitle="Durable operator and lifecycle activity"><button onClick={() => onOpen("Events")} className="p-6 text-left text-sm font-semibold hover:bg-muted/30">Open event stream</button></Panel></div></>
}

function Requests() {
  const [query, setQuery] = useState("")
  const [method, setMethod] = useState("All methods")
  const [status, setStatus] = useState("All statuses")
  const [dateFrom, setDateFrom] = useState("")
  const [dateTo, setDateTo] = useState("")
  const [cursor, setCursor] = useState("")
  const [history, setHistory] = useState<string[]>([])
  const filters = {
    method: method === "All methods" ? undefined : method,
    statusClass: status === "Success" ? "success" as const : status === "Client error" ? "client_error" as const : status === "Server error" ? "server_error" as const : undefined,
    dateFrom: dateFrom || undefined,
    dateTo: dateTo ? dateTo + "T23:59:59Z" : undefined,
    cursor: cursor || undefined,
    limit: 50,
  }
  const result = useQuery({ queryKey: queryKeys.observabilityRequests(filters), queryFn: ({ signal }) => api.observabilityRequests(filters, signal), refetchInterval: 10000 })
  if (result.isPending) return <Loading />
  if (result.isError) return <ErrorState title="Request records could not be loaded" retry={() => result.refetch()} />
  const rows = result.data.requests.filter((row) => [row.method, row.path, row.request_id, row.status, row.application_id, row.service_id, row.environment_id].join(" ").toLowerCase().includes(query.toLowerCase()))
  const resetPage = () => { setCursor(""); setHistory([]) }
  return <Panel title="Request records" subtitle="Persisted server request spans">
    <div className="grid gap-3 border-b bg-muted/30 p-4 md:grid-cols-2 xl:grid-cols-[9rem_10rem_10rem_10rem_minmax(16rem,1fr)]">
      <AppSelect options={["All methods", "GET", "POST", "PATCH", "DELETE"]} value={method} onValueChange={(value) => { setMethod(value); resetPage() }} />
      <AppSelect options={["All statuses", "Success", "Client error", "Server error"]} value={status} onValueChange={(value) => { setStatus(value); resetPage() }} />
      <Input aria-label="Requests from date" type="date" value={dateFrom} onChange={(event) => { setDateFrom(event.target.value); resetPage() }} className="bg-card text-xs" />
      <Input aria-label="Requests to date" type="date" value={dateTo} onChange={(event) => { setDateTo(event.target.value); resetPage() }} className="bg-card text-xs" />
      <Search value={query} onChange={setQuery} border={false} />
    </div>
    <RequestTable rows={rows} />
    <PaginationFooter count={rows.length} noun="requests" cursor={cursor} history={history} nextCursor={result.data.next_cursor} fetching={result.isFetching} setCursor={setCursor} setHistory={setHistory} />
  </Panel>
}

function RequestTable({ rows }: { rows: HTTPRequestDTO[] }) {
  return rows.length ? <div className="overflow-x-auto"><Table className="min-w-[920px]"><TableHeader><TableRow><TableHead>Started</TableHead><TableHead>Request ID</TableHead><TableHead>Scope</TableHead><TableHead>Method</TableHead><TableHead>Path</TableHead><TableHead>Status</TableHead><TableHead>Duration</TableHead></TableRow></TableHeader><TableBody>{rows.map((row) => <TableRow key={row.id}><TableCell className="font-mono text-[10px] text-muted-foreground">{new Date(row.started_at).toLocaleString()}</TableCell><TableCell className="font-mono text-[10px]">{row.request_id}</TableCell><TableCell><p className="font-mono text-[10px]">{row.service_id || row.application_id || "platform"}</p>{row.environment_id && <p className="font-mono text-[9px] text-muted-foreground">{row.environment_id}</p>}</TableCell><TableCell><Badge variant="secondary">{row.method}</Badge></TableCell><TableCell className="max-w-96 truncate font-mono text-xs">{row.path}</TableCell><TableCell><StatusBadge tone={row.status >= 500 ? "error" : row.status >= 400 ? "warning" : "success"}>{row.status}</StatusBadge></TableCell><TableCell className="text-xs text-muted-foreground">{row.duration_ms} ms</TableCell></TableRow>)}</TableBody></Table></div> : <Empty text="No request records match the current search." />
}

function DeploySteps() {
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState("All statuses")
  const [dateFrom, setDateFrom] = useState("")
  const [dateTo, setDateTo] = useState("")
  const [cursor, setCursor] = useState("")
  const [history, setHistory] = useState<string[]>([])
  const filters = {
    status: status === "All statuses" ? undefined : status.toLowerCase(),
    dateFrom: dateFrom || undefined,
    dateTo: dateTo ? dateTo + "T23:59:59Z" : undefined,
    cursor: cursor || undefined,
    limit: 50,
  }
  const result = useQuery({ queryKey: queryKeys.observabilityDeploySteps(filters), queryFn: ({ signal }) => api.observabilityDeploySteps(filters, signal), refetchInterval: 10000 })
  if (result.isPending) return <Loading />
  if (result.isError) return <ErrorState title="Deployment steps could not be loaded" retry={() => result.refetch()} />
  const rows = result.data.deploy_steps.filter((row) => [row.deployment_id, row.service_name, row.environment_name, row.step, row.status, row.error_code].join(" ").toLowerCase().includes(query.toLowerCase()))
  const resetPage = () => { setCursor(""); setHistory([]) }
  return <Panel title="Deployment-step records" subtitle="Measured build and release stages">
    <div className="grid gap-3 border-b bg-muted/30 p-4 md:grid-cols-2 xl:grid-cols-[10rem_10rem_10rem_minmax(16rem,1fr)]">
      <AppSelect options={["All statuses", "Ok", "Error"]} value={status} onValueChange={(value) => { setStatus(value); resetPage() }} />
      <Input aria-label="Deployment steps from date" type="date" value={dateFrom} onChange={(event) => { setDateFrom(event.target.value); resetPage() }} className="bg-card text-xs" />
      <Input aria-label="Deployment steps to date" type="date" value={dateTo} onChange={(event) => { setDateTo(event.target.value); resetPage() }} className="bg-card text-xs" />
      <Search value={query} onChange={setQuery} border={false} />
    </div>
    {rows.length ? <div className="overflow-x-auto"><Table className="min-w-[880px]"><TableHeader><TableRow><TableHead>Ended</TableHead><TableHead>Service / environment</TableHead><TableHead>Deployment</TableHead><TableHead>Step</TableHead><TableHead>Status</TableHead><TableHead>Error code</TableHead><TableHead>Duration</TableHead></TableRow></TableHeader><TableBody>{rows.map((row) => <StepRow key={row.id} row={row} />)}</TableBody></Table></div> : <Empty text="No deployment steps match the current search." />}
    <PaginationFooter count={rows.length} noun="steps" cursor={cursor} history={history} nextCursor={result.data.next_cursor} fetching={result.isFetching} setCursor={setCursor} setHistory={setHistory} />
  </Panel>
}

function StepRow({ row }: { row: DeploymentStepDTO }) {
  return <TableRow><TableCell className="font-mono text-[10px] text-muted-foreground">{new Date(row.ended_at).toLocaleString()}</TableCell><TableCell><p className="text-xs font-semibold">{row.service_name || row.service_id}</p><p className="text-[10px] text-muted-foreground">{row.environment_name || row.environment_id}</p></TableCell><TableCell className="font-mono text-xs">{row.deployment_id}</TableCell><TableCell className="text-xs">{row.step}</TableCell><TableCell><StatusBadge tone={row.status === "ok" ? "success" : "error"}>{row.status}</StatusBadge></TableCell><TableCell className="font-mono text-xs text-muted-foreground">{row.error_code || "-"}</TableCell><TableCell className="text-xs">{row.duration_ms} ms</TableCell></TableRow>
}

function Host() {
  const snapshot = useQuery({ queryKey: queryKeys.hostSnapshot, queryFn: ({ signal }) => api.hostSnapshot(signal), refetchInterval: 5000, retry: false })
  const history = useQuery({ queryKey: queryKeys.hostHistory(120), queryFn: ({ signal }) => api.hostHistory(120, signal), refetchInterval: 15000, retry: false })
  if (snapshot.isPending || history.isPending) return <Loading />
  if (snapshot.isError || history.isError) return <ErrorState title="Host metrics are warming up or unavailable" retry={() => { snapshot.refetch(); history.refetch() }} />
  if (!snapshot.data.supported || !history.data.supported || !snapshot.data.sample) return <Empty text={"Host metrics are not supported: " + (snapshot.data.error_code || history.data.error_code || "unknown reason")} />
  const sample = snapshot.data.sample
  return <div className="grid gap-5 md:grid-cols-2"><Metric title="CPU" value={sample.cpu_pct} suffix="%" icon={CpuIcon} samples={history.data.samples.map((item) => item.cpu_pct)} /><Metric title="Memory" value={sample.mem.used_pct} suffix="%" icon={MemoryIcon} samples={history.data.samples.map((item) => item.mem.used_pct)} /><Metric title="Root disk" value={sample.disks[0]?.used_pct || 0} suffix="%" icon={HardDrivesIcon} samples={history.data.samples.map((item) => item.disks[0]?.used_pct || 0)} /><Metric title="Network receive" value={sample.net.reduce((sum, item) => sum + item.rx_bps, 0) / 1024 / 1024} suffix=" MB/s" icon={WifiHighIcon} samples={history.data.samples.map(netRx)} /></div>
}

function netRx(sample: HostSampleDTO) { return sample.net.reduce((sum, item) => sum + item.rx_bps, 0) / 1024 / 1024 }

function Metric({ title, value, suffix, icon: Icon, samples }: { title: string; value: number; suffix: string; icon: React.ComponentType<{ size?: number; className?: string }>; samples: number[] }) {
  const max = Math.max(...samples, 1)
  return <Panel title={title} subtitle="Sampled host telemetry"><div className="p-5"><div className="mb-5 flex items-center gap-2"><Icon size={18} className="text-muted-foreground" /><span className="text-2xl font-semibold">{value.toFixed(1)}{suffix}</span></div>{samples.length ? <div className="flex h-36 items-end gap-1 border-b">{samples.map((point, index) => <div key={index} className="flex-1 rounded-t-sm bg-accent/80" style={{ height: Math.max(3, point / max * 100) + "%" }} />)}</div> : <Empty text="No history samples yet." />}</div></Panel>
}

function Events() {
  const [query, setQuery] = useState("")
  const [type, setType] = useState("All event types")
  const [dateFrom, setDateFrom] = useState("")
  const [dateTo, setDateTo] = useState("")
  const [cursor, setCursor] = useState("")
  const [history, setHistory] = useState<string[]>([])
  const result = useQuery({ queryKey: [...queryKeys.events("", "", type === "All event types" ? "" : type), dateFrom, dateTo, cursor], queryFn: ({ signal }) => api.events({ type: type === "All event types" ? undefined : type, dateFrom: dateFrom || undefined, dateTo: dateTo ? dateTo + "T23:59:59Z" : undefined, cursor: cursor || undefined, limit: 50 }, signal), refetchInterval: 10000 })
  if (result.isPending) return <Loading />
  if (result.isError) return <ErrorState title="Platform events could not be loaded" retry={() => result.refetch()} />
  const rows = result.data.events.filter((row) => [row.event_type, row.message, row.detail, row.service_id, row.application_id].join(" ").toLowerCase().includes(query.toLowerCase()))
  const resetPage = () => { setCursor(""); setHistory([]) }
  return <Panel title="Platform events" subtitle="Durable lifecycle and operator activity"><div className="grid gap-3 border-b bg-muted/30 p-4 lg:grid-cols-[12rem_10rem_10rem_minmax(14rem,1fr)]"><AppSelect options={["All event types", "deployment", "domain", "configuration", "runtime", "application", "service"]} value={type} onValueChange={(value) => { setType(value); resetPage() }} /><Input aria-label="Events from date" type="date" value={dateFrom} onChange={(event) => { setDateFrom(event.target.value); resetPage() }} className="bg-card text-xs" /><Input aria-label="Events to date" type="date" value={dateTo} onChange={(event) => { setDateTo(event.target.value); resetPage() }} className="bg-card text-xs" /><Search value={query} onChange={setQuery} border={false} /></div>{rows.length ? <div className="divide-y">{rows.map((row) => <EventRow key={row.id} row={row} />)}</div> : <Empty text="No platform events match the current filters." />}<footer className="flex items-center gap-2 border-t bg-muted/30 px-5 py-3"><span className="text-[11px] text-muted-foreground">{rows.length} events on this page</span><Button className="ml-auto" size="sm" variant="outline" disabled={!history.length || result.isFetching} onClick={() => { setCursor(history[history.length - 1] || ""); setHistory((items) => items.slice(0, -1)) }}>Previous</Button><Button size="sm" variant="outline" disabled={!result.data.next_cursor || result.isFetching} onClick={() => { setHistory((items) => [...items, cursor]); setCursor(result.data.next_cursor) }}>Next</Button></footer></Panel>
}

function EventRow({ row }: { row: PlatformEventDTO }) {
  return <div className="flex gap-4 px-5 py-4"><span className={`mt-1 size-2.5 shrink-0 rounded-full ${row.status === "FAILED" || row.status === "error" ? "bg-red-500" : row.status === "SUCCESS" ? "bg-emerald-500" : "bg-accent"}`} /><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="text-xs font-semibold">{row.message}</p><Badge variant="secondary">{row.event_type}</Badge></div><p className="mt-1 break-words text-[11px] text-muted-foreground">{row.detail || [row.application_id, row.service_id, row.deployment_id].filter(Boolean).join(" / ")}</p></div><span className="ml-auto shrink-0 font-mono text-[10px] text-muted-foreground">{new Date(row.created_at).toLocaleString()}</span></div>
}

function Search({ value, onChange, border = true }: { value: string; onChange: (value: string) => void; border?: boolean }) {
  return <label className={`relative block min-w-0 ${border ? "border-b bg-muted/30 p-4" : ""}`}><MagnifyingGlassIcon className={`absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground ${border ? "ml-4" : ""}`} size={14} /><Input value={value} onChange={(event) => onChange(event.target.value)} className="h-9 w-full bg-card pl-9 text-xs" placeholder="Search records" type="search" /></label>
}

function PaginationFooter({ count, noun, cursor, history, nextCursor, fetching, setCursor, setHistory }: { count: number; noun: string; cursor: string; history: string[]; nextCursor: string; fetching: boolean; setCursor: (value: string) => void; setHistory: React.Dispatch<React.SetStateAction<string[]>> }) {
  return <footer className="flex items-center gap-2 border-t bg-muted/30 px-5 py-3"><span className="text-[11px] text-muted-foreground">{count} {noun} on this page</span><Button className="ml-auto" size="sm" variant="outline" disabled={!history.length || fetching} onClick={() => { setCursor(history[history.length - 1] || ""); setHistory((items) => items.slice(0, -1)) }}>Previous</Button><Button size="sm" variant="outline" disabled={!nextCursor || fetching} onClick={() => { setHistory((items) => [...items, cursor]); setCursor(nextCursor) }}>Next</Button></footer>
}

function Loading() { return <div className="animate-pulse rounded-xl border bg-card p-6"><div className="h-72 rounded bg-muted" /></div> }
function ErrorState({ title, retry }: { title: string; retry: () => unknown }) { return <div className="rounded-xl border bg-card p-10 text-center"><WarningCircleIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">{title}</p><Button className="mt-4" variant="outline" onClick={retry}>Retry</Button></div> }
function Empty({ text }: { text: string }) { return <div className="p-10 text-center text-xs text-muted-foreground"><PulseIcon className="mx-auto mb-3" size={22} />{text}</div> }
function Panel({ title, subtitle, children }: { title: string; subtitle?: string; children: React.ReactNode }) { return <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-3"><h2 className="text-sm font-semibold">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</header>{children}</section> }
