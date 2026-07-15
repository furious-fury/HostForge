import { useId } from "react"

import { cn } from "@/lib/utils"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

type SelectOption = string | { value: string; label: string; disabled?: boolean }

type AppSelectProps = {
  options: readonly SelectOption[]
  value?: string
  defaultValue?: string
  onValueChange?: (value: string) => void
  placeholder?: string
  className?: string
  disabled?: boolean
  "aria-label"?: string
}

export function AppSelect({ options, value, defaultValue, onValueChange, placeholder, className, disabled, "aria-label": ariaLabel }: AppSelectProps) {
  const id = useId()
  const normalized = options.map((option) => typeof option === "string" ? { value: option, label: option } : option)
  const initialValue = defaultValue ?? normalized[0]?.value

  return (
    <Select value={value} defaultValue={value === undefined ? initialValue : undefined} onValueChange={onValueChange} disabled={disabled}>
      <SelectTrigger id={id} aria-label={ariaLabel} className={cn("h-9 min-w-0 border-border bg-card px-3 text-xs shadow-[0_2px_8px_rgb(31_35_30_/_0.045)] focus:ring-3 focus:ring-ring/20", className)}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent position="popper" className="border-border bg-card text-foreground shadow-[0_16px_40px_rgb(20_22_19_/_0.14)]">
        {normalized.map((option) => <SelectItem key={option.value} value={option.value} disabled={option.disabled} className="text-xs focus:bg-muted focus:text-foreground">{option.label}</SelectItem>)}
      </SelectContent>
    </Select>
  )
}
