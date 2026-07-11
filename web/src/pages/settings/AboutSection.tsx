import type { HostForgeSettings } from "../../api/settings";
import { Panel } from "../../components/Panel";
import { SettingsRow } from "./SettingsRow";

const REPO = "https://github.com/hostforge/hostforge";
const README = `${REPO}/blob/main/README.md`;

type Props = {
  settings: HostForgeSettings;
};

export function AboutSection({ settings }: Props) {
  const b = settings.build;
  return (
    <Panel title="About HostForge">
      <p className="border-b border-border px-4 py-3 text-sm text-muted">
        HostForge is a self-hosted control plane for git-triggered Docker deployments, Caddy routing, and operator-facing
        DNS guidance. Sensitive configuration stays in environment variables on the server — this UI only reflects what
        is already running.
      </p>
      <SettingsRow label="Version" value={b.version_display} mono />
      <SettingsRow label="Uptime (seconds)" value={String(b.uptime_seconds)} />
      <div className="space-y-2 border-t border-border px-4 py-3 text-sm">
        <a className="text-primary underline underline-offset-2" href={README} target="_blank" rel="noreferrer">
          README (GitHub)
        </a>
        <div>
          <a className="text-primary underline underline-offset-2" href={REPO} target="_blank" rel="noreferrer">
            Source repository
          </a>
        </div>
      </div>
      <p className="px-4 py-3 text-xs text-muted">
        Out of scope today: editing env vars from the UI, multi-user RBAC, and live Caddyfile editing — see task backlog
        in the repo.
      </p>
    </Panel>
  );
}
