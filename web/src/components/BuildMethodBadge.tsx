export function buildMethodLabel(kind?: string): string {
  switch ((kind || "").trim().toLowerCase()) {
    case "railpack":
      return "Railpack";
    case "dockerfile":
      return "Dockerfile fallback";
    default:
      return "Unknown builder";
  }
}

export function BuildMethodBadge({ kind }: { kind?: string }) {
  const normalized = (kind || "").trim().toLowerCase();
  if (!normalized) return null;
  return (
    <span className="mono inline-flex border border-border bg-surface-alt px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted">
      {buildMethodLabel(normalized)}
    </span>
  );
}
