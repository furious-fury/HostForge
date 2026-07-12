import { Search, Sparkles } from "lucide-react";
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

  if (segments.length === 0) {
    return crumbs;
  }

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
        crumbs.push({ label: "Application settings" });
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
    <header className="flex flex-col justify-between gap-4 rounded-[28px] border border-border bg-surface/80 px-5 py-4 shadow-[var(--hf-shadow-panel)] backdrop-blur-xl sm:px-6 lg:h-20 lg:flex-row lg:items-center">
      <div className="min-w-0">
        <div className="mono mb-2 inline-flex items-center gap-2 rounded-full border border-border bg-surface-alt/80 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.22em] text-muted">
          <Sparkles className="h-3.5 w-3.5" />
          Application Workspace
        </div>
        <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-2 text-sm">
          {crumbs.map((crumb, idx) => {
            const last = idx === crumbs.length - 1;
            return (
              <span key={`crumb-${idx}`} className="flex min-w-0 max-w-[18rem] items-center gap-2">
                {idx > 0 && <span className="shrink-0 text-muted/70" aria-hidden>/</span>}
                {crumb.to && !last ? (
                  <Link to={crumb.to} className="min-w-0 truncate text-muted transition-colors hover:text-text" title={crumb.label}>
                    {crumb.label}
                  </Link>
                ) : (
                  <span className={`min-w-0 truncate ${last ? "font-semibold text-text" : "text-muted"}`} title={crumb.label}>
                    {crumb.label}
                  </span>
                )}
              </span>
            );
          })}
        </nav>
      </div>

      <div className="flex flex-1 flex-col gap-3 lg:max-w-[42rem] lg:flex-row lg:items-center lg:justify-end">
        <label className="relative flex min-w-0 flex-1 items-center">
          <Search className="pointer-events-none absolute left-3 h-4 w-4 text-muted" />
          <input
            ref={searchFieldRef}
            type="text"
            readOnly
            placeholder="Search applications, deployments, and settings"
            className="w-full rounded-2xl border border-border bg-surface-alt/80 px-3 py-2.5 pl-9 pr-16 text-sm text-text placeholder:text-muted focus:border-border-strong focus:outline-none"
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
          <span className="mono pointer-events-none absolute right-3 rounded-lg border border-border bg-surface px-2 py-1 text-[10px] uppercase tracking-[0.16em] text-muted">
            {modKey}K
          </span>
        </label>

        <div className="flex items-center gap-2 self-end lg:self-auto">
          <ThemeToggle preference={theme} onCycle={onThemeCycle} />
          <Button variant="ghost" size="sm" onClick={onLogout} className="rounded-xl">
            Logout
          </Button>
        </div>
      </div>
    </header>
  );
}
