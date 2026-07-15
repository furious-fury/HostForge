import { useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api, queryKeys } from "@/api"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  BracketsCurlyIcon,
  CpuIcon,
  DownloadSimpleIcon,
  GlobeIcon,
  HardDrivesIcon,
  KeyIcon,
  MagnifyingGlassIcon,
  MemoryIcon,
  PauseIcon,
  PencilSimpleIcon,
  PlayIcon,
  PlusIcon,
  TrashIcon,
  WifiHighIcon,
} from "@phosphor-icons/react"

import { AppSelect } from "@/components/app-select"
import { RouteTabs } from "@/components/route-tabs"
import { StatusBadge } from "@/components/status-badge"
import { ConfirmationAction } from "@/components/confirmation-action"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import "@/operations.css"
import { useToast } from "@/toast-provider"
import { useDeploymentLogStream } from "@/use-deployment-log-stream"

function PageHeader({ title, description, back, children }: { title: string; description: string; back: { label: string; href: string }; children?: React.ReactNode }) {
  return <><Link to={back.href} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{back.label}</Link><div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 text-sm text-muted-foreground">{description}</p></div>{children && <div className="flex flex-wrap gap-2 sm:ml-auto">{children}</div>}</div></>
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function ServiceTabs({ active, service, applicationID }: { active: string; service: string; applicationID: string }) {
  const tabs = ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]
  return <RouteTabs active={active} label="Service navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? "/applications/" + applicationID + "/services/" + service : "/applications/" + applicationID + "/services/" + service + "/" + tab.toLowerCase() }))} />
}

function ApplicationTabs({ active, applicationID }: { active: string; applicationID: string }) {
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <RouteTabs active={active} label="Application navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? "/applications/" + applicationID : "/applications/" + applicationID + "/" + tab.toLowerCase() }))} />
}

export function ServiceLogs({ applicationID, service }: { applicationID: string; service: string }) {
  const [environmentName, setEnvironmentName] = useState("Production")
  const [streaming, setStreaming] = useState(true)
  const [query, setQuery] = useState("")
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const serviceQuery = useQuery({ queryKey: queryKeys.service(service), queryFn: ({ signal }) => api.service(service, signal) })
  const environments = applicationQuery.data?.environments || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const binding = serviceQuery.data?.bindings.find((item) => item.environment_id === environment?.id)
  const deploymentID = binding?.active_deployment_id || ""
  const stream = useDeploymentLogStream(deploymentID, streaming && Boolean(deploymentID), "container")
  const text = stream.text
  const connection = stream.connection

  const lines = text.split(/\r?\n/).filter((line) => line && line.toLowerCase().includes(query.toLowerCase()))
  const base = "/applications/" + applicationID + "/services/" + service
  function download() {
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }))
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = service + "-" + (environment?.slug || "runtime") + ".log"
    anchor.click()
    URL.revokeObjectURL(url)
  }

  if (applicationQuery.isPending || serviceQuery.isPending) return <OperationLoading />
  if (applicationQuery.isError || serviceQuery.isError) return <OperationError title="Runtime logs could not be loaded" retry={() => { applicationQuery.refetch(); serviceQuery.refetch() }} />

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Runtime logs" description="Live stdout and stderr from the active container." back={{ label: "Service overview", href: base }}><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={(value) => { setEnvironmentName(value); stream.clear() }} className="h-9 min-w-36 bg-card text-xs" /><Button variant="outline" disabled={!text} onClick={download}><DownloadSimpleIcon />Download</Button><Button disabled={!deploymentID} onClick={() => setStreaming((current) => !current)}>{streaming ? <PauseIcon /> : <PlayIcon />}{streaming ? "Pause stream" : "Resume stream"}</Button></PageHeader><ServiceTabs active="Logs" service={service} applicationID={applicationID} />
    {!deploymentID ? <StateCard title="No active container" description="Deploy this environment before opening runtime logs." /> : <section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/75 p-4 sm:flex-row sm:items-center"><span className="flex items-center gap-2 text-xs font-semibold"><span className={`size-2 rounded-full ${connection === "connected" ? "bg-emerald-500" : connection === "error" ? "bg-red-500" : "animate-pulse bg-amber-500"}`} />{streaming ? connection : "paused"}</span><label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full bg-card pl-9 text-xs" placeholder="Search buffered logs" /></label><Button variant="outline" size="sm" onClick={stream.clear}>Clear buffer</Button></header>{stream.error && <p role="alert" className="border-b bg-destructive/5 px-4 py-2 text-xs text-destructive">{stream.error}</p>}<div className="hf-runtime-log min-h-[28rem]" role="log">{lines.length ? lines.map((line, index) => <div key={index} className="hf-runtime-log-line"><span className="col-span-full whitespace-pre-wrap break-all font-mono text-[11px] text-neutral-200">{line}</span></div>) : <p className="p-5 font-mono text-xs text-neutral-500">{connection === "connected" ? "Waiting for container output..." : connection === "error" ? "Runtime stream unavailable." : "Connecting to the runtime stream..."}</p>}</div><footer className="border-t bg-muted/30 px-4 py-2.5 text-[10px] text-muted-foreground">{lines.length} visible lines / buffer capped at 1 MB</footer></section>}
  </main>
}

