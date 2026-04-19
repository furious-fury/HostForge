import { NavLink } from "react-router-dom";
import { RELEASE_LABEL } from "../uiVersion";

type NavItem = {
  to: string;
  label: string;
  end?: boolean;
  disabled?: boolean;
};

type NavGroup = {
  label: string;
  items: NavItem[];
};

const groups: NavGroup[] = [
  {
    label: "Main",
    items: [
      { to: "/", label: "Overview", end: true },
      { to: "/projects", label: "Projects" },
      { to: "/deployments", label: "Deployments" },
    ],
  },
  {
    label: "Observe",
    items: [
      { to: "/observability", label: "Observability" },
      { to: "/logs", label: "Logs", disabled: true },
      { to: "/domains", label: "Domains", disabled: true },
    ],
  },
  {
    label: "System",
    items: [{ to: "/settings", label: "Settings", disabled: true }],
  },
];

export function Sidebar() {
  return (
    <aside className="row-span-2 flex h-screen flex-col border-r border-border bg-surface">
      <div className="flex h-14 shrink-0 flex-col justify-center gap-0.5 border-b border-border px-4">
        <div className="mono text-[10px] font-semibold uppercase leading-tight tracking-[0.2em] text-muted">HostForge</div>
        <div className="text-base font-semibold leading-tight tracking-tight text-text">Control Plane</div>
      </div>

      <nav className="flex-1 overflow-y-auto py-2">
        {groups.map((group) => (
          <div key={group.label} className="px-2 py-2">
            <div className="px-3 pb-1 mono text-[10px] font-semibold uppercase tracking-[0.2em] text-muted">
              {group.label}
            </div>
            <ul className="flex flex-col">
              {group.items.map((item) =>
                item.disabled ? (
                  <li key={item.to}>
                    <span className="block cursor-not-allowed border-l-2 border-transparent px-3 py-2 text-sm text-muted opacity-50">
                      {item.label}
                    </span>
                  </li>
                ) : (
                  <li key={item.to}>
                    <NavLink
                      to={item.to}
                      end={item.end}
                      className={({ isActive }) =>
                        `block border-l-2 px-3 py-2 text-sm ${
                          isActive
                            ? "border-primary bg-surface-alt text-text"
                            : "border-transparent text-muted hover:border-border-strong hover:text-text"
                        }`
                      }
                    >
                      {item.label}
                    </NavLink>
                  </li>
                ),
              )}
            </ul>
          </div>
        ))}
      </nav>

      <div className="border-t border-border px-4 py-3 text-[11px] text-muted">
        <div className="flex items-center justify-between">
          <span className="mono uppercase tracking-wider">{RELEASE_LABEL}</span>
          <span className="mono inline-flex items-center gap-1 border border-success px-1.5 py-0.5 text-success">
            <span aria-hidden>●</span>
            <span>online</span>
          </span>
        </div>
      </div>
    </aside>
  );
}
