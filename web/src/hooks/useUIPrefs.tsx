import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { resolveFormatLocale } from "../format";

const STORAGE_KEY = "hf-prefs";
const LEGACY_THEME_KEY = "hf-theme";

/** Dispatched on same-tab pref writes so multiple subscribers refresh. */
export const PREFS_CHANGED_EVENT = "hf-prefs-changed";

export type ThemePreference = "light" | "dark" | "system";
export type LandingPath = "/" | "/projects" | "/deployments";
export type DeploymentsPageSize = 25 | 50 | 100 | 200;
export type UIPrefs = {
  theme: ThemePreference;
  defaultLanding: LandingPath;
  deploymentsPageSize: DeploymentsPageSize;
  /** When true, live log views start in auto-scroll (not paused). */
  logAutoScroll: boolean;
  numericLocale: "en-US" | "system";
};

export const DEFAULT_UI_PREFS: UIPrefs = {
  theme: "system",
  defaultLanding: "/",
  deploymentsPageSize: 50,
  logAutoScroll: true,
  numericLocale: "en-US",
};

function isDeploymentsPageSize(n: number): n is DeploymentsPageSize {
  return n === 25 || n === 50 || n === 100 || n === 200;
}

function normalizePartial(raw: Record<string, unknown>): Partial<UIPrefs> {
  const out: Partial<UIPrefs> = {};
  if (raw.theme === "light" || raw.theme === "dark" || raw.theme === "system") {
    out.theme = raw.theme;
  }
  if (raw.defaultLanding === "/" || raw.defaultLanding === "/projects" || raw.defaultLanding === "/deployments") {
    out.defaultLanding = raw.defaultLanding;
  }
  if (typeof raw.deploymentsPageSize === "number" && isDeploymentsPageSize(raw.deploymentsPageSize)) {
    out.deploymentsPageSize = raw.deploymentsPageSize;
  }
  if (typeof raw.logAutoScroll === "boolean") {
    out.logAutoScroll = raw.logAutoScroll;
  }
  if (raw.numericLocale === "en-US" || raw.numericLocale === "system") {
    out.numericLocale = raw.numericLocale as "en-US" | "system";
  }
  return out;
}

function readPartialFromStorage(): Partial<UIPrefs> {
  if (typeof window === "undefined") {
    return {};
  }
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      return normalizePartial(parsed);
    }
    const legacy = window.localStorage.getItem(LEGACY_THEME_KEY);
    if (legacy === "light" || legacy === "dark") {
      return { theme: legacy };
    }
  } catch {
    return {};
  }
  return {};
}

export function loadUIPrefs(): UIPrefs {
  return { ...DEFAULT_UI_PREFS, ...readPartialFromStorage() };
}

export function resolveEffectiveTheme(prefs: UIPrefs): "light" | "dark" {
  if (prefs.theme !== "system") {
    return prefs.theme;
  }
  if (typeof window === "undefined" || !window.matchMedia) {
    return "dark";
  }
  return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

type UIPrefsContextValue = {
  prefs: UIPrefs;
  setPrefs: (patch: Partial<UIPrefs>) => void;
  resetUIPrefs: () => void;
};

const UIPrefsContext = createContext<UIPrefsContextValue | null>(null);

export function UIPrefsProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefsState] = useState<UIPrefs>(() => loadUIPrefs());

  const setPrefs = useCallback((patch: Partial<UIPrefs>) => {
    setPrefsState((prev) => {
      const next = { ...prev, ...patch };
      try {
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
        window.dispatchEvent(new Event(PREFS_CHANGED_EVENT));
      } catch {
        // ignore quota / private mode
      }
      return next;
    });
  }, []);

  const resetUIPrefs = useCallback(() => {
    try {
      window.localStorage.removeItem(STORAGE_KEY);
      window.localStorage.removeItem(LEGACY_THEME_KEY);
    } catch {
      //
    }
    const fresh = { ...DEFAULT_UI_PREFS };
    setPrefsState(fresh);
    window.dispatchEvent(new Event(PREFS_CHANGED_EVENT));
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY || e.key === LEGACY_THEME_KEY) {
        setPrefsState(loadUIPrefs());
      }
    };
    const onLocal = () => setPrefsState(loadUIPrefs());
    window.addEventListener("storage", onStorage);
    window.addEventListener(PREFS_CHANGED_EVENT, onLocal);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(PREFS_CHANGED_EVENT, onLocal);
    };
  }, []);

  const value = useMemo(() => ({ prefs, setPrefs, resetUIPrefs }), [prefs, setPrefs, resetUIPrefs]);
  return <UIPrefsContext.Provider value={value}>{children}</UIPrefsContext.Provider>;
}

export function useUIPrefs(): UIPrefsContextValue {
  const ctx = useContext(UIPrefsContext);
  if (!ctx) {
    throw new Error("useUIPrefs must be used within UIPrefsProvider");
  }
  return ctx;
}

/** Browser locale string for dates/relative times from Preferences. */
export function useFormatLocale(): string {
  const { prefs } = useUIPrefs();
  return resolveFormatLocale(prefs.numericLocale);
}
