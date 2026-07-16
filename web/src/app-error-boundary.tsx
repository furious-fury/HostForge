import { Component, type ErrorInfo, type ReactNode } from "react"
import { Link, useLocation } from "react-router-dom"

type State = { error?: Error }
type Props = { children: ReactNode; resetKey?: string }

export class AppErrorBoundary extends Component<Props, State> {
  state: State = {}

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("HostForge UI render failure", error, info.componentStack)
  }

  componentDidUpdate(previous: Props) {
    if (this.state.error && previous.resetKey !== this.props.resetKey) {
      this.setState({ error: undefined })
    }
  }

  render() {
    if (!this.state.error) return this.props.children
    return <main className="grid min-h-svh place-items-center bg-background p-6 text-foreground"><section className="w-full max-w-lg rounded-xl border bg-card p-8 text-center shadow-sm"><p className="text-xs font-semibold uppercase tracking-[0.14em] text-destructive">Interface error</p><h1 className="mt-3 text-xl font-semibold">This screen could not be rendered</h1><p className="mt-2 text-xs leading-5 text-muted-foreground">Your server session and deployment state were not changed. You can retry this screen or leave it safely.</p>{import.meta.env.DEV && <details className="mt-4 rounded-lg border bg-muted/35 p-3 text-left"><summary className="cursor-pointer text-[11px] font-semibold">Technical details</summary><code className="mt-2 block break-words text-[10px] text-destructive">{this.state.error.message || this.state.error.name}</code></details>}<div className="mt-5 flex flex-wrap justify-center gap-2"><button className="rounded-md border bg-muted px-4 py-2 text-xs font-semibold hover:bg-muted/75 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" onClick={() => this.setState({ error: undefined })}>Retry screen</button><Link to="/" className="rounded-md bg-primary px-4 py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/85 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">Return to overview</Link></div></section></main>
  }
}

export function RouteAwareAppErrorBoundary({ children }: { children: ReactNode }) {
  const location = useLocation()
  return <AppErrorBoundary resetKey={`${location.pathname}${location.search}`}>{children}</AppErrorBoundary>
}
