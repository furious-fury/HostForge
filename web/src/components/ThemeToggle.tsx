import type { ThemePreference } from "../hooks/useUIPrefs";

type ThemeToggleProps = {
  preference: ThemePreference;
  effective: "light" | "dark";
  onCycle: () => void;
};

export function ThemeToggle({ preference, effective, onCycle }: ThemeToggleProps) {
  const label =
    preference === "system"
      ? `System (${effective === "dark" ? "Dark" : "Light"})`
      : preference === "dark"
        ? "Dark"
        : "Light";
  const icon = effective === "dark" ? "☾" : "☀";
  return (
    <button
      type="button"
      onClick={onCycle}
      title="Cycle theme: dark → light → system"
      className="mono inline-flex items-center gap-2 border border-border-strong bg-transparent px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-text hover:bg-surface-alt"
    >
      <span aria-hidden>{icon}</span>
      <span>{label}</span>
    </button>
  );
}
