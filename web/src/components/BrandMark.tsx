type BrandMarkProps = {
  /** Font-size tier. `xs` for sidebar eyebrows, `md` for titles / login. */
  size?: "xs" | "sm" | "md" | "lg";
  className?: string;
};

const SIZE_CLASSES: Record<NonNullable<BrandMarkProps["size"]>, { text: string; gap: string }> = {
  xs: { text: "text-[11px]", gap: "gap-1" },
  sm: { text: "text-sm", gap: "gap-1.5" },
  md: { text: "text-base", gap: "gap-2" },
  lg: { text: "text-lg", gap: "gap-2" },
};

/**
 * HostForge wordmark: orange ✦ sparkle glyph + mono "HostForge" label.
 * Matches the marketing site navbar brand.
 */
export function BrandMark({ size = "md", className = "" }: BrandMarkProps) {
  const cfg = SIZE_CLASSES[size];
  return (
    <span
      className={`inline-flex items-center ${cfg.gap} mono ${cfg.text} font-semibold tracking-tight text-text ${className}`}
    >
      <span className="text-primary" aria-hidden>
        ✦
      </span>
      <span>HostForge</span>
    </span>
  );
}
