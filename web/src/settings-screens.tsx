import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router-dom"
import {
  ArrowLeftIcon,
  CheckCircleIcon,
  GithubLogoIcon,
  GlobeIcon,
  HardDrivesIcon,
  ShieldCheckIcon,
  TrashIcon,
  WarningCircleIcon,
  WrenchIcon,
} from "@phosphor-icons/react"

import { api, APIError, queryKeys, type EnvironmentDTO, type ServiceDTO, type ServiceEnvironmentDTO } from "@/api"
import { AppSelect } from "@/components/app-select"
import { ConfirmationAction } from "@/components/confirmation-action"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useToast } from "@/toast-provider"
import { Textarea } from "@/components/ui/textarea"

function Loading() {
  return <main className="mx-auto w-full max-w-[1400px] animate-pulse px-4 py-8 sm:px-6 lg:px-8"><div className="h-9 w-48 rounded bg-muted" /><div className="mt-7 h-96 rounded-xl border bg-card" /></main>
}

function ErrorState({ retry }: { retry: () => void }) {
  return <main className="mx-auto w-full max-w-[1400px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><WarningCircleIcon className="mx-auto text-destructive" size={24} /><h1 className="mt-3 text-sm font-semibold">Settings could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">HostForge did not substitute local defaults for unavailable server data.</p><Button className="mt-4" variant="outline" onClick={retry}>Retry</Button></section></main>
}

function Page({ title, description, back, children }: { title: string; description: string; back?: { label: string; href: string }; children: React.ReactNode }) {
  return <main className="mx-auto w-full max-w-[1400px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">{back && <Link to={back.href} className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />{back.label}</Link>}<div className="mb-7"><h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p></div>{children}</main>
}

function Section({ title, description, children, footer }: { title: string; description: string; children: React.ReactNode; footer?: React.ReactNode }) {
  return <section className="overflow-hidden rounded-xl border bg-card"><header className="border-b bg-muted/75 px-5 py-4"><h2 className="text-sm font-semibold">{title}</h2><p className="mt-1 text-xs text-muted-foreground">{description}</p></header><div className="space-y-5 p-5 sm:p-6">{children}</div>{footer && <footer className="flex justify-end gap-2 border-t bg-muted/30 px-5 py-4">{footer}</footer>}</section>
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}{hint && <span className="mt-2 block text-[11px] leading-5 text-muted-foreground">{hint}</span>}</label>
}

function Row({ label, value, mono = false }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return <div className="flex flex-col justify-between gap-1 border-b px-4 py-3.5 last:border-b-0 sm:flex-row sm:gap-4"><span className="text-[11px] text-muted-foreground">{label}</span><span className={mono ? "break-all font-mono text-[11px] font-medium" : "text-[11px] font-medium"}>{value}</span></div>
}

function mutationMessage(error: unknown) {
  return error instanceof APIError ? error.message.replaceAll("_", " ") : "The server could not complete this action."
}

