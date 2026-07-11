import { motion } from "framer-motion";

type Feature = {
  eyebrow: string;
  title: string;
  body: string;
};

const FEATURES: Feature[] = [
  {
    eyebrow: "01",
    title: "Git → Nixpacks → Docker",
    body: "Clone any repo, auto-detect the stack with Nixpacks, and ship a Docker image tagged with a deterministic UTC build id.",
  },
  {
    eyebrow: "02",
    title: "Caddy zero-downtime cutover",
    body: "Each deploy lands on a candidate container, passes a loopback health probe, then Caddy flips upstreams atomically.",
  },
  {
    eyebrow: "03",
    title: "Live WebSocket logs",
    body: "Stream build + runtime logs straight from the daemon into the control plane and CLI with no polling.",
  },
  {
    eyebrow: "04",
    title: "Observability built in",
    body: "HTTP access logs, deploy step timings, and failure reasons surfaced per project — no external stack required.",
  },
  {
    eyebrow: "05",
    title: "Linux host metrics",
    body: "CPU, memory, disk, and network panels read directly from the host — the same box that serves your apps.",
  },
  {
    eyebrow: "06",
    title: "Encrypted project env",
    body: "Per-project environment variables stored encrypted at rest and injected into containers at deploy time.",
  },
  {
    eyebrow: "07",
    title: "GitHub webhooks",
    body: "Wire a webhook once; HostForge redeploys on push with signature verification and idempotent build ids.",
  },
  {
    eyebrow: "08",
    title: "React UI + command palette",
    body: "Brutalist, keyboard-first control plane with ⌘K, live deploy status, and a unified light/dark theme.",
  },
  {
    eyebrow: "09",
    title: "SQLite persistence",
    body: "A single file at rest. Back it up with cp. No Postgres, no Redis, no external dependencies to babysit.",
  },
];

const VIEWPORT = { once: true, margin: "0px 0px -10% 0px" } as const;

export function FeatureGridSection() {
  return (
    <section id="features" className="w-full border-t border-border bg-bg" aria-labelledby="features-heading">
      <div className="mx-auto max-w-6xl px-6 py-16 md:py-24">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={VIEWPORT}
          transition={{ duration: 0.5, ease: [0.4, 0, 0.2, 1] }}
          className="mb-12 max-w-2xl"
        >
          <p className="mb-3 font-mono text-xs font-semibold uppercase tracking-[0.22em] text-primary">Capabilities</p>
          <h2 id="features-heading" className="font-mono text-3xl font-semibold tracking-tight text-text md:text-4xl">
            Everything a single-host PaaS actually needs.
          </h2>
          <p className="mt-4 text-base text-muted md:text-lg">
            Nine primitives that turn one Linux box into a production-ready deploy target — without handing your
            infrastructure to anyone.
          </p>
        </motion.div>

        <div className="grid grid-cols-1 border border-border sm:grid-cols-2 lg:grid-cols-3">
          {FEATURES.map((f, i) => (
            <motion.article
              key={f.eyebrow}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={VIEWPORT}
              transition={{ duration: 0.45, delay: Math.min(i * 0.05, 0.35), ease: [0.4, 0, 0.2, 1] }}
              className="flex flex-col border-b border-r border-border bg-surface p-6 sm:[&:nth-child(2n)]:border-r-0 lg:[&:nth-child(2n)]:border-r lg:[&:nth-child(3n)]:border-r-0"
            >
              <span className="font-mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">
                {f.eyebrow}
              </span>
              <h3 className="mt-3 font-mono text-lg font-semibold text-text">{f.title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-muted">{f.body}</p>
            </motion.article>
          ))}
        </div>
      </div>
    </section>
  );
}
