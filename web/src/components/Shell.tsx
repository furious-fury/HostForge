import { ReactNode, useCallback, useEffect, useState } from "react";
import { ProjectBreadcrumbProvider } from "../ProjectBreadcrumbContext";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import {
  Theme,
  applyTheme,
  getInitialTheme,
  hasUserOverride,
  persistTheme,
  subscribeToSystemTheme,
} from "../theme";

type ShellProps = {
  children: ReactNode;
  onLogout: () => void;
};

export function Shell({ children, onLogout }: ShellProps) {
  const [theme, setTheme] = useState<Theme>(() => getInitialTheme());

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    return subscribeToSystemTheme((next) => {
      if (!hasUserOverride()) {
        setTheme(next);
      }
    });
  }, []);

  const handleThemeChange = useCallback((next: Theme) => {
    persistTheme(next);
    setTheme(next);
  }, []);

  return (
    <ProjectBreadcrumbProvider>
      <div className="grid h-screen grid-cols-[16rem_1fr] grid-rows-[3.5rem_1fr] bg-bg text-text">
        <Sidebar />
        <Topbar theme={theme} onThemeChange={handleThemeChange} onLogout={onLogout} />
        <main className="overflow-y-auto bg-bg p-6">{children}</main>
      </div>
    </ProjectBreadcrumbProvider>
  );
}
