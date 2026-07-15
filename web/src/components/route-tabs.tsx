import { Link } from "react-router-dom"

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"

type RouteTab = {
  label: string
  href: string
}

export function RouteTabs({ tabs, active, label }: { tabs: readonly RouteTab[]; active: string; label: string }) {
  return <Tabs value={active} className="mb-5 overflow-x-auto rounded-xl border bg-card p-1"><TabsList className="h-auto min-w-max justify-start bg-transparent p-0" aria-label={label}>{tabs.map((tab) => <TabsTrigger key={tab.label} value={tab.label} asChild className="px-3.5 py-2 text-xs data-[state=active]:bg-accent data-[state=active]:text-accent-foreground data-[state=active]:shadow-none"><Link to={tab.href}>{tab.label}</Link></TabsTrigger>)}</TabsList></Tabs>
}
