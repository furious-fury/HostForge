import type { TocItem } from "../lib/mdRender";

type DocsTOCProps = {
  items: TocItem[];
};

export function DocsTOC({ items }: DocsTOCProps) {
  if (items.length === 0) return null;
  return (
    <aside className="hidden w-52 shrink-0 xl:block">
      <div className="sticky top-24 space-y-2">
        <p className="font-mono text-[10px] font-semibold uppercase tracking-wide text-muted">On this page</p>
        <ul className="space-y-1.5 border-l border-border pl-3 font-mono text-sm">
          {items.map((h) => (
            <li key={h.id} className={h.depth === 3 ? "pl-2" : ""}>
              <a
                href={`#${h.id}`}
                className="block text-muted transition-colors hover:text-primary"
              >
                {h.text}
              </a>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  );
}
