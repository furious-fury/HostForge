import { useCallback, useEffect, useReducer, useRef, useState } from "react"
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query"
import { api, APIError, queryKeys, type DatabaseEngineDTO, type DatabaseMetricDTO, type DatabaseResourcePresetDTO, type EnvironmentDTO, type ServiceDTO, type ServiceEnvironmentDTO } from "@/api"
import { Link, useLocation, useNavigate } from "react-router-dom"
import {
  ActivityIcon,
  ArrowSquareOutIcon,
  CalendarDotsIcon,
  CodeIcon,
  CubeIcon,
  DatabaseIcon,
  GithubLogoIcon,
  GlobeIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  MagnifyingGlassIcon,
  PauseIcon,
  PlusIcon,
  RocketLaunchIcon,
  TrashIcon,
} from "@phosphor-icons/react"

import { AppSelect } from "@/components/app-select"
import { ApplicationTabs } from "@/components/application-tabs"
import { ServiceTabs } from "@/components/service-tabs"
import { DatabaseServiceTabs } from "@/components/database-service-tabs"
import { DatabaseIdentity } from "@/components/database-identity"
import { StackIdentity } from "@/components/stack-identity"
import { StatusBadge } from "@/components/status-badge"
import { ConfirmationAction } from "@/components/confirmation-action"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Input } from "@/components/ui/input"
import { Slider } from "@/components/ui/slider"
import { Switch } from "@/components/ui/switch"
import { useToast } from "@/toast-provider"
import "@/services.css"

function StatusPill({ status }: { status: string }) {
  const tone = status === "Running" || status === "Healthy" || status === "Live" ? "success" : status === "Deploying" || status === "Building" || status === "Provisioning" || status === "Queued" || status === "Starting" ? "info" : status === "Failed" ? "error" : "neutral"
  return <StatusBadge tone={tone} dot>{status}</StatusBadge>
}

function Panel({ title, subtitle, action, children, className = "" }: { title: string; subtitle?: string; action?: React.ReactNode; children: React.ReactNode; className?: string }) {
  return <section className={`overflow-hidden rounded-xl border bg-card ${className}`}><header className="flex min-h-14 items-center gap-4 border-b bg-muted/75 px-5 py-3"><div className="min-w-0"><h2 className="text-sm font-semibold tracking-tight">{title}</h2>{subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}</div>{action && <div className="ml-auto">{action}</div>}</header>{children}</section>
}

function serviceSourceError(error: unknown) {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : "The service could not be created."
  if (error.code === "repository_not_accessible") return "The selected repository is no longer accessible to this GitHub App installation. Refresh the repository list and choose again."
  if (error.code === "github_installation_suspended") return "This GitHub App installation is suspended. Restore it on GitHub, synchronize installations, and try again."
  if (error.code === "github_installation_not_found") return "This GitHub App installation is no longer known to HostForge. Synchronize installations and choose again."
  if (error.code === "app_not_configured") return "Configure the GitHub App before creating a service."
  if (error.code === "repositories_list_failed") return "GitHub could not confirm repository access. Check the integration and try again."
  return error.message.replaceAll("_", " ")
}

class InitialServiceDeploymentError extends Error {
  constructor(public service: ServiceDTO, public deploymentError: unknown) {
    super("The service and branch were saved, but the first deployment could not be started.")
    this.name = "InitialServiceDeploymentError"
  }
}

