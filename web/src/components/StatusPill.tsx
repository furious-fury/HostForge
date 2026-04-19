type StatusPillProps = {
  status: string;
  size?: "sm" | "md";
};

function classify(status: string): { color: string; label: string } {
  const normalized = (status || "").toUpperCase();
  if (normalized === "SUCCESS" || normalized === "RUNNING" || normalized === "READY") {
    return { color: "border-success text-success", label: normalized };
  }
  if (normalized === "FAILED" || normalized === "ERROR" || normalized === "CRASHED") {
    return { color: "border-danger text-danger", label: normalized };
  }
  if (normalized === "BUILDING" || normalized === "QUEUED" || normalized === "DEPLOYING" || normalized === "PENDING") {
    return { color: "border-warning text-warning", label: normalized };
  }
  if (normalized === "STOPPED" || normalized === "PAUSED") {
    return { color: "border-muted text-muted", label: normalized };
  }
  return { color: "border-border text-muted", label: normalized || "UNKNOWN" };
}

export function StatusPill({ status, size = "md" }: StatusPillProps) {
  const { color, label } = classify(status);
  const sizing = size === "sm" ? "px-1.5 py-0.5 text-[10px]" : "px-2 py-1 text-[11px]";
  return (
    <span className={`mono inline-flex items-center gap-1 border ${color} ${sizing} font-semibold uppercase tracking-wider`}>
      <span aria-hidden>●</span>
      <span>{label}</span>
    </span>
  );
}
