import { ReactNode } from "react";

type KpiTileProps = {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  /** Rendered inside the tile below the hint (e.g. a sparkline). */
  footer?: ReactNode;
  tone?: "default" | "success" | "danger" | "warning" | "info";
};

const toneClass: Record<NonNullable<KpiTileProps["tone"]>, string> = {
  default: "text-text",
  success: "text-success",
  danger: "text-danger",
  warning: "text-warning",
  info: "text-info",
};

export function KpiTile({ label, value, hint, footer, tone = "default" }: KpiTileProps) {
  return (
    <div className="flex flex-col gap-2 border border-border bg-surface p-4">
      <div className="mono text-[11px] font-semibold uppercase tracking-[0.18em] text-muted">{label}</div>
      <div className={`text-3xl font-semibold tabular-nums ${toneClass[tone]}`}>{value}</div>
      {hint && <div className="text-xs text-muted">{hint}</div>}
      {footer ? <div className="mt-1 border-t border-border/60 pt-2">{footer}</div> : null}
    </div>
  );
}
