import type { HostForgeSettings } from "../../api/settings";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { useSettingsMutations } from "../../hooks/settingsQueries";
import { CopyValueButton, SettingsRow } from "./SettingsRow";

type Props = {
  settings: HostForgeSettings;
};

export function CaddySection({ settings }: Props) {
  const c = settings.caddy;
  const { caddyValidate, caddySync } = useSettingsMutations();

  return (
    <div className="flex flex-col gap-6">
      <Panel title="Caddy configuration (read-only)">
        <SettingsRow label="Caddy binary" value={c.bin} env={c.bin_env} mono />
        <SettingsRow label="Generated snippet path" value={c.generated_path} env={c.generated_path_env} mono>
          <CopyValueButton text={c.generated_path} />
        </SettingsRow>
        <SettingsRow label="Root Caddyfile" value={c.root_config || "—"} env={c.root_config_env} mono>
          {c.root_config ? <CopyValueButton text={c.root_config} /> : null}
        </SettingsRow>
        <SettingsRow label="Sync after deploy" value={c.sync_caddy ? "true" : "false"} env={c.sync_caddy_env} />
        <SettingsRow
          label="Sync after domain mutate"
          value={c.domain_sync_after_mutate ? "true" : "false"}
          env={c.domain_sync_after_mutate_env}
        />
        <SettingsRow
          label="Cert poll interval (sec)"
          value={String(c.cert_poll_interval_sec)}
          env={c.cert_poll_interval_sec_env}
        />
        <SettingsRow label="Caddy admin URL" value={c.admin_url} env={c.admin_url_env} mono>
          <CopyValueButton text={c.admin_url} />
        </SettingsRow>
        <SettingsRow label="Caddy storage root" value={c.storage_root || "—"} env={c.storage_root_env} mono>
          {c.storage_root ? <CopyValueButton text={c.storage_root} /> : null}
        </SettingsRow>
      </Panel>

      <Panel title="Actions">
        <div className="flex flex-col gap-3 px-4 py-3">
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" disabled={caddyValidate.isPending} onClick={() => caddyValidate.mutate()}>
              {caddyValidate.isPending ? "Validating…" : "Run caddy validate"}
            </Button>
            <Button variant="secondary" size="sm" disabled={caddySync.isPending} onClick={() => caddySync.mutate()}>
              {caddySync.isPending ? "Syncing…" : "Sync Caddy now"}
            </Button>
          </div>
          {caddyValidate.isError && (
            <div className="text-xs text-danger">
              {caddyValidate.error instanceof Error ? caddyValidate.error.message : "validate request failed"}
            </div>
          )}
          {caddyValidate.data && (
            <div className="mono text-xs text-text">
              <div className={caddyValidate.data.ok ? "text-success" : "text-danger"}>
                {caddyValidate.data.ok ? "OK" : "Failed"} ({caddyValidate.data.took_ms} ms)
              </div>
              {caddyValidate.data.stdout ? <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap">{caddyValidate.data.stdout}</pre> : null}
              {caddyValidate.data.stderr ? <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap text-warning">{caddyValidate.data.stderr}</pre> : null}
              {caddyValidate.data.error && <div className="mt-2 text-danger">{caddyValidate.data.error}</div>}
            </div>
          )}
          {caddySync.data && (
            <div className="text-xs text-muted">
              Sync: attempted={String(caddySync.data.caddy_sync.attempted)} ok={String(caddySync.data.caddy_sync.ok)}
              {caddySync.data.caddy_sync.error ? ` error=${caddySync.data.caddy_sync.error}` : ""} ({caddySync.data.duration_ms} ms)
            </div>
          )}
        </div>
      </Panel>
    </div>
  );
}
