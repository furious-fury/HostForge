/** Format byte count with KiB/MiB/GiB using locale for grouping. */
export function formatBytes(n: number, locale: string): string {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 1024) return `${Math.round(n)} B`;
  const kb = n / 1024;
  if (kb < 1024) return `${kb.toLocaleString(locale, { maximumFractionDigits: 1 })} KiB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toLocaleString(locale, { maximumFractionDigits: 1 })} MiB`;
  const gb = mb / 1024;
  return `${gb.toLocaleString(locale, { maximumFractionDigits: 2 })} GiB`;
}

/** Format bits per second (e.g. network) as Mb/s or Gb/s. */
export function formatBitsPerSec(bytesPerSec: number, locale: string): string {
  if (!Number.isFinite(bytesPerSec) || bytesPerSec < 0) return "—";
  const bps = bytesPerSec * 8;
  if (bps < 1000) return `${bps.toLocaleString(locale, { maximumFractionDigits: 0 })} b/s`;
  const kbps = bps / 1000;
  if (kbps < 1000) return `${kbps.toLocaleString(locale, { maximumFractionDigits: 1 })} Kb/s`;
  const mbps = kbps / 1000;
  if (mbps < 1000) return `${mbps.toLocaleString(locale, { maximumFractionDigits: 2 })} Mb/s`;
  return `${(mbps / 1000).toLocaleString(locale, { maximumFractionDigits: 2 })} Gb/s`;
}

export function formatPct(n: number, locale: string, digits = 1): string {
  if (!Number.isFinite(n)) return "—";
  return `${n.toLocaleString(locale, { maximumFractionDigits: digits, minimumFractionDigits: 0 })}%`;
}
