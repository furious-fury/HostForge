/**
 * Shared SEO constants and helpers for the marketing + docs site.
 *
 * `SITE_URL` mirrors the sitemap/llms build scripts. Set `VITE_SITE_URL` in the
 * build environment to override the public origin for OG/canonical links.
 */

const rawSiteUrl =
  (typeof import.meta !== "undefined" && (import.meta as { env?: Record<string, string> }).env?.VITE_SITE_URL) ||
  (typeof process !== "undefined" && process.env?.SITE_URL) ||
  "https://hostforge.example";

export const SITE_URL = rawSiteUrl.replace(/\/$/, "");
export const SITE_NAME = "HostForge";
export const SITE_TAGLINE = "Self-hosted PaaS from Git";
export const SITE_DESCRIPTION =
  "HostForge is a self-hosted PaaS that ships your Git repos to Docker on a single Linux host. Git → Nixpacks → Docker with Caddy zero-downtime cutover, live logs, and first-class observability.";

export const DEFAULT_KEYWORDS = [
  "self-hosted PaaS",
  "hostforge",
  "docker deployment",
  "nixpacks",
  "caddy",
  "zero-downtime deploy",
  "heroku alternative",
  "render alternative",
  "git deploy",
  "single host PaaS",
  "self hosting",
].join(", ");

export const TWITTER_HANDLE = "@hostforge";
export const OG_IMAGE_PATH = "/og-cover.png";
export const OG_IMAGE_WIDTH = 1200;
export const OG_IMAGE_HEIGHT = 630;

/** Build an absolute URL from a path relative to the site root. */
export function absoluteUrl(pathname: string): string {
  if (!pathname) return SITE_URL;
  if (/^https?:\/\//i.test(pathname)) return pathname;
  const p = pathname.startsWith("/") ? pathname : `/${pathname}`;
  return `${SITE_URL}${p}`;
}

export function ogImageUrl(): string {
  return absoluteUrl(OG_IMAGE_PATH);
}