export function ServicesList({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const [query, setQuery] = useState("")
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  if (applicationQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-48 rounded bg-muted" /><div className="mt-6 h-80 rounded-xl border bg-card" /></main>
  if (applicationQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Services could not be loaded</h1><Button className="mt-4" variant="outline" onClick={() => applicationQuery.refetch()}>Retry</Button></section></main>
  const { application, environments, services, service_bindings: bindings } = applicationQuery.data
  const databaseInstances = applicationQuery.data.database_instances || {}
  const visibleRows = services.filter((service) => [service.name, service.repo_url, service.runtime].join(" ").toLowerCase().includes(query.toLowerCase()))
  const states = services.map((service) => service.service_type === "database" ? databaseServiceListState(databaseInstances[service.id] || [], environments) : serviceListState(bindings[service.id] || [], environments))
  const running = states.filter((state) => state.status === "Running" || state.status === "Healthy").length
  const stopped = states.filter((state) => state.status === "Stopped").length
  const awaiting = states.filter((state) => state.status === "Awaiting deployment").length
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Services</h1><p className="mt-2 text-sm text-muted-foreground">Deployable components of {application.name} across every environment.</p></div><Button className="sm:ml-auto" onClick={() => navigate("/applications/" + applicationID + "/services/new")}><PlusIcon />Add service</Button></div>
    <ApplicationTabs active="Services" applicationID={applicationID} />
    <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "Total services", value: services.length }, { label: "Running", value: running }, { label: "Stopped", value: stopped }, { label: "Awaiting deploy", value: awaiting }].map((item) => <article key={item.label} className="hf-service-summary"><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">Across all environments</p></article>)}</section>
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/70 p-4 sm:flex-row sm:items-center"><div><h2 className="text-sm font-semibold">All services</h2><p className="mt-0.5 text-xs text-muted-foreground">Source and release bindings</p></div><label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full bg-card pl-9 text-xs" placeholder="Search services" /></label></header>
      {visibleRows.length ? <div className="p-4"><div className="overflow-x-auto rounded-lg border"><Table className="w-full min-w-[880px]"><TableHeader><TableRow><TableHead>Service</TableHead><TableHead>Source</TableHead><TableHead>Stack</TableHead><TableHead>Binding</TableHead><TableHead>Status</TableHead><TableHead>Active resource</TableHead><TableHead>Port</TableHead></TableRow></TableHeader><TableBody>{visibleRows.map((service) => { const databaseState = service.service_type === "database" ? databaseServiceListState(databaseInstances[service.id] || [], environments) : undefined; const state = databaseState || serviceListState(bindings[service.id] || [], environments); return <TableRow key={service.id}><TableCell><Link to={"/applications/" + applicationID + "/services/" + service.id} className="flex items-center gap-3 text-xs font-semibold hover:underline">{service.service_type === "database" ? <DatabaseIdentity engine={service.stack_kind} label={service.stack_label} showLabel={false} iconClassName="size-8" imageClassName="size-5" /> : <StackIdentity kind={service.stack_kind} label={service.stack_label} showLabel={false} />}{service.name}</Link></TableCell><TableCell className="max-w-64 truncate text-xs text-muted-foreground">{service.service_type === "database" ? "HostForge managed database" : service.repo_url}</TableCell><TableCell>{service.service_type === "database" ? <DatabaseIdentity engine={service.stack_kind} label={service.stack_label} iconClassName="size-7 rounded-md" imageClassName="size-5" /> : <StackIdentity kind={service.stack_kind} label={service.stack_label} iconClassName="size-7 rounded-md" />}</TableCell><TableCell><span className="font-mono text-xs">{databaseState ? `${databaseState.instanceCount} isolated env${databaseState.instanceCount === 1 ? "" : "s"}` : state.binding?.branch || "Not set"}</span>{state.environment && <span className="mt-1 block text-[10px] text-muted-foreground">{state.environment.name}</span>}</TableCell><TableCell><StatusPill status={state.status} />{state.environment && <span className="mt-1 block text-[10px] text-muted-foreground">{state.environment.name}</span>}</TableCell><TableCell className="font-mono text-xs text-muted-foreground">{databaseState ? databaseState.instance?.volume_name || "Reserved" : state.binding?.active_deployment_id || "None"}</TableCell><TableCell className="font-mono text-xs">:{service.internal_port}</TableCell></TableRow> })}</TableBody></Table></div></div> : <div className="px-6 py-14 text-center"><CubeIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">{services.length ? "No matching services" : "No services yet"}</p><Button className="mt-4" onClick={() => navigate("/applications/" + applicationID + "/services/new")}><PlusIcon />Add service</Button></div>}
      <footer className="border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground">{visibleRows.length} services</footer>
    </section>
  </main>
}

function serviceListState(bindings: ServiceEnvironmentDTO[], environments: EnvironmentDTO[]) {
  const binding = bindings.find((item) => item.active_deployment_id && item.desired_state === "running")
    || bindings.find((item) => item.active_deployment_id)
    || bindings.find((item) => item.branch)
    || bindings[0]
  const environment = environments.find((item) => item.id === binding?.environment_id)
  const status = binding?.desired_state === "stopped" && binding.active_deployment_id
    ? "Stopped"
    : binding?.active_deployment_id
      ? "Running"
      : binding?.branch
        ? "Awaiting deployment"
        : "Configuration required"
  return { binding, environment, status }
}

function databaseServiceListState(instances: import("@/api").DatabaseInstanceDTO[], environments: EnvironmentDTO[]) {
  const instance = instances.find((item) => item.status === "healthy")
    || instances.find((item) => item.status === "starting" || item.status === "provisioning")
    || instances.find((item) => item.status === "failed")
    || instances[0]
  const environment = environments.find((item) => item.id === instance?.environment_id)
  const status = instances.some((item) => item.status === "healthy")
    ? "Healthy"
    : instances.some((item) => item.status === "starting" || item.status === "provisioning")
      ? "Provisioning"
      : instances.some((item) => item.status === "failed")
        ? "Failed"
        : instances.some((item) => item.status === "deleted")
          ? "Deleted"
          : "Stopped"
  return { instance, instanceCount: instances.length, binding: undefined, environment, status }
}

export function AddService({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [serviceType, setServiceType] = useState<"application" | "database" | null>(null)
  const [installationName, setInstallationName] = useState("")
  const [repositoryName, setRepositoryName] = useState("")
  const [branch, setBranch] = useState("")
  const [environmentName, setEnvironmentName] = useState("Production")
  const [name, setName] = useState<string | null>(null)
  const [rootDirectory, setRootDirectory] = useState("")
  const [runtime, setRuntime] = useState("auto")
  const [installCmd, setInstallCmd] = useState("")
  const [buildCmd, setBuildCmd] = useState("")
  const [startCmd, setStartCmd] = useState("")
  const [internalPort, setInternalPort] = useState("3000")
  const [healthPath, setHealthPath] = useState("/")
  const [autoDeploy, setAutoDeploy] = useState(true)
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const installationsQuery = useQuery({ queryKey: queryKeys.githubInstallations, queryFn: ({ signal }) => api.githubInstallations(signal), enabled: serviceType === "application" })
  const installations = installationsQuery.data?.installations.filter((item) => !item.suspended) || []
  const installation = installations.find((item) => item.account_login === installationName) || installations[0]
  const repositoriesQuery = useQuery({ queryKey: queryKeys.githubRepositories(installation?.installation_id || 0), queryFn: ({ signal }) => api.githubRepositories(installation.installation_id, signal), enabled: Boolean(installation) })
  const repositories = repositoriesQuery.data?.repositories || []
  const repository = repositories.find((item) => item.full_name === repositoryName) || repositories[0]
  const branchesQuery = useQuery({ queryKey: queryKeys.repositoryBranches(repository?.clone_url || "", installation?.installation_id || 0), queryFn: ({ signal }) => api.repositoryBranches(repository.clone_url, installation.installation_id, signal), enabled: Boolean(repository && installation) })
  const branches = branchesQuery.data?.branches || []
  const selectedBranch = branches.includes(branch) ? branch : branchesQuery.data?.default_branch || repository?.default_branch || ""
  const environments = applicationQuery.data?.environments || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const applicationName = applicationQuery.data?.application.name || "application"
  const repositoryServiceName = repository?.full_name.split("/").filter(Boolean).at(-1) || repository?.clone_url.split("/").filter(Boolean).at(-1)?.replace(/\.git$/i, "") || ""
  const serviceName = name ?? repositoryServiceName
  const createMutation = useMutation({
    mutationFn: async () => {
      if (!installation || !repository || !environment || !selectedBranch) throw new Error("Select an installation, repository, environment, and branch.")
      const result = await api.createService(applicationID, {
        name: serviceName,
        repo_url: repository.clone_url,
        github_installation_id: installation.installation_id,
        environment_id: environment.id,
        branch: selectedBranch,
        auto_deploy: autoDeploy,
        root_directory: rootDirectory,
        runtime,
        install_cmd: installCmd,
        build_cmd: buildCmd,
        start_cmd: startCmd,
        internal_port: Number(internalPort),
        health_check_path: healthPath,
      })
      try {
        const deployment = await api.deploy(result.service.id, environment.id)
        return { service: result.service, deployment: deployment.deployment }
      } catch (error) {
        throw new InitialServiceDeploymentError(result.service, error)
      }
    },
    onSuccess: async ({ deployment }) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) })
      navigate("/deployments/" + deployment.id)
    },
    onError: async (error) => {
      if (!(error instanceof InitialServiceDeploymentError)) return
      await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) })
      toast(`${error.message} ${serviceSourceError(error.deploymentError)}`, { tone: "warning", duration: 15000 })
      navigate("/applications/" + applicationID + "/services/" + error.service.id)
    },
  })
  if (applicationQuery.isPending || serviceType === "application" && installationsQuery.isPending) return <main className="mx-auto w-full max-w-5xl animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-48 rounded bg-muted" /><div className="mt-8 h-64 rounded-xl border bg-card" /></main>
  if (applicationQuery.isError || serviceType === "application" && installationsQuery.isError) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><GithubLogoIcon className="mx-auto text-muted-foreground" size={24} /><h1 className="mt-3 text-sm font-semibold">Service prerequisites could not be loaded</h1><p className="mt-1 text-xs text-muted-foreground">Check the server and GitHub App integration, then retry.</p><Button className="mt-4" variant="outline" onClick={() => { applicationQuery.refetch(); installationsQuery.refetch() }}>Retry</Button></section></main>
  if (!environments.length) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><h1 className="text-sm font-semibold">No environment is available</h1><p className="mt-1 text-xs text-muted-foreground">This application needs an environment before a service can be configured.</p><Button asChild className="mt-4" variant="outline"><Link to={"/applications/" + applicationID + "/settings"}>Open application settings</Link></Button></section></main>
  if (serviceType === "application" && !installations.length) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><GithubLogoIcon className="mx-auto text-muted-foreground" size={26} /><h1 className="mt-3 text-sm font-semibold">No active GitHub installation</h1><p className="mx-auto mt-1 max-w-md text-xs leading-5 text-muted-foreground">Configure or restore a GitHub App installation, then synchronize it before adding a repository-backed service.</p><div className="mt-5 flex justify-center gap-2"><Button asChild><Link to="/onboarding">Configure GitHub App</Link></Button><Button variant="outline" onClick={() => installationsQuery.refetch()}>Check again</Button></div></section></main>
  return <main className="mx-auto w-full max-w-5xl px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-8"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Add service</h1><p className="mt-2 text-sm text-muted-foreground">{serviceType === "application" ? `Configure the application service for ${applicationName}.` : serviceType === "database" ? `Configure a private database for ${applicationName}.` : `Choose what ${applicationName} needs.`}</p></div>
    {!serviceType ? <ServiceTypeChooser onSelectApplication={() => setServiceType("application")} onSelectDatabase={() => setServiceType("database")} /> :
    serviceType === "database" ? <DatabaseServiceWizard applicationID={applicationID} applicationName={applicationName} environments={environments} services={applicationQuery.data?.services || []} onBack={() => setServiceType(null)} /> :
    <form className="space-y-5" onSubmit={(event) => { event.preventDefault(); createMutation.mutate() }}>
      <Panel title="GitHub source" subtitle="Choose exactly what HostForge should deploy for the first release"><div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Installation"><AppSelect options={installations.map((item) => item.account_login)} value={installation?.account_login || installationName} onValueChange={(value) => { setInstallationName(value); setRepositoryName(""); setBranch("") }} className="h-10 w-full bg-background text-xs" /></Field><Field label="Repository"><AppSelect searchable searchPlaceholder="Search repositories..." emptyMessage="No repository matches your search." options={repositories.map((item) => item.full_name)} value={repository?.full_name || repositoryName} onValueChange={(value) => { setRepositoryName(value); setBranch("") }} disabled={!installation || repositoriesQuery.isPending} className="h-10 w-full bg-background text-xs" /></Field><Field label="Environment"><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-10 w-full bg-background text-xs" /></Field><Field label="Branch"><AppSelect options={branches} value={selectedBranch} onValueChange={setBranch} disabled={!repository || branchesQuery.isPending} className="h-10 w-full bg-background text-xs" /></Field></div><div className="border-t px-6 py-3 text-xs" aria-live="polite">{repositoriesQuery.isPending ? <p className="text-muted-foreground">Loading repositories from GitHub...</p> : repositoriesQuery.isError ? <p role="alert" className="flex items-center justify-between gap-3 text-destructive"><span>Repositories could not be loaded for this installation.</span><Button type="button" size="sm" variant="outline" onClick={() => repositoriesQuery.refetch()}>Retry</Button></p> : !repositories.length ? <p className="text-muted-foreground">This installation does not expose any repositories. Update its repository access on GitHub.</p> : branchesQuery.isPending ? <p className="text-muted-foreground">Loading repository branches...</p> : branchesQuery.isError ? <p role="alert" className="flex items-center justify-between gap-3 text-destructive"><span>Branches could not be loaded for this repository.</span><Button type="button" size="sm" variant="outline" onClick={() => branchesQuery.refetch()}>Retry</Button></p> : !branches.length ? <p className="text-muted-foreground">No branches are available in this repository.</p> : <p className="text-muted-foreground">Ready to deploy <span className="font-mono font-semibold text-foreground">{selectedBranch}</span> to <span className="font-semibold text-foreground">{environment?.name}</span>.</p>}</div></Panel>
      <Panel title="Build and runtime" subtitle="Railpack detects framework defaults during the first deployment"><div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Service name"><Input value={serviceName} onChange={(event) => setName(event.target.value)} placeholder="api" /></Field><Field label="Runtime"><AppSelect options={["auto", "bun"]} value={runtime} onValueChange={setRuntime} className="h-10 w-full bg-background text-xs" /></Field><Field label="Root directory"><Input value={rootDirectory} onChange={(event) => setRootDirectory(event.target.value)} placeholder="Repository root" /></Field><Field label="Internal port"><Input type="number" min="1" max="65535" value={internalPort} onChange={(event) => setInternalPort(event.target.value)} /></Field><Field label="Install command"><Input value={installCmd} onChange={(event) => setInstallCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Build command"><Input value={buildCmd} onChange={(event) => setBuildCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Start command"><Input value={startCmd} onChange={(event) => setStartCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Health-check path"><Input value={healthPath} onChange={(event) => setHealthPath(event.target.value)} placeholder="/" /><span className="mt-1.5 block text-[10px] leading-4 text-muted-foreground">Use `/` for zero-config framework apps. Set a dedicated endpoint only when the application provides one.</span></Field></div></Panel>
      <Panel title="Release behavior" subtitle="The first deployment starts immediately after the service is created"><div className="p-6"><label className="flex items-center justify-between gap-5 rounded-lg border bg-muted/25 p-4"><span><span className="block text-xs font-semibold">Deploy future pushes automatically</span><span className="mt-1 block text-[11px] leading-5 text-muted-foreground">When enabled, pushes to {selectedBranch || "the selected branch"} will create new deployments for {environment?.name || "this environment"}.</span></span><Switch checked={autoDeploy} onCheckedChange={setAutoDeploy} aria-label="Deploy future pushes automatically" /></label><p className="mt-4 text-[11px] leading-5 text-muted-foreground">Railpack detects the application stack, build method, and runtime commands during the first deployment. You will be taken to live deployment details to watch detection and build progress.</p></div></Panel>
      {createMutation.isError && !(createMutation.error instanceof InitialServiceDeploymentError) && <div role="alert" className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{serviceSourceError(createMutation.error)}</div>}
      <div className="flex justify-end gap-2"><Button type="button" variant="ghost" onClick={() => setServiceType(null)}>Change service type</Button><Button type="button" variant="outline" onClick={() => navigate("/applications/" + applicationID + "/services")}>Cancel</Button><Button type="submit" disabled={!serviceName.trim() || !installation || !repository || !selectedBranch || createMutation.isPending}><RocketLaunchIcon weight="fill" />{createMutation.isPending ? "Creating and starting deployment..." : "Create and deploy"}</Button></div>
    </form>
    }
  </main>
}

