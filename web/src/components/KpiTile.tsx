import { ReactNode } from "react";

type KpiTileProps = {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  footer?: ReactNode;
  tone?: "default" | "success" | "danger" | "warning" | "info";
};

const toneClass: Record<NonNullable<KpiTileProps["tone"]>, { value: string; badge: string }> = {
  default: { value: "text-text", badge: "bg-surface-alt text-muted" },
  success: { value: "text-success", badge: "bg-emerald-500/10 text-success" },
  danger: { value: "text-danger", badge: "bg-rose-500/10 text-danger" },
  warning: { value: "text-warning", badge: "bg-amber-500/10 text-warning" },
  info: { value: "text-info", badge: "bg-sky-500/10 text-info" },
};

export function KpiTile({ label, value, hint, footer, tone = "default" }: KpiTileProps) {
  const style = toneClass[tone];
  return (
    <div className="flex flex-col gap-3 rounded-[12px] border border-border bg-surface p-4 sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <div className="mono text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">{label}</div>
        <span className={`rounded-full px-2 py-1 text-[10px] font-medium ${style.badge}`}>{tone}</span>
      </div>
      <div className={`text-3xl font-semibold tabular-nums ${style.value}`}>{value}</div>
      {hint && <div className="text-xs text-muted">{hint}</div>}
      {footer ? <div className="border-t border-border pt-3">{footer}</div> : null}
    </div>
  );
}
