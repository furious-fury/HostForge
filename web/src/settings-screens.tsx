import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router-dom"
import {
  CheckCircleIcon,
  GithubLogoIcon,
  GlobeIcon,
  HardDrivesIcon,
  ShieldCheckIcon,
  TrashIcon,
  WarningCircleIcon,
  WrenchIcon,
} from "@phosphor-icons/react"

import { api, APIError, queryKeys, type BackupDestinationDTO, type EnvironmentDTO, type ServiceDTO, type ServiceEnvironmentDTO } from "@/api"
import { AppSelect } from "@/components/app-select"
import { ApplicationTabs } from "@/components/application-tabs"
import { ServiceTabs } from "@/components/service-tabs"
import { DatabaseServiceTabs } from "@/components/database-service-tabs"
import { DatabaseExternalAccess } from "@/database-external-access"
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

function DeletingApplication({ name }: { name: string }) {
  return <main className="grid min-h-[calc(100svh-4rem)] place-items-center px-4 py-12"><section className="w-full max-w-md rounded-xl border bg-card p-8 text-center shadow-sm" role="status" aria-live="polite"><span className="mx-auto grid size-12 place-items-center rounded-xl bg-destructive/10 text-destructive"><TrashIcon className="animate-pulse" size={22} /></span><h1 className="mt-4 text-base font-semibold">Deleting {name}</h1><p className="mt-2 text-xs leading-5 text-muted-foreground">Removing services, environments, deployments, domains, and configuration. You’ll return to the applications list when deletion is complete.</p><div className="mx-auto mt-5 h-1.5 w-48 overflow-hidden rounded-full bg-muted"><span className="block h-full w-2/3 animate-pulse rounded-full bg-destructive" /></div></section></main>
}

function ErrorState({ retry }: { retry: () => void }) {
  return <main className="mx-auto w-full max-w-[1400px] px-4 py-16 sm:px-6 lg:px-8"><section className="rounded-xl border bg-card p-8 text-center"><WarningCircleIcon className="mx-auto text-destructive" size={24} /><h1 className="mt-3 text-sm font-semibold">Settings could not be loaded</h1><p className="mt-2 text-xs text-muted-foreground">HostForge did not substitute local defaults for unavailable server data.</p><Button className="mt-4" variant="outline" onClick={retry}>Retry</Button></section></main>
}

