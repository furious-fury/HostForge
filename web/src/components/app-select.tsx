import { useId, useState } from "react"
import { CaretDownIcon, CheckIcon } from "@phosphor-icons/react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
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
  searchable?: boolean
  searchPlaceholder?: string
  emptyMessage?: string
  "aria-label"?: string
}

export function AppSelect({ options, value, defaultValue, onValueChange, placeholder, className, disabled, searchable = false, searchPlaceholder = "Search options...", emptyMessage = "No matching options.", "aria-label": ariaLabel }: AppSelectProps) {
  const id = useId()
  const [open, setOpen] = useState(false)
  const normalized = options.map((option) => typeof option === "string" ? { value: option, label: option } : option)
  const initialValue = defaultValue ?? normalized[0]?.value
  const selectedValue = value ?? initialValue
  const selected = normalized.find((option) => option.value === selectedValue)

  if (searchable) {
    return <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button id={id} type="button" variant="outline" role="combobox" aria-expanded={open} aria-label={ariaLabel} disabled={disabled} className={cn("h-9 min-w-0 justify-between border-border bg-card px-3 text-xs font-normal shadow-[0_2px_8px_rgb(31_35_30_/_0.045)] focus:ring-3 focus:ring-ring/20", className)}>
          <span className={cn("truncate", !selected && "text-muted-foreground")}>{selected?.label || placeholder || "Select an option"}</span>
          <CaretDownIcon className="ml-2 size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] min-w-64 p-0">
        <Command>
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList className="max-h-72">
            <CommandEmpty>{emptyMessage}</CommandEmpty>
            {normalized.map((option) => <CommandItem key={option.value} value={`${option.label} ${option.value}`} disabled={option.disabled} onSelect={() => { onValueChange?.(option.value); setOpen(false) }} className="text-xs">
              <CheckIcon className={cn("size-4", selectedValue === option.value ? "opacity-100" : "opacity-0")} />
              <span className="truncate">{option.label}</span>
            </CommandItem>)}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  }

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
