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
  bodyClassName = "p-5 sm:p-6",
  noBody = false,
}: PanelProps) {
  const borderClass = tone === "danger" ? "border-danger" : "border-border";
  return (
    <section className={`overflow-hidden rounded-panel border ${borderClass} bg-surface shadow-[var(--hf-shadow-panel)] ${className}`}>
      {title !== undefined && (
        <header className={`flex items-center justify-between gap-4 border-b ${borderClass} px-5 py-4 sm:px-6`}>
          <div className="text-sm font-semibold text-text">{title}</div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </header>
      )}
      {noBody ? children : <div className={bodyClassName}>{children}</div>}
    </section>
  );
}
