import { ReactNode } from "react";

type KpiTileProps = {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  /** Rendered inside the tile below the hint (e.g. a sparkline). */
  footer?: ReactNode;
  tone?: "default" | "success" | "danger" | "warning" | "info";
};

const toneClass: Record<NonNullable<KpiTileProps["tone"]>, { value: string; accent: string; dot: string }> = {
  default: { value: "text-text", accent: "border-t-border-strong", dot: "bg-border-strong" },
  success: { value: "text-success", accent: "border-t-success", dot: "bg-success" },
  danger: { value: "text-danger", accent: "border-t-danger", dot: "bg-danger" },
  warning: { value: "text-warning", accent: "border-t-warning", dot: "bg-warning" },
  info: { value: "text-info", accent: "border-t-info", dot: "bg-info" },
};

export function KpiTile({ label, value, hint, footer, tone = "default" }: KpiTileProps) {
  const style = toneClass[tone];
  return (
    <div className={`flex flex-col gap-3 rounded-b-panel rounded-t-none border border-border border-t-2 bg-surface p-5 shadow-[var(--hf-shadow-panel)] sm:p-6 ${style.accent}`}>
      <div className="flex items-center gap-2 mono text-[10px] font-medium uppercase tracking-[0.08em] text-muted">
        <span aria-hidden className={`size-1.5 rounded-full ${style.dot}`} />
        {label}
      </div>
      <div className={`font-display text-3xl font-semibold tabular-nums ${style.value}`}>{value}</div>
      {hint && <div className="text-xs text-muted">{hint}</div>}
      {footer ? <div className="mt-2 border-t border-border/60 pt-3">{footer}</div> : null}
    </div>
  );
}
