import { ReactNode } from "react";

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
};

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-start gap-3 border border-dashed border-border bg-surface-alt p-8">
      <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Empty</div>
      <div className="text-base font-semibold text-text">{title}</div>
      {description && <div className="max-w-prose text-sm text-muted">{description}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
