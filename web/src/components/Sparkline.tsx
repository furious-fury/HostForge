type SparklineProps = {
  values: number[];
  width?: number;
  height?: number;
  min?: number;
  max?: number;
  className?: string;
  strokeClassName?: string;
};

export function Sparkline({
  values,
  width = 80,
  height = 24,
  min: minOverride,
  max: maxOverride,
  className = "",
  strokeClassName = "stroke-primary",
}: SparklineProps) {
  const filtered = values.filter((v) => Number.isFinite(v));
  if (filtered.length === 0) {
    return (
      <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className={className} aria-hidden>
        <line x1={0} y1={height / 2} x2={width} y2={height / 2} className="stroke-border" strokeWidth={1} />
      </svg>
    );
  }
  let lo = minOverride ?? Math.min(...filtered);
  let hi = maxOverride ?? Math.max(...filtered);
  if (hi - lo < 1e-9) {
    lo -= 1;
    hi += 1;
  }
  const pad = 2;
  const w = width - pad * 2;
  const h = height - pad * 2;
  const n = filtered.length;
  const pts: string[] = [];
  for (let i = 0; i < n; i++) {
    const x = pad + (n === 1 ? w / 2 : (i / (n - 1)) * w);
    const t = (filtered[i] - lo) / (hi - lo);
    const y = pad + (1 - t) * h;
    pts.push(`${x.toFixed(2)},${y.toFixed(2)}`);
  }
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className={className} aria-hidden>
      <polyline fill="none" className={strokeClassName} strokeWidth={1.5} strokeLinejoin="round" points={pts.join(" ")} />
    </svg>
  );
}