export function ServiceMetrics({ applicationID, service }: { applicationID: string; service: string }) {
  const [environmentName, setEnvironmentName] = useState("Production")
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const environments = applicationQuery.data?.environments || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const metricsQuery = useQuery({ queryKey: queryKeys.serviceMetrics(service, environment?.id || "", 120), queryFn: ({ signal }) => api.serviceMetrics(service, environment.id, 120, signal), enabled: Boolean(environment), refetchInterval: 10000, retry: false })
  const base = "/applications/" + applicationID + "/services/" + service
  if (applicationQuery.isPending) return <OperationLoading />
  if (applicationQuery.isError) return <OperationError title="Service metrics could not be loaded" retry={() => applicationQuery.refetch()} />
  if (!environment) return <main className="mx-auto w-full max-w-[1600px] px-4 py-12"><StateCard title="No environments available" description="Create an environment before collecting service metrics." /></main>
  if (metricsQuery.isPending) return <OperationLoading />
  if (metricsQuery.isError) return <OperationError title="Service metrics unavailable" retry={() => metricsQuery.refetch()} />
  if (!metricsQuery.data.supported) return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Metrics" description="Container resource samples for this service." back={{ label: "Service overview", href: base }}><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} /></PageHeader><ServiceTabs active="Metrics" service={service} applicationID={applicationID} /><StateCard title="No active container" description={metricsQuery.data.error_code || "Deploy this environment before collecting service metrics."} /></main>
  const samples = metricsQuery.data.samples
  const current = metricsQuery.data.sample || samples[samples.length - 1]
  const rx = metricRates(samples, "network_rx_bytes")
  const tx = metricRates(samples, "network_tx_bytes")
  const sampleTime = current?.sampled_at ? new Date(current.sampled_at).toLocaleTimeString() : "No sample"
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><PageHeader title="Metrics" description="Persisted Docker resource samples for the active container." back={{ label: "Service overview", href: base }}><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-9 min-w-36 bg-card text-xs" /><Button variant="outline" onClick={() => metricsQuery.refetch()}><ActivityIcon />Refresh</Button></PageHeader><ServiceTabs active="Metrics" service={service} applicationID={applicationID} />
    {metricsQuery.data.stale && <div className="mb-5 rounded-xl border bg-muted/40 p-4 text-xs text-muted-foreground">The service is stopped. Showing the last persisted samples.</div>}
    <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "CPU", value: (current?.cpu_percent || 0).toFixed(1) + "%", icon: CpuIcon }, { label: "Memory", value: formatBytes(current?.memory_bytes || 0), icon: MemoryIcon }, { label: "Network ingress", value: formatRate(rx[rx.length - 1] || 0), icon: WifiHighIcon }, { label: "Network egress", value: formatRate(tx[tx.length - 1] || 0), icon: HardDrivesIcon }].map((item) => { const Icon = item.icon; return <article key={item.label} className="hf-operation-summary"><div className="flex justify-between"><p className="text-xs text-muted-foreground">{item.label}</p><Icon size={16} /></div><p className="mt-4 text-2xl font-semibold">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{sampleTime}</p></article> })}</section>
    {samples.length ? <div className="grid gap-5 lg:grid-cols-2"><MetricChart title="CPU usage" subtitle="Percentage across available cores" value={(current?.cpu_percent || 0).toFixed(1) + "%" } data={samples.map((item) => item.cpu_percent)} /><MetricChart title="Memory usage" subtitle="Container working-set megabytes" value={formatBytes(current?.memory_bytes || 0)} data={samples.map((item) => item.memory_bytes / 1024 / 1024)} /><MetricChart title="Network ingress" subtitle="Megabytes per second between samples" value={formatRate(rx[rx.length - 1] || 0)} data={rx} /><MetricChart title="Network egress" subtitle="Megabytes per second between samples" value={formatRate(tx[tx.length - 1] || 0)} data={tx} /></div> : <StateCard title="Waiting for history" description="Refresh after another collection interval to build charts." />}
  </main>
}

