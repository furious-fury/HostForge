import { motion } from "framer-motion";
import { useMemo } from "react";

import { useSiteTheme } from "../hooks/useSiteTheme";

const sparkPrimary =
  "M0 44 C 48 40, 72 12, 120 22 C 168 32, 200 6, 248 18 C 296 30, 320 38, 360 20 L 400 14";
const sparkInfo =
  "M0 28 C 60 32, 100 8, 160 20 C 220 32, 280 4, 340 16 L 400 22";
const sparkWarn =
  "M0 36 C 80 8, 120 48, 200 16 C 260 40, 300 10, 360 28 L 400 18";
const sparkSuccess =
  "M0 22 C 100 36, 180 4, 260 24 C 300 34, 340 8, 400 20 L 400 26";

function MiniSparkline({ pathD, strokeVar }: { pathD: string; strokeVar: string }) {
  return (
    <svg className="h-10 w-full" viewBox="0 0 400 52" preserveAspectRatio="none" aria-hidden>
      <path d={pathD} fill="none" stroke={strokeVar} strokeLinecap="round" strokeWidth="1.5" />
    </svg>
  );
}

function StatusPillMock({
  label,
  variant,
}: {
  label: string;
  variant: "success" | "danger" | "warning" | "muted";
}) {
  const cls =
    variant === "success"
      ? "border-success text-success"
      : variant === "danger"
        ? "border-danger text-danger"
        : variant === "warning"
          ? "border-warning text-warning"
          : "border-muted text-muted";
  return (
    <span
      className={`mono inline-flex items-center gap-0.5 border px-1.5 py-0.5 text-[8px] font-semibold uppercase tracking-wider ${cls}`}
    >
      <span aria-hidden>●</span>
      {label}
    </span>
  );
}