export function GlobalSettings() {
  const toast = useToast()
  const queryClient = useQueryClient()
  const settingsQuery = useQuery({ queryKey: queryKeys.settings, queryFn: ({ signal }) => api.settings(signal) })
  const statusQuery = useQuery({ queryKey: queryKeys.systemStatus, queryFn: ({ signal }) => api.systemStatus(signal) })
  const githubAppQuery = useQuery({ queryKey: queryKeys.githubApp, queryFn: ({ signal }) => api.githubApp(signal) })
  const installationsQuery = useQuery({ queryKey: queryKeys.githubInstallations, queryFn: ({ signal }) => api.githubInstallations(signal), enabled: githubAppQuery.data?.app.configured === true })
  const [result, setResult] = useState("")
  const [githubResult, setGithubResult] = useState("")
  const action = useMutation({
    mutationFn: api.settingsAction,
    onSuccess: async (payload, kind) => {
      setResult(kind.replaceAll("-", " ") + " completed" + (payload.detail ? ": " + String(payload.detail) : ""))
      toast(kind.replaceAll("-", " ") + " completed")
      await Promise.all([settingsQuery.refetch(), statusQuery.refetch()])
    },
  })
  const syncInstallations = useMutation({
    mutationFn: api.syncGitHubInstallations,
    onMutate: () => setGithubResult(""),
    onSuccess: async () => {
      toast("GitHub installations synchronized")
      await queryClient.invalidateQueries({ queryKey: queryKeys.githubInstallations })
    },
  })
  const disconnectGitHubApp = useMutation({
    mutationFn: api.deleteGitHubApp,
    onMutate: () => setGithubResult(""),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: queryKeys.githubRepositoriesRoot })
      queryClient.removeQueries({ queryKey: queryKeys.repositoryBranchesRoot })
      setGithubResult("GitHub App credentials and synchronized installation records were removed from HostForge.")
      toast("GitHub App disconnected")
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.githubApp }),
        queryClient.invalidateQueries({ queryKey: queryKeys.githubInstallations }),
        queryClient.invalidateQueries({ queryKey: queryKeys.onboarding }),
        queryClient.invalidateQueries({ queryKey: queryKeys.systemStatus }),
      ])
    },
  })
  if (settingsQuery.isPending || statusQuery.isPending) return <Loading />
  if (settingsQuery.isError || statusQuery.isError) return <ErrorState retry={() => { settingsQuery.refetch(); statusQuery.refetch() }} />
  const settings = settingsQuery.data
  const checks = statusQuery.data.checks
  const githubApp = githubAppQuery.data?.app

  return <Page title="Settings" description="Inspect the active control-plane configuration and run safe validation actions. Server configuration is environment-managed and read-only here.">
    <div className="grid gap-5 xl:grid-cols-2">
      <Section title="Installation" description="Build, process, authentication, and storage information returned by the server.">
        <div className="overflow-hidden rounded-lg border"><Row label="HostForge" value={settings.build.version_display} mono /><Row label="Commit" value={settings.build.commit || "development"} mono /><Row label="Go runtime" value={settings.build.go_version} mono /><Row label="Platform" value={settings.build.os + " / " + settings.build.arch} mono /><Row label="Authentication" value={settings.auth.scheme} /><Row label="Session expiry" value={settings.auth.expires_at ? new Date(settings.auth.expires_at).toLocaleString() : "API token request"} /></div>
        <div className="overflow-hidden rounded-lg border"><Row label="Data directory" value={settings.paths.data_dir} mono /><Row label="Database" value={settings.paths.db_path} mono /><Row label="Logs" value={settings.paths.logs_dir} mono /></div>
      </Section>

      <Section title="Platform health" description="Read-only dependency checks. Daemon restart controls are intentionally not exposed.">
        <div className="overflow-hidden rounded-lg border">{checks.map((check) => { const healthy = ["RUNNING", "READY"].includes(check.status); return <div key={check.id} className="flex items-start gap-3 border-b p-4 last:border-b-0"><span className="grid size-8 shrink-0 place-items-center rounded-lg border bg-muted">{healthy ? <CheckCircleIcon className="text-emerald-600" size={16} weight="fill" /> : <WarningCircleIcon className="text-amber-600" size={16} weight="fill" />}</span><div className="min-w-0"><p className="text-xs font-semibold">{check.label}</p><p className="mt-1 text-[10px] leading-4 text-muted-foreground">{check.detail || "Check completed successfully."}</p></div><StatusBadge className="ml-auto shrink-0" tone={healthy ? "success" : check.status === "SKIPPED" ? "neutral" : "warning"}>{check.status}</StatusBadge></div> })}</div>
        <Button asChild variant="outline"><Link to="/status">Open full diagnostics</Link></Button>
      </Section>

      <Section title="GitHub App" description="GitHub App is the only supported private-source authentication path.">
        {githubAppQuery.isPending ? <div className="h-20 animate-pulse rounded-lg bg-muted" /> : githubAppQuery.isError ? <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4"><p className="text-xs text-destructive">GitHub App configuration could not be loaded.</p><Button className="mt-3" size="sm" variant="outline" onClick={() => githubAppQuery.refetch()}>Retry</Button></div> : githubApp?.configured ? <div className="flex items-center gap-3 rounded-lg border p-4"><span className="grid size-9 shrink-0 place-items-center rounded-lg bg-foreground text-background"><GithubLogoIcon size={18} weight="fill" /></span><div className="min-w-0"><p className="truncate text-xs font-semibold">{githubApp.slug || `GitHub App ${githubApp.app_id}`}</p><p className="mt-1 text-[10px] text-muted-foreground">Credentials stored and webhook verification enabled</p></div><StatusBadge className="ml-auto" tone="success">Configured</StatusBadge></div> : <div className="rounded-lg border border-dashed p-6 text-center"><GithubLogoIcon className="mx-auto text-muted-foreground" size={22} /><p className="mt-3 text-sm font-semibold">GitHub App is not configured</p><p className="mt-1 text-xs text-muted-foreground">Complete setup before adding private repository services.</p></div>}
        {githubApp?.configured && (installationsQuery.isPending ? <p className="text-xs text-muted-foreground">Loading installations...</p> : installationsQuery.isError ? <p className="text-xs text-destructive">GitHub installations could not be loaded.</p> : installationsQuery.data.installations.length ? <div className="overflow-hidden rounded-lg border">{installationsQuery.data.installations.map((installation) => <div key={installation.installation_id} className="flex items-center gap-3 border-b p-4 last:border-b-0"><GithubLogoIcon size={18} /><div><p className="text-xs font-semibold">{installation.account_login}</p><p className="mt-1 font-mono text-[10px] text-muted-foreground">Installation {installation.installation_id}</p></div><StatusBadge className="ml-auto" tone={installation.suspended ? "warning" : "success"}>{installation.suspended ? "Suspended" : "Connected"}</StatusBadge></div>)}</div> : <div className="rounded-lg border border-dashed p-5 text-center"><p className="text-xs font-semibold">No installations synchronized</p><p className="mt-1 text-[10px] text-muted-foreground">Install this App on a GitHub account or organization, then synchronize.</p></div>)}
        <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={!githubApp?.configured || syncInstallations.isPending || disconnectGitHubApp.isPending} onClick={() => syncInstallations.mutate()}><GithubLogoIcon />{syncInstallations.isPending ? "Synchronizing..." : "Sync installations"}</Button><Button asChild variant="outline"><Link to="/onboarding">{githubApp?.configured ? "Review GitHub App setup" : "Configure GitHub App"}</Link></Button>{githubApp?.configured && <ConfirmationAction title="Disconnect the GitHub App from HostForge?" description="This removes locally stored App credentials and synchronized installation records. Existing services keep their repository configuration, but private-source builds and webhook deployments will fail until a GitHub App is configured again. This does not uninstall the App on GitHub." confirmLabel="Disconnect GitHub App" destructive onConfirm={() => disconnectGitHubApp.mutate()} trigger={<Button variant="destructive" disabled={disconnectGitHubApp.isPending}><TrashIcon />{disconnectGitHubApp.isPending ? "Disconnecting..." : "Disconnect"}</Button>} />}</div>
        {githubResult && <p role="status" className="rounded-md border border-emerald-200 bg-emerald-50 p-3 text-[11px] text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/20 dark:text-emerald-200">{githubResult}</p>}
        {syncInstallations.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(syncInstallations.error)}</p>}
        {disconnectGitHubApp.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(disconnectGitHubApp.error)}</p>}
      </Section>

      <Section title="Networking and Caddy" description="Detected addresses, route paths, and safe operator actions.">
        <div className="overflow-hidden rounded-lg border"><Row label="Listen address" value={settings.network.listen} mono /><Row label="Public IPv4" value={settings.dns.detected_ipv4 || "Not detected"} mono /><Row label="Detection source" value={settings.dns.detected_ipv4_source || "Unavailable"} /><Row label="Webhook path" value={settings.webhooks.base_path} mono /><Row label="Caddy root" value={settings.caddy.root_config || "Not configured"} mono /><Row label="Generated routes" value={settings.caddy.generated_path || "Not configured"} mono /></div>
        <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={action.isPending} onClick={() => action.mutate("detect-public-ipv4")}><GlobeIcon />Detect public IP</Button><Button variant="outline" disabled={action.isPending || !settings.caddy.root_config} onClick={() => action.mutate("caddy-validate")}><ShieldCheckIcon />Validate Caddy</Button><ConfirmationAction title="Synchronize Caddy routes?" description="HostForge will regenerate managed routes from current application domains, validate the root configuration, and reload Caddy only if validation succeeds." confirmLabel="Sync routes" onConfirm={() => action.mutate("caddy-sync")} trigger={<Button disabled={action.isPending || !settings.caddy.root_config}><WrenchIcon />Sync routes</Button>} /></div>
        {result && <p role="status" className="rounded-md border border-emerald-200 bg-emerald-50 p-3 text-[11px] text-emerald-800">{result}</p>}
        {action.isError && <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-[11px] text-destructive">{mutationMessage(action.error)}</p>}
      </Section>
    </div>
  </Page>
}

