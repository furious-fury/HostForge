import { ReactNode, useEffect, useRef } from "react";

type TerminalProps = {
  toolbar?: ReactNode;
  text: string;
  scrollLocked: boolean;
  emptyMessage?: string;
  height?: string;
};

export function Terminal({ toolbar, text, scrollLocked, emptyMessage = "Awaiting logs...", height = "60vh" }: TerminalProps) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (scrollLocked) return;
    const node = ref.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [text, scrollLocked]);

  return (
    <div className="flex flex-col border border-terminal-border bg-terminal-bg text-terminal-fg">
      {toolbar && (
        <div className="flex flex-wrap items-center gap-2 border-b border-terminal-border bg-surface px-3 py-2 text-text">
          {toolbar}
        </div>
      )}
      <div
        ref={ref}
        className="mono overflow-auto whitespace-pre-wrap px-3 py-3 text-xs leading-5"
        style={{ height }}
      >
        {text || <span className="opacity-60">{emptyMessage}</span>}
      </div>
    </div>
  );
}
