import { useEffect, useLayoutEffect, useState } from "react"

import { type Accent, ThemeContext, type ThemeMode } from "@/theme-context"

import "@/theme-accents.css"

function isThemeMode(value: string | null): value is ThemeMode {
  return value === "light" || value === "dark" || value === "system"
}

function isAccent(value: string | null): value is Accent {
  return value === "neutral" || value === "ocean" || value === "indigo" || value === "cyan" || value === "forest" || value === "amber" || value === "rose" || value === "violet"
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setMode] = useState<ThemeMode>(() => {
    const saved = window.localStorage.getItem("hostforge-theme")
    return isThemeMode(saved) ? saved : "system"
  })
  const [accent, setAccent] = useState<Accent>(() => {
    const saved = window.localStorage.getItem("hostforge-accent")
    return isAccent(saved) ? saved : "neutral"
  })

  useLayoutEffect(() => {
    const root = document.documentElement
    const media = window.matchMedia("(prefers-color-scheme: dark)")
    const applyMode = () => root.classList.toggle("dark", mode === "dark" || (mode === "system" && media.matches))

    root.dataset.theme = mode
    root.dataset.accent = accent
    applyMode()
    media.addEventListener("change", applyMode)

    return () => media.removeEventListener("change", applyMode)
  }, [accent, mode])

  useEffect(() => {
    window.localStorage.setItem("hostforge-theme", mode)
    window.localStorage.setItem("hostforge-accent", accent)
  }, [accent, mode])

  return <ThemeContext value={{ accent, mode, setAccent, setMode }}>{children}</ThemeContext>
}
