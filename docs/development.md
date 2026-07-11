# Local development

The normal product surface is `cmd/server` plus the browser UI. The legacy CLI
remains only during migration and should not receive new features.

## Prerequisites

- Go 1.25+
- Git and Docker Engine
- Node.js/npm for `web/` and `site/`

The default legacy deploy path also needs Nixpacks. Railpack is disabled unless
explicitly configured and validated on Linux.

## Run the control plane

1. Copy [`scripts/hostforge-server.env.example`](../scripts/hostforge-server.env.example)
   to an untracked environment file and set its required secrets.
2. Build the UI:

   ```bash
   npm --prefix web install
   npm --prefix web run build
   ```

3. Start the server:

   ```bash
   go run ./cmd/server -data-dir ./.hostforge -listen 127.0.0.1:8080
   ```

4. For UI hot reload, run `npm --prefix web run dev` in another terminal.

Vite proxies `/api`, `/auth`, and `/hooks` to the Go server in development.

## Verification

```bash
go test ./...
npm --prefix web run build
npm --prefix site run build
```

The npm commands require the respective `node_modules` directory.

## Webhook testing

Use [`scripts/ngrok-dev.sh`](../scripts/ngrok-dev.sh) when GitHub must reach a
loopback-bound local server. It is development-only; Caddy HTTPS hostnames are
the intended application ingress.
