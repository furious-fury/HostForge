import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { splitFrontmatter } from "../src/lib/frontmatter.ts";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const siteRoot = path.resolve(__dirname, "..");
const srcDir = path.join(siteRoot, "src/content/docs");

const base = (process.env.SITE_URL ?? "https://hostforge.example").replace(/\/$/, "");

type Entry = { slug: string; title: string; description: string; body: string };

const entries: Entry[] = [];

for (const file of fs.readdirSync(srcDir).filter((f) => f.endsWith(".md"))) {
  const raw = fs.readFileSync(path.join(srcDir, file), "utf8");
  const parsed = splitFrontmatter(raw);
  const data = parsed.data as { slug?: string; title?: string; description?: string };
  if (!data.slug || !data.title || !data.description) {
    throw new Error(`build-llms: bad frontmatter in ${file}`);
  }
  entries.push({
    slug: data.slug,
    title: data.title,
    description: data.description,
    body: parsed.body.trim(),
  });
}

entries.sort((a, b) => a.slug.localeCompare(b.slug));

const llmsTxt = [
  `# HostForge`,
  ``,
  `> Self-hosted PaaS: Git → Nixpacks → Docker on one machine, with API, UI, GitHub webhooks, and Caddy routing.`,
  ``,
  `## Docs`,
  ``,
  ...entries.map((e) => `- [${e.title}](${base}/docs/${e.slug}): ${e.description}`),
  ``,
  `Raw Markdown (includes frontmatter) for agents:`,
  ...entries.map((e) => `- ${base}/docs/${e.slug}.md`),
  ``,
].join("\n");

const llmsFull = entries
  .map((e) => [`# ${e.title}`, `slug: ${e.slug}`, ``, e.body].join("\n"))
  .join("\n\n---\n\n");

fs.writeFileSync(path.join(siteRoot, "dist/llms.txt"), llmsTxt, "utf8");
fs.writeFileSync(path.join(siteRoot, "dist/llms-full.txt"), llmsFull, "utf8");

console.log("build-llms: wrote dist/llms.txt and dist/llms-full.txt");
