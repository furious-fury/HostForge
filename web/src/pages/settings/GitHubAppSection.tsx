import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ApiGitHubApp,
  ApiGitHubInstallation,
  createGitHubAppManifest,
  deleteGitHubApp,
  exchangeGitHubAppManifest,
  fetchGitHubApp,
  fetchGitHubInstallations,
  syncGitHubInstallations,
} from "../../api";
import { Button } from "../../components/Button";
import { Panel } from "../../components/Panel";
import { useToast } from "../../components/ToastProvider";
import { useConfirm } from "../../components/useConfirm";

type Props = {
  /** Present so this section can be added to the existing settings rail without tripping type checks. */
  onStateChange?: () => void;
};

export function GitHubAppSection({ onStateChange }: Props) {
  const toast = useToast();
  const confirm = useConfirm();
  const [app, setApp] = useState<ApiGitHubApp | null>(null);
  const [appError, setAppError] = useState("");
  const [appLoading, setAppLoading] = useState(true);
  const [installs, setInstalls] = useState<ApiGitHubInstallation[]>([]);
  const [installsError, setInstallsError] = useState("");
  const [installsLoading, setInstallsLoading] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [connectBusy, setConnectBusy] = useState(false);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [appName, setAppName] = useState("HostForge");
  const [organization, setOrganization] = useState("");
  const [webhookURL, setWebhookURL] = useState("");

  const baseURL = useMemo(() => {
    if (typeof window === "undefined") return "";
    return `${window.location.protocol}//${window.location.host}`;
  }, []);

  const baseIsLocal = useMemo(() => isLocalURL(baseURL), [baseURL]);

  const refreshApp = useCallback(async () => {
    try {
      const a = await fetchGitHubApp();
      setApp(a);
      setAppError("");
    } catch (err) {
      setApp({ configured: false });
      setAppError(err instanceof Error ? err.message : "failed to load github app");
    }
  }, []);

  const refreshInstalls = useCallback(async () => {
    try {
      setInstallsLoading(true);
      const list = await fetchGitHubInstallations();
      setInstalls(list);
      setInstallsError("");
    } catch (err) {
      setInstalls([]);
      setInstallsError(err instanceof Error ? err.message : "failed to load installations");
    } finally {
      setInstallsLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await refreshApp();
      } finally {
        if (!cancelled) setAppLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [refreshApp]);

  useEffect(() => {
    if (app?.configured) {
      void refreshInstalls();
    } else {
      setInstalls([]);
    }
  }, [app?.configured, refreshInstalls]);

  // Handle manifest callback: if the URL includes ?code=...&state=hf-... (from GitHub),
  // redeem it for App credentials and clean the URL bar.
  const exchangingRef = useRef(false);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const state = params.get("state");
    if (!code || !state || !state.startsWith("hf-")) return;
    if (exchangingRef.current) return;
    exchangingRef.current = true;
    (async () => {
      try {
        const result = await exchangeGitHubAppManifest(code);
        setApp(result.app);
        toast.success("GitHub App connected. Redirecting to install…");
        onStateChange?.();

        // Strip code/state before redirecting so we don't replay on back-nav.
        const clean = new URL(window.location.href);
        clean.searchParams.delete("code");
        clean.searchParams.delete("state");
        window.history.replaceState(null, "", clean.pathname + clean.search + clean.hash);

        if (result.install_url) {
          window.location.href = result.install_url;
          return;
        }
        void refreshInstalls();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "manifest exchange failed");
        const clean = new URL(window.location.href);
        clean.searchParams.delete("code");
        clean.searchParams.delete("state");
        window.history.replaceState(null, "", clean.pathname + clean.search + clean.hash);
      } finally {
        exchangingRef.current = false;
      }
    })();
  }, [onStateChange, refreshInstalls, toast]);

  // Handle installation callback: GitHub redirects here with ?installation_id=...
  // after the user finishes installing the App. Trigger a sync to import the new
  // installation, then clean the URL.
  const installSyncRef = useRef(false);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const installationId = params.get("installation_id");
    if (!installationId) return;
    if (installSyncRef.current) return;
    installSyncRef.current = true;
    (async () => {
      try {
        const list = await syncGitHubInstallations();
        setInstalls(list);
        toast.success(`Installation synced — ${list.length} installation${list.length === 1 ? "" : "s"} found.`);
      } catch {
        void refreshInstalls();
      } finally {
        const clean = new URL(window.location.href);
        clean.searchParams.delete("installation_id");
        clean.searchParams.delete("setup_action");
        window.history.replaceState(null, "", clean.pathname + clean.search + clean.hash);
        installSyncRef.current = false;
      }
    })();
  }, [refreshInstalls, toast]);

  async function connect() {
    const hook = webhookURL.trim();
    if (baseIsLocal && !hook) {
      toast.error(
        "GitHub can't reach a localhost webhook. Paste a public webhook URL (e.g. an ngrok HTTPS URL) before connecting.",
      );
      return;
    }
    if (hook && isLocalURL(hook)) {
      toast.error("Webhook URL must be reachable over the public Internet (no localhost / private IPs).");
      return;
    }
    setConnectBusy(true);
    try {
      const payload = await createGitHubAppManifest({
        name: appName.trim() || "HostForge",
        url: baseURL,
        organization: organization.trim() || undefined,
        callback_url: `${baseURL}/settings?tab=github-app`,
        webhook_url: hook || undefined,
      });
      submitManifestForm(payload.post_url, payload.manifest);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "manifest request failed");
    } finally {
      setConnectBusy(false);
    }
  }

  async function removeApp() {
    const ok = await confirm({
      title: "Remove GitHub App",
      description: "Remove the GitHub App configuration from this server? Projects using it will fall back to PAT or SSH deploy key authentication.",
      confirmLabel: "Remove App",
      confirmVariant: "danger",
      dangerBanner: (
        <div>
          <p className="text-base font-bold leading-tight text-danger">Warning</p>
          <p className="mt-1.5 text-sm leading-snug text-danger">
            This will delete the stored App credentials (private key, client secret, webhook secret). The App
            registration on GitHub is <span className="font-semibold">not</span> deleted — you can re-connect it
            later or remove it from GitHub separately.
          </p>
        </div>
      ),
    });
    if (!ok) return;
    setDeleteBusy(true);
    try {
      await deleteGitHubApp();
      await refreshApp();
      setInstalls([]);
      onStateChange?.();
      toast.success("GitHub App configuration removed.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "could not remove github app");
    } finally {
      setDeleteBusy(false);
    }
  }

  async function sync() {
    setSyncing(true);
    try {
      const list = await syncGitHubInstallations();
      setInstalls(list);
      toast.success(`Synced ${list.length} installation${list.length === 1 ? "" : "s"}.`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "sync failed");
    } finally {
      setSyncing(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Panel title="GitHub App">
        <div className="space-y-3 px-4 py-3">
          <p className="text-sm text-muted">
            Connect a single GitHub App so HostForge can clone and pull private repositories without you pasting a
            Personal Access Token per project. The App private key and webhook secret are sealed with{" "}
            <span className="mono">HOSTFORGE_ENV_ENCRYPTION_KEY</span>.
          </p>

          {appLoading ? (
            <div className="text-sm text-muted">Loading App configuration…</div>
          ) : appError ? (
            <div className="border border-danger/50 bg-danger/5 px-3 py-2 text-xs text-danger">{appError}</div>
          ) : app?.configured ? (
            <div className="flex flex-col gap-2 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <span className="mono rounded-sm border border-border bg-surface-alt px-2 py-0.5 text-[11px] uppercase tracking-wider text-success">
                  Connected
                </span>
                <span className="font-medium text-text">{app.slug || `App #${app.app_id}`}</span>
                {app.html_url && (
                  <a
                    href={app.html_url}
                    className="mono text-xs text-primary hover:underline"
                    target="_blank"
                    rel="noreferrer"
                  >
                    View on GitHub ↗
                  </a>
                )}
              </div>
              <div className="mono grid gap-x-4 gap-y-1 text-[11px] text-muted sm:grid-cols-2">
                <span>
                  App ID <span className="text-text">{app.app_id}</span>
                </span>
                {app.client_id && (
                  <span>
                    Client ID <span className="text-text">{app.client_id}</span>
                  </span>
                )}
                {app.updated_at && (
                  <span>
                    Updated <span className="text-text">{app.updated_at}</span>
                  </span>
                )}
              </div>
              <div className="mt-2 flex flex-wrap gap-2">
                {app.slug && (
                  <a
                    className="inline-flex items-center rounded-sm border border-border bg-surface-alt px-3 py-1.5 text-sm hover:bg-surface"
                    href={`https://github.com/apps/${app.slug}/installations/new`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Install on another account ↗
                  </a>
                )}
                <Button variant="danger" size="sm" onClick={() => void removeApp()} disabled={deleteBusy}>
                  {deleteBusy ? "Removing…" : "Remove App"}
                </Button>
              </div>
            </div>
          ) : (
            <ConnectForm
              busy={connectBusy}
              appName={appName}
              setAppName={setAppName}
              organization={organization}
              setOrganization={setOrganization}
              webhookURL={webhookURL}
              setWebhookURL={setWebhookURL}
              baseIsLocal={baseIsLocal}
              onConnect={connect}
              baseURL={baseURL}
            />
          )}
        </div>
      </Panel>

      {app?.configured && (
        <Panel
          title="Installations"
          actions={
            <Button variant="secondary" size="sm" onClick={() => void sync()} disabled={syncing || installsLoading}>
              {syncing ? "Syncing…" : "Sync from GitHub"}
            </Button>
          }
        >
          <div className="px-4 py-3">
            {installsError && (
              <div className="mb-3 border border-danger/50 bg-danger/5 px-3 py-2 text-xs text-danger">
                {installsError}
              </div>
            )}
            {installsLoading ? (
              <div className="text-sm text-muted">Loading installations…</div>
            ) : installs.length === 0 ? (
              <div className="text-sm text-muted">
                No installations yet. Use the{" "}
                <span className="font-medium text-text">Install on another account</span> link above to install the App
                on a user or organization account, then click <span className="font-medium text-text">Sync from GitHub</span>.
              </div>
            ) : (
              <ul className="divide-y divide-border border border-border">
                {installs.map((i) => (
                  <li key={i.installation_id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
                    <div className="flex flex-col">
                      <span className="text-sm font-medium text-text">
                        {i.account_login}
                        <span className="mono ml-2 text-[10px] uppercase tracking-wider text-muted">
                          {i.account_type || i.target_type || "account"}
                        </span>
                        {i.suspended && (
                          <span className="mono ml-2 rounded-sm border border-danger/60 bg-danger/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-danger">
                            Suspended
                          </span>
                        )}
                      </span>
                      <span className="mono text-[11px] text-muted">
                        id {i.installation_id} · {i.repo_selection || "selected"} repos
                        {i.last_synced_at ? ` · synced ${i.last_synced_at}` : ""}
                      </span>
                    </div>
                    {app.slug && (
                      <a
                        href={`https://github.com/settings/installations/${i.installation_id}`}
                        target="_blank"
                        rel="noreferrer"
                        className="mono text-xs text-primary hover:underline"
                      >
                        Manage ↗
                      </a>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </Panel>
      )}
    </div>
  );
}

function ConnectForm({
  busy,
  appName,
  setAppName,
  organization,
  setOrganization,
  webhookURL,
  setWebhookURL,
  baseIsLocal,
  onConnect,
  baseURL,
}: {
  busy: boolean;
  appName: string;
  setAppName: (v: string) => void;
  organization: string;
  setOrganization: (v: string) => void;
  webhookURL: string;
  setWebhookURL: (v: string) => void;
  baseIsLocal: boolean;
  onConnect: () => void;
  baseURL: string;
}) {
  return (
    <div className="flex flex-col gap-3 rounded border border-border bg-surface-alt/40 p-4">
      <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
        Create a new GitHub App for this server
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1.5">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">App name</span>
          <input
            className="w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
            value={appName}
            onChange={(e) => setAppName(e.target.value)}
            placeholder="HostForge"
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
            Organization (optional)
          </span>
          <input
            className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
            value={organization}
            onChange={(e) => setOrganization(e.target.value)}
            placeholder="my-org"
          />
          <span className="mono text-[10px] text-muted">
            Leave empty to register the App on your personal account.
          </span>
        </label>
      </div>
      <label className="flex flex-col gap-1.5">
        <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
          Public webhook URL {baseIsLocal ? "(required)" : "(optional)"}
        </span>
        <input
          className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
          value={webhookURL}
          onChange={(e) => setWebhookURL(e.target.value)}
          placeholder="https://your-tunnel.ngrok.app/hooks/github"
        />
        <span className="mono text-[10px] text-muted">
          GitHub rejects manifests with a <span className="text-text">localhost</span> / private-IP webhook. For local
          dev use a tunnel (ngrok, cloudflared, tailscale funnel) and paste the public URL here. On a public server you
          can leave this empty to use{" "}
          <span className="text-text">{baseURL}/hooks/github</span>.
        </span>
      </label>
      {baseIsLocal && (
        <div className="border border-warning/50 bg-warning/5 px-3 py-2 text-xs text-warning">
          This server's URL (<span className="mono">{baseURL}</span>) looks local, so GitHub won't be able to deliver
          webhooks there. Paste a public webhook URL above (an ngrok tunnel is the quickest path) before connecting.
        </div>
      )}
      <p className="text-xs text-muted">
        Clicking <span className="font-medium text-text">Connect GitHub App</span> takes you to GitHub with a prefilled
        manifest. After you confirm, GitHub redirects back to{" "}
        <span className="mono">{baseURL}/settings?tab=github-app</span> with a one-time code; HostForge exchanges it,
        seals the private key, and lists your installations.
      </p>
      <div>
        <Button variant="primary" size="sm" onClick={onConnect} disabled={busy}>
          {busy ? "Preparing manifest…" : "Connect GitHub App"}
        </Button>
      </div>
    </div>
  );
}

// isLocalURL returns true for URLs GitHub's manifest validator would reject
// (loopback / private IPs, localhost). Kept in sync with the server-side
// isPublicWebhookURL check in cmd/server/api_github_app.go.
function isLocalURL(raw: string): boolean {
  const trimmed = raw.trim();
  if (!trimmed) return false;
  let u: URL;
  try {
    u = new URL(trimmed);
  } catch {
    return false;
  }
  const host = u.hostname.toLowerCase();
  if (!host) return false;
  if (host === "localhost" || host.endsWith(".localhost")) return true;
  if (host === "0.0.0.0" || host === "::" || host === "[::]") return true;
  if (/^127\.\d+\.\d+\.\d+$/.test(host)) return true;
  if (/^10\.\d+\.\d+\.\d+$/.test(host)) return true;
  if (/^192\.168\.\d+\.\d+$/.test(host)) return true;
  const m = host.match(/^172\.(\d+)\.\d+\.\d+$/);
  if (m) {
    const second = Number(m[1]);
    if (second >= 16 && second <= 31) return true;
  }
  if (/^169\.254\./.test(host)) return true;
  if (host.startsWith("[fe80:") || host === "[::1]") return true;
  return false;
}

// submitManifestForm builds a hidden HTML form and POSTs it to GitHub with the manifest JSON.
// This is the flow GitHub expects for `/settings/apps/new` — a standard HTML form POST, not AJAX.
function submitManifestForm(postURL: string, manifest: Record<string, unknown>) {
  const form = document.createElement("form");
  form.method = "POST";
  form.action = postURL;
  form.style.display = "none";
  const field = document.createElement("input");
  field.type = "hidden";
  field.name = "manifest";
  field.value = JSON.stringify(manifest);
  form.appendChild(field);
  document.body.appendChild(form);
  form.submit();
}
