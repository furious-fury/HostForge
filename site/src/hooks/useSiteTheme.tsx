import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

/** `useLayoutEffect` on the client only — avoids SSR warnings from vite-react-ssg. */
const useIsomorphicLayoutEffect = typeof window !== "undefined" ? useLayoutEffect : useEffect;

/** Same key/event as control-plane `web/` so theme can stay in sync on the same origin. */
export const PREFS_STORAGE_KEY = "hf-prefs";
export const PREFS_CHANGED_EVENT = "hf-prefs-changed";

/** Marketing site: explicit light/dark only (no system). */
export type ThemePreference = "light" | "dark";

type MergedPrefs = {
  theme: ThemePreference;
  defaultLanding: string;
  deploymentsPageSize: number;
  logAutoScroll: boolean;
  numericLocale: string;
};

const DEFAULT_PREFS: MergedPrefs = {
  theme: "dark",
  defaultLanding: "/",
  deploymentsPageSize: 50,
  logAutoScroll: true,
  numericLocale: "en-US",
};

/** Map stored prefs (including dashboard `system`) to a concrete site theme. */
function normalizeSiteTheme(v: unknown): ThemePreference {
  if (v === "light") return "light";
  if (v === "dark") return "dark";
  return "dark";
}

export function loadMergedPrefs(): MergedPrefs {
  if (typeof window === "undefined") {
    return { ...DEFAULT_PREFS };
  }
  try {
    const raw = window.localStorage.getItem(PREFS_STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      return {
        ...DEFAULT_PREFS,
        ...parsed,
        theme: normalizeSiteTheme(parsed.theme),
      } as MergedPrefs;
    }
    const legacy = window.localStorage.getItem("hf-theme");
    if (legacy === "light" || legacy === "dark") {
      return { ...DEFAULT_PREFS, theme: legacy };
    }
  } catch {
    return { ...DEFAULT_PREFS };
  }
  return { ...DEFAULT_PREFS };
}

export function persistMergedPrefs(next: MergedPrefs): void {
  try {
    window.localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify(next));
    window.dispatchEvent(new Event(PREFS_CHANGED_EVENT));
  } catch {
    // private mode / quota
  }
}

function applyDomTheme(mode: "light" | "dark"): void {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", mode);
  document.documentElement.classList.toggle("dark", mode === "dark");
}

function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

type SiteThemeContextValue = {
  preference: ThemePreference;
  cycleTheme: () => void;
};

const SiteThemeContext = createContext<SiteThemeContextValue | null>(null);

export function SiteThemeProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<MergedPrefs>(() => loadMergedPrefs());
  const themeLayoutRan = useRef(false);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === PREFS_STORAGE_KEY || e.key === "hf-theme") {
        setPrefs(loadMergedPrefs());
      }
    };
    const onLocal = () => setPrefs(loadMergedPrefs());
    window.addEventListener("storage", onStorage);
    window.addEventListener(PREFS_CHANGED_EVENT, onLocal);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(PREFS_CHANGED_EVENT, onLocal);
    };
  }, []);

  const preference = prefs.theme;

  /** Sync `<html>` with theme, using View Transitions when supported (smooth cross-fade). */
  useIsomorphicLayoutEffect(() => {
    const run = () => applyDomTheme(preference);
    if (!themeLayoutRan.current) {
      themeLayoutRan.current = true;
      run();
      return;
    }
    if (prefersReducedMotion() || typeof document.startViewTransition !== "function") {
      run();
      return;
    }
    document.startViewTransition(run);
  }, [preference]);

  const cycleTheme = useCallback(() => {
    const next: ThemePreference = prefs.theme === "dark" ? "light" : "dark";
    const merged = { ...prefs, theme: next };
    persistMergedPrefs(merged);
    setPrefs(merged);
  }, [prefs]);

  const value = useMemo(() => ({ preference, cycleTheme }), [preference, cycleTheme]);

  return <SiteThemeContext.Provider value={value}>{children}</SiteThemeContext.Provider>;
}

export function useSiteTheme(): SiteThemeContextValue {
  const ctx = useContext(SiteThemeContext);
  if (!ctx) {
    throw new Error("useSiteTheme must be used within SiteThemeProvider");
  }
  return ctx;
}
