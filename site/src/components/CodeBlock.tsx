import { useCallback, useEffect, useRef, useState } from "react";

type CodeBlockProps = {
  code: string;
  /** Optional mono eyebrow above the block (e.g. "bash", "hostforge", "env"). */
  language?: string;
  /** Optional title shown to the left of the language chip. */
  title?: string;
  className?: string;
};

type CopyState = "idle" | "copied" | "failed";

async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "absolute";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function CodeBlock({ code, language, title, className = "" }: CodeBlockProps) {
  const [state, setState] = useState<CopyState>("idle");
  const resetRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (resetRef.current !== null) {
        window.clearTimeout(resetRef.current);
      }
    };
  }, []);

  const onCopy = useCallback(async () => {
    const ok = await writeClipboard(code);
    setState(ok ? "copied" : "failed");
    if (resetRef.current !== null) {
      window.clearTimeout(resetRef.current);
    }
    resetRef.current = window.setTimeout(() => setState("idle"), 1400);
  }, [code]);

  const label = state === "copied" ? "Copied!" : state === "failed" ? "Failed" : "Copy";

  return (
    <div className={`group relative ${className}`}>
      {(language || title) && (
        <div className="flex items-center justify-between border border-b-0 border-border bg-surface px-3 py-1.5">
          <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
            {title ?? language}
          </span>
          {language && title ? (
            <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-muted">
              {language}
            </span>
          ) : null}
        </div>
      )}
      <pre className="overflow-x-auto border border-border bg-surface-alt px-4 py-3 font-mono text-[0.85rem] leading-relaxed text-text">
        <code>{code}</code>
      </pre>
      <button
        type="button"
        onClick={onCopy}
        aria-label="Copy code to clipboard"
        className={`absolute right-2 z-10 border border-border bg-surface px-2 py-1 font-mono text-[10px] font-semibold uppercase tracking-wider transition-opacity duration-150 hover:text-text focus:opacity-100 focus:outline-none focus:ring-1 focus:ring-primary ${
          language || title ? "top-10" : "top-2"
        } ${state === "idle" ? "text-muted opacity-0 group-hover:opacity-100" : "text-text opacity-100"}`}
      >
        {label}
      </button>
    </div>
  );
}
