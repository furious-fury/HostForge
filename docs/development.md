# Local development

The product surface is `cmd/server` plus the browser UI. There is no separate operator CLI.

## Prerequisites

- Go 1.25+
- Git, Docker Engine, Railpack, `buildctl`, and a running BuildKit daemon
- Node.js/npm for `web/` and `site/`

Copy `scripts/hostforge-server.env.example` to an untracked environment file. Configure management/session/webhook secrets and the required digest-pinned Railpack/BuildKit settings.

## Run

```bash
npm --prefix web install
npm --prefix web run dev
```

In another terminal:

```bash
go run ./cmd/server -data-dir ./.hostforge -listen 127.0.0.1:8080
```

Vite proxies `/api`, `/auth`, and `/hooks` to the Go server.

## Verify

```bash
go test ./...
npm --prefix web test
npm --prefix web run lint
npm --prefix web run build
npm --prefix site run build
```
