import { useQueryClient } from "@tanstack/react-query";
import { FormEvent, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ApiDeployment,
  createProject,
  deployProject,
  fetchDeploymentLogs,
  fetchProjectDeployments,
  fetchRepositoryBranches,
} from "../api";
import { Button, ButtonLink } from "../components/Button";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { Stepper } from "../components/Stepper";
import { Terminal } from "../components/Terminal";
import { fleetKeys } from "../hooks/fleetQueries";

const STEPS = [
  { id: "source", label: "Source" },
  { id: "deploying", label: "Deploying" },
  { id: "result", label: "Result" },
];

type Phase = "form" | "deploying" | "success" | "failure";

export function NewProjectPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [repoURL, setRepoURL] = useState("");
  const [branch, setBranch] = useState("main");
  const [projectName, setProjectName] = useState("");
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

  const stepIndex = phase === "form" ? 0 : phase === "deploying" ? 1 : 2;
  const failedIndex = phase === "failure" ? stepIndex : undefined;

  function suggestName(url: string) {
    const trimmed = url.trim().replace(/\/$/, "");
    const piece = trimmed.split("/").pop() || "project";
    return piece.replace(/\.git$/, "");
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setMessage("");
    setErrorMessage("");
    try {
      const project = await createProject({
        repo_url: repoURL.trim(),
        branch: branch.trim(),
        project_name: projectName.trim(),
      });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.projects });
      setProjectID(project.id);
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
            void queryClient.invalidateQueries({ queryKey: fleetKeys.projects });
            void queryClient.invalidateQueries({ queryKey: ["deployments", "list"] });
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
    const timer = window.setTimeout(async () => {
      try {
        const result = await fetchRepositoryBranches(trimmedRepo);
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
  }, [repoURL, branchTouched]);

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <header>
        <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">New Project</div>
        <h1 className="text-2xl font-semibold tracking-tight">Deploy from GitHub</h1>
        <p className="mt-1 text-sm text-muted">
          Step-driven flow with immediate state transitions and live logs. Env-var configuration is intentionally deferred in this phase.
        </p>
      </header>

      <Stepper steps={STEPS} currentIndex={stepIndex} failedIndex={failedIndex} />

      {phase === "form" && (
        <Panel title="Step 1 · Choose Source">
          <form className="flex flex-col gap-4" onSubmit={onSubmit}>
            <Field label="Repo URL" required>
              <input
                className="mono w-full border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
                value={repoURL}
                onChange={(e) => {
                  const next = e.target.value;
                  setRepoURL(next);
                  setBranchTouched(false);
                  if (!projectName) {
                    setProjectName(suggestName(next));
                  }
                }}
                placeholder="https://github.com/user/repo"
                required
              />
            </Field>
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
                  onChange={(e) => setProjectName(e.target.value)}
                  placeholder="my-app"
                />
              </Field>
            </div>
            <div className="flex items-center justify-between border-t border-border pt-4">
              <div className="text-xs text-muted">
                Env vars and advanced configuration land in Phase 7.
              </div>
              <Button variant="primary" type="submit" disabled={submitting}>
                {submitting ? "Deploying…" : "Continue and Deploy"}
              </Button>
            </div>
          </form>
        </Panel>
      )}

      {phase !== "form" && (
        <>
          <Panel
            title={phase === "deploying" ? "Step 2 · Deploying" : "Step 3 · Result"}
            actions={
              <StatusPill
                status={
                  phase === "success"
                    ? "SUCCESS"
                    : phase === "failure"
                    ? "FAILED"
                    : (deployment?.status || "BUILDING")
                }
              />
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
                  <Button variant="ghost" size="sm" onClick={() => setPhase("form")}>
                    Try again
                  </Button>
                )}
              </div>
            </div>
          </Panel>

          {deploymentID ? (
            <InlineDeploymentLogs deploymentID={deploymentID} />
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
};

function InlineDeploymentLogs({ deploymentID }: { deploymentID: string }) {
  const [lines, setLines] = useState("");
  const [paused, setPaused] = useState(false);
  const [streamState, setStreamState] = useState("connecting");
  const [copied, setCopied] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(false);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setStreamState("loading tail");
        const tail = await fetchDeploymentLogs(deploymentID, "build");
        if (!cancelled) {
          setLines(tail);
        }
      } catch {
        // tail failure is non-fatal; the live stream below will start filling in
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [deploymentID]);

  useEffect(() => {
    wsRef.current?.close();
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(
      `${protocol}://${window.location.host}/api/deployments/${deploymentID}/logs/live?source=build`,
    );
    wsRef.current = ws;
    setStreamState("connecting");
    ws.onopen = () => setStreamState("live");
    ws.onerror = () => setStreamState("error");
    ws.onclose = () => setStreamState("ended");
    ws.onmessage = (event) => {
      if (!pausedRef.current) {
        setLines((prev) => `${prev}${event.data}`);
      }
    };
    return () => ws.close();
  }, [deploymentID]);

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

  return (
    <Panel
      title="Build Logs"
      actions={
        <span className="mono inline-flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wider">
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
