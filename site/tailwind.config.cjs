/** @type {import('tailwindcss').Config} */
module.exports = {
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
        danger: "rgb(var(--hf-danger-rgb) / <alpha-value>)",
        warning: "var(--hf-warning)",
        info: "var(--hf-info)",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "SFMono-Regular", "Menlo", "monospace"],
      },
      borderRadius: {
        none: "0px",
      },
      boxShadow: {
        dashboard: "var(--shadow-dashboard)",
      },
    },
  },
  plugins: [require("@tailwindcss/typography"), require("tailwindcss-animate")],
};
