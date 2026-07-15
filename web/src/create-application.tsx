import { useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { Link, useNavigate } from "react-router-dom"
import { ArrowLeftIcon, CubeIcon, PlusIcon } from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { api, queryKeys } from "@/api"
import { queryClient } from "@/query-client"

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}{hint && <span className="mt-2 block text-[11px] leading-5 text-muted-foreground">{hint}</span>}</label>
}

export function CreateApplication() {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const navigate = useNavigate()
  const create = useMutation({
    mutationFn: (input: { name: string; description: string; addService: boolean }) => api.createApplication({ name: input.name, description: input.description }),
    onSuccess: async (result, input) => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.applications })
      const addService = input.addService
      navigate(addService ? `/applications/${result.application.id}/services/new` : `/applications/${result.application.id}`)
    },
  })

  const submit = (addService: boolean) => {
    if (!name.trim()) return
    create.mutate({ name: name.trim(), description: description.trim(), addService })
  }

  return <main className="mx-auto w-full max-w-3xl px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
    <Link to="/applications" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />Back to applications</Link>
    <div className="mb-7"><h1 className="text-3xl font-semibold tracking-[-0.035em]">Create application</h1><p className="mt-2 text-sm text-muted-foreground">Applications group production and staging environments, services, domains, variables, and deployment history.</p></div>
    <section className="overflow-hidden rounded-xl border bg-card"><header className="flex items-center gap-3 border-b bg-muted/75 px-6 py-5"><span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground"><CubeIcon size={18} weight="fill" /></span><div><h2 className="text-sm font-semibold">Application details</h2><p className="mt-1 text-xs text-muted-foreground">Production and staging environments are created automatically.</p></div></header>
      <div className="space-y-6 p-6"><Field label="Application name" hint="Use the product or platform name shown in navigation and activity."><Input autoFocus value={name} onChange={(event) => setName(event.target.value)} className="h-10 bg-background text-xs" placeholder="My application" /></Field><Field label="Description" hint="Optional context for operators."><Textarea value={description} onChange={(event) => setDescription(event.target.value)} className="min-h-28 resize-none bg-background text-xs" placeholder="What does this application contain?" /></Field><div className="rounded-lg border bg-muted/35 p-4"><p className="text-xs font-semibold">No deployment starts automatically</p><p className="mt-1 text-[11px] leading-5 text-muted-foreground">After creation, add a service from a GitHub App repository, choose environment branches, then explicitly deploy.</p></div></div>
      {create.isError && <p role="alert" className="border-t border-destructive/30 bg-destructive/5 px-6 py-3 text-[11px] text-destructive">The server could not create the application. Check the name and try again.</p>}
      <footer className="flex flex-col-reverse gap-2 border-t bg-muted/30 px-6 py-4 sm:flex-row sm:justify-end"><Button variant="outline" disabled={create.isPending || !name.trim()} onClick={() => submit(false)}>Create application only</Button><Button disabled={create.isPending || !name.trim()} onClick={() => submit(true)}><PlusIcon />{create.isPending ? "Creating..." : "Create and add service"}</Button></footer>
    </section>
  </main>
}
