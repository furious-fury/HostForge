import { useCallback, useState } from "react";
import { deleteSession } from "../../api";
import type { HostForgeSettings } from "../../api/settings";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { CopyValueButton, SecretPill, SettingsRow } from "./SettingsRow";

function randomHex(bytes: number): string {
  const a = new Uint8Array(bytes);
  crypto.getRandomValues(a);
  return Array.from(a, (b) => b.toString(16).padStart(2, "0")).join("");
}

type Props = {
  settings: HostForgeSettings;
};

export function AccountSection({ settings }: Props) {
  const { auth, session } = settings;
  const [tokenGen, setTokenGen] = useState("");
  const [whSecretGen, setWhSecretGen] = useState("");

  const genApiToken = useCallback(() => {
    setTokenGen(randomHex(32));
  }, []);

  const genWebhookSecret = useCallback(() => {
    setWhSecretGen(randomHex(32));
  }, []);

  async function signOut() {
    try {
      await deleteSession();
    } finally {
      window.location.href = "/";
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Panel title="Session">
        <SettingsRow
          label="Auth scheme"
          value={auth.scheme === "bearer" ? "Bearer API token" : auth.scheme === "session" ? "Signed cookie" : auth.scheme}
        />
        {auth.expires_at && (
          <SettingsRow label="Session expires" value={auth.expires_at} mono />
        )}
        {auth.subject && <SettingsRow label="Subject" value={auth.subject} mono />}
        <div className="border-t border-border px-4 py-3">
          <Button variant="secondary" size="sm" onClick={() => void signOut()}>
            Sign out
          </Button>
          <p className="mt-2 text-xs text-muted">
            Ends the browser session. You will need the API token again to sign in.
          </p>
        </div>
      </Panel>

      <Panel title="API token">
        <div className="space-y-3 px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-muted">Management API</span>
            <SecretPill set={session.api_token_set} />
          </div>
          <p className="text-sm text-muted">
            The token value is never shown. To rotate: set <span className="mono">{session.api_token_env}</span> on the
            server, restart HostForge, then sign in again with the new token.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" onClick={genApiToken}>
              Generate new token (copy only)
            </Button>
            {tokenGen && (
              <>
                <span className="mono max-w-full break-all text-xs text-text">{tokenGen}</span>
                <CopyValueButton text={tokenGen} />
              </>
            )}
          </div>
        </div>
      </Panel>

      <Panel title="Session cookie (read-only)">
        <SettingsRow label="Cookie name" value={session.cookie_name} env={session.cookie_name_env} mono />
        <SettingsRow label="TTL (minutes)" value={String(session.ttl_minutes)} env={session.ttl_minutes_env} />
        <SettingsRow label="Secure flag" value={session.cookie_secure ? "true" : "false"} env={session.cookie_secure_env} />
        <div className="flex flex-wrap items-center gap-2 border-t border-border px-4 py-3">
          <span className="text-sm text-muted">Session signing secret</span>
          <SecretPill set={session.session_secret_set} />
        </div>
        <p className="px-4 pb-3 text-sm text-muted">
          Rotating <span className="mono">{session.session_secret_env}</span> invalidates all existing UI sessions.
        </p>
      </Panel>

      <Panel title="Webhook secret (rotation helper)">
        <div className="space-y-3 px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <SecretPill set={settings.webhooks.secret_set} />
          </div>
          <p className="text-sm text-muted">
            Update <span className="mono">{settings.webhooks.secret_env}</span> and the same secret in your GitHub
            repository webhook settings, then restart HostForge.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" size="sm" onClick={genWebhookSecret}>
              Generate webhook secret (copy only)
            </Button>
            {whSecretGen && (
              <>
                <span className="mono max-w-full break-all text-xs text-text">{whSecretGen}</span>
                <CopyValueButton text={whSecretGen} />
              </>
            )}
          </div>
        </div>
      </Panel>
    </div>
  );
}
