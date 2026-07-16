import { createContext, useCallback, useContext, useState, type ReactNode } from "react"

type ToastTone = "default" | "warning" | "error"
type ToastOptions = { tone?: ToastTone; duration?: number }
type Toast = { id: string; message: string; tone: ToastTone }
type ToastContextValue = { toast: (message: string, options?: ToastOptions) => void }

const ToastContext = createContext<ToastContextValue | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([])
  const dismiss = useCallback((id: string) => setItems((current) => current.filter((item) => item.id !== id)), [])
  const toast = useCallback((message: string, options: ToastOptions = {}) => {
    const id = crypto.randomUUID()
    setItems((current) => [...current, { id, message, tone: options.tone || "default" }])
    const duration = options.duration ?? 4000
    if (duration > 0) window.setTimeout(() => dismiss(id), duration)
  }, [dismiss])

  return <ToastContext.Provider value={{ toast }}>{children}<div className="pointer-events-none fixed bottom-5 right-5 z-[100] flex w-[min(28rem,calc(100vw-2.5rem))] flex-col gap-2" role="status" aria-live="polite">{items.map((item) => <div key={item.id} className={`pointer-events-auto flex items-start gap-3 rounded-lg border px-4 py-3 text-xs font-semibold shadow-lg ${item.tone === "warning" ? "border-amber-300 bg-amber-50 text-amber-950 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-100" : item.tone === "error" ? "border-destructive/40 bg-destructive text-destructive-foreground" : "bg-foreground text-background"}`}><span className="min-w-0 flex-1 leading-5">{item.message}</span><button type="button" className="shrink-0 rounded px-1 text-[10px] opacity-70 hover:opacity-100" onClick={() => dismiss(item.id)} aria-label="Dismiss notification">Dismiss</button></div>)}</div></ToastContext.Provider>
}

// The hook intentionally shares this module with its provider so they cannot
// drift onto different context instances.
// eslint-disable-next-line react-refresh/only-export-components
export function useToast() {
  const value = useContext(ToastContext)
  if (!value) throw new Error("useToast must be used within ToastProvider")
  return value.toast
}
