import { Link, useLocation, useParams } from "react-router-dom";
import { Theme } from "../theme";
import { ButtonLink } from "./Button";
import { ThemeToggle } from "./ThemeToggle";

type TopbarProps = {
  theme: Theme;
  onThemeChange: (theme: Theme) => void;
};

type Crumb = { label: string; to?: string };

function useBreadcrumbs(): Crumb[] {
  const location = useLocation();
  const params = useParams();
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
      crumbs.push({ label: shortenID(projectID), to: `/projects/${projectID}` });
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

export function Topbar({ theme, onThemeChange }: TopbarProps) {
  const crumbs = useBreadcrumbs();

  return (
    <header className="flex h-14 items-center justify-between border-b border-border bg-surface px-6">
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm">
        {crumbs.map((crumb, idx) => {
          const last = idx === crumbs.length - 1;
          return (
            <span key={`${crumb.label}-${idx}`} className="flex items-center gap-2">
              {idx > 0 && <span className="text-muted" aria-hidden>/</span>}
              {crumb.to && !last ? (
                <Link to={crumb.to} className="text-muted hover:text-text">
                  {crumb.label}
                </Link>
              ) : (
                <span className={last ? "font-semibold text-text" : "text-muted"}>{crumb.label}</span>
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
            type="text"
            placeholder="Search projects, deployments, domains"
            className="mono w-full border border-border bg-surface-alt px-3 py-2 pl-7 text-xs text-text placeholder:text-muted focus:border-border-strong focus:outline-none"
            aria-label="Search"
          />
          <span className="mono pointer-events-none absolute right-3 border border-border px-1.5 py-0.5 text-[10px] uppercase text-muted">
            ⌘K
          </span>
        </label>
      </div>

      <div className="flex items-center gap-2">
        <ThemeToggle theme={theme} onChange={onThemeChange} />
        <ButtonLink to="/projects/new" variant="primary" size="sm">
          + New Project
        </ButtonLink>
      </div>
    </header>
  );
}