export function ApplicationSettings({ applicationID }: { applicationID: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const query = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const [draft, setDraft] = useState<{ name: string; description: string } | null>(null)
  const update = useMutation({
    mutationFn: (input: { name?: string; description?: string; archived?: boolean }) => api.updateApplication(applicationID, input),
    onSuccess: async () => { setDraft(null); await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }); await queryClient.invalidateQueries({ queryKey: queryKeys.applications }) },
  })
  const remove = useMutation({ mutationFn: () => api.deleteApplication(applicationID), onSuccess: async (result) => { await queryClient.invalidateQueries({ queryKey: queryKeys.applications }); if (result.routing_warning) toast(`Application deleted, but Caddy routing cleanup needs attention: ${result.routing_warning.replaceAll("_", " ")}. Run route synchronization from Settings.`, { tone: "warning", duration: 15000 }); else toast("Application deleted."); navigate("/applications", { replace: true }) } })
  if (query.isPending) return <Loading />
  if (query.isError) return <ErrorState retry={() => query.refetch()} />
  const application = query.data.application
  const form = draft || { name: application.name, description: application.description }

  return <Page title="Application settings" description={`Manage ${application.name} identity and lifecycle.`} back={{ label: application.name + " overview", href: "/applications/" + application.id }}>
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Section title="General" description="Application identity shared by every environment and service." footer={<Button disabled={update.isPending || !form.name.trim()} onClick={() => update.mutate({ name: form.name.trim(), description: form.description.trim() })}>{update.isPending ? "Saving..." : "Save changes"}</Button>}>
        <Field label="Application name"><Input value={form.name} onChange={(event) => setDraft({ ...form, name: event.target.value })} className="h-10 bg-background text-xs" /></Field>
        <Field label="Description"><Textarea value={form.description} onChange={(event) => setDraft({ ...form, description: event.target.value })} className="min-h-28 resize-none bg-background text-xs" /></Field>
        <div className="overflow-hidden rounded-lg border"><Row label="Services" value={query.data.services.length} /><Row label="Created" value={new Date(application.created_at).toLocaleString()} /></div>
        <div><p className="text-xs font-semibold">Environment labels</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Rename labels shown to operators without changing stable environment IDs or deployment targeting.</p><div className="mt-3 space-y-3">{query.data.environments.map((environment) => <EnvironmentNameEditor key={environment.id + environment.updated_at} applicationID={applicationID} environment={environment} />)}</div></div>
        {update.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(update.error)}</p>}
      </Section>
      <Section title="Lifecycle" description="Archive or permanently remove this application.">
        <div className="rounded-lg border p-4"><HardDrivesIcon size={18} /><p className="mt-3 text-xs font-semibold">{application.archived ? "Application is archived" : "Archive application"}</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Archiving removes it from active workflows without deleting deployment history.</p><Button className="mt-4" variant="outline" disabled={update.isPending} onClick={() => update.mutate({ archived: !application.archived })}>{application.archived ? "Restore application" : "Archive application"}</Button></div>
        <div className="rounded-lg border border-destructive/30 p-4"><TrashIcon className="text-destructive" size={18} /><p className="mt-3 text-xs font-semibold">Delete application</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Permanently deletes its services, environments, deployment records, variables, domains, metrics, and events.</p><ConfirmationAction title={`Delete ${application.name} permanently?`} description="This cascades through every service and environment record and cannot be undone." confirmLabel="Delete application" destructive onConfirm={() => remove.mutate()} trigger={<Button className="mt-4" variant="destructive" disabled={remove.isPending}>Delete application</Button>} /></div>
        {remove.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(remove.error)}</p>}
      </Section>
    </div>
  </Page>
}

