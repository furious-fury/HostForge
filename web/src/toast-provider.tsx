import { createContext, useCallback, useContext, useState, type ReactNode } from "react"

type Toast = { id: string; message: string }
type ToastContextValue = { toast: (message: string) => void }

const ToastContext = createContext<ToastContextValue | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([])
  const toast = useCallback((message: string) => {
    const id = crypto.randomUUID()
    setItems((current) => [...current, { id, message }])
    window.setTimeout(() => setItems((current) => current.filter((item) => item.id !== id)), 4000)
  }, [])

  return <ToastContext.Provider value={{ toast }}>{children}<div className="pointer-events-none fixed bottom-5 right-5 z-[100] flex w-[min(24rem,calc(100vw-2.5rem))] flex-col gap-2" role="status" aria-live="polite">{items.map((item) => <div key={item.id} className="rounded-lg border bg-foreground px-4 py-3 text-xs font-semibold text-background shadow-lg">{item.message}</div>)}</div></ToastContext.Provider>
}

// The hook intentionally shares this module with its provider so they cannot
// drift onto different context instances.
// eslint-disable-next-line react-refresh/only-export-components
export function useToast() {
  const value = useContext(ToastContext)
  if (!value) throw new Error("useToast must be used within ToastProvider")
  return value.toast
}