function ServiceTypeChooser({ onSelectApplication, onSelectDatabase }: { onSelectApplication: () => void; onSelectDatabase: () => void }) {
  return <section className="grid gap-4 md:grid-cols-3">
    <button type="button" onClick={onSelectApplication} className="group rounded-xl border bg-card p-6 text-left shadow-sm transition hover:border-accent hover:bg-muted/30 focus:outline-none focus:ring-3 focus:ring-ring/20">
      <span className="grid size-11 place-items-center rounded-lg bg-accent text-accent-foreground"><CodeIcon size={21} /></span>
      <h2 className="mt-5 text-sm font-semibold">Application service</h2>
      <p className="mt-2 text-xs leading-5 text-muted-foreground">Frontend, backend, API, worker, or full-stack repository. Railpack detects the framework and build process.</p>
      <span className="mt-5 inline-flex text-xs font-semibold group-hover:underline">Configure service</span>
    </button>
    <button type="button" onClick={onSelectDatabase} className="group rounded-xl border bg-card p-6 text-left shadow-sm transition hover:border-accent hover:bg-muted/30 focus:outline-none focus:ring-3 focus:ring-ring/20">
      <span className="grid size-11 place-items-center rounded-lg bg-accent text-accent-foreground"><DatabaseIcon size={21} /></span>
      <h2 className="mt-5 text-sm font-semibold">Database</h2>
      <p className="mt-2 text-xs leading-5 text-muted-foreground">Persistent PostgreSQL, MySQL, MariaDB, MongoDB, Redis, or Valkey isolated by environment.</p>
      <span className="mt-5 inline-flex text-xs font-semibold group-hover:underline">Configure database</span>
    </button>
    <PlannedServiceType icon={CalendarDotsIcon} title="Cron job" description="Run repository commands on a recurring schedule with execution history and logs." />
  </section>
}

