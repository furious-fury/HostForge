import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { splitFrontmatter } from "../src/lib/frontmatter.ts";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const siteRoot = path.resolve(__dirname, "..");
const srcDir = path.join(siteRoot, "src/content/docs");

const base = (process.env.SITE_URL ?? "https://hostforge.example").replace(/\/$/, "");

const urls: string[] = [`${base}/`];

for (const file of fs.readdirSync(srcDir).filter((f) => f.endsWith(".md"))) {
  const raw = fs.readFileSync(path.join(srcDir, file), "utf8");
  const slug = (splitFrontmatter(raw).data as { slug?: string }).slug;
  if (!slug) throw new Error(`build-sitemap: missing slug in ${file}`);
  urls.push(`${base}/docs/${slug}`);
}

const xml =
  `<?xml version="1.0" encoding="UTF-8"?>\n` +
  `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n` +
  urls.map((loc) => `  <url><loc>${escapeXml(loc)}</loc></url>`).join("\n") +
  `\n</urlset>\n`;

fs.writeFileSync(path.join(siteRoot, "dist/sitemap.xml"), xml, "utf8");
console.log("build-sitemap: wrote dist/sitemap.xml with", urls.length, "URLs (SITE_URL=", base, ")");

const robots =
  `User-agent: *\n` +
  `Allow: /\n\n` +
  `Sitemap: ${base}/sitemap.xml\n\n` +
  `# Agent-readable docs index (see https://llmstxt.org/)\n` +
  `# llms.txt: ${base}/llms.txt\n`;
fs.writeFileSync(path.join(siteRoot, "dist/robots.txt"), robots, "utf8");
console.log("build-sitemap: wrote dist/robots.txt");

function escapeXml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}
