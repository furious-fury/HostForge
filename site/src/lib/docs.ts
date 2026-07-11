import { splitFrontmatter } from "./frontmatter";

export type DocMeta = {
  title: string;
  description: string;
  slug: string;
  group: string;
  order: number;
};

export type LoadedDoc = {
  meta: DocMeta;
  /** Full file including frontmatter (for raw copy + llms). */
  raw: string;
  /** Markdown body only. */
  body: string;
};

const rawModules = import.meta.glob<string>("../content/docs/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

function parseDocFile(raw: string): LoadedDoc {
  const parsed = splitFrontmatter(raw);
  const data = parsed.data as Partial<DocMeta>;
  if (!data.title || !data.description || !data.slug || !data.group || typeof data.order !== "number") {
    throw new Error("Invalid docs frontmatter: expected title, description, slug, group, order");
  }
  return {
    meta: {
      title: data.title,
      description: data.description,
      slug: data.slug,
      group: data.group,
      order: data.order,
    },
    raw,
    body: parsed.body,
  };
}

export function loadAllDocs(): LoadedDoc[] {
  const out: LoadedDoc[] = [];
  for (const raw of Object.values(rawModules)) {
    out.push(parseDocFile(raw));
  }
  out.sort((a, b) => {
    const g = a.meta.group.localeCompare(b.meta.group);
    if (g !== 0) return g;
    return a.meta.order - b.meta.order;
  });
  return out;
}

export function getDocBySlug(slug: string): LoadedDoc | undefined {
  return loadAllDocs().find((d) => d.meta.slug === slug);
}

export function getDocsGrouped(): Map<string, LoadedDoc[]> {
  const map = new Map<string, LoadedDoc[]>();
  for (const doc of loadAllDocs()) {
    const g = doc.meta.group;
    const list = map.get(g) ?? [];
    list.push(doc);
    map.set(g, list);
  }
  for (const [g, list] of map) {
    list.sort((a, b) => a.meta.order - b.meta.order);
    map.set(g, list);
  }
  return map;
}

/** Paths for vite-react-ssg `getStaticPaths` (no leading slash). */
export function getStaticDocPaths(): string[] {
  return loadAllDocs().map((d) => `docs/${d.meta.slug}`);
}
