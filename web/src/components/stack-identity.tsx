import {
  FileCodeIcon,
} from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

type StackIdentityProps = {
  kind?: string
  label?: string
  className?: string
  iconClassName?: string
  showLabel?: boolean
}

function stackIconPath(kind = "", label = "") {
  const value = `${kind} ${label}`.toLowerCase()
  if (value.includes("vite")) return "/vite.png"
  if (value.includes("next")) return "/next.png"
  if (value.includes("vue")) return "/vue.png"
  if (value.includes("react") || value.includes("create react app")) return "/react.png"
  if (value.includes("django")) return "/django.png"
  if (value.includes("python")) return "/python.png"
  if (value.includes("golang") || value.match(/\bgo\b/)) return "/golang.png"
  if (value.includes("deno")) return "/deno.png"
  if (value.includes("ruby")) return "/ruby.png"
  if (value.includes("rust")) return "/rust.png"
  if (value.includes("java")) return "/java.png"
  if (value.includes("php")) return "/php.png"
  if (value.includes("c#") || value.includes("dotnet")) return "/c%23.png"
  if (value.includes("static") || value.includes("html")) return "/html5.png"
  if (value.includes("node") || value.includes("javascript") || value.includes("typescript")) return "/node.png"
  return ""
}

export function StackIdentity({ kind, label, className, iconClassName, showLabel = true }: StackIdentityProps) {
  const name = label || kind || "Not detected"
  const iconPath = stackIconPath(kind, label)
  return <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg border bg-card", iconClassName)} title={`${name} stack`}>
      {iconPath ? <img src={iconPath} alt={`${name} stack icon`} className="size-5 object-contain" /> : <FileCodeIcon className="size-[18px] text-muted-foreground" weight="bold" aria-hidden="true" />}
    </span>
    {showLabel && <span className="truncate text-xs font-medium">{name}</span>}
  </span>
}
