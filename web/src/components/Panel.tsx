import { ReactNode } from "react";
import { Card, CardContent, CardHeader } from "./ui/card";

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
  return (
    <Card className={`${tone === "danger" ? "border-danger" : ""} ${className}`}>
      {title !== undefined && (
        <CardHeader className={tone === "danger" ? "border-danger" : ""}>
          <div className="text-sm font-semibold text-text">{title}</div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </CardHeader>
      )}
      {noBody ? children : <CardContent className={bodyClassName}>{children}</CardContent>}
    </Card>
  );
}
