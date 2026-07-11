type StatusPillProps = {
  status: string;
  size?: "sm" | "md";
};

function classify(status: string): { color: string; label: string } {
  const normalized = (status || "").toUpperCase();
  if (normalized === "SUCCESS" || normalized === "RUNNING" || normalized === "READY") {
    return { color: "bg-success/10 text-success", label: normalized };
  }
  if (normalized === "FAILED" || normalized === "ERROR" || normalized === "CRASHED" || normalized === "DOWN") {
    return { color: "bg-danger/10 text-danger", label: normalized };
  }
  if (
    normalized === "BUILDING" ||
    normalized === "QUEUED" ||
    normalized === "DEPLOYING" ||
    normalized === "PENDING" ||
    normalized === "WARNING"
  ) {
    return { color: "bg-warning/10 text-warning", label: normalized };
  }
  if (normalized === "STOPPED" || normalized === "PAUSED" || normalized === "SKIPPED" || normalized === "NOT_CONFIGURED") {
    return { color: "bg-surface-alt text-muted", label: normalized };
  }
  return { color: "bg-surface-alt text-muted", label: normalized || "UNKNOWN" };
}

export function StatusPill({ status, size = "md" }: StatusPillProps) {
  const { color, label } = classify(status);
  const sizing = size === "sm" ? "px-2 py-1 text-[10px]" : "px-2.5 py-1 text-xs";
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-md ${color} ${sizing} font-medium`}>
      <span aria-hidden>●</span>
      <span>{label}</span>
    </span>
  );
}