function formatMemory(bytes: number) {
  return bytes >= 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(1)} GB` : `${(bytes / 1024 ** 2).toFixed(1)} MB`
}

function floorToStep(value: number, step: number) {
  return Math.floor((value + Number.EPSILON) / step) * step
}

function ResourceSlider({ id, label, value, min, max, step, unit, minimumLabel, capacityLabel, disabled, onChange }: {
  id: string
  label: string
  value: number
  min: number
  max: number
  step: number
  unit: string
  minimumLabel?: string
  capacityLabel?: string
  disabled?: boolean
  onChange: (value: number) => void
}) {
  const formattedValue = Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)
  return <div className="rounded-lg border bg-background p-4">
    <div className="mb-4 flex items-center justify-between gap-4"><label id={`${id}-label`} className="text-xs font-semibold">{label}</label><output className="rounded-md border bg-muted/40 px-2.5 py-1 font-mono text-xs font-semibold">{formattedValue} {unit}</output></div>
    <Slider id={id} min={min} max={max} step={step} value={[value]} onValueChange={([nextValue]) => onChange(nextValue)} disabled={disabled} aria-labelledby={`${id}-label`} aria-valuetext={`${formattedValue} ${unit}`} />
    <div className="mt-2 flex items-center justify-between font-mono text-[10px] text-muted-foreground"><span>{min} {unit}</span><span>{max} {unit}</span></div>
    {minimumLabel && <p className="mt-3 text-[10px] text-muted-foreground">{minimumLabel}</p>}
    {capacityLabel && <p className="mt-1 text-[10px] text-muted-foreground">{capacityLabel}</p>}
  </div>
}

function DatabaseWizardConnection({ applicationID, consumer, environmentIDs, variableKey, selected, replaceExisting, onToggle, onReplacementChange, onConflictChange }: {
  applicationID: string
  consumer: ServiceDTO
  environmentIDs: string[]
  variableKey: string
  selected: boolean
  replaceExisting: boolean
  onToggle: () => void
  onReplacementChange: (value: boolean) => void
  onConflictChange: (serviceID: string, conflict: boolean) => void
}) {
  const normalizedKey = variableKey.trim().toUpperCase()
  const variableQueries = useQueries({ queries: environmentIDs.flatMap((environmentID) => [
    { queryKey: queryKeys.variables(applicationID, environmentID, ""), queryFn: ({ signal }: { signal: AbortSignal }) => api.environmentVariables(applicationID, environmentID, "", signal), enabled: selected && Boolean(normalizedKey) },
    { queryKey: queryKeys.variables(applicationID, environmentID, consumer.id), queryFn: ({ signal }: { signal: AbortSignal }) => api.environmentVariables(applicationID, environmentID, consumer.id, signal), enabled: selected && Boolean(normalizedKey) },
  ]) })
  const conflictCount = selected ? variableQueries.filter((query) => query.data?.variables.some((variable) => variable.key === normalizedKey)).length : 0
  const conflict = conflictCount > 0
  useEffect(() => onConflictChange(consumer.id, conflict), [conflict, consumer.id, onConflictChange])
  return <div className={`rounded-lg border bg-background p-4 ${conflict ? "border-amber-500/50" : ""}`}><label className="flex cursor-pointer items-center justify-between gap-4"><span><span className="block text-xs font-semibold">{consumer.name}</span><span className="mt-1 block font-mono text-[10px] text-muted-foreground">{normalizedKey}</span></span><Switch checked={selected} onCheckedChange={onToggle} aria-label={`Connect ${consumer.name}`} /></label>{selected && conflict && <label className="mt-3 flex items-start gap-3 border-t pt-3"><Switch checked={replaceExisting} onCheckedChange={onReplacementChange} aria-label={`Replace existing ${normalizedKey} for ${consumer.name}`} /><span className="text-[10px] leading-4 text-amber-700 dark:text-amber-400"><strong className="block">Existing variable conflict</strong>{normalizedKey} already exists in {conflictCount} selected environment scope{conflictCount === 1 ? "" : "s"}. Confirm that this managed database URL may replace that value for {consumer.name} at deployment time. The saved variable remains unchanged for other services.</span></label>}</div>
}

function DatabaseServiceWizard({ applicationID, applicationName, environments, services, onBack }: { applicationID: string; applicationName: string; environments: EnvironmentDTO[]; services: ServiceDTO[]; onBack: () => void }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const catalogQuery = useQuery({ queryKey: queryKeys.databaseEngines, queryFn: ({ signal }) => api.databaseEngines(signal) })
  const destinationsQuery = useQuery({ queryKey: queryKeys.backupDestinations, queryFn: ({ signal }) => api.backupDestinations(signal) })
  const [engineID, setEngineID] = useState<DatabaseEngineDTO["id"]>("postgresql")
  const [version, setVersion] = useState("")
  const [databaseName, setDatabaseName] = useState("database")
  const [presetID, setPresetID] = useState<DatabaseResourcePresetDTO["id"]>("development")
  const [customCPU, setCustomCPU] = useState("2")
  const [customMemoryGB, setCustomMemoryGB] = useState("2")
  const [backupEnabled, setBackupEnabled] = useState(false)
  const [backupDestinationID, setBackupDestinationID] = useState("")
  const [selectedEnvironmentIDs, setSelectedEnvironmentIDs] = useState<string[]>(() => environments.map((environment) => environment.id))
  const [consumerServiceIDs, setConsumerServiceIDs] = useState<string[]>([])
	const [replacementServiceIDs, setReplacementServiceIDs] = useState<string[]>([])
	const [conflictingServiceIDs, setConflictingServiceIDs] = useState<string[]>([])
  const [variableKey, setVariableKey] = useState("DATABASE_URL")
  const engines = catalogQuery.data?.engines || []
  const engine = engines.find((candidate) => candidate.id === engineID) || engines[0]
  const availableVersions = engine?.versions.filter((candidate) => candidate.provisioning_available) || []
  const selectedVersion = availableVersions.some((candidate) => candidate.version === version) ? version : availableVersions.find((candidate) => candidate.default)?.version || availableVersions[0]?.version || ""
  const presets = catalogQuery.data?.resource_presets || []
	const destinations = destinationsQuery.data?.destinations || []
	const backupDestination = destinations.find((item) => item.id === backupDestinationID) || destinations[0]
  const selectedPreset = presets.find((candidate) => candidate.id === presetID)
  const preset = selectedPreset && (selectedPreset.id === "custom" || selectedPreset.memory_limit_bytes >= (engine?.minimum_memory_bytes || 0)) ? selectedPreset : presets.find((candidate) => candidate.id !== "custom" && candidate.memory_limit_bytes >= (engine?.minimum_memory_bytes || 0))
  const capacity = catalogQuery.data?.resource_capacity
  const capacityAvailable = capacity?.available === true
  const instanceCount = Math.max(1, selectedEnvironmentIDs.length)
  const minimumMemoryGB = (engine?.minimum_memory_bytes || 512 * 1024 ** 2) / 1024 ** 3
  const availableCPUPerInstance = capacityAvailable ? capacity.cpu_available_millis / 1000 / instanceCount : 32
  const availableMemoryPerInstanceGB = capacityAvailable ? capacity.memory_available_bytes / 1024 ** 3 / instanceCount : 256
  const customCPUCapacityValid = !capacityAvailable || availableCPUPerInstance >= 0.1
  const customMemoryCapacityValid = !capacityAvailable || availableMemoryPerInstanceGB >= minimumMemoryGB
  const customCPUMax = capacityAvailable ? Math.max(0.1, floorToStep(Math.min(32, availableCPUPerInstance), 0.1)) : 32
  const customMemoryMax = capacityAvailable ? Math.max(minimumMemoryGB, floorToStep(Math.min(256, availableMemoryPerInstanceGB), 0.5)) : 256
  const effectiveCustomCPU = Math.min(customCPUMax, Math.max(0.1, Number(customCPU)))
  const effectiveCustomMemoryGB = Math.min(customMemoryMax, Math.max(minimumMemoryGB, Number(customMemoryGB)))
  const effectiveCPUMillis = preset?.id === "custom" ? Math.round(effectiveCustomCPU * 1000) : preset?.cpu_limit_millis || 0
  const effectiveMemoryBytes = preset?.id === "custom" ? Math.round(effectiveCustomMemoryGB * 1024 ** 3) : preset?.memory_limit_bytes || 0
  const customResourcesValid = preset?.id !== "custom" || customCPUCapacityValid && customMemoryCapacityValid
  const createMutation = useMutation({
    mutationFn: () => api.createDatabaseService(applicationID, {
      name: databaseName,
      engine: engineID,
      version: selectedVersion,
      environment_ids: selectedEnvironmentIDs,
      resource_preset: preset?.id || presetID,
	  custom_cpu_millis: preset?.id === "custom" ? Math.round(effectiveCustomCPU * 1000) : undefined,
	  custom_memory_bytes: preset?.id === "custom" ? Math.round(effectiveCustomMemoryGB * 1024 ** 3) : undefined,
	  backup_enabled: backupEnabled,
	  backup_destination_id: backupEnabled ? backupDestination?.id : undefined,
      connections: consumerServiceIDs.map((serviceID) => ({ service_id: serviceID, variable_key: variableKey, replace_existing: conflictingServiceIDs.includes(serviceID) && replacementServiceIDs.includes(serviceID) })),
    }),
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) })
      navigate(`/applications/${applicationID}/services/${result.service.id}`)
    },
  })
  const setConnectionConflict = useCallback((serviceID: string, conflict: boolean) => {
    setConflictingServiceIDs((current) => conflict ? current.includes(serviceID) ? current : [...current, serviceID] : current.filter((id) => id !== serviceID))
  }, [])
  if (catalogQuery.isPending) return <section className="h-96 animate-pulse rounded-xl border bg-card" />
  if (catalogQuery.isError) return <section className="rounded-xl border bg-card p-10 text-center"><DatabaseIcon className="mx-auto text-muted-foreground" size={26} /><h2 className="mt-3 text-sm font-semibold">Database catalog could not be loaded</h2><p className="mt-1 text-xs text-muted-foreground">Retry after checking the HostForge server.</p><Button className="mt-4" variant="outline" onClick={() => catalogQuery.refetch()}>Retry</Button></section>
  if (!engine) return <section className="rounded-xl border bg-card p-10 text-center text-xs text-muted-foreground">No database engines are available.</section>
  const toggleEnvironment = (environmentID: string) => setSelectedEnvironmentIDs((current) => current.includes(environmentID) ? current.filter((id) => id !== environmentID) : [...current, environmentID])
  const toggleConsumer = (serviceID: string) => {
    setConsumerServiceIDs((current) => current.includes(serviceID) ? current.filter((id) => id !== serviceID) : [...current, serviceID])
    setReplacementServiceIDs((current) => current.filter((id) => id !== serviceID))
  }
  return <div className="space-y-5">
    <Panel title="Database engine" subtitle="HostForge controls tested versions, ports, and persistent data paths">
      <div className="grid gap-3 p-5 sm:grid-cols-2 lg:grid-cols-3">{engines.map((candidate) => { const available = candidate.versions.some((item) => item.provisioning_available); return <button key={candidate.id} type="button" disabled={!available} onClick={() => { setEngineID(candidate.id); setVersion(""); setVariableKey(candidate.connection_variable); setCustomMemoryGB((current) => String(Math.max(Number(current), candidate.minimum_memory_bytes / 1024 ** 3))) }} className={`rounded-lg border p-4 text-left transition disabled:cursor-not-allowed disabled:opacity-50 ${candidate.id === engine.id ? "border-accent bg-accent/10 ring-2 ring-accent/15" : "bg-background hover:bg-muted/40"}`}><span className="flex items-start gap-3"><DatabaseIdentity engine={candidate.id} label={candidate.name} showLabel={false} iconClassName="size-11 bg-card" imageClassName="size-8" /><span className="min-w-0 flex-1"><span className="flex items-center justify-between gap-2 text-xs font-semibold">{candidate.name}{!available && <StatusBadge tone="neutral">Next</StatusBadge>}</span><span className="mt-1 block text-[10px] font-medium text-muted-foreground">{candidate.category} · :{candidate.internal_port}</span></span></span><span className="mt-3 block text-[11px] leading-5 text-muted-foreground">{candidate.description}</span></button> })}</div>
      <div className="grid gap-5 border-t p-5 sm:grid-cols-3"><Field label="Service name"><Input value={databaseName} onChange={(event) => setDatabaseName(event.target.value)} placeholder="database" /></Field><Field label="Version"><AppSelect options={availableVersions.map((item) => item.version)} value={selectedVersion} onValueChange={setVersion} className="h-10 w-full bg-background text-xs" /></Field><Field label="Connection variable"><Input value={variableKey} onChange={(event) => setVariableKey(event.target.value.toUpperCase())} /></Field></div>
    </Panel>
    <Panel title="Environment isolation" subtitle="Each selected environment receives its own container, volume, credentials, and private network identity">
      <div className="grid gap-3 p-5 sm:grid-cols-2">{environments.map((environment) => <label key={environment.id} className="flex cursor-pointer items-center justify-between gap-4 rounded-lg border bg-background p-4"><span><span className="block text-xs font-semibold">{environment.name}</span><span className="mt-1 block text-[11px] text-muted-foreground">{environment.kind === "production" ? "Production data" : "Non-production data"} remains isolated.</span></span><Switch checked={selectedEnvironmentIDs.includes(environment.id)} onCheckedChange={() => toggleEnvironment(environment.id)} aria-label={`Create ${environment.name} database`} /></label>)}</div>
    </Panel>
    <Panel title="Resources" subtitle={`Choose limits for each ${engine.name} instance`}>
      <div className="grid gap-3 p-5 sm:grid-cols-2 lg:grid-cols-4">{presets.map((candidate) => { const custom = candidate.id === "custom"; const belowEngineMinimum = !custom && candidate.memory_limit_bytes < engine.minimum_memory_bytes; const exceedsCapacity = capacityAvailable && !custom && (candidate.cpu_limit_millis * instanceCount > capacity.cpu_available_millis || candidate.memory_limit_bytes * instanceCount > capacity.memory_available_bytes); const unavailable = belowEngineMinimum || exceedsCapacity; return <button key={candidate.id} type="button" disabled={unavailable} onClick={() => setPresetID(candidate.id)} className={`rounded-lg border p-4 text-left transition disabled:cursor-not-allowed disabled:opacity-45 ${candidate.id === preset?.id ? "border-accent bg-accent/10 ring-2 ring-accent/15" : "bg-background hover:bg-muted/40"}`}><span className="text-xs font-semibold">{candidate.name}</span><span className="mt-1 block text-[10px] font-medium text-muted-foreground">{custom ? "Set exact limits" : `${candidate.cpu_limit_millis / 1000} vCPU · ${formatMemory(candidate.memory_limit_bytes)}`}</span><span className="mt-2 block text-[11px] leading-5 text-muted-foreground">{candidate.description}</span>{belowEngineMinimum && <span className="mt-2 block text-[10px] text-destructive">Below the {formatMemory(engine.minimum_memory_bytes)} engine minimum</span>}{exceedsCapacity && <span className="mt-2 block text-[10px] text-destructive">Exceeds current host capacity for {instanceCount} instance{instanceCount === 1 ? "" : "s"}</span>}</button> })}</div>
	  {preset?.id === "custom" && <div className="grid gap-5 border-t bg-muted/10 p-5 sm:grid-cols-2"><ResourceSlider id="database-custom-cpu" label="CPU allocation" value={effectiveCustomCPU} min={0.1} max={customCPUMax} step={0.1} unit="vCPU" disabled={!customCPUCapacityValid} capacityLabel={capacityAvailable ? `${(capacity.cpu_available_millis / 1000).toFixed(1)} allocatable vCPU remains across ${instanceCount} selected environment${instanceCount === 1 ? "" : "s"}.` : "Host capacity is unavailable; the server validates this value before provisioning."} onChange={(value) => setCustomCPU(String(value))} /><ResourceSlider id="database-custom-memory" label="Memory allocation" value={effectiveCustomMemoryGB} min={minimumMemoryGB} max={customMemoryMax} step={0.5} unit="GB" disabled={!customMemoryCapacityValid} minimumLabel={`Minimum for ${engine.name}: ${formatMemory(engine.minimum_memory_bytes)}`} capacityLabel={capacityAvailable ? `${formatMemory(capacity.memory_available_bytes)} allocatable memory remains across ${instanceCount} selected environment${instanceCount === 1 ? "" : "s"}.` : "Host capacity is unavailable; the server validates this value before provisioning."} onChange={(value) => setCustomMemoryGB(String(value))} /></div>}
      {preset?.id === "custom" && capacityAvailable && (!customCPUCapacityValid || !customMemoryCapacityValid) && <div role="alert" className="border-t px-5 py-3 text-xs text-destructive">The host does not currently have enough allocatable resources for {instanceCount} {engine.name} instance{instanceCount === 1 ? "" : "s"}. Reduce the selected environments or free host capacity.</div>}
    </Panel>
    <Panel title="Application connections" subtitle="HostForge injects the private connection URL when these services are next deployed">
      {services.filter((service) => service.service_type === "application").length ? <div className="grid gap-3 p-5 sm:grid-cols-2">{services.filter((service) => service.service_type === "application").map((service) => <DatabaseWizardConnection key={service.id} applicationID={applicationID} consumer={service} environmentIDs={selectedEnvironmentIDs} variableKey={variableKey || engine.connection_variable} selected={consumerServiceIDs.includes(service.id)} replaceExisting={replacementServiceIDs.includes(service.id)} onToggle={() => toggleConsumer(service.id)} onReplacementChange={(value) => setReplacementServiceIDs((current) => value ? current.includes(service.id) ? current : [...current, service.id] : current.filter((id) => id !== service.id))} onConflictChange={setConnectionConflict} />)}</div> : <div className="p-5 text-xs text-muted-foreground">No application services are available yet. You can attach this database later.</div>}
    </Panel>
	<Panel title="Backups" subtitle="Optionally enable a daily encrypted backup policy for every selected environment">
	  {destinations.length ? <div className="grid gap-4 p-5 sm:grid-cols-[minmax(0,1fr)_auto]"><Field label="Backup destination"><AppSelect options={destinations.map((item) => item.name)} value={backupDestination?.name || ""} onValueChange={(name) => setBackupDestinationID(destinations.find((item) => item.name === name)?.id || "")} disabled={!backupEnabled} className="h-10 w-full bg-background text-xs" /></Field><label className="flex items-center gap-3 self-end rounded-lg border bg-background px-4 py-2.5"><span className="text-xs font-semibold">Daily at 02:00 UTC</span><Switch checked={backupEnabled} onCheckedChange={setBackupEnabled} aria-label="Enable daily database backups" /></label></div> : <div className="p-5"><p className="text-xs font-semibold">No backup destination connected</p><p className="mt-1 text-[11px] text-muted-foreground">You can create the database now, then connect Cloudflare R2 or generic S3 and enable backups from its detail page.</p><Button asChild className="mt-3" size="sm" variant="outline"><Link to="/settings">Connect backup storage</Link></Button></div>}
	</Panel>
    <Panel title="Review" subtitle="Host allocation and persistent resources created by this request"><div className="grid grid-cols-2 gap-px bg-border sm:grid-cols-4"><RuntimeValue label="Instances" value={String(selectedEnvironmentIDs.length)} /><RuntimeValue label="Persistent volumes" value={String(selectedEnvironmentIDs.length)} /><RuntimeValue label="Total CPU limit" value={`${(effectiveCPUMillis * selectedEnvironmentIDs.length / 1000).toFixed(1)} vCPU`} /><RuntimeValue label="Total memory limit" value={formatMemory(effectiveMemoryBytes * selectedEnvironmentIDs.length)} /></div><div className="border-t px-5 py-4 text-[11px] leading-5 text-muted-foreground">{consumerServiceIDs.length ? `${consumerServiceIDs.length} application service${consumerServiceIDs.length === 1 ? "" : "s"} will receive ${variableKey || engine.connection_variable} independently in each selected environment.` : "No application binding will be created yet."} {backupEnabled && backupDestination ? `Daily encrypted backups will use ${backupDestination.name}.` : "Remote backups can be configured later, but this database is not protected from VPS loss until then."}</div></Panel>
    <section className="rounded-xl border border-dashed bg-muted/20 p-5"><div className="flex items-start gap-3"><HardDrivesIcon className="mt-0.5 text-muted-foreground" size={19} /><div><h2 className="text-xs font-semibold">Private by default</h2><p className="mt-1 text-[11px] leading-5 text-muted-foreground">{applicationName} will receive {selectedEnvironmentIDs.length} isolated {engine.name} instance{selectedEnvironmentIDs.length === 1 ? "" : "s"}. No database port is published to the internet. Data volumes are retained for seven days after deletion.</p></div></div></section>
    {createMutation.isError && <div role="alert" className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{createMutation.error instanceof APIError ? createMutation.error.code.replaceAll("_", " ") : createMutation.error.message}</div>}
    <div className="flex justify-end gap-2"><Button type="button" variant="ghost" onClick={onBack}>Change service type</Button><Button type="button" onClick={() => createMutation.mutate()} disabled={!databaseName.trim() || !selectedVersion || !selectedEnvironmentIDs.length || !preset || !customResourcesValid || backupEnabled && !backupDestination || conflictingServiceIDs.some((id) => consumerServiceIDs.includes(id) && !replacementServiceIDs.includes(id)) || createMutation.isPending}><DatabaseIcon />{createMutation.isPending ? "Queuing database..." : "Create database"}</Button></div>
  </div>
}

function PlannedServiceType({ icon: Icon, title, description }: { icon: React.ComponentType<{ size?: number }>; title: string; description: string }) {
  return <article className="rounded-xl border border-dashed bg-muted/15 p-6">
    <div className="flex items-start justify-between gap-3"><span className="grid size-11 place-items-center rounded-lg border bg-card text-muted-foreground"><Icon size={21} /></span><StatusBadge tone="neutral">Planned</StatusBadge></div>
    <h2 className="mt-5 text-sm font-semibold">{title}</h2>
    <p className="mt-2 text-xs leading-5 text-muted-foreground">{description}</p>
    <p className="mt-5 text-[11px] font-medium text-muted-foreground">Setup wizard shell only; provisioning is not available yet.</p>
  </article>
}

export function ServiceOverview({ applicationID, service: serviceID }: { applicationID: string; service: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const serviceQuery = useQuery({
    queryKey: queryKeys.service(serviceID),
    queryFn: ({ signal }) => api.service(serviceID, signal),
    refetchInterval: (query) => query.state.data?.service.service_type === "database" && query.state.data.database_operations?.some((operation) => operation.status === "queued" || operation.status === "running") ? 2000 : false,
  })
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const environments = applicationQuery.data?.environments || []
  const deploymentsQuery = useQuery({ queryKey: queryKeys.deployments(serviceID), queryFn: ({ signal }) => api.deployments({ serviceID }, signal), enabled: serviceQuery.data?.service.service_type === "application" })
  const invalidate = async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) })
    await queryClient.invalidateQueries({ queryKey: queryKeys.deployments(serviceID) })
  }
  const deployMutation = useMutation({ mutationFn: (environmentID: string) => api.deploy(serviceID, environmentID), onSuccess: async (result) => { await invalidate(); navigate("/deployments/" + result.deployment.id) } })
  const stopMutation = useMutation({ mutationFn: (environmentID: string) => api.stopService(serviceID, environmentID), onSuccess: invalidate })
  const restartMutation = useMutation({ mutationFn: (environmentID: string) => api.restartService(serviceID, environmentID), onSuccess: invalidate })
  const runtimeMutationPending = deployMutation.isPending || stopMutation.isPending || restartMutation.isPending

  if (serviceQuery.isPending || applicationQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-10 w-56 rounded bg-muted" /><div className="mt-6 h-96 rounded-xl border bg-card" /></main>
  if (serviceQuery.isError || applicationQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Service could not be loaded</h1><Button className="mt-4" variant="outline" onClick={() => { serviceQuery.refetch(); applicationQuery.refetch() }}>Retry</Button></section></main>

  const service = serviceQuery.data.service
  if (service.service_type === "database") {
    return <DatabaseServiceOverview service={service} data={serviceQuery.data} environments={environments} />
  }
  const latest = deploymentsQuery.data?.deployments[0]
  const activeEnvironments = serviceQuery.data.environment_states.filter((item) => item.active_deployment_id)
  const state = activeEnvironments.some((item) => item.desired_state === "running") ? "Running" : activeEnvironments.length ? "Stopped" : serviceQuery.data.bindings.some((item) => item.branch) ? "Awaiting deployment" : "Configuration required"
  const base = "/applications/" + applicationID + "/services/" + serviceID
  const mutationError = deployMutation.error || stopMutation.error || restartMutation.error

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-6 flex flex-col gap-4 xl:flex-row xl:items-end"><div className="flex items-start gap-4"><StackIdentity kind={service.stack_kind} label={service.stack_label} showLabel={false} iconClassName="size-12 rounded-xl bg-accent text-accent-foreground" /><div><p className="mb-1"><StatusPill status={state} /></p><h1 className="text-3xl font-semibold tracking-[-0.035em]">{service.name}</h1><p className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span className="flex items-center gap-1"><GithubLogoIcon size={13} />{service.repo_url}</span><StackIdentity kind={service.stack_kind} label={service.stack_label} iconClassName="size-6 rounded-md" /><span>{activeEnvironments.length} active {activeEnvironments.length === 1 ? "environment" : "environments"}</span></p></div></div>
      <Button className="xl:ml-auto" variant="outline" onClick={() => navigate(base + "/settings")}>Configure environments</Button>
    </div>
    {mutationError && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{mutationError.message}</div>}
    <ServiceTabs active="Overview" serviceID={serviceID} applicationID={applicationID} />

    <section className="mb-5">
      <div className="mb-3"><h2 className="text-sm font-semibold">Environment deployments</h2><p className="mt-1 text-xs text-muted-foreground">Active releases and public URLs across this service. No environment switch is required.</p></div>
      <div className="grid gap-5 xl:grid-cols-2">{environments.map((environment) => {
        const binding = serviceQuery.data.bindings.find((item) => item.environment_id === environment.id)
        const environmentState = serviceQuery.data.environment_states.find((item) => item.environment_id === environment.id)
        const environmentStatus = binding?.desired_state === "stopped" ? "Stopped" : binding?.active_deployment_id ? "Running" : binding?.branch ? "Awaiting deployment" : "Configuration required"
        return <Panel key={environment.id} title={environment.name} subtitle={`${environment.kind} · ${binding?.branch || "No branch configured"}`} action={<StatusPill status={environmentStatus} />}>
          <div className="border-b p-5">
            {environmentState?.public_url ? <div><a href={environmentState.public_url} target="_blank" rel="noreferrer" className="group flex items-center justify-between gap-4 rounded-lg border bg-muted/25 p-4 hover:bg-muted/50"><span className="min-w-0"><span className="block text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Active deployed URL</span><span className="mt-1 block truncate font-mono text-xs font-semibold group-hover:underline">{environmentState.public_url}</span></span><ArrowSquareOutIcon className="shrink-0" size={17} /></a>{environmentState.public_url_status === "route_sync_failed" && <p className="mt-2 text-[11px] text-amber-700">The URL was generated, but route synchronization needs attention. Run Caddy route synchronization from Settings.</p>}</div> : <MissingPublicURL active={Boolean(binding?.active_deployment_id)} status={environmentState?.public_url_status} domainsHref={base + "/domains"} />}
          </div>
          <div className="grid grid-cols-2 gap-px bg-border"><RuntimeValue label="Active deployment" value={binding?.active_deployment_id || "None"} /><RuntimeValue label="Container" value={environmentState?.current_container?.status || "None"} /><RuntimeValue label="Automatic deploy" value={binding?.auto_deploy ? "Enabled" : "Disabled"} /><RuntimeValue label="Desired state" value={binding?.desired_state || "unconfigured"} /></div>
          <footer className="flex flex-wrap gap-2 border-t bg-muted/20 px-5 py-3">{binding?.active_deployment_id && <Button asChild size="sm" variant="ghost"><Link to={"/deployments/" + binding.active_deployment_id}>View deployment</Link></Button>}<span className="flex-1" />{binding?.desired_state !== "stopped" && binding?.active_deployment_id && <ConfirmationAction title={`Stop ${environment.name}?`} description="The active container will stop until it is restarted or redeployed." confirmLabel="Stop service" onConfirm={() => stopMutation.mutateAsync(environment.id)} trigger={<Button size="sm" variant="outline" disabled={runtimeMutationPending}><PauseIcon />Stop</Button>} />}{binding?.active_deployment_id && <ConfirmationAction title={`Restart ${environment.name}?`} description="Requests may be interrupted briefly while the active container returns." confirmLabel="Restart service" onConfirm={() => restartMutation.mutateAsync(environment.id)} trigger={<Button size="sm" variant="outline" disabled={runtimeMutationPending}><ActivityIcon />Restart</Button>} />}<Button size="sm" disabled={!binding?.branch || runtimeMutationPending} onClick={() => deployMutation.mutate(environment.id)}><RocketLaunchIcon weight="fill" />Deploy to {environment.name}</Button></footer>
        </Panel>
      })}</div>
    </section>

    <div className="grid gap-5 xl:grid-cols-2">
      <Panel title="Source configuration" subtitle="Repository and runtime settings"><div className="divide-y"><SourceValue icon={GithubLogoIcon} label="Repository" value={service.repo_url} /><SourceValue icon={CodeIcon} label="Runtime" value={service.runtime} /><SourceValue icon={HardDrivesIcon} label="Root directory" value={service.root_directory || "Repository root"} /><SourceValue icon={HeartbeatIcon} label="Health check" value={service.health_check_path + " on port " + service.internal_port} /></div></Panel>
      <Panel title="Latest deployment" subtitle={latest ? new Date(latest.created_at).toLocaleString() : "No deployment recorded"} action={latest ? <Link to={"/deployments/" + latest.id} className="text-xs font-medium hover:underline">View deployment</Link> : undefined}>{deploymentsQuery.isPending ? <div className="h-36 animate-pulse bg-muted/40" /> : deploymentsQuery.isError ? <PanelQueryError message="Deployment history could not be loaded." retry={() => deploymentsQuery.refetch()} /> : latest ? <div className="p-5"><div className="flex items-center gap-2"><StatusPill status={latest.status === "SUCCESS" ? "Healthy" : latest.status[0] + latest.status.slice(1).toLowerCase()} /><span className="font-mono text-xs">{latest.id}</span></div><p className="mt-4 font-mono text-xs text-muted-foreground">{latest.commit_hash || "Commit pending"}</p><p className="mt-2 text-xs text-muted-foreground">{latest.trigger || "manual"} / {latest.actor || "operator"}</p></div> : <div className="p-8 text-center text-xs text-muted-foreground">Deploy this environment to create its first release.</div>}</Panel>
    </div>
  </main>
}

function databaseStatusLabel(status: string) {
  return status.split("_").map((part) => part ? part[0].toUpperCase() + part.slice(1) : part).join(" ")
}

function metricHistoryReducer(history: DatabaseMetricDTO[], metric: DatabaseMetricDTO) {
  const next = history.at(-1)?.sampled_at === metric.sampled_at ? [...history.slice(0, -1), metric] : [...history, metric]
  return next.slice(-60)
}

function databaseMetricRates(history: DatabaseMetricDTO[], field: "network_rx_bytes" | "network_tx_bytes") {
  return history.slice(1).map((sample, index) => {
    const previous = history[index]
    const seconds = Math.max(1, (new Date(sample.sampled_at).getTime() - new Date(previous.sampled_at).getTime()) / 1000)
    return Math.max(0, sample[field] - previous[field]) / seconds
  })
}

function formatByteRate(value: number) {
  return `${formatMemory(value)}/s`
}

function MetricSparkline({ label, description, values, ceiling, formatValue }: { label: string; description: string; values: number[]; ceiling: number; formatValue: (value: number) => string }) {
  const width = 360
  const height = 96
  const safeCeiling = Math.max(1, ceiling, ...values)
  const points = values.map((value, index) => {
    const x = values.length <= 1 ? width / 2 : index / (values.length - 1) * width
    const y = height - Math.max(0, Math.min(1, value / safeCeiling)) * height
    return `${x.toFixed(1)},${y.toFixed(1)}`
  }).join(" ")
  const current = values.at(-1) || 0
  const peak = values.length ? Math.max(...values) : 0
  const lastPoint = values.length ? points.split(" ").at(-1)!.split(",") : [String(width / 2), String(height)]
  return <div className="rounded-lg border bg-muted/10 p-4">
    <div className="mb-3 flex items-end justify-between gap-4"><div><p className="text-xs font-medium text-muted-foreground">{label}</p><p className="mt-2 text-2xl font-semibold tabular-nums">{formatValue(current)}</p><p className="mt-1 text-[10px] text-muted-foreground">{description}</p></div><p className="text-[10px] text-muted-foreground">Peak {formatValue(peak)}</p></div>
    <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${label} live trend over the last ${values.length} samples`} className="h-24 w-full overflow-visible">
      {[0.25, 0.5, 0.75].map((ratio) => <line key={ratio} x1="0" x2={width} y1={height * ratio} y2={height * ratio} className="stroke-border" strokeWidth="1" strokeDasharray="3 5" />)}
      {points && <polyline points={points} fill="none" className="stroke-accent" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />}
      <circle cx={lastPoint[0]} cy={lastPoint[1]} r="4" className="fill-accent animate-pulse" />
    </svg>
    <p className="mt-2 text-[10px] text-muted-foreground">Live five-second samples · up to five minutes</p>
  </div>
}

