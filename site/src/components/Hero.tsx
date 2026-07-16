import { motion } from "framer-motion";
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
        Ship from GitHub to <span className="text-primary">your</span> server.
      </motion.h1>

      <motion.p
        animate={{ opacity: 1, y: 0 }}
        className="relative z-10 mt-4 max-w-[650px] text-center text-base leading-relaxed text-muted md:text-lg"
        initial={{ opacity: 0, y: 16 }}
        transition={{ duration: 0.6, delay: 0.2 }}
      >
        HostForge maps GitHub App repositories and branches to production or staging, builds with Railpack and
        BuildKit, keeps containers on private host ports, and publishes only validated Caddy routes. SQLite stores
        applications, services, deployments, encrypted configuration, and observability data.
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
      </motion.div>
    </div>
  );
}
