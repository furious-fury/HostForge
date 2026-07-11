/** Maximum number of trailing characters from `prev` to consider when looking for overlap. */
const MAX_OVERLAP_CHECK = 256 * 1024;

/**
 * Merge an incoming log payload onto the existing buffer, dropping any prefix of `incoming`
 * that already appears as a suffix of `prev`. Used after a WebSocket reconnect, when the
 * server resends a rolling tail that may overlap with what the client has already rendered.
 *
 * Linear-time worst case is bounded by `MAX_OVERLAP_CHECK` (server tail is also bounded).
 */
export function mergeLogOverlap(prev: string, incoming: string): string {
  if (!prev) return incoming;
  if (!incoming) return prev;
  const maxOverlap = Math.min(prev.length, incoming.length, MAX_OVERLAP_CHECK);
  for (let len = maxOverlap; len > 0; len--) {
    if (prev.endsWith(incoming.slice(0, len))) {
      return prev + incoming.slice(len);
    }
  }
  return prev + incoming;
}
