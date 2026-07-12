import { useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ApiProject, deleteProject } from "../api";
import { projectReachSummary } from "../accessUrls";
import { Button, ButtonLink } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { EmptyState } from "../components/EmptyState";
import { LoadingState, RetryNotice } from "../components/OperationalFeedback";
import { Panel } from "../components/Panel";
import { StackBadge } from "../components/StackBadge";
import { StatusPill } from "../components/StatusPill";
import { useToast } from "../components/ToastProvider";
import { formatRelative } from "../format";
import { fleetKeys, useProjectsQuery } from "../hooks/fleetQueries";
import { invalidateFleetProjectsAndDeployments } from "../hooks/mutationCache";
import { useFormatLocale } from "../hooks/useUIPrefs";

type Filter = "all" | "running" | "failed";

export function ProjectsPage() {
  const fmtLocale = useFormatLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const projectsQ = useProjectsQuery({ refetchWhileInFlight: true });
  const projects = projectsQ.data ?? [];
  const loading = projectsQ.isPending && projectsQ.data === undefined;
  const error = projectsQ.isError
    ? projectsQ.error instanceof Error
      ? projectsQ.error.message
      : "failed to load applications"
    : "";
  const [filter, setFilter] = useState<Filter>("all");
  const [deletingId, setDeletingId] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<ApiProject | null>(null);

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
    const prev = queryClient.getQueryData<ApiProject[]>(fleetKeys.projects);
    queryClient.setQueryData<ApiProject[]>(fleetKeys.projects, (old) =>
      old ? old.filter((p) => p.id !== project.id) : old,
    );
    try {
      await deleteProject(project.id);
      await invalidateFleetProjectsAndDeployments(queryClient);
      toast.success(`Deleted application "${project.name}".`);
      setDeleteTarget(null);
    } catch (err) {
      if (prev !== undefined) {
        queryClient.setQueryData(fleetKeys.projects, prev);
      } else {
        void queryClient.invalidateQueries({ queryKey: fleetKeys.projects });
      }
      toast.error(err instanceof Error ? err.message : "Delete failed.");
    } finally {
      setDeletingId("");
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete application"
        description={
          deleteTarget ? (
            <>
              <span className="font-semibold text-text">{`"${deleteTarget.name}"`}</span> will be removed permanently.
              This stops and removes Docker containers, deletes deployments and domain records, and cannot be undone.
            </>
          ) : null
        }
        confirmLabel="Delete"
        cancelLabel="Cancel"
        confirmVariant="danger"
        typeConfirm={
          deleteTarget
            ? {
                prompt: "Type the application name exactly to enable Delete",
                expected: deleteTarget.name.trim() || deleteTarget.id,
              }
            : undefined
        }
        onClose={() => {
          if (!deletingId) setDeleteTarget(null);
        }}
        onConfirm={async () => {
          if (deleteTarget) await executeDelete(deleteTarget);
        }}
      />

      <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Applications</div>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight text-text">Application fleet</h1>
          <p className="mt-1 text-sm text-muted">Deploy, monitor, and manage applications from a single workspace.</p>
        </div>
        <div className="flex items-center gap-2">
          <ButtonLink to="/projects/new" variant="primary" size="sm">
            <Plus className="h-4 w-4" />
            New application
          </ButtonLink>
        </div>
      </header>

      <div className="grid gap-4 md:grid-cols-3">
        <SummaryCard label="Total applications" value={counts.all} />
        <SummaryCard label="Healthy" value={counts.running} />
        <SummaryCard label="Failed deployments" value={counts.failed} tone={counts.failed > 0 ? "danger" : "default"} />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <FilterTab current={filter} value="all" onChange={setFilter} count={counts.all}>All</FilterTab>
          <FilterTab current={filter} value="running" onChange={setFilter} count={counts.running}>Healthy</FilterTab>
          <FilterTab current={filter} value="failed" onChange={setFilter} count={counts.failed}>Failed</FilterTab>
        </div>
        <div className="text-sm text-muted">{filtered.length} applications</div>
      </div>

      {error && <RetryNotice title="Applications could not be refreshed" detail={error} onRetry={() => void projectsQ.refetch()} />}
      {loading && projects.length === 0 && <Panel noBody><LoadingState label="Loading applications" /></Panel>}

      {!loading && filtered.length === 0 && projects.length === 0 && (
        <EmptyState
          title="No applications yet"
          description="Create your first application from a GitHub repository or image."
          action={<ButtonLink to="/projects/new" variant="primary" size="sm"><Plus className="h-4 w-4" />New application</ButtonLink>}
        />
      )}

      {!loading && filtered.length === 0 && projects.length > 0 && (
        <EmptyState title={`No ${filter} applications`} description="Try a different filter to see other applications." />
      )}

      {filtered.length > 0 && (
        <Panel title="Applications" noBody>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[980px] table-fixed text-left text-sm">
              <thead>
                <tr className="mono border-b border-border bg-surface-alt text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="w-[24%] px-4 py-3">Application</th>
                  <th className="w-[12%] px-4 py-3">Branch</th>
                  <th className="w-[16%] px-4 py-3">Stack</th>
                  <th className="w-[18%] px-4 py-3">Reach</th>
                  <th className="w-[12%] px-4 py-3">Runtime</th>
                  <th className="w-[12%] px-4 py-3">Deploy</th>
                  <th className="w-[12%] px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((project) => {
                  const latest = project.latest_deployment;
                  return (
                    <tr key={project.id} className="border-b border-border align-top hover:bg-surface-alt/60">
                      <td className="px-4 py-4">
                        <div className="min-w-0">
                          <Link to={`/projects/${project.id}`} className="font-medium text-text hover:text-primary">
                            {project.name}
                          </Link>
                          <div className="mono mt-1 truncate text-[11px] text-muted">{project.repo_url}</div>
                        </div>
                      </td>
                      <td className="mono px-4 py-4 text-xs text-text">{project.branch || "main"}</td>
                      <td className="px-4 py-4">
                        <StackBadge
                          stackKind={project.stack_kind || latest?.stack_kind}
                          stackLabel={project.stack_label || latest?.stack_label}
                          compact
                        />
                      </td>
                      <td className="px-4 py-4">
                        <div className="mono break-all text-xs text-text">{projectReachSummary(project)}</div>
                      </td>
                      <td className="px-4 py-4">
                        <div className="text-xs text-text">{project.current_container?.status || "Unknown"}</div>
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex flex-col gap-2">
                          <StatusPill status={latest?.status || "UNKNOWN"} size="sm" />
                          <span className="text-xs text-muted">{formatRelative(latest?.created_at, new Date(), fmtLocale)}</span>
                        </div>
                      </td>
                      <td className="px-4 py-4">
                        <div className="flex justify-end gap-2">
                          <ButtonLink to={`/projects/${project.id}`} variant="secondary" size="sm">
                            Open
                          </ButtonLink>
                          <Button variant="danger" size="sm" disabled={deletingId !== ""} onClick={() => setDeleteTarget(project)}>
                            {deletingId === project.id ? "..." : "Delete"}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Panel>
      )}

      <Panel title="Create flow">
        <div className="grid gap-3 md:grid-cols-3">
          <FlowStep title="1. Create application" description="Connect a Git repository or container image." />
          <FlowStep title="2. Add services" description="Attach web, worker, database, cache, or cron services." />
          <FlowStep title="3. Deploy and monitor" description="Watch deployments, logs, and observability in one workspace." />
        </div>
      </Panel>
    </div>
  );
}

function FilterTab({ current, value, onChange, count, children }: { current: Filter; value: Filter; onChange: (next: Filter) => void; count: number; children: string }) {
  const active = current === value;
  return (
    <button
      type="button"
      onClick={() => onChange(value)}
      className={[
        "rounded-md border px-3 py-2 text-sm transition-colors",
        active ? "border-border-strong bg-surface-alt text-text" : "border-border bg-surface text-muted hover:bg-surface-alt hover:text-text",
      ].join(" ")}
    >
      {children}
      <span className="mono ml-2 text-[11px] text-muted">{count}</span>
    </button>
  );
}

function SummaryCard({ label, value, tone = "default" }: { label: string; value: number; tone?: "default" | "danger" }) {
  return (
    <div className="rounded-[12px] border border-border bg-surface p-4">
      <div className="mono text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">{label}</div>
      <div className={tone === "danger" ? "mt-2 text-2xl font-semibold text-danger" : "mt-2 text-2xl font-semibold text-text"}>{value}</div>
    </div>
  );
}

function FlowStep({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-[12px] border border-border bg-surface-alt p-4">
      <div className="font-medium text-text">{title}</div>
      <div className="mt-1 text-sm text-muted">{description}</div>
    </div>
  );
}
