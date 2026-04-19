import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  ApiDeployment,
  ApiProject,
  deleteProject,
  deployProject,
  fetchProject,
  fetchProjectDeployments,
  restartProject,
  rollbackProject,
  stopProject,
} from "../api";
import { projectAccessLinks } from "../accessUrls";
import { useProjectBreadcrumb } from "../ProjectBreadcrumbContext";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { useToast } from "../components/ToastProvider";
import { EmptyState } from "../components/EmptyState";
import { Panel } from "../components/Panel";
import { StatusPill } from "../components/StatusPill";
import { formatDuration, formatRelative, shortHash } from "../format";

export function ProjectPage() {
  const toast = useToast();
  const { registerProject } = useProjectBreadcrumb();
  const { projectID = "" } = useParams();
  const navigate = useNavigate();
  const [project, setProject] = useState<ApiProject | null>(null);
  const [deployments, setDeployments] = useState<ApiDeployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionBusy, setActionBusy] = useState("");
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const [projectData, deploymentData] = await Promise.all([
        fetchProject(projectID),
        fetchProjectDeployments(projectID),
      ]);
      setProject(projectData);
      setDeployments(deploymentData);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load project");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (!projectID) return;
    void load();
  }, [projectID]);

  useEffect(() => {
    if (project && project.id === projectID) {
      registerProject(project.id, project.name);
    }
  }, [project, projectID, registerProject]);

  async function confirmDeleteProject() {
    setDeleteBusy(true);
    setError("");
    try {
      await deleteProject(projectID);
      const name = project?.name || "Project";
      setDeleteDialogOpen(false);
      toast.success(`Deleted project "${name}".`);
      navigate("/projects", { replace: true });
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed.";
      toast.error(msg);
    } finally {
      setDeleteBusy(false);
    }
  }

  async function runControl(label: string, fn: () => Promise<void>) {
    setActionBusy(label);
    setError("");
    try {
      await fn();
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : `${label} failed`);
    } finally {
      setActionBusy("");
    }
  }

  const containerStatus = project?.current_container?.status || "UNKNOWN";
  const latest = project?.latest_deployment;
  const accessLinks = projectAccessLinks(project);
  const domainSummary =
    (project?.domains || []).length === 0
      ? "none configured"
      : (project?.domains || []).map((d) => d.domain_name).join(", ");

  return (
    <div className="flex flex-col gap-6">
      <ConfirmDialog
        open={deleteDialogOpen}
        title="Delete project"
        description={
          project ? (
            <>
              <span className="font-semibold text-text">{`"${project.name}"`}</span> will be removed permanently. This stops
              and removes Docker containers, deletes all deployments and domain records, and cannot be undone.
            </>
          ) : (
            "This action cannot be undone."
          )
        }
        confirmLabel="Delete"
        cancelLabel="Cancel"
        confirmVariant="danger"
        typeConfirm={
          project
            ? {
                prompt: "Type the project name exactly to enable Delete",
                expected: project.name.trim() || projectID,
              }
            : undefined
        }
        onClose={() => {
          if (!deleteBusy) setDeleteDialogOpen(false);
        }}
        onConfirm={confirmDeleteProject}
      />

      <header className="border border-border bg-surface">
        <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border p-4">
          <div className="min-w-0">
            <div className="mono text-[11px] font-semibold uppercase tracking-[0.2em] text-muted">Project</div>
            <h1 className="text-2xl font-semibold tracking-tight">{project?.name || "—"}</h1>
            <a
              href={project?.repo_url}
              target="_blank"
              rel="noreferrer"
              className="mono mt-1 block break-all text-xs text-muted hover:text-text"
            >
              {project?.repo_url || ""}
            </a>
          </div>
          <StatusPill status={latest?.status || "UNKNOWN"} />
        </div>
        <dl className="grid grid-cols-2 gap-px bg-border md:grid-cols-3">
          <Stat label="Branch" value={project?.branch || "main"} />
          <Stat label="Container" value={containerStatus} />
          <Stat label="Last deploy" value={formatRelative(latest?.created_at)} />
        </dl>
        <div className="border-t border-border bg-surface px-4 py-3">
          <div className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Access</div>
          {accessLinks.length > 0 ? (
            <ul className="mt-2 flex flex-col gap-2">
              {accessLinks.map((link) => (
                <li key={`${link.kind}-${link.href}`}>
                  <a
                    href={link.href}
                    target="_blank"
                    rel="noreferrer"
                    className="mono break-all text-sm text-info hover:underline"
                  >
                    {link.label}
                  </a>
                  {link.kind === "direct" && (
                    <span className="ml-2 text-xs text-muted">(Docker publish on loopback)</span>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-2 text-sm text-muted">
              No public hostname or loopback port yet. After a successful deploy you will see a{" "}
              <span className="mono text-text">http://127.0.0.1:…</span> link here; add domains in the data plane to
              show HTTPS URLs.
            </p>
          )}
          <p className="mt-2 text-xs text-muted">
            Domains (Caddy): <span className="mono text-text">{domainSummary}</span>
          </p>
        </div>
      </header>

      <Panel title="Controls">
        <div className="flex flex-wrap gap-2">
          <Button
            variant="primary"
            disabled={!!actionBusy}
            onClick={() => runControl("deploy", async () => void (await deployProject(projectID)))}
          >
            {actionBusy === "deploy" ? "Deploying…" : "Redeploy"}
          </Button>
          <Button
            variant="secondary"
            disabled={!!actionBusy}
            onClick={() => runControl("restart", () => restartProject(projectID))}
          >
            {actionBusy === "restart" ? "Restarting…" : "Restart"}
          </Button>
          <Button
            variant="secondary"
            disabled={!!actionBusy}
            onClick={() => runControl("rollback", () => rollbackProject(projectID))}
          >
            {actionBusy === "rollback" ? "Rolling back…" : "Rollback"}
          </Button>
          <Button
            variant="danger"
            disabled={!!actionBusy}
            onClick={() => runControl("stop", () => stopProject(projectID))}
          >
            {actionBusy === "stop" ? "Stopping…" : "Stop"}
          </Button>
        </div>
        <p className="mt-3 text-xs text-muted">
          Redeploy triggers a fresh build. Rollback re-points Caddy to the previous successful deploy. Stop halts the active container without removing it. The service URL is always under{" "}
          <span className="mono text-text">Access</span> above (restart does not change the loopback port).
        </p>
      </Panel>

      {error && <div className="border border-danger p-3 text-sm text-danger">{error}</div>}

      <Panel title="Deployment History" noBody>
        {loading && deployments.length === 0 ? (
          <div className="p-6 text-sm text-muted">Loading…</div>
        ) : deployments.length === 0 ? (
          <div className="p-4">
            <EmptyState
              title="No deployments yet"
              description="Trigger Redeploy above to build and run the project for the first time."
            />
          </div>
        ) : (
          <table className="w-full table-fixed text-sm">
            <thead>
              <tr className="mono border-b border-border text-left text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                <th className="px-4 py-2 w-[28%]">Deployment</th>
                <th className="px-4 py-2 w-[18%]">Commit</th>
                <th className="px-4 py-2 w-[16%]">Status</th>
                <th className="px-4 py-2 w-[18%]">Started</th>
                <th className="px-4 py-2 w-[20%]">Duration</th>
              </tr>
            </thead>
            <tbody>
              {deployments.map((deployment) => (
                <tr
                  key={deployment.id}
                  className="border-b border-border/60 cursor-pointer hover:bg-surface-alt"
                  onClick={() => navigate(`/projects/${projectID}/deployments/${deployment.id}`)}
                >
                  <td className="px-4 py-3">
                    <div className="mono text-xs text-text">{shortHash(deployment.id, 12)}</div>
                    {deployment.error_message && (
                      <div className="mt-1 text-xs text-danger">{deployment.error_message}</div>
                    )}
                  </td>
                  <td className="px-4 py-3 mono text-xs text-text">{shortHash(deployment.commit_hash, 7)}</td>
                  <td className="px-4 py-3"><StatusPill status={deployment.status} size="sm" /></td>
                  <td className="px-4 py-3 text-xs text-muted">{formatRelative(deployment.created_at)}</td>
                  <td className="px-4 py-3 mono text-xs text-text">
                    {formatDuration(deployment.created_at, deployment.updated_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel title="Danger Zone" tone="danger">
        <p className="text-sm text-muted">
          Use <span className="mono text-text">Stop</span> above to halt traffic without removing the project. Deleting a project removes all deployments and domain rows and tears down Docker containers.
        </p>
        <div className="mt-4">
          <Button
            variant="danger"
            disabled={deleteBusy || !!actionBusy || loading || !project}
            onClick={() => setDeleteDialogOpen(true)}
            type="button"
          >
            Delete project
          </Button>
        </div>
      </Panel>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-surface px-4 py-3">
      <dt className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">{label}</dt>
      <dd className="mt-1 truncate text-sm text-text">{value}</dd>
    </div>
  );
}
