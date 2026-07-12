import { useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowRight, GitBranch, Globe, Plus, Rocket } from "lucide-react";
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

  const latestUpdated = useMemo(() => {
    const stamped = projects
      .map((project) => project.latest_deployment?.created_at)
      .filter((value): value is string => Boolean(value))
      .sort((a, b) => Date.parse(b) - Date.parse(a));
    return stamped[0] ?? "";
  }, [projects]);

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
          if (deleteTarget) {
            await executeDelete(deleteTarget);
          }
        }}
      />

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(22rem,0.9fr)]">
        <Panel className="overflow-hidden" bodyClassName="p-0">
          <div className="relative overflow-hidden px-6 py-6 sm:px-7">
            <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(56,189,248,0.16),transparent_32%),radial-gradient(circle_at_left,rgba(249,115,22,0.18),transparent_28%)]" />
            <div className="relative">
              <div className="mono text-[11px] font-semibold uppercase tracking-[0.24em] text-muted">Applications</div>
              <h1 className="mt-3 text-3xl font-semibold tracking-tight text-text">Deploy around applications, not containers.</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted">
                This workspace groups each application with its latest deployment, runtime status, and public reach so the
                operational picture stays visible without drilling into infrastructure too early.
              </p>
              <div className="mt-6 flex flex-wrap gap-3">
                <ButtonLink to="/projects/new" variant="primary" className="rounded-xl">
                  <Plus className="h-4 w-4" />
                  New application
                </ButtonLink>
                <ButtonLink to="/observability" variant="secondary" className="rounded-xl">
                  <Rocket className="h-4 w-4" />
                  Monitor fleet
                </ButtonLink>
              </div>
            </div>
          </div>
        </Panel>

        <Panel title="Fleet snapshot">
          <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
            <Metric label="Applications" value={String(counts.all)} hint="Registered in HostForge" />
            <Metric label="Healthy" value={String(counts.running)} hint="Running with successful deploys" />
            <Metric label="Needs attention" value={String(counts.failed)} hint="Latest deploy failed" tone={counts.failed > 0 ? "danger" : "default"} />
          </div>
          <div className="mt-4 rounded-2xl border border-border bg-surface-alt/70 p-4 text-sm text-muted">
            <div className="mono text-[10px] font-semibold uppercase tracking-[0.2em] text-muted">Last deployment activity</div>
            <div className="mt-2 text-text">{latestUpdated ? formatRelative(latestUpdated, new Date(), fmtLocale) : "No deployments yet"}</div>
          </div>
        </Panel>
      </section>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <FilterTab current={filter} value="all" onChange={setFilter} count={counts.all}>
            All applications
          </FilterTab>
          <FilterTab current={filter} value="running" onChange={setFilter} count={counts.running}>
            Healthy
          </FilterTab>
          <FilterTab current={filter} value="failed" onChange={setFilter} count={counts.failed}>
            Attention
          </FilterTab>
        </div>
        <div className="text-sm text-muted">{filtered.length} shown</div>
      </div>

      {error && <RetryNotice title="Applications could not be refreshed" detail={error} onRetry={() => void projectsQ.refetch()} />}
      {loading && projects.length === 0 && (
        <Panel noBody>
          <LoadingState label="Loading applications" />
        </Panel>
      )}

      {!loading && filtered.length === 0 && projects.length === 0 && (
        <EmptyState
          title="No applications yet"
          description="Create your first application from a GitHub repository or image. HostForge will take care of build, deploy, and routing."
          action={
            <ButtonLink to="/projects/new" variant="primary" size="sm">
              <Plus className="h-4 w-4" />
              New application
            </ButtonLink>
          }
        />
      )}

      {!loading && filtered.length === 0 && projects.length > 0 && (
        <EmptyState title={`No ${filter} applications`} description="Try another filter to reveal other applications in the fleet." />
      )}

      {filtered.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
          {filtered.map((project) => {
            const reach = projectReachSummary(project);
            const latest = project.latest_deployment;
            const hasFailed = latest?.status?.toUpperCase() === "FAILED";
            return (
              <Panel key={project.id} className="h-full" bodyClassName="p-5">
                <div className="flex h-full flex-col gap-5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <Link to={`/projects/${project.id}`} className="block text-lg font-semibold text-text transition-colors hover:text-primary">
                        {project.name}
                      </Link>
                      <div className="mono mt-1 truncate text-[11px] text-muted">{project.repo_url}</div>
                    </div>
                    <StatusPill status={project.latest_deployment?.status || "UNKNOWN"} size="sm" />
                  </div>

                  <div className="grid gap-3 sm:grid-cols-2">
                    <InfoTile icon={GitBranch} label="Branch" value={project.branch || "main"} />
                    <InfoTile icon={Globe} label="Reach" value={reach} mono />
                  </div>

                  <div className="flex flex-wrap items-center gap-2">
                    <StackBadge
                      stackKind={project.stack_kind || project.latest_deployment?.stack_kind}
                      stackLabel={project.stack_label || project.latest_deployment?.stack_label}
                    />
                    {hasFailed ? (
                      <span className="inline-flex items-center gap-1 rounded-full border border-danger/30 bg-danger/10 px-2.5 py-1 text-xs text-danger">
                        <AlertTriangle className="h-3.5 w-3.5" />
                        Needs attention
                      </span>
                    ) : null}
                  </div>

                  <div className="grid gap-3 rounded-2xl border border-border bg-surface-alt/60 p-4 text-sm sm:grid-cols-2">
                    <div>
                      <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Container</div>
                      <div className="mt-2 text-text">{project.current_container?.status || "Unknown"}</div>
                    </div>
                    <div>
                      <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Last deploy</div>
                      <div className="mt-2 text-text">{formatRelative(latest?.created_at, new Date(), fmtLocale)}</div>
                    </div>
                  </div>

                  <div className="mt-auto flex items-center justify-between gap-3 pt-1">
                    <ButtonLink to={`/projects/${project.id}`} variant="secondary" size="sm" className="rounded-xl">
                      Open workspace
                      <ArrowRight className="h-4 w-4" />
                    </ButtonLink>
                    <Button variant="danger" size="sm" className="rounded-xl" disabled={deletingId !== ""} onClick={() => setDeleteTarget(project)}>
                      {deletingId === project.id ? "Deleting…" : "Delete"}
                    </Button>
                  </div>
                </div>
              </Panel>
            );
          })}
        </div>
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
      className={[
        "inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm transition-colors",
        active
          ? "border-border-strong bg-surface-alt text-text"
          : "border-border bg-surface text-muted hover:border-border-strong hover:text-text",
      ].join(" ")}
    >
      <span>{children}</span>
      <span className="mono text-[11px] text-muted">{count}</span>
    </button>
  );
}

function Metric({ label, value, hint, tone = "default" }: { label: string; value: string; hint: string; tone?: "default" | "danger" }) {
  return (
    <div className="rounded-2xl border border-border bg-surface-alt/60 p-4">
      <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">{label}</div>
      <div className={tone === "danger" ? "mt-2 text-2xl font-semibold text-danger" : "mt-2 text-2xl font-semibold text-text"}>{value}</div>
      <div className="mt-1 text-xs text-muted">{hint}</div>
    </div>
  );
}

function InfoTile({ icon: Icon, label, value, mono = false }: { icon: typeof GitBranch; label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-2xl border border-border bg-surface-alt/60 p-4">
      <div className="inline-flex items-center gap-2 text-xs text-muted">
        <Icon className="h-3.5 w-3.5" />
        <span>{label}</span>
      </div>
      <div className={mono ? "mono mt-2 break-all text-sm text-text" : "mt-2 text-sm text-text"}>{value}</div>
    </div>
  );
}