function Page({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <main className="mx-auto w-full max-w-[1400px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9"><div className="mb-7"><h1 className="text-3xl font-semibold tracking-[-0.035em]">{title}</h1><p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p></div>{children}</main>
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

type DatabaseBinding = { id: string; consumer_service_id: string; variable_key: string; replace_existing: boolean }

function DatabaseBindingControl({ applicationID, environmentID, consumer, binding, defaultVariableKey, busy, onConnect, onUpdate, onDisconnect }: {
  applicationID: string
  environmentID: string
  consumer: ServiceDTO
  binding?: DatabaseBinding
  defaultVariableKey: string
  busy: boolean
  onConnect: (variableKey: string, replaceExisting: boolean) => void
  onUpdate: (variableKey: string, replaceExisting: boolean) => void
  onDisconnect: () => void
}) {
  const [variableKey, setVariableKey] = useState(binding?.variable_key || defaultVariableKey)
  const [replaceExisting, setReplaceExisting] = useState(binding?.replace_existing || false)
  const normalizedKey = variableKey.trim().toUpperCase()
  const globalVariables = useQuery({ queryKey: queryKeys.variables(applicationID, environmentID, ""), queryFn: ({ signal }) => api.environmentVariables(applicationID, environmentID, "", signal), enabled: Boolean(normalizedKey) })
  const serviceVariables = useQuery({ queryKey: queryKeys.variables(applicationID, environmentID, consumer.id), queryFn: ({ signal }) => api.environmentVariables(applicationID, environmentID, consumer.id, signal), enabled: Boolean(normalizedKey) })
  const conflict = [...(globalVariables.data?.variables || []), ...(serviceVariables.data?.variables || [])].some((variable) => variable.key === normalizedKey)
  const dirty = Boolean(binding && normalizedKey && (normalizedKey !== binding.variable_key || replaceExisting !== binding.replace_existing))
  const confirmationMissing = conflict && !replaceExisting
  return <article className={`overflow-hidden rounded-lg border bg-background transition-colors ${conflict ? "border-amber-500/60" : binding ? "border-emerald-500/30" : ""}`}>
    <div className="flex items-start gap-3 p-4">
      <span className={`grid size-9 shrink-0 place-items-center rounded-lg ${binding ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400" : "bg-muted text-muted-foreground"}`}>
        {binding ? <CheckCircleIcon size={18} weight="fill" /> : <GlobeIcon size={18} />}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0"><h3 className="truncate text-xs font-semibold">{consumer.name}</h3><p className="mt-1 truncate text-[10px] text-muted-foreground">{consumer.stack_label || consumer.runtime} application</p></div>
          <StatusBadge tone={binding ? "success" : "neutral"}>{binding ? "Connected" : "Available"}</StatusBadge>
        </div>
        <label className="mt-4 block"><span className="mb-1.5 block text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">Injected environment variable</span><Input aria-label={`${consumer.name} connection variable`} value={variableKey} onChange={(event) => { setVariableKey(event.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "")); setReplaceExisting(false) }} className="h-9 bg-background font-mono text-xs" /></label>
      </div>
    </div>
    {conflict && <label className="flex items-start gap-3 border-t border-amber-500/30 bg-amber-500/5 px-4 py-3"><Switch checked={replaceExisting} onCheckedChange={setReplaceExisting} aria-label={`Replace existing ${normalizedKey} for ${consumer.name}`} /><span className="text-[10px] leading-4 text-amber-800 dark:text-amber-300"><strong className="block text-[11px]">Variable already exists</strong>Allow HostForge to replace {normalizedKey} for this service during deployment. Other services keep their stored value.</span></label>}
    {binding && <div className="border-t bg-emerald-500/5 px-4 py-3 text-[10px] leading-4 text-muted-foreground">The private connection URL is ready to inject. Redeploy <strong className="text-foreground">{consumer.name}</strong> after changing this key or rotating credentials.</div>}
    <footer className="flex flex-wrap items-center justify-end gap-2 border-t bg-muted/20 px-4 py-3">
      {binding ? <><Button size="sm" variant="ghost" disabled={busy} onClick={onDisconnect}>Disconnect</Button><Button size="sm" disabled={busy || !dirty || confirmationMissing} onClick={() => onUpdate(normalizedKey, replaceExisting)}>{busy ? "Saving…" : "Save changes"}</Button></> : <Button size="sm" disabled={busy || !normalizedKey || confirmationMissing} onClick={() => onConnect(normalizedKey, replaceExisting)}>{busy ? "Connecting…" : `Connect ${consumer.name}`}</Button>}
    </footer>
  </article>
}

function BackupDestinationsSettings() {
  const toast = useToast()
  const queryClient = useQueryClient()
  const query = useQuery({ queryKey: queryKeys.backupDestinations, queryFn: ({ signal }) => api.backupDestinations(signal) })
  const [provider, setProvider] = useState<"r2" | "s3">("r2")
  const [name, setName] = useState("")
  const [accountID, setAccountID] = useState("")
  const [endpoint, setEndpoint] = useState("")
  const [region, setRegion] = useState("us-east-1")
  const [bucket, setBucket] = useState("")
  const [prefix, setPrefix] = useState("hostforge")
  const [pathStyle, setPathStyle] = useState(false)
  const [serverSideEncryption, setServerSideEncryption] = useState<"" | "AES256" | "aws:kms">("")
  const [sseKMSKeyID, setSSEKMSKeyID] = useState("")
  const [accessKey, setAccessKey] = useState("")
  const [secretKey, setSecretKey] = useState("")
  const [editingID, setEditingID] = useState("")
  const clearForm = () => { setEditingID(""); setName(""); setAccountID(""); setEndpoint(""); setRegion("us-east-1"); setBucket(""); setPrefix("hostforge"); setPathStyle(false); setServerSideEncryption(""); setSSEKMSKeyID(""); setAccessKey(""); setSecretKey("") }
  const beginEdit = (destination: BackupDestinationDTO) => {
    setEditingID(destination.id); setProvider(destination.provider); setName(destination.name); setEndpoint(destination.endpoint); setRegion(destination.region); setBucket(destination.bucket); setPrefix(destination.object_prefix); setPathStyle(destination.path_style); setServerSideEncryption(destination.server_side_encryption || ""); setSSEKMSKeyID(destination.sse_kms_key_id || ""); setAccessKey(""); setSecretKey("")
    if (destination.provider === "r2") setAccountID(new URL(destination.endpoint).hostname.split(".")[0] || "")
  }
  const save = useMutation({
    mutationFn: () => editingID
      ? api.updateBackupDestination(editingID, { name, provider, account_id: accountID, endpoint, region: provider === "r2" ? "auto" : region, bucket, object_prefix: prefix, path_style: pathStyle, server_side_encryption: provider === "s3" ? serverSideEncryption : "", sse_kms_key_id: provider === "s3" && serverSideEncryption === "aws:kms" ? sseKMSKeyID : "", ...(accessKey.trim() ? { access_key_id: accessKey } : {}), ...(secretKey.trim() ? { secret_access_key: secretKey } : {}) })
      : api.createBackupDestination({ name, provider, account_id: accountID, endpoint, region: provider === "r2" ? "auto" : region, bucket, object_prefix: prefix, path_style: pathStyle, server_side_encryption: provider === "s3" ? serverSideEncryption : "", sse_kms_key_id: provider === "s3" && serverSideEncryption === "aws:kms" ? sseKMSKeyID : "", access_key_id: accessKey, secret_access_key: secretKey }),
    onSuccess: async () => {
      const edited = Boolean(editingID)
      clearForm()
      toast(edited ? "Backup destination updated and verified" : "Backup destination connected and verified")
      await queryClient.invalidateQueries({ queryKey: queryKeys.backupDestinations })
    },
  })
  const test = useMutation({ mutationFn: api.testBackupDestination, onSuccess: async () => { toast("Backup destination verified"); await queryClient.invalidateQueries({ queryKey: queryKeys.backupDestinations }) } })
  const remove = useMutation({ mutationFn: api.deleteBackupDestination, onSuccess: async () => { toast("Backup destination removed"); await queryClient.invalidateQueries({ queryKey: queryKeys.backupDestinations }) } })
  const valid = name.trim() && bucket.trim() && (editingID || (accessKey.trim() && secretKey.trim())) && (provider === "r2" ? accountID.trim() : endpoint.trim() && region.trim() && (serverSideEncryption !== "aws:kms" || sseKMSKeyID.trim()))
  return <Section title="Database backup storage" description="Connect Cloudflare R2 in one guided step or use any HTTPS S3-compatible bucket. HostForge verifies write, read, and delete access before saving encrypted credentials.">
    {query.isPending ? <div className="h-20 animate-pulse rounded-lg bg-muted" /> : query.isError ? <p className="text-xs text-destructive">Backup destinations could not be loaded.</p> : query.data.destinations.length > 0 && <div className="space-y-2">{query.data.destinations.map((destination) => <div key={destination.id} className="flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center"><span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted"><HardDrivesIcon size={17} /></span><div className="min-w-0"><p className="text-xs font-semibold">{destination.name}</p><p className="mt-1 truncate font-mono text-[10px] text-muted-foreground">{destination.provider.toUpperCase()} · {destination.bucket}/{destination.object_prefix}{destination.server_side_encryption ? ` · ${destination.server_side_encryption}` : ""}</p></div><StatusBadge className="sm:ml-auto" tone={destination.last_test_status === "success" ? "success" : "warning"}>{destination.last_test_status === "success" ? "Verified" : "Check required"}</StatusBadge><Button size="sm" variant="outline" disabled={save.isPending} onClick={() => beginEdit(destination)}>Edit</Button><Button size="sm" variant="outline" disabled={test.isPending} onClick={() => test.mutate(destination.id)}>Test</Button><ConfirmationAction title={`Remove ${destination.name}?`} description="Existing backup policies must be moved to another destination before it can be removed. Stored backup objects are not deleted." confirmLabel="Remove destination" destructive onConfirm={() => remove.mutateAsync(destination.id)} trigger={<Button size="sm" variant="destructive" disabled={remove.isPending}>Remove</Button>} /></div>)}</div>}
    <div className="rounded-lg border bg-muted/15 p-4 sm:p-5">
      <div className="mb-5 flex flex-wrap items-center gap-2"><Button size="sm" variant={provider === "r2" ? "default" : "outline"} onClick={() => setProvider("r2")}>Cloudflare R2</Button><Button size="sm" variant={provider === "s3" ? "default" : "outline"} onClick={() => setProvider("s3")}>S3 compatible</Button>{editingID && <><span className="ml-auto text-[10px] font-semibold text-muted-foreground">Editing saved destination</span><Button size="sm" variant="ghost" onClick={clearForm}>Cancel</Button></>}</div>
      <div className="grid gap-4 sm:grid-cols-2"><Field label="Connection name"><Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Production backups" /></Field><Field label="Bucket"><Input value={bucket} onChange={(event) => setBucket(event.target.value)} placeholder="hostforge-backups" /></Field>{provider === "r2" ? <><Field label="Cloudflare account ID"><Input value={accountID} onChange={(event) => setAccountID(event.target.value.trim())} className="font-mono" /></Field><Field label="R2 endpoint override" hint="Optional. Use the jurisdiction-specific HTTPS endpoint from Cloudflare when data location restrictions apply."><Input value={endpoint} onChange={(event) => setEndpoint(event.target.value)} placeholder={`https://${accountID || "account-id"}.r2.cloudflarestorage.com`} className="font-mono" /></Field></> : <><Field label="HTTPS endpoint"><Input value={endpoint} onChange={(event) => setEndpoint(event.target.value)} placeholder="https://s3.example.com" className="font-mono" /></Field><Field label="Region"><Input value={region} onChange={(event) => setRegion(event.target.value)} className="font-mono" /></Field><Field label="Provider-side encryption"><AppSelect options={["Provider default", "SSE-S3 (AES256)", "SSE-KMS"]} value={serverSideEncryption === "AES256" ? "SSE-S3 (AES256)" : serverSideEncryption === "aws:kms" ? "SSE-KMS" : "Provider default"} onValueChange={(value) => { setServerSideEncryption(value === "SSE-KMS" ? "aws:kms" : value === "SSE-S3 (AES256)" ? "AES256" : ""); if (value !== "SSE-KMS") setSSEKMSKeyID("") }} className="h-10 bg-background text-xs" /></Field>{serverSideEncryption === "aws:kms" && <Field label="KMS key ID or ARN"><Input value={sseKMSKeyID} onChange={(event) => setSSEKMSKeyID(event.target.value)} className="font-mono" /></Field>}</>}<Field label="Object prefix"><Input value={prefix} onChange={(event) => setPrefix(event.target.value)} placeholder="hostforge" className="font-mono" /></Field><Field label="Access key ID" hint={editingID ? "Leave blank to retain the encrypted key." : undefined}><Input value={accessKey} onChange={(event) => setAccessKey(event.target.value)} autoComplete="off" className="font-mono" /></Field><Field label="Secret access key" hint={editingID ? "Leave blank to retain the encrypted secret." : undefined}><Input type="password" value={secretKey} onChange={(event) => setSecretKey(event.target.value)} autoComplete="new-password" className="font-mono" /></Field>{provider === "s3" && <label className="flex items-center justify-between gap-4 rounded-md border bg-background px-3 py-2.5"><span><span className="block text-xs font-semibold">Path-style requests</span><span className="mt-0.5 block text-[10px] text-muted-foreground">Enable for providers that do not support virtual-hosted buckets.</span></span><Switch checked={pathStyle} onCheckedChange={setPathStyle} /></label>}</div>
      {save.isError && <p role="alert" className="mt-4 text-xs text-destructive">{save.error instanceof APIError && save.error.code === "backup_destination_test_failed" ? "The bucket probe failed. Verify the endpoint, bucket, credentials, and Object Read & Write permissions." : mutationMessage(save.error)}</p>}
      {(test.isError || remove.isError) && <p role="alert" className="mt-4 text-xs text-destructive">{mutationMessage(test.error || remove.error)}</p>}
      <div className="mt-5 flex justify-end"><Button disabled={!valid || save.isPending} onClick={() => save.mutate()}>{save.isPending ? "Testing connection…" : editingID ? "Test and save changes" : "Test and connect"}</Button></div>
    </div>
  </Section>
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
  const [platformDraft, setPlatformDraft] = useState("")
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
  const updatePlatformDomain = useMutation({
    mutationFn: (domain: string) => api.updatePlatformDomain(domain),
    onSuccess: async (payload) => {
      setPlatformDraft("")
      toast(`Platform domain changed to ${payload.platform_domain}`)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.settings }),
        queryClient.invalidateQueries({ queryKey: queryKeys.onboarding }),
        queryClient.invalidateQueries({ queryKey: ["services"] }),
        queryClient.invalidateQueries({ queryKey: ["deployments"] }),
      ])
    },
  })
  if (settingsQuery.isPending || statusQuery.isPending) return <Loading />
  if (settingsQuery.isError || statusQuery.isError) return <ErrorState retry={() => { settingsQuery.refetch(); statusQuery.refetch() }} />
  const settings = settingsQuery.data
  const checks = statusQuery.data.checks
  const githubApp = githubAppQuery.data?.app
  const platformDomain = settings.platform.domain
  const nextPlatformDomain = platformDraft.trim().toLowerCase()
  const platformDomainChanged = Boolean(nextPlatformDomain && nextPlatformDomain !== platformDomain)

  return <Page title="Settings" description="Manage platform identity and inspect the active control-plane configuration. Runtime environment variables remain server-managed.">
    <div className="grid gap-5 xl:grid-cols-2">
      <Section title="Platform domain" description="Control-plane address and the parent domain used for generated deployment share URLs.">
        {settings.platform.configured ? <>
          <div className="overflow-hidden rounded-lg border"><Row label="Control plane" value={<a href={"https://" + platformDomain} target="_blank" rel="noreferrer" className="hover:underline">https://{platformDomain}</a>} mono /><Row label="Deployment wildcard" value={`*.${platformDomain}`} mono /><Row label="Managed share URLs" value={settings.platform.managed_domain_count} /><Row label="Expected IPv4" value={settings.dns.detected_ipv4 || settings.dns.server_ipv4 || "Unavailable"} mono /></div>
          <div className="rounded-lg border bg-muted/20 p-4"><Field label="Change platform domain" hint="Before saving, point both the new apex and wildcard A records to this server. Existing generated URL labels are preserved beneath the new domain."><Input value={platformDraft} onChange={(event) => setPlatformDraft(event.target.value.toLowerCase())} placeholder="forge.example.com" className="h-10 bg-background font-mono text-xs" /></Field><div className="mt-3 flex justify-end"><ConfirmationAction title={`Change platform domain to ${nextPlatformDomain || "the entered hostname"}?`} description={`HostForge will verify ${nextPlatformDomain || "the new domain"} and *.${nextPlatformDomain || "the new domain"}, migrate managed share URLs, and reload Caddy. Existing custom domains are unchanged.`} confirmLabel="Change platform domain" onConfirm={() => updatePlatformDomain.mutate(nextPlatformDomain)} trigger={<Button disabled={!platformDomainChanged || updatePlatformDomain.isPending}>{updatePlatformDomain.isPending ? "Changing domain..." : "Verify DNS and change"}</Button>} /></div></div>
          {updatePlatformDomain.isError && <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-[11px] text-destructive">{platformDomainError(updatePlatformDomain.error)}</p>}
        </> : <div className="rounded-lg border border-dashed p-6 text-center"><GlobeIcon className="mx-auto text-muted-foreground" size={22} /><p className="mt-3 text-sm font-semibold">Platform domain is not configured</p><p className="mt-1 text-xs text-muted-foreground">Complete permanent ingress setup to enable control-plane HTTPS and generated deployment URLs.</p><Button asChild className="mt-4"><Link to="/onboarding">Complete setup</Link></Button></div>}
      </Section>

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

      <Section title="Deployment runtime" description="Active port allocation and health-check defaults used during release cutover.">
        <div className="overflow-hidden rounded-lg border"><Row label="Host port range" value={`${settings.network.port_start}–${settings.network.port_end}`} mono /><Row label="Default container port" value={settings.network.container_port} mono /><Row label="Health-check path" value={settings.health.path} mono /><Row label="Health timeout" value={`${settings.health.timeout_ms} ms`} /><Row label="Retries" value={settings.health.retries} /><Row label="Expected response" value={`${settings.health.expected_min}–${settings.health.expected_max}`} mono /><Row label="Automatic route sync" value={settings.caddy.domain_sync_after_mutate ? "Enabled" : "Disabled"} /></div>
      </Section>

      <div className="xl:col-span-2"><BackupDestinationsSettings /></div>

      <Section title="Security and delivery" description="Authentication, webhook, and session safeguards currently enforced by the server.">
        <div className="overflow-hidden rounded-lg border"><Row label="Session cookies" value={settings.session.cookie_secure ? "Secure HTTPS only" : "Secure flag disabled"} /><Row label="Session lifetime" value={`${settings.session.ttl_minutes} minutes`} /><Row label="Session secret" value={settings.session.session_secret_set ? "Configured" : "Missing"} /><Row label="API token" value={settings.session.api_token_set ? "Configured" : "Not configured"} /><Row label="Webhook secret" value={settings.webhooks.secret_set ? "Configured" : "Missing"} /><Row label="Webhook processing" value={settings.webhooks.async ? "Asynchronous" : "Synchronous"} /><Row label="Webhook rate limit" value={`${settings.webhooks.rate_limit_per_minute}/minute`} /></div>
      </Section>
    </div>
  </Page>
}

function platformDomainError(error: unknown) {
  if (!(error instanceof APIError)) return mutationMessage(error)
  if (error.code === "platform_dns_not_ready") return "The new apex or wildcard A record does not resolve to this server yet."
  if (error.code === "expected_public_ipv4_unavailable") return "HostForge cannot determine the server IPv4. Configure HOSTFORGE_DNS_SERVER_IPV4 first."
  if (error.code === "platform_caddy_update_failed" || error.code === "platform_routes_update_failed") return "Caddy could not apply the new platform domain. HostForge restored the previous domain and routes."
  return error.message.replaceAll("_", " ")
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
  const remove = useMutation({
    mutationFn: () => api.deleteApplication(applicationID),
    onSuccess: async (result) => {
      navigate("/applications", { replace: true })
      await queryClient.cancelQueries({ queryKey: queryKeys.application(applicationID) })
      queryClient.removeQueries({ queryKey: queryKeys.application(applicationID) })
      await queryClient.invalidateQueries({ queryKey: queryKeys.applications })
      if (result.routing_warning) toast(`Application deleted, but Caddy routing cleanup needs attention: ${result.routing_warning.replaceAll("_", " ")}. Run route synchronization from Settings.`, { tone: "warning", duration: 15000 })
      else toast("Application deleted.")
    },
  })
  if (query.isPending) return <Loading />
  if (query.isError) return <ErrorState retry={() => query.refetch()} />
  const application = query.data.application
  if (remove.isPending) return <DeletingApplication name={application.name} />
  const form = draft || { name: application.name, description: application.description }

  return <Page title="Application settings" description={`Manage ${application.name} identity and lifecycle.`}>
    <ApplicationTabs active="Settings" applicationID={applicationID} />
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
        <div className="rounded-lg border border-destructive/30 p-4"><TrashIcon className="text-destructive" size={18} /><p className="mt-3 text-xs font-semibold">Delete application</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Permanently deletes its services, environments, deployment records, variables, domains, metrics, and events.</p><ConfirmationAction title={`Delete ${application.name} permanently?`} description="This cascades through every service and environment record and cannot be undone." confirmLabel="Delete application" destructive onConfirm={() => remove.mutate()} trigger={<Button className="mt-4" variant="destructive">Delete application</Button>} /></div>
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
	const [currentTime] = useState(() => Date.now())
  const update = useMutation({
    mutationFn: () => api.updateService(serviceID, draft),
    onSuccess: async () => { setDraft({}); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }); await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }) },
  })
  const remove = useMutation({ mutationFn: () => api.deleteService(serviceID, query.data?.service.service_type === "database" ? query.data.service.name : undefined), onSuccess: async (result) => { await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }); if (result.routing_warning) toast(`Service deleted, but Caddy routing cleanup needs attention: ${result.routing_warning.replaceAll("_", " ")}. Run route synchronization from Settings.`, { tone: "warning", duration: 15000 }); else toast("Service deleted."); navigate("/applications/" + applicationID + "/services", { replace: true }) } })
	const createDatabaseBinding = useMutation({ mutationFn: ({ instanceID, consumerServiceID, variableKey, replaceExisting }: { instanceID: string; consumerServiceID: string; variableKey: string; replaceExisting: boolean }) => api.createDatabaseBinding(instanceID, { consumer_service_id: consumerServiceID, variable_key: variableKey, replace_existing: replaceExisting }), onSuccess: async () => { toast("Application connected. Redeploy it to receive the database URL."); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }) } })
	const updateDatabaseBinding = useMutation({ mutationFn: ({ bindingID, consumerServiceID, variableKey, replaceExisting }: { bindingID: string; consumerServiceID: string; variableKey: string; replaceExisting: boolean }) => api.updateDatabaseBinding(bindingID, { consumer_service_id: consumerServiceID, variable_key: variableKey, replace_existing: replaceExisting }), onSuccess: async () => { toast("Application connection updated. Redeploy it to apply the change."); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }) } })
	const deleteDatabaseBinding = useMutation({ mutationFn: (bindingID: string) => api.deleteDatabaseBinding(bindingID), onSuccess: async () => { toast("Application disconnected from this database."); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }) } })
	const rotateDatabaseCredentials = useMutation({ mutationFn: (instanceID: string) => api.rotateDatabaseCredentials(instanceID), onSuccess: async () => { toast("Credential rotation queued."); await queryClient.invalidateQueries({ queryKey: queryKeys.service(serviceID) }) } })
	const purgeDatabase = useMutation({ mutationFn: (name: string) => api.purgeDatabaseService(serviceID, name), onSuccess: async () => { toast("Database service and retained volumes permanently purged."); await queryClient.invalidateQueries({ queryKey: queryKeys.application(applicationID) }); navigate(`/applications/${applicationID}/services`, { replace: true }) } })
  if (query.isPending || applicationQuery.isPending) return <Loading />
  if (query.isError || applicationQuery.isError) return <ErrorState retry={() => { query.refetch(); applicationQuery.refetch() }} />
	if (query.data.service.service_type === "database") {
		const database = query.data.database
		const instances = query.data.database_instances || []
		const applicationServices = applicationQuery.data.services.filter((item) => item.service_type === "application")
		const defaultVariableKey = database?.engine === "redis" ? "REDIS_URL" : database?.engine === "valkey" ? "VALKEY_URL" : "DATABASE_URL"
		const allDeleted = instances.length > 0 && instances.every((instance) => instance.status === "deleted")
		const purgeDates = instances.map((instance) => instance.purge_after ? new Date(instance.purge_after) : null).filter((value): value is Date => value !== null)
		const purgeReady = allDeleted && purgeDates.length === instances.length && purgeDates.every((value) => value.getTime() <= currentTime)
		const nextPurgeDate = purgeDates.length ? new Date(Math.max(...purgeDates.map((value) => value.getTime()))) : null
		const bindingBusy = createDatabaseBinding.isPending || updateDatabaseBinding.isPending || deleteDatabaseBinding.isPending
		return <Page title="Database settings" description={`Persistent-resource configuration and lifecycle controls for ${query.data.service.name}.`}>
			<DatabaseServiceTabs active="Settings" serviceID={serviceID} applicationID={applicationID} />
			<div className="mb-5">
				<Section title="Application connections" description="Choose which applications receive this database's private connection URL. Connections are isolated by environment.">
					{applicationServices.length ? <div className="space-y-5">{instances.map((instance) => {
						const environment = applicationQuery.data.environments.find((item) => item.id === instance.environment_id)
						const bindings = query.data.database_bindings?.[instance.id] || []
						return <section key={instance.id} className="overflow-hidden rounded-xl border bg-muted/10">
							<header className="flex flex-col gap-3 border-b bg-muted/30 px-4 py-3 sm:flex-row sm:items-center">
								<div><div className="flex items-center gap-2"><h3 className="text-xs font-semibold">{environment?.name || "Environment"}</h3><StatusBadge tone={instance.status === "healthy" ? "success" : "neutral"}>{instance.status}</StatusBadge></div><p className="mt-1 font-mono text-[10px] text-muted-foreground">{instance.network_alias}:{instance.internal_port}</p></div>
								<p className="text-[10px] text-muted-foreground sm:ml-auto"><strong className="text-foreground">{bindings.length}</strong> of {applicationServices.length} connected</p>
							</header>
							<div className="grid gap-3 p-3 lg:grid-cols-2">{applicationServices.map((consumer) => {
								const binding = bindings.find((item) => item.consumer_service_id === consumer.id)
								return <DatabaseBindingControl key={consumer.id} applicationID={applicationID} environmentID={instance.environment_id} consumer={consumer} binding={binding} defaultVariableKey={defaultVariableKey} busy={bindingBusy} onConnect={(variableKey, replaceExisting) => createDatabaseBinding.mutate({ instanceID: instance.id, consumerServiceID: consumer.id, variableKey, replaceExisting })} onUpdate={(variableKey, replaceExisting) => binding && updateDatabaseBinding.mutate({ bindingID: binding.id, consumerServiceID: consumer.id, variableKey, replaceExisting })} onDisconnect={() => binding && deleteDatabaseBinding.mutate(binding.id)} />
							})}</div>
						</section>
					})}</div> : <div className="rounded-xl border border-dashed bg-muted/15 px-5 py-8 text-center"><GlobeIcon className="mx-auto text-muted-foreground" size={24} /><h3 className="mt-3 text-xs font-semibold">No application services yet</h3><p className="mx-auto mt-1 max-w-md text-[11px] leading-5 text-muted-foreground">Add an application service first, then return here to inject this database's private connection URL.</p><Button asChild className="mt-4" size="sm"><Link to={`/applications/${applicationID}/services/new`}>Add application service</Link></Button></div>}
					{(createDatabaseBinding.isError || updateDatabaseBinding.isError || deleteDatabaseBinding.isError) && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">{mutationMessage(createDatabaseBinding.error || updateDatabaseBinding.error || deleteDatabaseBinding.error)}</p>}
				</Section>
			</div>
			<div className="mb-5">
				<DatabaseExternalAccess engine={database?.engine || "unknown"} instances={instances} environments={applicationQuery.data.environments} />
			</div>
			<div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
				<Section title="Managed database" description="Engine images and major versions are selected from HostForge's tested, digest-pinned catalog.">
					<div className="overflow-hidden rounded-lg border"><Row label="Engine" value={database?.engine || "database"} mono /><Row label="Pinned version" value={database?.default_version || "unknown"} mono /><Row label="Network access" value="HostForge services in the same environment only" /></div>
					<div className="space-y-3">{instances.map((instance) => { const environment = applicationQuery.data.environments.find((item) => item.id === instance.environment_id); return <div key={instance.id} className="rounded-lg border bg-muted/20 p-4"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs font-semibold">{environment?.name || "Environment"}</p><p className="mt-1 font-mono text-[10px] text-muted-foreground">{instance.network_alias}:{instance.internal_port}</p></div>{instance.status === "healthy" && <ConfirmationAction title={`Rotate ${environment?.name || "database"} credentials?`} description="HostForge changes the sealed application password. Redeploy bound services afterward so they receive the new connection URL." confirmLabel="Rotate credentials" onConfirm={() => rotateDatabaseCredentials.mutateAsync(instance.id)} trigger={<Button size="sm" variant="outline" disabled={rotateDatabaseCredentials.isPending}>Rotate credentials</Button>} />}</div><div className="mt-3 grid gap-2 text-[11px] sm:grid-cols-2"><span>{instance.cpu_limit_millis / 1000} vCPU</span><span>{(instance.memory_limit_bytes / 1024 ** 3).toFixed(1)} GB memory</span><span className="truncate font-mono" title={instance.volume_name}>{instance.volume_name}</span><span>{instance.resource_preset} preset</span></div></div> })}</div>
					{rotateDatabaseCredentials.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(rotateDatabaseCredentials.error)}</p>}
				</Section>
				<Section title="Lifecycle" description="Manage retained data independently from private application bindings and public gateway credentials.">
					<div className="rounded-lg border p-4"><p className="text-xs font-semibold">Access paths are separate</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Internal application credentials are injected only into bound HostForge services. Public external credentials are managed and revoked in the gateway section above.</p></div>
					{allDeleted ? <div className="rounded-lg border border-destructive/30 p-4"><TrashIcon className="text-destructive" size={18} /><p className="mt-3 text-xs font-semibold">Retained data volumes</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">{purgeReady ? "The seven-day recovery window has ended. Permanent purge removes every retained volume and database record." : `Recovery remains available until ${nextPurgeDate?.toLocaleString() || "the retention deadline"}. HostForge purges expired volumes automatically.`}</p>{purgeReady && <ConfirmationAction title={`Purge ${query.data.service.name} permanently?`} description="Every retained database volume and record will be removed and cannot be recovered." confirmLabel="Purge permanently" confirmationText={query.data.service.name} destructive onConfirm={() => purgeDatabase.mutateAsync(query.data.service.name)} trigger={<Button className="mt-4" variant="destructive" disabled={purgeDatabase.isPending}>Purge permanently</Button>} />}</div> : <div className="rounded-lg border border-destructive/30 p-4"><TrashIcon className="text-destructive" size={18} /><p className="mt-3 text-xs font-semibold">Delete database service</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Containers stop immediately. Labelled data volumes remain recoverable for seven days before permanent purge.</p><ConfirmationAction title={`Delete ${query.data.service.name}?`} description="HostForge will stop every database instance and begin the seven-day retained-volume recovery window." confirmLabel="Delete and retain volumes" confirmationText={query.data.service.name} destructive onConfirm={() => remove.mutate()} trigger={<Button className="mt-4" variant="destructive" disabled={remove.isPending}>Delete database</Button>} /></div>}
					{(remove.isError || purgeDatabase.isError) && <p role="alert" className="text-xs text-destructive">{mutationMessage(remove.error || purgeDatabase.error)}</p>}
				</Section>
			</div>
		</Page>
	}
  const service = { ...query.data.service, ...draft }
  const set = <K extends keyof ServiceDTO>(key: K, value: ServiceDTO[K]) => setDraft((current) => ({ ...current, [key]: value }))

  return <Page title="Service settings" description={`Configure source, build, and runtime values for ${query.data.service.name}.`}>
    <ServiceTabs active="Settings" serviceID={serviceID} applicationID={applicationID} />
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
