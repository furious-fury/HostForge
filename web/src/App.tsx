import { ArrowRightIcon, CubeIcon } from "@phosphor-icons/react"

import { Button } from "@/components/ui/button"

function App() {
  return (
    <main className="grid min-h-svh place-items-center bg-background px-6 text-foreground">
      <section className="w-full max-w-xl rounded-2xl border bg-card shadow-sm">
        <header className="flex items-center gap-3 border-b bg-muted/70 px-6 py-4">
          <span className="grid size-9 place-items-center rounded-lg bg-accent text-accent-foreground">
            <CubeIcon size={20} weight="fill" />
          </span>
          <div>
            <p className="font-semibold">HostForge</p>
            <p className="text-sm text-muted-foreground">Management UI foundation</p>
          </div>
        </header>

        <div className="space-y-5 p-6">
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">Ready to rebuild.</h1>
            <p className="max-w-md text-sm leading-6 text-muted-foreground">
              Vite, TypeScript, Tailwind, shadcn, and Phosphor Icons are configured. We can now
              rebuild each HostForge workflow one piece at a time.
            </p>
          </div>

          <Button>
            Start with the first screen
            <ArrowRightIcon data-icon="inline-end" />
          </Button>
        </div>
      </section>
    </main>
  )
}

export default App
