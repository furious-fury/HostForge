export function formatDate(raw?: string | null, locale?: string): string {
  if (!raw) {
    return "never";
  }
  const ts = Date.parse(raw);
  if (Number.isNaN(ts)) {
    return raw;
  }
  return new Date(ts).toLocaleString(locale && locale.trim() !== "" ? locale : undefined);
}

export function formatRelative(raw?: string | null, now: Date = new Date(), locale?: string): string {
  if (!raw) {
    return "never";
  }
  const ts = Date.parse(raw);
  if (Number.isNaN(ts)) {
    return raw;
  }
  const diffMs = now.getTime() - ts;
  if (diffMs < 0) {
    return "just now";
  }
  const rtf =
    typeof Intl !== "undefined" && Intl.RelativeTimeFormat
      ? new Intl.RelativeTimeFormat(locale && locale.trim() !== "" ? locale : undefined, { numeric: "auto" })
      : null;
  const sec = Math.floor(diffMs / 1000);
  if (sec < 45) return rtf ? rtf.format(-sec, "second") : `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return rtf ? rtf.format(-min, "minute") : `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return rtf ? rtf.format(-hr, "hour") : `${hr}h ago`;
  const d = Math.floor(hr / 24);
  if (d < 30) return rtf ? rtf.format(-d, "day") : `${d}d ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return rtf ? rtf.format(-mo, "month") : `${mo}mo ago`;
  const y = Math.floor(d / 365);
  return rtf ? rtf.format(-y, "year") : `${y}y ago`;
}

/** Resolve UI numeric/date locale from prefs (`en-US` or browser). */
export function resolveFormatLocale(pref: "en-US" | "system"): string {
  if (pref === "system" && typeof navigator !== "undefined" && navigator.language) {
    return navigator.language;
  }
  return "en-US";
}

export function formatDuration(startRaw?: string | null, endRaw?: string | null): string {
  if (!startRaw) {
    return "—";
  }
  const start = Date.parse(startRaw);
  if (Number.isNaN(start)) {
    return "—";
  }
  const end = endRaw ? Date.parse(endRaw) : Date.now();
  if (Number.isNaN(end)) {
    return "—";
  }
  const diff = Math.max(0, end - start);
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  const remSec = sec % 60;
  if (min < 60) return `${min}m ${remSec}s`;
  const hr = Math.floor(min / 60);
  const remMin = min % 60;
  return `${hr}h ${remMin}m`;
}

export function shortHash(raw?: string | null, length = 7): string {
  if (!raw) {
    return "—";
  }
  return raw.slice(0, length);
}
