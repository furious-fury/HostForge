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
      <div className="min-h-screen bg-transparent px-4 py-4 text-text sm:px-5 sm:py-5">
        <div className="grid min-h-[calc(100vh-2rem)] grid-cols-1 gap-4 lg:grid-cols-[18rem_minmax(0,1fr)] lg:grid-rows-[5rem_minmax(0,1fr)]">
          <Sidebar />
          <Topbar
            theme={prefs.theme}
            onThemeCycle={onThemeCycle}
            onLogout={onLogout}
            onOpenCommandPalette={() => setCommandPaletteOpen(true)}
          />
          <a className="skip-link" href="#main-content">
            Skip to content
          </a>
          <main
            id="main-content"
            tabIndex={-1}
            className="overflow-y-auto rounded-[28px] border border-border bg-surface/80 p-5 shadow-[var(--hf-shadow-panel)] backdrop-blur-xl sm:p-6 lg:p-8"
          >
            <div className="mx-auto max-w-[1480px]">{children}</div>
          </main>
          <CommandPalette open={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} />
        </div>
      </div>
    </ProjectBreadcrumbProvider>
  );
}
