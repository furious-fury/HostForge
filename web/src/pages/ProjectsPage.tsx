import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ApiProject, deleteProject, fetchProjects } from "../api";
import { projectReachSummary } from "../accessUrls";
import { Button, ButtonLink } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { EmptyState } from "../components/EmptyState";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { useToast } from "../components/ToastProvider";
import { formatRelative } from "../format";

type Filter = "all" | "running" | "failed";

export function ProjectsPage() {
  const toast = useToast();
  const [projects, setProjects] = useState<ApiProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [deletingId, setDeletingId] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ApiProject | null>(null);

  const reload = useCallback(async () => {
    const items = await fetchProjects();
    setProjects(items);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        await reload();
        if (!cancelled) {
          setError("");
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load projects");
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
  }, [reload]);

  const counts = useMemo(() => {
    let running = 0;
    let failed = 0;
    for (const p of projects) {
      const status = p.latest_deployment?.status?.toUpperCase();
      if (status === "SUCCESS" && p.current_container?.status?.toUpperCase() === "RUNNING") running += 1;
      if (status === "FAILED") failed += 1;
    }
    return { all: projects.length, running, failed };
  }, [projects]);

  const filtered = useMemo(() => {
    if (filter === "all") return projects;
    if (filter === "running") {
      return projects.filter(
        (p) =>
          p.latest_deployment?.status?.toUpperCase() === "SUCCESS" &&
          p.current_container?.status?.toUpperCase() === "RUNNING",
      );
    }
    return projects.filter((p) => p.latest_deployment?.status?.toUpperCase() === "FAILED");
  }, [projects, filter]);

  async function executeDelete(project: ApiProject) {
    setDeletingId(project.id);
    setError("");
    try {
      await deleteProject(project.id);
      await reload();
      toast.success(`Deleted project "${project.name}".`);
      setDeleteTarget(null);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed.";
      toast.error(msg);
    } finally {
      setDeletingId("");
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete project"
        description={
          deleteTarget ? (
            <>
              <span className="font-semibold text-text">{`"${deleteTarget.name}"`}</span> will be removed permanently. This
              stops and removes Docker containers, deletes all deployments and domain records, and cannot be undone.
            </>
          ) : null
        }
        confirmLabel="Delete"
        cancelLabel="Cancel"
        confirmVariant="danger"
        typeConfirm={
          deleteTarget
            ? {
                prompt: "Type the project name exactly to enable Delete",
                expected: deleteTarget.name.trim() || deleteTarget.id,
              }
            : undefined
        }
        onClose={() => {
          if (!deletingId) setDeleteTarget(null);
        }}
        onConfirm={async () => {
          if (deleteTarget) {
            await executeDelete(deleteTarget);
          }
        }}
      />

      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Projects</div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {projects.length} project{projects.length === 1 ? "" : "s"}
          </h1>
          <p className="mt-1 text-sm text-muted">Status, last deploy, and how to reach each project.</p>
        </div>
        <ButtonLink to="/projects/new" variant="primary">
          + New Project
        </ButtonLink>
      </header>

      <div className="flex flex-wrap items-center gap-1 self-start border border-border bg-surface p-1">
        <FilterTab current={filter} value="all" onChange={setFilter} count={counts.all}>
          All
        </FilterTab>
        <FilterTab current={filter} value="running" onChange={setFilter} count={counts.running}>
          Running
        </FilterTab>
        <FilterTab current={filter} value="failed" onChange={setFilter} count={counts.failed}>
          Failed
        </FilterTab>
      </div>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}
      {loading && projects.length === 0 && <div className="text-sm text-muted">Loading projects…</div>}

      {!loading && filtered.length === 0 && projects.length === 0 && (
        <EmptyState
          title="No projects yet"
          description="Connect a GitHub repository and HostForge will build, deploy, and route traffic for it."
          action={
            <ButtonLink to="/projects/new" variant="primary" size="sm">
              + New Project
            </ButtonLink>
          }
        />
      )}

      {!loading && filtered.length === 0 && projects.length > 0 && (
        <EmptyState title={`No ${filter} projects`} description="Try a different filter to see other projects." />
      )}

      {filtered.length > 0 && (
        <Panel title="Project Fleet" noBody>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] table-fixed border-collapse text-left text-sm">
              <thead>
                <tr className="mono border-b border-border bg-surface-alt text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="px-4 py-3 w-[30%]">Project</th>
                  <th className="px-4 py-3 w-[10%]">Branch</th>
                  <th className="px-4 py-3 w-[14%]">Last deploy</th>
                  <th className="px-4 py-3 w-[22%]">Reach</th>
                  <th className="px-4 py-3 w-[14%]">Status</th>
                  <th className="px-4 py-3 w-[10%] text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((project) => (
                  <tr key={project.id} className="border-b border-border hover:bg-surface-alt">
                    <td className="px-4 py-3 align-top">
                      <Link to={`/projects/${project.id}`} className="block font-semibold text-text hover:underline">
                        {project.name}
                      </Link>
                      <div className="mono mt-0.5 truncate text-[11px] text-muted">{project.repo_url}</div>
                    </td>
                    <td className="px-4 py-3 align-top font-mono text-xs text-text">{project.branch || "main"}</td>
                    <td className="px-4 py-3 align-top text-xs text-text">
                      {formatRelative(project.latest_deployment?.created_at)}
                    </td>
                    <td className="px-4 py-3 align-top">
                      <div className="mono break-all text-xs text-text">{projectReachSummary(project)}</div>
                    </td>
                    <td className="px-4 py-3 align-top">
                      <StatusPill status={project.latest_deployment?.status || "UNKNOWN"} size="sm" />
                    </td>
                    <td className="px-4 py-3 align-top text-right">
                      <Button
                        variant="danger"
                        size="sm"
                        disabled={deletingId !== ""}
                        onClick={() => setDeleteTarget(project)}
                      >
                        {deletingId === project.id ? "…" : "Delete"}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>
      )}
    </div>
  );
}

function FilterTab({
  current,
  value,
  onChange,
  count,
  children,
}: {
  current: Filter;
  value: Filter;
  onChange: (next: Filter) => void;
  count: number;
  children: string;
}) {
  const active = current === value;
  return (
    <button
      type="button"
      onClick={() => onChange(value)}
      className={`mono px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider ${
        active ? "bg-primary text-primary-ink" : "text-muted hover:bg-surface-alt hover:text-text"
      }`}
    >
      {children}
      <span className="ml-2 opacity-70">{count}</span>
    </button>
  );
}
