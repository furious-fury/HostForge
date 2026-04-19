import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";

type ProjectEntry = { projectID: string; name: string };

const ProjectBreadcrumbContext = createContext<{
  entry: ProjectEntry | null;
  registerProject: (projectID: string, name: string) => void;
} | null>(null);

/**
 * Keeps the human-readable project name in sync with the URL for breadcrumbs and headers.
 * Clears when navigating away from /projects/:id routes (excluding /projects/new).
 */
export function ProjectBreadcrumbProvider({ children }: { children: ReactNode }) {
  const location = useLocation();
  const [entry, setEntry] = useState<ProjectEntry | null>(null);

  useEffect(() => {
    const segs = location.pathname.split("/").filter(Boolean);
    if (segs[0] !== "projects" || !segs[1] || segs[1] === "new") {
      setEntry(null);
      return;
    }
    const pid = segs[1];
    setEntry((prev) => (prev && prev.projectID === pid ? prev : null));
  }, [location.pathname]);

  const registerProject = useCallback((projectID: string, name: string) => {
    const trimmed = name.trim() || "Unnamed project";
    setEntry({ projectID, name: trimmed });
  }, []);

  const value = useMemo(() => ({ entry, registerProject }), [entry, registerProject]);

  return <ProjectBreadcrumbContext.Provider value={value}>{children}</ProjectBreadcrumbContext.Provider>;
}

export function useProjectBreadcrumb() {
  const ctx = useContext(ProjectBreadcrumbContext);
  if (!ctx) {
    throw new Error("useProjectBreadcrumb must be used within ProjectBreadcrumbProvider");
  }
  return ctx;
}
