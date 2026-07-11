import { motion } from "framer-motion";
import { Play } from "lucide-react";
import { Link } from "react-router-dom";

export function Hero() {
  return (
    <div className="relative z-10 flex w-full shrink-0 flex-col items-center overflow-hidden px-6 pt-2">
      <div className="pointer-events-none absolute inset-0 z-0 bg-gradient-to-b from-primary/20 via-bg/90 to-bg" />

      <motion.div
        animate={{ opacity: 1, y: 0 }}
        className="relative z-10 mb-6 mt-2 inline-flex items-center gap-1.5 border border-border bg-surface px-4 py-1.5 font-mono text-[11px] font-medium uppercase tracking-wide text-muted"
        initial={{ opacity: 0, y: 10 }}
        transition={{ duration: 0.5 }}
      >
        Self-hosted PaaS for one machine
      </motion.div>

      <motion.h1
        animate={{ opacity: 1, y: 0 }}
        className="relative z-10 max-w-xl text-center font-mono text-4xl font-semibold leading-tight tracking-tight text-text md:text-5xl lg:text-6xl"
        initial={{ opacity: 0, y: 16 }}
        transition={{ duration: 0.6, delay: 0.1 }}
      >
        Ship from Git in <span className="text-primary">one</span> command.
      </motion.h1>

      <motion.p
        animate={{ opacity: 1, y: 0 }}
        className="relative z-10 mt-4 max-w-[650px] text-center text-base leading-relaxed text-muted md:text-lg"
        initial={{ opacity: 0, y: 16 }}
        transition={{ duration: 0.6, delay: 0.2 }}
      >
        Push to GitHub (or run the CLI): HostForge clones your repo, builds with Nixpacks, runs a Docker container on a
        published host port, and—when you wire Caddy—terminates TLS on the public internet. SQLite keeps projects,
        deployments, domains, and observability rows.
      </motion.p>

      <motion.div
        animate={{ opacity: 1, y: 0 }}
        className="relative z-10 mt-5 flex items-center gap-3"
        initial={{ opacity: 0, y: 16 }}
        transition={{ duration: 0.6, delay: 0.3 }}
      >
        <Link
          to="/docs/introduction"
          className="border border-border-strong bg-primary px-6 py-3 font-mono text-sm font-semibold text-primary-ink transition-opacity hover:opacity-90"
        >
          Read the docs
        </Link>
        <button
          type="button"
          className="flex h-11 w-11 items-center justify-center border border-border bg-surface text-text shadow-[0_2px_12px_rgba(0,0,0,0.35)] transition-colors hover:bg-surface-alt"
          aria-label="Play overview (coming soon)"
        >
          <Play className="h-4 w-4 fill-text" />
        </button>
      </motion.div>
    </div>
  );
}
