export type Theme = "light" | "dark";

const STORAGE_KEY = "hf-theme";

export function getInitialTheme(): Theme {
  if (typeof window === "undefined") {
    return "dark";
  }
  const stored = window.localStorage.getItem(STORAGE_KEY);
  if (stored === "light" || stored === "dark") {
    return stored;
  }
  if (window.matchMedia("(prefers-color-scheme: light)").matches) {
    return "light";
  }
  return "dark";
}

export function applyTheme(theme: Theme): void {
  if (typeof document === "undefined") {
    return;
  }
  document.documentElement.setAttribute("data-theme", theme);
}

export function persistTheme(theme: Theme): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(STORAGE_KEY, theme);
}

export function hasUserOverride(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.localStorage.getItem(STORAGE_KEY) !== null;
}

export function subscribeToSystemTheme(handler: (theme: Theme) => void): () => void {
  if (typeof window === "undefined" || !window.matchMedia) {
    return () => undefined;
  }
  const media = window.matchMedia("(prefers-color-scheme: light)");
  const listener = (event: MediaQueryListEvent) => {
    handler(event.matches ? "light" : "dark");
  };
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
}
