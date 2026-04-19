import { ReactNode, useCallback, useEffect, useState } from "react";
import { ProjectBreadcrumbProvider } from "../ProjectBreadcrumbContext";
import { resolveEffectiveTheme, useUIPrefs, type ThemePreference } from "../hooks/useUIPrefs";
import { applyTheme } from "../theme";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";

type ShellProps = {
  children: ReactNode;
  onLogout: () => void;
};

export function Shell({ children, onLogout }: ShellProps) {
  const { prefs, setPrefs } = useUIPrefs();
  const [, bump] = useState(0);

  useEffect(() => {
    if (prefs.theme !== "system") {
      return;
    }
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const fn = () => bump((x) => x + 1);
    mq.addEventListener("change", fn);
    return () => mq.removeEventListener("change", fn);
  }, [prefs.theme]);

  const effective = resolveEffectiveTheme(prefs);

  useEffect(() => {
    applyTheme(effective);
  }, [effective]);

  const onThemeCycle = useCallback(() => {
    const order: ThemePreference[] = ["dark", "light", "system"];
    const idx = order.indexOf(prefs.theme);
    setPrefs({ theme: order[(idx + 1) % order.length] });
  }, [prefs.theme, setPrefs]);

  return (
    <ProjectBreadcrumbProvider>
      <div className="grid h-screen grid-cols-[16rem_1fr] grid-rows-[3.5rem_1fr] bg-bg text-text">
        <Sidebar />
        <Topbar
          themePreference={prefs.theme}
          effectiveTheme={effective}
          onThemeCycle={onThemeCycle}
          onLogout={onLogout}
        />
        <main className="overflow-y-auto bg-bg p-6">{children}</main>
      </div>
    </ProjectBreadcrumbProvider>
  );
}
