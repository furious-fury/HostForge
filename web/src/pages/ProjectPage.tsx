import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  ApiDeployment,
  ApiDomain,
  ApiProject,
  createProjectDomain,
  deleteProject,
  deleteProjectDomain,
  deployProject,
  DnsGuidance,
  DnsGuidanceRecord,
  fetchProject,
  fetchProjectDeployments,
  restartProject,
  rollbackProject,
  stopProject,
  updateProjectDeploy,
  updateProjectDomain,
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
import { useFormatLocale } from "../hooks/useUIPrefs";

export function ProjectPage() {
  const fmtLocale = useFormatLocale();
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
  const [domainInput, setDomainInput] = useState("");
  const [domainBusy, setDomainBusy] = useState("");
  const [editDomainId, setEditDomainId] = useState<string | null>(null);
  const [editDomainValue, setEditDomainValue] = useState("");
  const [domainDeleteTarget, setDomainDeleteTarget] = useState<ApiDomain | null>(null);
  const [domainDeleteOpen, setDomainDeleteOpen] = useState(false);
  const [dnsRefreshing, setDnsRefreshing] = useState(false);
  const [deployForm, setDeployForm] = useState({
    runtime: "auto",
    install_cmd: "",
    build_cmd: "",
    start_cmd: "",
  });
  const [deployBusy, setDeployBusy] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const [projectData, deploymentData] = await Promise.all([
        fetchProject(projectID),
        fetchProjectDeployments(projectID),
      ]);
      setProject(projectData);
      setDeployments(deploymentData);
      setDeployForm({
        runtime: projectData.deploy?.runtime || "auto",
        install_cmd: projectData.deploy?.install_cmd || "",
        build_cmd: projectData.deploy?.build_cmd || "",
        start_cmd: projectData.deploy?.start_cmd || "",
      });
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

  const registrarDnsNeedsAttention = useMemo(() => {
    const list = project?.domains;
    if (!list?.length) return false;
    return list.some((d) => (d.registrar_dns_status || "").toLowerCase() !== "ok");
  }, [project?.domains]);

  const refreshRegistrarDns = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!projectID) return;
      const manual = !opts?.silent;
      if (manual) setDnsRefreshing(true);
      try {
        const p = await fetchProject(projectID);
        setProject((prev) => {
          if (prev && prev.id !== projectID) return prev;
          return p;
        });
        if (manual) {
          toast.success("DNS status refreshed.");
        }
      } catch (err) {
        if (manual) {
          toast.error(err instanceof Error ? err.message : "Could not refresh DNS status.");
        }
      } finally {
        if (manual) setDnsRefreshing(false);
      }
    },
    [projectID, toast],
  );

  useEffect(() => {
    if (!projectID || !registrarDnsNeedsAttention) return;
    const id = window.setInterval(() => {
      void refreshRegistrarDns({ silent: true });
    }, 30_000);
    return () => window.clearInterval(id);
  }, [projectID, registrarDnsNeedsAttention, refreshRegistrarDns]);

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
        open={domainDeleteOpen}
        title="Remove domain"
        dangerBanner={
          <p className="text-sm text-danger">
            Removes the hostname from HostForge and updates Caddy when configured. Your registrar DNS is unchanged—
            delete old records there manually if needed.
          </p>
        }
        description={
          domainDeleteTarget ? (
            <>
              Remove <span className="mono font-semibold text-text">{domainDeleteTarget.domain_name}</span> from this
              project?
            </>
          ) : (
            "Remove this domain?"
          )
        }
        confirmLabel="Remove"
        cancelLabel="Cancel"
        confirmVariant="danger"
        onClose={() => setDomainDeleteOpen(false)}
        onConfirm={async () => {
          if (!domainDeleteTarget) return;
          try {
            const out = await deleteProjectDomain(projectID, domainDeleteTarget.id);
            if (out.caddy_sync?.attempted && !out.caddy_sync.ok) {
              toast.error(`Caddy sync failed: ${out.caddy_sync.error || "unknown"}`);
            }
            toast.success(`Removed domain ${domainDeleteTarget.domain_name}.`);
            setDomainDeleteOpen(false);
            setDomainDeleteTarget(null);
            await load();
          } catch (err) {
            toast.error(err instanceof Error ? err.message : "Remove domain failed.");
          }
        }}
      />
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
          <Stat label="Last deploy" value={formatRelative(latest?.created_at, new Date(), fmtLocale)} />
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

      <Panel title="Build & runtime (Nixpacks)">
        <p className="mb-3 text-xs text-muted">
          Settings here are written to a generated <span className="mono text-text">nixpacks.toml</span> in the deploy
          worktree before each build (see README). Use <span className="font-medium text-text">Bun</span> if Nixpacks
          otherwise pulls an EOL Node 18 toolchain for Bun monorepos.
        </p>
        <div className="grid max-w-3xl gap-3 md:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">Runtime</span>
            <select
              className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={deployForm.runtime === "bun" ? "bun" : "auto"}
              onChange={(e) =>
                setDeployForm((f) => ({ ...f, runtime: e.target.value === "bun" ? "bun" : "auto" }))
              }
              disabled={loading || !project}
            >
              <option value="auto">Auto</option>
              <option value="bun">Bun</option>
            </select>
          </label>
          <label className="flex flex-col gap-1.5 md:col-span-2">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
              Install command
            </span>
            <input
              className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={deployForm.install_cmd}
              onChange={(e) => setDeployForm((f) => ({ ...f, install_cmd: e.target.value }))}
              placeholder="optional"
              disabled={loading || !project}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
              Build command
            </span>
            <input
              className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={deployForm.build_cmd}
              onChange={(e) => setDeployForm((f) => ({ ...f, build_cmd: e.target.value }))}
              placeholder="optional"
              disabled={loading || !project}
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
              Start command
            </span>
            <input
              className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={deployForm.start_cmd}
              onChange={(e) => setDeployForm((f) => ({ ...f, start_cmd: e.target.value }))}
              placeholder="optional"
              disabled={loading || !project}
            />
          </label>
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            variant="secondary"
            size="sm"
            disabled={deployBusy || loading || !projectID}
            onClick={async () => {
              setDeployBusy(true);
              try {
                const updated = await updateProjectDeploy(projectID, {
                  runtime: deployForm.runtime,
                  install_cmd: deployForm.install_cmd.trim(),
                  build_cmd: deployForm.build_cmd.trim(),
                  start_cmd: deployForm.start_cmd.trim(),
                });
                setProject(updated);
                setDeployForm({
                  runtime: updated.deploy.runtime || "auto",
                  install_cmd: updated.deploy.install_cmd || "",
                  build_cmd: updated.deploy.build_cmd || "",
                  start_cmd: updated.deploy.start_cmd || "",
                });
                toast.success("Deploy settings saved. Redeploy to apply in the next image build.");
              } catch (err) {
                toast.error(err instanceof Error ? err.message : "Save failed.");
              } finally {
                setDeployBusy(false);
              }
            }}
          >
            {deployBusy ? "Saving…" : "Save deploy settings"}
          </Button>
        </div>
      </Panel>

      <Panel
        title="Domains"
        actions={
          project?.domains?.length ? (
            <Button
              variant="secondary"
              size="sm"
              type="button"
              disabled={dnsRefreshing || loading || !project}
              onClick={() => void refreshRegistrarDns()}
            >
              {dnsRefreshing ? "Checking DNS…" : "Check DNS now"}
            </Button>
          ) : undefined
        }
      >
        <p className="mb-3 text-xs text-muted">
          <span className="font-medium text-text">Caddy route</span> means HostForge applied a reverse-proxy snippet on this
          server (not that browsers can reach the site yet). <span className="font-medium text-text">Registrar DNS</span> is a
          quick public-DNS check against the same IPv4 we suggest below. Add the records at your DNS provider so the hostname
          resolves here; then HTTPS from the internet can succeed.
          {registrarDnsNeedsAttention ? (
            <>
              {" "}
              <span className="text-text">
                While any hostname is not pointed here, HostForge rechecks public DNS about every 30 seconds.
              </span>
            </>
          ) : null}
        </p>
        <div className="flex flex-wrap gap-2 border-b border-border pb-4">
          <input
            type="text"
            value={domainInput}
            onChange={(e) => setDomainInput(e.target.value)}
            placeholder="app.example.com"
            className="mono min-w-[200px] flex-1 border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
            disabled={!!domainBusy || loading || !project}
          />
          <Button
            variant="secondary"
            disabled={!!domainBusy || loading || !project || !domainInput.trim()}
            onClick={async () => {
              if (!projectID) return;
              setDomainBusy("add");
              try {
                const out = await createProjectDomain(projectID, domainInput.trim());
                setDomainInput("");
                if (out.caddy_sync?.attempted && !out.caddy_sync.ok) {
                  toast.error(`Caddy sync failed: ${out.caddy_sync.error || "unknown"}`);
                } else if (out.caddy_sync?.attempted) {
                  toast.success("Domain added; Caddy reloaded.");
                } else {
                  toast.success("Domain added.");
                }
                await load();
              } catch (err) {
                toast.error(err instanceof Error ? err.message : "Add domain failed.");
              } finally {
                setDomainBusy("");
              }
            }}
          >
            {domainBusy === "add" ? "Adding…" : "Add domain"}
          </Button>
        </div>
        {!project?.domains?.length ? (
          <p className="pt-4 text-sm text-muted">No domains yet. Add a hostname above.</p>
        ) : (
          <div className="overflow-x-auto pt-4">
            <table className="w-full text-sm">
              <thead>
                <tr className="mono border-b border-border text-left text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                  <th className="py-2 pr-4">Hostname</th>
                  <th className="py-2 pr-4">Caddy route</th>
                  <th className="py-2 pr-4">Registrar DNS</th>
                  <th className="py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {(project.domains || []).map((d) => (
                  <tr key={d.id} className="border-b border-border/60">
                    <td className="py-3 pr-4">
                      {editDomainId === d.id ? (
                        <input
                          type="text"
                          value={editDomainValue}
                          onChange={(e) => setEditDomainValue(e.target.value)}
                          className="mono w-full border border-border bg-surface-alt px-2 py-1 text-xs text-text focus:border-border-strong focus:outline-none"
                        />
                      ) : (
                        <span className="mono text-xs text-text">{d.domain_name}</span>
                      )}
                    </td>
                    <td className="py-3 pr-4 align-top">
                      <DomainCaddyCell
                        sslStatus={d.ssl_status}
                        lastCertMessage={d.last_cert_message}
                        certCheckedAt={d.cert_checked_at}
                      />
                    </td>
                    <td className="py-3 pr-4 align-top">
                      <DomainRegistrarDnsCell
                        status={d.registrar_dns_status}
                        resolved={d.resolved_ipv4}
                      />
                    </td>
                    <td className="py-3 text-right">
                      {editDomainId === d.id ? (
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="secondary"
                            size="sm"
                            disabled={!!domainBusy}
                            onClick={() => {
                              setEditDomainId(null);
                              setEditDomainValue("");
                            }}
                          >
                            Cancel
                          </Button>
                          <Button
                            variant="primary"
                            size="sm"
                            disabled={!!domainBusy || !editDomainValue.trim()}
                            onClick={async () => {
                              setDomainBusy("edit");
                              try {
                                const out = await updateProjectDomain(projectID, d.id, editDomainValue.trim());
                                if (out.caddy_sync?.attempted && !out.caddy_sync.ok) {
                                  toast.error(`Caddy sync failed: ${out.caddy_sync.error || "unknown"}`);
                                } else {
                                  toast.success("Domain updated.");
                                }
                                setEditDomainId(null);
                                setEditDomainValue("");
                                await load();
                              } catch (err) {
                                toast.error(err instanceof Error ? err.message : "Update failed.");
                              } finally {
                                setDomainBusy("");
                              }
                            }}
                          >
                            {domainBusy === "edit" ? "Saving…" : "Save"}
                          </Button>
                        </div>
                      ) : (
                        <div className="flex justify-end gap-2">
                          <Button
                            variant="secondary"
                            size="sm"
                            disabled={!!domainBusy}
                            onClick={() => {
                              setEditDomainId(d.id);
                              setEditDomainValue(d.domain_name);
                            }}
                          >
                            Edit
                          </Button>
                          <Button
                            variant="danger"
                            size="sm"
                            disabled={!!domainBusy}
                            onClick={() => {
                              setDomainDeleteTarget(d);
                              setDomainDeleteOpen(true);
                            }}
                          >
                            Remove
                          </Button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {project?.domains?.some((d) => (d.registrar_dns_status || "").toLowerCase() === "ok") && (
          <div className="mt-4 border border-border bg-surface-alt p-3 text-xs leading-relaxed text-muted">
            <p className="font-semibold text-text">Site still times out but registrar shows &quot;Points here&quot;?</p>
            <p className="mt-1.5">
              That only means public DNS resolves your hostname to this server&apos;s IP. The browser still needs something
              listening on <span className="mono text-text">0.0.0.0:80</span> and <span className="mono text-text">:443</span>{" "}
              (usually Caddy), with OS and cloud firewalls / security groups allowing inbound HTTP/S. A connection timeout is
              almost never a DNS-panel issue at that point. Also: once the A record targets this VPS, you will{" "}
              <span className="font-medium text-text">not</span> see your previous registrar or parking page—traffic goes to
              this machine, not Hostinger&apos;s shared hosting frontends.
            </p>
          </div>
        )}
        {project?.dns_guidance && (
          <DnsHintsBlock
            guidance={project.dns_guidance}
            defaultExpanded={registrarDnsNeedsAttention}
            onCopied={() => toast.success("Copied to clipboard.")}
            onCopyError={() => toast.error("Could not copy to clipboard.")}
          />
        )}
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
                  <td className="px-4 py-3 text-xs text-muted">{formatRelative(deployment.created_at, new Date(), fmtLocale)}</td>
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

function DomainCaddyCell({
  sslStatus,
  lastCertMessage,
  certCheckedAt,
}: {
  sslStatus: string;
  lastCertMessage?: string;
  certCheckedAt?: string;
}) {
  const u = (sslStatus || "").toUpperCase();
  const certLine =
    (lastCertMessage && lastCertMessage.trim()) || (certCheckedAt && certCheckedAt.trim())
      ? (
          <p className="mt-1 font-mono text-[10px] leading-snug text-muted">
            {lastCertMessage && lastCertMessage.trim() ? <span className="block break-all">{lastCertMessage.trim()}</span> : null}
            {certCheckedAt && certCheckedAt.trim() ? (
              <span className="mt-0.5 block text-[9px] opacity-80">Cert poll: {certCheckedAt.trim()}</span>
            ) : null}
          </p>
        )
      : null;
  if (u === "ACTIVE") {
    return (
      <div className="text-xs">
        <span className="font-medium text-success">Route synced</span>
        <p className="mt-0.5 text-[10px] leading-snug text-muted">
          Caddy on this server has the route. Browsers still need your registrar DNS pointed at this machine before HTTPS works
          from the internet.
        </p>
        {certLine}
      </div>
    );
  }
  if (u === "PENDING") {
    return (
      <div className="text-xs">
        <span className="font-medium text-warning">Route pending</span>
        <p className="mt-0.5 text-[10px] leading-snug text-muted">Deploy or Caddy sync has not marked this host active yet.</p>
        {certLine}
      </div>
    );
  }
  return (
    <div className="text-xs">
      <StatusPill status={sslStatus} size="sm" />
      {certLine}
    </div>
  );
}

function DomainRegistrarDnsCell({ status, resolved }: { status?: string; resolved?: string[] }) {
  const st = (status || "unknown").toLowerCase();
  if (st === "ok") {
    return (
      <div className="text-xs">
        <span className="font-medium text-success">Points here</span>
        <p className="mt-0.5 text-[10px] leading-snug text-muted">
          Public DNS returns the same IPv4 HostForge expects for this server.
        </p>
      </div>
    );
  }
  if (st === "pending") {
    const extra =
      resolved && resolved.length > 0
        ? `Public DNS currently resolves to ${resolved.join(", ")} — not the server IP below.`
        : "No public IPv4 A record yet, or DNS is still propagating.";
    return (
      <div className="text-xs">
        <span className="font-medium text-warning">Not pointed here yet</span>
        <p className="mt-0.5 text-[10px] leading-snug text-muted">{extra}</p>
      </div>
    );
  }
  if (st === "lookup_error") {
    return (
      <div className="text-xs">
        <span className="font-medium text-danger">Lookup failed</span>
        <p className="mt-0.5 text-[10px] leading-snug text-muted">
          This host could not be resolved from the HostForge server (typo, no zone, or resolver error).
        </p>
      </div>
    );
  }
  return (
    <div className="text-xs">
      <span className="font-medium text-muted">Cannot verify</span>
      <p className="mt-0.5 text-[10px] leading-snug text-muted">
        Set HOSTFORGE_DNS_SERVER_IPV4 on the HostForge server so we know which IP registrar DNS should use.
      </p>
    </div>
  );
}

function DnsCopyButton({
  ariaLabel,
  value,
  onCopied,
  onCopyError,
}: {
  ariaLabel: string;
  value: string;
  onCopied: () => void;
  onCopyError: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={ariaLabel}
      title="Copy"
      className="ml-1.5 inline-flex shrink-0 border border-border bg-surface p-1 text-muted hover:border-border-strong hover:text-text"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value);
          onCopied();
        } catch {
          onCopyError();
        }
      }}
    >
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
        <rect x="9" y="9" width="13" height="13" rx="0" />
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
      </svg>
    </button>
  );
}

