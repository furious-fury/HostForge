import { useEffect, useRef } from "react";
import { useSiteTheme } from "../hooks/useSiteTheme";

type ProseProps = {
  html: string;
};

const sharedClasses = [
  "prose max-w-none font-sans",

  "prose-headings:font-mono prose-headings:font-semibold prose-headings:tracking-tight prose-headings:text-text",
  "prose-h2:mt-10 prose-h2:mb-4 prose-h2:text-2xl",
  "prose-h3:mt-8 prose-h3:mb-3 prose-h3:text-lg",

  "prose-p:text-text prose-li:text-text",
  "prose-strong:text-text",
  "prose-a:text-primary prose-a:no-underline hover:prose-a:underline",
  "prose-blockquote:border-l-primary prose-blockquote:text-muted",
  "prose-hr:border-border",

  "prose-code:font-mono prose-code:text-[0.875em] prose-code:text-text",
  "prose-code:border prose-code:border-border prose-code:bg-surface-alt prose-code:px-1.5 prose-code:py-0.5",
  "prose-code:before:content-none prose-code:after:content-none",
  "prose-code:font-normal",

  "prose-pre:border prose-pre:border-border prose-pre:bg-surface-alt",
  "prose-pre:text-text prose-pre:font-mono prose-pre:text-[0.85rem] prose-pre:leading-relaxed",
  "prose-pre:px-4 prose-pre:py-3 prose-pre:overflow-x-auto",

  "prose-table:border-collapse prose-table:text-sm",
  "prose-th:border prose-th:border-border prose-th:bg-surface-alt prose-th:text-left prose-th:text-text",
  "prose-td:border prose-td:border-border prose-td:text-text",
];

const COPY_BUTTON_CLASS = [
  "hf-copy-btn",
  "absolute",
  "top-2",
  "right-2",
  "z-10",
  "border",
  "border-border",
  "bg-surface",
  "px-2",
  "py-1",
  "font-mono",
  "text-[10px]",
  "font-semibold",
  "uppercase",
  "tracking-wider",
  "text-muted",
  "opacity-0",
  "transition-opacity",
  "duration-150",
  "hover:text-text",
  "focus:opacity-100",
  "focus:outline-none",
  "focus:ring-1",
  "focus:ring-primary",
].join(" ");

/** CSS selector class added to each `<pre>` so our delegated listener can find it. */
const PRE_GROUP_CLASS = "hf-code-block";

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to textarea fallback
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

export function Prose({ html }: ProseProps) {
  const { preference } = useSiteTheme();
  const isDark = preference === "dark";
  const variant = isDark ? "prose-invert" : "prose-neutral";
  const articleRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const root = articleRef.current;
    if (!root) return;

    const wrappers: HTMLDivElement[] = [];

    root.querySelectorAll<HTMLPreElement>("pre").forEach((pre) => {
      if (pre.dataset.hfCopy === "1") return;
      pre.dataset.hfCopy = "1";

      const wrapper = document.createElement("div");
      wrapper.className = `${PRE_GROUP_CLASS} group relative my-6`;
      pre.parentNode?.insertBefore(wrapper, pre);
      wrapper.appendChild(pre);
      pre.style.marginTop = "0";
      pre.style.marginBottom = "0";

      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = `${COPY_BUTTON_CLASS} group-hover:opacity-100`;
      btn.setAttribute("aria-label", "Copy code to clipboard");
      btn.dataset.hfCopyBtn = "1";
      btn.textContent = "Copy";
      wrapper.appendChild(btn);

      wrappers.push(wrapper);
    });

    const onClick = async (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      const btn = target?.closest<HTMLButtonElement>("button[data-hf-copy-btn='1']");
      if (!btn) return;
      const wrapper = btn.closest(`.${PRE_GROUP_CLASS}`);
      const pre = wrapper?.querySelector("pre");
      if (!pre) return;
      const code = pre.querySelector("code");
      const text = (code ?? pre).textContent ?? "";
      const ok = await copyText(text);
      const prev = btn.textContent;
      btn.textContent = ok ? "Copied!" : "Failed";
      btn.classList.add("text-text");
      window.setTimeout(() => {
        btn.textContent = prev || "Copy";
        btn.classList.remove("text-text");
      }, 1400);
    };

    root.addEventListener("click", onClick);
    return () => {
      root.removeEventListener("click", onClick);
      wrappers.forEach((wrapper) => {
        const pre = wrapper.querySelector("pre");
        if (pre) {
          pre.style.marginTop = "";
          pre.style.marginBottom = "";
          delete (pre as HTMLPreElement).dataset.hfCopy;
          wrapper.parentNode?.insertBefore(pre, wrapper);
        }
        wrapper.remove();
      });
    };
  }, [html]);

  return (
    <article
      ref={articleRef}
      className={[variant, ...sharedClasses].join(" ")}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
