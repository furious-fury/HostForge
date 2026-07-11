import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { splitFrontmatter } from "../src/lib/frontmatter.ts";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const siteRoot = path.resolve(__dirname, "..");
const srcDir = path.join(siteRoot, "src/content/docs");
const distDocs = path.join(siteRoot, "dist/docs");

if (!fs.existsSync(path.join(siteRoot, "dist"))) {
  console.error("site/scripts/copy-raw-md: dist/ not found — run vite-react-ssg build first.");
  process.exit(1);
}

fs.mkdirSync(distDocs, { recursive: true });

for (const file of fs.readdirSync(srcDir).filter((f) => f.endsWith(".md"))) {
  const full = path.join(srcDir, file);
  const raw = fs.readFileSync(full, "utf8");
  const { data } = splitFrontmatter(raw);
  const slug = (data as { slug?: string }).slug;
  if (!slug) {
    throw new Error(`copy-raw-md: missing slug in ${file}`);
  }
  fs.writeFileSync(path.join(distDocs, `${slug}.md`), raw, "utf8");
}

console.log("copy-raw-md: wrote", fs.readdirSync(distDocs).filter((f) => f.endsWith(".md")).length, "files to dist/docs/");
