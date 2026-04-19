import type { HostForgeSettings } from "../../api/settings";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { StatusPill } from "../../components/StatusPill";
import { useSystemStatusQuery } from "../../hooks/fleetQueries";
import { useSettingsMutations } from "../../hooks/settingsQueries";
import { RELEASE_LABEL } from "../../uiVersion";
import { CopyValueButton, SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

function formatBytes(n: number): string {
  if (n < 0) return "—";
  if (n < 1024) return `${n} B`;
  const kb = n / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KiB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MiB`;
  return `${(mb / 1024).toFixed(2)} GiB`;
}

export function SystemSection({ settings }: Props) {
  const { build, paths, network } = settings;
  const statusQ = useSystemStatusQuery();
  const { refreshStatus, detectIPv4 } = useSettingsMutations();
  const checks =
    refreshStatus.isSuccess && refreshStatus.data?.checks?.length
      ? refreshStatus.data.checks
      : (statusQ.data?.checks ?? []);

  return (
    <div className="flex flex-col gap-6">
      <Panel title="Build & runtime">
        <SettingsRow label="UI bundle" value={RELEASE_LABEL} />
        <SettingsRow label="Server version" value={build.version_display} mono />
        <SettingsRow label="Version (semver)" value={build.version} mono />
        <SettingsRow label="Git commit" value={build.commit || "—"} mono />
        <SettingsRow label="Build time" value={build.build_time || "—"} mono />
        <SettingsRow label="Go" value={build.go_version} mono />
        <SettingsRow label="OS / arch" value={`${build.os} / ${build.arch}`} mono />
        <SettingsRow label="PID" value={String(build.pid)} mono />
        <SettingsRow label="Started at" value={build.started_at} mono />
        <SettingsRow label="Uptime (seconds)" value={String(build.uptime_seconds)} />
      </Panel>

      <Panel title="Paths">
        <SettingsRow label="Data directory" value={paths.data_dir} env={paths.data_dir_env} mono>
          <CopyValueButton text={paths.data_dir} />
        </SettingsRow>
        <SettingsRow label="Logs directory" value={paths.logs_dir} env={paths.logs_dir_env} mono>
          <CopyValueButton text={paths.logs_dir} />
        </SettingsRow>
        <SettingsRow label="SQLite database" value={paths.db_path} mono>
          <CopyValueButton text={paths.db_path} />
        </SettingsRow>
        <SettingsRow label="DB size" value={formatBytes(paths.db_size_bytes)} />
        <SettingsRow label="Logs dir (approx size)" value={formatBytes(paths.logs_dir_size_bytes)} />
      </Panel>

      <Panel title="Network">
        <SettingsRow label="Listen" value={network.listen} env={network.listen_env} mono />
        <SettingsRow label="Host port" value={String(network.host_port)} env={network.host_port_env} />
        <SettingsRow
          label="Port range"
          value={`${network.port_start}–${network.port_end}`}
          env={`${network.port_start_env} / ${network.port_end_env}`}
        />
        <SettingsRow label="Container port" value={String(network.container_port)} env={network.container_port_env} />
      </Panel>

      <Panel
        title="Live status"
        actions={
          <Button
            variant="secondary"
            size="sm"
            disabled={refreshStatus.isPending}
            onClick={() => refreshStatus.mutate()}
          >
            {refreshStatus.isPending ? "Refreshing…" : "Refresh now"}
          </Button>
        }
      >
        <div className="space-y-2 px-4 py-3">
          {statusQ.isLoading && <div className="text-sm text-muted">Loading cached status…</div>}
          {statusQ.isError && <div className="text-sm text-danger">Could not load status.</div>}
          <div className="flex flex-col gap-2">
            {checks.map((c) => (
              <div key={c.id} className="flex flex-wrap items-center justify-between gap-2 border border-border bg-surface-alt px-3 py-2">
                <div>
                  <div className="text-sm font-medium text-text">{c.label}</div>
                  {c.detail && <div className="mt-0.5 text-xs text-muted">{c.detail}</div>}
                </div>
                <StatusPill status={c.status} />
              </div>
            ))}
          </div>
          {refreshStatus.isSuccess && (
            <div className="mt-3 text-xs text-muted">Showing fresh status from server (bypasses short TTL cache).</div>
          )}
        </div>
      </Panel>

      <Panel title="Public IPv4 (DNS hints)">
        <SettingsRow label="Detected IPv4" value={settings.dns.detected_ipv4 || "—"} mono />
        <SettingsRow label="Source" value={settings.dns.detected_ipv4_source} mono />
        {settings.dns.detected_ipv4_warning && (
          <SettingsRow label="Warning" value={settings.dns.detected_ipv4_warning} />
        )}
        <div className="border-t border-border px-4 py-3">
          <Button variant="secondary" size="sm" disabled={detectIPv4.isPending} onClick={() => detectIPv4.mutate()}>
            {detectIPv4.isPending ? "Detecting…" : "Re-detect now"}
          </Button>
          {detectIPv4.data && (
            <div className="mt-2 mono text-xs text-text">
              {detectIPv4.data.ipv4} ({detectIPv4.data.source}){detectIPv4.data.warning ? ` — ${detectIPv4.data.warning}` : ""}
            </div>
          )}
        </div>
      </Panel>
    </div>
  );
}
