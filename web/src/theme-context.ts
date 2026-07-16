import { createContext, useContext } from "react"

export type ThemeMode = "light" | "dark" | "system"
export type Accent = "neutral" | "ocean" | "indigo" | "cyan" | "forest" | "amber" | "rose" | "violet"

export type ThemeContextValue = {
  accent: Accent
  mode: ThemeMode
  setAccent: (accent: Accent) => void
  setMode: (mode: ThemeMode) => void
}

export const ThemeContext = createContext<ThemeContextValue | null>(null)

export function useTheme() {
  const context = useContext(ThemeContext)
  if (!context) throw new Error("useTheme must be used inside ThemeProvider")
  return context
}
