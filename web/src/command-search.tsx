import { useEffect, useRef, useState } from "react"
import { useQueries, useQuery } from "@tanstack/react-query"
import { useNavigate } from "react-router-dom"
import {
  ActivityIcon,
  AppWindowIcon,
  BookOpenIcon,
  CloudArrowUpIcon,
  GearSixIcon,
  MagnifyingGlassIcon,
  PulseIcon,
  SquaresFourIcon,
  CubeIcon,
} from "@phosphor-icons/react"
import { Command as CommandPrimitive } from "cmdk"

import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { api, queryKeys, type ApplicationDTO, type ServiceDTO } from "@/api"

const destinationGroups = [
  {
    label: "Workspace",
    items: [
      { label: "Overview", detail: "Host resources and system health", href: "/", icon: SquaresFourIcon },
      { label: "Applications", detail: "Browse applications and services", href: "/applications", icon: AppWindowIcon },
      { label: "Deployments", detail: "Build and release activity", href: "/deployments", icon: CloudArrowUpIcon },
      { label: "Observability", detail: "Metrics, logs, and runtime health", href: "/observability", icon: PulseIcon },
    ],
  },
  {
    label: "Platform",
    items: [
      { label: "Settings", detail: "Platform and deployment configuration", href: "/settings", icon: GearSixIcon },
      { label: "Documentation", detail: "Guides and operator references", href: "/docs", icon: BookOpenIcon },
      { label: "System status", detail: "Services and integration health", href: "/status", icon: ActivityIcon },
    ],
  },
]

function applicationResourceItems(detail: { application: ApplicationDTO; services?: ServiceDTO[] | null } | undefined) {
  if (!detail?.application) return []

  const application = detail.application
  const services = Array.isArray(detail.services) ? detail.services : []
  return [
    { label: application.name, detail: application.description || "Application overview", href: "/applications/" + application.id, icon: AppWindowIcon },
    ...services.map((service) => ({ label: service.name, detail: application.name + " service", href: "/applications/" + application.id + "/services/" + service.id, icon: CubeIcon })),
  ]
}

export function CommandSearch() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const applicationsQuery = useQuery({ queryKey: queryKeys.applications, queryFn: ({ signal }) => api.applications(signal) })
  const applications = Array.isArray(applicationsQuery.data?.applications) ? applicationsQuery.data.applications : []
  const detailQueries = useQueries({ queries: applications.map((application) => ({ queryKey: queryKeys.application(application.id), queryFn: ({ signal }) => api.application(application.id, signal), enabled: open, staleTime: 60_000 })) })
  const resourceItems = detailQueries.flatMap((detail) => applicationResourceItems(detail.data))

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setOpen(true)
        requestAnimationFrame(() => inputRef.current?.focus())
      }

      if (event.key === "Escape" && open) {
        setOpen(false)
        inputRef.current?.blur()
      }
    }

    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [open])

  const openDestination = (href: string) => {
    setOpen(false)
    setQuery("")
    inputRef.current?.blur()
    navigate(href)
  }

  return (
    <Command className="contents">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverAnchor asChild>
          <div className="hf-command">
            <MagnifyingGlassIcon size={16} className="shrink-0" />
            <CommandPrimitive.Input
              ref={inputRef}
              value={query}
              onValueChange={(value) => {
                setQuery(value)
                if (!open) setOpen(true)
              }}
              onClick={() => setOpen(true)}
              placeholder="Search HostForge..."
              aria-label="Search HostForge"
              className="min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none placeholder:text-muted-foreground"
            />
            <span className="hf-command-keys" aria-hidden="true">
              <kbd>Ctrl</kbd><kbd>K</kbd>
            </span>
          </div>
        </PopoverAnchor>

        <PopoverContent
          align="center"
          side="bottom"
          sideOffset={9}
          onOpenAutoFocus={(event) => event.preventDefault()}
          onCloseAutoFocus={(event) => event.preventDefault()}
          className="w-[clamp(20rem,38vw,36rem)] overflow-hidden rounded-xl border bg-popover p-0 shadow-[0_18px_55px_-22px_rgb(0_0_0_/_0.38),0_4px_16px_rgb(0_0_0_/_0.08)]"
        >
          <CommandList className="max-h-[min(25rem,var(--radix-popover-content-available-height))] p-2">
            <CommandEmpty className="px-5 py-10 text-center">
              <MagnifyingGlassIcon className="mx-auto mb-3 text-muted-foreground" size={22} />
              <p className="text-xs font-semibold">No matching destination</p>
              <p className="mt-1 text-[11px] text-muted-foreground">Try an application, deployment, or setting.</p>
            </CommandEmpty>
            {resourceItems.length > 0 && <CommandGroup heading="Applications and services" className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.14em]">{resourceItems.map((item) => { const ItemIcon = item.icon; return <CommandItem key={item.href} value={item.label + " " + item.detail} onSelect={() => openDestination(item.href)} className="hf-command-item cursor-pointer rounded-lg px-2.5 py-2.5"><span className="hf-command-item-icon grid size-8 shrink-0 place-items-center rounded-lg border bg-card text-muted-foreground"><ItemIcon size={15} /></span><span className="min-w-0"><span className="block text-xs font-semibold">{item.label}</span><span className="hf-command-item-detail block truncate text-[10px] text-muted-foreground">{item.detail}</span></span></CommandItem> })}</CommandGroup>}
            {destinationGroups.map((group) => (
              <CommandGroup key={group.label} heading={group.label} className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:pb-2 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-[0.14em]">
                {group.items.map((item) => {
                  const ItemIcon = item.icon
                  return (
                    <CommandItem
                      key={item.href}
                      value={`${item.label} ${item.detail}`}
                      onSelect={() => openDestination(item.href)}
                      className="hf-command-item cursor-pointer rounded-lg px-2.5 py-2.5"
                    >
                      <span className="hf-command-item-icon grid size-8 shrink-0 place-items-center rounded-lg border bg-card text-muted-foreground shadow-sm">
                        <ItemIcon size={15} />
                      </span>
                      <span className="min-w-0">
                        <span className="block text-xs font-semibold">{item.label}</span>
                        <span className="hf-command-item-detail mt-0.5 block truncate text-[10px] text-muted-foreground">{item.detail}</span>
                      </span>
                      <CommandShortcut><kbd className="hf-command-key">Enter</kbd></CommandShortcut>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            ))}
          </CommandList>
          <footer className="flex items-center gap-4 border-t bg-muted/35 px-4 py-2.5 text-[10px] font-semibold text-muted-foreground">
            <span className="flex items-center gap-1.5"><kbd className="hf-command-key">Up</kbd><kbd className="hf-command-key">Down</kbd> navigate</span>
            <span className="flex items-center gap-1.5"><kbd className="hf-command-key">Enter</kbd> open</span>
            <span className="ml-auto flex items-center gap-1.5"><kbd className="hf-command-key">Esc</kbd> close</span>
          </footer>
        </PopoverContent>
      </Popover>
    </Command>
  )
}
