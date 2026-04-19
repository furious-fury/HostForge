/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ["class", '[data-theme="dark"]'],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "var(--hf-bg)",
        surface: "var(--hf-surface)",
        "surface-alt": "var(--hf-surface-alt)",
        border: "var(--hf-border)",
        "border-strong": "var(--hf-border-strong)",
        text: "var(--hf-text)",
        muted: "var(--hf-muted)",
        primary: "var(--hf-primary)",
        "primary-ink": "var(--hf-primary-ink)",
        success: "var(--hf-success)",
        // `<alpha-value>` enables bg-danger/30, text-danger/80, etc. (see --hf-danger-rgb in index.css).
        danger: "rgb(var(--hf-danger-rgb) / <alpha-value>)",
        warning: "var(--hf-warning)",
        info: "var(--hf-info)",
        terminal: "var(--hf-terminal)",
        "terminal-bg": "var(--hf-terminal-bg)",
        "terminal-fg": "var(--hf-terminal-fg)",
        "terminal-border": "var(--hf-terminal-border)",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "SFMono-Regular", "Menlo", "monospace"],
      },
      borderRadius: {
        none: "0px",
      },
    },
  },
  plugins: [],
};
