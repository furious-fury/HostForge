import { RouteTabs } from "@/components/route-tabs"

const serviceTabs = ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]

export function ServiceTabs({ active, serviceID, applicationID }: { active: string; serviceID: string; applicationID: string }) {
  const base = `/applications/${applicationID}/services/${serviceID}`
  return <RouteTabs active={active} label="Service navigation" tabs={serviceTabs.map((tab) => ({ label: tab, href: tab === "Overview" ? base : `${base}/${tab.toLowerCase()}` }))} />
}
