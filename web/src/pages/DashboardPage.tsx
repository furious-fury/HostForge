import type { ReactNode } from "react";
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowRight, Boxes, Rocket, Server } from "lucide-react";
import { Link } from "react-router-dom";
import { ApiDeployment, ApiProject, fetchOnboardingStatus } from "../api";
import { hostDiskMounts, hostMem, hostNetIfaces, type HostDiskUsage, type HostSample } from "../api/host";
import { ButtonLink } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { KpiTile } from "../components/KpiTile";
import { Panel } from "../components/Panel";
import { Sparkline } from "../components/Sparkline";
import { StackBadge } from "../components/StackBadge";
import { StatusPill } from "../components/StatusPill";
import { formatBitsPerSec, formatBytes, formatPct } from "../format/bytes";
import { formatDuration, formatRelative, shortHash } from "../format";
import { useDeploymentsListQuery, useProjectsQuery, useSystemStatusQuery } from "../hooks/fleetQueries";
import { useHostHistory, useHostSnapshot } from "../hooks/hostQueries";
import { useFormatLocale } from "../hooks/useUIPrefs";
import { effectiveBuildLabel } from "../uiVersion";

const DAY_MS = 24 * 60 * 60 * 1000;

function dashOr(n: number | null): ReactNode {
  return n === null ? "—" : n;
}

function pctTone(pct: number): "default" | "success" | "warning" | "danger" {
  if (pct < 60) return "success";
  if (pct <= 85) return "warning";
  return "danger";
}

function pickRootDisk(disks: HostDiskUsage[]): HostDiskUsage | null {
  for (const d of disks) {
    if (d.mount === "/") return d;
  }
  return disks[0] ?? null;
}

function totalNetBytesPerSec(sample: HostSample): number {
  let t = 0;
  for (const n of hostNetIfaces(sample)) {
    t += n.rx_bps + n.tx_bps;
  }
  return t;
}

function seriesFromHistory(samples: HostSample[], pick: (s: HostSample) => number): number[] {
  return samples.map(pick);
}

