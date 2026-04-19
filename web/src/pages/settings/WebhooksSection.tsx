import type { HostForgeSettings } from "../../api/settings";
import { Panel } from "../../components/Panel";
import { StatusPill } from "../../components/StatusPill";
import { useSystemStatusQuery } from "../../hooks/fleetQueries";
import { SecretPill, SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

export function WebhooksSection({ settings }: Props) {
  const w = settings.webhooks;
  const statusQ = useSystemStatusQuery();
  const hookCheck = statusQ.data?.checks.find((c) => c.id === "webhooks");

  return (
    <div className="flex flex-col gap-6">
      <Panel title="Webhook configuration (read-only)">
        <SettingsRow label="Base path" value={w.base_path} env={w.base_path_env} mono />
        <SettingsRow label="Max body (bytes)" value={String(w.max_body_bytes)} env={w.max_body_bytes_env} />
        <SettingsRow label="Async mode" value={w.async ? "true" : "false"} env={w.async_env} />
        <SettingsRow label="Rate limit / minute" value={String(w.rate_limit_per_minute)} env={w.rate_limit_per_minute_env} />
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-3">
          <span className="text-sm text-muted">Shared secret</span>
          <SecretPill set={w.secret_set} />
        </div>
      </Panel>

      <Panel title="Route probe (from system status)">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <div>
            <div className="text-sm font-medium text-text">GET probe on webhook path</div>
            {hookCheck?.detail && <div className="mt-1 text-xs text-muted">{hookCheck.detail}</div>}
          </div>
          {hookCheck ? <StatusPill status={hookCheck.status} /> : <span className="text-sm text-muted">—</span>}
        </div>
      </Panel>
    </div>
  );
}
