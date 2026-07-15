import { Component, type ErrorInfo, type ReactNode } from "react"

type State = { error?: Error }

export class AppErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = {}

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("HostForge UI render failure", error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return <main className="grid min-h-svh place-items-center bg-background p-6 text-foreground"><section className="w-full max-w-lg rounded-xl border bg-card p-8 text-center shadow-sm"><p className="text-xs font-semibold uppercase tracking-[0.14em] text-destructive">Interface error</p><h1 className="mt-3 text-xl font-semibold">This screen could not be rendered</h1><p className="mt-2 text-xs leading-5 text-muted-foreground">Your server session and deployment state were not changed. Reload the interface to recover.</p><button className="mt-5 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/85 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => window.location.reload()}>Reload HostForge</button></section></main>
  }
}
