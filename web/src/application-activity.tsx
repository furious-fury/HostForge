import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ActivityIcon,
  ArrowLeftIcon,
  BracketsCurlyIcon,
  CloudArrowUpIcon,
  FunnelSimpleIcon,
  GlobeIcon,
  MagnifyingGlassIcon,
  UserCircleIcon,
} from "@phosphor-icons/react"

const activity = [
  { time: "13:48", date: "Today", type: "Deployment", title: "api deployment completed", detail: "dep_7H3KD9 · commit 13d298a · triggered by Mr Fury", icon: CloudArrowUpIcon, tone: "bg-emerald-500" },
  { time: "12:31", date: "Today", type: "Configuration", title: "Shared variable updated", detail: "SENTRY_DSN · value remains hidden · changed by Mr Fury", icon: BracketsCurlyIcon, tone: "bg-blue-500" },
  { time: "10:14", date: "Today", type: "Domain", title: "staging.taxio.ng added", detail: "Assigned to web · DNS verification pending", icon: GlobeIcon, tone: "bg-amber-500" },
  { time: "16:42", date: "Yesterday", type: "Operator", title: "Application description changed", detail: "Updated by Mr Fury from application settings", icon: UserCircleIcon, tone: "bg-neutral-500" },
  { time: "14:18", date: "Yesterday", type: "Deployment", title: "worker deployment failed", detail: "dep_9BK4TR · image build exited with status 1", icon: CloudArrowUpIcon, tone: "bg-red-500" },
]

export function ApplicationActivity() {
  const [filter, setFilter] = useState("All activity")
  const [query, setQuery] = useState("")
  const visible = activity.filter((event) => (filter === "All activity" || event.type === filter) && `${event.title} ${event.detail}`.toLowerCase().includes(query.toLowerCase()))
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <main className="mx-auto w-full max-w-[1500px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><Link to="/applications/taxio" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />TaxIO overview</Link><div className="mb-7"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Activity</h1><p className="mt-2 text-sm text-muted-foreground">Configuration, deployment, domain, and operator events for TaxIO.</p></div><nav className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><div className="flex min-w-max gap-1">{tabs.map((tab) => <Link key={tab} to={tab === "Overview" ? "/applications/taxio" : `/applications/taxio/${tab.toLowerCase()}`} className={`rounded-lg px-3.5 py-2 text-xs font-medium ${tab === "Activity" ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`}>{tab}</Link>)}</div></nav><section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/75 p-4 sm:flex-row sm:items-center"><div className="flex items-center gap-2"><ActivityIcon size={16} /><span className="text-sm font-semibold">Application timeline</span></div><div className="flex gap-2 sm:ml-auto"><label className="relative"><FunnelSimpleIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><select value={filter} onChange={(event) => setFilter(event.target.value)} className="hf-compact-field pl-8"><option>All activity</option><option>Deployment</option><option>Configuration</option><option>Domain</option><option>Operator</option></select></label><label className="relative"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} /><input value={query} onChange={(event) => setQuery(event.target.value)} className="hf-compact-field w-56 pl-8" placeholder="Search activity" /></label></div></header><div className="divide-y">{visible.map((event) => { const EventIcon = event.icon; return <div key={`${event.date}-${event.time}-${event.title}`} className="flex gap-4 px-5 py-4"><div className="relative"><span className={`mt-1 block size-2.5 rounded-full ${event.tone}`} /></div><span className="w-20 shrink-0 font-mono text-[10px] text-muted-foreground">{event.date}<br />{event.time}</span><span className="grid size-8 shrink-0 place-items-center rounded-md border bg-muted"><EventIcon size={14} /></span><div><div className="flex items-center gap-2"><p className="text-xs font-semibold">{event.title}</p><span className="rounded bg-muted px-1.5 py-0.5 text-[9px] text-muted-foreground">{event.type}</span></div><p className="mt-1 text-[11px] text-muted-foreground">{event.detail}</p></div></div> })}</div></section></main>
}
