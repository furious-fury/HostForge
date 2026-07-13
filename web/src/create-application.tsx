import { useState } from "react"
import { Link } from "react-router-dom"
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  CheckIcon,
  CubeIcon,
  GitBranchIcon,
  GithubLogoIcon,
  GlobeIcon,
  LinkIcon,
  RocketLaunchIcon,
} from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"
import { navigateTo } from "@/navigation"
import "@/create-application.css"

const steps = ["Application details", "First service", "Review"]

export function CreateApplication() {
  const [step, setStep] = useState(1)
  const [source, setSource] = useState("github")
  const [name, setName] = useState("TaxIO")
  const [description, setDescription] = useState("Nigerian personal income tax platform")
  const [environment, setEnvironment] = useState("Production")

  return (
    <main className="mx-auto w-full max-w-5xl px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
      <Link to="/applications" className="mb-5 inline-flex items-center gap-2 text-xs font-medium text-muted-foreground hover:text-foreground"><ArrowLeftIcon size={14} />Back to applications</Link>
      <div className="mb-8">
        <h1 className="text-3xl font-semibold tracking-[-0.035em]">Create application</h1>
        <p className="mt-2 text-sm text-muted-foreground">Create a product container, then connect its first deployable service.</p>
      </div>

      <div className="mb-5 grid overflow-hidden rounded-xl border bg-card sm:grid-cols-3">
        {steps.map((label, index) => {
          const number = index + 1
          const complete = number < step
          const active = number === step
          return (
            <button key={label} onClick={() => number < step && setStep(number)} className={`flex items-center gap-3 border-b px-4 py-4 text-left last:border-b-0 sm:border-b-0 sm:border-r sm:last:border-r-0 ${active ? "bg-muted/75" : "bg-card"}`}>
              <span className={`grid size-7 place-items-center rounded-full text-[11px] font-semibold ${active || complete ? "bg-accent text-accent-foreground" : "border bg-background text-muted-foreground"}`}>{complete ? <CheckIcon size={12} weight="bold" /> : number}</span>
              <span><span className={`block text-xs font-semibold ${!active && !complete ? "text-muted-foreground" : ""}`}>{label}</span><span className="mt-0.5 block text-[10px] text-muted-foreground">Step {number} of 3</span></span>
            </button>
          )
        })}
      </div>

      <section className="overflow-hidden rounded-xl border bg-card">
        <header className="border-b bg-muted/75 px-6 py-4">
          <h2 className="text-sm font-semibold">{steps[step - 1]}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{step === 1 ? "Name the product and choose its initial environment." : step === 2 ? "Connect a repository now or start with an empty application." : "Confirm the application before creating it."}</p>
        </header>

        {step === 1 && (
          <div className="space-y-6 p-6">
            <Field label="Application name" hint="Used in navigation and deployment activity."><input value={name} onChange={(event) => setName(event.target.value)} className="hf-field" /></Field>
            <Field label="Description" hint="A short explanation of what this application contains."><textarea value={description} onChange={(event) => setDescription(event.target.value)} rows={4} className="hf-field resize-none" /></Field>
            <Field label="Environment" hint="You can add staging environments later."><select value={environment} onChange={(event) => setEnvironment(event.target.value)} className="hf-field"><option>Production</option><option>Staging</option><option>Development</option></select></Field>
          </div>
        )}

        {step === 2 && (
          <div className="grid gap-3 p-6 sm:grid-cols-3">
            <SourceOption selected={source === "github"} onSelect={() => setSource("github")} icon={<GithubLogoIcon size={22} />} title="Import from GitHub" description="Choose a repository from your GitHub App installation." />
            <SourceOption selected={source === "url"} onSelect={() => setSource("url")} icon={<LinkIcon size={22} />} title="Repository URL" description="Connect a public Git repository by URL." />
            <SourceOption selected={source === "empty"} onSelect={() => setSource("empty")} icon={<CubeIcon size={22} />} title="Empty application" description="Configure domains and variables before adding services." />
            {source === "github" && <div className="mt-3 rounded-lg border bg-muted/30 p-5 sm:col-span-3"><div className="grid gap-4 sm:grid-cols-2"><Field label="Repository"><select className="hf-field"><option>mr-fury/taxio</option><option>mr-fury/hostforge</option></select></Field><Field label="Branch"><div className="relative"><GitBranchIcon className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={15} /><input className="hf-field pl-9" defaultValue="main" /></div></Field></div></div>}
            {source === "url" && <div className="mt-3 rounded-lg border bg-muted/30 p-5 sm:col-span-3"><Field label="Repository URL"><input className="hf-field" placeholder="https://github.com/org/repository.git" /></Field></div>}
          </div>
        )}

        {step === 3 && (
          <div className="grid gap-5 p-6 md:grid-cols-[1fr_0.8fr]">
            <div className="overflow-hidden rounded-lg border"><ReviewRow label="Application" value={name || "Untitled application"} /><ReviewRow label="Description" value={description || "No description"} /><ReviewRow label="Environment" value={environment} /><ReviewRow label="Initial service" value={source === "empty" ? "None" : source === "github" ? "mr-fury/taxio" : "Repository URL"} /><ReviewRow label="Branch" value={source === "empty" ? "Not applicable" : "main"} /></div>
            <div className="rounded-lg border bg-muted/40 p-5"><span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground"><GlobeIcon size={18} /></span><h3 className="mt-4 text-sm font-semibold">Ready to create</h3><p className="mt-2 text-xs leading-5 text-muted-foreground">HostForge will create the application and prepare its shared configuration. No deployment starts until you confirm it.</p></div>
          </div>
        )}

        <footer className="flex items-center justify-between border-t bg-muted/30 px-6 py-4">
          <Button variant="outline" disabled={step === 1} onClick={() => setStep((current) => Math.max(1, current - 1))}><ArrowLeftIcon /> Back</Button>
          {step < 3 ? <Button disabled={step === 1 && !name.trim()} onClick={() => setStep((current) => Math.min(3, current + 1))}>Continue <ArrowRightIcon /></Button> : <div className="flex gap-2"><Button variant="outline" onClick={() => navigateTo("/applications/taxio")}>Create without deploying</Button><Button onClick={() => navigateTo("/applications/taxio")}><RocketLaunchIcon weight="fill" /> Create and deploy</Button></div>}
        </footer>
      </section>
    </main>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return <label className="block"><span className="mb-2 block text-xs font-semibold">{label}</span>{children}{hint && <span className="mt-2 block text-[11px] text-muted-foreground">{hint}</span>}</label>
}

function SourceOption({ selected, onSelect, icon, title, description }: { selected: boolean; onSelect: () => void; icon: React.ReactNode; title: string; description: string }) {
  return <button onClick={onSelect} className={`relative min-h-40 rounded-lg border p-5 text-left transition-colors ${selected ? "border-foreground bg-muted/60 ring-1 ring-foreground" : "hover:bg-muted/40"}`}><span className="text-muted-foreground">{icon}</span><span className="mt-5 block text-xs font-semibold">{title}</span><span className="mt-2 block text-[11px] leading-5 text-muted-foreground">{description}</span>{selected && <span className="absolute right-3 top-3 grid size-5 place-items-center rounded-full bg-accent text-accent-foreground"><CheckIcon size={11} weight="bold" /></span>}</button>
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return <div className="flex items-start gap-4 border-b px-4 py-3.5 last:border-b-0"><span className="w-28 shrink-0 text-[11px] text-muted-foreground">{label}</span><span className="text-xs font-medium">{value}</span></div>
}
