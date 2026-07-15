import type { ReactNode } from "react"

import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

export type StatusTone = "success" | "warning" | "error" | "info" | "neutral"

const toneClasses: Record<StatusTone, string> = {
  success: "border-emerald-600/15 bg-emerald-50 text-emerald-700 dark:bg-emerald-950/45 dark:text-emerald-300",
  warning: "border-amber-600/15 bg-amber-50 text-amber-700 dark:bg-amber-950/45 dark:text-amber-300",
  error: "border-red-600/15 bg-red-50 text-red-700 dark:bg-red-950/45 dark:text-red-300",
  info: "border-blue-600/15 bg-blue-50 text-blue-700 dark:bg-blue-950/45 dark:text-blue-300",
  neutral: "border-border bg-muted text-muted-foreground",
}

type StatusBadgeProps = {
  children: ReactNode
  tone?: StatusTone
  dot?: boolean
  icon?: ReactNode
  className?: string
}

export function StatusBadge({ children, tone = "neutral", dot = false, icon, className }: StatusBadgeProps) {
  return <Badge variant="outline" className={cn("gap-1.5 rounded-full px-2 py-1 text-[10px] leading-none", toneClasses[tone], className)}>{dot && <span className="size-1.5 rounded-full bg-current" />}{icon}{children}</Badge>
}
