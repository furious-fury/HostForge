import { useMemo } from "react";
import { Link } from "react-router-dom";
import type { DeployStepRow } from "../api";
import { DeployStepTimeline } from "../components/DeployStepTimeline";
import { KpiTile } from "../components/KpiTile";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import {
  useObservabilityDeployStepsQuery,
  useObservabilityRequestsQuery,
  useObservabilitySummaryQuery,
} from "../hooks/observabilityQueries";
import { formatDurationMs } from "../format";
import { effectiveBuildLabel } from "../uiVersion";

function pct(n: number, d: number): string {
  if (d <= 0) return "0";
  return ((100 * n) / d).toFixed(1);
}

function formatStepTime(iso: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  return new Date(t).toLocaleString();
}

export function ObservabilityPage() {
  const sumQ = useObservabilitySummaryQuery();
  const reqQ = useObservabilityRequestsQuery(100);
  const stepQ = useObservabilityDeployStepsQuery(200);

  const summary = sumQ.data?.summary;
  const system = sumQ.data?.system;

  const deployGroups = useMemo(() => {
    const steps = stepQ.data ?? [];
    const order: string[] = [];
    const byDep = new Map<string, DeployStepRow[]>();
    for (const s of steps) {
      const id = s.deployment_id;
      if (!id) continue;
      if (!byDep.has(id)) {
        order.push(id);
        byDep.set(id, []);
      }
      byDep.get(id)!.push(s);
    }
    return order.slice(0, 20).map((id) => ({ id, steps: byDep.get(id) || [], name: byDep.get(id)?.[0]?.project_name || "" }));
  }, [stepQ.data]);

  const err = sumQ.isError
    ? sumQ.error instanceof Error
      ? sumQ.error.message
      : "failed to load"
    : "";

  return (
    <div className="flex flex-col gap-6">
      <header>
        <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Observe</div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-text">Observability</h1>
        <p className="mt-2 max-w-3xl text-sm text-muted">
          Last-window aggregates, recent HTTP samples, and deploy step timings persisted on this HostForge instance
          (bounded SQLite retention). Correlation ids match server logs.
        </p>
      </header>

      {err && <div className="border border-danger bg-danger/10 p-3 text-sm text-danger">{err}</div>}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <KpiTile label="HTTP requests (24h)" value={summary?.http_request_count ?? "—"} />
        <KpiTile
          label="HTTP errors (≥400)"
          value={summary?.http_error_count ?? "—"}
          hint={
            summary && summary.http_request_count > 0
              ? `${pct(summary.http_error_count, summary.http_request_count)}% of requests`
              : undefined
          }
          tone={summary && summary.http_error_count > 0 ? "warning" : "default"}
        />
        <KpiTile
          label="HTTP p50 / p95"
          value={
            summary
              ? `${formatDurationMs(summary.http_duration_p50_ms)} / ${formatDurationMs(summary.http_duration_p95_ms)}`
              : "—"
          }
        />
        <KpiTile label="Deploys started (24h)" value={summary?.deploy_count ?? "—"} />
        <KpiTile
          label="Deploy failures"
          value={summary?.deploy_failed_count ?? "—"}
          hint={summary && summary.deploy_count > 0 ? `${pct(summary.deploy_failed_count, summary.deploy_count)}%` : undefined}
          tone={summary && summary.deploy_failed_count > 0 ? "danger" : "default"}
        />
        <KpiTile
          label="Deploy total p50 / p95"
          value={
            summary
              ? `${formatDurationMs(summary.deploy_duration_p50_ms)} / ${formatDurationMs(summary.deploy_duration_p95_ms)}`
              : "—"
          }
        />
      </div>

      <Panel title="System checks">
        <p className="mb-3 text-[11px] text-muted">
          Same probes as the dashboard; <span className="mono">error_code</span> helps when filing issues.
        </p>
        <ul className="flex flex-col divide-y divide-border">
          {(system?.checks ?? []).map((c) => (
            <li key={c.id} className="py-2 text-sm" title={[c.error_code, c.detail].filter(Boolean).join(" — ") || undefined}>
              <div className="flex items-start justify-between gap-2">
                <span className="text-muted">{c.label}</span>
                <StatusPill status={c.status} size="sm" />
              </div>
              {c.error_code ? (
                <p className="mt-1 font-mono text-[10px] text-text">
                  <span className="text-muted">code</span> {c.error_code}
                </p>
              ) : null}
              {c.detail ? <p className="mt-1 line-clamp-2 font-mono text-[10px] text-muted">{c.detail}</p> : null}
            </li>
          ))}
          {!system && sumQ.isPending ? <li className="py-2 text-xs text-muted">Loading…</li> : null}
        </ul>
        <div className="mt-3 border-t border-border pt-3 text-xs text-muted">
          Build <span className="mono text-text">{effectiveBuildLabel(system?.version)}</span>
        </div>
      </Panel>

      <div className="grid gap-6 lg:grid-cols-2">
        <Panel title="Recent deploy timelines">
          <p className="mb-3 text-xs text-muted">
            One deploy per row: bar segments ≈ phase time (hover). Total ={" "}
            <span className="mono text-text">deploy_total</span>.
          </p>
          {stepQ.isPending ? <p className="text-sm text-muted">Loading steps…</p> : null}
          {stepQ.isError ? (
            <p className="text-sm text-danger">{stepQ.error instanceof Error ? stepQ.error.message : "load failed"}</p>
          ) : null}
          <div className="max-h-[min(28rem,55vh)] overflow-auto rounded border border-border bg-surface-alt/30 p-2">
            <ul className="flex flex-col gap-4">
              {deployGroups.map(({ id, steps, name }) => {
                const totalMs = steps.find((s) => s.step === "deploy_total")?.duration_ms;
                return (
                  <li key={id} className="border border-border bg-surface p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <Link
                        to={
                          steps[0]?.project_id
                            ? `/projects/${steps[0].project_id}/deployments/${id}`
                            : "/deployments"
                        }
                        className="mono text-sm font-semibold hover:underline"
                      >
                        {name ? `${name} · ` : ""}
                        {id.slice(0, 12)}…
                      </Link>
                      <span className="mono text-[10px] text-muted" title={`${totalMs ?? "—"} ms`}>
                        {totalMs != null ? `${formatDurationMs(totalMs)} total` : "—"}
                      </span>
                    </div>
                    <div className="mt-2">
                      <DeployStepTimeline steps={steps} />
                    </div>
                  </li>
                );
              })}
              {deployGroups.length === 0 && !stepQ.isPending ? (
                <li className="text-sm text-muted">No deploy step samples yet. Run a deployment.</li>
              ) : null}
            </ul>
          </div>
        </Panel>

        <Panel title="Recent HTTP requests">
          <p className="mb-3 text-xs text-muted">
            Sampled API traffic (bounded SQLite). Correlate with logs via{" "}
            <span className="mono text-text">request_id</span>.
          </p>
          {reqQ.isPending ? <p className="text-sm text-muted">Loading…</p> : null}
          {reqQ.isError ? (
            <p className="text-sm text-danger">{reqQ.error instanceof Error ? reqQ.error.message : "load failed"}</p>
          ) : null}
          <div className="max-h-[min(28rem,55vh)] overflow-auto rounded border border-border">
            <table className="w-full min-w-[32rem] border-collapse text-left text-sm">
              <thead className="sticky top-0 z-[1] border-b border-border bg-surface">
                <tr className="text-[10px] uppercase tracking-wider text-muted">
                  <th className="bg-surface py-2 pr-2">Time</th>
                  <th className="bg-surface py-2 pr-2">Method</th>
                  <th className="bg-surface py-2 pr-2">Path</th>
                  <th className="bg-surface py-2 pr-2">Status</th>
                  <th className="bg-surface py-2 pr-2">ms</th>
                  <th className="bg-surface py-2">request_id</th>
                </tr>
              </thead>
              <tbody>
                {(reqQ.data ?? []).map((r) => (
                  <tr key={r.id} className="border-b border-border/60">
                    <td className="py-1.5 pr-2 font-mono text-[11px] text-muted">{formatStepTime(r.started_at)}</td>
                    <td className="py-1.5 pr-2 mono text-xs">{r.method}</td>
                    <td className="max-w-[12rem] truncate py-1.5 pr-2 font-mono text-[11px] text-text">{r.path}</td>
                    <td className="py-1.5 pr-2">
                      <span
                        className={`mono text-xs font-semibold ${
                          r.status >= 500 ? "text-danger" : r.status >= 400 ? "text-warning" : "text-success"
                        }`}
                      >
                        {r.status}
                      </span>
                    </td>
                    <td className="py-1.5 pr-2 mono text-xs tabular-nums">{r.duration_ms}</td>
                    <td className="py-1.5 font-mono text-[10px] text-muted">{r.request_id}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {(reqQ.data ?? []).length === 0 && !reqQ.isPending ? (
            <p className="mt-2 text-sm text-muted">No HTTP samples recorded yet.</p>
          ) : null}
        </Panel>
      </div>
    </div>
  );
}