function DatabaseInstanceDiagnostics({ instanceID, running, mode, memoryLimitBytes }: { instanceID: string; running: boolean; mode: "metrics" | "logs"; memoryLimitBytes: number }) {
  const viewport = useRef<HTMLDivElement>(null)
  const [metricHistory, appendMetric] = useReducer(metricHistoryReducer, [])
  const logsQuery = useQuery({
    queryKey: ["database-instance", instanceID, "logs"],
    queryFn: ({ signal }) => api.databaseLogs(instanceID, 200, signal),
    enabled: mode === "logs",
    refetchInterval: mode === "logs" && running ? 5000 : false,
  })
  const metricsQuery = useQuery({
    queryKey: ["database-instance", instanceID, "metrics"],
    queryFn: async ({ signal }) => {
      const result = await api.databaseMetrics(instanceID, signal)
      appendMetric(result.metric)
      return result
    },
    enabled: mode === "metrics" && running,
    refetchInterval: mode === "metrics" && running ? 5000 : false,
  })
  useEffect(() => {
    if (viewport.current) viewport.current.scrollTop = viewport.current.scrollHeight
  }, [logsQuery.data?.logs])
  const lines = logsQuery.data?.logs.trimEnd().split("\n").filter(Boolean) || []
  const metric = metricsQuery.data?.metric
  const ingressRates = databaseMetricRates(metricHistory, "network_rx_bytes")
  const egressRates = databaseMetricRates(metricHistory, "network_tx_bytes")
  if (mode === "metrics") return <div className="p-4">
    {!running ? <p className="text-[11px] text-muted-foreground">Live metrics are unavailable while this database container is stopped or failed.</p> : metricsQuery.isPending ? <div className="h-24 animate-pulse rounded-lg bg-muted" /> : metricsQuery.isError ? <p className="text-[11px] text-destructive">Database metrics could not be loaded.</p> : metric ? <div className="grid gap-4 lg:grid-cols-2"><MetricSparkline label="CPU usage" description="Percentage across available cores" values={metricHistory.map((sample) => sample.cpu_percent)} ceiling={100} formatValue={(value) => `${value.toFixed(1)}%`} /><MetricSparkline label="Memory usage" description="Container working set" values={metricHistory.map((sample) => sample.memory_bytes)} ceiling={memoryLimitBytes} formatValue={formatMemory} /><MetricSparkline label="Network ingress" description="Received bytes per second" values={ingressRates.length ? ingressRates : [0]} ceiling={0} formatValue={formatByteRate} /><MetricSparkline label="Network egress" description="Sent bytes per second" values={egressRates.length ? egressRates : [0]} ceiling={0} formatValue={formatByteRate} /></div> : <p className="text-[11px] text-muted-foreground">No database metric has been sampled yet.</p>}
  </div>
  return <div className="p-4">
    {!running && <p className="mb-3 text-[11px] text-muted-foreground">The instance is stopped or failed. Showing its retained container output.</p>}
    {logsQuery.isError && <p className="mb-3 text-[11px] text-destructive">Database logs could not be loaded.</p>}
    <div ref={viewport} role="log" aria-label="Database logs" className="h-64 overflow-y-auto rounded-lg bg-neutral-950 p-3 text-neutral-200">
      {logsQuery.isPending ? <p className="font-mono text-xs text-neutral-500">Loading database logs…</p> : lines.length ? lines.map((line, index) => <code key={`${index}-${line}`} className="block whitespace-pre-wrap break-words font-mono text-[11px] leading-5">{line}</code>) : <p className="font-mono text-xs text-neutral-500">No database output recorded.</p>}
    </div>
    <p className="mt-2 text-[10px] text-muted-foreground">Last 200 lines · refreshes every 5 seconds while running</p>
  </div>
}

