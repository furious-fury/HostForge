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
    <div className="flex flex-col gap-3 rounded-[10px] border border-border bg-surface p-5 sm:p-6">
      <div className="mono text-[10px] font-medium uppercase tracking-[0.08em] text-muted">{label}</div>
      <div className={`font-display text-3xl font-semibold tabular-nums ${toneClass[tone]}`}>{value}</div>
      {hint && <div className="text-xs text-muted">{hint}</div>}
      {footer ? <div className="mt-2 border-t border-border/60 pt-3">{footer}</div> : null}
    </div>
  );
}