function StackGlyph({ kind }: { kind: "node" | "html" | "react" | "go" }) {
  const common = {
    className: "shrink-0 text-muted",
    width: 18,
    height: 18,
    viewBox: "0 0 24 24" as const,
    fill: "none" as const,
    stroke: "currentColor",
    strokeWidth: 2,
    "aria-hidden": true as const,
  };
  if (kind === "go") {
    return (
      <svg {...common}>
        <circle cx="12" cy="12" r="9" />
        <path d="M8 10c1.5-2 6.5-2 8 0M8 14c1.5 2 6.5 2 8 0" strokeLinecap="round" />
      </svg>
    );
  }
  if (kind === "node") {
    return (
      <svg {...common}>
        <path d="M12 2 3 7v10l9 5 9-5V7l-9-5Z" strokeLinejoin="round" />
        <path d="M12 22V12" />
        <path d="M3 7l9 5 9-5" />
      </svg>
    );
  }
  if (kind === "html") {
    return (
      <svg {...common}>
        <path d="M4 7h16v10H4z" strokeLinejoin="round" />
        <path d="M8 11h8M8 15h5" strokeLinecap="round" />
      </svg>
    );
  }
  return (
    <svg {...common}>
      <ellipse cx="12" cy="12" rx="9" ry="3.5" transform="rotate(0 12 12)" />
      <ellipse cx="12" cy="12" rx="9" ry="3.5" transform="rotate(60 12 12)" />
      <ellipse cx="12" cy="12" rx="9" ry="3.5" transform="rotate(-60 12 12)" />
      <circle cx="12" cy="12" r="1.8" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function DashboardPreview() {
  const { preference } = useSiteTheme();
  const modKey = useMemo(() => {
    if (typeof navigator === "undefined") return "Ctrl";
    return /Mac|iPhone|iPod|iPad/i.test(navigator.userAgent) ? "⌘" : "Ctrl";
  }, []);

  const themeLabel = preference === "dark" ? "Dark" : "Light";
  const themeIcon = preference === "dark" ? "☾" : "☀";

  return (
    <motion.div
      animate={{ opacity: 1, y: 0 }}
      className="mt-4 w-full max-w-5xl select-none"
      initial={{ opacity: 0, y: 30 }}
      transition={{ duration: 0.8, delay: 0.5 }}
    >
      <div
        className="overflow-hidden p-3 md:p-4"
        style={{
          background: "rgba(17, 24, 39, 0.55)",
          border: "1px solid var(--hf-border-strong)",
          boxShadow: "var(--shadow-dashboard)",
        }}
      >
        <div className="pointer-events-none grid h-[min(28rem,58vh)] max-h-[520px] min-h-[260px] grid-cols-[minmax(0,9.5rem)_1fr] grid-rows-[auto_minmax(0,1fr)] overflow-hidden border border-border bg-surface font-sans text-text shadow-sm">
          {/* Sidebar — matches web Sidebar */}
          <aside className="row-span-2 flex flex-col border-r border-border bg-surface">
            <div className="flex shrink-0 flex-col justify-center gap-0.5 border-b border-border px-3 py-2">
              <div className="mono text-[8px] font-semibold uppercase leading-tight tracking-[0.2em] text-muted">
                HostForge
              </div>
              <div className="text-xs font-semibold leading-tight tracking-tight text-text">Control Plane</div>
            </div>
            <nav className="min-h-0 flex-1 overflow-hidden py-1.5">
              <div className="px-1.5 py-1">
                <div className="mono px-2 pb-0.5 text-[8px] font-semibold uppercase tracking-[0.2em] text-muted">
                  Main
                </div>
                <ul className="flex flex-col">
                  <li>
                    <span className="block border-l-2 border-primary bg-surface-alt px-2 py-1.5 text-[11px] text-text">
                      Overview
                    </span>
                  </li>
                  <li>
                    <span className="block border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted">Projects</span>
                  </li>
                  <li>
                    <span className="block border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted">
                      Deployments
                    </span>
                  </li>
                </ul>
              </div>
              <div className="px-1.5 py-1">
                <div className="mono px-2 pb-0.5 text-[8px] font-semibold uppercase tracking-[0.2em] text-muted">
                  Observe
                </div>
                <ul className="flex flex-col">
                  <li>
                    <span className="block border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted">
                      Observability
                    </span>
                  </li>
                  <li>
                    <span className="block cursor-not-allowed border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted opacity-50">
                      Logs
                    </span>
                  </li>
                  <li>
                    <span className="block cursor-not-allowed border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted opacity-50">
                      Domains
                    </span>
                  </li>
                </ul>
              </div>
              <div className="px-1.5 py-1">
                <div className="mono px-2 pb-0.5 text-[8px] font-semibold uppercase tracking-[0.2em] text-muted">
                  System
                </div>
                <ul className="flex flex-col">
                  <li>
                    <span className="block border-l-2 border-transparent px-2 py-1.5 text-[11px] text-muted">Settings</span>
                  </li>
                </ul>
              </div>
            </nav>
            <div className="mt-auto border-t border-border px-3 py-2 text-[9px] text-muted">
              <div className="flex items-center justify-between gap-1">
                <span className="mono uppercase tracking-wider">v0.7.8</span>
                <span className="mono inline-flex items-center gap-0.5 border border-success px-1 py-0.5 text-[8px] text-success">
                  <span aria-hidden>●</span>
                  <span>online</span>
                </span>
              </div>
            </div>
          </aside>

          {/* Topbar — matches web Topbar */}
          <header className="flex h-11 min-h-11 items-center justify-between gap-2 border-b border-border bg-surface px-3">
            <nav className="flex min-w-0 items-center gap-1.5 text-[11px]" aria-label="Breadcrumb">
              <span className="truncate font-semibold text-text">Overview</span>
            </nav>
            <div className="mx-1 hidden min-w-0 max-w-[14rem] flex-1 md:block">
              <div className="relative">
                <span className="mono pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-[9px] font-semibold uppercase tracking-wider text-muted">
                  ⌕
                </span>
                <div className="mono truncate border border-border bg-surface-alt py-1.5 pl-6 pr-10 text-[10px] text-muted">
                  Search projects and deployments
                </div>
                <span className="mono pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 border border-border px-1 py-0.5 text-[8px] uppercase text-muted">
                  {modKey}K
                </span>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <span className="mono inline-flex items-center gap-1.5 border border-border-strong bg-transparent px-2 py-1 text-[8px] font-semibold uppercase tracking-wider text-text">
                <span aria-hidden>{themeIcon}</span>
                <span className="relative inline-block">
                  <span className="invisible select-none whitespace-nowrap" aria-hidden>
                    Light
                  </span>
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 whitespace-nowrap">{themeLabel}</span>
                </span>
              </span>
              <span className="mono border border-transparent px-2 py-1 text-[8px] font-semibold uppercase tracking-wider text-muted">
                Logout
              </span>
            </div>
          </header>

          {/* Main — Fleet dashboard */}
          <main className="min-h-0 overflow-y-auto overflow-x-hidden bg-bg p-2.5 md:p-3">
            <header className="flex flex-wrap items-end justify-between gap-2">
              <div className="min-w-0">
                <div className="mono text-[9px] font-semibold uppercase tracking-[0.2em] text-muted">Overview</div>
                <h2 className="text-sm font-semibold tracking-tight text-text md:text-base">Fleet status</h2>
                <p className="mt-0.5 max-w-md text-[9px] leading-snug text-muted md:text-[10px]">
                  Live fleet pulse: dozens of services shipping on your metal. Full history on Deployments.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-1">
                <span className="inline-flex border border-border-strong bg-transparent px-2 py-1 text-[8px] font-semibold uppercase tracking-wider text-text">
                  All deployments
                </span>
                <span className="inline-flex border border-border-strong bg-transparent px-2 py-1 text-[8px] font-semibold uppercase tracking-wider text-text">
                  Open Projects
                </span>
                <span className="inline-flex border border-primary bg-primary px-2 py-1 text-[8px] font-semibold uppercase tracking-wider text-primary-ink">
                  + New Project
                </span>
              </div>
            </header>

            <div className="mt-2.5 grid grid-cols-2 gap-1.5 xl:grid-cols-4">
              {[
                { label: "Active Projects", value: "38", tone: "text-text" as const },
                { label: "Deploys (24h)", value: "164", tone: "text-info" as const },
                { label: "Failed (24h)", value: "0", tone: "text-success" as const },
                { label: "Containers Running", value: "36", tone: "text-success" as const },
              ].map((k) => (
                <div key={k.label} className="flex flex-col gap-1 border border-border bg-surface p-2">
                  <div className="mono text-[8px] font-semibold uppercase tracking-[0.18em] text-muted">{k.label}</div>
                  <div className={`text-lg font-semibold tabular-nums md:text-xl ${k.tone}`}>{k.value}</div>
                </div>
              ))}
            </div>

            <section className="mt-2.5 border border-border bg-surface">
              <header className="flex items-center justify-between border-b border-border px-2 py-1.5">
                <span className="text-[9px] font-semibold uppercase tracking-wider text-muted">Host</span>
                <span className="mono text-[8px] font-semibold uppercase tracking-wider text-muted">System →</span>
              </header>
              <div className="grid grid-cols-2 gap-1.5 p-2 sm:grid-cols-4">
                {[
                  { label: "CPU", value: "3.8%", stroke: "var(--hf-primary)", path: sparkPrimary },
                  { label: "Memory", value: "44.2%", stroke: "var(--hf-info)", path: sparkInfo },
                  { label: "Disk (root)", value: "12.6%", stroke: "var(--hf-warning)", path: sparkWarn },
                  { label: "Network", value: "218 Mb/s", stroke: "var(--hf-success)", path: sparkSuccess },
                ].map((h) => (
                  <div key={h.label} className="flex flex-col gap-1 border border-border bg-surface p-2">
                    <div className="mono text-[8px] font-semibold uppercase tracking-[0.18em] text-muted">{h.label}</div>
                    <div className="text-base font-semibold tabular-nums text-success">{h.value}</div>
                    <div className="border-t border-border/60 pt-1">
                      <MiniSparkline pathD={h.path} strokeVar={h.stroke} />
                    </div>
                  </div>
                ))}
              </div>
            </section>

            <div className="mt-2.5 grid grid-cols-1 gap-2 lg:grid-cols-3">
              <section className="border border-border bg-surface lg:col-span-2">
                <header className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-2 py-1.5">
                  <span className="text-[9px] font-semibold uppercase tracking-wider text-muted">Recent activity</span>
                  <div className="flex gap-2">
                    <span className="mono text-[8px] font-semibold uppercase tracking-wider text-muted">
                      All deployments →
                    </span>
                    <span className="mono text-[8px] font-semibold uppercase tracking-wider text-muted">Projects →</span>
                  </div>
                </header>
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[28rem] table-fixed text-left text-[9px]">
                    <thead>
                      <tr className="mono border-b border-border text-[8px] font-semibold uppercase tracking-[0.16em] text-muted">
                        <th className="w-[26%] px-2 py-1">Project</th>
                        <th className="w-[10%] px-2 py-1">Stack</th>
                        <th className="w-[14%] px-2 py-1">Commit</th>
                        <th className="w-[16%] px-2 py-1">Status</th>
                        <th className="w-[16%] px-2 py-1">Started</th>
                        <th className="w-[18%] px-2 py-1">Duration</th>
                      </tr>
                    </thead>
                    <tbody className="text-text">
                      <tr className="border-b border-border/60">
                        <td className="px-2 py-1.5 align-top">
                          <div className="truncate font-semibold">forge-edge</div>
                          <div className="mono truncate text-[8px] text-muted">github.com/hostforge/edge-router</div>
                        </td>
                        <td className="px-2 py-1.5 align-middle">
                          <StackGlyph kind="go" />
                        </td>
                        <td className="mono px-2 py-1.5 align-middle text-[9px]">f4a91c2</td>
                        <td className="px-2 py-1.5 align-middle">
                          <StatusPillMock label="SUCCESS" variant="success" />
                        </td>
                        <td className="px-2 py-1.5 align-middle text-muted">Just now</td>
                        <td className="mono px-2 py-1.5 align-middle">38s</td>
                      </tr>
                      <tr className="border-b border-border/60">
                        <td className="px-2 py-1.5 align-top">
                          <div className="truncate font-semibold">api-prod</div>
                          <div className="mono truncate text-[8px] text-muted">github.com/acme/api-prod</div>
                        </td>
                        <td className="px-2 py-1.5 align-middle">
                          <StackGlyph kind="node" />
                        </td>
                        <td className="mono px-2 py-1.5 align-middle text-[9px]">8d3e1b7</td>
                        <td className="px-2 py-1.5 align-middle">
                          <StatusPillMock label="SUCCESS" variant="success" />
                        </td>
                        <td className="px-2 py-1.5 align-middle text-muted">3 min ago</td>
                        <td className="mono px-2 py-1.5 align-middle">1m 04s</td>
                      </tr>
                      <tr className="border-b border-border/60">
                        <td className="px-2 py-1.5 align-top">
                          <div className="truncate font-semibold">marketing-www</div>
                          <div className="mono truncate text-[8px] text-muted">github.com/acme/marketing-www</div>
                        </td>
                        <td className="px-2 py-1.5 align-middle">
                          <StackGlyph kind="html" />
                        </td>
                        <td className="mono px-2 py-1.5 align-middle text-[9px]">c0ffee0</td>
                        <td className="px-2 py-1.5 align-middle">
                          <StatusPillMock label="BUILDING" variant="warning" />
                        </td>
                        <td className="px-2 py-1.5 align-middle text-muted">6 min ago</td>
                        <td className="mono px-2 py-1.5 align-middle">—</td>
                      </tr>
                      <tr>
                        <td className="px-2 py-1.5 align-top">
                          <div className="truncate font-semibold">control-ui</div>
                          <div className="mono truncate text-[8px] text-muted">github.com/hostforge/control-ui</div>
                        </td>
                        <td className="px-2 py-1.5 align-middle">
                          <StackGlyph kind="react" />
                        </td>
                        <td className="mono px-2 py-1.5 align-middle text-[9px]">2b9a441</td>
                        <td className="px-2 py-1.5 align-middle">
                          <StatusPillMock label="SUCCESS" variant="success" />
                        </td>
                        <td className="px-2 py-1.5 align-middle text-muted">22 min ago</td>
                        <td className="mono px-2 py-1.5 align-middle">2m 18s</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section>

              <section className="border border-border bg-surface">
                <header className="border-b border-border px-2 py-1.5">
                  <span className="text-[9px] font-semibold uppercase tracking-wider text-muted">System</span>
                </header>
                <div className="p-2">
                  <p className="mb-2 text-[8px] leading-snug text-muted">
                    Health checks for this server, updated live.
                  </p>
                  <ul className="flex flex-col divide-y divide-border">
                    <li className="py-1.5">
                      <div className="flex items-start justify-between gap-1">
                        <span className="text-[10px] text-muted">Docker daemon</span>
                        <StatusPillMock label="RUNNING" variant="success" />
                      </div>
                    </li>
                    <li className="py-1.5">
                      <div className="flex items-start justify-between gap-1">
                        <span className="text-[10px] text-muted">Caddy</span>
                        <StatusPillMock label="READY" variant="success" />
                      </div>
                    </li>
                    <li className="py-1.5">
                      <div className="flex items-start justify-between gap-1">
                        <span className="text-[10px] text-muted">Webhook route</span>
                        <StatusPillMock label="READY" variant="success" />
                      </div>
                    </li>
                    <li className="flex items-center justify-between py-1.5 text-[10px]">
                      <span className="text-muted">Build version</span>
                      <span className="mono text-[9px] text-text">v0.7.8</span>
                    </li>
                  </ul>
                  <div className="mt-2 border-t border-border pt-2">
                    <div className="mono mb-1 text-[8px] font-semibold uppercase tracking-[0.18em] text-muted">
                      Quick Actions
                    </div>
                    <div className="flex flex-col gap-1">
                      <span className="inline-flex justify-center border border-primary bg-primary px-2 py-1 text-center text-[8px] font-semibold uppercase tracking-wider text-primary-ink">
                        + New Project
                      </span>
                      <span className="inline-flex justify-center border border-border-strong bg-transparent px-2 py-1 text-center text-[8px] font-semibold uppercase tracking-wider text-text">
                        Open Projects
                      </span>
                    </div>
                  </div>
                </div>
              </section>
            </div>
          </main>
        </div>
      </div>
    </motion.div>
  );
}
