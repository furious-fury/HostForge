import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import type { ApiDeployment, ApiProject } from "../api";
import { fetchAllDeployments, fetchProjects } from "../api";
import { fleetKeys } from "../hooks/fleetQueries";

const DEPLOYMENTS_LIMIT = 200;

type PaletteRow =
  | { kind: "project"; project: ApiProject }
  | { kind: "deployment"; deployment: ApiDeployment; projectName: string | null };

function normalize(s: string): string {
  return s.trim().toLowerCase();
}

function projectMatches(p: ApiProject, q: string): boolean {
  if (!q) return true;
  const hay = [
    p.name,
    p.id,
    p.repo_url,
    p.branch,
    p.latest_deployment?.id ?? "",
    p.latest_deployment?.status ?? "",
  ]
    .join(" ")
    .toLowerCase();
  return hay.includes(q);
}

function deploymentMatches(d: ApiDeployment, q: string): boolean {
  if (!q) return true;
  const hay = [d.id, d.project_id, d.status, d.commit_hash, d.image_ref, d.worktree].join(" ").toLowerCase();
  return hay.includes(q);
}

function buildRows(
  projects: ApiProject[] | undefined,
  deployments: ApiDeployment[] | undefined,
  q: string,
): PaletteRow[] {
  const nq = normalize(q);
  const projectById = new Map<string, ApiProject>();
  for (const p of projects ?? []) {
    projectById.set(p.id, p);
  }

  const rows: PaletteRow[] = [];
  for (const p of projects ?? []) {
    if (projectMatches(p, nq)) {
      rows.push({ kind: "project", project: p });
    }
  }
  for (const d of deployments ?? []) {
    if (deploymentMatches(d, nq)) {
      const proj = projectById.get(d.project_id);
      rows.push({ kind: "deployment", deployment: d, projectName: proj?.name ?? null });
    }
  }
  return rows.slice(0, 80);
}

type CommandPaletteProps = {
  open: boolean;
  onClose: () => void;
};

