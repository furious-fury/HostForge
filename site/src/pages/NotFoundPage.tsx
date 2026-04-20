import { Link } from "react-router-dom";
import { Head } from "vite-react-ssg";

import { useSiteTheme } from "../hooks/useSiteTheme";
import { ThemeToggle } from "../components/ThemeToggle";

export function NotFoundPage() {
  const { preference, cycleTheme } = useSiteTheme();
  return (
    <>
      <Head>
        <title>Not found · HostForge</title>
      </Head>
      <div className="flex min-h-screen flex-col items-center justify-center bg-bg px-6 text-text">
        <div className="absolute right-4 top-4 md:right-8">
          <ThemeToggle onCycle={cycleTheme} preference={preference} />
        </div>
        <p className="font-mono text-4xl font-bold tracking-tight text-primary">404</p>
        <p className="mt-2 text-center text-sm text-muted">This page does not exist.</p>
        <Link
          to="/"
          className="mt-6 border border-border-strong bg-primary px-5 py-2.5 font-mono text-sm font-semibold text-primary-ink"
        >
          Back home
        </Link>
      </div>
    </>
  );
}
