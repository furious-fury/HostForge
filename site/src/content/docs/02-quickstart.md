---
title: Quickstart
description: Local dev server, golden-path CLI deploy, and where artifacts land on disk.
slug: quickstart
group: Getting Started
order: 3
---

## Local development (summary)

1. Install **Go 1.22+**, **Git**, **Nixpacks**, **Docker**.
2. Build the CLI: `go build -o hostforge ./cmd/cli`
3. Copy `scripts/hostforge-server.env.example`, set required secrets and **`HOSTFORGE_DATA_DIR`**, export env vars.
4. Build the UI: `npm --prefix web install && npm --prefix web run build`
5. Run the server: `go run ./cmd/server -data-dir "$HOSTFORGE_DATA_DIR" -listen "${HOSTFORGE_LISTEN:-:8080}"`
6. UI hot reload: `npm --prefix web run dev` (Vite proxies `/api`, `/hooks`, `/auth` to the Go server).

## Golden path (CLI)

Use a small public Node app with a root `package.json`:

```bash
go run ./cmd/cli deploy https://github.com/heroku/node-js-getting-started
```

**Expected:** Git clone progress on stderr, Nixpacks logs on stdout/stderr, exit code **0**, and artifacts under `.hostforge/builds/<hash>/` (or under your configured data directory).

Deploy builds a Docker image with tags like `hostforge/<worktree-slug>:<utc-build-id>` and runs a container with a published host port.

## Data layout

Under your data directory (default `./.hostforge`):

- **Worktrees:** `<data-dir>/worktrees/<hash>/`
- **Nixpacks output:** `<data-dir>/builds/<hash>/`
- **SQLite:** `<data-dir>/hostforge.db` (after server/CLI has initialized the DB)

## Next steps

- [Architecture](/docs/architecture) — how CLI, server, Docker, and Caddy fit together.
- [CLI reference](/docs/cli-reference) — all subcommands and flags.
