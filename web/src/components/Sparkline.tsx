type SparklineProps = {
  values: number[];
  width?: number;
  height?: number;
  min?: number;
  max?: number;
  className?: string;
  strokeClassName?: string;
  showGrid?: boolean;
};

export function Sparkline({
  values,
  width = 80,
  height = 24,
  min: minOverride,
  max: maxOverride,
  className = "",
  strokeClassName = "stroke-primary",
  showGrid = false,
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
      {showGrid ? (
        <>
          <line x1={pad} y1={pad + h * 0.25} x2={width - pad} y2={pad + h * 0.25} className="stroke-border" strokeDasharray="2 3" strokeWidth={0.75} />
          <line x1={pad} y1={pad + h * 0.75} x2={width - pad} y2={pad + h * 0.75} className="stroke-border" strokeDasharray="2 3" strokeWidth={0.75} />
        </>
      ) : null}
      <polyline fill="none" className={strokeClassName} strokeWidth={2} strokeLinejoin="round" points={pts.join(" ")} />
      <circle cx={pts.at(-1)?.split(",")[0]} cy={pts.at(-1)?.split(",")[1]} r={1.75} className={strokeClassName.replace("stroke-", "fill-")} />
    </svg>
  );
}
