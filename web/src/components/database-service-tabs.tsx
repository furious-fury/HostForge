import { RouteTabs } from "@/components/route-tabs"

export function DatabaseServiceTabs({ active, serviceID, applicationID }: { active: string; serviceID: string; applicationID: string }) {
  const base = `/applications/${applicationID}/services/${serviceID}`
  return <RouteTabs active={active} label="Database service navigation" tabs={[
    { label: "Overview", href: base },
    { label: "Data & connections", href: `${base}#connections` },
    { label: "Backups", href: `${base}#backups` },
    { label: "Metrics", href: `${base}#metrics` },
    { label: "Logs", href: `${base}#logs` },
    { label: "Settings", href: `${base}/settings` },
  ]} />
}
