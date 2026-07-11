import { motion } from "framer-motion";

import { CodeBlock } from "./CodeBlock";

const INSTALL_SNIPPET = `# On a fresh Debian/Ubuntu host
curl -fsSL https://raw.githubusercontent.com/your-org/hostforge/main/scripts/install.sh | sudo bash`;

const CLI_SNIPPET = `# Authenticate once, then deploy any repo
hostforge login https://forge.example.com
hostforge project create api-prod --repo https://github.com/acme/api
hostforge deploy api-prod --ref main
hostforge caddy sync`;

const VIEWPORT = { once: true, margin: "0px 0px -10% 0px" } as const;

export function CodeSampleSection() {
  return (
    <section id="install" className="w-full border-t border-border bg-bg" aria-labelledby="install-heading">
      <div className="mx-auto grid max-w-6xl gap-10 px-6 py-16 md:py-24 lg:grid-cols-2 lg:gap-16">
        <motion.div
          initial={{ opacity: 0, x: -16 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={VIEWPORT}
          transition={{ duration: 0.55, ease: [0.4, 0, 0.2, 1] }}
          className="flex flex-col justify-center"
        >
          <p className="mb-3 font-mono text-xs font-semibold uppercase tracking-[0.22em] text-primary">Installation</p>
          <h2 id="install-heading" className="font-mono text-3xl font-semibold tracking-tight text-text md:text-4xl">
            One script on one host. Deploys in minutes.
          </h2>
          <p className="mt-4 text-base leading-relaxed text-muted md:text-lg">
            The installer provisions the daemon, Docker, Caddy, and the control plane. Point a DNS record at the box and
            you're shipping — no Kubernetes, no external control plane, no per-seat pricing.
          </p>
          <ul className="mt-6 space-y-2 text-sm text-muted">
            <li className="flex gap-2">
              <span className="text-primary" aria-hidden>
                ▸
              </span>
              <span>Systemd units for the daemon and Caddy</span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary" aria-hidden>
                ▸
              </span>
              <span>
                SQLite at <span className="font-mono text-text">/var/lib/hostforge/hostforge.db</span>
              </span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary" aria-hidden>
                ▸
              </span>
              <span>Automatic HTTPS via Caddy's built-in ACME</span>
            </li>
          </ul>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, x: 16 }}
          whileInView={{ opacity: 1, x: 0 }}
          viewport={VIEWPORT}
          transition={{ duration: 0.55, delay: 0.1, ease: [0.4, 0, 0.2, 1] }}
          className="flex flex-col gap-4"
        >
          <CodeBlock title="install" language="bash" code={INSTALL_SNIPPET} />
          <CodeBlock title="first deploy" language="hostforge" code={CLI_SNIPPET} />
        </motion.div>
      </div>
    </section>
  );
}
