import type { ThemePreference } from "../hooks/useSiteTheme";

type ThemeToggleProps = {
  preference: ThemePreference;
  onCycle: () => void;
};

export function ThemeToggle({ preference, onCycle }: ThemeToggleProps) {
  const label = preference === "dark" ? "Dark" : "Light";
  const icon = preference === "dark" ? "☾" : "☀";
  return (
    <button
      type="button"
      onClick={onCycle}
      title="Toggle theme: light / dark"
      className="font-mono inline-flex items-center gap-2 border border-border-strong bg-transparent px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-text hover:bg-surface-alt"
    >
      <span aria-hidden>{icon}</span>
      <span className="relative inline-block">
        <span className="invisible select-none whitespace-nowrap" aria-hidden>
          Light
        </span>
        <span className="absolute left-0 top-1/2 -translate-y-1/2 whitespace-nowrap">{label}</span>
      </span>
    </button>
  );
}
