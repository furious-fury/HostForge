function apiFetch(input: RequestInfo | URL, init: RequestInit = {}) {
  return fetch(input, { credentials: "same-origin", ...init });
}

export type HostMemSample = {
  total_bytes: number;
  used_bytes: number;
  available_bytes: number;
  buffers_cached_bytes: number;
  swap_total_bytes: number;
  swap_used_bytes: number;
  used_pct: number;
};

export type HostNetSample = {
  iface: string;
  rx_bps: number;
  tx_bps: number;
};

export type HostDiskUsage = {
  mount: string;
  fs_type: string;
  total_bytes: number;
  used_bytes: number;
  avail_bytes: number;
  used_pct: number;
};

export type HostDiskIOSample = {
  device: string;
  read_bps: number;
  write_bps: number;
  busy_pct: number;
};

export type HostSample = {
  at: string;
  cpu_pct: number;
  /** May be null/omitted in JSON when empty; always use helpers below for iteration. */
  per_core_pct?: number[] | null;
  load_avg?: [number, number, number] | number[] | null;
  mem?: HostMemSample | null;
  net?: HostNetSample[] | null;
  disks?: HostDiskUsage[] | null;
  disk_io?: HostDiskIOSample[] | null;
  uptime_seconds: number;
  rates_ready: boolean;
  err?: string;
};

export function hostNetIfaces(s: HostSample): HostNetSample[] {
  const n = s.net;
  return Array.isArray(n) ? n : [];
}

export function hostDiskMounts(s: HostSample): HostDiskUsage[] {
  const d = s.disks;
  return Array.isArray(d) ? d : [];
}

export function hostDiskIO(s: HostSample): HostDiskIOSample[] {
  const d = s.disk_io;
  return Array.isArray(d) ? d : [];
}

export function hostPerCorePct(s: HostSample): number[] {
  const p = s.per_core_pct;
  return Array.isArray(p) ? p : [];
}

export function hostLoadAvg(s: HostSample): [number, number, number] {
  const L = s.load_avg;
  if (Array.isArray(L) && L.length >= 3) {
    return [Number(L[0]) || 0, Number(L[1]) || 0, Number(L[2]) || 0];
  }
  return [0, 0, 0];
}

const zeroMem = (): HostMemSample => ({
  total_bytes: 0,
  used_bytes: 0,
  available_bytes: 0,
  buffers_cached_bytes: 0,
  swap_total_bytes: 0,
  swap_used_bytes: 0,
  used_pct: 0,
});

export function hostMem(s: HostSample): HostMemSample {
  return s.mem ?? zeroMem();
}

export type HostSnapshot = {
  supported: boolean;
  error_code?: string;
  sample?: HostSample;
  /** Present when server returns 503 warming_up */
  warming?: boolean;
};

export type HostHistory = {
  supported: boolean;
  error_code?: string;
  samples: HostSample[];
};

export async function fetchHostSnapshot(): Promise<HostSnapshot | null> {
  try {
    const res = await apiFetch("/api/system/host/snapshot");
    const text = await res.text();
    let parsed: Record<string, unknown> = {};
    if (text.trim() !== "") {
      try {
        parsed = JSON.parse(text) as Record<string, unknown>;
      } catch {
        return null;
      }
    }
    if (res.status === 503) {
      return { supported: true, warming: true };
    }
    if (!res.ok) {
      if (parsed.error && typeof parsed.error === "string") {
        throw new Error(parsed.error);
      }
      return null;
    }
    return parsed as unknown as HostSnapshot;
  } catch {
    return null;
  }
}

export async function fetchHostHistory(points: number): Promise<HostHistory | null> {
  try {
    const res = await apiFetch(`/api/system/host/history?points=${encodeURIComponent(String(points))}`);
    const text = await res.text();
    if (!res.ok) {
      return null;
    }
    if (text.trim() === "") {
      return { supported: true, samples: [] };
    }
    return JSON.parse(text) as HostHistory;
  } catch {
    return null;
  }
}
