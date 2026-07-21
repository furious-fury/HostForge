import { useEffect, useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  ArrowClockwiseIcon,
  CheckCircleIcon,
  ClipboardIcon,
  EyeIcon,
  GlobeHemisphereWestIcon,
  KeyIcon,
  LockKeyIcon,
  PauseIcon,
  PlusIcon,
  ShieldCheckIcon,
  TrashIcon,
  WarningCircleIcon,
} from "@phosphor-icons/react"

import {
  api,
  APIError,
  queryKeys,
  type DatabaseExternalConnectionDTO,
  type DatabaseExternalCredentialRevealDTO,
  type DatabaseGatewayOperationDTO,
  type DatabaseInstanceDTO,
  type EnvironmentDTO,
} from "@/api"
import { AppSelect } from "@/components/app-select"
import { ConfirmationAction } from "@/components/confirmation-action"
import { StatusBadge, type StatusTone } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useToast } from "@/toast-provider"

type Props = {
  engine: string
  instances: DatabaseInstanceDTO[]
  environments: EnvironmentDTO[]
}

const profileOptions = [
  { value: "read_only", label: "Read only" },
  { value: "read_write", label: "Read and write" },
  { value: "migration", label: "Migration (owner-equivalent)" },
] as const

function mutationMessage(error: unknown) {
  if (!(error instanceof APIError)) return "The server could not complete this action."
  const messages: Record<string, string> = {
    database_gateways_disabled: "Database gateways are disabled on this HostForge installation.",
    external_access_engine_unsupported: "A public gateway adapter is not available for this database engine yet.",
    database_gateway_platform_domain_required: "Configure the HostForge platform domain before creating the gateway.",
    database_gateway_dns_mismatch: "The reserved PostgreSQL hostname does not point to this server yet.",
    database_gateway_port_occupied: "TCP port 5432 is already occupied by another process.",
    database_gateway_tls_unavailable: "A valid TLS certificate for the reserved hostname is not available yet.",
    invalid_external_access_cidr: "Enter at least one valid IPv4 or IPv6 CIDR.",
    external_access_open_confirmation_required: "Confirm the open-network warning before allowing access from every address.",
    invalid_external_access_profile: "Choose one of the fixed permission profiles.",
    invalid_external_access_expiry: "The expiry must be a future date and time.",
    invalid_external_connection_state: "This action is not available in the connection's current state.",
    external_connection_credentials_unavailable: "Credentials are not available until the connection becomes active.",
    database_gateway_has_active_connections: "Revoke every external connection before tearing down the gateway.",
  }
  return messages[error.code] || error.message.replaceAll("_", " ")
}

function operationLabel(operation: DatabaseGatewayOperationDTO) {
  const label = operation.operation_type.replaceAll("_", " ")
  return label.charAt(0).toUpperCase() + label.slice(1)
}

function OperationWatcher({ id, onFinished }: { id: string; onFinished: () => void }) {
  const toast = useToast()
  const announced = useRef("")
  const query = useQuery({
    queryKey: queryKeys.databaseGatewayOperation(id),
    queryFn: ({ signal }) => api.databaseGatewayOperation(id, signal),
    refetchInterval: (current) => {
      const status = current.state.data?.operation.status
      return status === "success" || status === "failed" ? false : 1200
    },
  })
  const operation = query.data?.operation
  useEffect(() => {
    if (!operation || (operation.status !== "success" && operation.status !== "failed") || announced.current === operation.status) return
    announced.current = operation.status
    toast(operation.status === "success" ? `${operationLabel(operation)} completed.` : `${operationLabel(operation)} failed: ${operation.error_code?.replaceAll("_", " ") || "check the gateway status"}`, { tone: operation.status === "failed" ? "error" : "default", duration: 7000 })
    onFinished()
  }, [onFinished, operation, toast])
  if (!operation || operation.status === "success") return null
  return <div className={`rounded-lg border px-3 py-2.5 text-[11px] ${operation.status === "failed" ? "border-destructive/30 bg-destructive/5 text-destructive" : "bg-muted/25 text-muted-foreground"}`} role="status">
    <div className="flex items-center justify-between gap-3"><span className="font-semibold capitalize">{operation.progress_step.replaceAll("_", " ") || operationLabel(operation)}</span><span>{operation.progress_percent}%</span></div>
    {operation.status !== "failed" && <div className="mt-2 h-1 overflow-hidden rounded-full bg-muted"><span className="block h-full rounded-full bg-foreground transition-all" style={{ width: `${Math.max(3, operation.progress_percent)}%` }} /></div>}
  </div>
}

