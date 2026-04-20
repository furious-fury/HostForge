import { NavLink } from "react-router-dom";

import { getDocsGrouped } from "../lib/docs";

export function DocsSidebar() {
  const grouped = getDocsGrouped();
  const groups = [...grouped.keys()].sort((a, b) => a.localeCompare(b));

  return (
    <aside className="hidden w-56 shrink-0 border-r border-border pr-4 md:block">
      <nav className="sticky top-24 space-y-6">
        {groups.map((group) => (
          <div key={group}>
            <p className="font-mono text-[10px] font-semibold uppercase tracking-wide text-muted">{group}</p>
            <ul className="mt-2 space-y-0.5">
              {(grouped.get(group) ?? []).map((doc) => (
                <li key={doc.meta.slug}>
                  <NavLink
                    to={`/docs/${doc.meta.slug}`}
                    className={({ isActive }) =>
                      `block border border-transparent px-2 py-1.5 font-mono text-sm transition-colors ${
                        isActive
                          ? "border-border bg-surface font-semibold text-text"
                          : "text-muted hover:border-border hover:bg-surface-alt/60 hover:text-text"
                      }`
                    }
                  >
                    {doc.meta.title}
                  </NavLink>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </nav>
    </aside>
  );
}
