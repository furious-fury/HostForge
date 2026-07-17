import { DatabaseIcon } from "@phosphor-icons/react"

import { cn } from "@/lib/utils"

const databaseIcons = new Set(["postgresql", "mysql", "mariadb", "mongodb", "redis", "valkey"])

type DatabaseIdentityProps = {
  engine?: string
  label?: string
  className?: string
  iconClassName?: string
  imageClassName?: string
  showLabel?: boolean
}

function databaseIconPath(engine = "") {
  const normalized = engine.trim().toLowerCase()
  return databaseIcons.has(normalized) ? `/db/${normalized}.png` : ""
}

export function DatabaseIdentity({ engine, label, className, iconClassName, imageClassName, showLabel = true }: DatabaseIdentityProps) {
  const name = label || engine || "Database"
  const iconPath = databaseIconPath(engine)
  return <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
    <span className={cn("grid size-9 shrink-0 place-items-center rounded-lg border bg-card", iconClassName)} title={`${name} database`}>
      {iconPath ? <img src={iconPath} alt={`${name} database icon`} className={cn("size-6 object-contain", imageClassName)} /> : <DatabaseIcon className="size-5 text-muted-foreground" weight="bold" aria-hidden="true" />}
    </span>
    {showLabel && <span className="truncate text-xs font-medium">{name}</span>}
  </span>
}
