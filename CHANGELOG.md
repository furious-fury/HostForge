# Changelog

All notable changes to HostForge are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/): `0.MINOR.PATCH`
while pre-1.0 (ADR-0002 §24.1) — a `MINOR` bump may still include breaking
changes, since the API has not yet reached 1.0 stability.

## [Unreleased]

## [0.8.0] - 2026-08-30

### Added

- `-version` CLI flag on `hostforge-server`, and an unauthenticated
  `GET /api/version` endpoint, both reporting the release version, commit,
  and build time.
- Downgrade protection: the server now refuses to start against a database
  whose schema is newer than the binary understands, instead of silently
  running against it.
- A tagged-release GitHub Actions workflow (`.github/workflows/release.yml`)
  that cross-compiles `linux/amd64` and `linux/arm64` binaries, packages
  them with the built web UI into checksummed tarballs, and publishes a
  GitHub Release.
- `HOSTFORGE_VERSION` pinning and a `--download-release` mode in
  `scripts/install.sh`, to install a specific tagged release instead of
  building from source.
- Resource limits and hardening on every deployed application container:
  configurable memory, CPU, and process-count limits, plus `CapDrop: ALL`,
  `no-new-privileges`, and a locked-down `/tmp`.
- The Railpack build plan and detected-stack info are now persisted per
  deployment, instead of being discarded once the build finishes.
- A required encryption key, verified at startup against a stored canary —
  a key mismatch now fails loudly at boot instead of silently degrading.
- Login rate limiting on `/auth/session`.
- `Origin` header validation on WebSocket log-stream upgrades.
- Graceful shutdown on `SIGINT`/`SIGTERM`: HTTP connections and in-flight
  deploys are drained before the process exits.
- Apache-2.0 licensing (`LICENSE`, `NOTICE`, `CONTRIBUTING.md`).

### Changed

- The Go module path is now `github.com/furious-fury/HostForge` (previously
  `github.com/hostforge/hostforge`).
- The Docker client is now constructed once and shared across every deploy,
  provisioning, and metrics path, instead of each one opening its own.

### Removed

- The legacy `cmd/cli` binary. Its `deploy`/`domain` subcommands had already
  been removed from the platform; the binary itself is gone too. Use the
  HTTP API, or `hostforge-server -version` for version info.

### Fixed

- A Docker-unavailable code path in application deletion that could panic
  instead of degrading.
- A certificate-poll background loop that ignored shutdown cancellation.
- SQLite DSN pragmas that the `modernc.org/sqlite` driver was silently
  discarding (busy timeout, WAL mode, foreign keys, synchronous mode).
- Deployment event messages that were being overwritten instead of
  concatenated.