function DatabaseInstanceBackups({ instanceID, serviceID, serviceName, running, defaultOpen = false, anchorID }: { instanceID: string; serviceID: string; serviceName: string; running: boolean; defaultOpen?: boolean; anchorID?: string }) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(defaultOpen)
  const destinationsQuery = useQuery({ queryKey: queryKeys.backupDestinations, queryFn: ({ signal }) => api.backupDestinations(signal), enabled: open })
  const policyQuery = useQuery({ queryKey: ["database-backup-policy", instanceID], queryFn: ({ signal }) => api.databaseBackupPolicy(instanceID, signal), enabled: open })
  const backupsQuery = useQuery({ queryKey: ["database-backups", instanceID], queryFn: ({ signal }) => api.databaseBackups(instanceID, signal), enabled: open, refetchInterval: (query) => query.state.data?.backups.some((backup) => backup.status === "queued" || backup.status === "running") ? 2000 : false })
  const [draft, setDraft] = useState<{ destination_id: string; enabled: boolean; schedule: string; timezone: string; retention_days: number } | null>(null)
  const policy = draft || policyQuery.data?.policy || { destination_id: destinationsQuery.data?.destinations[0]?.id || "", enabled: false, schedule: "0 2 * * *", timezone: "UTC", retention_days: 30 }
  const destinationNames = destinationsQuery.data?.destinations.map((item) => item.name) || []
  const selectedDestination = destinationsQuery.data?.destinations.find((item) => item.id === policy.destination_id)
  const save = useMutation({ mutationFn: () => api.saveDatabaseBackupPolicy(instanceID, policy), onSuccess: async (result) => { setDraft({ destination_id: result.policy.destination_id, enabled: result.policy.enabled, schedule: result.policy.schedule, timezone: result.policy.timezone, retention_days: result.policy.retention_days }); await queryClient.invalidateQueries({ queryKey: ["database-backup-policy", instanceID] }) } })
  const run = useMutation({ mutationFn: () => api.createDatabaseBackup(instanceID), onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["database-backups", instanceID] }), queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) })]) } })
  const restore = useMutation({ mutationFn: ({ backupID, mode }: { backupID: string; mode: "new_service" | "replace_current" }) => api.restoreDatabaseBackup(backupID, mode === "new_service" ? { mode } : { mode, target_instance_id: instanceID, confirmation: serviceName }), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }) } })
  const remove = useMutation({ mutationFn: (backupID: string) => api.deleteDatabaseBackup(backupID), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["database-backups", instanceID] }) } })
  const set = <K extends keyof typeof policy>(key: K, value: (typeof policy)[K]) => setDraft({ ...policy, [key]: value })
  return <div className="border-t" id={anchorID}>
    <button type="button" className="flex w-full items-center justify-between bg-muted/10 px-5 py-3 text-left text-xs font-semibold hover:bg-muted/30" onClick={() => setOpen((value) => !value)}><span>Backups</span><span className="text-[10px] text-muted-foreground">{open ? "Hide" : "Configure"}</span></button>
    {open && <div className="space-y-4 border-t p-4">
      {destinationsQuery.isPending || policyQuery.isPending ? <div className="h-24 animate-pulse rounded-lg bg-muted" /> : !destinationsQuery.data?.destinations.length ? <div className="rounded-lg border border-dashed p-4 text-center"><p className="text-xs font-semibold">Connect backup storage first</p><p className="mt-1 text-[11px] text-muted-foreground">Add a verified Cloudflare R2 or S3 destination in platform Settings.</p><Button asChild className="mt-3" size="sm" variant="outline"><Link to="/settings">Open backup settings</Link></Button></div> : <div className="grid gap-3 rounded-lg border bg-muted/15 p-4 sm:grid-cols-2"><Field label="Destination"><AppSelect options={destinationNames} value={selectedDestination?.name || destinationNames[0]} onValueChange={(name) => set("destination_id", destinationsQuery.data?.destinations.find((item) => item.name === name)?.id || "")} className="h-10 w-full bg-background text-xs" /></Field><Field label="Retention days"><Input type="number" min={1} max={3650} value={policy.retention_days} onChange={(event) => set("retention_days", Number(event.target.value))} /></Field><Field label="Schedule"><Input value={policy.schedule} onChange={(event) => set("schedule", event.target.value)} className="font-mono" /></Field><Field label="Timezone"><Input value={policy.timezone} onChange={(event) => set("timezone", event.target.value)} className="font-mono" /></Field><label className="flex items-center justify-between gap-4 rounded-md border bg-background px-3 py-2.5 sm:col-span-2"><span><span className="block text-xs font-semibold">Scheduled backups</span><span className="mt-0.5 block text-[10px] text-muted-foreground">Run this cron schedule in the selected timezone.</span></span><Switch checked={policy.enabled} onCheckedChange={(value) => set("enabled", value)} /></label><div className="flex flex-wrap justify-end gap-2 sm:col-span-2"><Button size="sm" variant="outline" disabled={!running || !policyQuery.data?.policy || run.isPending} onClick={() => run.mutate()}>{run.isPending ? "Queuing…" : "Back up now"}</Button><Button size="sm" disabled={!policy.destination_id || save.isPending} onClick={() => save.mutate()}>{save.isPending ? "Saving…" : "Save backup policy"}</Button></div></div>}
      {(save.isError || run.isError || restore.isError || remove.isError) && <p className="text-[11px] text-destructive">The backup operation could not be completed. Safety backups referenced by restore history cannot be removed.</p>}
      {backupsQuery.data?.backups.length ? <div className="overflow-hidden rounded-lg border">{backupsQuery.data.backups.slice(0, 5).map((backup) => <div key={backup.id} className="flex flex-wrap items-center gap-3 border-b p-3 last:border-b-0"><StatusPill status={databaseStatusLabel(backup.status)} /><div className="min-w-0 flex-1"><p className="truncate font-mono text-[10px]">{backup.object_key || backup.id}</p><p className="mt-1 text-[10px] text-muted-foreground">{backup.trigger_kind} · {backup.compressed_size ? formatMemory(backup.compressed_size) : "size pending"} · {new Date(backup.created_at).toLocaleString()}</p></div>{backup.status === "success" && <div className="flex gap-2"><Button size="sm" variant="outline" disabled={restore.isPending} onClick={() => restore.mutate({ backupID: backup.id, mode: "new_service" })}>Restore as copy</Button><ConfirmationAction title={`Replace ${serviceName} from this backup?`} description="HostForge will first create a safety backup, stop bound application containers, replace the current database contents, and roll back automatically if the restore fails." confirmLabel="Create safety backup and replace" onConfirm={() => restore.mutateAsync({ backupID: backup.id, mode: "replace_current" })} trigger={<Button size="sm" variant="destructive" disabled={!running || restore.isPending}>Replace current</Button>} /></div>}{["success", "failed", "cancelled"].includes(backup.status) && <ConfirmationAction title="Delete this backup?" description={backup.status === "success" ? "The encrypted object will be permanently removed from backup storage. Backups referenced by restore history remain protected." : "The terminal backup record will be permanently removed."} confirmLabel="Delete backup" destructive onConfirm={() => remove.mutateAsync(backup.id)} trigger={<Button size="sm" variant="ghost" aria-label={`Delete backup ${backup.id}`} disabled={remove.isPending}><TrashIcon /></Button>} />}</div>)}</div> : open && !backupsQuery.isPending && <p className="text-center text-[11px] text-muted-foreground">No backups recorded for this environment.</p>}
    </div>}
  </div>
}

