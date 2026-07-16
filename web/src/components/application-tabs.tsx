import { RouteTabs } from "@/components/route-tabs"

const applicationTabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]

export function ApplicationTabs({ active, applicationID }: { active: string; applicationID: string }) {
  const base = "/applications/" + applicationID
  return <RouteTabs active={active} label="Application navigation" tabs={applicationTabs.map((tab) => ({ label: tab, href: tab === "Overview" ? base : base + "/" + tab.toLowerCase() }))} />
}
