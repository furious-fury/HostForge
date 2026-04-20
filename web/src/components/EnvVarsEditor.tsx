import { useCallback, useEffect, useMemo, useState } from "react";
import type { ApiProjectEnvVar } from "../api";
import { deleteProjectEnv, listProjectEnv, updateProjectEnv, upsertProjectEnv } from "../api";
import { Button } from "./Button";
import { useConfirm } from "./useConfirm";

function EyeIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function EyeOffIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24" />
      <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
  );
}

function VisibilityIconButton({
  visible,
  onToggle,
  labelShow,
  labelHide,
}: {
  visible: boolean;
  onToggle: () => void;
  labelShow: string;
  labelHide: string;
}) {
  return (
    <button
      type="button"
      className="inline-flex h-9 w-9 shrink-0 items-center justify-center border border-border bg-surface text-muted hover:border-border-strong hover:text-text"
      aria-pressed={visible}
      aria-label={visible ? labelHide : labelShow}
      onClick={onToggle}
    >
      {visible ? <EyeOffIcon /> : <EyeIcon />}
    </button>
  );
}

function PencilIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </svg>
  );
}

function TrashIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <polyline points="3 6 5 6 21 6" />
      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
      <line x1="10" y1="11" x2="10" y2="17" />
      <line x1="14" y1="11" x2="14" y2="17" />
    </svg>
  );
}

function CheckIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function XIcon({ className = "size-4" }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  );
}

type IconActionVariant = "default" | "danger" | "primary";

