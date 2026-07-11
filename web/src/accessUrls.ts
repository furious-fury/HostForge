import type { ApiProject } from "./api";

export type AccessLink = {
  href: string;
  label: string;
  kind: "domain" | "direct";
};

function domainScheme(ssl: string): "http" | "https" {
  const s = (ssl || "").toUpperCase();
  if (s === "ERROR") {
    return "http";
  }
  return "https";
}

/** Resolves where traffic can reach this project (Caddy hostnames + loopback bind). */
export function projectAccessLinks(project: ApiProject | null): AccessLink[] {
  if (!project) {
    return [];
  }
  const out: AccessLink[] = [];
  for (const d of project.domains || []) {
    const scheme = domainScheme(d.ssl_status);
    const href = `${scheme}://${d.domain_name}`;
    out.push({ href, label: d.domain_name, kind: "domain" });
  }
  const c = project.current_container;
  const hp = c?.host_port;
  if (hp && hp > 0) {
    const href = `http://127.0.0.1:${hp}`;
    out.push({
      href,
      label: `${href} (direct)`,
      kind: "direct",
    });
  }
  return out;
}

/** One-line summary for fleet tables (hostnames or loopback, comma-separated). */
export function projectReachSummary(project: ApiProject | null): string {
  const links = projectAccessLinks(project);
  if (links.length === 0) {
    return "—";
  }
  return links
    .map((l) => (l.kind === "domain" ? l.label : l.href.replace(/^https?:\/\//, "")))
    .join(", ");
}