export function CommandPalette({ open, onClose }: CommandPaletteProps) {
  const navigate = useNavigate();
  const titleId = useId();
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const lastFocusRef = useRef<HTMLElement | null>(null);

  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);

  const projectsQ = useQuery({
    queryKey: fleetKeys.projects,
    queryFn: fetchProjects,
    enabled: open,
    staleTime: 45_000,
  });

  const deploymentsQ = useQuery({
    queryKey: fleetKeys.deployments(DEPLOYMENTS_LIMIT),
    queryFn: () => fetchAllDeployments(DEPLOYMENTS_LIMIT),
    enabled: open,
    staleTime: 45_000,
    retry: 1,
  });

  const rows = useMemo(
    () => buildRows(projectsQ.data, deploymentsQ.data, query),
    [projectsQ.data, deploymentsQ.data, query],
  );

  useEffect(() => {
    setActiveIndex(0);
  }, [query, open]);

  useEffect(() => {
    if (rows.length && activeIndex >= rows.length) {
      setActiveIndex(rows.length - 1);
    }
  }, [rows.length, activeIndex]);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setActiveIndex(0);
      return;
    }
    lastFocusRef.current = document.activeElement as HTMLElement;
    const t = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(t);
  }, [open]);

  const close = useCallback(() => {
    onClose();
    window.setTimeout(() => {
      lastFocusRef.current?.focus?.();
      lastFocusRef.current = null;
    }, 0);
  }, [onClose]);

  const goRow = useCallback(
    (row: PaletteRow) => {
      if (row.kind === "project") {
        navigate(`/projects/${encodeURIComponent(row.project.id)}`);
      } else {
        navigate(
          `/projects/${encodeURIComponent(row.deployment.project_id)}/deployments/${encodeURIComponent(row.deployment.id)}`,
        );
      }
      close();
    },
    [navigate, close],
  );

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setActiveIndex((i) => Math.min(i + 1, Math.max(rows.length - 1, 0)));
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setActiveIndex((i) => Math.max(i - 1, 0));
        return;
      }
      if (e.key === "Enter" && rows.length) {
        e.preventDefault();
        const row = rows[Math.min(activeIndex, rows.length - 1)];
        if (row) goRow(row);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, rows, activeIndex, close, goRow]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      const el = panelRef.current;
      if (!el) return;
      if (!el.contains(e.target as Node)) {
        close();
      }
    };
    document.addEventListener("pointerdown", onPointerDown, true);
    return () => document.removeEventListener("pointerdown", onPointerDown, true);
  }, [open, close]);

  useEffect(() => {
    if (!open) return;
    const trap = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || !panelRef.current) return;
      const focusables = panelRef.current.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      const list = [...focusables].filter((el) => !el.hasAttribute("disabled") && el.offsetParent !== null);
      if (list.length === 0) return;
      const first = list[0];
      const last = list[list.length - 1];
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last.focus();
        }
      } else if (document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", trap);
    return () => document.removeEventListener("keydown", trap);
  }, [open]);

  if (!open) {
    return null;
  }

  const loading = projectsQ.isPending || deploymentsQ.isPending;
  const fetchFailed =
    !loading &&
    projectsQ.isError &&
    deploymentsQ.isError &&
    projectsQ.data === undefined &&
    deploymentsQ.data === undefined;

  const activeDescendant =
    rows.length > 0 ? `palette-opt-${Math.min(activeIndex, rows.length - 1)}` : undefined;

  const content = (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-black/40 px-4 pt-[12vh]"
      role="presentation"
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="mono w-full max-w-lg border border-border-strong bg-surface shadow-lg"
      >
        <div className="border-b border-border px-3 py-2">
          <span id={titleId} className="sr-only">
            Command palette: search projects and deployments
          </span>
          <input
            ref={inputRef}
            type="search"
            autoComplete="off"
            spellCheck={false}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter projects and deployments…"
            className="w-full border border-border bg-surface-alt px-2 py-2 text-xs text-text placeholder:text-muted focus:border-border-strong focus:outline-none"
            aria-autocomplete="list"
            aria-controls={listId}
            aria-activedescendant={activeDescendant}
            aria-expanded
          />
        </div>

        <div
          id={listId}
          role="listbox"
          aria-label="Results"
          className="max-h-[min(50vh,24rem)] overflow-y-auto py-1"
        >
          {loading && <div className="px-3 py-4 text-xs text-muted">Loading…</div>}
          {fetchFailed && (
            <div className="px-3 py-4 text-xs text-danger">Could not load data. Try again.</div>
          )}
          {!loading && !fetchFailed && rows.length === 0 && (
            <div className="px-3 py-4 text-xs text-muted">
              {normalize(query) ? "No matches." : "Type to filter projects and deployments."}
            </div>
          )}
          {!loading &&
            !fetchFailed &&
            rows.map((row, idx) => {
              const selected = idx === activeIndex;
              const id = `palette-opt-${idx}`;
              if (row.kind === "project") {
                const p = row.project;
                return (
                  <div
                    key={`p-${p.id}`}
                    id={id}
                    role="option"
                    aria-selected={selected}
                    className={`cursor-pointer px-3 py-2 text-xs ${selected ? "bg-surface-alt text-text" : "text-text"}`}
                    onMouseEnter={() => setActiveIndex(idx)}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => goRow(row)}
                  >
                    <div className="font-semibold">{p.name}</div>
                    <div className="truncate text-muted">Project · {p.id}</div>
                  </div>
                );
              }
              const d = row.deployment;
              return (
                <div
                  key={`d-${d.id}`}
                  id={id}
                  role="option"
                  aria-selected={selected}
                  className={`cursor-pointer px-3 py-2 text-xs ${selected ? "bg-surface-alt text-text" : "text-text"}`}
                  onMouseEnter={() => setActiveIndex(idx)}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => goRow(row)}
                >
                  <div className="font-semibold">
                    Deployment {d.id.slice(0, 8)}
                    {d.id.length > 8 ? "…" : ""}
                    <span className="ml-2 font-normal text-muted">{d.status}</span>
                  </div>
                  <div className="truncate text-muted">
                    {row.projectName ? `${row.projectName} · ` : ""}
                    {d.project_id}
                  </div>
                </div>
              );
            })}
        </div>

        <div className="flex items-center justify-between border-t border-border px-3 py-2 text-[10px] uppercase tracking-wider text-muted">
          <span>
            <kbd className="border border-border px-1">↑</kbd>{" "}
            <kbd className="border border-border px-1">↓</kbd> move ·{" "}
            <kbd className="border border-border px-1">Enter</kbd> open ·{" "}
            <kbd className="border border-border px-1">Esc</kbd> close
          </span>
          <button
            type="button"
            className="border border-border bg-surface-alt px-2 py-1 text-[10px] text-text hover:border-border-strong"
            onClick={close}
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );

  return createPortal(content, document.body);
}