function IconActionButton({
  label,
  onClick,
  disabled,
  variant = "default",
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  variant?: IconActionVariant;
  children: React.ReactNode;
}) {
  const variantClass =
    variant === "danger"
      ? "border-border bg-surface text-muted hover:border-danger/60 hover:text-danger"
      : variant === "primary"
        ? "border-primary bg-primary text-primary-ink hover:brightness-110"
        : "border-border bg-surface text-muted hover:border-border-strong hover:text-text";
  return (
    <button
      type="button"
      disabled={disabled}
      className={`inline-flex h-9 w-9 shrink-0 items-center justify-center border ${variantClass} disabled:cursor-not-allowed disabled:opacity-50`}
      aria-label={label}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

export type EnvDraftPair = { key: string; value: string };

type LocalProps = {
  mode: "local";
  value: EnvDraftPair[];
  onChange: (rows: EnvDraftPair[]) => void;
};

type RemoteProps = {
  mode: "remote";
  projectID: string;
  onChange?: () => void;
};

export type EnvVarsEditorProps = LocalProps | RemoteProps;

const KEY_RE = /^[A-Z][A-Z0-9_]*$/;

export function normalizeEnvKey(raw: string): string {
  return raw.trim().replace(/-/g, "_").toUpperCase();
}

function validateKey(k: string): string | null {
  if (!k.trim()) return "Key is required.";
  if (k.length > 128) return "Key is too long (max 128).";
  if (k === "PORT") return "PORT is reserved (set by HostForge).";
  if (!KEY_RE.test(k)) return "Use A–Z, 0–9, and underscores; must start with a letter.";
  return null;
}

function validateValue(v: string): string | null {
  if (new TextEncoder().encode(v).length > 8 * 1024) return "Value is too long (max 8 KiB).";
  return null;
}

function parseDotEnvBlock(text: string): EnvDraftPair[] {
  const out: EnvDraftPair[] = [];
  const seen = new Set<string>();
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const eq = line.indexOf("=");
    if (eq <= 0) continue;
    const keyRaw = line.slice(0, eq).trim();
    const value = line.slice(eq + 1);
    const key = normalizeEnvKey(keyRaw);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push({ key, value });
  }
  return out;
}

function maskPreview(last4: string): string {
  return `••••••••${last4 || ""}`;
}

export function EnvVarsEditor(props: EnvVarsEditorProps) {
  if (props.mode === "local") {
    return <LocalEnvEditor rows={props.value} onChange={props.onChange} />;
  }

  return <RemoteEnvEditor projectID={props.projectID} onChange={props.onChange} />;
}

function LocalEnvEditor({ rows, onChange }: { rows: EnvDraftPair[]; onChange: (rows: EnvDraftPair[]) => void }) {
  const [newKey, setNewKey] = useState("");
  const [newVal, setNewVal] = useState("");
  const [showNewVal, setShowNewVal] = useState(false);
  const [revealedKeys, setRevealedKeys] = useState<Set<string>>(() => new Set());
  const [editIdx, setEditIdx] = useState<number | null>(null);
  const [editKey, setEditKey] = useState("");
  const [editVal, setEditVal] = useState("");
  const [showEditVal, setShowEditVal] = useState(false);
  const [err, setErr] = useState("");
  const [pasteText, setPasteText] = useState("");

  const cancelEdit = useCallback(() => {
    setEditIdx(null);
    setEditKey("");
    setEditVal("");
    setShowEditVal(false);
  }, []);

  const toggleRevealKey = useCallback((key: string) => {
    setRevealedKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  const beginEdit = useCallback(
    (idx: number) => {
      const r = rows[idx];
      if (!r) return;
      setErr("");
      setEditIdx(idx);
      setEditKey(r.key);
      setEditVal(r.value);
      setShowEditVal(false);
    },
    [rows],
  );

  const saveEdit = useCallback(() => {
    if (editIdx === null) return;
    setErr("");
    const k = normalizeEnvKey(editKey);
    const ke = validateKey(k);
    if (ke) {
      setErr(ke);
      return;
    }
    const vv = validateValue(editVal);
    if (vv) {
      setErr(vv);
      return;
    }
    const oldKey = rows[editIdx]?.key;
    if (oldKey === undefined) return;
    if (rows.some((row, i) => i !== editIdx && normalizeEnvKey(row.key) === k)) {
      setErr("That key is already in the list.");
      return;
    }
    const next = rows.map((row, i) => (i === editIdx ? { key: k, value: editVal } : row));
    onChange(next);
    if (oldKey !== k) {
      setRevealedKeys((prev) => {
        const n = new Set(prev);
        n.delete(oldKey);
        return n;
      });
    }
    cancelEdit();
  }, [cancelEdit, editIdx, editKey, editVal, onChange, rows]);

  const removeRow = useCallback(
    (idx: number) => {
      setErr("");
      if (editIdx === idx) {
        cancelEdit();
      } else if (editIdx !== null && editIdx > idx) {
        setEditIdx(editIdx - 1);
      }
      onChange(rows.filter((_, i) => i !== idx));
    },
    [cancelEdit, editIdx, onChange, rows],
  );

  const addRow = useCallback(() => {
    setErr("");
    const k = normalizeEnvKey(newKey);
    const ve = validateKey(k);
    if (ve) {
      setErr(ve);
      return;
    }
    const vv = validateValue(newVal);
    if (vv) {
      setErr(vv);
      return;
    }
    if (rows.some((r) => normalizeEnvKey(r.key) === k)) {
      setErr("That key is already in the list.");
      return;
    }
    onChange([...rows, { key: k, value: newVal }]);
    setNewKey("");
    setNewVal("");
  }, [newKey, newVal, onChange, rows]);

  const applyPaste = useCallback(() => {
    setErr("");
    const parsed = parseDotEnvBlock(pasteText);
    if (parsed.length === 0) {
      setErr("No KEY=value lines found.");
      return;
    }
    const merged = [...rows];
    const keys = new Set(merged.map((r) => normalizeEnvKey(r.key)));
    for (const p of parsed) {
      const ve = validateKey(p.key) || validateValue(p.value);
      if (ve) {
        setErr(`${p.key}: ${ve}`);
        return;
      }
      if (keys.has(p.key)) {
        const idx = merged.findIndex((r) => normalizeEnvKey(r.key) === p.key);
        if (idx >= 0) merged[idx] = { key: p.key, value: p.value };
      } else {
        merged.push(p);
        keys.add(p.key);
      }
    }
    if (merged.length > 100) {
      setErr("At most 100 variables.");
      return;
    }
    onChange(merged);
    setPasteText("");
  }, [onChange, pasteText, rows]);

  return (
    <div className="flex flex-col gap-4">
      {err && <div className="border border-danger bg-danger/10 p-2 text-xs text-danger">{err}</div>}

      <div className="max-h-[min(24rem,60vh)] w-full overflow-auto border border-border">
        <table className="w-full table-fixed text-left text-xs">
          <colgroup>
            <col className="w-[40%]" />
            <col className="w-[calc(100%-40%-5.5rem)]" />
            <col className="w-[5.5rem]" />
          </colgroup>
          <thead>
            <tr className="mono border-b border-border bg-surface-alt text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
              <th className="sticky top-0 z-[1] min-w-0 max-w-0 bg-surface-alt px-3 py-2 shadow-[0_1px_0_0_var(--hf-border)]">
                Key
              </th>
              <th className="sticky top-0 z-[1] min-w-0 max-w-0 bg-surface-alt px-3 py-2 shadow-[0_1px_0_0_var(--hf-border)]">
                Value
              </th>
              <th className="sticky top-0 z-[1] w-[5.5rem] min-w-0 max-w-[5.5rem] bg-surface-alt px-3 py-2 text-right shadow-[0_1px_0_0_var(--hf-border)]">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td colSpan={3} className="px-3 py-4 text-muted">
                  No variables yet. Add rows below or paste a <span className="mono">.env</span> fragment.
                </td>
              </tr>
            ) : (
              rows.map((r, idx) => (
                <tr key={`${r.key}-${idx}`} className="border-b border-border/60 last:border-b-0">
                  <td className="mono min-w-0 max-w-0 px-3 py-2 font-medium text-text align-top">
                    {editIdx === idx ? (
                      <input
                        className="mono box-border w-full min-w-0 border border-border bg-surface-alt px-2 py-1 text-xs font-medium text-text focus:border-border-strong focus:outline-none"
                        value={editKey}
                        onChange={(e) => setEditKey(e.target.value)}
                        onBlur={() => setEditKey((k) => normalizeEnvKey(k))}
                        aria-label="Environment variable key"
                        spellCheck={false}
                        autoComplete="off"
                      />
                    ) : (
                      <span className="block whitespace-normal break-anywhere text-sm leading-snug">{r.key}</span>
                    )}
                  </td>
                  <td className="min-w-0 max-w-0 px-3 py-2 align-top">
                    {editIdx === idx ? (
                      <div className="flex min-w-0 items-start gap-2">
                        <VisibilityIconButton
                          visible={showEditVal}
                          onToggle={() => setShowEditVal((v) => !v)}
                          labelShow="Show value while editing"
                          labelHide="Hide value while editing"
                        />
                        <input
                          type={showEditVal ? "text" : "password"}
                          className="mono box-border min-w-0 w-0 flex-1 border border-border bg-surface-alt px-2 py-1 text-xs text-text focus:border-border-strong focus:outline-none"
                          value={editVal}
                          onChange={(e) => setEditVal(e.target.value)}
                          aria-label="Environment variable value"
                          spellCheck={false}
                          autoComplete="off"
                        />
                      </div>
                    ) : (
                      <div className="flex min-w-0 items-start gap-2">
                        <VisibilityIconButton
                          visible={revealedKeys.has(r.key)}
                          onToggle={() => toggleRevealKey(r.key)}
                          labelShow={`Show value for ${r.key}`}
                          labelHide={`Hide value for ${r.key}`}
                        />
                        <div className="min-w-0 flex-1">
                          {revealedKeys.has(r.key) ? (
                            <span className="mono block whitespace-normal break-anywhere text-sm text-text">{r.value}</span>
                          ) : (
                            <span className="block text-muted">•••••••• (set before deploy)</span>
                          )}
                        </div>
                      </div>
                    )}
                  </td>
                  <td className="w-[5.5rem] min-w-0 max-w-[5.5rem] px-3 py-2 align-top">
                    <div className="flex justify-end gap-1">
                      {editIdx === idx ? (
                        <>
                          <IconActionButton label="Save changes" variant="primary" onClick={() => void saveEdit()}>
                            <CheckIcon />
                          </IconActionButton>
                          <IconActionButton label="Cancel editing" onClick={cancelEdit}>
                            <XIcon />
                          </IconActionButton>
                        </>
                      ) : (
                        <>
                          <IconActionButton label={`Edit ${r.key}`} onClick={() => beginEdit(idx)}>
                            <PencilIcon />
                          </IconActionButton>
                          <IconActionButton label={`Remove ${r.key}`} variant="danger" onClick={() => removeRow(idx)}>
                            <TrashIcon />
                          </IconActionButton>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">New key</span>
          <input
            className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
            value={newKey}
            onChange={(e) => setNewKey(e.target.value)}
            onBlur={() => setNewKey((k) => normalizeEnvKey(k))}
            placeholder="DATABASE_URL"
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">New value</span>
          <div className="flex gap-2">
            <input
              type={showNewVal ? "text" : "password"}
              className="mono min-w-0 flex-1 border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={newVal}
              onChange={(e) => setNewVal(e.target.value)}
              placeholder="secret"
              spellCheck={false}
              autoComplete="off"
            />
            <VisibilityIconButton
              visible={showNewVal}
              onToggle={() => setShowNewVal((v) => !v)}
              labelShow="Show new value"
              labelHide="Hide new value"
            />
          </div>
        </label>
      </div>
      <div>
        <Button type="button" variant="secondary" size="sm" onClick={addRow}>
          Add variable
        </Button>
      </div>

      <details>
        <summary className="cursor-pointer text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
          Paste from .env
        </summary>
        <div className="mt-2 flex flex-col gap-2">
          <textarea
            className="mono min-h-[120px] w-full border border-border bg-surface-alt p-3 text-xs text-text focus:border-border-strong focus:outline-none"
            value={pasteText}
            onChange={(e) => setPasteText(e.target.value)}
            placeholder={"FOO=bar\n# comment\nBAZ=qux"}
            spellCheck={false}
          />
          <Button type="button" variant="secondary" size="sm" onClick={applyPaste}>
            Merge pasted lines
          </Button>
        </div>
      </details>
    </div>
  );
}

function RemoteEnvEditor({ projectID, onChange }: { projectID: string; onChange?: () => void }) {
  const confirmDialog = useConfirm();
  const [rows, setRows] = useState<ApiProjectEnvVar[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [noKey, setNoKey] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newVal, setNewVal] = useState("");
  const [showNewVal, setShowNewVal] = useState(false);
  const [replaceId, setReplaceId] = useState<string | null>(null);
  const [replaceVal, setReplaceVal] = useState("");
  const [showReplaceVal, setShowReplaceVal] = useState(false);
  const [busy, setBusy] = useState(false);
  const [pasteText, setPasteText] = useState("");

  const reload = useCallback(async () => {
    setLoading(true);
    setErr("");
    setNoKey(false);
    try {
      const list = await listProjectEnv(projectID);
      setRows(list);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("env_encryption_key_missing")) {
        setNoKey(true);
        setRows([]);
      } else {
        setErr(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [projectID]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const fireChange = useCallback(() => {
    onChange?.();
  }, [onChange]);

  const addRemote = useCallback(async () => {
    setErr("");
    const k = normalizeEnvKey(newKey);
    const ke = validateKey(k);
    if (ke) {
      setErr(ke);
      return;
    }
    const ve = validateValue(newVal);
    if (ve) {
      setErr(ve);
      return;
    }
    setBusy(true);
    try {
      const rec = await upsertProjectEnv(projectID, k, newVal);
      setRows((prev) => {
        const others = prev.filter((r) => r.key !== rec.key);
        return [...others, rec].sort((a, b) => a.key.localeCompare(b.key));
      });
      setNewKey("");
      setNewVal("");
      fireChange();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
    }
  }, [fireChange, newKey, newVal, projectID]);

  const saveReplace = useCallback(async () => {
    if (!replaceId) return;
    const ve = validateValue(replaceVal);
    if (ve) {
      setErr(ve);
      return;
    }
    setBusy(true);
    setErr("");
    try {
      const rec = await updateProjectEnv(projectID, replaceId, replaceVal);
      setRows((prev) => prev.map((r) => (r.id === rec.id ? rec : r)));
      setReplaceId(null);
      setReplaceVal("");
      fireChange();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "update failed");
    } finally {
      setBusy(false);
    }
  }, [fireChange, projectID, replaceId, replaceVal]);

  const removeRemote = useCallback(
    async (id: string) => {
      const row = rows.find((r) => r.id === id);
      const ok = await confirmDialog({
        title: "Remove environment variable",
        description: (
          <>
            Remove <span className="mono font-semibold">{row?.key ?? id}</span> from this project? The variable will be
            deleted immediately and won't be injected into future deployments.
          </>
        ),
        confirmLabel: "Remove",
        confirmVariant: "danger",
        dangerBanner: null,
      });
      if (!ok) return;
      setBusy(true);
      setErr("");
      try {
        await deleteProjectEnv(projectID, id);
        setRows((prev) => prev.filter((r) => r.id !== id));
        if (replaceId === id) {
          setReplaceId(null);
          setReplaceVal("");
        }
        fireChange();
      } catch (e) {
        setErr(e instanceof Error ? e.message : "delete failed");
      } finally {
        setBusy(false);
      }
    },
    [confirmDialog, fireChange, projectID, replaceId, rows],
  );

  const applyPasteRemote = useCallback(async () => {
    setErr("");
    const parsed = parseDotEnvBlock(pasteText);
    if (parsed.length === 0) {
      setErr("No KEY=value lines found.");
      return;
    }
    if (rows.length + parsed.length > 100) {
      setErr("At most 100 variables.");
      return;
    }
    setBusy(true);
    try {
      for (const p of parsed) {
        const ke = validateKey(p.key) || validateValue(p.value);
        if (ke) throw new Error(`${p.key}: ${ke}`);
        await upsertProjectEnv(projectID, p.key, p.value);
      }
      await reload();
      setPasteText("");
      fireChange();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "paste apply failed");
    } finally {
      setBusy(false);
    }
  }, [fireChange, pasteText, projectID, reload, rows.length]);

  const sorted = useMemo(() => [...rows].sort((a, b) => a.key.localeCompare(b.key)), [rows]);

  if (loading) {
    return <p className="text-xs text-muted">Loading environment variables…</p>;
  }

  if (noKey) {
    return (
      <div className="border border-warning/50 bg-warning/5 p-3 text-xs text-muted">
        <p className="font-semibold text-text">Encryption key not configured</p>
        <p className="mt-1">
          Set <span className="mono text-text">HOSTFORGE_ENV_ENCRYPTION_KEY</span> on the server (see README), restart HostForge, then
          reload this page to manage environment variables.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {err && <div className="border border-danger bg-danger/10 p-2 text-xs text-danger">{err}</div>}

      <div className="max-h-[min(24rem,60vh)] w-full overflow-auto border border-border">
        <table className="w-full table-fixed text-left text-xs">
          <colgroup>
            <col className="w-[40%]" />
            <col className="w-[calc(100%-40%-5.5rem)]" />
            <col className="w-[5.5rem]" />
          </colgroup>
          <thead>
            <tr className="mono border-b border-border bg-surface-alt text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
              <th className="sticky top-0 z-[1] min-w-0 max-w-0 bg-surface-alt px-3 py-2 shadow-[0_1px_0_0_var(--hf-border)]">
                Key
              </th>
              <th className="sticky top-0 z-[1] min-w-0 max-w-0 bg-surface-alt px-3 py-2 shadow-[0_1px_0_0_var(--hf-border)]">
                Value
              </th>
              <th className="sticky top-0 z-[1] w-[5.5rem] min-w-0 max-w-[5.5rem] bg-surface-alt px-3 py-2 text-right shadow-[0_1px_0_0_var(--hf-border)]">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={3} className="px-3 py-4 text-muted">
                  No variables yet. Add below or paste a <span className="mono">.env</span> fragment.
                </td>
              </tr>
            ) : (
              sorted.map((r) => (
                <tr key={r.id} className="border-b border-border/60 last:border-b-0">
                  <td className="mono min-w-0 max-w-0 px-3 py-2 font-medium text-text align-top">
                    <span className="block whitespace-normal break-anywhere text-sm leading-snug">{r.key}</span>
                  </td>
                  <td className="min-w-0 max-w-0 px-3 py-2 align-top text-muted">
                    {replaceId === r.id ? (
                      <div className="flex min-w-0 items-start gap-2">
                        <VisibilityIconButton
                          visible={showReplaceVal}
                          onToggle={() => setShowReplaceVal((v) => !v)}
                          labelShow={`Show new value for ${r.key}`}
                          labelHide={`Hide new value for ${r.key}`}
                        />
                        <input
                          type={showReplaceVal ? "text" : "password"}
                          className="mono box-border min-w-0 w-0 flex-1 border border-border bg-surface-alt px-2 py-1 text-xs text-text focus:border-border-strong focus:outline-none"
                          value={replaceVal}
                          onChange={(e) => setReplaceVal(e.target.value)}
                          aria-label={`New value for ${r.key}`}
                          autoComplete="off"
                        />
                      </div>
                    ) : (
                      <span className="mono block whitespace-normal break-anywhere">{maskPreview(r.value_last4)}</span>
                    )}
                  </td>
                  <td className="w-[5.5rem] min-w-0 max-w-[5.5rem] px-3 py-2 align-top">
                    <div className="flex justify-end gap-1">
                      {replaceId === r.id ? (
                        <>
                          <IconActionButton
                            label={`Save new value for ${r.key}`}
                            variant="primary"
                            disabled={busy}
                            onClick={() => void saveReplace()}
                          >
                            <CheckIcon />
                          </IconActionButton>
                          <IconActionButton
                            label="Cancel replace"
                            disabled={busy}
                            onClick={() => {
                              setReplaceId(null);
                              setReplaceVal("");
                            }}
                          >
                            <XIcon />
                          </IconActionButton>
                        </>
                      ) : (
                        <>
                          <IconActionButton
                            label={`Replace value for ${r.key}`}
                            disabled={busy}
                            onClick={() => {
                              setReplaceId(r.id);
                              setReplaceVal("");
                              setShowReplaceVal(false);
                            }}
                          >
                            <PencilIcon />
                          </IconActionButton>
                          <IconActionButton
                            label={`Delete ${r.key}`}
                            variant="danger"
                            disabled={busy}
                            onClick={() => void removeRemote(r.id)}
                          >
                            <TrashIcon />
                          </IconActionButton>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <label className="flex flex-col gap-1">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">New key</span>
          <input
            className="mono border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
            value={newKey}
            onChange={(e) => setNewKey(e.target.value)}
            onBlur={() => setNewKey((k) => normalizeEnvKey(k))}
            disabled={busy}
            spellCheck={false}
            autoComplete="off"
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">New value</span>
          <div className="flex gap-2">
            <input
              type={showNewVal ? "text" : "password"}
              className="mono min-w-0 flex-1 border border-border bg-surface-alt px-3 py-2 text-sm text-text focus:border-border-strong focus:outline-none"
              value={newVal}
              onChange={(e) => setNewVal(e.target.value)}
              disabled={busy}
              autoComplete="off"
            />
            <VisibilityIconButton
              visible={showNewVal}
              onToggle={() => setShowNewVal((v) => !v)}
              labelShow="Show new value"
              labelHide="Hide new value"
            />
          </div>
        </label>
      </div>
      <Button type="button" variant="secondary" size="sm" disabled={busy} onClick={() => void addRemote()}>
        Add variable
      </Button>

      <details>
        <summary className="cursor-pointer text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
          Paste from .env
        </summary>
        <div className="mt-2 flex flex-col gap-2">
          <textarea
            className="mono min-h-[120px] w-full border border-border bg-surface-alt p-3 text-xs text-text focus:border-border-strong focus:outline-none"
            value={pasteText}
            onChange={(e) => setPasteText(e.target.value)}
            disabled={busy}
            spellCheck={false}
          />
          <Button type="button" variant="secondary" size="sm" disabled={busy} onClick={() => void applyPasteRemote()}>
            Apply pasted lines
          </Button>
        </div>
      </details>
    </div>
  );
}
