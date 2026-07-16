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
  web: GlobeIcon,
  javascript: BracketsCurlyIcon,
  container: CubeIcon,
  backend: CodeIcon,
  unknown: FileCodeIcon,
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
  const Icon = stackIcons[stackIconKey(kind, label)]
  const name = label || kind || "Not detected"
  return <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg border bg-muted text-muted-foreground", iconClassName)}>
      <Icon size={17} weight="duotone" />
    </span>
    {showLabel && <span className="truncate text-xs font-medium">{name}</span>}
  </span>
}
