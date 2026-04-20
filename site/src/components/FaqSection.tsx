import { motion } from "framer-motion";

type QA = { q: string; a: string };

const FAQS: QA[] = [
  {
    q: "Is HostForge self-hosted only?",
    a: "Yes. There is no managed service. You install the daemon on your own Linux host and own every bit of the stack — binaries, database, containers, and traffic.",
  },
  {
    q: "Which operating systems are supported?",
    a: "Debian 12+ and Ubuntu 22.04+ are the primary targets. Any recent systemd-based Linux with Docker should work; the installer assumes apt for dependencies.",
  },
  {
    q: "Do I have to use Caddy?",
    a: "Caddy ships as the default reverse proxy because of its atomic config reloads and automatic HTTPS. The cutover logic is decoupled enough that swapping in Traefik or nginx is possible, but unsupported today.",
  },
  {
    q: "How does authentication work?",
    a: "Local accounts stored in SQLite with hashed passwords. The control plane issues short-lived session tokens; the CLI uses API tokens minted from the UI. No SSO yet.",
  },
  {
    q: "How do I upgrade HostForge itself?",
    a: "Re-run the installer script or update the systemd-managed binary. Schema migrations run on start; SQLite means a cp of the database file is your backup.",
  },
];

const VIEWPORT = { once: true, margin: "0px 0px -10% 0px" } as const;

export function FaqSection() {
  return (
    <section id="faq" className="w-full border-t border-border bg-bg" aria-labelledby="faq-heading">
      <div className="mx-auto max-w-4xl px-6 py-16 md:py-24">
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={VIEWPORT}
          transition={{ duration: 0.5, ease: [0.4, 0, 0.2, 1] }}
          className="mb-10"
        >
          <p className="mb-3 font-mono text-xs font-semibold uppercase tracking-[0.22em] text-primary">FAQ</p>
          <h2 id="faq-heading" className="font-mono text-3xl font-semibold tracking-tight text-text md:text-4xl">
            Answers before you clone.
          </h2>
        </motion.div>

        <div>
          {FAQS.map((qa, i) => (
            <motion.div
              key={qa.q}
              initial={{ opacity: 0, y: 10 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={VIEWPORT}
              transition={{ duration: 0.45, delay: Math.min(i * 0.08, 0.4), ease: [0.4, 0, 0.2, 1] }}
              className="border border-t-0 border-border bg-surface p-6 first:border-t"
            >
              <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-primary">Q</p>
              <h3 className="mt-1 font-mono text-lg font-semibold text-text">{qa.q}</h3>
              <p className="mt-3 text-sm leading-relaxed text-muted">{qa.a}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  );
}
