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

type TabId = "account" | "github-app" | "webhooks" | "dns" | "caddy" | "deploy" | "system" | "preferences" | "about";
type SettingsGroup = { label: string; items: { id: TabId; label: string }[] };
const GROUPS: SettingsGroup[] = [
  { label: "Access and GitHub", items: [{ id: "account", label: "Account" }, { id: "github-app", label: "GitHub App" }, { id: "webhooks", label: "Webhooks" }] },
  { label: "Domains and TLS", items: [{ id: "dns", label: "DNS" }, { id: "caddy", label: "Caddy" }] },
  { label: "Deploy defaults", items: [{ id: "deploy", label: "Build and health" }] },
  { label: "Advanced and system", items: [{ id: "system", label: "System" }, { id: "preferences", label: "Preferences" }, { id: "about", label: "About" }] },
];
const ALL_TABS = GROUPS.flatMap((group) => group.items);
function isTabId(value: string | null): value is TabId { return ALL_TABS.some((tab) => tab.id === value); }
export function SettingsPage() {
  const [params, setParams] = useSearchParams();
  const tabParam = params.get("tab");
  const active: TabId = isTabId(tabParam) ? tabParam : "account";
  const settingsQ = useSettingsQuery();
  const setTab = (id: TabId) => { const next = new URLSearchParams(params); next.set("tab", id); setParams(next, { replace: true }); };

  return <div className="mx-auto flex max-w-[1400px] flex-col gap-6 lg:flex-row lg:items-start">
    <aside className="w-full shrink-0 lg:w-64">
      <Panel title="Settings">
        <nav className="space-y-4 py-2" aria-label="Settings sections">
          {GROUPS.map((group) => <div key={group.label} className="space-y-1">
            <p className="mono px-3 pb-1 text-[10px] uppercase tracking-[0.08em] text-muted">{group.label}</p>
            {group.items.map((item) => <button key={item.id} type="button" onClick={() => setTab(item.id)} className={`block w-full rounded-md px-3 py-2.5 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary ${active === item.id ? "bg-primary font-medium text-primary-ink shadow-sm" : "text-muted hover:bg-surface-alt hover:text-text"}`}>{item.label}</button>)}
          </div>)}
        </nav>
      </Panel>
    </aside>
    <main className="min-w-0 flex-1 space-y-4">
      <header><p className="mono text-[10px] uppercase tracking-[0.08em] text-muted">Configuration</p><h1 className="font-display mt-1 text-2xl font-semibold tracking-tight">Settings</h1><p className="mt-2 max-w-2xl text-sm text-muted">Configure access, domains, deployment defaults, and host-level behavior without exposing server secrets.</p></header>
      {settingsQ.isLoading && <div className="text-sm text-muted">Loading settings...</div>}
      {settingsQ.isError && <div className="rounded-[10px] border border-danger bg-danger/10 p-4 text-sm text-danger">{settingsQ.error instanceof Error ? settingsQ.error.message : "Failed to load settings"}</div>}
      {settingsQ.data && <div>{active === "account" && <AccountSection settings={settingsQ.data} />}{active === "system" && <SystemSection settings={settingsQ.data} />}{active === "deploy" && <DeploySection settings={settingsQ.data} />}{active === "caddy" && <CaddySection settings={settingsQ.data} />}{active === "github-app" && <GitHubAppSection />}{active === "webhooks" && <WebhooksSection settings={settingsQ.data} />}{active === "dns" && <DnsSection settings={settingsQ.data} />}{active === "preferences" && <PreferencesSection />}{active === "about" && <AboutSection settings={settingsQ.data} />}</div>}
    </main>
  </div>;
}