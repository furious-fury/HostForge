# Optional Caddy certificate poll (§2.1)

`hostforge-server` can periodically update each `domains` row with:

- **`last_cert_message`** — short operator summary (leaf expiry / issuer from on-disk PEM, plus a tiny admin probe note).
- **`cert_checked_at`** — RFC3339 UTC timestamp of the last poll.

## Enable

Set in the server environment (see [README](../README.md) env table):

| Variable | Purpose |
|----------|---------|
| `HOSTFORGE_CADDY_CERT_POLL_INTERVAL_SEC` | Poll interval in seconds; **`0` (default) disables** the feature. |
| `HOSTFORGE_CADDY_ADMIN` | Caddy admin base URL (default `http://127.0.0.1:2019`); HostForge issues **read-only** `GET /config/` (first bytes only). |
| `HOSTFORGE_CADDY_STORAGE_ROOT` | Caddy data directory so HostForge can glob `certificates/*/<hostname>/<hostname>.crt` and parse the **leaf** x509 `NotAfter` / issuer. Use `~` for the service user’s home (expanded at startup). |

## Guarantees (and non-goals)

- **`ssl_status`** remains **route / snippet sync** state (`PENDING` / `ACTIVE` / `ERROR`) from `caddy validate` / `caddy reload`, unchanged by this poll.
- The poll **does not** run ACME, rotate certs, or parse full admin config trees; it avoids duplicating Caddy’s certificate state machine.
- If `HOSTFORGE_CADDY_STORAGE_ROOT` is unset, messages include `storage: unset` so operators know leaf scans are disabled.

## Code

- Poller: [`internal/services/cert_poll.go`](../internal/services/cert_poll.go)
- Schema: [`internal/database/migrations/0002_domain_cert_observability.sql`](../internal/database/migrations/0002_domain_cert_observability.sql)
