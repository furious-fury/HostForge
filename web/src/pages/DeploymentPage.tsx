import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ApiDeployment, ApiProject, fetchDeploymentLogs, fetchProject, fetchProjectDeployments } from "../api";
import { useProjectBreadcrumb } from "../ProjectBreadcrumbContext";
import { Button, ButtonLink } from "../components/Button";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { Terminal } from "../components/Terminal";
import { formatDuration, formatRelative, shortHash } from "../format";

type SourceKind = "build" | "container";

const STREAM_LABEL: Record<string, string> = {
  connecting: "CONNECTING",
  live: "LIVE",
  ended: "ENDED",
  error: "ERROR",
  "loading tail": "LOADING",
};

export function DeploymentPage() {
  const { projectID = "", deploymentID = "" } = useParams();
  const { registerProject } = useProjectBreadcrumb();
  const [project, setProject] = useState<ApiProject | null>(null);
  const [deployments, setDeployments] = useState<ApiDeployment[]>([]);
  const [source, setSource] = useState<SourceKind>("build");
  const [lines, setLines] = useState("");
  const [paused, setPaused] = useState(false);
  const [streamState, setStreamState] = useState("connecting");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(false);

  const deployment = useMemo(
    () => deployments.find((d) => d.id === deploymentID) || null,
    [deployments, deploymentID],
  );

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

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
  }, [deploymentID, source]);

  useEffect(() => {
    wsRef.current?.close();
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(
      `${protocol}://${window.location.host}/api/deployments/${deploymentID}/logs/live?source=${source}`,
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
  }, [deploymentID, source]);

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
          <Stat label="Started" value={formatRelative(deployment?.created_at)} />
          <Stat label="Duration" value={formatDuration(deployment?.created_at, deployment?.updated_at)} mono />
        </dl>
      </header>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}

      <Panel
        title="Live Logs"
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
              <SourceTab active={source === "build"} onClick={() => setSource("build")}>Build</SourceTab>
              <SourceTab active={source === "container"} onClick={() => setSource("container")}>Runtime</SourceTab>
              <span className="mx-1 h-4 w-px bg-border" />
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