function metricRates(samples: Array<{ sampled_at: string; network_rx_bytes: number; network_tx_bytes: number }>, field: "network_rx_bytes" | "network_tx_bytes") {
  return samples.slice(1).map((item, index) => {
    const previous = samples[index]
    const seconds = Math.max(1, (new Date(item.sampled_at).getTime() - new Date(previous.sampled_at).getTime()) / 1000)
    return Math.max(0, item[field] - previous[field]) / seconds / 1024 / 1024
  })
}
function formatBytes(value: number) { return value >= 1024 ** 3 ? (value / 1024 ** 3).toFixed(2) + " GB" : (value / 1024 ** 2).toFixed(1) + " MB" }
function formatRate(value: number) { return value.toFixed(2) + " MB/s" }
function StateCard({ title, description, retry }: { title: string; description: string; retry?: () => unknown }) { return <div className="rounded-xl border bg-card p-10 text-center"><ActivityIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">{title}</p><p className="mt-1 text-xs text-muted-foreground">{description}</p>{retry && <Button className="mt-4" variant="outline" onClick={retry}>Retry</Button>}</div> }

function MetricChart({ title, subtitle, value, data }: { title: string; subtitle: string; value: string; data: number[] }) {
  const max = Math.max(...data)
  return <Panel title={title} subtitle={subtitle} action={<span className="text-xs font-semibold tabular-nums">{value}</span>}><div className="p-5"><div className="flex h-44 items-end gap-1 border-b border-dashed pb-1">{data.map((point, index) => <div key={index} className="group relative flex-1 rounded-t-sm bg-accent/80 transition-opacity hover:opacity-65" style={{ height: `${Math.max(5, (point / max) * 100)}%` }}><span className="pointer-events-none absolute bottom-full left-1/2 mb-1 hidden -translate-x-1/2 rounded bg-foreground px-1.5 py-1 text-[9px] text-background group-hover:block">{point}</span></div>)}</div><div className="mt-2 flex justify-between text-[9px] text-muted-foreground"><span>60m ago</span><span>30m ago</span><span>Now</span></div></div></Panel>
}

export function EnvironmentScreen({ scope, applicationID, service = "" }: { scope: "application" | "service"; applicationID: string; service?: string }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [environmentName, setEnvironmentName] = useState("Production")
  const [adding, setAdding] = useState(false)
  const [key, setKey] = useState("")
  const [value, setValue] = useState("")
  const [importEntries, setImportEntries] = useState<Array<{ key: string; value: string }>>([])
  const [importMessage, setImportMessage] = useState("")
  const fileInput = useRef<HTMLInputElement>(null)
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const environments = applicationQuery.data?.environments || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const serviceID = scope === "service" ? service : ""
  const variablesQuery = useQuery({ queryKey: queryKeys.variables(applicationID, environment?.id || "", serviceID), queryFn: ({ signal }) => api.environmentVariables(applicationID, environment.id, serviceID, signal), enabled: Boolean(environment) })
  const upsertMutation = useMutation({
    mutationFn: () => api.upsertEnvironmentVariable(applicationID, environment.id, { key, value, ...(serviceID ? { service_id: serviceID } : {}) }),
    onSuccess: async () => { toast(replacing ? `${key} replaced.` : `${key} saved.`); setKey(""); setValue(""); setAdding(false); await queryClient.invalidateQueries({ queryKey: queryKeys.variables(applicationID, environment.id, serviceID) }) },
  })
  const deleteMutation = useMutation({
    mutationFn: (variableID: string) => api.deleteEnvironmentVariable(applicationID, environment.id, variableID),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: queryKeys.variables(applicationID, environment.id, serviceID) }),
  })
  const importMutation = useMutation({
    mutationFn: async () => {
      const failures: string[] = []
      for (const entry of importEntries) {
        try { await api.upsertEnvironmentVariable(applicationID, environment.id, { ...entry, ...(serviceID ? { service_id: serviceID } : {}) }) } catch { failures.push(entry.key) }
      }
      if (failures.length) throw new Error(`Saved ${importEntries.length - failures.length} variables; failed: ${failures.join(", ")}`)
    },
    onSuccess: async () => { toast(`${importEntries.length} variables imported.`); setImportMessage(`${importEntries.length} variables imported.`); setImportEntries([]); await queryClient.invalidateQueries({ queryKey: queryKeys.variables(applicationID, environment.id, serviceID) }) },
  })
  if (applicationQuery.isPending) return <OperationLoading />
  if (applicationQuery.isError) return <OperationError title="Environment configuration could not be loaded" retry={() => applicationQuery.refetch()} />
  if (!environment) return <main className="mx-auto w-full max-w-[1600px] px-4 py-12"><StateCard title="No environments available" description="Create an environment before managing variables." /></main>
  const base = scope === "application" ? "/applications/" + applicationID : "/applications/" + applicationID + "/services/" + service
  const variables = variablesQuery.data?.variables || []
  const replacing = variables.some((item) => item.key === key.trim().toUpperCase())
  function exportNames() {
    const blob = new Blob([variables.map((item) => item.key + "=").join("\n")], { type: "text/plain;charset=utf-8" })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = (serviceID || applicationID) + ".env.example"
    anchor.click()
    URL.revokeObjectURL(url)
  }
  async function readEnvFile(file?: File) {
    if (!file) return
    const parsed: Array<{ key: string; value: string }> = []
    const seen = new Set<string>()
    const errors: string[] = []
    ;(await file.text()).split(/\r?\n/).forEach((raw, index) => {
      const line = raw.trim()
      if (!line || line.startsWith("#")) return
      const normalized = line.startsWith("export ") ? line.slice(7).trim() : line
      const split = normalized.indexOf("=")
      const parsedKey = split > 0 ? normalized.slice(0, split).trim().toUpperCase() : ""
      const parsedValue = split > 0 ? normalized.slice(split + 1) : ""
      if (!/^[A-Z_][A-Z0-9_]*$/.test(parsedKey) || !parsedValue) errors.push(`line ${index + 1}`)
      else if (seen.has(parsedKey)) errors.push(`duplicate ${parsedKey}`)
      else { seen.add(parsedKey); parsed.push({ key: parsedKey, value: parsedValue }) }
    })
    if (errors.length || !parsed.length) { setImportEntries([]); setImportMessage(errors.length ? `Import rejected: ${errors.join(", ")}.` : "Import rejected: no variables found."); return }
    setImportEntries(parsed)
    setImportMessage(`${parsed.length} validated variables are ready to import.`)
  }
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <PageHeader title="Environment" description={scope === "application" ? "Manage encrypted variables inherited by services." : "Manage encrypted overrides for this service."} back={{ label: "Back to overview", href: base }}>
      <AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-9 min-w-36 bg-card text-xs" />
      <Button variant="outline" disabled={!variables.length} onClick={exportNames}><DownloadSimpleIcon />Export names</Button>
      <input ref={fileInput} className="hidden" type="file" accept=".env,text/plain" onChange={(event) => { void readEnvFile(event.target.files?.[0]); event.currentTarget.value = "" }} />
      <Button variant="outline" disabled={!environment || importMutation.isPending} onClick={() => fileInput.current?.click()}><DownloadSimpleIcon className="rotate-180" />Import .env</Button>
      <Button onClick={() => setAdding((current) => !current)}><PlusIcon />Add variable</Button>
    </PageHeader>
    {scope === "application" ? <ApplicationTabs active="Environment" applicationID={applicationID} /> : <ServiceTabs active="Environment" service={service} applicationID={applicationID} />}
    {importMessage && <div className={`mb-5 rounded-xl border p-4 text-xs ${importMutation.isError || importMessage.startsWith("Import rejected") ? "border-destructive/30 bg-destructive/5 text-destructive" : "bg-card text-muted-foreground"}`}>{importMutation.isError ? importMutation.error.message : importMessage}{importEntries.length > 0 && <ConfirmationAction title={`Import ${importEntries.length} encrypted variables?`} description="Existing keys will be replaced. Secret values will be encrypted and never returned by the server." confirmLabel="Import variables" onConfirm={() => importMutation.mutateAsync()} trigger={<Button className="ml-4" size="sm" disabled={importMutation.isPending}>{importMutation.isPending ? "Importing..." : "Confirm import"}</Button>} />}</div>}
    {adding && environment && <Panel title="Add or replace variable" subtitle="Values are encrypted and never returned by the API"><form className="grid gap-4 p-5 sm:grid-cols-[1fr_1.5fr_auto] sm:items-end" onSubmit={(event) => { event.preventDefault(); if (!replacing) upsertMutation.mutate() }}><Field label="Key"><Input value={key} onChange={(event) => setKey(event.target.value.toUpperCase())} placeholder="DATABASE_URL" autoComplete="off" /></Field><Field label="Secret value"><Input type="password" value={value} onChange={(event) => setValue(event.target.value)} placeholder="Enter a new value" autoComplete="new-password" /></Field>{replacing ? <ConfirmationAction title={`Replace ${key}?`} description="The previous secret cannot be recovered. Running containers keep it until the next deployment." confirmLabel="Replace secret" onConfirm={() => upsertMutation.mutateAsync()} trigger={<Button type="button" disabled={!value || upsertMutation.isPending}>{upsertMutation.isPending ? "Saving..." : "Replace variable"}</Button>} /> : <Button type="submit" disabled={!key.trim() || !value || upsertMutation.isPending}>{upsertMutation.isPending ? "Saving..." : "Save variable"}</Button>}</form>{upsertMutation.isError && <p className="border-t px-5 py-3 text-xs text-destructive">{upsertMutation.error.message}</p>}</Panel>}
    <section className={`overflow-hidden rounded-xl border bg-card ${adding ? "mt-5" : ""}`}><header className="flex items-center gap-3 border-b bg-muted/70 px-5 py-4"><BracketsCurlyIcon size={17} /><div><h2 className="text-sm font-semibold">{scope === "application" ? "Shared variables" : "Service overrides"}</h2><p className="mt-0.5 text-xs text-muted-foreground">Only masked secret metadata is visible.</p></div><span className="ml-auto text-xs text-muted-foreground">{variables.length} variables</span></header>
      {variablesQuery.isPending ? <div className="animate-pulse p-6"><div className="h-32 rounded bg-muted" /></div> : variablesQuery.isError ? <div className="p-8 text-center"><p className="text-sm font-semibold">Variables could not be loaded</p><Button className="mt-4" variant="outline" onClick={() => variablesQuery.refetch()}>Retry</Button></div> : variables.length ? <div className="overflow-x-auto"><Table className="w-full min-w-[720px]"><TableHeader><TableRow><TableHead>Key</TableHead><TableHead>Stored value</TableHead><TableHead>Scope</TableHead><TableHead>Last updated</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader><TableBody>{variables.map((variable) => <TableRow key={variable.id}><TableCell className="font-mono text-xs font-semibold">{variable.key}</TableCell><TableCell className="font-mono text-xs text-muted-foreground">******** ({variable.value_last4 || "hidden"})</TableCell><TableCell><Badge variant="secondary">{variable.service_id ? "Service override" : "Application"}</Badge></TableCell><TableCell className="text-xs text-muted-foreground">{new Date(variable.updated_at).toLocaleString()}</TableCell><TableCell className="text-right"><ConfirmationAction title={"Delete " + variable.key + "?"} description="Running containers retain the previous value until redeployed." confirmLabel="Delete variable" destructive onConfirm={() => deleteMutation.mutateAsync(variable.id)} trigger={<Button variant="ghost" size="icon" aria-label={"Delete " + variable.key}><TrashIcon /></Button>} /></TableCell></TableRow>)}</TableBody></Table></div> : <div className="px-6 py-14 text-center"><KeyIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No variables in this scope</p></div>}
    </section>
  </main>
}

