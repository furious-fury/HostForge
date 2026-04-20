# HostForge marketing + docs site

Static **Vite + React + TypeScript + Tailwind** app in `site/`, separate from the control-plane UI in `web/`. **Visuals match the dashboard:** **`#f59e0b` / `#d97706` (light)** primary, **Inter** + **JetBrains Mono**, **no border radius**, and **light / dark** only via **`hf-prefs.theme`** (stored `system` from the control plane is shown as **dark** here until you toggle). Theme control is in the navbar and docs header.

## Commands

```bash
npm install
npm run dev      # vite-react-ssg dev (SSR in dev)
npm run build    # SSG + copy raw .md + sitemap + llms.txt
npm run preview  # vite preview (serves dist/)
```

## Markdown rendering

Docs HTML is produced with **remark → rehype → rehype-stringify** (GFM, heading slugs, autolinked headings). Syntax highlighting is intentionally plain `<pre><code>` to keep the client bundle smaller than pulling **Shiki** into the browser.

Frontmatter is parsed with the **`yaml`** package (browser-safe). **`gray-matter` is not used** in the client bundle because it depends on Node’s `Buffer`.

## Optional hero video

Place a looping **`public/demo.mp4`** (muted H.264) for the landing background. If the file is missing or fails to load, the page falls back to a light gradient only.

## Absolute URLs in sitemap / llms

Post-build scripts use **`SITE_URL`** (no trailing slash), defaulting to `https://hostforge.example` if unset:

```bash
SITE_URL=https://docs.yourdomain.com npm run build
```

## Deploying `dist/`

`dist/` is a static bundle suitable for any static host or `file_server` behind Caddy. Not wired into `hostforge-server` by default.

Artifacts for AI agents:

- `/docs/<slug>.md` — raw Markdown (with frontmatter), copied from `src/content/docs/*.md`
- `/llms.txt` — short index with links
- `/llms-full.txt` — concatenated doc bodies (no frontmatter)
- `/sitemap.xml` — URLs derived from slugs