function EnvironmentNameEditor({ applicationID, environment }: { applicationID: string; environment: EnvironmentDTO }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [name, setName] = useState(environment.name)
  const update = useMutation({
    mutationFn: () => api.updateEnvironment(applicationID, environment.id, { name: name.trim() }),
    onSuccess: async (result) => {
      setName(result.environment.name)
      toast(`${result.environment.kind === "production" ? "Production" : "Staging"} environment renamed.`)
      await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) })
    },
  })
  const dirty = Boolean(name.trim()) && name.trim() !== environment.name

  return <div className="rounded-lg border bg-muted/20 p-3"><div className="mb-2 flex items-center justify-between gap-3"><span className="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">{environment.kind}</span><StatusBadge tone="neutral">{environment.slug}</StatusBadge></div><div className="flex flex-col gap-2 sm:flex-row"><Input value={name} onChange={(event) => setName(event.target.value)} className="h-9 min-w-0 flex-1 bg-background text-xs" aria-label={`${environment.kind} environment name`} /><Button size="sm" variant="outline" disabled={!dirty || update.isPending} onClick={() => update.mutate()}>{update.isPending ? "Saving..." : "Save label"}</Button></div>{update.isError && <p role="alert" className="mt-2 text-[11px] text-destructive">{mutationMessage(update.error)}</p>}</div>
}

