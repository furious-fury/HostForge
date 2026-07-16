import {
  BracketsCurlyIcon,
  CodeIcon,
  CubeIcon,
  FileCodeIcon,
  GlobeIcon,
} from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

type StackIdentityProps = {
  kind?: string
  label?: string
  className?: string
  iconClassName?: string
  showLabel?: boolean
}

const stackIcons = {
  web: { icon: GlobeIcon, color: "bg-sky-500/10 text-sky-600 dark:text-sky-400" },
  javascript: { icon: BracketsCurlyIcon, color: "bg-amber-500/10 text-amber-700 dark:text-amber-400" },
  container: { icon: CubeIcon, color: "bg-blue-500/10 text-blue-600 dark:text-blue-400" },
  backend: { icon: CodeIcon, color: "bg-violet-500/10 text-violet-600 dark:text-violet-400" },
  unknown: { icon: FileCodeIcon, color: "bg-muted text-foreground" },
}

function stackIconKey(kind = "", label = ""): keyof typeof stackIcons {
  const value = `${kind} ${label}`.toLowerCase()
  if (/(next|react|vue|svelte|astro|static|frontend)/.test(value)) return "web"
  if (/(node|bun|deno|javascript|typescript)/.test(value)) return "javascript"
  if (/(docker|dockerfile|container)/.test(value)) return "container"
  if (/(go|python|ruby|php|java|rust|dotnet)/.test(value)) return "backend"
  return "unknown"
}

export function StackIdentity({ kind, label, className, iconClassName, showLabel = true }: StackIdentityProps) {
  const identity = stackIcons[stackIconKey(kind, label)]
  const Icon = identity.icon
  const name = label || kind || "Not detected"
  return <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg border", identity.color, iconClassName)} title={`${name} stack`}>
      <Icon className="size-[18px]" weight="bold" aria-hidden="true" />
    </span>
    {showLabel && <span className="truncate text-xs font-medium">{name}</span>}
  </span>
}
