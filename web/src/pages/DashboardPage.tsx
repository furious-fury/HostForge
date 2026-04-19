import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  ApiDeployment,
  ApiProject,
  fetchAllDeployments,
  fetchProjects,
} from "../api";
import { ButtonLink } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { KpiTile } from "../components/KpiTile";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { formatDuration, formatRelative, shortHash } from "../format";

const DAY_MS = 24 * 60 * 60 * 1000;

export function DashboardPage() {
  const [projects, setProjects] = useState<ApiProject[]>([]);
  const [deployments, setDeployments] = useState<ApiDeployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        const [p, d] = await Promise.all([
          fetchProjects(),
          fetchAllDeployments(30).catch(() => [] as ApiDeployment[]),
        ]);
        if (!cancelled) {
          setProjects(p);
          setDeployments(d);
          setError("");
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load dashboard");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const projectByID = useMemo(() => {
    const map = new Map<string, ApiProject>();
    for (const p of projects) {
      map.set(p.id, p);
    }
    return map;
  }, [projects]);

  const stats = useMemo(() => {
    const cutoff = Date.now() - DAY_MS;
    let deploys24 = 0;
    let failed24 = 0;
    for (const d of deployments) {
      const ts = Date.parse(d.created_at);
      if (Number.isNaN(ts) || ts < cutoff) continue;
      deploys24 += 1;
      if (d.status?.toUpperCase() === "FAILED") failed24 += 1;
    }
    let runningContainers = 0;
    for (const p of projects) {
      if (p.current_container?.status?.toUpperCase() === "RUNNING") {
        runningContainers += 1;
      }
    }
    return {
      activeProjects: projects.length,
      deploys24,
      failed24,
      runningContainers,
    };
  }, [projects, deployments]);

  const recent = useMemo(() => deployments.slice(0, 5), [deployments]);

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Overview</div>
          <h1 className="text-2xl font-semibold tracking-tight">Fleet status</h1>
          <p className="mt-1 text-sm text-muted">
            KPIs and a quick snapshot of recent activity. Full deployment history lives on{" "}
            <Link to="/deployments" className="text-text underline decoration-border-strong underline-offset-2 hover:text-primary">
              Deployments
            </Link>
            .
          </p>
        </div>
        <div className="flex items-center gap-2">
          <ButtonLink to="/deployments" variant="secondary" size="sm">
            All deployments
          </ButtonLink>
          <ButtonLink to="/projects" variant="secondary" size="sm">Open Projects</ButtonLink>
          <ButtonLink to="/projects/new" variant="primary" size="sm">+ New Project</ButtonLink>
        </div>
      </header>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <KpiTile label="Active Projects" value={stats.activeProjects} hint="Projects registered with the control plane" />
        <KpiTile
          label="Deploys (24h)"
          value={stats.deploys24}
          hint="Total deployments started in the last day"
          tone={stats.deploys24 > 0 ? "info" : "default"}
        />
        <KpiTile
          label="Failed (24h)"
          value={stats.failed24}
          hint={stats.failed24 === 0 ? "No failures detected" : "Investigate failed deploys"}
          tone={stats.failed24 > 0 ? "danger" : "success"}
        />
        <KpiTile
          label="Containers Running"
          value={stats.runningContainers}
          hint="Currently active runtime containers"
          tone={stats.runningContainers > 0 ? "success" : "default"}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <Panel
          className="xl:col-span-2"
          title="Recent activity"
          actions={
            <div className="flex flex-wrap items-center gap-3">
              <Link
                to="/deployments"
                className="mono text-[11px] font-semibold uppercase tracking-wider text-muted hover:text-text"
              >
                All deployments →
              </Link>
              <Link to="/projects" className="mono text-[11px] font-semibold uppercase tracking-wider text-muted hover:text-text">
                Projects →
              </Link>
            </div>
          }
          noBody
        >
          {loading && recent.length === 0 ? (
            <div className="p-6 text-sm text-muted">Loading deployments…</div>
          ) : recent.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No deployments yet"
                description="Start by creating a project from a GitHub repository. Deployments will stream here as they run."
                action={<ButtonLink to="/projects/new" variant="primary" size="sm">+ New Project</ButtonLink>}
              />
            </div>
          ) : (
            <table className="w-full table-fixed text-sm">
              <thead>
                <tr className="mono border-b border-border text-left text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="px-4 py-2 w-[28%]">Project</th>
                  <th className="px-4 py-2 w-[18%]">Commit</th>
                  <th className="px-4 py-2 w-[18%]">Status</th>
                  <th className="px-4 py-2 w-[18%]">Started</th>
                  <th className="px-4 py-2 w-[18%]">Duration</th>
                </tr>
              </thead>
              <tbody>
                {recent.map((d) => {
                  const proj = projectByID.get(d.project_id);
                  const projectName = proj?.name || shortHash(d.project_id, 8);
                  return (
                    <tr
                      key={d.id}
                      className="border-b border-border/60 hover:bg-surface-alt"
                    >
                      <td className="px-4 py-3 truncate">
                        <Link
                          to={`/projects/${d.project_id}/deployments/${d.id}`}
                          className="font-semibold text-text hover:underline"
                        >
                          {projectName}
                        </Link>
                        {proj?.repo_url && (
                          <div className="mono truncate text-[11px] text-muted">{proj.repo_url}</div>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className="mono text-xs text-text">{shortHash(d.commit_hash, 7)}</span>
                      </td>
                      <td className="px-4 py-3">
                        <StatusPill status={d.status} size="sm" />
                      </td>
                      <td className="px-4 py-3 text-xs text-muted">{formatRelative(d.created_at)}</td>
                      <td className="px-4 py-3 mono text-xs text-text">
                        {formatDuration(d.created_at, d.updated_at)}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </Panel>

        <Panel title="System">
          <ul className="flex flex-col divide-y divide-border">
            <li className="flex items-center justify-between py-2 text-sm">
              <span className="text-muted">Docker daemon</span>
              <StatusPill status="RUNNING" size="sm" />
            </li>
            <li className="flex items-center justify-between py-2 text-sm">
              <span className="text-muted">Caddy admin</span>
              <StatusPill status="READY" size="sm" />
            </li>
            <li className="flex items-center justify-between py-2 text-sm">
              <span className="text-muted">Webhook listener</span>
              <StatusPill status="READY" size="sm" />
            </li>
            <li className="flex items-center justify-between py-2 text-sm">
              <span className="text-muted">Build version</span>
              <span className="mono text-xs text-text">v0.6.0 · phase 6</span>
            </li>
          </ul>
          <div className="mt-4 border-t border-border pt-4">
            <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Quick Actions</div>
            <div className="flex flex-col gap-2">
              <ButtonLink to="/projects/new" variant="primary" size="sm">+ New Project</ButtonLink>
              <ButtonLink to="/projects" variant="secondary" size="sm">Open Projects</ButtonLink>
            </div>
          </div>
        </Panel>
      </div>
    </div>
  );
}
