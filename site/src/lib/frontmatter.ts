import { parse } from "yaml";

const DOC_RE = /^---\r?\n([\s\S]*?)\r?\n---\r?\n([\s\S]*)$/;

/**
 * Split YAML frontmatter + markdown body (same shape as gray-matter, no Node Buffer).
 */
export function splitFrontmatter(raw: string): { data: unknown; body: string } {
  const m = raw.match(DOC_RE);
  if (!m) {
    throw new Error("Expected doc to start with ---\\n YAML frontmatter \\n---\\n");
  }
  let data: unknown;
  try {
    data = parse(m[1]);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    throw new Error(`Invalid YAML frontmatter: ${msg}`);
  }
  return { data, body: m[2] };
}
