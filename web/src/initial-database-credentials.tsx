import { useEffect, useRef, useState } from "react"
import { useQueries } from "@tanstack/react-query"
import { ClipboardIcon, KeyIcon, WarningCircleIcon } from "@phosphor-icons/react"

import {
  api,
  queryKeys,
  type DatabaseExternalCredentialRevealDTO,
} from "@/api"
import { Button } from "@/components/ui/button"
import type { InitialDatabaseCredentialProgress } from "@/initial-database-credential-progress"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useToast } from "@/toast-provider"

type RevealedCredential = {
  environmentName: string
  credential: DatabaseExternalCredentialRevealDTO
}

export function InitialDatabaseCredentials({
  entries,
  errors = [],
  onDone,
}: {
  entries: InitialDatabaseCredentialProgress[]
  errors?: Array<{ environment_id: string; environment_name?: string; error: string }>
  onDone: () => void
}) {
  const toast = useToast()
  const requested = useRef(false)
  const [credentials, setCredentials] = useState<RevealedCredential[]>([])
  const [revealError, setRevealError] = useState("")
  const operationQueries = useQueries({
    queries: entries.map((entry) => ({
      queryKey: queryKeys.databaseGatewayOperation(entry.operationId),
      queryFn: ({ signal }: { signal: AbortSignal }) => api.databaseGatewayOperation(entry.operationId, signal),
      refetchInterval: (query: { state: { data?: { operation?: { status?: string } } } }) => {
        const status = query.state.data?.operation?.status
        return status === "success" || status === "failed" ? false : 1500
      },
    })),
  })
  const operations = operationQueries.map((query) => query.data?.operation)
  const failed = operations.find((operation) => operation?.status === "failed")
  const allSuccessful = entries.length > 0 && operations.length === entries.length && operations.every((operation) => operation?.status === "success")

  useEffect(() => {
    if (!allSuccessful || requested.current) return
    requested.current = true
    void Promise.all(entries.map(async (entry) => ({
      environmentName: entry.environmentName,
      credential: await api.revealDatabaseExternalCredentials(entry.connectionId),
    }))).then(setCredentials).catch((error: unknown) => {
      setRevealError(error instanceof Error ? error.message : "Credentials could not be revealed.")
    })
  }, [allSuccessful, entries])

  const terminal = Boolean(failed || revealError || errors.length > 0 && entries.length === 0 || entries.length > 0 && credentials.length === entries.length)
  const copy = async (value: string, label: string) => {
    await navigator.clipboard.writeText(value)
    toast(`${label} copied.`)
  }

  return <Dialog open onOpenChange={(next) => { if (!next && terminal) onDone() }}>
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{credentials.length === entries.length ? "Database credentials are ready" : failed || revealError ? "Public credential setup needs attention" : "Provisioning database and public credentials"}</DialogTitle>
        <DialogDescription>
          {credentials.length === entries.length
            ? "Each environment has an isolated PostgreSQL URL restricted to the public IP used to create this database."
            : failed || revealError
              ? "The private database remains available. Review the error, then retry public access from Database settings."
              : "HostForge is creating the database, TLS gateway route, scoped role, and source-IP rule as one durable workflow."}
        </DialogDescription>
      </DialogHeader>

      {!terminal && <div className="space-y-3">
        {entries.map((entry, index) => {
          const operation = operations[index]
          return <div key={entry.connectionId} className="rounded-lg border p-3">
            <div className="flex items-center justify-between gap-3 text-xs"><span className="font-semibold">{entry.environmentName}</span><span className="text-muted-foreground">{operation?.progress_percent || 0}%</span></div>
            <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted"><span className="block h-full rounded-full bg-accent transition-all" style={{ width: `${Math.max(3, operation?.progress_percent || 0)}%` }} /></div>
            <p className="mt-2 text-[10px] capitalize text-muted-foreground">{(operation?.progress_step || "waiting for database").replaceAll("_", " ")}</p>
          </div>
        })}
      </div>}

      {(failed || revealError || errors.length > 0) && <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
        <div className="flex items-start gap-2"><WarningCircleIcon className="mt-0.5 shrink-0" size={16} /><div><strong>Public access was not completed.</strong><p className="mt-1">{revealError || failed?.error_message || failed?.error_code?.replaceAll("_", " ") || errors[0]?.error.replaceAll("_", " ")}</p></div></div>
      </div>}

      {credentials.length > 0 && <div className="space-y-4">
        {credentials.map(({ environmentName, credential }) => <section key={environmentName} className="overflow-hidden rounded-xl border">
          <header className="flex items-center gap-2 border-b bg-muted/35 px-4 py-3"><KeyIcon size={16} /><div><h3 className="text-xs font-semibold">{environmentName}</h3><p className="mt-0.5 text-[10px] text-muted-foreground">Generation {credential.generation} · TLS verify-full</p></div></header>
          <div className="space-y-3 p-4">
            <CredentialRow label="PostgreSQL URL" value={credential.url} onCopy={() => copy(credential.url, `${environmentName} URL`)} />
            <CredentialRow label="Username" value={credential.username} onCopy={() => copy(credential.username, `${environmentName} username`)} />
            <CredentialRow label="Password" value={credential.password} onCopy={() => copy(credential.password, `${environmentName} password`)} />
            <CredentialRow label="Database alias" value={credential.database_alias} onCopy={() => copy(credential.database_alias, `${environmentName} database alias`)} />
          </div>
        </section>)}
      </div>}

      <DialogFooter>
        {terminal
          ? <Button onClick={onDone}>{credentials.length ? "I saved the credentials" : "Close"}</Button>
          : <Button disabled>Provisioning…</Button>}
      </DialogFooter>
    </DialogContent>
  </Dialog>
}

function CredentialRow({ label, value, onCopy }: { label: string; value: string; onCopy: () => void }) {
  return <div className="overflow-hidden rounded-lg border"><div className="flex items-center justify-between border-b bg-muted/35 px-3 py-2"><span className="text-[10px] font-semibold uppercase tracking-[0.1em] text-muted-foreground">{label}</span><Button size="sm" variant="ghost" onClick={onCopy}><ClipboardIcon size={13} /> Copy</Button></div><p className="break-all px-3 py-3 font-mono text-[11px]">{value}</p></div>
}
