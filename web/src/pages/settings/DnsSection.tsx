import type { HostForgeSettings } from "../../api/settings";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { useSettingsMutations } from "../../hooks/settingsQueries";
import { CopyValueButton, SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

export function DnsSection({ settings }: Props) {
  const d = settings.dns;
  const { detectIPv4 } = useSettingsMutations();

  return (
    <Panel title="DNS & public IP detection">
      <SettingsRow label="Fixed IPv4 (override)" value={d.server_ipv4 || "—"} env={d.server_ipv4_env} mono />
      <SettingsRow label="Fixed IPv6 (override)" value={d.server_ipv6 || "—"} env={d.server_ipv6_env} mono />
      <SettingsRow label="IPv4 detect URL" value={d.detect_url} env={d.detect_url_env} mono>
        <CopyValueButton text={d.detect_url} />
      </SettingsRow>
      <SettingsRow label="IPv6 detect URL" value={d.detect_ipv6_url} env={d.detect_ipv6_url_env} mono>
        <CopyValueButton text={d.detect_ipv6_url} />
      </SettingsRow>
      <SettingsRow label="Detect timeout (ms)" value={String(d.detect_timeout_ms)} env={d.detect_timeout_ms_env} />
      <SettingsRow label="Current detected IPv4" value={d.detected_ipv4 || "—"} mono />
      <SettingsRow label="Source" value={d.detected_ipv4_source} mono />
      {d.detected_ipv4_warning ? <SettingsRow label="Warning" value={d.detected_ipv4_warning} /> : null}
      <div className="border-t border-border px-4 py-3">
        <Button variant="secondary" size="sm" disabled={detectIPv4.isPending} onClick={() => detectIPv4.mutate()}>
          {detectIPv4.isPending ? "Detecting…" : "Re-detect public IPv4"}
        </Button>
        {detectIPv4.data && (
          <div className="mt-2 mono text-xs text-text">
            {detectIPv4.data.ipv4} ({detectIPv4.data.source})
            {detectIPv4.data.warning ? ` — ${detectIPv4.data.warning}` : ""}
          </div>
        )}
      </div>
    </Panel>
  );
}
