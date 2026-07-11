import { useSearchParams } from "react-router-dom";
import { Panel } from "../components/Panel";
import { useSettingsQuery } from "../hooks/settingsQueries";
import { AccountSection } from "./settings/AccountSection";
import { AboutSection } from "./settings/AboutSection";
import { CaddySection } from "./settings/CaddySection";
import { DeploySection } from "./settings/DeploySection";
import { DnsSection } from "./settings/DnsSection";
import { GitHubAppSection } from "./settings/GitHubAppSection";
import { PreferencesSection } from "./settings/PreferencesSection";
import { SystemSection } from "./settings/SystemSection";
import { WebhooksSection } from "./settings/WebhooksSection";

const TABS = [
  { id: "account", label: "Account" },
  { id: "system", label: "System" },
  { id: "deploy", label: "Deploy" },
  { id: "caddy", label: "Caddy" },
  { id: "github-app", label: "GitHub App" },
  { id: "webhooks", label: "Webhooks" },
  { id: "dns", label: "DNS" },
  { id: "preferences", label: "Preferences" },
  { id: "about", label: "About" },
] as const;

type TabId = (typeof TABS)[number]["id"];

function isTabId(s: string | null): s is TabId {
  return TABS.some((t) => t.id === s);
}

export function SettingsPage() {
  const [params, setParams] = useSearchParams();
  const tabParam = params.get("tab");
  const active: TabId = isTabId(tabParam) ? tabParam : "account";

  const settingsQ = useSettingsQuery();

  function setTab(id: TabId) {
    const next = new URLSearchParams(params);
    next.set("tab", id);
    setParams(next, { replace: true });
  }

  return (
    <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
      <aside className="w-full shrink-0 lg:w-56">
        <Panel title="Settings">
          <nav className="flex flex-col py-1" role="tablist" aria-label="Settings sections">
            {TABS.map((t) => {
              const isActive = active === t.id;
              return (
                <button
                  key={t.id}
                  type="button"
                  role="tab"
                  aria-selected={isActive}
                  id={`settings-tab-${t.id}`}
                  onClick={() => setTab(t.id)}
                  className={[
                    "block w-full border-l-2 py-2.5 pl-3 pr-2 text-left text-sm transition-colors",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-bg",
                    isActive
                      ? "border-primary bg-surface-alt font-semibold tracking-tight text-text"
                      : "border-transparent text-muted hover:bg-surface-alt/50 hover:text-text",
                  ].join(" ")}
                >
                  {t.label}
                </button>
              );
            })}
          </nav>
        </Panel>
      </aside>

      <div className="min-w-0 flex-1 space-y-4">
        <header>
          <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">System</div>
          <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
          <p className="mt-1 text-sm text-muted">
            Runtime snapshot, safe actions, and UI-only preferences. Server secrets are never returned in JSON. The{" "}
            <span className="font-medium text-text">System</span> tab includes live host CPU/memory/disk/network charts
            (Linux in-process metrics).
          </p>
        </header>

        {settingsQ.isLoading && <div className="text-sm text-muted">Loading settings…</div>}
        {settingsQ.isError && (
          <div className="border border-danger bg-danger/10 p-4 text-sm text-danger">
            {settingsQ.error instanceof Error ? settingsQ.error.message : "Failed to load settings"}
          </div>
        )}

        {settingsQ.data && (
          <>
            <div
              role="tabpanel"
              id={`settings-panel-${active}`}
              aria-labelledby={`settings-tab-${active}`}
              className="min-w-0"
            >
            {active === "account" && <AccountSection settings={settingsQ.data} />}
            {active === "system" && <SystemSection settings={settingsQ.data} />}
            {active === "deploy" && <DeploySection settings={settingsQ.data} />}
            {active === "caddy" && <CaddySection settings={settingsQ.data} />}
            {active === "github-app" && <GitHubAppSection />}
            {active === "webhooks" && <WebhooksSection settings={settingsQ.data} />}
            {active === "dns" && <DnsSection settings={settingsQ.data} />}
            {active === "preferences" && <PreferencesSection />}
            {active === "about" && <AboutSection settings={settingsQ.data} />}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
