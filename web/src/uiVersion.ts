declare const __HOSTFORGE_VERSION__: string;

/** Semver from `internal/version/VERSION` (injected at Vite build / dev start). */
export const RELEASE_SEMVER =
  typeof __HOSTFORGE_VERSION__ !== "undefined" && __HOSTFORGE_VERSION__
    ? __HOSTFORGE_VERSION__
    : "0.0.0-dev";

/** Matches Go `version.Display()` for sidebar chrome. */
export const RELEASE_LABEL = `v${RELEASE_SEMVER}`;

/**
 * Prefer the UI bundle release when the server still returns a legacy
 * `… · phase N` string (older binary). Otherwise show the server-reported value.
 */
export function effectiveBuildLabel(serverVersion: string | undefined | null): string {
  const v = (serverVersion ?? "").trim();
  if (!v) return RELEASE_LABEL;
  if (/\bphase\b/i.test(v)) return RELEASE_LABEL;
  return v;
}
