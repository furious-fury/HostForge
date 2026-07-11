import { useState, type ReactNode } from "react";
import { Button } from "../../components/Button";

type SettingsRowProps = {
  label: string;
  env?: string;
  value?: ReactNode;
  mono?: boolean;
  children?: ReactNode;
};

export function SettingsRow({ label, env, value, mono, children }: SettingsRowProps) {
  return (
    <div className="flex flex-col gap-1 border-b border-border py-3 last:border-b-0 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="min-w-0 shrink-0 sm:w-48">
        <div className="text-sm font-medium text-text">{label}</div>
        {env && (
          <div className="mono mt-0.5 text-[10px] uppercase tracking-wider text-muted">
            via <span className="text-muted">{env}</span>
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1 text-sm">
        {value !== undefined && value !== null && value !== "" ? (
          <div className={mono ? "mono break-all text-text" : "text-text"}>{value}</div>
        ) : null}
        {children}
      </div>
    </div>
  );
}

export function CopyValueButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setDone(true);
      window.setTimeout(() => setDone(false), 1500);
    } catch {
      //
    }
  }
  if (!text) return null;
  return (
    <Button variant="ghost" size="sm" className="mt-1" onClick={() => void copy()}>
      {done ? "Copied" : label}
    </Button>
  );
}

export function SecretPill({ set: isSet }: { set: boolean }) {
  return (
    <span
      className={`mono inline-flex items-center gap-1 border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${
        isSet ? "border-success text-success" : "border-warning text-warning"
      }`}
    >
      <span aria-hidden>●</span>
      {isSet ? "Set" : "Not set"}
    </span>
  );
}