export function DashboardPage() {
  const fmtLocale = useFormatLocale();
  const projectsQ = useProjectsQuery({ refetchWhileInFlight: true });
  const deploysQ = useDeploymentsListQuery(30, { refetchWhileInFlight: true });
  const systemQ = useSystemStatusQuery();
  const hostSnapQ = useHostSnapshot();
  const hostHistQ = useHostHistory(120);
  const onboardingQ = useQuery({ queryKey: ["onboarding"], queryFn: fetchOnboardingStatus, staleTime: 30_000, retry: 1 });

  const projects: ApiProject[] = projectsQ.data ?? [];
  const deployments: ApiDeployment[] = deploysQ.data ?? [];
  const systemStatus = systemQ.data ?? null;

  const projectsReady = projectsQ.data !== undefined;
  const deploysReady = deploysQ.data !== undefined;

  const projectByID = useMemo(() => {
    const map = new Map<string, ApiProject>();
    for (const p of projects) {
      map.set(p.id, p);
    }
    return map;
  }, [projects]);

  const stats = useMemo(() => {
    const cutoff = Date.now() - DAY_MS;
    let deploys24: number | null = null;
    let failed24: number | null = null;
    if (deploysReady) {
      deploys24 = 0;
      failed24 = 0;
      for (const d of deployments) {
        const ts = Date.parse(d.created_at);
        if (Number.isNaN(ts) || ts < cutoff) continue;
        deploys24 += 1;
        if (d.status?.toUpperCase() === "FAILED") failed24 += 1;
      }
    }
    let runningContainers: number | null = null;
    if (projectsReady) {
      runningContainers = 0;
      for (const p of projects) {
        if (p.current_container?.status?.toUpperCase() === "RUNNING") {
          runningContainers += 1;
        }
      }
    }
    const activeProjects = projectsReady ? projects.length : null;
    return {
      activeProjects,
      deploys24,
      failed24,
      runningContainers,
    };
  }, [projects, deployments, projectsReady, deploysReady]);

  const recent = useMemo(() => deployments.slice(0, 5), [deployments]);
  const controlPlaneHealthy = useMemo(() => systemStatus?.checks?.every((check) => (check.status || "").toUpperCase() === "OK") ?? false, [systemStatus]);
  const needsAttention = useMemo(
    () =>
      projects.filter(
        (project) =>
          project.latest_deployment?.status?.toUpperCase() === "FAILED" ||
          project.current_container?.status?.toUpperCase() === "EXITED",
      ),
    [projects],
  );

  const projectsError = projectsQ.isError
    ? projectsQ.error instanceof Error
      ? projectsQ.error.message
      : "failed to load dashboard"
    : "";

  const recentLoading = deploysQ.isPending && deploysQ.data === undefined;

  const hostSnap = hostSnapQ.data;
  const hostHist = hostHistQ.data?.samples ?? [];
  const histSlice = hostHist.length > 60 ? hostHist.slice(-60) : hostHist;
  const snap =
    hostSnap &&
    hostSnap.supported !== false &&
    !hostSnap.error_code &&
    hostSnap.warming !== true &&
    hostSnap.sample
      ? hostSnap.sample
      : null;
  const rootDisk = snap ? pickRootDisk(hostDiskMounts(snap)) : null;
  const cpuSeries = seriesFromHistory(histSlice, (s) => s.cpu_pct);
  const memSeries = seriesFromHistory(histSlice, (s) => hostMem(s).used_pct);
  const diskSeries = seriesFromHistory(histSlice, (s) => pickRootDisk(hostDiskMounts(s))?.used_pct ?? 0);
  const netSeries = seriesFromHistory(histSlice, (s) => totalNetBytesPerSec(s));

  return (
    <div className="flex flex-col gap-6">
      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.45fr)_minmax(22rem,0.85fr)]">
        <Panel className="overflow-hidden" bodyClassName="p-0">
          <div className="relative overflow-hidden px-6 py-6 sm:px-7">
            <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(56,189,248,0.16),transparent_30%),radial-gradient(circle_at_left,rgba(249,115,22,0.18),transparent_26%)]" />
            <div className="relative">
              <div className="mono text-[11px] font-semibold uppercase tracking-[0.24em] text-muted">Dashboard</div>
              <h1 className="mt-3 text-3xl font-semibold tracking-tight text-text">Operate the platform through applications.</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted">
                Start from application state, recent deployments, and platform health. Container-level details stay close,
                but they no longer lead the experience.
              </p>
              <div className="mt-6 flex flex-wrap gap-3">
                <ButtonLink to="/projects" variant="secondary" className="rounded-xl">
                  <Boxes className="h-4 w-4" />
                  View applications
                </ButtonLink>
                <ButtonLink to="/projects/new" variant="primary" className="rounded-xl">
                  <Rocket className="h-4 w-4" />
                  New application
                </ButtonLink>
              </div>
            </div>
          </div>
        </Panel>

        <Panel title="Platform health">
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
            <HealthRow icon={Server} label="Control plane" value={controlPlaneHealthy ? "Healthy" : "Degraded"} detail={effectiveBuildLabel(systemStatus?.version)} />
            <HealthRow icon={AlertTriangle} label="Applications needing attention" value={String(needsAttention.length)} detail={needsAttention.length ? "Latest deploy failed or runtime exited" : "No urgent issues right now"} danger={needsAttention.length > 0} />
          </div>
          {onboardingQ.data?.bootstrap_enabled && !onboardingQ.data.bootstrap_complete ? (
            <div className="mt-4 rounded-2xl border border-warning/30 bg-warning/10 p-4 text-sm text-muted">
              <div className="font-medium text-text">Onboarding is still in progress</div>
              <div className="mt-2 grid gap-2 sm:grid-cols-3 xl:grid-cols-1">
                <span>{onboardingQ.data.github_app_complete ? "GitHub App ready" : "GitHub App pending"}</span>
                <span>{onboardingQ.data.platform_domain ? `Domain: ${onboardingQ.data.platform_domain}` : "Platform domain pending"}</span>
                <span>{onboardingQ.data.permanent_ingress_complete ? "HTTPS ready" : "Permanent HTTPS pending"}</span>
              </div>
            </div>
          ) : null}
        </Panel>
      </section>

      {projectsError && <div className="rounded-2xl border border-danger/40 bg-danger/10 p-4 text-sm text-danger">{projectsError}</div>}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <KpiTile label="Applications" value={dashOr(stats.activeProjects)} hint="Applications registered in HostForge" />
        <KpiTile label="Deployments (24h)" value={dashOr(stats.deploys24)} hint="Total deployments started in the last day" tone={(stats.deploys24 ?? 0) > 0 ? "info" : "default"} />
        <KpiTile label="Failed (24h)" value={dashOr(stats.failed24)} hint={(stats.failed24 ?? 0) === 0 ? "No failures detected" : "Investigate recent deploy errors"} tone={(stats.failed24 ?? 0) > 0 ? "danger" : "success"} />
        <KpiTile label="Running containers" value={dashOr(stats.runningContainers)} hint="Runtime containers currently online" tone={(stats.runningContainers ?? 0) > 0 ? "success" : "default"} />
      </div>

      {hostSnap?.supported === false ? null : (
        <Panel
          title="Infrastructure capacity"
          actions={
            <Link to="/settings?tab=system" className="inline-flex items-center gap-2 text-sm text-muted transition-colors hover:text-text">
              System details
              <ArrowRight className="h-4 w-4" />
            </Link>
          }
        >
          {!hostSnap && !hostSnapQ.isPending ? (
            <p className="text-sm text-muted">Host metrics unavailable.</p>
          ) : hostSnap?.warming ? (
            <p className="text-sm text-muted">Host metrics warming up. We need two samples before rates become useful.</p>
          ) : snap ? (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
              <KpiTile
                label="CPU"
                value={formatPct(snap.cpu_pct, fmtLocale, 1)}
                hint={snap.rates_ready ? "Busy % since last tick" : "Rates warming up"}
                tone={pctTone(snap.cpu_pct)}
                footer={<Sparkline values={cpuSeries} width={240} height={52} className="w-full" strokeClassName="stroke-primary" showGrid />}
              />
              <KpiTile
                label="Memory"
                value={formatPct(hostMem(snap).used_pct, fmtLocale, 1)}
                hint={`${formatBytes(hostMem(snap).used_bytes, fmtLocale)} / ${formatBytes(hostMem(snap).total_bytes, fmtLocale)}`}
                tone={pctTone(hostMem(snap).used_pct)}
                footer={<Sparkline values={memSeries} width={240} height={52} className="w-full" strokeClassName="stroke-info" showGrid />}
              />
              <KpiTile
                label="Disk (root)"
                value={rootDisk ? formatPct(rootDisk.used_pct, fmtLocale, 1) : "—"}
                hint={rootDisk ? `${formatBytes(rootDisk.used_bytes, fmtLocale)} / ${formatBytes(rootDisk.total_bytes, fmtLocale)} on ${rootDisk.mount}` : "No mount data"}
                tone={rootDisk ? pctTone(rootDisk.used_pct) : "default"}
                footer={<Sparkline values={diskSeries} width={240} height={52} className="w-full" strokeClassName="stroke-warning" showGrid />}
              />
              <KpiTile
                label="Network"
                value={formatBitsPerSec(totalNetBytesPerSec(snap), fmtLocale)}
                hint="Total throughput across primary interfaces"
                tone="info"
                footer={<Sparkline values={netSeries} width={240} height={52} className="w-full" strokeClassName="stroke-success" showGrid />}
              />
            </div>
          ) : (
            <p className="text-sm text-muted">Loading host metrics…</p>
          )}
        </Panel>
      )}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.8fr)]">
        <Panel
          title="Recent deployments"
          actions={
            <Link to="/projects" className="inline-flex items-center gap-2 text-sm text-muted transition-colors hover:text-text">
              All applications
              <ArrowRight className="h-4 w-4" />
            </Link>
          }
          noBody
        >
          {recentLoading && recent.length === 0 ? (
            <div className="p-6 text-sm text-muted">Loading deployments…</div>
          ) : recent.length === 0 ? (
            <div className="p-4">
              <EmptyState
                title="No deployments yet"
                description="Create your first application and its deployment history will start streaming here."
                action={
                  <ButtonLink to="/projects/new" variant="primary" size="sm">
                    New application
                  </ButtonLink>
                }
              />
            </div>
          ) : (
            <table className="w-full table-fixed text-sm">
              <thead>
                <tr className="mono border-b border-border text-left text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="w-[24%] px-4 py-2">Application</th>
                  <th className="w-[10%] px-4 py-2">Stack</th>
                  <th className="w-[16%] px-4 py-2">Commit</th>
                  <th className="w-[16%] px-4 py-2">Status</th>
                  <th className="w-[16%] px-4 py-2">Started</th>
                  <th className="w-[18%] px-4 py-2">Duration</th>
                </tr>
              </thead>
              <tbody>
                {recent.map((deployment) => {
                  const project = deployment.project_id ? projectByID.get(deployment.project_id) : null;
                  return (
                    <tr key={deployment.id} className="border-b border-border/60 hover:bg-surface-alt/60">
                      <td className="px-4 py-3 align-middle">
                        {project ? (
                          <Link to={`/projects/${project.id}`} className="font-medium text-text hover:text-primary">
                            {project.name}
                          </Link>
                        ) : (
                          <span className="text-muted">Unknown application</span>
                        )}
                      </td>
                      <td className="px-4 py-3 align-middle">
                        <StackBadge stackKind={deployment.stack_kind} stackLabel={deployment.stack_label} compact />
                      </td>
                      <td className="mono px-4 py-3 align-middle text-xs text-text">{shortHash(deployment.commit_hash, 7)}</td>
                      <td className="px-4 py-3 align-middle">
                        <StatusPill status={deployment.status} size="sm" />
                      </td>
                      <td className="px-4 py-3 align-middle text-xs text-muted">{formatRelative(deployment.created_at, new Date(), fmtLocale)}</td>
                      <td className="mono px-4 py-3 align-middle text-xs text-text">{formatDuration(deployment.created_at, deployment.updated_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </Panel>

        <Panel title="Attention queue">
          {needsAttention.length === 0 ? (
            <div className="rounded-2xl border border-border bg-surface-alt/60 p-4 text-sm text-muted">No applications are currently flagged for attention.</div>
          ) : (
            <div className="space-y-3">
              {needsAttention.slice(0, 4).map((project) => (
                <Link key={project.id} to={`/projects/${project.id}`} className="block rounded-2xl border border-border bg-surface-alt/60 p-4 transition-colors hover:border-border-strong hover:bg-surface-alt">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="font-medium text-text">{project.name}</div>
                      <div className="mt-1 text-xs text-muted">{project.latest_deployment?.error_message || project.repo_url}</div>
                    </div>
                    <StatusPill status={project.latest_deployment?.status || project.current_container?.status || "UNKNOWN"} size="sm" />
                  </div>
                </Link>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </div>
  );
}

function HealthRow({ icon: Icon, label, value, detail, danger = false }: { icon: typeof Server; label: string; value: string; detail: string; danger?: boolean }) {
  return (
    <div className="rounded-2xl border border-border bg-surface-alt/60 p-4">
      <div className="flex items-start gap-3">
        <div className={danger ? "flex h-10 w-10 items-center justify-center rounded-xl bg-danger/10 text-danger" : "flex h-10 w-10 items-center justify-center rounded-xl bg-surface text-info"}>
          <Icon className="h-4 w-4" />
        </div>
        <div>
          <div className="text-sm font-medium text-text">{label}</div>
          <div className={danger ? "mt-1 text-lg font-semibold text-danger" : "mt-1 text-lg font-semibold text-text"}>{value}</div>
          <div className="mt-1 text-xs text-muted">{detail}</div>
        </div>
      </div>
    </div>
  );
}
