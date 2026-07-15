import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api, queryKeys } from "@/api"
import { AppSelect } from "@/components/app-select"
import { Link, useNavigate } from "react-router-dom"
import {
  ArrowLeftIcon,
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  CodeIcon,
  CopyIcon,
  DownloadSimpleIcon,
  MagnifyingGlassIcon,
  TerminalWindowIcon,
} from "@phosphor-icons/react"

import { StatusBadge } from "@/components/status-badge"
import { ConfirmationAction } from "@/components/confirmation-action"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Input } from "@/components/ui/input"
import "@/deployments.css"
import { useDeploymentLogStream } from "@/use-deployment-log-stream"


function StatusPill({ status }: { status: string }) {
  const tone = status === "Healthy" ? "success" : status === "Building" || status === "Releasing" || status === "Queued" ? "info" : status === "Failed" ? "error" : "neutral"
  const indicatorClass = status === "Building" ? "size-1.5 rounded-full bg-current animate-pulse" : "size-1.5 rounded-full bg-current"
  return <StatusBadge tone={tone} icon={<span className={indicatorClass} />}>{status}</StatusBadge>
}

export function DeploymentsList({ scope = "global", service = "", applicationID = "" }: { scope?: "global" | "application" | "service"; service?: string; applicationID?: string }) {
  const [status, setStatus] = useState("All statuses")
  const [trigger, setTrigger] = useState("All triggers")
  const [cursor, setCursor] = useState("")
  const [cursorHistory, setCursorHistory] = useState<string[]>([])
  const [query, setQuery] = useState("")
  const deploymentsQuery = useQuery({ queryKey: ["deployments", { scope, service, applicationID, status, trigger, cursor }], queryFn: ({ signal }) => api.deployments({ serviceID: scope === "service" ? service : undefined, applicationID: scope === "application" ? applicationID : undefined, status: status === "All statuses" ? undefined : status.toUpperCase(), trigger: trigger === "All triggers" ? undefined : trigger.toLowerCase(), cursor: cursor || undefined, limit: 50 }, signal), refetchInterval: 5000 })
  if (deploymentsQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-48 rounded bg-muted" /><div className="mt-6 h-80 rounded-xl border bg-card" /></main>
  if (deploymentsQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Deployments could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">The server did not return deployment history.</p><Button className="mt-4" variant="outline" onClick={() => deploymentsQuery.refetch()}>Retry</Button></section></main>
  const deployments = deploymentsQuery.data.deployments
  const scoped = deployments.filter((item) => [item.id, item.application_name, item.service_name, item.environment_name, item.branch, item.commit_hash, item.trigger].join(" ").toLowerCase().includes(query.toLowerCase()))
  const healthy = deployments.filter((item) => item.status === "SUCCESS").length
  const active = deployments.filter((item) => item.status === "QUEUED" || item.status === "BUILDING").length
  const failed = deployments.filter((item) => item.status === "FAILED").length
  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Deployments</h1><p className="mt-2 text-sm text-muted-foreground">Track build and release activity from the HostForge server.</p></div><Button className="sm:ml-auto" variant="outline" onClick={() => deploymentsQuery.refetch()}><ArrowsClockwiseIcon />Refresh</Button></div>
      <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "Total", value: deployments.length }, { label: "Successful", value: healthy }, { label: "In progress", value: active }, { label: "Failed", value: failed }].map((item) => <article key={item.label} className="hf-deployment-summary"><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">Current result set</p></article>)}</section>
      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="grid gap-3 border-b bg-muted/70 p-3 sm:grid-cols-[9rem_9rem_minmax(14rem,1fr)]"><AppSelect options={["All statuses", "Queued", "Building", "Success", "Failed", "Cancelled"]} value={status} onValueChange={(value) => { setStatus(value); setCursor(""); setCursorHistory([]) }} className="h-[2.1rem] min-w-0 bg-card px-2.5 text-[0.68rem]" /><AppSelect options={["All triggers", "Manual", "Webhook", "Redeploy", "Rollback"]} value={trigger} onValueChange={(value) => { setTrigger(value); setCursor(""); setCursorHistory([]) }} className="h-[2.1rem] min-w-0 bg-card px-2.5 text-[0.68rem]" /><label className="relative min-w-0"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="hf-deployment-search h-[2.1rem] w-full min-w-0 bg-card px-2.5 text-[0.68rem]" placeholder="Search this result page" /></label></header>
        {scoped.length ? <div className="overflow-x-auto"><Table className="w-full min-w-[850px] text-left"><TableHeader><TableRow><TableHead>Deployment</TableHead><TableHead>Service</TableHead><TableHead>Environment</TableHead><TableHead>Commit</TableHead><TableHead>Status</TableHead><TableHead>Trigger</TableHead><TableHead>Started</TableHead></TableRow></TableHeader><TableBody>{scoped.map((deployment) => <TableRow key={deployment.id}><TableCell><Link to={"/deployments/" + deployment.id} className="font-mono text-xs font-semibold hover:underline">{deployment.id}</Link></TableCell><TableCell><span className="text-xs font-semibold">{deployment.service_name || deployment.service_id}</span><span className="block text-[10px] text-muted-foreground">{deployment.application_name}</span></TableCell><TableCell><span className="text-xs">{deployment.environment_name || deployment.environment_id}</span><span className="block font-mono text-[10px] text-muted-foreground">{deployment.branch}</span></TableCell><TableCell className="font-mono text-xs">{deployment.commit_hash || "Pending"}</TableCell><TableCell><StatusPill status={deployment.status === "SUCCESS" ? "Healthy" : deployment.status[0] + deployment.status.slice(1).toLowerCase()} /></TableCell><TableCell className="text-xs text-muted-foreground">{deployment.trigger}</TableCell><TableCell className="text-xs text-muted-foreground">{new Date(deployment.created_at).toLocaleString()}</TableCell></TableRow>)}</TableBody></Table></div> : <div className="px-6 py-16 text-center"><CloudArrowUpIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No deployments found</p><p className="mt-1 text-xs text-muted-foreground">Deployments appear here after a configured service is deployed.</p></div>}
        <footer className="flex items-center gap-3 border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground"><span>Showing {scoped.length} deployments</span><div className="ml-auto flex gap-2"><Button size="sm" variant="outline" disabled={!cursorHistory.length || deploymentsQuery.isFetching} onClick={() => { const previous = cursorHistory[cursorHistory.length - 1] || ""; setCursorHistory((items) => items.slice(0, -1)); setCursor(previous) }}>Previous</Button><Button size="sm" variant="outline" disabled={!deploymentsQuery.data.next_cursor || deploymentsQuery.isFetching} onClick={() => { setCursorHistory((items) => [...items, cursor]); setCursor(deploymentsQuery.data.next_cursor) }}>Next</Button></div></footer>
      </section>
    </main>
  )
}

