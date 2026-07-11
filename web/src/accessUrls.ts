import type { ApiProject } from "./api";

export type AccessLink = {
  href: string;
  label: string;
  kind: "domain";
};

function domainScheme(ssl: string): "http" | "https" {
  const s = (ssl || "").toUpperCase();
  if (s === "ERROR") {
    return "http";
  }
  return "https";
}

/** Resolves public Caddy hostnames for a project. Container ports stay private. */
export function projectAccessLinks(project: ApiProject | null): AccessLink[] {
  if (!project) {
    return [];
  }
  const out: AccessLink[] = [];
  if (project.default_url) {
    out.push({ href: project.default_url, label: project.default_url.replace(/^https:\/\//, ""), kind: "domain" });
  }
  for (const d of project.domains || []) {
    if (project.default_url === `https://${d.domain_name}`) continue;
    const scheme = domainScheme(d.ssl_status);
    const href = `${scheme}://${d.domain_name}`;
    out.push({ href, label: d.domain_name, kind: "domain" });
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
    .map((l) => l.label)
    .join(", ");
}
