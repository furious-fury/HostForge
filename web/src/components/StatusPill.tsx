type StatusPillProps = {
  status: string;
  size?: "sm" | "md";
};

type OperationalState = "Queued" | "Building" | "Deploying" | "Running health checks" | "Healthy" | "Degraded" | "Failed" | "Rolling back" | "Unknown";

export function isOperationallyActive(status?: string): boolean {
  const normalized = status?.trim().toUpperCase();
  return normalized === "QUEUED" || normalized === "PENDING" || normalized === "BUILDING" || normalized === "DEPLOYING" || normalized === "RUNNING_HEALTH_CHECKS" || normalized === "HEALTH_CHECKING" || normalized === "ROLLING_BACK" || normalized === "ROLLBACK";
}

function classify(status: string): { color: string; label: OperationalState; active: boolean } {
  const normalized = status.trim().toUpperCase();
  if (normalized === "QUEUED" || normalized === "PENDING") return { color: "border-signal/60 bg-signal/15 text-signal-ink", label: "Queued", active: true };
  if (normalized === "BUILDING") return { color: "border-signal/60 bg-signal/15 text-signal-ink", label: "Building", active: true };
  if (normalized === "DEPLOYING") return { color: "border-signal/60 bg-signal/15 text-signal-ink", label: "Deploying", active: true };
  if (normalized === "RUNNING_HEALTH_CHECKS" || normalized === "HEALTH_CHECKING") return { color: "border-signal/60 bg-signal/15 text-signal-ink", label: "Running health checks", active: true };
  if (normalized === "SUCCESS" || normalized === "READY" || normalized === "RUNNING") return { color: "border-success bg-success text-success-ink", label: "Healthy", active: false };
  if (normalized === "DEGRADED" || normalized === "WARNING") return { color: "border-warning/40 bg-warning/10 text-warning", label: "Degraded", active: false };
  if (normalized === "ROLLING_BACK" || normalized === "ROLLBACK") return { color: "border-warning/40 bg-warning/10 text-warning", label: "Rolling back", active: true };
  if (normalized === "FAILED" || normalized === "ERROR" || normalized === "CRASHED" || normalized === "DOWN") return { color: "border-danger/40 bg-danger/10 text-danger", label: "Failed", active: false };
  return { color: "border-border bg-surface-alt text-muted", label: "Unknown", active: false };
}

export function StatusPill({ status, size = "md" }: StatusPillProps) {
  const { color, label, active } = classify(status || "UNKNOWN");
  const sizing = size === "sm" ? "min-h-6 px-2 py-0.5 text-[11px]" : "min-h-7 px-2.5 py-1 text-xs";
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border ${color} ${sizing} font-medium`} aria-label={`Status: ${label}`}>
      <span aria-hidden className={active ? "size-1.5 rounded-full bg-current motion-safe:animate-pulse" : "size-1.5 rounded-full bg-current"} />
      <span>{label}</span>
    </span>
  );
}
