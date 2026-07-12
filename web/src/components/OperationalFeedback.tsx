import { ReactNode } from "react";
import { Button } from "./Button";
import { Skeleton } from "./ui/skeleton";

type NoticeTone = "info" | "warning" | "danger" | "success";

const toneClass: Record<NoticeTone, string> = {
  info: "border-info bg-surface-alt",
  warning: "border-warning bg-warning/10",
  danger: "border-danger bg-danger/10",
  success: "border-success bg-success/10",
};

export function OperationalNotice({
  title,
  children,
  tone = "info",
  action,
  live = false,
}: {
  title: string;
  children?: ReactNode;
  tone?: NoticeTone;
  action?: ReactNode;
  live?: boolean;
}) {
  return (
    <div
      className={`flex flex-wrap items-start justify-between gap-3 rounded-panel border px-4 py-3 text-sm text-text ${toneClass[tone]}`}
      role={tone === "danger" ? "alert" : "status"}
      aria-live={live ? "polite" : undefined}
    >
      <div className="min-w-0 flex-1">
        <p className="font-semibold">{title}</p>
        {children ? <div className="mt-1 text-xs text-muted">{children}</div> : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="space-y-3 p-5" role="status" aria-live="polite" aria-label={label}>
      <span className="sr-only">{label}</span>
      <Skeleton className="h-4 w-2/5" />
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-4/5" />
    </div>
  );
}

export function RetryNotice({ title, detail, onRetry, retryLabel = "Try again" }: { title: string; detail?: ReactNode; onRetry: () => void; retryLabel?: string }) {
  return (
    <OperationalNotice title={title} tone="danger" action={<Button type="button" size="sm" variant="secondary" onClick={onRetry}>{retryLabel}</Button>}>
      {detail}
    </OperationalNotice>
  );
}
