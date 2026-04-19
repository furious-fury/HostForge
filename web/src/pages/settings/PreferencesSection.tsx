import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import {
  DEFAULT_UI_PREFS,
  type DeploymentsPageSize,
  type LandingPath,
  type ThemePreference,
  useUIPrefs,
} from "../../hooks/useUIPrefs";
import { SettingsRow } from "./SettingsRow";

const LANDING_OPTIONS: { value: LandingPath; label: string }[] = [
  { value: "/", label: "Overview" },
  { value: "/projects", label: "Projects" },
  { value: "/deployments", label: "Deployments" },
];

const PAGE_OPTIONS: DeploymentsPageSize[] = [25, 50, 100, 200];

export function PreferencesSection() {
  const { prefs, setPrefs, resetUIPrefs } = useUIPrefs();

  return (
    <div className="flex flex-col gap-6">
      <Panel title="Appearance">
        <SettingsRow label="Theme">
          <select
            className="mono mt-1 max-w-xs border border-border bg-surface-alt px-3 py-2 text-sm text-text"
            value={prefs.theme}
            onChange={(e) => setPrefs({ theme: e.target.value as ThemePreference })}
          >
            <option value="dark">Dark</option>
            <option value="light">Light</option>
            <option value="system">System</option>
          </select>
        </SettingsRow>
      </Panel>

      <Panel title="Navigation">
        <SettingsRow label="Default landing page">
          <select
            className="mono mt-1 max-w-xs border border-border bg-surface-alt px-3 py-2 text-sm text-text"
            value={prefs.defaultLanding}
            onChange={(e) => setPrefs({ defaultLanding: e.target.value as LandingPath })}
          >
            {LANDING_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label} ({o.value})
              </option>
            ))}
          </select>
          <p className="mt-2 text-xs text-muted">Applies to “/” and unknown paths after login.</p>
        </SettingsRow>
      </Panel>

      <Panel title="Deployments list">
        <SettingsRow label="Default page size / load-more step">
          <select
            className="mono mt-1 max-w-xs border border-border bg-surface-alt px-3 py-2 text-sm text-text"
            value={prefs.deploymentsPageSize}
            onChange={(e) => setPrefs({ deploymentsPageSize: Number(e.target.value) as DeploymentsPageSize })}
          >
            {PAGE_OPTIONS.map((n) => (
              <option key={n} value={n}>
                {n} rows
              </option>
            ))}
          </select>
        </SettingsRow>
      </Panel>

      <Panel title="Logs">
        <SettingsRow label="Live logs start with auto-scroll">
          <label className="mt-1 flex items-center gap-2 text-sm text-text">
            <input
              type="checkbox"
              checked={prefs.logAutoScroll}
              onChange={(e) => setPrefs({ logAutoScroll: e.target.checked })}
            />
            Auto-scroll new lines until paused
          </label>
        </SettingsRow>
      </Panel>

      <Panel title="Locale">
        <SettingsRow label="Numbers & dates">
          <select
            className="mono mt-1 max-w-xs border border-border bg-surface-alt px-3 py-2 text-sm text-text"
            value={prefs.numericLocale}
            onChange={(e) => setPrefs({ numericLocale: e.target.value as "en-US" | "system" })}
          >
            <option value="en-US">en-US</option>
            <option value="system">System (browser)</option>
          </select>
        </SettingsRow>
      </Panel>

      <Panel title="Reset">
        <div className="px-4 py-3">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              if (window.confirm("Reset all UI preferences to defaults?")) {
                resetUIPrefs();
              }
            }}
          >
            Reset preferences to defaults
          </Button>
          <p className="mt-2 text-xs text-muted">
            Defaults: theme={DEFAULT_UI_PREFS.theme}, landing={DEFAULT_UI_PREFS.defaultLanding}, page size=
            {DEFAULT_UI_PREFS.deploymentsPageSize}.
          </p>
        </div>
      </Panel>
    </div>
  );
}
