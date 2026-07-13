import { useEffect, useRef, useState } from "react"
import { Link } from "react-router-dom"
import { createPortal } from "react-dom"
import { CommandIcon, MagnifyingGlassIcon, XIcon } from "@phosphor-icons/react"

const destinations = [
  { label: "Overview", detail: "Host resources and system health", href: "/" },
  { label: "Applications", detail: "Browse applications and services", href: "/applications" },
  { label: "Deployments", detail: "Build and release activity", href: "/deployments" },
  { label: "Observability", detail: "Metrics, logs, and runtime health", href: "/observability" },
  { label: "Settings", detail: "Platform and deployment configuration", href: "/settings" },
]

export function CommandSearch() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setOpen((current) => !current)
      }
      if (event.key === "Escape") setOpen(false)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  useEffect(() => {
    if (open) requestAnimationFrame(() => inputRef.current?.focus())
  }, [open])

  const matches = destinations.filter((item) => `${item.label} ${item.detail}`.toLowerCase().includes(query.toLowerCase()))

  return <><button className="hf-command hidden sm:flex" onClick={() => setOpen(true)} aria-label="Open command search"><MagnifyingGlassIcon size={15} /><span>Search HostForge</span><kbd><CommandIcon size={11} /> K</kbd></button>{open && createPortal(<div className="hf-command-overlay" role="presentation" onMouseDown={(event) => event.target === event.currentTarget && setOpen(false)}><section className="hf-command-dialog" role="dialog" aria-modal="true" aria-label="Search HostForge"><div className="hf-command-input"><MagnifyingGlassIcon size={19} /><input ref={inputRef} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search applications, deployments, settings..." /><button onClick={() => setOpen(false)} aria-label="Close search"><XIcon size={17} /></button></div><div className="hf-command-results"><p>Quick navigation</p>{matches.map((item) => <Link key={item.href} to={item.href} onClick={() => setOpen(false)}><span>{item.label}</span><small>{item.detail}</small></Link>)}{!matches.length && <div className="hf-command-empty">No matching pages found.</div>}</div><footer><span><kbd>Enter</kbd> open</span><span><kbd>Esc</kbd> close</span></footer></section></div>, document.body)}</>
}
