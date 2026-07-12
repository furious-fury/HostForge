import { Activity, Blocks, ChevronRight, LayoutDashboard, Plus, Settings2 } from "lucide-react";
import { NavLink } from "react-router-dom";
import { RELEASE_LABEL } from "../uiVersion";
import { BrandMark } from "./BrandMark";
import { ButtonLink } from "./Button";

type NavItem = {
  to: string;
  label: string;
  description: string;
  icon: typeof LayoutDashboard;
  end?: boolean;
};

const items: NavItem[] = [
  {
    to: "/",
    label: "Dashboard",
    description: "Fleet health and activity",
    icon: LayoutDashboard,
    end: true,
  },
  {
    to: "/projects",
    label: "Applications",
    description: "Deploy and manage apps",
    icon: Blocks,
  },
  {
    to: "/observability",
    label: "Observability",
    description: "Metrics, incidents, trends",
    icon: Activity,
  },
  {
    to: "/settings",
    label: "Settings",
    description: "Platform configuration",
    icon: Settings2,
  },
];

export function Sidebar() {
  return (
    <aside className="flex min-h-[16rem] flex-col rounded-[28px] border border-border bg-surface/80 shadow-[var(--hf-shadow-panel)] backdrop-blur-xl lg:row-span-2 lg:h-[calc(100vh-2rem)]">
      <div className="border-b border-border px-5 py-5">
        <BrandMark size="md" className="leading-tight" />
        <div className="mt-3 rounded-2xl border border-border bg-surface-alt/70 p-3">
          <div className="mono text-[10px] font-semibold uppercase tracking-[0.24em] text-muted">Application Platform</div>
          <p className="mt-2 text-sm leading-6 text-muted">
            Deploy applications first. Services, domains, and runtime details stay close by when you need them.
          </p>
        </div>
        <ButtonLink to="/projects/new" variant="primary" size="sm" className="mt-4 w-full justify-between rounded-xl">
          <span className="inline-flex items-center gap-2">
            <Plus className="h-4 w-4" />
            New application
          </span>
          <ChevronRight className="h-4 w-4" />
        </ButtonLink>
      </div>

      <nav className="flex-1 space-y-2 overflow-y-auto px-3 py-4">
        {items.map((item) => {
          const Icon = item.icon;
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                [
                  "group flex items-center gap-3 rounded-2xl border px-3 py-3 transition-all duration-150",
                  isActive
                    ? "border-border-strong bg-surface-alt text-text shadow-sm"
                    : "border-transparent text-muted hover:border-border hover:bg-surface-alt/70 hover:text-text",
                ].join(" ")
              }
            >
              {({ isActive }) => (
                <>
                  <div
                    className={[
                      "flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border transition-colors",
                      isActive
                        ? "border-transparent bg-primary text-primary-ink"
                        : "border-border bg-surface text-muted group-hover:text-text",
                    ].join(" ")}
                  >
                    <Icon className="h-4 w-4" />
                  </div>
                  <div className="min-w-0">
                    <div className={isActive ? "font-medium text-text" : "font-medium"}>{item.label}</div>
                    <div className="text-xs text-muted">{item.description}</div>
                  </div>
                </>
              )}
            </NavLink>
          );
        })}
      </nav>

      <div className="border-t border-border px-5 py-4 text-[11px] text-muted">
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="mono uppercase tracking-[0.22em]">{RELEASE_LABEL}</div>
            <div className="mt-1 text-xs text-muted">Application-centric workspace</div>
          </div>
          <span className="mono inline-flex items-center gap-2 rounded-full border border-emerald-400/25 bg-emerald-400/10 px-2.5 py-1 uppercase tracking-[0.18em] text-success">
            <span className="h-2 w-2 rounded-full bg-success" aria-hidden />
            online
          </span>
        </div>
      </div>
    </aside>
  );
}
