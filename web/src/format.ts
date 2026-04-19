export function formatDate(raw?: string | null): string {
  if (!raw) {
    return "never";
  }
  const ts = Date.parse(raw);
  if (Number.isNaN(ts)) {
    return raw;
  }
  return new Date(ts).toLocaleString();
}

export function formatRelative(raw?: string | null, now: Date = new Date()): string {
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
  const sec = Math.floor(diffMs / 1000);
  if (sec < 45) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const d = Math.floor(hr / 24);
  if (d < 30) return `${d}d ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo}mo ago`;
  const y = Math.floor(d / 365);
  return `${y}y ago`;
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
