import { FailedDeployment, LiveDeployment } from "@/deployment-state-screens"
import { DeploymentDetail, DeploymentsList } from "@/deployment-screens"
import { DomainsScreen, EnvironmentScreen, ServiceLogs, ServiceMetrics } from "@/operations-screens"
import { ServiceSettings } from "@/settings-screens"
import { AddService, ServiceOverview, ServicesList } from "@/services-screens"

export function ServicesRouter({ path }: { path: string }) {
  if (path === "/applications/taxio/services/new") {
    return <AddService />
  }

  const operationParts = path.split("/")
  if (operationParts.length === 6 && operationParts[1] === "applications" && operationParts[3] === "services") {
    const service = operationParts[4]
    const operation = operationParts[5]
    if (operation === "logs") return <ServiceLogs service={service} />
    if (operation === "metrics") return <ServiceMetrics service={service} />
    if (operation === "environment") return <EnvironmentScreen scope="service" service={service} />
    if (operation === "domains") return <DomainsScreen scope="service" service={service} />
    if (operation === "settings") return <ServiceSettings service={service} />
  }

  const deploymentDetail = path.match(/^\/applications\/taxio\/services\/([^/]+)\/deployments\/([^/]+)\/?$/)
  if (deploymentDetail) {
    if (deploymentDetail[2] === "dep_9BK4TR") return <FailedDeployment service={deploymentDetail[1]} deploymentID={deploymentDetail[2]} />
    if (deploymentDetail[2] === "dep_BUILD01") return <LiveDeployment service={deploymentDetail[1]} deploymentID={deploymentDetail[2]} />
    return <DeploymentDetail service={deploymentDetail[1]} deploymentID={deploymentDetail[2]} />
  }

  const serviceDeployments = path.match(/^\/applications\/taxio\/services\/([^/]+)\/deployments\/?$/)
  if (serviceDeployments) {
    return <DeploymentsList scope="service" service={serviceDeployments[1]} />
  }

  const serviceMatch = path.match(/^\/applications\/taxio\/services\/([^/]+)\/?$/)
  if (serviceMatch) {
    return <ServiceOverview service={serviceMatch[1]} />
  }

  return <ServicesList />
}
