import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Button } from "./Button";

/** User must type `expected` exactly (after trim on both sides) before confirm is enabled. */
export type TypeConfirmConfig = {
  /** Short line above the field (e.g. explain why). */
  prompt: string;
  /** Required text, usually the resource name. */
  expected: string;
};

export type ConfirmDialogProps = {
  open: boolean;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Primary action button style */
  confirmVariant?: "danger" | "primary";
  /** When set, confirm stays disabled until the input matches `expected` (trimmed). */
  typeConfirm?: TypeConfirmConfig;
  /**
   * Warning callout (red-tinted panel + thick left border). When omitted and this is a destructive dialog
   * (`confirmVariant === "danger"` or `typeConfirm` is set), a default warning title + body is shown.
   */
  dangerBanner?: ReactNode;
  onConfirm: () => void | Promise<void>;
  onClose: () => void;
};

const defaultDangerBanner = (
  <div>
    <p className="text-base font-bold leading-tight text-danger">Warning</p>
    <p className="mt-1.5 text-sm leading-snug text-danger">
      This action is permanent and cannot be undone. HostForge will stop and remove{" "}
      <span className="font-semibold">linked Docker containers</span> and delete{" "}
      <span className="font-semibold">deployment and domain records</span> from its database.
    </p>
  </div>
);

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  confirmVariant = "danger",
  typeConfirm,
  dangerBanner,
  onConfirm,
  onClose,
}: ConfirmDialogProps) {
  const [pending, setPending] = useState(false);
  const [typed, setTyped] = useState("");

  useEffect(() => {
    if (!open) {
      setPending(false);
      setTyped("");
      return;
    }
    setTyped("");
  }, [open, typeConfirm?.expected]);

  const required = typeConfirm?.expected.trim() ?? "";
  const typedOk = required !== "" && typed.trim() === required;
  const confirmEnabled = !typeConfirm || typedOk;

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !pending) {
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose, pending]);

  if (!open) {
    return null;
  }

  const isDestructive = confirmVariant === "danger" || typeConfirm !== undefined;
  const bannerNode = dangerBanner !== undefined ? dangerBanner : isDestructive ? defaultDangerBanner : null;

  async function handleConfirm() {
    if (!confirmEnabled) {
      return;
    }
    setPending(true);
    try {
      await onConfirm();
    } finally {
      setPending(false);
    }
  }

  return createPortal(
    <>
      <button
        type="button"
        className="fixed inset-0 z-[90] cursor-default bg-black/75"
        aria-label="Close dialog"
        onClick={() => {
          if (!pending) onClose();
        }}
      />
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="hf-confirm-title"
        className="fixed left-1/2 top-1/2 z-[95] w-[min(100vw-2rem,28rem)] -translate-x-1/2 -translate-y-1/2 border border-border-strong bg-surface p-6 text-text"
      >
        <h2 id="hf-confirm-title" className="text-lg font-semibold tracking-tight">
          {title}
        </h2>
        {bannerNode != null && (
          <div
            role="note"
            className="mt-4 border-l-[4px] border-danger bg-danger/5 py-3 pl-3 pr-4"
          >
            {bannerNode}
          </div>
        )}
        <div className="mt-3 text-sm leading-relaxed text-muted">{description}</div>

        {typeConfirm && (
          <div className="mt-5 border border-border bg-surface-alt p-4">
            <label className="block text-xs font-medium text-text" htmlFor="hf-confirm-type">
              <span className="mono text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                {typeConfirm.prompt}
              </span>
              <div className="mt-1 mono text-sm text-muted">
                Required: <span className="font-semibold text-text">{typeConfirm.expected}</span>
              </div>
              <input
                id="hf-confirm-type"
                type="text"
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                autoComplete="off"
                autoCapitalize="off"
                autoCorrect="off"
                spellCheck={false}
                disabled={pending}
                aria-invalid={typed.length > 0 && !typedOk}
                className="mt-2 w-full border border-border bg-bg px-3 py-2 font-mono text-sm text-text focus:border-border-strong focus:outline-none disabled:opacity-50"
                placeholder="Type here…"
                autoFocus
              />
            </label>
            {typed.length > 0 && !typedOk && (
              <p className="mt-2 text-xs text-danger">Text does not match yet.</p>
            )}
          </div>
        )}

        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button variant="secondary" disabled={pending} onClick={onClose} type="button">
            {cancelLabel}
          </Button>
          <Button
            variant={confirmVariant === "danger" ? "danger" : "primary"}
            disabled={pending || !confirmEnabled}
            onClick={() => void handleConfirm()}
            type="button"
          >
            {pending ? "…" : confirmLabel}
          </Button>
        </div>
      </div>
    </>,
    document.body,
  );
}