function visibleDnsRecords(records: DnsGuidanceRecord[] | undefined): DnsGuidanceRecord[] {
  return (records || []).filter((r) => r.value?.trim());
}

function formatDnsGuidanceClipboard(g: DnsGuidance): string {
  const lines: string[] = [];
  lines.push("# Add these at your DNS provider / registrar (Hostinger, Cloudflare, etc.) — not inside HostForge.");
  lines.push("# Most UIs ask for: Record type, Host / Name, Value / Points to / Target.");
  lines.push(`# IPv4 source for this server: ${g.ipv4_source}`);
  if (g.ipv4) {
    lines.push(`# Use this IPv4 in each A record value: ${g.ipv4}`);
  }
  if (g.ipv6) {
    lines.push(`# IPv6 source: ${g.ipv6_source || ""}`);
    lines.push(`# Suggested AAAA value: ${g.ipv6}`);
  }
  if (g.steps?.length) {
    lines.push("# --- Steps (same as the UI list) ---");
    for (const step of g.steps) {
      lines.push(`# ${step}`);
    }
  }
  lines.push("# --- Rows (Type / Host / Value; zone hint in comment) ---");
  for (const r of g.records || []) {
    if (!r.value?.trim()) continue;
    lines.push(`${r.type}\t${r.name}\t${r.value}\t# zone: ${r.zone_hint || ""}`);
    if (r.note) lines.push(`# ${r.note}`);
  }
  if (g.message) {
    lines.push(`# Note: ${g.message}`);
  }
  return lines.join("\n");
}

