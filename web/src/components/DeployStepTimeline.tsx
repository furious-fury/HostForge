import { useMemo } from "react";
import type { DeployStepRow } from "../api";

export function DeployStepTimeline({ steps }: { steps: DeployStepRow[] }) {
  const { filtered, total } = useMemo(() => {
    const filtered = steps.filter((s) => s.step !== "deploy_total" && s.step !== "cert_poll" && s.duration_ms > 0);
    const total = filtered.reduce((a, s) => a + s.duration_ms, 0) || 1;
    return { filtered, total };
  }, [steps]);

  if (filtered.length === 0) {
    return <span className="text-xs text-muted">No step timings</span>;
  }

  return (
    <div className="flex h-6 w-full min-w-0 overflow-hidden rounded border border-border bg-surface-alt">
      {filtered.map((s) => {
        const w = Math.max(2, Math.round((100 * s.duration_ms) / total));
        const bg =
          s.status === "ok" ? "bg-success/70" : s.status === "failed" ? "bg-danger/80" : "bg-muted";
        return (
          <div
            key={`${s.id}-${s.step}`}
            style={{ width: `${w}%` }}
            className={`${bg} shrink-0 border-r border-border/40 last:border-r-0`}
            title={`${s.step} · ${s.duration_ms}ms · ${s.status}${s.error_code ? ` · ${s.error_code}` : ""}`}
          />
        );
      })}
    </div>
  );
}