export function DeploymentDetail({ deploymentID }: { deploymentID: string }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [wrap, setWrap] = useState(false)
  const [logQuery, setLogQuery] = useState("")
  const deploymentQuery = useQuery({
    queryKey: queryKeys.deployment(deploymentID),
    queryFn: ({ signal }) => api.deployment(deploymentID, signal),
    refetchInterval: (query) => {
      const state = query.state.data?.deployment.status
      return state === "QUEUED" || state === "BUILDING" ? 2000 : false
    },
  })
  const stepsQuery = useQuery({
    queryKey: [...queryKeys.deployment(deploymentID), "steps"],
    queryFn: ({ signal }) => api.deploymentSteps(deploymentID, signal),
    refetchInterval: deploymentQuery.data?.deployment.status === "QUEUED" || deploymentQuery.data?.deployment.status === "BUILDING" ? 2000 : false,
  })
  const logsQuery = useQuery({
    queryKey: [...queryKeys.deployment(deploymentID), "logs"],
    queryFn: ({ signal }) => api.deploymentLogs(deploymentID, signal),
    retry: false,
    enabled: Boolean(deploymentQuery.data && deploymentQuery.data.deployment.status !== "QUEUED" && deploymentQuery.data.deployment.status !== "BUILDING"),
  })
  const active = deploymentQuery.data?.deployment.status === "QUEUED" || deploymentQuery.data?.deployment.status === "BUILDING"
  const liveLogs = useDeploymentLogStream(deploymentID, active)
  const redeployMutation = useMutation({ mutationFn: () => api.redeploy(deploymentID), onSuccess: (result) => navigate("/deployments/" + result.deployment.id) })
  const rollbackMutation = useMutation({ mutationFn: () => api.rollback(deploymentID), onSuccess: (result) => navigate("/deployments/" + result.deployment.id) })
  const cancelMutation = useMutation({
    mutationFn: () => api.cancelDeployment(deploymentID),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.deployment(deploymentID) })
      await queryClient.invalidateQueries({ queryKey: queryKeys.deployments() })
    },
  })

  if (deploymentQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-56 rounded bg-muted" /><div className="mt-6 h-96 rounded-xl border bg-card" /></main>
  if (deploymentQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Deployment could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">It may have been removed or the server is unavailable.</p><Button className="mt-4" variant="outline" onClick={() => deploymentQuery.refetch()}>Retry</Button></section></main>

  const deployment = deploymentQuery.data.deployment
  const displayStatus = deployment.status === "SUCCESS" ? "Healthy" : deployment.status[0] + deployment.status.slice(1).toLowerCase()
  const logText = active ? liveLogs.text : logsQuery.data?.text || ""
  const lines = logText.split(/\r?\n/).filter((line) => line.toLowerCase().includes(logQuery.toLowerCase()))
  const steps = stepsQuery.data?.steps || []

  function downloadLogs() {
    const blob = new Blob([logText], { type: "text/plain;charset=utf-8" })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = deployment.id + ".log"
    anchor.click()
    URL.revokeObjectURL(url)
  }

  return (
    <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to="/deployments" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />All deployments</Link>
      <div className="mb-7 flex flex-col gap-4 xl:flex-row xl:items-end">
        <div><p className="mb-2 flex items-center gap-2"><StatusPill status={displayStatus} /><span className="font-mono text-xs text-muted-foreground">{deployment.environment_id}</span></p><h1 className="font-mono text-3xl font-semibold tracking-[-0.04em]">{deployment.id}</h1><p className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"><span className="font-mono">{deployment.service_id}</span><span>{deployment.trigger || "manual"}</span><span>{new Date(deployment.created_at).toLocaleString()}</span></p></div>
        <div className="flex flex-wrap gap-2 xl:ml-auto">
          <Button variant="outline" disabled={!logText} onClick={downloadLogs}><DownloadSimpleIcon />Download logs</Button>
          <Button variant="outline" disabled={active || redeployMutation.isPending || !deployment.commit_hash} onClick={() => redeployMutation.mutate()}><ArrowsClockwiseIcon />Redeploy commit</Button>
          {deployment.status === "SUCCESS" && <ConfirmationAction title="Roll back to this release?" description="HostForge will create a new auditable deployment from this exact commit. Current history remains unchanged, and cutover only occurs after the new release passes its health check." confirmLabel="Create rollback" onConfirm={() => rollbackMutation.mutate()} trigger={<Button variant="outline" disabled={rollbackMutation.isPending}>Rollback to this release</Button>} />}
          {active && <ConfirmationAction title="Cancel this deployment?" description="The queued or running build will stop. Existing active releases are not changed." confirmLabel="Cancel deployment" onConfirm={() => cancelMutation.mutateAsync()} trigger={<Button disabled={cancelMutation.isPending}>Cancel deployment</Button>} />}
        </div>
      </div>
      {deployment.error_message && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{deployment.error_message}</div>}
      {cancelMutation.isError && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">The server could not cancel this deployment. Its state may have already changed.</div>}
      {rollbackMutation.isError && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">The server could not create the rollback deployment.</div>}

      <div className="mb-5 grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(340px,0.65fr)]">
        <Panel title="Deployment timeline" subtitle={stepsQuery.isPending ? "Loading recorded stages" : steps.length ? steps.length + " recorded stages" : "No stages have been recorded yet"}>
          {stepsQuery.isError ? <div className="p-6 text-xs text-muted-foreground">Deployment stages could not be loaded.</div> : steps.length ? <div className="p-5">{steps.map((step, index) => <div key={step.id} className="relative flex gap-4 pb-6 last:pb-0"><div className="relative z-10 grid size-7 shrink-0 place-items-center rounded-full bg-muted text-foreground ring-1 ring-border"><CheckCircleIcon size={15} weight={step.status === "success" ? "fill" : "regular"} /></div>{index < steps.length - 1 && <span className="absolute left-[13px] top-7 h-[calc(100%-1.75rem)] w-px bg-border" />}<div className="min-w-0 flex-1 pt-0.5"><div className="flex flex-wrap items-center gap-2"><p className="text-xs font-semibold">{step.step}</p><span className="ml-auto font-mono text-[10px] text-muted-foreground">{step.duration_ms} ms</span></div><p className="mt-1 text-[11px] text-muted-foreground">{step.status}{step.error_code ? " / " + step.error_code : ""}</p></div></div>)}</div> : <div className="p-8 text-center text-xs text-muted-foreground">Stages will appear as the deployment advances.</div>}
        </Panel>

        <Panel title="Deployment information" subtitle="Server-recorded build and release data">
          <div className="divide-y">{[
            { label: "Commit", value: deployment.commit_hash || "Pending" },
            { label: "Build method", value: deployment.builder_kind || "Not detected" },
            { label: "Stack", value: deployment.stack_label || deployment.stack_kind || "Not detected" },
            { label: "Image", value: deployment.image_ref || "Not created" },
            { label: "Triggered by", value: deployment.actor || deployment.trigger || "Unknown" },
            ...(deployment.rollback_of ? [{ label: "Rollback source", value: deployment.rollback_of }] : []),
            { label: "Last update", value: new Date(deployment.updated_at).toLocaleString() },
          ].map((item) => <div key={item.label} className="flex items-start gap-3 px-5 py-3.5"><CodeIcon size={15} className="mt-0.5 shrink-0 text-muted-foreground" /><div className="min-w-0"><p className="text-[10px] text-muted-foreground">{item.label}</p><p className="mt-0.5 break-all font-mono text-[11px] font-medium">{item.value}</p></div></div>)}</div>
        </Panel>
      </div>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="flex flex-col gap-3 border-b bg-muted/75 p-4 xl:flex-row xl:items-center"><div className="flex items-center gap-2"><TerminalWindowIcon size={16} /><div><h2 className="text-sm font-semibold">Deployment logs</h2><p className="mt-0.5 text-[11px] text-muted-foreground">{active ? "Live resumable build stream" : "Completed server log snapshot"}</p></div></div><div className="flex flex-wrap gap-2 xl:ml-auto"><label className="relative"><MagnifyingGlassIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" size={13} /><Input value={logQuery} onChange={(event) => setLogQuery(event.target.value)} className="h-[2.1rem] w-56 min-w-0 bg-card pl-8 text-[0.68rem]" placeholder="Search logs" /></label><button onClick={() => setWrap(!wrap)} className={`hf-log-toggle ${wrap ? "hf-log-toggle-active" : ""}`}>Wrap</button><Button variant="outline" size="icon" aria-label="Copy log output" disabled={!logText} onClick={() => navigator.clipboard.writeText(logText)}><CopyIcon /></Button></div></header>
        {active && liveLogs.error && <p role="alert" className="border-b bg-destructive/5 px-4 py-2 text-xs text-destructive">{liveLogs.error}</p>}
        <div className={`hf-terminal ${wrap ? "whitespace-pre-wrap" : "whitespace-pre"}`} role="log" aria-live="polite" aria-label="Deployment log output">{!active && logsQuery.isPending ? <span className="text-muted-foreground">Loading logs...</span> : !active && logsQuery.isError ? <span className="text-muted-foreground">Logs are not available yet.</span> : lines.length ? lines.map((line, index) => <div key={index} className="hf-log-line"><span className="hf-log-message">{line}</span></div>) : <span className="text-muted-foreground">{active ? "Waiting for build output..." : "No matching log lines."}</span>}</div>
        <footer className="flex flex-wrap items-center gap-4 border-t bg-muted/30 px-4 py-2.5 text-[10px] text-muted-foreground"><span className="flex items-center gap-1.5"><span className={`size-1.5 rounded-full ${active && liveLogs.connection === "connected" ? "bg-emerald-500" : active ? "animate-pulse bg-amber-500" : "bg-muted-foreground"}`} />{active ? liveLogs.connection : "Log snapshot"}</span><span>{lines.length} lines</span><span className="ml-auto">UTF-8</span></footer>
      </section>
    </main>
  )
}

function Panel({ title, subtitle, action, children }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode }) {
  return <section className="overflow-hidden rounded-xl border bg-card"><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}
