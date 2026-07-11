import { ReactNode } from "react";

type PanelProps = {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  tone?: "default" | "danger";
  className?: string;
  bodyClassName?: string;
  noBody?: boolean;
};

export function Panel({
  title,
  actions,
  children,
  tone = "default",
  className = "",
  bodyClassName = "p-4",
  noBody = false,
}: PanelProps) {
  const borderClass = tone === "danger" ? "border-danger" : "border-border";
  return (
    <section className={`border ${borderClass} bg-surface ${className}`}>
      {title !== undefined && (
        <header className={`flex items-center justify-between border-b ${borderClass} px-4 py-3`}>
          <div className="text-xs font-semibold uppercase tracking-wider text-muted">{title}</div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </header>
      )}
      {noBody ? children : <div className={bodyClassName}>{children}</div>}
    </section>
  );
}
