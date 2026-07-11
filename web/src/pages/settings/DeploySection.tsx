import type { HostForgeSettings } from "../../api/settings";
import { Panel } from "../../components/Panel";
import { SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

export function DeploySection({ settings }: Props) {
  const h = settings.health;
  return (
    <Panel title="Deploy health checks">
      <p className="border-b border-border px-4 py-3 text-sm text-muted">
        Before marking a deployment healthy, HostForge probes the new container on <span className="mono">{h.path}</span>{" "}
        until responses fall between HTTP {h.expected_min}–{h.expected_max}, retrying with the timeouts below.
      </p>
      <SettingsRow label="Health path" value={h.path} env={h.path_env} mono />
      <SettingsRow label="Timeout (ms)" value={String(h.timeout_ms)} env={h.timeout_ms_env} />
      <SettingsRow label="Retries" value={String(h.retries)} env={h.retries_env} />
      <SettingsRow label="Interval (ms)" value={String(h.interval_ms)} env={h.interval_ms_env} />
      <SettingsRow label="Expected min status" value={String(h.expected_min)} env={h.expected_min_env} />
      <SettingsRow label="Expected max status" value={String(h.expected_max)} env={h.expected_max_env} />
    </Panel>
  );
}