function DatabaseInstanceUpgrade({ instanceID }: { instanceID: string }) {
  const queryClient = useQueryClient()
  const preflight = useQuery({ queryKey: ["database-upgrade", instanceID], queryFn: ({ signal }) => api.databaseUpgradePreflight(instanceID, signal), refetchInterval: 30_000 })
  const upgrade = useMutation({ mutationFn: () => api.upgradeDatabaseInstance(instanceID), onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["database-upgrade", instanceID] }), queryClient.invalidateQueries({ queryKey: ["service"] })]) } })
  if (preflight.isPending || preflight.isError || !preflight.data.available) return null
  const reason = preflight.data.reason === "recent_backup_required" ? `Create a successful backup within ${preflight.data.backup_max_age_hours} hours before upgrading.` : preflight.data.reason === "database_not_healthy" ? "The database must be healthy and running before upgrading." : "Upgrade preflight is not ready."
  return <div className="border-t bg-muted/15 px-5 py-4"><div className="flex flex-col gap-3 sm:flex-row sm:items-center"><div className="min-w-0 flex-1"><p className="text-[11px] font-semibold">Patch image update available</p><p className="mt-1 text-[10px] leading-4 text-muted-foreground">{preflight.data.ready ? "HostForge will replace only the container, retain this volume and network identity, verify health, and restore the previous digest automatically on failure." : reason}</p></div><ConfirmationAction title="Apply the tested patch image?" description="This causes a short database restart. A recent logical backup is required, the major engine version does not change, and the previous digest is retained for automatic rollback." confirmLabel="Upgrade and verify" onConfirm={() => upgrade.mutateAsync()} trigger={<Button size="sm" disabled={!preflight.data.ready || upgrade.isPending}>{upgrade.isPending ? "Queuing…" : "Upgrade patch"}</Button>} /></div>{upgrade.isError && <p role="alert" className="mt-2 text-[10px] text-destructive">{serviceSourceError(upgrade.error)}</p>}</div>
}