export function DomainsScreen({ scope, applicationID, service = "" }: { scope: "application" | "service"; applicationID: string; service?: string }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [environmentName, setEnvironmentName] = useState("Production")
  const [adding, setAdding] = useState(false)
  const [domainName, setDomainName] = useState("")
  const [targetName, setTargetName] = useState("")
  const [editingID, setEditingID] = useState("")
  const [editingName, setEditingName] = useState("")
  const [editingServiceName, setEditingServiceName] = useState("")
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const environments = applicationQuery.data?.environments || []
  const services = applicationQuery.data?.services || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const scopedServiceID = scope === "service" ? service : ""
  const targetService = scopedServiceID ? services.find((item) => item.id === scopedServiceID) : services.find((item) => item.name === targetName) || services[0]
  const domainsQuery = useQuery({ queryKey: queryKeys.domains(applicationID, environment?.id || "", scopedServiceID), queryFn: ({ signal }) => api.domains(applicationID, environment.id, scopedServiceID, signal), enabled: Boolean(environment) })
  const createMutation = useMutation({
    mutationFn: () => api.createDomain(applicationID, environment.id, { domain_name: domainName, service_id: targetService!.id }),
    onSuccess: async () => { toast(`${domainName} added.`); setDomainName(""); setAdding(false); await queryClient.invalidateQueries({ queryKey: queryKeys.domains(applicationID, environment.id, scopedServiceID) }) },
  })
  const deleteMutation = useMutation({
    mutationFn: (domainID: string) => api.deleteDomain(applicationID, environment.id, domainID),
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: queryKeys.domains(applicationID, environment.id, scopedServiceID) }),
  })
  const updateMutation = useMutation({
    mutationFn: () => {
      const serviceID = services.find((item) => item.name === editingServiceName)?.id || scopedServiceID
      if (!serviceID) throw new Error("Select a target service.")
      return api.updateDomain(applicationID, environment.id, editingID, { domain_name: editingName, service_id: serviceID })
    },
    onSuccess: async () => { toast(`${editingName} updated.`); setEditingID(""); setEditingName(""); setEditingServiceName(""); await queryClient.invalidateQueries({ queryKey: queryKeys.domains(applicationID, environment.id, scopedServiceID) }) },
  })
  if (applicationQuery.isPending) return <OperationLoading />
  if (applicationQuery.isError) return <OperationError title="Domains could not be loaded" retry={() => applicationQuery.refetch()} />
  if (!environment) return <main className="mx-auto w-full max-w-[1600px] px-4 py-12"><StateCard title="No environments available" description="Create an environment before configuring domains." /></main>
  const base = scope === "application" ? "/applications/" + applicationID : "/applications/" + applicationID + "/services/" + service
  const domains = domainsQuery.data?.domains || []
  const serviceName = (id: string) => services.find((item) => item.id === id)?.name || id

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <PageHeader title="Domains" description="Manage public Caddy routes for the selected environment." back={{ label: "Back to overview", href: base }}>
      <AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-9 min-w-36 bg-card text-xs" />
      <Button variant="outline" onClick={() => domainsQuery.refetch()}><ActivityIcon />Refresh</Button>
      <Button disabled={!services.length} onClick={() => setAdding((current) => !current)}><PlusIcon />Add domain</Button>
    </PageHeader>
    {scope === "application" ? <ApplicationTabs active="Domains" applicationID={applicationID} /> : <ServiceTabs active="Domains" service={service} applicationID={applicationID} />}
    {adding && environment && <Panel title="Add domain" subtitle="Caddy configuration is validated before the route is accepted">
      <form className="grid gap-4 p-5 sm:grid-cols-[1.5fr_1fr_auto] sm:items-end" onSubmit={(event) => { event.preventDefault(); createMutation.mutate() }}>
        <Field label="Hostname"><Input value={domainName} onChange={(event) => setDomainName(event.target.value.toLowerCase())} placeholder="api.example.com" /></Field>
        <Field label="Target service"><AppSelect options={services.map((item) => item.name)} value={targetService?.name || targetName} onValueChange={setTargetName} disabled={Boolean(scopedServiceID)} className="h-10 w-full bg-background text-xs" /></Field>
        <Button type="submit" disabled={!domainName.trim() || !targetService || createMutation.isPending}>{createMutation.isPending ? "Activating..." : "Add route"}</Button>
      </form>
      {createMutation.isError && <p className="border-t px-5 py-3 text-xs text-destructive">{createMutation.error.message}</p>}
    </Panel>}
    <section className={`overflow-hidden rounded-xl border bg-card ${adding ? "mt-5" : ""}`}><header className="flex items-center gap-3 border-b bg-muted/70 px-5 py-4"><GlobeIcon size={17} /><div><h2 className="text-sm font-semibold">Configured domains</h2><p className="mt-0.5 text-xs text-muted-foreground">Environment-specific routing and certificate state</p></div><span className="ml-auto text-xs text-muted-foreground">{domains.length} domains</span></header>
      {updateMutation.isError && <p role="alert" className="border-b bg-destructive/5 px-5 py-3 text-xs text-destructive">{updateMutation.error.message}</p>}{domainsQuery.isPending ? <div className="animate-pulse p-6"><div className="h-40 rounded bg-muted" /></div> : domainsQuery.isError ? <div className="p-8 text-center"><p className="text-sm font-semibold">Domains could not be loaded</p><Button className="mt-4" variant="outline" onClick={() => domainsQuery.refetch()}>Retry</Button></div> : domains.length ? <div className="overflow-x-auto"><Table className="w-full min-w-[860px]"><TableHeader><TableRow><TableHead>Domain</TableHead><TableHead>Service</TableHead><TableHead>Environment</TableHead><TableHead>Certificate</TableHead><TableHead>Last update</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader><TableBody>{domains.map((domain) => <TableRow key={domain.id}><TableCell>{editingID === domain.id ? <div className="flex gap-2"><Input value={editingName} onChange={(event) => setEditingName(event.target.value.toLowerCase())} className="h-8 min-w-56 text-xs" /></div> : <a href={"https://" + domain.domain_name} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-xs font-semibold hover:underline">{domain.domain_name}<ArrowSquareOutIcon size={12} /></a>}</TableCell><TableCell className="text-xs">{editingID === domain.id ? <AppSelect options={services.map((item) => item.name)} value={editingServiceName || serviceName(domain.service_id)} onValueChange={setEditingServiceName} disabled={Boolean(scopedServiceID)} className="h-8 min-w-36" /> : serviceName(domain.service_id)}</TableCell><TableCell className="text-xs text-muted-foreground">{environment?.name}</TableCell><TableCell><DomainBadge value={domain.ssl_status === "ACTIVE" ? "Active" : domain.ssl_status === "ERROR" ? "Error" : "Pending" } /></TableCell><TableCell className="text-xs text-muted-foreground">{new Date(domain.updated_at).toLocaleString()}</TableCell><TableCell className="text-right">{editingID === domain.id ? <><Button size="sm" disabled={!editingName.trim() || !editingServiceName || updateMutation.isPending} onClick={() => updateMutation.mutate()}>Save</Button><Button size="sm" variant="ghost" onClick={() => { setEditingID(""); setEditingServiceName("") }}>Cancel</Button></> : <><Button variant="ghost" size="icon" aria-label={"Edit " + domain.domain_name} onClick={() => { setEditingID(domain.id); setEditingName(domain.domain_name); setEditingServiceName(serviceName(domain.service_id)) }}><PencilSimpleIcon /></Button><ConfirmationAction title={"Remove " + domain.domain_name + "?"} description="Caddy will stop routing this hostname after validation succeeds." confirmLabel="Remove domain" destructive onConfirm={() => deleteMutation.mutateAsync(domain.id)} trigger={<Button variant="ghost" size="icon" aria-label={"Remove " + domain.domain_name}><TrashIcon /></Button>} /></>}</TableCell></TableRow>)}</TableBody></Table></div> : <div className="px-6 py-14 text-center"><GlobeIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No domains configured</p><p className="mt-1 text-xs text-muted-foreground">Add a hostname and select the service it should route to.</p></div>}
    </section>
  </main>
}

function DomainBadge({ value }: { value: string }) {
  const active = value === "Verified" || value === "Active"
  return <StatusBadge tone={active ? "success" : "warning"} dot>{value}</StatusBadge>
}

function OperationLoading() {
  return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="mb-7 h-8 w-52 rounded bg-muted" /><div className="h-72 rounded-xl border bg-card" /></main>
}

function OperationError({ title, retry }: { title: string; retry: () => unknown }) {
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-12 sm:px-6 lg:px-8"><StateCard title={title} description="The server did not return the required application or service data." retry={retry} /></main>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label> }
