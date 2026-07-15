import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import { ActivityIcon, CloudArrowUpIcon, FunnelSimpleIcon, GlobeIcon, MagnifyingGlassIcon, SlidersHorizontalIcon } from "@phosphor-icons/react"

import { api, queryKeys } from "@/api"
import { AppSelect } from "@/components/app-select"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { RouteTabs } from "@/components/route-tabs"

const eventTypes = ["All activity", "deployment", "configuration", "domain", "runtime", "application", "service"]

function EventIcon({ type }: { type: string }) {
  if (type === "deployment") return <CloudArrowUpIcon size={14} />
  if (type === "domain") return <GlobeIcon size={14} />
  if (type === "configuration") return <SlidersHorizontalIcon size={14} />
  return <ActivityIcon size={14} />
}

export function ApplicationActivity({ applicationID }: { applicationID: string }) {
  const [filter, setFilter] = useState("All activity")
  const [query, setQuery] = useState("")
  const [cursor, setCursor] = useState("")
  const [history, setHistory] = useState<string[]>([])
  const eventsQuery = useQuery({
    queryKey: [...queryKeys.events(applicationID, "", filter === "All activity" ? "" : filter), cursor],
    queryFn: ({ signal }) => api.events({ applicationID, type: filter === "All activity" ? undefined : filter, cursor: cursor || undefined, limit: 50 }, signal),
    refetchInterval: 10000,
  })
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const events = eventsQuery.data?.events || []
  const visible = events.filter((event) => [event.event_type, event.message, event.detail, event.status, event.actor].join(" ").toLowerCase().includes(query.toLowerCase()))
  const base = "/applications/" + applicationID
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <main className="mx-auto w-full max-w-[1500px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <Link to={base} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground">Back to {applicationQuery.data?.application.name || "application"}</Link>
    <div className="mb-7"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Activity</h1><p className="mt-2 text-sm text-muted-foreground">Durable deployment, configuration, domain, and operator events.</p></div>
    <RouteTabs active="Activity" label="Application navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? base : base + "/" + tab.toLowerCase() }))} />
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/75 p-4 sm:flex-row sm:items-center"><div className="flex items-center gap-2"><ActivityIcon size={16} /><span className="text-sm font-semibold">Application timeline</span></div><div className="flex flex-col gap-2 sm:ml-auto sm:flex-row"><label className="relative"><FunnelSimpleIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><AppSelect options={eventTypes} value={filter} onValueChange={(value) => { setFilter(value); setCursor(""); setHistory([]) }} className="h-[2.1rem] min-w-40 bg-card pl-8 text-[0.68rem]" /></label><label className="relative"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-[2.1rem] w-full bg-card pl-8 text-[0.68rem] sm:w-64" placeholder="Search this page" /></label></div></header>
      {eventsQuery.isPending ? <div className="animate-pulse p-6"><div className="h-56 rounded bg-muted" /></div> : eventsQuery.isError ? <div className="p-10 text-center"><p className="text-sm font-semibold">Activity could not be loaded</p><Button className="mt-4" variant="outline" onClick={() => eventsQuery.refetch()}>Retry</Button></div> : visible.length ? <div className="divide-y">{visible.map((event) => <div key={event.id} className="flex gap-4 px-5 py-4"><span className={`mt-1 block size-2.5 shrink-0 rounded-full ${event.status === "FAILED" || event.status === "error" ? "bg-red-500" : event.status === "SUCCESS" ? "bg-emerald-500" : "bg-accent"}`} /><span className="w-32 shrink-0 font-mono text-[10px] text-muted-foreground">{new Date(event.created_at).toLocaleString()}</span><span className="grid size-8 shrink-0 place-items-center rounded-md border bg-muted"><EventIcon type={event.event_type} /></span><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><p className="text-xs font-semibold">{event.message}</p><Badge variant="secondary" className="text-[9px]">{event.event_type}</Badge></div><p className="mt-1 break-words text-[11px] text-muted-foreground">{event.detail || [event.deployment_id, event.actor].filter(Boolean).join(" / ")}</p></div></div>)}</div> : <div className="p-12 text-center"><ActivityIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">No activity recorded</p><p className="mt-1 text-xs text-muted-foreground">Events appear here as HostForge changes this application.</p></div>}
      {!eventsQuery.isPending && !eventsQuery.isError && <footer className="flex items-center gap-2 border-t bg-muted/30 px-5 py-3"><span className="text-[11px] text-muted-foreground">{visible.length} events on this page</span><Button className="ml-auto" size="sm" variant="outline" disabled={!history.length || eventsQuery.isFetching} onClick={() => { setCursor(history[history.length - 1] || ""); setHistory((items) => items.slice(0, -1)) }}>Previous</Button><Button size="sm" variant="outline" disabled={!eventsQuery.data?.next_cursor || eventsQuery.isFetching} onClick={() => { setHistory((items) => [...items, cursor]); setCursor(eventsQuery.data!.next_cursor) }}>Next</Button></footer>}
    </section>
  </main>
}

export default ApplicationActivity
