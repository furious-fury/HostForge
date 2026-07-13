import { useEffect, useRef, useState } from "react"
import { CheckIcon, DesktopIcon, MoonIcon, PaletteIcon, SunIcon } from "@phosphor-icons/react"

import { type Accent, type ThemeMode, useTheme } from "@/theme-context"
import "@/theme-switcher.css"

const modes: Array<{ id: ThemeMode; label: string; icon: typeof SunIcon }> = [
  { id: "light", label: "Light", icon: SunIcon },
  { id: "dark", label: "Dark", icon: MoonIcon },
  { id: "system", label: "System", icon: DesktopIcon },
]

const accents: Array<{ id: Accent; label: string; color: string }> = [
  { id: "neutral", label: "Graphite", color: "#454541" },
  { id: "ocean", label: "Ocean", color: "#547fc2" },
  { id: "forest", label: "Forest", color: "#4b8a6d" },
  { id: "amber", label: "Amber", color: "#bc8745" },
  { id: "rose", label: "Rose", color: "#bd6b7c" },
]

export function ThemeSwitcher() {
  const { accent, mode, setAccent, setMode } = useTheme()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) setOpen(false)
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false)
    }
    document.addEventListener("mousedown", close)
    document.addEventListener("keydown", closeOnEscape)
    return () => {
      document.removeEventListener("mousedown", close)
      document.removeEventListener("keydown", closeOnEscape)
    }
  }, [open])

  return (
    <div className="hf-theme-switcher" ref={containerRef}>
      <button aria-expanded={open} aria-haspopup="dialog" aria-label="Choose theme and accent color" className="hf-theme-trigger" onClick={() => setOpen(!open)}>
        <PaletteIcon size={17} />
        <span className="hf-theme-trigger-dot" />
      </button>

      {open && (
        <div className="hf-theme-popover" role="dialog" aria-label="Appearance settings">
          <header><p>Appearance</p><span>Saved on this device</span></header>
          <div className="hf-theme-section">
            <p className="hf-theme-label">Theme</p>
            <div className="hf-theme-modes">
              {modes.map((item) => {
                const ModeIcon = item.icon
                return <button className={mode === item.id ? "hf-theme-mode-active" : ""} key={item.id} onClick={() => setMode(item.id)}><ModeIcon size={16} />{item.label}</button>
              })}
            </div>
          </div>
          <div className="hf-theme-section">
            <p className="hf-theme-label">Accent color</p>
            <div className="hf-accent-options">
              {accents.map((item) => <button key={item.id} onClick={() => setAccent(item.id)}><span className="hf-accent-swatch" style={{ backgroundColor: item.color }}>{accent === item.id && <CheckIcon size={11} weight="bold" />}</span><span>{item.label}</span></button>)}
            </div>
          </div>
          <footer>Primary actions, charts, and active navigation use this accent.</footer>
        </div>
      )}
    </div>
  )
}
