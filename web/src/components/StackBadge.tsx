import { useEffect, useMemo, useState } from "react";

/** Nixpacks-derived stack slug from the server (`stack_kind`); open set (matches NIXPACKS_METADATA, e.g. `java`, `c#`, `node_vite`). */
export type StackKindSlug = string;

/** Base URL for `public/stack-icons/*` (Vite `BASE_URL` ends with `/`). */
function stackIconsBase(): string {
  const base = import.meta.env.BASE_URL;
  const prefix = base.endsWith("/") ? base : `${base}/`;
  return `${prefix}stack-icons/`;
}

/** One path segment under `stack-icons/` (basename only; encodes `#` etc. for `src`). */
function stackIconAssetUrl(base: string, basename: string, ext: "png" | "svg"): string {
  return `${base}${encodeURIComponent(basename)}.${ext}`;
}

/** Legacy rows: `stack_kind` was `unknown` but `stack_label` still reflects the language (e.g. `Java`). */
function legacyIconBasenamesFromLabel(stackLabel: string): string[] {
  const raw = stackLabel.trim();
  if (!raw) return [];
  const head = raw.split("·")[0]?.trim() ?? raw;
  const slug = head.toLowerCase().replace(/[^a-z0-9]+/g, "");
  if (slug.length < 2 || slug === "unknown") return [];
  return [slug];
}

/**
 * Ordered list of image URLs to try under `web/public/stack-icons/`.
 * PNG first (common for bundled icons), then SVG per basename.
 * Built-in basename aliases (optional files): golang→go, next→node_next, react→node_cra, vite→node_vite, vue→node_nuxt, html5→Staticfile.
 * Always ends with default.*, node.*, then the inline glyph.
 */
function stackIconCandidates(kind: string, stackLabel: string): string[] {
  const b = stackIconsBase();
  const k = (kind || "").toLowerCase() || "unknown";
  const lab = (stackLabel || "").toLowerCase();

  const aliasBasenames: Record<string, string[]> = {
    go: ["golang", "go"],
    node_next: ["next", "node_next"],
    node_cra: ["react", "node_cra"],
    node_vite: ["vite", "node_vite"],
    node_nuxt: ["vue", "node_nuxt"],
  };

  let basenames: string[];
  if (aliasBasenames[k]) {
    basenames = aliasBasenames[k]!;
  } else if ((k === "unknown" && lab.includes("staticfile")) || k === "staticfile") {
    basenames = ["html5", "staticfile", "unknown"];
  } else if (k === "unknown") {
    basenames = ["unknown", ...legacyIconBasenamesFromLabel(stackLabel)];
  } else {
    basenames = [k];
  }

  const urls: string[] = [];
  for (const name of basenames) {
    urls.push(stackIconAssetUrl(b, name, "png"), stackIconAssetUrl(b, name, "svg"));
  }
  urls.push(
    stackIconAssetUrl(b, "default", "png"),
    stackIconAssetUrl(b, "default", "svg"),
    stackIconAssetUrl(b, "node", "png"),
    stackIconAssetUrl(b, "node", "svg"),
  );
  return urls;
}

