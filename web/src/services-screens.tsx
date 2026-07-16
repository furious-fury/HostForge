import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api, APIError, queryKeys, type ServiceDTO } from "@/api"
import { Link, useNavigate } from "react-router-dom"
import {
  ActivityIcon,
  ArrowLeftIcon,
  CodeIcon,
  CubeIcon,
  GithubLogoIcon,
  GitBranchIcon,
  GlobeIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  MagnifyingGlassIcon,
  PauseIcon,
  PlusIcon,
  RocketLaunchIcon,
} from "@phosphor-icons/react"

import { AppSelect } from "@/components/app-select"
import { RouteTabs } from "@/components/route-tabs"
import { StatusBadge } from "@/components/status-badge"
import { ConfirmationAction } from "@/components/confirmation-action"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useToast } from "@/toast-provider"
import "@/services.css"

function StatusPill({ status }: { status: string }) {
  const tone = status === "Running" || status === "Healthy" || status === "Live" ? "success" : status === "Deploying" || status === "Building" ? "info" : status === "Failed" ? "error" : "neutral"
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

function ApplicationTabs({ active, applicationID }: { active: string; applicationID: string }) {
  const tabs = ["Overview", "Services", "Deployments", "Domains", "Environment", "Activity", "Settings"]
  return <RouteTabs active={active} label="Application navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? "/applications/" + applicationID : "/applications/" + applicationID + "/" + tab.toLowerCase() }))} />
}

export function ServicesList({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const [query, setQuery] = useState("")
  const [environmentName, setEnvironmentName] = useState("Production")
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  if (applicationQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-48 rounded bg-muted" /><div className="mt-6 h-80 rounded-xl border bg-card" /></main>
  if (applicationQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Services could not be loaded</h1><Button className="mt-4" variant="outline" onClick={() => applicationQuery.refetch()}>Retry</Button></section></main>
  const { application, environments, services, service_bindings: bindings } = applicationQuery.data
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const visibleRows = services.filter((service) => [service.name, service.repo_url, service.runtime].join(" ").toLowerCase().includes(query.toLowerCase()))
  const running = services.filter((service) => bindings[service.id]?.find((item) => item.environment_id === environment?.id)?.active_deployment_id && bindings[service.id]?.find((item) => item.environment_id === environment?.id)?.desired_state === "running").length
  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <Link to={"/applications/" + applicationID} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{application.name} overview</Link>
    <div className="mb-7 flex flex-col gap-4 sm:flex-row sm:items-end"><div><h1 className="text-3xl font-semibold tracking-[-0.035em]">Services</h1><p className="mt-2 text-sm text-muted-foreground">Deployable components of {application.name}.</p></div><div className="flex gap-2 sm:ml-auto"><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-9 min-w-36 bg-card text-xs" /><Button onClick={() => navigate("/applications/" + applicationID + "/services/new")}><PlusIcon />Add service</Button></div></div>
    <ApplicationTabs active="Services" applicationID={applicationID} />
    <section className="mb-5 grid grid-cols-2 overflow-hidden rounded-xl border bg-card lg:grid-cols-4">{[{ label: "Total services", value: services.length }, { label: "Running", value: running }, { label: "Stopped", value: services.filter((service) => bindings[service.id]?.find((item) => item.environment_id === environment?.id)?.desired_state === "stopped").length }, { label: "Awaiting deploy", value: services.length - running }].map((item) => <article key={item.label} className="hf-service-summary"><p className="text-xs text-muted-foreground">{item.label}</p><p className="mt-4 text-2xl font-semibold tracking-tight">{item.value}</p><p className="mt-1 text-[11px] text-muted-foreground">{environment?.name}</p></article>)}</section>
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex flex-col gap-3 border-b bg-muted/70 p-4 sm:flex-row sm:items-center"><div><h2 className="text-sm font-semibold">All services</h2><p className="mt-0.5 text-xs text-muted-foreground">Source and release bindings</p></div><label className="relative sm:ml-auto sm:w-72"><MagnifyingGlassIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><Input value={query} onChange={(event) => setQuery(event.target.value)} className="h-9 w-full bg-card pl-9 text-xs" placeholder="Search services" /></label></header>
      {visibleRows.length ? <div className="overflow-x-auto"><Table className="w-full min-w-[880px]"><TableHeader><TableRow><TableHead>Service</TableHead><TableHead>Source</TableHead><TableHead>Runtime</TableHead><TableHead>Branch</TableHead><TableHead>Status</TableHead><TableHead>Active deployment</TableHead><TableHead>Port</TableHead></TableRow></TableHeader><TableBody>{visibleRows.map((service) => { const binding = bindings[service.id]?.find((item) => item.environment_id === environment?.id); const status = binding?.desired_state === "stopped" ? "Stopped" : binding?.active_deployment_id ? "Running" : binding?.branch ? "Awaiting deployment" : "Configuration required"; return <TableRow key={service.id}><TableCell><Link to={"/applications/" + applicationID + "/services/" + service.id} className="flex items-center gap-3 text-xs font-semibold hover:underline"><span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={17} weight="fill" /></span>{service.name}</Link></TableCell><TableCell className="max-w-64 truncate text-xs text-muted-foreground">{service.repo_url}</TableCell><TableCell className="text-xs">{service.runtime}</TableCell><TableCell className="font-mono text-xs">{binding?.branch || "Not set"}</TableCell><TableCell><StatusPill status={status} /></TableCell><TableCell className="font-mono text-xs text-muted-foreground">{binding?.active_deployment_id || "None"}</TableCell><TableCell className="font-mono text-xs">:{service.internal_port}</TableCell></TableRow> })}</TableBody></Table></div> : <div className="px-6 py-14 text-center"><CubeIcon className="mx-auto text-muted-foreground" size={24} /><p className="mt-3 text-sm font-semibold">{services.length ? "No matching services" : "No services yet"}</p><Button className="mt-4" onClick={() => navigate("/applications/" + applicationID + "/services/new")}><PlusIcon />Add service</Button></div>}
      <footer className="border-t bg-muted/30 px-5 py-3 text-[11px] text-muted-foreground">{visibleRows.length} services</footer>
    </section>
  </main>
}

export function AddService({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [installationName, setInstallationName] = useState("")
  const [repositoryName, setRepositoryName] = useState("")
  const [branch, setBranch] = useState("")
  const [environmentName, setEnvironmentName] = useState("Production")
  const [name, setName] = useState("")
  const [rootDirectory, setRootDirectory] = useState("")
  const [runtime, setRuntime] = useState("auto")
  const [installCmd, setInstallCmd] = useState("")
  const [buildCmd, setBuildCmd] = useState("")
  const [startCmd, setStartCmd] = useState("")
  const [internalPort, setInternalPort] = useState("3000")
  const [healthPath, setHealthPath] = useState("/")
  const [autoDeploy, setAutoDeploy] = useState(true)
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const installationsQuery = useQuery({ queryKey: queryKeys.githubInstallations, queryFn: ({ signal }) => api.githubInstallations(signal) })
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
  const createMutation = useMutation({
    mutationFn: async () => {
      if (!installation || !repository || !environment || !selectedBranch) throw new Error("Select an installation, repository, environment, and branch.")
      const result = await api.createService(applicationID, {
        name,
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
  if (applicationQuery.isPending || installationsQuery.isPending) return <main className="mx-auto w-full max-w-5xl animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-8 w-48 rounded bg-muted" /><div className="mt-8 h-64 rounded-xl border bg-card" /></main>
  if (applicationQuery.isError || installationsQuery.isError) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><GithubLogoIcon className="mx-auto text-muted-foreground" size={24} /><h1 className="mt-3 text-sm font-semibold">Service prerequisites could not be loaded</h1><p className="mt-1 text-xs text-muted-foreground">Check the server and GitHub App integration, then retry.</p><Button className="mt-4" variant="outline" onClick={() => { applicationQuery.refetch(); installationsQuery.refetch() }}>Retry</Button></section></main>
  if (!environments.length) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><h1 className="text-sm font-semibold">No environment is available</h1><p className="mt-1 text-xs text-muted-foreground">This application needs an environment before a service can be configured.</p><Button asChild className="mt-4" variant="outline"><Link to={"/applications/" + applicationID + "/settings"}>Open application settings</Link></Button></section></main>
  if (!installations.length) return <main className="mx-auto w-full max-w-5xl px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-10 text-center"><GithubLogoIcon className="mx-auto text-muted-foreground" size={26} /><h1 className="mt-3 text-sm font-semibold">No active GitHub installation</h1><p className="mx-auto mt-1 max-w-md text-xs leading-5 text-muted-foreground">Configure or restore a GitHub App installation, then synchronize it before adding a repository-backed service.</p><div className="mt-5 flex justify-center gap-2"><Button asChild><Link to="/onboarding">Configure GitHub App</Link></Button><Button variant="outline" onClick={() => installationsQuery.refetch()}>Check again</Button></div></section></main>
  const applicationName = applicationQuery.data?.application.name || "application"
  return <main className="mx-auto w-full max-w-5xl px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <Link to={"/applications/" + applicationID + "/services"} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />Back to services</Link>
    <div className="mb-8"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Add service</h1><p className="mt-2 text-sm text-muted-foreground">Connect a GitHub App repository to {applicationName}.</p></div>
    <form className="space-y-5" onSubmit={(event) => { event.preventDefault(); createMutation.mutate() }}>
      <Panel title="GitHub source" subtitle="Choose exactly what HostForge should deploy for the first release"><div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Installation"><AppSelect options={installations.map((item) => item.account_login)} value={installation?.account_login || installationName} onValueChange={(value) => { setInstallationName(value); setRepositoryName(""); setBranch("") }} className="h-10 w-full bg-background text-xs" /></Field><Field label="Repository"><AppSelect options={repositories.map((item) => item.full_name)} value={repository?.full_name || repositoryName} onValueChange={(value) => { setRepositoryName(value); setBranch("") }} disabled={!installation || repositoriesQuery.isPending} className="h-10 w-full bg-background text-xs" /></Field><Field label="Environment"><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} className="h-10 w-full bg-background text-xs" /></Field><Field label="Branch"><AppSelect options={branches} value={selectedBranch} onValueChange={setBranch} disabled={!repository || branchesQuery.isPending} className="h-10 w-full bg-background text-xs" /></Field></div><div className="border-t px-6 py-3 text-xs" aria-live="polite">{repositoriesQuery.isPending ? <p className="text-muted-foreground">Loading repositories from GitHub...</p> : repositoriesQuery.isError ? <p role="alert" className="flex items-center justify-between gap-3 text-destructive"><span>Repositories could not be loaded for this installation.</span><Button type="button" size="sm" variant="outline" onClick={() => repositoriesQuery.refetch()}>Retry</Button></p> : !repositories.length ? <p className="text-muted-foreground">This installation does not expose any repositories. Update its repository access on GitHub.</p> : branchesQuery.isPending ? <p className="text-muted-foreground">Loading repository branches...</p> : branchesQuery.isError ? <p role="alert" className="flex items-center justify-between gap-3 text-destructive"><span>Branches could not be loaded for this repository.</span><Button type="button" size="sm" variant="outline" onClick={() => branchesQuery.refetch()}>Retry</Button></p> : !branches.length ? <p className="text-muted-foreground">No branches are available in this repository.</p> : <p className="text-muted-foreground">Ready to deploy <span className="font-mono font-semibold text-foreground">{selectedBranch}</span> to <span className="font-semibold text-foreground">{environment?.name}</span>.</p>}</div></Panel>
      <Panel title="Build and runtime" subtitle="Railpack detects framework defaults during the first deployment"><div className="grid gap-5 p-6 sm:grid-cols-2"><Field label="Service name"><Input value={name} onChange={(event) => setName(event.target.value)} placeholder="api" /></Field><Field label="Runtime"><AppSelect options={["auto", "bun"]} value={runtime} onValueChange={setRuntime} className="h-10 w-full bg-background text-xs" /></Field><Field label="Root directory"><Input value={rootDirectory} onChange={(event) => setRootDirectory(event.target.value)} placeholder="Repository root" /></Field><Field label="Internal port"><Input type="number" min="1" max="65535" value={internalPort} onChange={(event) => setInternalPort(event.target.value)} /></Field><Field label="Install command"><Input value={installCmd} onChange={(event) => setInstallCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Build command"><Input value={buildCmd} onChange={(event) => setBuildCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Start command"><Input value={startCmd} onChange={(event) => setStartCmd(event.target.value)} placeholder="Auto-detected" /></Field><Field label="Health-check path"><Input value={healthPath} onChange={(event) => setHealthPath(event.target.value)} placeholder="/" /><span className="mt-1.5 block text-[10px] leading-4 text-muted-foreground">Use `/` for zero-config framework apps. Set a dedicated endpoint only when the application provides one.</span></Field></div></Panel>
      <Panel title="Release behavior" subtitle="The first deployment starts immediately after the service is created"><div className="p-6"><label className="flex items-center justify-between gap-5 rounded-lg border bg-muted/25 p-4"><span><span className="block text-xs font-semibold">Deploy future pushes automatically</span><span className="mt-1 block text-[11px] leading-5 text-muted-foreground">When enabled, pushes to {selectedBranch || "the selected branch"} will create new deployments for {environment?.name || "this environment"}.</span></span><Switch checked={autoDeploy} onCheckedChange={setAutoDeploy} aria-label="Deploy future pushes automatically" /></label><p className="mt-4 text-[11px] leading-5 text-muted-foreground">Railpack detects the application stack, build method, and runtime commands during the first deployment. You will be taken to live deployment details to watch detection and build progress.</p></div></Panel>
      {createMutation.isError && !(createMutation.error instanceof InitialServiceDeploymentError) && <div role="alert" className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{serviceSourceError(createMutation.error)}</div>}
      <div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={() => navigate("/applications/" + applicationID + "/services")}>Cancel</Button><Button type="submit" disabled={!name.trim() || !installation || !repository || !selectedBranch || createMutation.isPending}><RocketLaunchIcon weight="fill" />{createMutation.isPending ? "Creating and starting deployment..." : "Create and deploy"}</Button></div>
    </form>
  </main>
}

export function ServiceOverview({ applicationID, service: serviceID }: { applicationID: string; service: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [environmentName, setEnvironmentName] = useState("Production")
  const serviceQuery = useQuery({ queryKey: queryKeys.service(serviceID), queryFn: ({ signal }) => api.service(serviceID, signal) })
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const environments = applicationQuery.data?.environments || []
  const environment = environments.find((item) => item.name === environmentName) || environments[0]
  const binding = serviceQuery.data?.bindings.find((item) => item.environment_id === environment?.id)
  const environmentState = serviceQuery.data?.environment_states.find((item) => item.environment_id === environment?.id)
  const deploymentsQuery = useQuery({ queryKey: queryKeys.deployments(serviceID, environment?.id || ""), queryFn: ({ signal }) => api.deployments({ serviceID, environmentID: environment.id }, signal), enabled: Boolean(environment) })
  const domainsQuery = useQuery({ queryKey: queryKeys.domains(applicationID, environment?.id || "", serviceID), queryFn: ({ signal }) => api.domains(applicationID, environment.id, serviceID, signal), enabled: Boolean(environment) })
  const invalidate = async () => {
    await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) })
    await queryClient.invalidateQueries({ queryKey: queryKeys.deployments(serviceID, environment?.id || "") })
  }
  const deployMutation = useMutation({ mutationFn: () => api.deploy(serviceID, environment.id), onSuccess: async (result) => { await invalidate(); navigate("/deployments/" + result.deployment.id) } })
  const stopMutation = useMutation({ mutationFn: () => api.stopService(serviceID, environment.id), onSuccess: invalidate })
  const restartMutation = useMutation({ mutationFn: () => api.restartService(serviceID, environment.id), onSuccess: invalidate })
  const runtimeMutationPending = deployMutation.isPending || stopMutation.isPending || restartMutation.isPending

  if (serviceQuery.isPending || applicationQuery.isPending) return <main className="mx-auto w-full max-w-[1600px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-10 w-56 rounded bg-muted" /><div className="mt-6 h-96 rounded-xl border bg-card" /></main>
  if (serviceQuery.isError || applicationQuery.isError) return <main className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><h1 className="text-sm font-semibold">Service could not be loaded</h1><Button className="mt-4" variant="outline" onClick={() => { serviceQuery.refetch(); applicationQuery.refetch() }}>Retry</Button></section></main>

  const service = serviceQuery.data.service
  const latest = deploymentsQuery.data?.deployments[0]
  const domains = domainsQuery.data?.domains || []
  const state = binding?.desired_state === "stopped" ? "Stopped" : binding?.active_deployment_id ? "Running" : binding?.branch ? "Awaiting deployment" : "Configuration required"
  const tabs = ["Overview", "Deployments", "Logs", "Metrics", "Environment", "Domains", "Settings"]
  const base = "/applications/" + applicationID + "/services/" + serviceID
  const mutationError = deployMutation.error || stopMutation.error || restartMutation.error

  return <main className="mx-auto w-full max-w-[1600px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <Link to={"/applications/" + applicationID + "/services"} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />All services</Link>
    <div className="mb-6 flex flex-col gap-4 xl:flex-row xl:items-end"><div className="flex items-start gap-4"><span className="grid size-12 place-items-center rounded-xl bg-accent text-accent-foreground"><CubeIcon size={22} weight="fill" /></span><div><p className="mb-1"><StatusPill status={state} /></p><h1 className="text-3xl font-semibold tracking-[-0.035em]">{service.name}</h1><p className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span className="flex items-center gap-1"><GithubLogoIcon size={13} />{service.repo_url}</span><span className="flex items-center gap-1"><GitBranchIcon size={13} />{binding?.branch || "No branch selected"}</span></p></div></div>
      <div className="flex flex-wrap gap-2 xl:ml-auto"><AppSelect options={environments.map((item) => item.name)} value={environment?.name || environmentName} onValueChange={setEnvironmentName} disabled={runtimeMutationPending} className="h-9 min-w-36 bg-card text-xs" />{binding?.desired_state !== "stopped" && binding?.active_deployment_id && <ConfirmationAction title="Stop this service?" description="The active container will stop until it is restarted or redeployed." confirmLabel="Stop service" onConfirm={() => stopMutation.mutateAsync()} trigger={<Button variant="outline" disabled={runtimeMutationPending}><PauseIcon />Stop</Button>} />}{binding?.active_deployment_id && <ConfirmationAction title="Restart this service?" description="HostForge will restart the active container for this environment. Requests may be interrupted briefly while the container returns." confirmLabel="Restart service" onConfirm={() => restartMutation.mutateAsync()} trigger={<Button variant="outline" disabled={runtimeMutationPending}><ActivityIcon />Restart</Button>} />}<Button disabled={!binding?.branch || runtimeMutationPending} onClick={() => deployMutation.mutate()}><RocketLaunchIcon weight="fill" />{deployMutation.isPending ? "Deploying..." : "Deploy"}</Button></div>
    </div>
    {mutationError && <div className="mb-5 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-xs text-destructive">{mutationError.message}</div>}
    <RouteTabs active="Overview" label="Service navigation" tabs={tabs.map((tab) => ({ label: tab, href: tab === "Overview" ? base : base + "/" + tab.toLowerCase() }))} />

    <div className="grid gap-5 xl:grid-cols-2">
      <Panel title="Runtime binding" subtitle={environment?.name || "Environment"} action={<StatusPill status={state} />}><div className="grid grid-cols-2 gap-px bg-border"><RuntimeValue label="Desired state" value={binding?.desired_state || "unconfigured"} /><RuntimeValue label="Branch" value={binding?.branch || "Not selected"} /><RuntimeValue label="Active deployment" value={binding?.active_deployment_id || "None"} /><RuntimeValue label="Automatic deploy" value={binding?.auto_deploy ? "Enabled" : "Disabled"} /><RuntimeValue label="Container" value={environmentState?.current_container ? environmentState.current_container.status : "None"} /><RuntimeValue label="Public URL" value={environmentState?.public_url || "Not configured"} /></div></Panel>
      <Panel title="Source configuration" subtitle="Repository and runtime settings"><div className="divide-y"><SourceValue icon={GithubLogoIcon} label="Repository" value={service.repo_url} /><SourceValue icon={CodeIcon} label="Runtime" value={service.runtime} /><SourceValue icon={HardDrivesIcon} label="Root directory" value={service.root_directory || "Repository root"} /><SourceValue icon={HeartbeatIcon} label="Health check" value={service.health_check_path + " on port " + service.internal_port} /></div></Panel>
      <Panel title="Latest deployment" subtitle={latest ? new Date(latest.created_at).toLocaleString() : "No deployment recorded"} action={latest ? <Link to={"/deployments/" + latest.id} className="text-xs font-medium hover:underline">View deployment</Link> : undefined}>{deploymentsQuery.isPending ? <div className="h-36 animate-pulse bg-muted/40" /> : deploymentsQuery.isError ? <PanelQueryError message="Deployment history could not be loaded." retry={() => deploymentsQuery.refetch()} /> : latest ? <div className="p-5"><div className="flex items-center gap-2"><StatusPill status={latest.status === "SUCCESS" ? "Healthy" : latest.status[0] + latest.status.slice(1).toLowerCase()} /><span className="font-mono text-xs">{latest.id}</span></div><p className="mt-4 font-mono text-xs text-muted-foreground">{latest.commit_hash || "Commit pending"}</p><p className="mt-2 text-xs text-muted-foreground">{latest.trigger || "manual"} / {latest.actor || "operator"}</p></div> : <div className="p-8 text-center text-xs text-muted-foreground">Deploy this environment to create its first release.</div>}</Panel>
      <Panel title="Domains" subtitle="Routes for this environment" action={<Link to={base + "/domains"} className="text-xs font-medium hover:underline">Manage</Link>}>{domainsQuery.isPending ? <div className="h-36 animate-pulse bg-muted/40" /> : domainsQuery.isError ? <PanelQueryError message="Domain routes could not be loaded." retry={() => domainsQuery.refetch()} /> : domains.length ? <div className="divide-y">{domains.map((domain) => <div key={domain.id} className="flex items-center gap-3 px-5 py-4"><GlobeIcon size={15} className="text-muted-foreground" /><span className="text-xs font-medium">{domain.domain_name}</span><DomainStatus value={domain.ssl_status} /></div>)}</div> : <div className="p-8 text-center text-xs text-muted-foreground">No public routes configured.</div>}</Panel>
    </div>
  </main>
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

function DomainStatus({ value }: { value: string }) {
  return <span className="ml-auto text-[10px] font-medium text-muted-foreground">{value.toLowerCase()}</span>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}</label>
}