export function ServiceSettings({ applicationID, serviceID }: { applicationID: string; serviceID: string }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const toast = useToast()
  const query = useQuery({ queryKey: queryKeys.service(serviceID), queryFn: ({ signal }) => api.service(serviceID, signal) })
  const applicationQuery = useQuery({ queryKey: queryKeys.application(applicationID), queryFn: ({ signal }) => api.application(applicationID, signal) })
  const [draft, setDraft] = useState<Partial<ServiceDTO>>({})
  const update = useMutation({
    mutationFn: () => api.updateService(serviceID, draft),
    onSuccess: async () => { setDraft({}); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }); await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }) },
  })
  const remove = useMutation({ mutationFn: () => api.deleteService(serviceID), onSuccess: async (result) => { await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }); if (result.routing_warning) toast(`Service deleted, but Caddy routing cleanup needs attention: ${result.routing_warning.replaceAll("_", " ")}. Run route synchronization from Settings.`, { tone: "warning", duration: 15000 }); else toast("Service deleted."); navigate("/applications/" + applicationID + "/services", { replace: true }) } })
  if (query.isPending || applicationQuery.isPending) return <Loading />
  if (query.isError || applicationQuery.isError) return <ErrorState retry={() => { query.refetch(); applicationQuery.refetch() }} />
  const service = { ...query.data.service, ...draft }
  const set = <K extends keyof ServiceDTO>(key: K, value: ServiceDTO[K]) => setDraft((current) => ({ ...current, [key]: value }))

  return <Page title="Service settings" description={`Configure source, build, and runtime values for ${query.data.service.name}.`} back={{ label: query.data.service.name + " overview", href: "/applications/" + applicationID + "/services/" + serviceID }}>
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Section title="Service configuration" description="Changes apply to future deployments; active releases are not rebuilt automatically." footer={<Button disabled={update.isPending || !Object.keys(draft).length} onClick={() => update.mutate()}>{update.isPending ? "Saving..." : "Save changes"}</Button>}>
        <div className="grid gap-5 sm:grid-cols-2"><Field label="Service name"><Input value={service.name} onChange={(event) => set("name", event.target.value)} className="h-10 bg-background text-xs" /></Field><Field label="Runtime"><AppSelect options={["auto", "bun"]} value={service.runtime} onValueChange={(value) => set("runtime", value)} className="h-10 bg-background font-mono text-xs" /></Field></div>
        <div className="overflow-hidden rounded-lg border"><Row label="GitHub repository" value={service.repo_url} mono /><Row label="GitHub installation" value={service.github_installation_id} mono /></div>
        <p className="text-[11px] leading-5 text-muted-foreground">Repository identity is selected through the GitHub App during service creation and cannot be replaced with an arbitrary URL.</p>
        <Field label="Root directory"><Input value={service.root_directory} onChange={(event) => set("root_directory", event.target.value)} className="h-10 bg-background font-mono text-xs" /></Field>
        <div className="grid gap-5 sm:grid-cols-2"><Field label="Install command"><Input value={service.install_cmd} onChange={(event) => set("install_cmd", event.target.value)} className="h-10 bg-background font-mono text-xs" /></Field><Field label="Build command"><Input value={service.build_cmd} onChange={(event) => set("build_cmd", event.target.value)} className="h-10 bg-background font-mono text-xs" /></Field><Field label="Start command"><Input value={service.start_cmd} onChange={(event) => set("start_cmd", event.target.value)} className="h-10 bg-background font-mono text-xs" /></Field><Field label="Internal port"><Input type="number" value={service.internal_port} onChange={(event) => set("internal_port", Number(event.target.value))} className="h-10 bg-background font-mono text-xs" /></Field></div>
        <Field label="Health-check path"><Input value={service.health_check_path} onChange={(event) => set("health_check_path", event.target.value)} className="h-10 bg-background font-mono text-xs" /></Field>
        {update.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(update.error)}</p>}
      </Section>
      <Section title="Environment and lifecycle" description="Branches and automatic deployment are configured per environment.">
        <div className="space-y-3">{applicationQuery.data.environments.map((environment) => <BindingEditor key={environment.id + (query.data.bindings.find((item) => item.environment_id === environment.id)?.updated_at || "")} service={query.data.service} environment={environment} binding={query.data.bindings.find((item) => item.environment_id === environment.id)} />)}</div>
        <div className="rounded-lg border border-destructive/30 p-4"><TrashIcon className="text-destructive" size={18} /><p className="mt-3 text-xs font-semibold">Delete service</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Permanently removes this service and its deployments, containers, domains, variables, metrics, and events.</p><ConfirmationAction title={`Delete ${query.data.service.name} permanently?`} description="This action cannot be undone. Stop public traffic first if the service is still active." confirmLabel="Delete service" destructive onConfirm={() => remove.mutate()} trigger={<Button className="mt-4" variant="destructive" disabled={remove.isPending}>Delete service</Button>} /></div>
        {remove.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(remove.error)}</p>}
      </Section>
    </div>
  </Page>
}

