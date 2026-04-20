import { Link } from "react-router-dom";

import type { DocMeta } from "../lib/docs";
import { useSiteTheme } from "../hooks/useSiteTheme";
import type { TocItem } from "../lib/mdRender";
import { DocsSidebar } from "./DocsSidebar";
import { DocsTOC } from "./DocsTOC";
import { Prose } from "./Prose";
import { ThemeToggle } from "./ThemeToggle";

type DocsLayoutProps = {
  meta: DocMeta;
  slug: string;
  headings: TocItem[];
  html: string;
};

export function DocsLayout({ meta, slug, headings, html }: DocsLayoutProps) {
  const { preference, cycleTheme } = useSiteTheme();
  return (
    <div className="min-h-screen bg-bg font-sans text-text">
      <header className="border-b border-border bg-bg/95 backdrop-blur">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-6 py-4">
          <div className="flex items-center gap-4">
            <Link to="/" className="flex items-center gap-2 font-mono text-lg font-semibold tracking-tight text-text">
              <span className="text-primary" aria-hidden>
                ✦
              </span>
              HostForge
            </Link>
            <span className="hidden font-mono text-xs font-medium uppercase tracking-wide text-muted sm:inline">
              Documentation
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-2 font-mono text-xs md:gap-3">
            <ThemeToggle onCycle={cycleTheme} preference={preference} />
            <a
              className="text-muted transition-colors hover:text-text"
              href={`/docs/${slug}.md`}
              rel="noreferrer"
              target="_blank"
            >
              View raw .md
            </a>
            <Link
              className="border border-border-strong bg-primary px-4 py-2 font-semibold uppercase tracking-wide text-primary-ink"
              to="/"
            >
              Home
            </Link>
          </div>
        </div>
      </header>

      <div className="mx-auto flex max-w-6xl gap-8 px-6 py-10">
        <DocsSidebar />
        <div className="flex min-w-0 flex-1 gap-10">
          <div className="min-w-0 flex-1">
            <p className="font-mono text-xs font-semibold uppercase tracking-wide text-muted">{meta.group}</p>
            <h1 className="mt-2 font-mono text-3xl font-semibold tracking-tight text-text md:text-4xl">{meta.title}</h1>
            <p className="mt-2 text-sm text-muted">{meta.description}</p>
            <div className="mt-8">
              <Prose html={html} />
            </div>
          </div>
          <DocsTOC items={headings} />
        </div>
      </div>
    </div>
  );
}
