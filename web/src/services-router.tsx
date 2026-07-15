import { lazy } from "react"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"

const DeploymentDetail = lazy(() => import("@/deployment-screens").then((module) => ({ default: module.DeploymentDetail })))
const DeploymentsList = lazy(() => import("@/deployment-screens").then((module) => ({ default: module.DeploymentsList })))
const DomainsScreen = lazy(() => import("@/operations-screens").then((module) => ({ default: module.DomainsScreen })))
const EnvironmentScreen = lazy(() => import("@/operations-screens").then((module) => ({ default: module.EnvironmentScreen })))
const ServiceLogs = lazy(() => import("@/operations-screens").then((module) => ({ default: module.ServiceLogs })))
const ServiceMetrics = lazy(() => import("@/operations-screens").then((module) => ({ default: module.ServiceMetrics })))
const ServiceSettings = lazy(() => import("@/settings-screens").then((module) => ({ default: module.ServiceSettings })))
const AddService = lazy(() => import("@/services-screens").then((module) => ({ default: module.AddService })))
const ServiceOverview = lazy(() => import("@/services-screens").then((module) => ({ default: module.ServiceOverview })))
const ServicesList = lazy(() => import("@/services-screens").then((module) => ({ default: module.ServicesList })))

export function ServicesRouter({ path }: { path: string }) {
  if (/^\/applications\/[^/]+\/services\/new\/?$/.test(path)) {
    return <AddService applicationID={path.split("/")[2]} />
  }

  const operationParts = path.split("/")
  if (operationParts.length === 6 && operationParts[1] === "applications" && operationParts[3] === "services") {
    const service = operationParts[4]
    const operation = operationParts[5]
    if (operation === "logs") return <ServiceLogs applicationID={operationParts[2]} service={service} />
    if (operation === "metrics") return <ServiceMetrics applicationID={operationParts[2]} service={service} />
    if (operation === "environment") return <EnvironmentScreen scope="service" applicationID={operationParts[2]} service={service} />
    if (operation === "domains") return <DomainsScreen scope="service" applicationID={operationParts[2]} service={service} />
    if (operation === "settings") return <ServiceSettings applicationID={operationParts[2]} serviceID={service} />
  }

  const deploymentDetail = path.match(/^\/applications\/[^/]+\/services\/([^/]+)\/deployments\/([^/]+)\/?$/)
  if (deploymentDetail) {
    return <DeploymentDetail deploymentID={deploymentDetail[2]} />
  }

  const serviceDeployments = path.match(/^\/applications\/[^/]+\/services\/([^/]+)\/deployments\/?$/)
  if (serviceDeployments) {
    return <DeploymentsList scope="service" service={serviceDeployments[1]} />
  }

  const serviceMatch = path.match(/^\/applications\/[^/]+\/services\/([^/]+)\/?$/)
  if (serviceMatch) {
    return <ServiceOverview applicationID={path.split("/")[2]} service={serviceMatch[1]} />
  }

  if (/^\/applications\/[^/]+\/services\/?$/.test(path)) {
    return <ServicesList applicationID={path.split("/")[2]} />
  }

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><h1 className="text-lg font-semibold">Service page not found</h1><p className="mt-2 text-xs text-muted-foreground">This service route is not available.</p><Button asChild className="mt-5"><Link to={"/applications/" + path.split("/")[2] + "/services"}>Return to services</Link></Button></section></main>
}
