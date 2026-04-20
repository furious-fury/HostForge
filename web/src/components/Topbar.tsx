import { useCallback, useMemo, useRef } from "react";
import { Link, useLocation, useParams } from "react-router-dom";
import type { ThemePreference } from "../hooks/useUIPrefs";
import { useProjectBreadcrumb } from "../ProjectBreadcrumbContext";
import { Button } from "./Button";
import { ThemeToggle } from "./ThemeToggle";

type TopbarProps = {
  theme: ThemePreference;
  onThemeCycle: () => void;
  onLogout: () => void;
  onOpenCommandPalette: () => void;
};

type Crumb = { label: string; to?: string };

function useBreadcrumbs(): Crumb[] {
  const location = useLocation();
  const params = useParams();
  const { entry: projectEntry } = useProjectBreadcrumb();
  const segments = location.pathname.split("/").filter(Boolean);

  const crumbs: Crumb[] = [{ label: "Overview", to: "/" }];

  if (segments.length === 0) {
    return crumbs;
  }

  if (segments[0] === "projects") {
    crumbs.push({ label: "Projects", to: "/projects" });
    if (segments[1] === "new") {
      crumbs.push({ label: "New Project" });
    } else if (segments[1]) {
      const projectID = params.projectID || segments[1];
      const projectLabel =
        projectEntry?.projectID === projectID ? projectEntry.name : shortenID(projectID);
      crumbs.push({ label: projectLabel, to: `/projects/${projectID}` });
      if (segments[2] === "deployments" && segments[3]) {
        const deploymentID = params.deploymentID || segments[3];
        crumbs.push({ label: `Deployment ${shortenID(deploymentID)}` });
      }
    }
    return crumbs;
  }

  if (segments[0] === "deployments") {
    crumbs.push({ label: "Deployments", to: "/deployments" });
    return crumbs;
  }

  if (segments[0] === "settings") {
    crumbs.push({ label: "Settings", to: "/settings" });
    return crumbs;
  }

  crumbs.push({ label: titleize(segments[0]) });
  return crumbs;
}

function shortenID(id: string): string {
  if (id.length <= 10) return id;
  return `${id.slice(0, 8)}…`;
}

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function useModKeyLabel(): string {
  return useMemo(() => {
    if (typeof navigator === "undefined") return "Ctrl";
    return /Mac|iPhone|iPod|iPad/i.test(navigator.userAgent) ? "⌘" : "Ctrl";
  }, []);
}

export function Topbar({ theme, onThemeCycle, onLogout, onOpenCommandPalette }: TopbarProps) {
  const crumbs = useBreadcrumbs();
  const modKey = useModKeyLabel();
  const searchFieldRef = useRef<HTMLInputElement>(null);

  const openPalette = useCallback(() => {
    onOpenCommandPalette();
    requestAnimationFrame(() => {
      searchFieldRef.current?.blur();
    });
  }, [onOpenCommandPalette]);

  return (
    <header className="flex h-14 items-center justify-between border-b border-border bg-surface px-6">
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm">
        {crumbs.map((crumb, idx) => {
          const last = idx === crumbs.length - 1;
          return (
            <span key={`crumb-${idx}`} className="flex min-w-0 max-w-[18rem] items-center gap-2">
              {idx > 0 && <span className="shrink-0 text-muted" aria-hidden>/</span>}
              {crumb.to && !last ? (
                <Link
                  to={crumb.to}
                  className="min-w-0 truncate text-muted hover:text-text"
                  title={crumb.label}
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  className={`min-w-0 truncate ${last ? "font-semibold text-text" : "text-muted"}`}
                  title={crumb.label}
                >
                  {crumb.label}
                </span>
              )}
            </span>
          );
        })}
      </nav>

      <div className="flex flex-1 justify-center px-8">
        <label className="relative flex w-full max-w-md items-center">
          <span className="mono pointer-events-none absolute left-3 text-[11px] font-semibold uppercase tracking-wider text-muted">
            ⌕
          </span>
          <input
            ref={searchFieldRef}
            type="text"
            readOnly
            placeholder="Search projects and deployments"
            className="mono w-full cursor-pointer border border-border bg-surface-alt px-3 py-2 pl-7 text-xs text-text placeholder:text-muted focus:border-border-strong focus:outline-none"
            aria-label="Open command palette"
            aria-haspopup="dialog"
            onClick={openPalette}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                openPalette();
              }
            }}
          />
          <span className="mono pointer-events-none absolute right-3 border border-border px-1.5 py-0.5 text-[10px] uppercase text-muted">
            {modKey}K
          </span>
        </label>
      </div>

      <div className="flex items-center gap-2">
        <ThemeToggle preference={theme} onCycle={onThemeCycle} />
        <Button variant="ghost" size="sm" onClick={onLogout}>
          Logout
        </Button>
      </div>
    </header>
  );
}
