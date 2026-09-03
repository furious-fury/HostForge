# Changelog

All notable changes to HostForge are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/): `0.MINOR.PATCH`
while pre-1.0 (ADR-0002 §24.1) — a `MINOR` bump may still include breaking
changes, since the API has not yet reached 1.0 stability.

## [Unreleased]

## [0.9.3] - 2026-09-03

### Fixed

- **Cancelling a deploy did nothing for up to 30 seconds, and the deploy then
  overwrote the cancellation.** A running operation only checked for a
  cancellation request when it renewed its lease, so a cancel could sit
  unnoticed for a full lease interval — long enough for a health check and
  cutover to finish. Cancellation is now watched on its own one-second
  ticker, separate from lease renewal.
- **A cancelled deployment could be moved back out of CANCELLED.** Because
  the cancellation is written while the deploy is still running, work still in
  flight could overwrite it — producing a row that was cancelled and failed at
  once, or, had the health check passed, putting a cancelled deploy into
  production as SUCCESS. CANCELLED is now final.

## [0.9.2] - 2026-09-03

### Fixed

- **Registering the platform domain failed with "Caddy could not apply the
  permanent platform route".** The route HostForge writes for the domain
  carried a bare `tls` directive, which Caddy 2.11 rejects, so validation
  failed and the snippet was rolled back. The permanent route now emits no
  `tls` directive at all and lets automatic HTTPS obtain a publicly trusted
  certificate for the hostname; the raw-IP bootstrap route names Caddy's
  internal CA explicitly, since no public CA issues for an IP address.

  This is the same defect fixed in the installer script in 0.9.1. That fix
  corrected the script that writes the route at install time and missed the
  code that rewrites it afterwards.

### Fixed

- **The bootstrap installer could not finish on a fresh host.** The generated
  Caddy route used a bare `tls` directive, which Caddy 2.11 rejects, so
  `caddy validate` failed and the installer stopped before starting
  HostForge — leaving an enabled service that had never run. It now uses
  `tls internal`, which is also the accurate description: a raw IP address
  cannot be issued a publicly trusted certificate, so browsers will warn until
  a real domain is configured.
- The installer's certificate check verified the chain on a certificate issued
  by Caddy's own internal CA, which can never validate. It would have failed
  every attempt against a healthy Caddy and aborted the install.
- **Connecting the GitHub App appeared to do nothing.** GitHub returns a
  one-time manifest code in the URL, and the onboarding screen waited for a
  second, easily missed click before spending it. Navigating away or
  refreshing discarded the code silently, leaving an App on GitHub that
  HostForge had no credentials for. The exchange now happens on arrival, with
  a retry offered if it fails.

## [0.9.0] - 2026-09-02

### Added

- A generic operations queue, and a worker runtime that runs jobs off it.
  Database operations are the first subsystem on it. Work is serialised by a
  lock key when it is claimed, survives a crash, and can be cancelled at a
  step boundary.
- Interrupted operations are now recovered as part of starting the runtime,
  before any worker runs, rather than by a separate loop that had to be
  called first.
- Deploys run on the operations queue too, on their own worker pool sized by
  `HOSTFORGE_DEPLOY_CONCURRENCY` (1-8, default 2). Deploys of the same service
  and environment still run one at a time; different services now run in
  parallel.
- Installs and upgrades come from published releases: a checksum-verified
  prebuilt binary and UI, downloaded over HTTPS. `HOSTFORGE_VERSION=vX.Y.Z`
  pins a specific release. Building from source is still available with
  `--from-source`.
- `scripts/uninstall.sh`, the counterpart to the installer, removing the
  service, binaries, configuration, data, and install tree.

### Changed

- **A second action on a busy database is queued rather than rejected.**
  Previously it failed with `database operation already in progress`. Only
  one operation per database still runs at a time. The admission checks this
  replaces were bypassed by four of the seven paths that enqueue work, so the
  guarantee they appeared to give did not hold.
- Shutdown drains in-flight database operations instead of cancelling them,
  and releases their leases if the drain deadline passes so the next start
  recovers immediately.
- **Cancelling a deploy now works regardless of which process started it.**
  Cancellation went through an in-memory map, so only the process that
  launched a deploy could stop it, and a restart lost the ability entirely.
- Installing no longer requires git, Go, or Node on the host. None were ever
  needed to *run* HostForge: deploys clone in-process, and application builds
  happen inside Docker. They are installed only for `--from-source`.
- Upgrades are identified by release version rather than git commit, and roll
  back by reinstalling the previous version.

### Removed

- `HOSTFORGE_RAILPACK_BUILD_CONCURRENCY`. It was parsed and validated but
  bounded nothing; `HOSTFORGE_DEPLOY_CONCURRENCY` is the knob that does.

### Fixed

- Operations for a database being deleted are cancelled instead of being left
  queued forever, invisible to the queue and polled by the UI indefinitely.
- Deleting a database with work in flight now asks that work to stop, so a
  retry succeeds shortly after rather than failing until the operation
  finishes on its own.
- The database detail screen shows the operation that is running rather than
  the most recently queued one, which could leave the progress bar reading 0%
  and apparently frozen.
- Operations that exhausted their retries were re-claimed forever; they now
  fail with `interrupted`.
- The database operation claim and the per-instance admission checks had no
  supporting index and scanned the whole table.
- A persistent database error made the gateway worker spin without yielding.
- Deploys now stop at step boundaries when cancelled, instead of running to
  completion, and no longer leave the half-built container running.
- A deploy cancelled between its health check and being marked successful was
  overwritten as successful anyway.
- Two services sharing a repository and branch, or one service deployed to two
  environments, shared a git worktree and could run concurrent checkouts in
  it. Worktrees are now per service and environment.
- Two deploys of the same repository and branch starting in the same second
  produced identical image tags and container names.
- Interrupted deploys are recorded as failed rather than silently re-run on
  the next start.
- The installer's admin-secret prompt read from the same stream the piped
  script was arriving on, so what it compared was script text rather than
  what was typed -- reporting a mismatch for two identical entries.

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