/** Inline fallback when no asset under `public/stack-icons/` loads (same as previous StackIcon). */
function LegacyStackGlyph({
  kind,
  className = "shrink-0",
  sizePx,
}: {
  kind: string;
  className?: string;
  /** Box size in CSS px; do not use Tailwind `size-*` here or it overrides {@link STACK_ICON_PX}. */
  sizePx: number;
}) {
  const k = (kind || "").toLowerCase();
  const stroke = "currentColor";
  const box = { width: sizePx, height: sizePx } as const;
  const common = {
    className,
    style: box,
    viewBox: "0 0 24 24",
    fill: "none" as const,
    stroke,
    strokeWidth: 2,
    "aria-hidden": true as const,
  };

  switch (k) {
    case "node":
    case "node_spa":
    case "node_next":
    case "node_vite":
    case "node_remix":
    case "node_nuxt":
    case "node_svelte":
    case "node_astro":
    case "node_cra":
      return (
        <svg {...common}>
          <path d="M12 2 3 7v10l9 5 9-5V7l-9-5Z" strokeLinejoin="round" />
          <path d="M12 22V12" />
          <path d="M3 7l9 5 9-5" />
          {k !== "node" && <path d="M8 14h8M8 18h5" strokeLinecap="round" />}
        </svg>
      );
    case "go":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M8 10c1.5-2 6.5-2 8 0M8 14c1.5 2 6.5 2 8 0" strokeLinecap="round" />
        </svg>
      );
    case "python":
      return (
        <svg {...common}>
          <path d="M9 4h6a2 2 0 0 1 2 2v1H9a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h6" strokeLinecap="round" />
          <path d="M15 20H9a2 2 0 0 1-2-2v-1h8a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2H9" strokeLinecap="round" />
        </svg>
      );
    case "ruby":
      return (
        <svg {...common}>
          <path d="M12 3 4 9v6l8 6 8-6V9l-8-6Z" strokeLinejoin="round" />
          <path d="m4 9 8 6 8-6M12 15V3" />
        </svg>
      );
    case "php":
      return (
        <svg {...common}>
          <ellipse cx="12" cy="12" rx="8" ry="4" />
          <path d="M4 12v0M20 12v0" strokeLinecap="round" />
        </svg>
      );
    case "rust":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="3" />
          <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.2 4.2l2.2 2.2M17.6 17.6l2.2 2.2M19.8 4.2l-2.2 2.2M6.4 17.6l-2.2 2.2" strokeLinecap="round" />
        </svg>
      );
    case "deno":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <circle cx="9" cy="10" r="1.2" fill={stroke} stroke="none" />
          <circle cx="15" cy="10" r="1.2" fill={stroke} stroke="none" />
          <path d="M8 15c1.5 1.5 6.5 1.5 8 0" strokeLinecap="round" />
        </svg>
      );
    case "unknown":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M9 9a3 3 0 0 1 5.1 2.1c0 1.5-1.5 1.5-1.5 3M12 17h.01" strokeLinecap="round" />
        </svg>
      );
    default:
      return (
        <svg {...common}>
          <rect x="4" y="4" width="16" height="16" rx="2" />
          <path d="M8 12h8M12 8v8" strokeLinecap="round" />
        </svg>
      );
  }
}

/** Display size for stack icons (CSS px). Applied via inline style so it is not overridden by Tailwind `size-*`. */
const STACK_ICON_PX = 28;

function StackIcon({
  kind,
  stackLabel = "",
  className = "shrink-0",
}: {
  kind: string;
  stackLabel?: string;
  className?: string;
}) {
  const slug = (kind || "unknown").toLowerCase() || "unknown";
  const candidates = useMemo(() => stackIconCandidates(kind, stackLabel), [kind, stackLabel]);
  const [idx, setIdx] = useState(0);

  useEffect(() => {
    setIdx(0);
  }, [kind, stackLabel]);

  const boxStyle = { width: STACK_ICON_PX, height: STACK_ICON_PX } as const;

  if (idx >= candidates.length) {
    return <LegacyStackGlyph kind={slug} className={className} sizePx={STACK_ICON_PX} />;
  }

  return (
    <img
      key={candidates[idx]}
      src={candidates[idx]}
      alt=""
      width={STACK_ICON_PX}
      height={STACK_ICON_PX}
      decoding="async"
      loading="lazy"
      className={`${className} max-w-none object-contain`.trim()}
      style={boxStyle}
      aria-hidden
      onError={() => setIdx((i) => i + 1)}
    />
  );
}

export type StackBadgeProps = {
  stackKind?: string;
  stackLabel?: string;
  /** Icon only; full label in native tooltip. */
  compact?: boolean;
  className?: string;
};

export function StackBadge({ stackKind, stackLabel, compact, className = "" }: StackBadgeProps) {
  const kind = (stackKind || "").toLowerCase();
  const label = (stackLabel || "").trim();
  if (!kind && !label) return null;

  const title = label || kind || "Stack";
  const showText = !compact && !!label;

  return (
    <span
      className={`inline-flex max-w-full items-center gap-1 text-muted ${className}`.trim()}
      title={title}
    >
      <span className="text-text">
        <StackIcon kind={kind || "unknown"} stackLabel={label} />
      </span>
      {showText && <span className="truncate text-[11px] font-medium text-text">{label}</span>}
    </span>
  );
}
