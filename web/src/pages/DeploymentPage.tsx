import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiDeployment, ApiProject, fetchDeploymentLogs, fetchProject, fetchProjectDeployments } from "../api";
import { DeployStepTimeline } from "../components/DeployStepTimeline";
import { useDeploymentStepsQuery } from "../hooks/observabilityQueries";
import { useProjectBreadcrumb } from "../ProjectBreadcrumbContext";
import { Button, ButtonLink } from "../components/Button";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { Terminal } from "../components/Terminal";
import { formatDuration, formatRelative, shortHash } from "../format";
import { useFormatLocale, useUIPrefs } from "../hooks/useUIPrefs";

type SourceKind = "build" | "container";
type PanelTab = "logs" | "steps";

const STREAM_LABEL: Record<string, string> = {
  connecting: "CONNECTING",
  live: "LIVE",
  ended: "ENDED",
  error: "ERROR",
  "loading tail": "LOADING",
  reconnecting: "RECONNECTING",
};

function deploymentStatusInFlight(status: string | undefined): boolean {
  const u = status?.toUpperCase();
  return u === "QUEUED" || u === "BUILDING";
}

export function DeploymentPage() {
  const { projectID = "", deploymentID = "" } = useParams();
  const { registerProject } = useProjectBreadcrumb();
  const { prefs } = useUIPrefs();
  const fmtLocale = useFormatLocale();
  const [project, setProject] = useState<ApiProject | null>(null);
  const [deployments, setDeployments] = useState<ApiDeployment[]>([]);
  const [source, setSource] = useState<SourceKind>("build");
  const [lines, setLines] = useState("");
  const [paused, setPaused] = useState(() => !prefs.logAutoScroll);
  const [streamState, setStreamState] = useState("connecting");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const [panelTab, setPanelTab] = useState<PanelTab>("logs");
  const stepsQ = useDeploymentStepsQuery(deploymentID, 200);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(false);
  const deploymentRef = useRef<ApiDeployment | null>(null);

  const deployment = useMemo(
    () => deployments.find((d) => d.id === deploymentID) || null,
    [deployments, deploymentID],
  );

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    deploymentRef.current = deployment;
  }, [deployment]);

  useEffect(() => {
    setProject(null);
    setDeployments([]);
    let cancelled = false;
    (async () => {
      try {
        const [proj, deps] = await Promise.all([fetchProject(projectID), fetchProjectDeployments(projectID)]);
        if (!cancelled) {
          setProject(proj);
          setDeployments(deps);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load deployment");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [projectID]);

  useEffect(() => {
    if (project && project.id === projectID) {
      registerProject(project.id, project.name);
    }
  }, [project, projectID, registerProject]);

  useEffect(() => {
    if (panelTab !== "logs") {
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        setStreamState("loading tail");
        const tail = await fetchDeploymentLogs(deploymentID, source);
        if (!cancelled) {
          setLines(tail);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "failed to load logs");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [deploymentID, source, panelTab]);

  useEffect(() => {
    if (panelTab !== "logs") {
      wsRef.current?.close();
      wsRef.current = null;
      return;
    }
    let cancelled = false;
    let reconnectTimer: number | undefined;

    function connect() {
      if (cancelled) return;
      wsRef.current?.close();
      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(
        `${protocol}://${window.location.host}/api/deployments/${deploymentID}/logs/live?source=${source}`,
      );
      wsRef.current = ws;
      setStreamState("connecting");
      ws.onopen = () => {
        if (!cancelled) setStreamState("live");
      };
      ws.onerror = () => {
        if (!cancelled) setStreamState("error");
      };
      ws.onclose = () => {
        if (cancelled) return;
        if (deploymentStatusInFlight(deploymentRef.current?.status)) {
          setStreamState("reconnecting");
          reconnectTimer = window.setTimeout(async () => {
            if (cancelled) return;
            try {
              const deps = await fetchProjectDeployments(projectID);
              if (!cancelled) {
                setDeployments(deps);
              }
            } catch {
              // ignore; connect() still runs so logs resume when server is back
            }
            connect();
          }, 1500);
          return;
        }
        setStreamState("ended");
      };
      ws.onmessage = (event) => {
        if (!pausedRef.current) {
          setLines((prev) => `${prev}${event.data}`);
        }
      };
    }

    connect();

    return () => {
      cancelled = true;
      if (reconnectTimer !== undefined) {
        window.clearTimeout(reconnectTimer);
      }
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [deploymentID, source, panelTab, projectID]);

  async function copyAll() {
    try {
      await navigator.clipboard.writeText(lines);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "copy failed");
    }
  }

  const streamLabel = STREAM_LABEL[streamState] || streamState.toUpperCase();

  return (
    <div className="flex flex-col gap-6">
      <header className="border border-border bg-surface">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border p-4">
          <div className="min-w-0">
            <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Deployment</div>
            {project && (
              <div className="mt-1 text-sm text-text">
                <span className="text-muted">Project </span>
                <Link to={`/projects/${project.id}`} className="font-semibold hover:underline">
                  {project.name}
                </Link>
              </div>
            )}
            <h1 className="mono mt-2 text-lg text-text">{shortHash(deploymentID, 16)}</h1>
            <div className="mono mt-1 break-all text-[11px] text-muted">{deploymentID}</div>
          </div>
          <div className="flex items-center gap-2">
            <StatusPill status={deployment?.status || "UNKNOWN"} />
          </div>
        </div>
        <dl className="grid grid-cols-2 gap-px bg-border md:grid-cols-4">
          <Stat label="Commit" value={shortHash(deployment?.commit_hash || "", 12)} mono />
          <Stat label="Image" value={deployment?.image_ref || "—"} mono />
          <Stat label="Started" value={formatRelative(deployment?.created_at, new Date(), fmtLocale)} />
          <Stat label="Duration" value={formatDuration(deployment?.created_at, deployment?.updated_at)} mono />
        </dl>
      </header>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}

      <Panel
        title={panelTab === "logs" ? "Live Logs" : "Deploy steps"}
        actions={
          panelTab === "logs" ? (
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
          ) : (
            <span className="mono text-[10px] text-muted">SQLite samples · same as Observability page</span>
          )
        }
        noBody
      >
        <div className="border-b border-border bg-surface-alt px-3 py-2">
          <div className="flex flex-wrap gap-2">
            <SourceTab active={panelTab === "logs"} onClick={() => setPanelTab("logs")}>
              Logs
            </SourceTab>
            <SourceTab active={panelTab === "steps"} onClick={() => setPanelTab("steps")}>
              Steps
            </SourceTab>
          </div>
        </div>
        {panelTab === "logs" ? (
          <Terminal
            scrollLocked={paused}
            text={lines}
            toolbar={
              <>
                <SourceTab active={source === "build"} onClick={() => setSource("build")}>
                  Build
                </SourceTab>
                <SourceTab active={source === "container"} onClick={() => setSource("container")}>
                  Runtime
                </SourceTab>
                <span className="mx-1 h-4 w-px bg-border" />
                <Button variant="secondary" size="sm" onClick={() => setPaused((v) => !v)}>
                  {paused ? "Resume" : "Pause"}
                </Button>
                <Button variant="secondary" size="sm" onClick={copyAll}>
                  {copied ? "Copied" : "Copy"}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setLines("")}>
                  Clear
                </Button>
                <span className="ml-auto mono text-[10px] uppercase tracking-wider text-muted">
                  {paused ? "Scroll locked" : "Auto-scroll"}
                </span>
              </>
            }
          />
        ) : (
          <div className="p-4">
            {stepsQ.isPending ? <p className="text-sm text-muted">Loading steps…</p> : null}
            {stepsQ.isError ? (
              <p className="text-sm text-danger">
                {stepsQ.error instanceof Error ? stepsQ.error.message : "Failed to load steps"}
              </p>
            ) : null}
            {stepsQ.data && stepsQ.data.length > 0 ? (
              <>
                <div className="mb-4">
                  <div className="mono mb-2 text-[10px] font-semibold uppercase tracking-wider text-muted">Timeline</div>
                  <DeployStepTimeline steps={stepsQ.data} />
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse text-left text-sm">
                    <thead>
                      <tr className="border-b border-border text-[10px] uppercase tracking-wider text-muted">
                        <th className="py-2 pr-2">Step</th>
                        <th className="py-2 pr-2">Status</th>
                        <th className="py-2 pr-2">ms</th>
                        <th className="py-2 pr-2">request_id</th>
                        <th className="py-2">error_code</th>
                      </tr>
                    </thead>
                    <tbody>
                      {stepsQ.data.map((s) => (
                        <tr key={s.id} className="border-b border-border/60">
                          <td className="py-2 pr-2 font-mono text-xs">{s.step}</td>
                          <td className="py-2 pr-2">
                            <StatusPill status={s.status === "ok" ? "SUCCESS" : "FAILED"} size="sm" />
                          </td>
                          <td className="py-2 pr-2 mono tabular-nums">{s.duration_ms}</td>
                          <td className="max-w-[10rem] truncate py-2 pr-2 font-mono text-[10px] text-muted">{s.request_id || "—"}</td>
                          <td className="py-2 font-mono text-[10px] text-muted">{s.error_code || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : null}
            {!stepsQ.isPending && stepsQ.data?.length === 0 ? (
              <p className="text-sm text-muted">No recorded steps for this deployment yet.</p>
            ) : null}
          </div>
        )}
      </Panel>

      <div>
        <ButtonLink to={`/projects/${projectID}`} variant="secondary" size="sm">
          ← Back to project
        </ButtonLink>
      </div>
    </div>
  );
}

function SourceTab({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`mono px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider border ${
        active ? "border-primary bg-primary text-primary-ink" : "border-border-strong text-text hover:bg-surface-alt"
      }`}
    >
      {children}
    </button>
  );
}

function Stat({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="bg-surface px-4 py-3">
      <dt className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">{label}</dt>
      <dd className={`mt-1 truncate text-sm text-text ${mono ? "mono" : ""}`}>{value}</dd>
    </div>
  );
}
