import { Link, NavLink } from "react-router-dom";

import { useSiteTheme } from "../hooks/useSiteTheme";
import { ThemeToggle } from "./ThemeToggle";

const linkClass = ({ isActive }: { isActive: boolean }) =>
  `font-mono text-xs font-medium uppercase tracking-wide transition-colors ${
    isActive ? "text-text" : "text-muted hover:text-text"
  }`;

export function Navbar() {
  const { preference, cycleTheme } = useSiteTheme();
  return (
    <header className="relative z-20 flex shrink-0 items-center justify-between border-b border-border bg-bg/95 px-6 py-4 backdrop-blur md:px-12 lg:px-20">
      <Link to="/" className="flex items-center gap-2 font-mono text-lg font-semibold tracking-tight text-text">
        <span className="text-primary" aria-hidden>
          ✦
        </span>
        <span>HostForge</span>
      </Link>
      <nav className="hidden items-center gap-8 md:flex">
        <NavLink to="/" className={linkClass} end>
          Home
        </NavLink>
        <NavLink to="/docs/introduction" className={linkClass}>
          Docs
        </NavLink>
        <a
          href="https://github.com/search?q=HostForge+PaaS&type=repositories"
          className="font-mono text-xs font-medium uppercase tracking-wide text-muted transition-colors hover:text-text"
          rel="noreferrer"
          target="_blank"
        >
          GitHub
        </a>
      </nav>
      <div className="flex shrink-0 items-center gap-2 md:gap-3">
        <ThemeToggle onCycle={cycleTheme} preference={preference} />
        <Link
          to="/docs/installation"
          className="border border-border-strong bg-primary px-4 py-2 font-mono text-xs font-semibold uppercase tracking-wide text-primary-ink transition-opacity hover:opacity-90 md:px-5"
        >
          Install
        </Link>
      </div>
    </header>
  );
}
