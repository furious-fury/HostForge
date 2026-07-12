import { Activity, Blocks, LayoutDashboard, Plus, Settings2 } from "lucide-react";
import { NavLink } from "react-router-dom";
import { RELEASE_LABEL } from "../uiVersion";
import { BrandMark } from "./BrandMark";
import { ButtonLink } from "./Button";

type NavItem = {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
  end?: boolean;
};

const items: NavItem[] = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
  { to: "/projects", label: "Applications", icon: Blocks },
  { to: "/observability", label: "Observability", icon: Activity },
  { to: "/settings", label: "Settings", icon: Settings2 },
];

export function Sidebar() {
  return (
    <aside className="border-b border-border bg-surface lg:min-h-screen lg:border-b-0 lg:border-r">
      <div className="flex h-full flex-col px-4 py-4">
        <div className="flex items-center justify-between gap-3 px-2 py-2">
          <BrandMark size="md" className="leading-tight" />
        </div>

        <ButtonLink to="/projects/new" variant="primary" size="sm" className="mt-4 w-full justify-center">
          <Plus className="h-4 w-4" />
          New application
        </ButtonLink>

        <nav className="mt-6 flex-1 space-y-1">
          {items.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) =>
                  [
                    "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors",
                    isActive ? "bg-surface-alt text-text" : "text-muted hover:bg-surface-alt hover:text-text",
                  ].join(" ")
                }
              >
                <Icon className="h-4 w-4" />
                <span>{item.label}</span>
              </NavLink>
            );
          })}
        </nav>

        <div className="border-t border-border px-2 pt-4 text-xs text-muted">
          <div className="mono text-[10px] uppercase tracking-[0.18em]">{RELEASE_LABEL}</div>
          <div className="mt-2 flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-success" aria-hidden />
            <span>Platform online</span>
          </div>
        </div>
      </div>
    </aside>
  );
}
