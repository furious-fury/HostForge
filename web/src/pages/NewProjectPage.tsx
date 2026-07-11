import { useQueryClient } from "@tanstack/react-query";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ApiDeployment,
  ApiGitHubApp,
  ApiGitHubInstallation,
  ApiGitHubRepo,
  createProject,
  deployProject,
  fetchDeploymentLogs,
  fetchGitHubApp,
  fetchGitHubInstallations,
  fetchInstallationRepositories,
  fetchProjectDeployments,
  fetchRepositoryBranches,
  syncGitHubInstallations,
} from "../api";
import { Button, ButtonLink } from "../components/Button";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { Stepper } from "../components/Stepper";
import { EnvVarsEditor, type EnvDraftPair } from "../components/EnvVarsEditor";
import { Terminal } from "../components/Terminal";
import { invalidateFleetProjectsAndDeployments } from "../hooks/mutationCache";
import { useDeploymentLogStream } from "../hooks/useDeploymentLogStream";
import { useUIPrefs } from "../hooks/useUIPrefs";

const STEPS = [
  { id: "source", label: "Source" },
  { id: "deploying", label: "Deploying" },
  { id: "result", label: "Result" },
];

type Phase = "form" | "deploying" | "success" | "failure";

function formatElapsed(ms: number): string {
  const s = Math.floor(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  }
  return `${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
}

/** Ticks once per second while `active` so elapsed time updates without tying to log traffic. */
function useElapsedSince(anchorMs: number | null, active: boolean): number {
  const [, setTick] = useState(0);
  useEffect(() => {
    if (!active || anchorMs == null) return;
    const id = window.setInterval(() => setTick((n) => n + 1), 1000);
    return () => window.clearInterval(id);
  }, [active, anchorMs]);
  if (!active || anchorMs == null) return 0;
  return Math.max(0, Date.now() - anchorMs);
}

export function NewProjectPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [repoURL, setRepoURL] = useState("");
  const [branch, setBranch] = useState("main");
  const [projectName, setProjectName] = useState("");
  const [nameTouched, setNameTouched] = useState(false);
  const [phase, setPhase] = useState<Phase>("form");
  const [message, setMessage] = useState("");
  const [deploymentID, setDeploymentID] = useState("");
  const [projectID, setProjectID] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const [deployment, setDeployment] = useState<ApiDeployment | null>(null);
  const [availableBranches, setAvailableBranches] = useState<string[]>([]);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [branchLookupError, setBranchLookupError] = useState("");
  const [branchTouched, setBranchTouched] = useState(false);
  const branchLookupSeq = useRef(0);
  const [deployRuntime, setDeployRuntime] = useState<"auto" | "bun">("auto");
  const [deployInstallCmd, setDeployInstallCmd] = useState("");
  const [deployBuildCmd, setDeployBuildCmd] = useState("");
  const [deployStartCmd, setDeployStartCmd] = useState("");
  const [envDraft, setEnvDraft] = useState<EnvDraftPair[]>([]);
  /** Wall-clock anchor when the user starts this deploy (independent of log chunks). */
  const [deployStartedAtMs, setDeployStartedAtMs] = useState<number | null>(null);

  // Source tabs: GitHub App picker vs. raw URL paste.
  const [sourceTab, setSourceTab] = useState<"app" | "url">("url");
  const [app, setApp] = useState<ApiGitHubApp | null>(null);
  const [installations, setInstallations] = useState<ApiGitHubInstallation[]>([]);
  const [selectedInstallationID, setSelectedInstallationID] = useState<number>(0);
  const [installRepos, setInstallRepos] = useState<ApiGitHubRepo[]>([]);
  const [installLoading, setInstallLoading] = useState(false);
  const [installError, setInstallError] = useState("");
  const [repoSearch, setRepoSearch] = useState("");
  const [selectedRepoFullName, setSelectedRepoFullName] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const cfg = await fetchGitHubApp();
        if (!cancelled) {
          setApp(cfg);
          if (cfg.configured) setSourceTab("app");
        }
      } catch {
        if (!cancelled) setApp({ configured: false });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!app?.configured) return;
    let cancelled = false;
    (async () => {
      try {
        let list = await fetchGitHubInstallations();
        if (list.length === 0) {
          list = await syncGitHubInstallations();
        }
        if (cancelled) return;
        setInstallations(list);
        if (list.length > 0 && selectedInstallationID === 0) {
          setSelectedInstallationID(list[0].installation_id);
        }
      } catch (err) {
        if (!cancelled) {
          setInstallError(err instanceof Error ? err.message : "failed to load installations");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [app?.configured, selectedInstallationID]);

  useEffect(() => {
    if (!selectedInstallationID) {
      setInstallRepos([]);
      return;
    }
    let cancelled = false;
    setInstallLoading(true);
    setInstallError("");
    (async () => {
      try {
        const list = await fetchInstallationRepositories(selectedInstallationID);
        if (cancelled) return;
        setInstallRepos(list);
      } catch (err) {
        if (!cancelled) {
          setInstallRepos([]);
          setInstallError(err instanceof Error ? err.message : "failed to load repositories");
        }
      } finally {
        if (!cancelled) setInstallLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [selectedInstallationID]);

  const filteredRepos = useMemo(() => {
    const q = repoSearch.trim().toLowerCase();
    if (!q) return installRepos;
    return installRepos.filter((r) => r.full_name.toLowerCase().includes(q));
  }, [installRepos, repoSearch]);

  const selectedRepo = useMemo(
    () => installRepos.find((r) => r.full_name === selectedRepoFullName) || null,
    [installRepos, selectedRepoFullName],
  );

  // When the user picks a repo in App mode, seed repoURL, project name, and the default branch
  // and let the shared branch-lookup effect below resolve the full branch list via installation_id.
  useEffect(() => {
    if (sourceTab !== "app" || !selectedRepo) return;
    setRepoURL(selectedRepo.clone_url);
    if (!nameTouched) {
      setProjectName(selectedRepo.name);
    }
    setBranchTouched(false);
    if (selectedRepo.default_branch) {
      setBranch(selectedRepo.default_branch);
    }
  }, [sourceTab, selectedRepo, nameTouched]);

  // After success, use an index past the last step so the stepper shows all steps completed (not “stuck” on Result).
  const stepIndex =
    phase === "form" ? 0 : phase === "deploying" ? 1 : phase === "success" ? STEPS.length : 2;
  const failedIndex = phase === "failure" ? 2 : undefined;

  const resultAnchorRef = useRef<HTMLDivElement>(null);
  const [buildLogsExpanded, setBuildLogsExpanded] = useState(true);

  useEffect(() => {
    if (phase === "deploying") {
      setBuildLogsExpanded(true);
      return;
    }
    if (phase === "success" || phase === "failure") {
      setBuildLogsExpanded(false);
      resultAnchorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [phase]);

  function suggestName(url: string) {
    const trimmed = url.trim().replace(/\/$/, "");
    const piece = trimmed.split("/").pop() || "project";
    return piece.replace(/\.git$/, "");
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (sourceTab === "app" && (!selectedInstallationID || !selectedRepoFullName)) {
      setErrorMessage("Pick an installation and repository before continuing.");
      return;
    }
    if (!repoURL.trim()) {
      setErrorMessage("Repository URL is required.");
      return;
    }
    setSubmitting(true);
    setMessage("");
    setErrorMessage("");
    try {
      const hasDeployOverrides =
        deployRuntime === "bun" ||
        deployInstallCmd.trim() ||
        deployBuildCmd.trim() ||
        deployStartCmd.trim();
      const envPayload = envDraft
        .map((e) => ({ key: e.key.trim(), value: e.value }))
        .filter((e) => e.key !== "" && e.value !== "");
      const useApp = sourceTab === "app" && selectedInstallationID > 0;
      const project = await createProject({
        repo_url: repoURL.trim(),
        branch: branch.trim(),
        project_name: projectName.trim(),
        ...(useApp
          ? { git_source: "github_app", github_installation_id: selectedInstallationID }
          : {}),
        ...(hasDeployOverrides
          ? {
              deploy: {
                runtime: deployRuntime,
                install_cmd: deployInstallCmd.trim(),
                build_cmd: deployBuildCmd.trim(),
                start_cmd: deployStartCmd.trim(),
              },
            }
          : {}),
        ...(envPayload.length > 0 ? { env: envPayload } : {}),
      });
      void invalidateFleetProjectsAndDeployments(queryClient);
      setProjectID(project.id);
      setDeployStartedAtMs(Date.now());
      setPhase("deploying");
      setMessage("Build accepted. Streaming live logs.");
      const dep = await deployProject(project.id, { async: true });
      if (dep.deployment_id) {
        setDeploymentID(dep.deployment_id);
      } else if (dep.status === "success") {
        setPhase("success");
        setMessage("Deployment finished.");
      } else if (dep.status === "failed") {
        setPhase("failure");
        setErrorMessage(dep.error || "deployment failed");
      }
    } catch (err) {
      setPhase("failure");
      setErrorMessage(err instanceof Error ? err.message : "failed to create/deploy project");
    } finally {
      setSubmitting(false);
    }
  }

  // poll deployment status while in the deploying phase so we can transition to success/failure
  useEffect(() => {
    if (phase !== "deploying" || !projectID || !deploymentID) {
      return;
    }
    let cancelled = false;
    const tick = async () => {
      try {
        const list = await fetchProjectDeployments(projectID);
        const found = list.find((d) => d.id === deploymentID) || null;
        if (cancelled) return;
        if (found) {
          setDeployment(found);
          if (found.status === "SUCCESS") {
            void invalidateFleetProjectsAndDeployments(queryClient);
            setPhase("success");
            setMessage("Deployment finished.");
          } else if (found.status === "FAILED") {
            setPhase("failure");
            setErrorMessage(found.error_message || "deployment failed");
          }
        }
      } catch {
        // swallow polling errors; logs/WS surface real failures
      }
    };
    tick();
    const interval = window.setInterval(tick, 2000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [phase, projectID, deploymentID]);

  useEffect(() => {
    const trimmedRepo = repoURL.trim();
    if (!trimmedRepo || !/^https?:\/\//i.test(trimmedRepo)) {
      setAvailableBranches([]);
      setBranchesLoading(false);
      setBranchLookupError("");
      return;
    }
    const seq = ++branchLookupSeq.current;
    setBranchesLoading(true);
    setBranchLookupError("");
    const lookupOptions =
      sourceTab === "app" && selectedInstallationID > 0
        ? { installationID: selectedInstallationID }
        : {};
    const timer = window.setTimeout(async () => {
      try {
        const result = await fetchRepositoryBranches(trimmedRepo, lookupOptions);
        if (branchLookupSeq.current !== seq) return;
        const branches = result.branches || [];
        setAvailableBranches(branches);
        if (!branchTouched) {
          const fallback = result.default_branch || branches[0] || "main";
          setBranch(fallback);
        } else if (branch.trim() !== "" && branches.length > 0 && !branches.includes(branch.trim())) {
          setBranch(result.default_branch || branches[0] || "main");
          setBranchTouched(false);
        }
      } catch {
        if (branchLookupSeq.current !== seq) return;
        setAvailableBranches([]);
        setBranchLookupError("Could not load branches; you can still type one manually.");
      } finally {
        if (branchLookupSeq.current === seq) {
          setBranchesLoading(false);
        }
      }
    }, 400);

    return () => window.clearTimeout(timer);
  }, [repoURL, branchTouched, sourceTab, selectedInstallationID]);

  const buildElapsedMs = useElapsedSince(deployStartedAtMs, phase === "deploying");

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <header>
        <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">New Project</div>
        <h1 className="text-2xl font-semibold tracking-tight">Deploy from GitHub</h1>
        <p className="mt-1 text-sm text-muted">
          Step-driven flow with immediate state transitions and live logs. Optional environment variables are applied at
          container runtime after you deploy.
        </p>
      </header>

      <Stepper steps={STEPS} currentIndex={stepIndex} failedIndex={failedIndex} />

      {phase === "form" && (
        <Panel title="Step 1 · Choose Source">
          <form className="flex flex-col gap-4" onSubmit={onSubmit}>
            <div
              role="tablist"
              aria-label="Source"
              className="flex flex-wrap items-center gap-1 border-b border-border"
            >
              <SourceTab
                id="app"
                label="GitHub App"
                active={sourceTab === "app"}
                disabled={!app?.configured}
                onClick={() => setSourceTab("app")}
                hint={!app?.configured ? "Not connected" : undefined}
              />
              <SourceTab
                id="url"
                label="Public URL or PAT"
                active={sourceTab === "url"}
                onClick={() => setSourceTab("url")}
              />
            </div>

            {sourceTab === "app" ? (
              <AppSourcePicker
                appConfigured={Boolean(app?.configured)}
                installations={installations}
                selectedInstallationID={selectedInstallationID}
                setSelectedInstallationID={(id) => {
                  setSelectedInstallationID(id);
                  setSelectedRepoFullName("");
                  setRepoURL("");
                }}
                repos={filteredRepos}
                repoSearch={repoSearch}
                setRepoSearch={setRepoSearch}
                selectedRepoFullName={selectedRepoFullName}
                setSelectedRepoFullName={(v) => {
                  setSelectedRepoFullName(v);
                }}
                loading={installLoading}
                error={installError}
              />
            ) : (
              <Field label="Repo URL" required>
                <input
                  className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                  value={repoURL}
                  onChange={(e) => {
                    const next = e.target.value;
                    setRepoURL(next);
                    setBranchTouched(false);
                    if (!nameTouched) {
                      setProjectName(suggestName(next));
                    }
                  }}
                  placeholder="https://github.com/user/repo"
                  required={sourceTab === "url"}
                />
                <span className="mono mt-1 text-[10px] uppercase tracking-wider text-muted">
                  Public repositories deploy as-is. For a private repo via PAT or SSH deploy key, open the project page
                  after creation.
                </span>
              </Field>
            )}
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <Field label="Branch">
                {availableBranches.length > 0 ? (
                  <select
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={branch}
                    onChange={(e) => {
                      setBranch(e.target.value);
                      setBranchTouched(true);
                    }}
                  >
                    {availableBranches.map((name) => (
                      <option key={name} value={name}>
                        {name}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={branch}
                    onChange={(e) => {
                      setBranch(e.target.value);
                      setBranchTouched(true);
                    }}
                    placeholder="Auto-detected (main/master/remote default)"
                  />
                )}
                <div className="mono mt-1 text-[10px] uppercase tracking-wider text-muted">
                  {branchesLoading
                    ? "Loading branches..."
                    : branchLookupError
                    ? branchLookupError
                    : availableBranches.length > 0
                    ? `${availableBranches.length} branch${availableBranches.length > 1 ? "es" : ""} detected`
                    : "Enter repo URL to auto-detect branches"}
                </div>
              </Field>
              <Field label="Project name">
                <input
                  className="w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                  value={projectName}
                  onChange={(e) => {
                    setProjectName(e.target.value);
                    setNameTouched(true);
                  }}
                  placeholder="my-app"
                />
              </Field>
            </div>
            <div className="rounded border border-border bg-surface-alt/40 p-4">
              <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                Build runtime (Nixpacks)
              </div>
              <p className="mt-1 text-xs text-muted">
                Choose <span className="font-medium text-text">Bun</span> to pin a Bun-friendly Nixpacks plan (Node 20,
                not Node 18). Leave on <span className="font-medium text-text">Auto</span> for stock Nixpacks detection,
                or add custom install/build/start commands only when needed.
              </p>
              <div className="mt-3 grid gap-3 md:grid-cols-2">
                <Field label="Runtime">
                  <select
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={deployRuntime}
                    onChange={(e) => setDeployRuntime(e.target.value as "auto" | "bun")}
                  >
                    <option value="auto">Auto (Nixpacks default)</option>
                    <option value="bun">Bun (recommended for Bun apps)</option>
                  </select>
                </Field>
                <Field label="Install command (optional)">
                  <input
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={deployInstallCmd}
                    onChange={(e) => setDeployInstallCmd(e.target.value)}
                    placeholder={deployRuntime === "bun" ? "default: bun install" : "e.g. npm ci"}
                  />
                </Field>
                <Field label="Build command (optional)">
                  <input
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={deployBuildCmd}
                    onChange={(e) => setDeployBuildCmd(e.target.value)}
                    placeholder={deployRuntime === "bun" ? "default: bun run build" : "e.g. npm run build"}
                  />
                </Field>
                <Field label="Start command (optional)">
                  <input
                    className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                    value={deployStartCmd}
                    onChange={(e) => setDeployStartCmd(e.target.value)}
                    placeholder={deployRuntime === "bun" ? "default: bun run start" : "e.g. npm run start"}
                  />
                </Field>
              </div>
            </div>
            <details className="rounded border border-border bg-surface-alt/40 p-4">
              <summary className="cursor-pointer mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
                Environment variables (optional)
              </summary>
              <div className="mt-4">
                <EnvVarsEditor mode="local" value={envDraft} onChange={setEnvDraft} />
              </div>
            </details>
            <div className="flex items-center justify-end border-t border-border pt-4">
              <Button variant="primary" type="submit" disabled={submitting}>
                {submitting ? "Deploying…" : "Continue and Deploy"}
              </Button>
            </div>
          </form>
        </Panel>
      )}

      {phase !== "form" && (
        <>
          <div ref={resultAnchorRef} className="scroll-mt-4" />
          <Panel
            title={phase === "deploying" ? "Step 2 · Deploying" : "Step 3 · Result"}
            actions={
              <div className="flex flex-wrap items-center justify-end gap-2">
                {phase === "deploying" && deployStartedAtMs != null && (
                  <span
                    className="mono text-[11px] font-semibold tabular-nums uppercase tracking-wider text-muted"
                    title="Time since this deploy started (updates even when logs are quiet)"
                  >
                    {formatElapsed(buildElapsedMs)}
                  </span>
                )}
                <StatusPill
                  status={
                    phase === "success"
                      ? "SUCCESS"
                      : phase === "failure"
                      ? "FAILED"
                      : (deployment?.status || "BUILDING")
                  }
                />
              </div>
            }
          >
            <div className="flex flex-col gap-3">
              <p className="text-sm text-muted">{message || "Awaiting deployment status…"}</p>
              {errorMessage && (
                <div className="border border-danger bg-danger/10 p-3 text-sm text-danger">{errorMessage}</div>
              )}
              <div className="flex flex-wrap gap-2">
                {projectID && (
                  <Button variant="primary" size="sm" onClick={() => navigate(`/projects/${projectID}`)}>
                    Go to project
                  </Button>
                )}
                {deploymentID && (
                  <ButtonLink
                    to={`/projects/${projectID}/deployments/${deploymentID}`}
                    variant="secondary"
                    size="sm"
                  >
                    Open dedicated deployment view
                  </ButtonLink>
                )}
                {phase === "failure" && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setPhase("form");
                      setDeployStartedAtMs(null);
                    }}
                  >
                    Try again
                  </Button>
                )}
              </div>
            </div>
          </Panel>

          {deploymentID ? (
            <InlineDeploymentLogs
              deploymentID={deploymentID}
              streamActive={phase === "deploying"}
              collapsed={!buildLogsExpanded}
              onExpand={() => setBuildLogsExpanded(true)}
              elapsedMs={phase === "deploying" && deployStartedAtMs != null ? buildElapsedMs : undefined}
            />
          ) : (
            <Panel title="Build Logs" noBody>
              <div className="border-t border-border bg-terminal p-4 font-mono text-xs text-muted">
                Waiting for the build worker to register the deployment…
              </div>
            </Panel>
          )}
        </>
      )}
    </div>
  );
}

const STREAM_LABEL: Record<string, string> = {
  connecting: "CONNECTING",
  live: "LIVE",
  ended: "ENDED",
  error: "ERROR",
  "loading tail": "LOADING",
  reconnecting: "RECONNECTING",
};

function InlineDeploymentLogs({
  deploymentID,
  streamActive,
  collapsed,
  onExpand,
  elapsedMs,
}: {
  deploymentID: string;
  /** While true, the WebSocket will reconnect if the proxy or network drops the connection. */
  streamActive: boolean;
  collapsed?: boolean;
  onExpand?: () => void;
  /** Wall-clock elapsed for this deploy; ticks even when the log stream is idle. */
  elapsedMs?: number;
}) {
  const { prefs } = useUIPrefs();
  const [paused, setPaused] = useState(() => !prefs.logAutoScroll);
  const [copied, setCopied] = useState(false);
  const streamActiveRef = useRef(streamActive);
  streamActiveRef.current = streamActive;

  const fetchTail = useCallback(() => fetchDeploymentLogs(deploymentID, "build"), [deploymentID]);
  const shouldReconnectCb = useCallback(() => streamActiveRef.current, []);

  const { lines, setLines, streamState } = useDeploymentLogStream({
    deploymentId: deploymentID,
    source: "build",
    active: streamActive,
    paused,
    fetchTail,
    shouldReconnect: shouldReconnectCb,
  });

  async function copyAll() {
    try {
      await navigator.clipboard.writeText(lines);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard not available; ignore
    }
  }

  const streamLabel = STREAM_LABEL[streamState] || streamState.toUpperCase();

  if (collapsed) {
    return (
      <Panel title="Build Logs">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted">
            Log stream continues in the background. Expand to read the full build output.
          </p>
          <Button variant="secondary" size="sm" onClick={onExpand}>
            Show build logs
          </Button>
        </div>
      </Panel>
    );
  }

  return (
    <Panel
      title="Build Logs"
      actions={
        <span className="mono inline-flex flex-wrap items-center justify-end gap-x-3 gap-y-1 text-[10px] font-semibold uppercase tracking-wider">
          {elapsedMs !== undefined && (
            <span
              className="tabular-nums text-muted"
              title="Time since this deploy started (updates even when logs are quiet)"
            >
              {formatElapsed(elapsedMs)}
            </span>
          )}
          <span className="inline-flex items-center gap-1">
            <span
              aria-hidden
              className={
                streamState === "live"
                  ? "text-success"
                  : streamState === "error"
                    ? "text-danger"
                    : streamState === "ended"
                      ? "text-muted"
                      : "text-warning"
              }
            >
              ●
            </span>
            <span className="text-muted">Stream</span>
            <span className="text-text">{streamLabel}</span>
          </span>
        </span>
      }
      noBody
    >
      <Terminal
        scrollLocked={paused}
        text={lines}
        toolbar={
          <>
            <Button variant="secondary" size="sm" onClick={() => setPaused((v) => !v)}>
              {paused ? "Resume" : "Pause"}
            </Button>
            <Button variant="secondary" size="sm" onClick={copyAll}>
              {copied ? "Copied" : "Copy"}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setLines("")}>Clear</Button>
            <span className="ml-auto mono text-[10px] uppercase tracking-wider text-muted">
              {paused ? "Scroll locked" : "Auto-scroll"}
            </span>
          </>
        }
      />
    </Panel>
  );
}

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
        {label}{required && <span className="ml-1 text-danger">*</span>}
      </span>
      {children}
    </label>
  );
}

function SourceTab({
  id,
  label,
  active,
  onClick,
  disabled,
  hint,
}: {
  id: string;
  label: string;
  active: boolean;
  onClick: () => void;
  disabled?: boolean;
  hint?: string;
}) {
  return (
    <button
      type="button"
      role="tab"
      id={`source-tab-${id}`}
      aria-selected={active}
      onClick={onClick}
      disabled={disabled}
      className={[
        "relative -mb-px border-b-2 px-3 py-2 text-sm transition-colors",
        active
          ? "border-primary font-semibold text-text"
          : "border-transparent text-muted hover:text-text",
        disabled ? "cursor-not-allowed opacity-60" : "",
      ].join(" ")}
    >
      {label}
      {hint && (
        <span className="mono ml-2 rounded-sm border border-border bg-surface-alt px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-muted">
          {hint}
        </span>
      )}
    </button>
  );
}

function AppSourcePicker({
  appConfigured,
  installations,
  selectedInstallationID,
  setSelectedInstallationID,
  repos,
  repoSearch,
  setRepoSearch,
  selectedRepoFullName,
  setSelectedRepoFullName,
  loading,
  error,
}: {
  appConfigured: boolean;
  installations: ApiGitHubInstallation[];
  selectedInstallationID: number;
  setSelectedInstallationID: (id: number) => void;
  repos: ApiGitHubRepo[];
  repoSearch: string;
  setRepoSearch: (v: string) => void;
  selectedRepoFullName: string;
  setSelectedRepoFullName: (v: string) => void;
  loading: boolean;
  error: string;
}) {
  if (!appConfigured) {
    return (
      <div className="rounded border border-border bg-surface-alt/40 p-4 text-sm">
        <p className="text-muted">
          No GitHub App is connected for this server. Connect one to pick from your installations and deploy private
          repositories without pasting a Personal Access Token.
        </p>
        <ButtonLink to="/settings?tab=github-app" variant="secondary" size="sm" className="mt-3">
          Connect GitHub App
        </ButtonLink>
      </div>
    );
  }
  if (installations.length === 0) {
    return (
      <div className="rounded border border-border bg-surface-alt/40 p-4 text-sm">
        <p className="text-muted">
          The App is connected but has no installations yet. Install it on a user or organization account, then come
          back here.
        </p>
        <ButtonLink to="/settings?tab=github-app" variant="secondary" size="sm" className="mt-3">
          Manage installations
        </ButtonLink>
      </div>
    );
  }
  const selectedRepo = repos.find((r) => r.full_name === selectedRepoFullName) ?? null;

  return (
    <div className="flex flex-col gap-3">
      <Field label="Installation" required>
        <select
          className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
          value={selectedInstallationID || ""}
          onChange={(e) => setSelectedInstallationID(parseInt(e.target.value, 10) || 0)}
        >
          {installations.map((i) => (
            <option key={i.installation_id} value={i.installation_id}>
              {i.account_login} ({i.account_type || i.target_type || "account"})
              {i.suspended ? " · suspended" : ""}
            </option>
          ))}
        </select>
      </Field>

      {selectedRepo ? (
        <div className="flex flex-col gap-1.5">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
            Repository <span className="ml-1 text-danger">*</span>
          </span>
          <button
            type="button"
            onClick={() => {
              setSelectedRepoFullName("");
              setRepoSearch("");
            }}
            className="flex w-full cursor-pointer items-center justify-between gap-3 border border-border bg-surface-alt px-3 py-2 text-left transition-colors hover:bg-surface-alt/60"
          >
            <span className="flex flex-col">
              <span className="text-sm font-medium text-text">{selectedRepo.full_name}</span>
              <span className="mono text-[11px] text-muted">
                {selectedRepo.private ? "private" : "public"} · default {selectedRepo.default_branch || "main"}
              </span>
            </span>
            <span className="mono text-[10px] uppercase tracking-wider text-primary">
              Change
            </span>
          </button>
        </div>
      ) : (
        <>
          <Field label="Repository" required>
            <input
              className="w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={repoSearch}
              onChange={(e) => setRepoSearch(e.target.value)}
              placeholder="Search repositories…"
            />
          </Field>
          {error ? (
            <div className="border border-danger bg-danger/10 p-3 text-sm text-danger">{error}</div>
          ) : loading ? (
            <div className="text-sm text-muted">Loading repositories…</div>
          ) : repos.length === 0 ? (
            <div className="text-sm text-muted">No repositories match this filter for the selected installation.</div>
          ) : (
            <div className="max-h-64 overflow-y-auto border border-border">
              <ul className="divide-y divide-border">
                {repos.map((r) => (
                  <li key={r.id}>
                    <button
                      type="button"
                      onClick={() => setSelectedRepoFullName(r.full_name)}
                      className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-surface-alt/60"
                    >
                      <span className="flex flex-col">
                        <span className="font-medium text-text">{r.full_name}</span>
                        <span className="mono text-[11px] text-muted">
                          {r.private ? "private" : "public"} · default {r.default_branch || "main"}
                        </span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </div>
  );
}
