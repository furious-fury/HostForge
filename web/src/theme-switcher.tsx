import { CheckIcon, DesktopIcon, MoonIcon, PaletteIcon, SunIcon } from "@phosphor-icons/react"

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
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
  { id: "indigo", label: "Indigo", color: "#6869c9" },
  { id: "cyan", label: "Cyan", color: "#318c9b" },
  { id: "forest", label: "Forest", color: "#4b8a6d" },
  { id: "amber", label: "Amber", color: "#c65318" },
  { id: "rose", label: "Rose", color: "#bd6b7c" },
  { id: "violet", label: "Violet", color: "#8b64bf" },
]

export function ThemeSwitcher() {
  const { accent, mode, setAccent, setMode } = useTheme()

  return <Popover><PopoverTrigger asChild><button aria-label="Choose theme and accent color" className="hf-theme-trigger"><PaletteIcon size={17} /><span className="hf-theme-trigger-dot" /></button></PopoverTrigger><PopoverContent align="end" sideOffset={8} className="hf-theme-popover p-0"><header><p>Appearance</p><span>Saved on this device</span></header><div className="hf-theme-section"><p className="hf-theme-label">Theme</p><div className="hf-theme-modes">{modes.map((item) => { const ModeIcon = item.icon; return <button className={mode === item.id ? "hf-theme-mode-active" : ""} key={item.id} onClick={() => setMode(item.id)}><ModeIcon size={16} />{item.label}</button> })}</div></div><div className="hf-theme-section"><p className="hf-theme-label">Accent color</p><div className="hf-accent-options">{accents.map((item) => <button key={item.id} onClick={() => setAccent(item.id)}><span className="hf-accent-swatch" style={{ backgroundColor: item.color }}>{accent === item.id && <CheckIcon size={11} weight="bold" />}</span><span>{item.label}</span></button>)}</div></div><footer>Primary actions, charts, and active navigation use this accent.</footer></PopoverContent></Popover>
}
