import { Search } from "lucide-react";
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

  const crumbs: Crumb[] = [{ label: "Dashboard", to: "/" }];

  if (segments.length === 0) return crumbs;

  if (segments[0] === "projects") {
    crumbs.push({ label: "Applications", to: "/projects" });
    if (segments[1] === "new") {
      crumbs.push({ label: "New application" });
    } else if (segments[1]) {
      const projectID = params.projectID || segments[1];
      const projectLabel = projectEntry?.projectID === projectID ? projectEntry.name : shortenID(projectID);
      crumbs.push({ label: projectLabel, to: `/projects/${projectID}` });
      if (segments[2] === "deployments" && segments[3]) {
        const deploymentID = params.deploymentID || segments[3];
        crumbs.push({ label: `Deployment ${shortenID(deploymentID)}` });
      } else if (segments[2] === "settings") {
        crumbs.push({ label: "Settings" });
      }
    }
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
    <header className="border-b border-border bg-surface px-4 py-3 sm:px-5 lg:px-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-2 text-sm">
          {crumbs.map((crumb, idx) => {
            const last = idx === crumbs.length - 1;
            return (
              <span key={`crumb-${idx}`} className="flex min-w-0 max-w-[18rem] items-center gap-2">
                {idx > 0 && <span className="text-muted" aria-hidden>/</span>}
                {crumb.to && !last ? (
                  <Link to={crumb.to} className="min-w-0 truncate text-muted hover:text-text" title={crumb.label}>
                    {crumb.label}
                  </Link>
                ) : (
                  <span className={last ? "min-w-0 truncate font-medium text-text" : "min-w-0 truncate text-muted"} title={crumb.label}>
                    {crumb.label}
                  </span>
                )}
              </span>
            );
          })}
        </nav>

        <div className="flex flex-1 flex-col gap-3 lg:max-w-[34rem] lg:flex-row lg:items-center lg:justify-end">
          <label className="relative flex min-w-0 flex-1 items-center">
            <Search className="pointer-events-none absolute left-3 h-4 w-4 text-muted" />
            <input
              ref={searchFieldRef}
              type="text"
              readOnly
              placeholder="Search"
              className="w-full rounded-md border border-border bg-bg px-3 py-2 pl-9 pr-16 text-sm text-text placeholder:text-muted focus:border-border-strong focus:outline-none"
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
            <span className="mono pointer-events-none absolute right-3 text-[10px] uppercase tracking-[0.16em] text-muted">
              {modKey}K
            </span>
          </label>

          <div className="flex items-center gap-2 self-end lg:self-auto">
            <ThemeToggle preference={theme} onCycle={onThemeCycle} />
            <Button variant="ghost" size="sm" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </div>
      </div>
    </header>
  );
}
