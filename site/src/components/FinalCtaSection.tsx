import { Link } from "react-router-dom";
import { motion } from "framer-motion";

const VIEWPORT = { once: true, margin: "0px 0px -10% 0px" } as const;

export function FinalCtaSection() {
  return (
    <section
      id="get-started"
      className="relative w-full overflow-hidden border-t border-border bg-bg"
      aria-labelledby="cta-heading"
    >
      <motion.div
        aria-hidden
        initial={{ opacity: 0, scale: 1.04 }}
        whileInView={{ opacity: 0.35, scale: 1 }}
        viewport={VIEWPORT}
        transition={{ duration: 0.9, ease: [0.4, 0, 0.2, 1] }}
        className="hf-grid-bg pointer-events-none absolute inset-0 z-0"
      />
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={VIEWPORT}
        transition={{ duration: 0.55, ease: [0.4, 0, 0.2, 1] }}
        className="relative z-10 mx-auto flex max-w-4xl flex-col items-center px-6 py-24 text-center md:py-32"
      >
        <p className="mb-3 font-mono text-xs font-semibold uppercase tracking-[0.22em] text-primary">Ship from Git</p>
        <h2 id="cta-heading" className="font-mono text-4xl font-semibold tracking-tight text-text md:text-5xl">
          One box. Your code. Production.
        </h2>
        <p className="mt-5 max-w-xl text-base leading-relaxed text-muted md:text-lg">
          Read the docs, install in ten minutes, and own the whole pipeline from commit to cutover.
        </p>
        <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row">
          <Link
            to="/docs/introduction"
            className="border border-border-strong bg-primary px-6 py-3 font-mono text-sm font-semibold uppercase tracking-wide text-primary-ink transition-opacity hover:opacity-90"
          >
            Read the docs
          </Link>
          <a
            href="https://github.com/search?q=HostForge+PaaS&type=repositories"
            target="_blank"
            rel="noreferrer"
            className="border border-border bg-surface px-6 py-3 font-mono text-sm font-semibold uppercase tracking-wide text-text transition-colors hover:border-border-strong"
          >
            View on GitHub
          </a>
        </div>
      </motion.div>
    </section>
  );
}
