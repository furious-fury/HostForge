import type { HostForgeSettings } from "../../api/settings";
import {
  hostDiskIO,
  hostDiskMounts,
  hostLoadAvg,
  hostMem,
  hostNetIfaces,
  hostPerCorePct,
  type HostDiskIOSample,
  type HostSample,
} from "../../api/host";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { Sparkline } from "../../components/Sparkline";
import { StatusPill } from "../../components/StatusPill";
import { formatBitsPerSec, formatBytes, formatPct } from "../../format/bytes";
import { useHostHistory, useHostSnapshot } from "../../hooks/hostQueries";
import { useSystemStatusQuery } from "../../hooks/fleetQueries";
import { useFormatLocale } from "../../hooks/useUIPrefs";
import { useSettingsMutations } from "../../hooks/settingsQueries";
import { RELEASE_LABEL } from "../../uiVersion";
import { CopyValueButton, SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

function formatPathBytes(n: number, locale: string): string {
  if (n < 0) return "—";
  return formatBytes(n, locale);
}

function formatUptime(sec: number, locale: string): string {
  if (!Number.isFinite(sec) || sec < 0) return "—";
  const s = Math.floor(sec % 60);
  const m = Math.floor((sec / 60) % 60);
  const h = Math.floor((sec / 3600) % 24);
  const d = Math.floor(sec / 86400);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d.toLocaleString(locale)}d`);
  if (h > 0 || d > 0) parts.push(`${h}h`);
  parts.push(`${m}m`);
  parts.push(`${s}s`);
  return parts.join(" ");
}

function topDiskIO(io: HostDiskIOSample[], n: number): HostDiskIOSample[] {
  if (n <= 0) return [];
  return [...io]
    .sort((a, b) => b.read_bps + b.write_bps - (a.read_bps + a.write_bps))
    .slice(0, n);
}

function totalNetBps(s: HostSample): number {
  let t = 0;
  for (const x of hostNetIfaces(s)) t += x.rx_bps + x.tx_bps;
  return t;
}

export function SystemSection({ settings }: Props) {
  const { build, paths, network } = settings;
  const fmtLocale = useFormatLocale();
  const statusQ = useSystemStatusQuery();
  const hostSnapQ = useHostSnapshot();
  const hostHistQ = useHostHistory(180);
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
        <SettingsRow label="DB size" value={formatPathBytes(paths.db_size_bytes, fmtLocale)} />
        <SettingsRow label="Logs dir (approx size)" value={formatPathBytes(paths.logs_dir_size_bytes, fmtLocale)} />
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

      <HostMetricsPanel fmtLocale={fmtLocale} hostSnapQ={hostSnapQ} hostHistQ={hostHistQ} />

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

type HostPanelProps = {
  fmtLocale: string;
  hostSnapQ: ReturnType<typeof useHostSnapshot>;
  hostHistQ: ReturnType<typeof useHostHistory>;
};

function HostMetricsPanel({ fmtLocale, hostSnapQ, hostHistQ }: HostPanelProps) {
  const snap = hostSnapQ.data;
  const hist = hostHistQ.data?.samples ?? [];
  const histTail = hist.length > 90 ? hist.slice(-90) : hist;

  if (snap?.supported === false) {
    return (
      <Panel title="Host metrics">
        <p className="px-4 py-3 text-sm text-muted">
          Host metrics are only available on Linux (this server reports <span className="mono">{snap.error_code || "unsupported"}</span>).
        </p>
      </Panel>
    );
  }

  if (snap?.warming) {
    return (
      <Panel title="Host metrics">
        <p className="px-4 py-3 text-sm text-muted">Collecting first samples… rates appear after the second tick.</p>
      </Panel>
    );
  }

  const sample = snap?.sample;
  if (!sample && !hostSnapQ.isPending) {
    return (
      <Panel title="Host metrics">
        <p className="px-4 py-3 text-sm text-muted">Could not load host metrics.</p>
      </Panel>
    );
  }
  if (!sample) {
    return (
      <Panel title="Host metrics">
        <p className="px-4 py-3 text-sm text-muted">Loading host metrics…</p>
      </Panel>
    );
  }

  const topIO = topDiskIO(hostDiskIO(sample), 6);

  return (
    <Panel title="Host metrics">
      <div className="space-y-6 px-4 py-3">
        {!sample.rates_ready && <p className="text-xs text-muted">Rates warming up (need consecutive samples).</p>}
        {sample.err ? <p className="text-xs text-danger">{sample.err}</p> : null}

        <div>
          <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">CPU</div>
          <div className="mb-2 text-sm text-text">
            Aggregate <span className="font-semibold tabular-nums">{formatPct(sample.cpu_pct, fmtLocale)}</span>
            <span className="text-muted"> · load </span>
            <span className="mono tabular-nums text-xs">
              {hostLoadAvg(sample)
                .map((x) => x.toFixed(2))
                .join(" / ")}
            </span>
          </div>
          <Sparkline values={histTail.map((s) => s.cpu_pct)} width={320} height={28} className="max-w-full" />
          <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {hostPerCorePct(sample).map((pct, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="mono w-10 shrink-0 text-[10px] text-muted">cpu{i}</span>
                <div className="h-2 min-w-0 flex-1 rounded-sm bg-surface-alt">
                  <div
                    className="h-2 rounded-sm bg-primary"
                    style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
                    title={`${pct.toFixed(1)}%`}
                  />
                </div>
                <span className="mono w-12 shrink-0 text-right text-[11px] tabular-nums text-text">{pct.toFixed(0)}%</span>
              </div>
            ))}
          </div>
        </div>

        <div>
          <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Memory</div>
          <div className="grid gap-2 text-sm sm:grid-cols-2">
            <div>
              <span className="text-muted">Used </span>
              <span className="font-medium tabular-nums">{formatBytes(hostMem(sample).used_bytes, fmtLocale)}</span>
              <span className="text-muted"> ({formatPct(hostMem(sample).used_pct, fmtLocale)})</span>
            </div>
            <div>
              <span className="text-muted">Available </span>
              <span className="tabular-nums">{formatBytes(hostMem(sample).available_bytes, fmtLocale)}</span>
            </div>
            <div>
              <span className="text-muted">Buffers + cache </span>
              <span className="tabular-nums">{formatBytes(hostMem(sample).buffers_cached_bytes ?? 0, fmtLocale)}</span>
            </div>
            <div>
              <span className="text-muted">Swap </span>
              <span className="tabular-nums">
                {formatBytes(hostMem(sample).swap_used_bytes, fmtLocale)} / {formatBytes(hostMem(sample).swap_total_bytes, fmtLocale)}
              </span>
            </div>
          </div>
          <div className="mt-2">
            <Sparkline
              values={histTail.map((s) => hostMem(s).used_pct)}
              width={320}
              height={28}
              className="max-w-full"
              strokeClassName="stroke-info"
            />
          </div>
        </div>

        <div>
          <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Disk usage</div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[480px] text-left text-sm">
              <thead>
                <tr className="mono border-b border-border text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
                  <th className="py-2 pr-3">Mount</th>
                  <th className="py-2 pr-3">FS</th>
                  <th className="py-2 pr-3">Used</th>
                  <th className="py-2 pr-3">Avail</th>
                  <th className="py-2">%</th>
                </tr>
              </thead>
              <tbody>
                {hostDiskMounts(sample).map((d) => (
                  <tr key={d.mount} className="border-b border-border/60">
                    <td className="mono py-2 pr-3 text-xs text-text">{d.mount}</td>
                    <td className="py-2 pr-3 text-xs text-muted">{d.fs_type}</td>
                    <td className="py-2 pr-3 tabular-nums text-xs">{formatBytes(d.used_bytes, fmtLocale)}</td>
                    <td className="py-2 pr-3 tabular-nums text-xs">{formatBytes(d.avail_bytes, fmtLocale)}</td>
                    <td className="py-2 tabular-nums text-xs">{formatPct(d.used_pct, fmtLocale)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div>
          <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Network</div>
          <div className="grid gap-4 sm:grid-cols-2">
            {hostNetIfaces(sample).map((n) => (
              <div key={n.iface} className="rounded border border-border bg-surface-alt px-3 py-2">
                <div className="mono text-xs font-semibold text-text">{n.iface}</div>
                <div className="mt-1 text-[11px] text-muted">
                  RX {formatBitsPerSec(n.rx_bps, fmtLocale)} · TX {formatBitsPerSec(n.tx_bps, fmtLocale)}
                </div>
                <Sparkline
                  values={histTail.map((s) => {
                    const row = hostNetIfaces(s).find((x) => x.iface === n.iface);
                    return row ? row.rx_bps + row.tx_bps : 0;
                  })}
                  height={22}
                  width={200}
                  className="mt-1"
                  strokeClassName="stroke-success"
                />
              </div>
            ))}
          </div>
          <div className="mt-2 text-xs text-muted">
            Total throughput: <span className="tabular-nums text-text">{formatBitsPerSec(totalNetBps(sample), fmtLocale)}</span>
          </div>
          <Sparkline values={histTail.map((s) => totalNetBps(s))} width={320} height={28} className="mt-1 max-w-full" strokeClassName="stroke-success" />
        </div>

        <div>
          <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Disk I/O (top devices)</div>
          {topIO.length === 0 ? (
            <p className="text-xs text-muted">No block device stats.</p>
          ) : (
            <ul className="space-y-2 text-sm">
              {topIO.map((d) => (
                <li key={d.device} className="flex flex-wrap items-baseline justify-between gap-2 border border-border bg-surface-alt px-3 py-2">
                  <span className="mono text-xs font-semibold">{d.device}</span>
                  <span className="text-[11px] text-muted">
                    R {formatBitsPerSec(d.read_bps, fmtLocale)} · W {formatBitsPerSec(d.write_bps, fmtLocale)} · busy{" "}
                    {formatPct(d.busy_pct, fmtLocale, 0)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex flex-wrap gap-6 border-t border-border pt-3 text-sm">
          <div>
            <span className="text-muted">Kernel uptime </span>
            <span className="mono tabular-nums text-text">{formatUptime(sample.uptime_seconds, fmtLocale)}</span>
          </div>
          <div>
            <span className="text-muted">Sample at </span>
            <span className="mono text-xs text-text">{sample.at}</span>
          </div>
        </div>
      </div>
    </Panel>
  );
}