function DatabaseServiceOverview({ service, data, environments }: {
  service: ServiceDTO
  data: Awaited<ReturnType<typeof api.service>>
  environments: EnvironmentDTO[]
}) {
  const queryClient = useQueryClient()
  const location = useLocation()
  const [currentTime] = useState(() => Date.now())
  const database = data.database
  const instances = data.database_instances || []
  const operations = data.database_operations || []
  const activeOperation = operations.find((operation) => operation.status === "queued" || operation.status === "running")
  const restoreMutation = useMutation({
    mutationFn: () => api.restoreDeletedDatabase(service.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.service(service.id) }),
  })
  const runtimeMutation = useMutation({
    mutationFn: ({ instanceID, action }: { instanceID: string; action: "start" | "stop" | "restart" }) => api.databaseRuntimeAction(instanceID, action),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.service(service.id) }),
  })
  const rotateMutation = useMutation({
    mutationFn: (instanceID: string) => api.rotateDatabaseCredentials(instanceID),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.service(service.id) }),
  })
  const healthy = instances.filter((instance) => instance.status === "healthy").length
  const canRestore = instances.length > 0 && instances.every((instance) => instance.status === "deleted" && instance.purge_after && new Date(instance.purge_after).getTime() > currentTime)
  const overall = activeOperation ? databaseStatusLabel(activeOperation.status) : healthy === instances.length && instances.length ? "Healthy" : instances.some((instance) => instance.status === "failed") ? "Failed" : instances.some((instance) => instance.status === "deleted") ? "Deleted" : "Provisioning"
  const view = location.hash === "#connections" ? "connections" : location.hash === "#backups" ? "backups" : location.hash === "#metrics" ? "metrics" : location.hash === "#logs" ? "logs" : "overview"
  const activeTab = view === "connections" ? "Data & connections" : view === "backups" ? "Backups" : view === "metrics" ? "Metrics" : view === "logs" ? "Logs" : "Overview"
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <div className="mb-6 flex flex-col gap-4 xl:flex-row xl:items-end"><div className="flex items-start gap-4"><DatabaseIdentity engine={database?.engine} label={service.stack_label} showLabel={false} iconClassName="size-12 rounded-xl bg-accent/10" imageClassName="size-8" /><div><p className="mb-1"><StatusPill status={overall} /></p><h1 className="text-3xl font-semibold tracking-[-0.035em]">{service.name}</h1><p className="mt-2 text-xs text-muted-foreground">{database?.engine || "database"} {database?.default_version} · private environment networking</p></div></div><div className="flex items-center gap-2 xl:ml-auto"><StatusBadge tone="neutral">HostForge private network</StatusBadge>{canRestore && <Button size="sm" onClick={() => restoreMutation.mutate()} disabled={restoreMutation.isPending}>{restoreMutation.isPending ? "Queuing restore…" : "Restore database"}</Button>}</div></div>
    {restoreMutation.isError && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">The retained database could not be restored. Its retention window may have expired.</div>}
    <DatabaseServiceTabs active={activeTab} serviceID={service.id} applicationID={service.application_id} />
    {activeOperation && <Panel title="Provisioning database" subtitle="This operation is durable and continues if you leave the page"><div className="p-5"><div className="mb-2 flex items-center justify-between gap-4 text-xs"><span className="font-semibold">{databaseStatusLabel(activeOperation.progress_step)}</span><span className="font-mono text-muted-foreground">{activeOperation.progress_percent}%</span></div><div className="h-2 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-accent transition-all" style={{ width: `${activeOperation.progress_percent}%` }} /></div></div></Panel>}

    {view === "overview" && <section className="grid gap-5 xl:grid-cols-2" aria-label="Database instance overview">{instances.map((instance) => {
      const environment = environments.find((candidate) => candidate.id === instance.environment_id)
      const operation = operations.find((candidate) => candidate.database_instance_id === instance.id)
      const status = databaseStatusLabel(instance.status)
      return <Panel key={instance.id} title={environment?.name || "Environment"} subtitle={`${environment?.kind || "environment"} · ${instance.engine_version}`} action={<StatusPill status={status} />}>
        <div className="grid grid-cols-2 gap-px bg-border"><RuntimeValue label="Resources" value={`${instance.cpu_limit_millis / 1000} vCPU · ${formatMemory(instance.memory_limit_bytes)}`} /><RuntimeValue label="Storage" value={instance.storage_checked_at ? `${formatMemory(instance.storage_used_bytes || 0)} used` : "Usage pending"} /></div>
        {instance.health_message && <div className="border-t px-5 py-3 text-[11px] text-muted-foreground">Health: {instance.health_message.replaceAll("_", " ")}</div>}
        {operation?.status === "failed" && <div className="border-t bg-destructive/5 px-5 py-3 text-[11px] text-destructive"><strong className="block">{operation.error_code?.replaceAll("_", " ") || "Database operation failed"}</strong>{operation.error_message && <span className="mt-1 block">{operation.error_message}</span>}</div>}
        {instance.status !== "deleted" && instance.docker_container_id && <DatabaseInstanceUpgrade instanceID={instance.id} />}
      </Panel>
    })}</section>}

    {view === "connections" && <section aria-label="Database data and connections"><div className="mb-3"><h2 className="text-sm font-semibold">Data and connections</h2><p className="mt-1 text-xs text-muted-foreground">Each environment has independent credentials, storage, resources, and private connection identity.</p></div><div className="grid gap-5 xl:grid-cols-2">{instances.map((instance) => {
      const environment = environments.find((candidate) => candidate.id === instance.environment_id)
      const operation = operations.find((candidate) => candidate.database_instance_id === instance.id)
      const credential = data.database_credentials?.[instance.id]
      const runtimeBusy = runtimeMutation.isPending || rotateMutation.isPending || operation?.status === "queued" || operation?.status === "running"
      return <Panel key={instance.id} title={environment?.name || "Environment"} subtitle={`${environment?.kind || "environment"} · ${instance.engine_version}`} action={<StatusPill status={databaseStatusLabel(instance.status)} />}>
        <div className="grid grid-cols-2 gap-px bg-border lg:grid-cols-3"><RuntimeValue label="Private host" value={instance.network_alias} /><RuntimeValue label="Port" value={String(instance.internal_port)} /><RuntimeValue label="Database" value={credential?.database_name || "Unavailable"} /><RuntimeValue label="Username" value={credential?.username || "Unavailable"} /><RuntimeValue label="Persistent volume" value={instance.volume_name} /><RuntimeValue label="Resources" value={`${instance.cpu_limit_millis / 1000} vCPU · ${formatMemory(instance.memory_limit_bytes)}`} /></div>
        {credential && <div className="border-t px-5 py-3 text-[11px] text-muted-foreground">Credential generation {credential.generation}{credential.rotated_at ? ` · rotated ${new Date(credential.rotated_at).toLocaleString()}` : ""}. Passwords remain sealed and are injected only into bound services.</div>}
        {instance.status === "deleted" && instance.purge_after && <div className="border-t px-5 py-3 text-[11px] text-amber-700">Volume retained until {new Date(instance.purge_after).toLocaleString()}.</div>}
        {instance.status !== "deleted" && instance.docker_container_id && <footer className="flex flex-wrap justify-end gap-2 border-t bg-muted/20 px-5 py-3">{instance.status === "healthy" && <ConfirmationAction title={`Rotate ${environment?.name || "database"} credentials?`} description="HostForge will update the database password and encrypted connection bindings. Bound applications must be redeployed to receive the new connection URL." confirmLabel="Rotate credentials" onConfirm={() => rotateMutation.mutateAsync(instance.id)} trigger={<Button size="sm" variant="outline" disabled={runtimeBusy}>Rotate credentials</Button>} />}{instance.status === "stopped" ? <Button size="sm" disabled={runtimeBusy} onClick={() => runtimeMutation.mutate({ instanceID: instance.id, action: "start" })}>Start</Button> : <><Button size="sm" variant="outline" disabled={runtimeBusy} onClick={() => runtimeMutation.mutate({ instanceID: instance.id, action: "stop" })}>Stop</Button><Button size="sm" disabled={runtimeBusy} onClick={() => runtimeMutation.mutate({ instanceID: instance.id, action: "restart" })}>Restart</Button></>}</footer>}
      </Panel>
    })}</div><section className="mt-5 rounded-xl border border-dashed bg-muted/20 p-5"><div className="flex items-start gap-3"><GlobeIcon className="mt-0.5 text-muted-foreground" size={19} /><div><h2 className="text-xs font-semibold">Public access is disabled</h2><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Only HostForge application containers in the matching environment network can reach these instances. No database port is bound to the VPS public interfaces.</p></div></div></section></section>}

    {view === "backups" && <section className="grid gap-5 xl:grid-cols-2" aria-label="Database backups">{instances.map((instance) => { const environment = environments.find((candidate) => candidate.id === instance.environment_id); return <Panel key={instance.id} title={`${environment?.name || "Environment"} backups`} subtitle="Backup policy and restore history"><DatabaseInstanceBackups instanceID={instance.id} serviceID={service.id} serviceName={service.name} running={instance.status === "healthy"} defaultOpen /></Panel> })}</section>}

    {(view === "metrics" || view === "logs") && <section className="grid gap-5 xl:grid-cols-2" aria-label={view === "metrics" ? "Database metrics" : "Database logs"}>{instances.map((instance) => {
      const environment = environments.find((candidate) => candidate.id === instance.environment_id)
      const operation = operations.find((candidate) => candidate.database_instance_id === instance.id)
      return <Panel key={instance.id} title={`${environment?.name || "Environment"} ${view}`} subtitle={`${databaseStatusLabel(instance.status)} · ${instance.engine_version}`}>
        {instance.docker_container_id ? <DatabaseInstanceDiagnostics instanceID={instance.id} running={instance.status === "healthy" || instance.status === "starting"} mode={view} memoryLimitBytes={instance.memory_limit_bytes} /> : <div className="p-5 text-[11px] text-muted-foreground">{view === "logs" && operation?.status === "failed" ? <><p className="font-semibold text-destructive">The failed container was removed by the earlier provisioning workflow, so its raw output is no longer available.</p><p className="mt-2">{operation.error_message || operation.error_code?.replaceAll("_", " ")}</p><p className="mt-2">After this fix is deployed, failed containers are retained in a stopped state so their logs remain accessible here.</p></> : `No ${view} are available until the database container has been provisioned.`}</div>}
      </Panel>
    })}</section>}
  </main>
}

function MissingPublicURL({ active, status, domainsHref }: { active: boolean; status?: string; domainsHref: string }) {
  if (active && status === "platform_domain_required") {
    return <div className="rounded-lg border border-amber-200 bg-amber-50/60 p-4"><p className="text-xs font-semibold text-amber-950">Register the platform domain to generate this deployment URL.</p><p className="mt-1 text-[11px] leading-5 text-amber-800">After the platform hostname and wildcard DNS are verified, HostForge will automatically create a shareable URL for this environment.</p><Button asChild className="mt-3" size="sm" variant="outline"><Link to="/onboarding"><GlobeIcon />Register platform domain</Link></Button></div>
  }
  if (active && (status === "platform_state_unavailable" || status === "platform_domain_generation_failed")) {
    return <div className="rounded-lg border border-amber-200 bg-amber-50/60 p-4"><p className="text-xs font-semibold text-amber-950">HostForge could not generate the platform URL.</p><p className="mt-1 text-[11px] text-amber-800">Refresh the page, then review platform domain and Caddy status in Settings if this persists.</p><Button asChild className="mt-3" size="sm" variant="outline"><Link to="/settings"><GlobeIcon />Review platform settings</Link></Button></div>
  }
  return <div className="rounded-lg border border-dashed p-4"><p className="text-xs font-semibold">{active ? "Deployment is active, but no public domain is configured." : "No active deployment URL"}</p><Button asChild className="mt-3" size="sm" variant="outline"><Link to={domainsHref}><GlobeIcon />{active ? "Add a public domain" : "Configure domain"}</Link></Button></div>
}

function PanelQueryError({ message, retry }: { message: string; retry: () => unknown }) {
  return <div className="p-8 text-center"><p className="text-xs text-destructive">{message}</p><Button className="mt-3" size="sm" variant="outline" onClick={retry}>Retry</Button></div>
}

function RuntimeValue({ label, value }: { label: string; value: string }) {
  return <div className="bg-card p-4"><p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{label}</p><p className="mt-2 break-all font-mono text-xs font-medium">{value}</p></div>
}

function SourceValue({ icon: Icon, label, value }: { icon: React.ComponentType<{ size?: number; className?: string }>; label: string; value: string }) {
  return <div className="flex items-center gap-3 px-5 py-3.5"><Icon size={16} className="text-muted-foreground" /><div><p className="text-[10px] text-muted-foreground">{label}</p><p className="mt-0.5 break-all text-xs font-medium">{value}</p></div></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label>
}
