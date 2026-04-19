import { Theme } from "../theme";

type ThemeToggleProps = {
  theme: Theme;
  onChange: (theme: Theme) => void;
};

export function ThemeToggle({ theme, onChange }: ThemeToggleProps) {
  const next: Theme = theme === "dark" ? "light" : "dark";
  return (
    <button
      type="button"
      onClick={() => onChange(next)}
      title={`Switch to ${next} theme`}
      className="mono inline-flex items-center gap-2 border border-border-strong bg-transparent px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-text hover:bg-surface-alt"
    >
      <span aria-hidden>{theme === "dark" ? "☾" : "☀"}</span>
      <span>{theme === "dark" ? "Dark" : "Light"}</span>
    </button>
  );
}
