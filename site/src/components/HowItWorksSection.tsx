import { motion } from "framer-motion";

type Step = {
  number: string;
  title: string;
  body: string;
};

const STEPS: Step[] = [
  {
    number: "01",
    title: "Clone",
    body: "Repo cloned into a deterministic worktree keyed by project + commit.",
  },
  {
    number: "02",
    title: "Build",
    body: "Nixpacks detects the stack and ships hostforge/<slug>:<utc-build-id>.",
  },
  {
    number: "03",
    title: "Candidate",
    body: "A candidate container starts on a fresh host port, env injected from encrypted storage.",
  },
  {
    number: "04",
    title: "Health",
    body: "Loopback probe confirms the candidate responds before any traffic is sent its way.",
  },
  {
    number: "05",
    title: "Cutover",
    body: "Caddy upstreams flip atomically; the previous SUCCESS keeps serving if the candidate fails.",
  },
];

const VIEWPORT = { once: true, margin: "0px 0px -10% 0px" } as const;

export function HowItWorksSection() {
  return (
    <section id="how-it-works" className="w-full border-t border-border bg-surface-alt" aria-labelledby="how-heading">
      <div className="mx-auto max-w-6xl px-6 py-16 md:py-24">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={VIEWPORT}
          transition={{ duration: 0.5, ease: [0.4, 0, 0.2, 1] }}
          className="mb-12 max-w-2xl"
        >
          <p className="mb-3 font-mono text-xs font-semibold uppercase tracking-[0.22em] text-primary">
            Deploy pipeline
          </p>
          <h2 id="how-heading" className="font-mono text-3xl font-semibold tracking-tight text-text md:text-4xl">
            Push a commit. Five steps later it's live.
          </h2>
          <p className="mt-4 text-base text-muted md:text-lg">
            The same deterministic path runs on every deploy — from a manual CLI push to a GitHub webhook.
          </p>
        </motion.div>

        <div className="relative">
          <motion.div
            aria-hidden
            initial={{ scaleX: 0 }}
            whileInView={{ scaleX: 1 }}
            viewport={VIEWPORT}
            transition={{ duration: 0.9, ease: [0.4, 0, 0.2, 1] }}
            style={{ transformOrigin: "0% 50%" }}
            className="pointer-events-none absolute left-0 right-0 top-1/2 hidden h-px -translate-y-1/2 bg-border-strong lg:block"
          />
          <div className="relative grid grid-cols-1 gap-4 lg:grid-cols-5 lg:gap-0">
            {STEPS.map((s, i) => (
              <motion.div
                key={s.number}
                initial={{ opacity: 0, y: 18, scale: 0.97 }}
                whileInView={{ opacity: 1, y: 0, scale: 1 }}
                viewport={VIEWPORT}
                transition={{ duration: 0.5, delay: Math.min(i * 0.08, 0.4), ease: [0.4, 0, 0.2, 1] }}
                className="relative flex flex-1 flex-col border border-border bg-surface p-6"
              >
                <span className="font-mono text-3xl font-semibold tracking-tight text-primary md:text-4xl">
                  {s.number}
                </span>
                <h3 className="mt-3 font-mono text-lg font-semibold text-text">{s.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-muted">{s.body}</p>
              </motion.div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
