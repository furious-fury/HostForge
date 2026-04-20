import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ApiDeployment, ApiProject } from "../api";
import { Button, ButtonLink } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { formatDuration, formatRelative, shortHash } from "../format";
import { useDeploymentsListQuery, useProjectsQuery } from "../hooks/fleetQueries";
import { useFormatLocale, useUIPrefs } from "../hooks/useUIPrefs";

type StatusFilter = "all" | "building" | "success" | "failed";

const MAX_DEPLOYMENTS = 200;

function normStatus(s: string | undefined): string {
  return (s || "").toUpperCase();
}

export function DeploymentsPage() {
  const { prefs } = useUIPrefs();
  const pageStep = prefs.deploymentsPageSize;
  const [listLimit, setListLimit] = useState<number>(pageStep);
  const fmtLocale = useFormatLocale();

  useEffect(() => {
    setListLimit(pageStep);
  }, [pageStep]);

  const projectsQ = useProjectsQuery({ refetchWhileInFlight: true });
  const deploysQ = useDeploymentsListQuery(listLimit, {
    keepPreviousWhileFetching: true,
    refetchWhileInFlight: true,
  });

  const projects: ApiProject[] = projectsQ.data ?? [];
  const deployments: ApiDeployment[] = deploysQ.data ?? [];
  const tableLoading = deploysQ.isPending && deploysQ.data === undefined;
  const error =
    deploysQ.isError && deploysQ.error instanceof Error
      ? deploysQ.error.message
      : projectsQ.isError && projectsQ.error instanceof Error
        ? projectsQ.error.message
        : "";

  const [filter, setFilter] = useState<StatusFilter>("all");

  const projectByID = useMemo(() => {
    const map = new Map<string, ApiProject>();
    for (const p of projects) {
      map.set(p.id, p);
    }
    return map;
  }, [projects]);

  const filtered = useMemo(() => {
    return deployments.filter((d) => {
      const st = normStatus(d.status);
      switch (filter) {
        case "building":
          return st === "QUEUED" || st === "BUILDING";
        case "success":
          return st === "SUCCESS";
        case "failed":
          return st === "FAILED";
        default:
          return true;
      }
    });
  }, [deployments, filter]);

  const counts = useMemo(() => {
    let building = 0;
    let success = 0;
    let failed = 0;
    for (const d of deployments) {
      const st = normStatus(d.status);
      if (st === "QUEUED" || st === "BUILDING") building += 1;
      if (st === "SUCCESS") success += 1;
      if (st === "FAILED") failed += 1;
    }
    return { all: deployments.length, building, success, failed };
  }, [deployments]);

  const canLoadMore = listLimit < MAX_DEPLOYMENTS && deployments.length >= listLimit;

  function loadMore() {
    setListLimit((n) => Math.min(MAX_DEPLOYMENTS, n + pageStep));
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Deployments</div>
          <h1 className="text-2xl font-semibold tracking-tight">Build history</h1>
          <p className="mt-1 text-sm text-muted">
            Every deployment across the fleet. Status filters match deployment rows:{" "}
            <span className="font-medium text-text">Building</span> is <span className="mono">QUEUED</span> or{" "}
            <span className="mono">BUILDING</span>; <span className="font-medium text-text">Success</span> /{" "}
            <span className="font-medium text-text">Failed</span> are terminal outcomes. For KPIs and a short snapshot, see{" "}
            <Link to="/" className="text-text underline decoration-border-strong underline-offset-2 hover:text-primary">
              Overview
            </Link>
            .
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ButtonLink to="/" variant="secondary" size="sm">
            ← Overview
          </ButtonLink>
          <ButtonLink to="/projects" variant="secondary" size="sm">
            Projects
          </ButtonLink>
          <ButtonLink to="/projects/new" variant="primary" size="sm">
            + New Project
          </ButtonLink>
        </div>
      </header>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}

      <Panel
        title="All deployments"
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex flex-wrap gap-1">
              {(
                [
                  ["all", "All", counts.all],
                  ["building", "Building", counts.building],
                  ["success", "Success", counts.success],
                  ["failed", "Failed", counts.failed],
                ] as const
              ).map(([key, label, n]) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => setFilter(key as StatusFilter)}
                  className={`mono border px-2 py-1 text-[10px] font-semibold uppercase tracking-wider ${
                    filter === key
                      ? "border-primary bg-primary text-primary-ink"
                      : "border-border text-muted hover:border-border-strong hover:text-text"
                  }`}
                >
                  {label} ({n})
                </button>
              ))}
            </div>
            {canLoadMore ? (
              <Button
                variant="secondary"
                size="sm"
                type="button"
                disabled={deploysQ.isFetching}
                onClick={loadMore}
              >
                {deploysQ.isFetching ? "Loading…" : `Load more (up to ${MAX_DEPLOYMENTS})`}
              </Button>
            ) : null}
          </div>
        }
        noBody
      >
        {tableLoading && filtered.length === 0 ? (
          <div className="p-6 text-sm text-muted">Loading deployments…</div>
        ) : filtered.length === 0 ? (
          <div className="p-4">
            <EmptyState
              title={deployments.length === 0 ? "No deployments yet" : "No deployments match this filter"}
              description={
                deployments.length === 0
                  ? "Create a project and deploy to see rows here."
                  : "Try another filter or clear filters to see the full list."
              }
              action={
                deployments.length === 0 ? (
                  <ButtonLink to="/projects/new" variant="primary" size="sm">
                    + New Project
                  </ButtonLink>
                ) : (
                  <Button variant="secondary" size="sm" onClick={() => setFilter("all")}>
                    Show all
                  </Button>
                )
              }
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] table-fixed text-sm">
              <thead>
                <tr className="mono border-b border-border text-left text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="px-4 py-2 w-[22%]">Project</th>
                  <th className="px-4 py-2 w-[14%]">Deployment</th>
                  <th className="px-4 py-2 w-[14%]">Commit</th>
                  <th className="px-4 py-2 w-[14%]">Status</th>
                  <th className="px-4 py-2 w-[18%]">Started</th>
                  <th className="px-4 py-2 w-[18%]">Duration</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((d) => {
                  const proj = projectByID.get(d.project_id);
                  const projectName = proj?.name || shortHash(d.project_id, 8);
                  return (
                    <tr key={d.id} className="border-b border-border/60 hover:bg-surface-alt">
                      <td className="px-4 py-3 truncate">
                        <Link to={`/projects/${d.project_id}`} className="font-semibold text-text hover:underline">
                          {projectName}
                        </Link>
                        {proj?.repo_url && <div className="mono truncate text-[11px] text-muted">{proj.repo_url}</div>}
                      </td>
                      <td className="px-4 py-3">
                        <Link
                          to={`/projects/${d.project_id}/deployments/${d.id}`}
                          className="mono text-xs text-info hover:underline"
                        >
                          {shortHash(d.id, 10)}
                        </Link>
                      </td>
                      <td className="px-4 py-3">
                        <span className="mono text-xs text-text">{shortHash(d.commit_hash, 7)}</span>
                      </td>
                      <td className="px-4 py-3">
                        <StatusPill status={d.status} size="sm" />
                      </td>
                      <td className="px-4 py-3 text-xs text-muted">{formatRelative(d.created_at, new Date(), fmtLocale)}</td>
                      <td className="px-4 py-3 mono text-xs text-text">{formatDuration(d.created_at, d.updated_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
    </div>
  );
}
