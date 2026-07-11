# Operator guide

HostForge targets a Linux VPS. Keep the management server private and expose
applications only through Caddy HTTPS hostnames.

## Installation

```bash
sudo ./scripts/install.sh --with-systemd
```

The installer creates the service user, data directory, systemd unit, and an
environment-file template when one does not already exist. Configure secrets in
`/etc/hostforge/hostforge.env`; never commit that file.

See [`scripts/hostforge-server.env.example`](../scripts/hostforge-server.env.example)
for the configuration reference, including disabled-by-default Railpack/BuildKit
settings.

## Network boundary

- Allow public inbound TCP 80 and 443 for Caddy.
- Keep HostForge on `127.0.0.1:8080` unless an operator deliberately places it
  behind Caddy or an authenticated private network.
- Containers bind only to loopback ports. Never share raw Docker host ports as
  application URLs.
- Point platform or custom DNS hostnames at the VPS before Caddy can obtain
  certificates.

## Operations

- Use `systemctl status hostforge-server` and `journalctl -u hostforge-server`
  for service diagnostics.
- Treat the SQLite database, generated Caddy config, and environment file as
  sensitive backup inputs.
- Follow the [phase 1 validation runbook](./operator-validation-phase1.md) on a
  clean VPS before relying on a deployment path.
- Do not enable `HOSTFORGE_RAILPACK_ENABLED=true` until the Railpack/BuildKit
  smoke test in [ADR 0001](./adr-0001-railpack-buildkit.md) passes.