function BindingEditor({ service, environment, binding }: { service: ServiceDTO; environment: EnvironmentDTO; binding?: ServiceEnvironmentDTO }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [branch, setBranch] = useState(binding?.branch || "")
  const [autoDeploy, setAutoDeploy] = useState(binding?.auto_deploy || false)
  const branchesQuery = useQuery({
    queryKey: queryKeys.repositoryBranches(service.repo_url, service.github_installation_id),
    queryFn: ({ signal }) => api.repositoryBranches(service.repo_url, service.github_installation_id, signal),
    enabled: Boolean(service.repo_url && service.github_installation_id),
  })
  const save = useMutation({
    mutationFn: () => api.updateServiceBinding(service.id, environment.id, { branch, auto_deploy: autoDeploy }),
    onSuccess: async () => { toast(`${environment.name} deployment binding saved.`); await queryClient.invalidateQueries({ queryKey: queryKeys.service(service.id) }) },
  })
  const dirty = branch !== (binding?.branch || "") || autoDeploy !== (binding?.auto_deploy || false)
  return <div className="rounded-lg border p-4"><div className="mb-4 flex items-start justify-between gap-3"><div><p className="text-xs font-semibold">{environment.name}</p><p className="mt-1 text-[10px] text-muted-foreground">{environment.kind}</p></div><StatusBadge tone={binding?.active_deployment_id ? "success" : branch ? "neutral" : "warning"}>{binding?.active_deployment_id ? "Active release" : branch ? "Configured" : "Unconfigured"}</StatusBadge></div><div className="space-y-4"><Field label="Deployment branch"><AppSelect options={branchesQuery.data?.branches || (branch ? [branch] : [])} value={branch} onValueChange={setBranch} placeholder={branchesQuery.isPending ? "Loading branches..." : "Select a branch"} disabled={branchesQuery.isPending || branchesQuery.isError} className="h-9 bg-background font-mono text-xs" /></Field><label className="flex items-center justify-between gap-4 rounded-md border bg-muted/25 px-3 py-2.5"><span><span className="block text-xs font-semibold">Automatic deployments</span><span className="mt-0.5 block text-[10px] text-muted-foreground">Deploy matching GitHub pushes to this environment.</span></span><Switch checked={autoDeploy} onCheckedChange={setAutoDeploy} disabled={!branch || save.isPending} aria-label={`Automatic deployments for ${environment.name}`} /></label>{branchesQuery.isError && <p className="text-xs text-destructive">Branches could not be loaded from the linked GitHub installation.</p>}{save.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(save.error)}</p>}<Button className="w-full" size="sm" variant="outline" disabled={!branch || !dirty || save.isPending} onClick={() => save.mutate()}>{save.isPending ? "Saving binding..." : "Save " + environment.name.toLowerCase()}</Button></div></div>
}