function formatDate(value?: string) {
  if (!value) return "Never"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "Unknown" : date.toLocaleString()
}

function graceCountdown(value?: string) {
  if (!value) return "deadline unavailable"
  const remaining = new Date(value).getTime() - Date.now()
  if (!Number.isFinite(remaining) || remaining <= 0) return "grace ending now"
  const minutes = Math.ceil(remaining / 60_000)
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"} remaining`
  const hours = Math.ceil(minutes / 60)
  if (hours < 48) return `${hours} hour${hours === 1 ? "" : "s"} remaining`
  return `${Math.ceil(hours / 24)} days remaining`
}

function connectionTone(status: DatabaseExternalConnectionDTO["status"]): StatusTone {
  if (status === "active") return "success"
  if (status === "failed" || status === "revoked" || status === "expired") return "error"
  if (status === "pending" || status === "rotating" || status === "revoking") return "warning"
  return "neutral"
}

function parseCIDRs(value: string) {
  return [...new Set(value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean))]
}

function ConnectionForm({
  instanceID,
  currentIP,
  connection,
  onQueued,
}: {
  instanceID: string
  currentIP?: string
  connection?: DatabaseExternalConnectionDTO
  onQueued: (id: string) => void
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(connection?.name || "")
  const [profile, setProfile] = useState<DatabaseExternalConnectionDTO["permission_profile"]>(connection?.permission_profile || "read_only")
  const [cidrText, setCIDRText] = useState(connection?.cidrs.join("\n") || "")
  const [expiry, setExpiry] = useState(() => connection?.expires_at ? new Date(connection.expires_at).toISOString().slice(0, 16) : "")
  const [openConfirmation, setOpenConfirmation] = useState("")
  const cidrs = parseCIDRs(cidrText)
  const hasOpenCIDR = cidrs.some((cidr) => cidr === "0.0.0.0/0" || cidr === "::/0")
  const valid = Boolean(name.trim() && cidrs.length && (!hasOpenCIDR || openConfirmation === "ALLOW PUBLIC ACCESS"))
  const save = useMutation({
    mutationFn: () => {
      const input = {
        name: name.trim(),
        profile,
        cidrs,
        ...(expiry ? { expires_at: new Date(expiry).toISOString() } : connection ? { expires_at: "" } : {}),
        confirm_open_access: hasOpenCIDR && openConfirmation === "ALLOW PUBLIC ACCESS",
      }
      return connection ? api.updateDatabaseExternalConnection(connection.id, input) : api.createDatabaseExternalConnection(instanceID, input)
    },
    onSuccess: async (result) => {
      setOpen(false)
      onQueued(result.operation.id)
      toast(connection ? "External connection update queued." : "External connection creation queued.")
      await queryClient.invalidateQueries({ queryKey: queryKeys.databaseExternalAccess(instanceID) })
    },
  })
  const changeOpen = (next: boolean) => {
    setOpen(next)
    if (!next) setOpenConfirmation("")
  }
  return <Dialog open={open} onOpenChange={changeOpen}>
    <DialogTrigger asChild>{connection ? <Button size="sm" variant="outline">Edit</Button> : <Button size="sm"><PlusIcon size={14} /> Add external connection</Button>}</DialogTrigger>
    <DialogContent className="max-h-[90svh] overflow-y-auto">
      <DialogHeader><DialogTitle>{connection ? "Edit external connection" : "Create external connection"}</DialogTitle><DialogDescription>Access is scoped to this database environment. HostForge generates a dedicated PostgreSQL role and requires TLS.</DialogDescription></DialogHeader>
      <div className="space-y-4">
        <label className="block"><span className="mb-1.5 block text-xs font-semibold">Connection name</span><Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Developer laptop" autoFocus /></label>
        <label className="block"><span className="mb-1.5 block text-xs font-semibold">Permission profile</span><AppSelect options={profileOptions} value={profile} onValueChange={(value) => setProfile(value as DatabaseExternalConnectionDTO["permission_profile"])} className="w-full" /></label>
        {profile === "migration" && <div className="rounded-lg border border-amber-500/35 bg-amber-500/5 p-3 text-[11px] leading-5 text-amber-800 dark:text-amber-300"><strong>Owner-equivalent access.</strong> This profile can create, alter, and drop application objects. It is intended only for schema migration tools.</div>}
        <label className="block"><span className="mb-1.5 block text-xs font-semibold">Allowed source CIDRs</span><Textarea value={cidrText} onChange={(event) => setCIDRText(event.target.value)} placeholder="203.0.113.24/32" className="min-h-24 font-mono text-xs" /><span className="mt-1.5 block text-[10px] leading-4 text-muted-foreground">Enter IPv4 or IPv6 networks separated by spaces, commas, or new lines.</span></label>
        {currentIP && <Button type="button" size="sm" variant="ghost" onClick={() => setCIDRText((value) => [...parseCIDRs(value), currentIP].filter((item, index, all) => all.indexOf(item) === index).join("\n"))}><GlobeHemisphereWestIcon size={14} /> Use this browser IP ({currentIP})</Button>}
        {cidrs.length > 0 && <div className="flex flex-wrap gap-1.5">{cidrs.map((cidr) => <span key={cidr} className="rounded-md border bg-muted px-2 py-1 font-mono text-[10px]">{cidr}</span>)}</div>}
        {hasOpenCIDR && <label className="block rounded-lg border border-destructive/35 bg-destructive/5 p-3 text-[11px] leading-5 text-destructive"><strong className="block">This allows connection attempts from the entire internet.</strong>Type <span className="font-mono">ALLOW PUBLIC ACCESS</span> to continue.<Input className="mt-2 bg-background font-mono" value={openConfirmation} onChange={(event) => setOpenConfirmation(event.target.value)} autoComplete="off" /></label>}
        <label className="block"><span className="mb-1.5 block text-xs font-semibold">Expiry <span className="font-normal text-muted-foreground">(optional)</span></span><Input type="datetime-local" value={expiry} min={new Date().toISOString().slice(0, 16)} onChange={(event) => setExpiry(event.target.value)} /></label>
        {save.isError && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">{mutationMessage(save.error)}</p>}
      </div>
      <DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button><Button disabled={!valid || save.isPending} onClick={() => save.mutate()}>{save.isPending ? "Queuing…" : connection ? "Save changes" : "Create connection"}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
}

function RotateAction({ connection, onQueued }: { connection: DatabaseExternalConnectionDTO; onQueued: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const [hours, setHours] = useState(24)
  const mutation = useMutation({
    mutationFn: () => api.databaseExternalConnectionAction(connection.id, "rotate", { grace_period_hours: hours }),
    onSuccess: (result) => { setOpen(false); onQueued(result.operation.id) },
  })
  return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><Button size="sm" variant="outline"><ArrowClockwiseIcon size={14} /> Rotate</Button></DialogTrigger><DialogContent><DialogHeader><DialogTitle>Rotate {connection.name}?</DialogTitle><DialogDescription>HostForge creates generation {connection.current_generation + 1} first. The current credentials continue working during the grace period and are then revoked precisely.</DialogDescription></DialogHeader><label className="block"><span className="mb-1.5 block text-xs font-semibold">Grace period (0–168 hours)</span><Input type="number" min={0} max={168} value={hours} onChange={(event) => setHours(Math.min(168, Math.max(0, Number(event.target.value))))} /></label>{mutation.isError && <p role="alert" className="text-xs text-destructive">{mutationMessage(mutation.error)}</p>}<DialogFooter><Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button><Button disabled={mutation.isPending} onClick={() => mutation.mutate()}>{mutation.isPending ? "Queuing…" : "Rotate credentials"}</Button></DialogFooter></DialogContent></Dialog>
}

function CredentialDialog({ connection }: { connection: DatabaseExternalConnectionDTO }) {
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [credential, setCredential] = useState<DatabaseExternalCredentialRevealDTO>()
  const reveal = useMutation({ mutationFn: () => api.revealDatabaseExternalCredentials(connection.id), onSuccess: (result) => { setCredential(result); setOpen(true) } })
  const close = (next: boolean) => { setOpen(next); if (!next) setCredential(undefined) }
  const copy = async (value: string, label: string) => { await navigator.clipboard.writeText(value); toast(`${label} copied.`) }
  return <><Button size="sm" variant="outline" disabled={reveal.isPending || (connection.status !== "active" && connection.status !== "rotating")} onClick={() => reveal.mutate()}><EyeIcon size={14} /> {reveal.isPending ? "Revealing…" : "Reveal"}</Button><Dialog open={open} onOpenChange={close}><DialogContent><DialogHeader><DialogTitle>Connection credentials</DialogTitle><DialogDescription>Generation {credential?.generation}. Store this secret securely; this response is never cached by HostForge.</DialogDescription></DialogHeader>{credential && <div className="space-y-3"><SecretRow label="Username" value={credential.username} onCopy={() => copy(credential.username, "Username")} /><SecretRow label="Password" value={credential.password} onCopy={() => copy(credential.password, "Password")} /><SecretRow label="Database alias" value={credential.database_alias} onCopy={() => copy(credential.database_alias, "Database alias")} /><SecretRow label="PostgreSQL URL" value={credential.url} onCopy={() => copy(credential.url, "Connection URL")} /></div>}<DialogFooter><Button onClick={() => close(false)}>Done</Button></DialogFooter></DialogContent></Dialog>{reveal.isError && <span className="basis-full text-[10px] text-destructive">{mutationMessage(reveal.error)}</span>}</>
}

function SecretRow({ label, value, onCopy }: { label: string; value: string; onCopy: () => void }) {
  return <div className="overflow-hidden rounded-lg border"><div className="flex items-center justify-between border-b bg-muted/35 px-3 py-2"><span className="text-[10px] font-semibold uppercase tracking-[0.1em] text-muted-foreground">{label}</span><Button size="sm" variant="ghost" onClick={onCopy}><ClipboardIcon size={13} /> Copy</Button></div><p className="break-all px-3 py-3 font-mono text-[11px]">{value}</p></div>
}

function ConnectionCard({ connection, instanceID, currentIP, disabled, onQueued }: { connection: DatabaseExternalConnectionDTO; instanceID: string; currentIP?: string; disabled?: boolean; onQueued: (id: string) => void }) {
  const action = useMutation({
    mutationFn: ({ kind, input }: { kind: "disable" | "enable" | "revoke"; input?: { confirmation?: string } }) => api.databaseExternalConnectionAction(connection.id, kind, input),
    onSuccess: (result) => onQueued(result.operation.id),
  })
  const graceCredential = connection.credentials?.find((credential) => credential.state === "grace")
  return <article className="overflow-hidden rounded-xl border bg-background">
    <header className="flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-start"><span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted"><KeyIcon size={17} /></span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h4 className="text-xs font-semibold">{connection.name}</h4><StatusBadge tone={connectionTone(connection.status)} dot>{connection.status}</StatusBadge>{connection.permission_profile === "migration" && <StatusBadge tone="warning">Owner-equivalent</StatusBadge>}</div><p className="mt-1 text-[10px] text-muted-foreground">Generation {connection.current_generation} · {connection.client_connection_limit} client connections maximum</p></div></header>
    <div className="grid gap-3 p-4 text-[10px] sm:grid-cols-2"><div><span className="font-semibold text-muted-foreground">Permission</span><p className="mt-1 capitalize">{connection.permission_profile.replaceAll("_", " ")}</p></div><div><span className="font-semibold text-muted-foreground">Approximate last use</span><p className="mt-1">{formatDate(connection.last_used_at)}</p></div><div><span className="font-semibold text-muted-foreground">Expires</span><p className="mt-1">{formatDate(connection.expires_at)}</p></div><div><span className="font-semibold text-muted-foreground">Allowed networks</span><div className="mt-1 flex flex-wrap gap-1">{connection.cidrs.map((cidr) => <span key={cidr} className="rounded border bg-muted px-1.5 py-0.5 font-mono">{cidr}</span>)}</div></div>{graceCredential && <div className="rounded-md border border-amber-500/30 bg-amber-500/5 p-2 sm:col-span-2"><strong>Previous generation grace:</strong> ends {formatDate(graceCredential.grace_deadline)}</div>}{connection.last_error_code && <div className="rounded-md border border-destructive/30 bg-destructive/5 p-2 text-destructive sm:col-span-2">{connection.last_error_code.replaceAll("_", " ")}</div>}</div>
    {graceCredential && <p className="border-t border-amber-500/20 bg-amber-500/5 px-4 py-2 text-[10px] text-amber-800 dark:text-amber-300"><strong>Grace countdown:</strong> {graceCountdown(graceCredential.grace_deadline)}</p>}
    {!disabled && connection.status !== "revoked" && <footer className="flex flex-wrap items-center justify-end gap-2 border-t bg-muted/20 px-4 py-3"><CredentialDialog connection={connection} /><ConnectionForm instanceID={instanceID} currentIP={currentIP} connection={connection} onQueued={onQueued} />{connection.status === "active" || connection.status === "rotating" ? <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate({ kind: "disable" })}><PauseIcon size={14} /> Disable</Button> : connection.status === "disabled" || connection.status === "expired" || connection.status === "failed" ? <Button size="sm" variant="outline" disabled={action.isPending} onClick={() => action.mutate({ kind: "enable" })}><CheckCircleIcon size={14} /> {connection.status === "failed" ? "Retry public access" : "Enable"}</Button> : null}<RotateAction connection={connection} onQueued={onQueued} /><ConfirmationAction title={`Revoke ${connection.name} permanently?`} description="New access is denied immediately. HostForge terminates only this connection's sessions, removes its grants and roles, and erases the encrypted credential material." confirmLabel="Revoke permanently" confirmationText="REVOKE EXTERNAL CONNECTION" destructive onConfirm={() => action.mutate({ kind: "revoke", input: { confirmation: "REVOKE EXTERNAL CONNECTION" } })} trigger={<Button size="sm" variant="destructive" disabled={action.isPending}><TrashIcon size={14} /> Revoke</Button>} /></footer>}
    {action.isError && <p role="alert" className="border-t px-4 py-3 text-[10px] text-destructive">{mutationMessage(action.error)}</p>}
  </article>
}

function InstanceExternalAccess({ instance, environment, enabled }: { instance: DatabaseInstanceDTO; environment?: EnvironmentDTO; enabled: boolean }) {
  const queryClient = useQueryClient()
  const [operationID, setOperationID] = useState("")
  const accessQuery = useQuery({ queryKey: queryKeys.databaseExternalAccess(instance.id), queryFn: ({ signal }) => api.databaseExternalAccess(instance.id, signal), refetchInterval: operationID ? 1500 : false })
  const finish = () => { setOperationID(""); void queryClient.invalidateQueries({ queryKey: queryKeys.databaseExternalAccess(instance.id) }); void queryClient.invalidateQueries({ queryKey: queryKeys.databaseGateway("postgresql") }) }
  const access = accessQuery.data?.external_access
  return <section className="overflow-hidden rounded-xl border bg-muted/10">
    <header className="flex flex-col gap-3 border-b bg-muted/35 px-4 py-3 sm:flex-row sm:items-center"><div><div className="flex flex-wrap items-center gap-2"><h3 className="text-xs font-semibold">{environment?.name || "Environment"}</h3><StatusBadge tone={environment?.kind === "production" ? "success" : "info"}>{environment?.kind || "environment"}</StatusBadge><StatusBadge tone={instance.status === "healthy" ? "success" : "neutral"}>{instance.status}</StatusBadge></div><p className="mt-1 text-[10px] text-muted-foreground">Public credentials for this environment cannot reach another database instance.</p></div>{access?.route && <span className="font-mono text-[10px] text-muted-foreground sm:ml-auto">{access.route.route_alias}</span>}</header>
    <div className="space-y-3 p-3 sm:p-4">{operationID && <OperationWatcher id={operationID} onFinished={finish} />}{accessQuery.isPending ? <div className="h-20 animate-pulse rounded-lg bg-muted" /> : accessQuery.isError ? <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">External access state could not be loaded.</p> : <>{access?.connections.length ? access.connections.map((connection) => <ConnectionCard key={connection.id} connection={connection} instanceID={instance.id} currentIP={accessQuery.data?.client_ip} disabled={!enabled} onQueued={setOperationID} />) : <div className="rounded-lg border border-dashed bg-background px-5 py-7 text-center"><LockKeyIcon className="mx-auto text-muted-foreground" size={22} /><h4 className="mt-3 text-xs font-semibold">No public credentials</h4><p className="mx-auto mt-1 max-w-md text-[11px] leading-5 text-muted-foreground">Public access stays off until you create a named grant with an explicit permission profile and source network.</p></div>}{enabled && <div className="flex justify-end"><ConnectionForm instanceID={instance.id} currentIP={accessQuery.data?.client_ip} onQueued={setOperationID} /></div>}</>}</div>
    {!enabled && <div className="border-t bg-amber-500/5 px-4 py-3 text-[10px] text-amber-800 dark:text-amber-300">The gateway feature flag is disabled. Existing healthy data-plane connections are unaffected, but changes are unavailable.</div>}
  </section>
}

export function DatabaseExternalAccess({ engine, instances, environments }: Props) {
  const queryClient = useQueryClient()
  const [operationID, setOperationID] = useState("")
  const gatewayQuery = useQuery({ queryKey: queryKeys.databaseGateway(engine), queryFn: ({ signal }) => api.databaseGateway(engine, signal), refetchInterval: operationID ? 1500 : false })
  const gateway = gatewayQuery.data?.gateway
  const teardown = useMutation({ mutationFn: () => api.teardownDatabaseGateway(engine, "TEAR DOWN POSTGRESQL GATEWAY"), onSuccess: (result) => setOperationID(result.operation.id) })
  const finish = () => { setOperationID(""); void queryClient.invalidateQueries({ queryKey: queryKeys.databaseGateway(engine) }); for (const instance of instances) void queryClient.invalidateQueries({ queryKey: queryKeys.databaseExternalAccess(instance.id) }) }
  const unsupported = gatewayQuery.data && !gatewayQuery.data.adapter_available
  return <section className="overflow-hidden rounded-xl border bg-card">
    <header className="border-b bg-muted/75 px-5 py-4"><div className="flex flex-col gap-3 sm:flex-row sm:items-start"><span className="grid size-10 shrink-0 place-items-center rounded-xl bg-background text-foreground shadow-sm"><GlobeHemisphereWestIcon size={20} /></span><div><h2 className="text-sm font-semibold">External database access</h2><p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">Connect developer tools and services outside HostForge through a TLS-only, CIDR-restricted gateway. This is separate from internal application bindings.</p></div>{gatewayQuery.data && <StatusBadge className="sm:ml-auto" tone={unsupported ? "neutral" : gateway?.observed_status === "active" ? "success" : gatewayQuery.data.feature_enabled ? "warning" : "neutral"}>{unsupported ? "Adapter unavailable" : gateway?.observed_status || (gatewayQuery.data.feature_enabled ? "Not provisioned" : "Feature disabled")}</StatusBadge>}</div></header>
    <div className="space-y-5 p-5 sm:p-6">{gatewayQuery.isPending ? <div className="h-28 animate-pulse rounded-xl bg-muted" /> : gatewayQuery.isError ? <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">Gateway state could not be loaded.</p> : unsupported ? <div className="rounded-xl border border-dashed bg-muted/15 p-5"><WarningCircleIcon size={20} className="text-muted-foreground" /><h3 className="mt-3 text-xs font-semibold">Multi-engine foundation ready</h3><p className="mt-1 max-w-xl text-[11px] leading-5 text-muted-foreground">HostForge has the shared orchestration model, but the {engine} protocol adapter is not available yet. No unusable public-access form is shown.</p></div> : <><div className="overflow-hidden rounded-xl border bg-background"><div className="flex flex-col gap-4 p-4 sm:flex-row sm:items-start"><span className="grid size-9 shrink-0 place-items-center rounded-lg bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"><ShieldCheckIcon size={18} /></span><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><h3 className="text-xs font-semibold">PostgreSQL gateway</h3><StatusBadge tone="info">TLS 1.2+</StatusBadge><StatusBadge tone="neutral">Session pooling</StatusBadge></div><p className="mt-1 break-all font-mono text-[11px]">{gatewayQuery.data?.reserved_hostname || "Configure a platform domain"}:5432</p><p className="mt-2 text-[10px] leading-4 text-muted-foreground">Ensure <strong className="font-mono text-foreground">{gatewayQuery.data?.reserved_hostname || "postgres.<platform-domain>"}</strong>{gatewayQuery.data?.expected_ipv4 ? <> resolves to <strong className="font-mono text-foreground">{gatewayQuery.data.expected_ipv4}</strong></> : " resolves to this server"}. Your existing <strong className="font-mono text-foreground">*.&lt;platform-domain&gt;</strong> record already covers this hostname; add an explicit A/AAAA record only when no matching wildcard exists.</p></div><div className="flex flex-wrap gap-2">{!gateway || gateway.desired_status === "absent" ? <StatusBadge tone="neutral">Provisioned with first database</StatusBadge> : <ConfirmationAction title="Tear down the PostgreSQL gateway?" description="Every external connection must be revoked first. Internal application bindings and private database containers are unaffected." confirmLabel="Tear down gateway" confirmationText="TEAR DOWN POSTGRESQL GATEWAY" destructive onConfirm={() => teardown.mutate()} trigger={<Button size="sm" variant="outline" disabled={!gatewayQuery.data?.feature_enabled || teardown.isPending}>Tear down</Button>} />}</div></div>{gateway && <div className="grid border-t text-[10px] sm:grid-cols-4"><GatewayFact label="Container" value={gateway.docker_container_id ? gateway.observed_status : "Not running"} /><GatewayFact label="Config" value={`${gateway.applied_config_generation} / ${gateway.desired_config_generation}`} /><GatewayFact label="Certificate" value={gateway.certificate_expires_at ? `Expires ${formatDate(gateway.certificate_expires_at)}` : "Not synchronized"} /><GatewayFact label="Image" value={gateway.image_version || "Unconfigured"} /></div>}</div>{operationID && <OperationWatcher id={operationID} onFinished={finish} />}{teardown.isError && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">{mutationMessage(teardown.error)}</p>}<div><div className="mb-3"><h3 className="text-xs font-semibold">Environment access</h3><p className="mt-1 text-[11px] leading-5 text-muted-foreground">Production and Staging routes, roles, CIDRs, sessions, and credentials remain isolated.</p></div><div className="space-y-4">{instances.map((instance) => <InstanceExternalAccess key={instance.id} instance={instance} environment={environments.find((item) => item.id === instance.environment_id)} enabled={gatewayQuery.data?.feature_enabled === true} />)}</div></div></>}</div>
    {gateway?.certificate_fingerprint && <p className="border-t bg-muted/20 px-5 py-3 text-[10px] text-muted-foreground">Certificate SHA-256 <span className="ml-1 break-all font-mono text-foreground">{gateway.certificate_fingerprint}</span></p>}
  </section>
}

function GatewayFact({ label, value }: { label: string; value: string }) {
  return <div className="border-b px-4 py-3 last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0"><span className="block text-muted-foreground">{label}</span><span className="mt-1 block break-all font-medium">{value}</span></div>
}
