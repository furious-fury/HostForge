import { ReactNode, useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ProjectBreadcrumbProvider } from "../ProjectBreadcrumbContext";
import { useUIPrefs } from "../hooks/useUIPrefs";
import { applyTheme } from "../theme";
import { CommandPalette } from "./CommandPalette";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";

type ShellProps = {
  children: ReactNode;
  onLogout: () => void;
};

function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

export function Shell({ children, onLogout }: ShellProps) {
  const { prefs, setPrefs } = useUIPrefs();
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const themeLayoutRan = useRef(false);

  useLayoutEffect(() => {
    const mode = prefs.theme;
    const run = () => applyTheme(mode);
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
  }, [prefs.theme]);

  const onThemeCycle = useCallback(() => {
    setPrefs({ theme: prefs.theme === "dark" ? "light" : "dark" });
  }, [prefs.theme, setPrefs]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key !== "k" && e.key !== "K") return;
      e.preventDefault();
      setCommandPaletteOpen((open) => !open);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  return (
    <ProjectBreadcrumbProvider>
      <div className="min-h-screen bg-bg text-text">
        <a className="skip-link" href="#main-content">
          Skip to content
        </a>
        <div className="grid min-h-screen grid-cols-1 lg:grid-cols-[240px_minmax(0,1fr)]">
          <Sidebar />
          <div className="min-w-0">
            <Topbar
              theme={prefs.theme}
              onThemeCycle={onThemeCycle}
              onLogout={onLogout}
              onOpenCommandPalette={() => setCommandPaletteOpen(true)}
            />
            <main id="main-content" tabIndex={-1} className="min-w-0 p-4 sm:p-5 lg:p-6">
              <div className="mx-auto max-w-[1440px]">{children}</div>
            </main>
          </div>
          <CommandPalette open={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} />
        </div>
      </div>
    </ProjectBreadcrumbProvider>
  );
}
