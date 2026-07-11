import { ReactNode } from "react";

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
};

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-start gap-3 rounded-panel border border-dashed border-border bg-surface-alt p-6 sm:p-8">
      <div className="text-xs font-medium text-muted">No results</div>
      <div className="text-lg font-semibold tracking-tight text-text">{title}</div>
      {description && <div className="max-w-prose text-sm text-muted">{description}</div>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
