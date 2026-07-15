---
title: Quickstart
description: Run the v2 control plane and deploy the first service from the UI.
slug: quickstart
group: Getting Started
order: 3
---

## Local development

1. Copy `scripts/hostforge-server.env.example` to an untracked environment file and set its required values.
2. Start Docker, BuildKit, and Caddy.
3. Install and verify the pinned Railpack helper.
4. Build or start the UI:

```bash
npm --prefix web install
npm --prefix web run dev
```

5. Start the server in another terminal:

```bash
go run ./cmd/server -data-dir ./.hostforge -listen 127.0.0.1:8080
```

Vite proxies `/api`, `/auth`, and `/hooks` to the Go server.

## First deployment

1. Sign in with `HOSTFORGE_API_TOKEN`; the server stores a signed HttpOnly session cookie.
2. Complete GitHub App setup and connect an installation.
3. Create an application. Production and staging are created together.
4. Add a service, select an installation/repository, choose an environment branch, and review build/runtime settings.
5. Deploy explicitly and follow the deployment detail/log stream.
6. Add an environment variable or domain only after selecting its application environment and target service.

The deploy CLI no longer exists. Deployments are authenticated, auditable server operations.