function DnsHintsBlock({
  guidance,
  defaultExpanded,
  onCopied,
  onCopyError,
}: {
  guidance: DnsGuidance;
  /** When true, section starts open (DNS not fully pointed here yet). */
  defaultExpanded: boolean;
  onCopied: () => void;
  onCopyError: () => void;
}) {
  const text = formatDnsGuidanceClipboard(guidance);
  const rows = visibleDnsRecords(guidance.records);
  const [expanded, setExpanded] = useState(defaultExpanded);

  useEffect(() => {
    setExpanded(defaultExpanded);
  }, [defaultExpanded]);

  return (
    <div className="mt-4 border border-border bg-surface-alt">
      <div className="flex flex-wrap items-stretch gap-2 border-b border-border bg-surface-alt px-3 py-2">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-2 py-1 text-left hover:bg-surface"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          <span className="mono shrink-0 text-[10px] text-muted" aria-hidden>
            {expanded ? "▼" : "▶"}
          </span>
          <div>
            <div className="mono text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">DNS records</div>
            <div className="text-[11px] text-muted">
              {expanded ? "Hide registrar Type / Name / Value details" : `Show setup table (${rows.length} row${rows.length === 1 ? "" : "s"})`}
            </div>
          </div>
        </button>
        <Button
          variant="ghost"
          size="sm"
          type="button"
          className="shrink-0 self-center"
          onClick={async (e) => {
            e.stopPropagation();
            try {
              await navigator.clipboard.writeText(text);
              onCopied();
            } catch {
              onCopyError();
            }
          }}
        >
          Copy all as text
        </Button>
      </div>
      {!expanded ? null : (
        <div className="p-4 pt-3">
      <p className="text-sm leading-relaxed text-muted">
        Add these at your DNS provider or registrar (not in HostForge). The{" "}
        <span className="font-medium text-text">Type</span>, <span className="font-medium text-text">Name</span> (host), and{" "}
        <span className="font-medium text-text">Value</span> must match what your provider&apos;s UI expects—often labeled
        &quot;Points to&quot; or &quot;Target&quot; for the value.
      </p>
      {guidance.message && (
        <p className="mt-3 border border-warning/40 bg-warning/5 p-3 text-xs text-warning">{guidance.message}</p>
      )}
      {guidance.steps && guidance.steps.length > 0 && (
        <ol className="mt-4 list-decimal space-y-2 pl-5 text-xs leading-relaxed text-text">
          {guidance.steps.map((step, i) => (
            <li key={i}>{step}</li>
          ))}
        </ol>
      )}

      {rows.length > 0 ? (
        <div className="mt-4 overflow-x-auto border border-border bg-surface">
          <table className="w-full min-w-[320px] border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-border bg-surface-alt">
                <th className="mono py-2.5 pl-3 pr-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
                  Type
                </th>
                <th className="mono py-2.5 pr-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">Name</th>
                <th className="mono py-2.5 pr-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">Value</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <Fragment key={`${r.type}-${r.name}-${r.value}-${i}`}>
                  <tr className="border-b border-border last:border-b-0">
                    <td className="align-middle py-3 pl-3 pr-2">
                      <div className="flex items-center">
                        <span className="mono font-medium text-text">{r.type || "—"}</span>
                        <DnsCopyButton
                          ariaLabel={`Copy record type ${r.type}`}
                          value={r.type}
                          onCopied={onCopied}
                          onCopyError={onCopyError}
                        />
                      </div>
                    </td>
                    <td className="align-middle py-3 pr-2">
                      <div className="flex items-center break-all">
                        <span className="mono text-text">{r.name || "@"}</span>
                        <DnsCopyButton
                          ariaLabel={`Copy hostname ${r.name || "@"}`}
                          value={r.name || "@"}
                          onCopied={onCopied}
                          onCopyError={onCopyError}
                        />
                      </div>
                    </td>
                    <td className="align-middle py-3 pr-3">
                      <div className="flex items-start break-all">
                        <span className="mono text-text">{r.value}</span>
                        <DnsCopyButton
                          ariaLabel="Copy record value"
                          value={r.value}
                          onCopied={onCopied}
                          onCopyError={onCopyError}
                        />
                      </div>
                    </td>
                  </tr>
                  {(r.note || r.zone_hint) && (
                    <tr className="border-b border-border bg-surface-alt last:border-b-0">
                      <td colSpan={3} className="px-3 py-2 text-[11px] leading-snug text-muted">
                        {r.note}
                        {r.note && r.zone_hint ? " · " : null}
                        {r.zone_hint ? <span className="mono">Zone: {r.zone_hint}</span> : null}
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="mt-4 text-xs text-muted">No concrete records returned yet. Check server DNS detection settings.</p>
      )}

      <div className="mt-4 space-y-2">
        <div className="border border-border bg-surface p-3 text-[11px] leading-snug text-muted">
          <span className="font-semibold text-text">Server IPv4 reference: </span>
          <span className="mono text-text">{guidance.ipv4 || "—"}</span>
          <span className="text-muted"> — </span>
          {guidance.ipv4_source}
        </div>
        {guidance.ipv6 ? (
          <div className="border border-border bg-surface p-3 text-[11px] leading-snug text-muted">
            <span className="font-semibold text-text">IPv6 (AAAA): </span>
            <span className="mono text-text">{guidance.ipv6}</span>
            {guidance.ipv6_source ? (
              <>
                <span className="text-muted"> — </span>
                {guidance.ipv6_source}
              </>
            ) : null}
          </div>
        ) : null}
        <div className="border border-border bg-surface p-3 text-[11px] leading-snug text-muted">
          DNS changes can take a few minutes to hours to propagate. Use the registrar column above to see when public DNS
          matches this server.
        </div>
      </div>

      <details className="mt-4 border border-border border-dashed bg-surface p-3">
        <summary className="cursor-pointer text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
          Plain text block
        </summary>
        <pre className="mono mt-3 max-h-48 overflow-auto whitespace-pre-wrap break-all text-[11px] leading-relaxed text-text">
          {text}
        </pre>
      </details>
        </div>
      )}
    </div>
  );
}
