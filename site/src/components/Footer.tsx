export function Footer() {
  return (
    <footer className="relative z-20 shrink-0 border-t border-border bg-bg/90 px-6 py-3 text-center font-mono text-[10px] uppercase tracking-wide text-muted backdrop-blur md:px-12">
      HostForge — self-hosted on your metal. Raw Markdown for agents at{" "}
      <code className="border border-border bg-surface-alt px-1 py-0.5 text-[10px] normal-case text-text">
        /docs/&lt;slug&gt;.md
      </code>
      .
    </footer>
  );
}
