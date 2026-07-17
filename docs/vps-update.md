# Updating HostForge on the VPS

This runbook applies to the current production installation:

- Repository: `/opt/hostforge`
- Service: `hostforge-server.service`
- VPS IP: `169.58.1.87`
- Public URL: `https://hostforge.mrfury.dev/`
- HostForge listener: `127.0.0.1:8080` behind Caddy

## Deploy a committed update

SSH to the VPS as root, then run the fail-fast update helper with the public management origin:

```bash
cd /opt/hostforge
HF_SERVER_URL="https://hostforge.mrfury.dev" ./scripts/vps-update-and-smoke.sh
```

The currently deployed release predates this helper. For its first deployment only, restore the known generated artifact, pull the release, and then invoke the helper:

```bash
cd /opt/hostforge &&
git restore -- web/tsconfig.app.tsbuildinfo &&
git pull --ff-only origin main &&
HF_SERVER_URL="https://hostforge.mrfury.dev" ./scripts/vps-update-and-smoke.sh
```

Do not run the restore if `git status --short` shows unrelated tracked changes. Review those changes first.

The helper:

1. Restores only the known generated `web/tsconfig.app.tsbuildinfo` artifact when an older checkout still tracks it.
2. Refuses to continue when any other tracked VPS changes exist.
3. Pulls `main` with `--ff-only`, records the previous commit, and builds before restarting.
4. Waits for systemd and the public HTTPS origin.
5. Reads the management token from `/etc/hostforge/hostforge.env` without printing it.
6. Runs the authenticated v2 API smoke, including array contracts, legacy-route absence, logout, and post-logout `401`.

`install.sh --with-systemd` keeps the existing `/etc/hostforge/hostforge.env` file. Do not run `bootstrap-ubuntu.sh` for routine updates; it is only for first-time VPS provisioning.

## One-time Caddy layout migration

The HTTPS provisioning fix requires the control-plane route to live in a HostForge-managed snippet instead of the root-owned `/etc/caddy/Caddyfile`. During the update, the installer:

1. Adds the `hostforge` service user to the `caddy` group.
2. Creates or repairs the setgid Caddy snippet directory and group-readable files.
3. Moves the existing bootstrap or platform route into `/etc/caddy/hostforge.d/control-plane.caddy`.
4. Keeps generated deployment routes in `/etc/caddy/hostforge.d/routes.caddy`.
5. Replaces a recognized HostForge-owned root Caddyfile with imports for both snippets.
6. Validates and reloads Caddy before accepting the migration.
7. Saves the original root config as `/etc/caddy/Caddyfile.hostforge-before-managed-imports`.

This migration is idempotent and runs from `install.sh --with-systemd`, including when using `vps-update-and-smoke.sh`. It lets the unprivileged `hostforge-server` register or change the platform domain without writing the root Caddyfile.

After the update, verify the migrated layout:

```bash
id hostforge
ls -ld /etc/caddy/hostforge.d
ls -l /etc/caddy/hostforge.d/control-plane.caddy /etc/caddy/hostforge.d/routes.caddy
cat /etc/caddy/Caddyfile
caddy validate --config /etc/caddy/Caddyfile
systemctl is-active caddy hostforge-server
```

The `hostforge` user should include the `caddy` group. The root Caddyfile should import both managed snippets, and both services should report `active`.

Once those checks pass, return to `https://hostforge.mrfury.dev/`, hard-refresh with `Ctrl+Shift+R`, and retry **Verify and register domain**. Successful registration replaces the bootstrap IP route in `control-plane.caddy` with the permanent `hostforge.mrfury.dev` route. Generated deployment URLs continue to use `routes.caddy`.

If an older checkout stops with `scripts/migrate-caddy-layout.sh: Permission denied`, the update has not restarted HostForge. Run the migration through Bash, finish the installation, and restart the service:

```bash
cd /opt/hostforge &&
bash ./scripts/migrate-caddy-layout.sh &&
./scripts/install.sh --with-systemd &&
systemctl restart hostforge-server &&
systemctl --no-pager --full status hostforge-server
```

The installer invokes the migration through Bash in current releases, so the script does not depend on the executable bit being preserved by Git.

### Custom Caddyfile

The installer does not rewrite an unrecognized or operator-customized Caddyfile. If it prints a migration warning:

1. Move the existing HostForge control-plane site block from the root Caddyfile into `/etc/caddy/hostforge.d/control-plane.caddy`.
2. Preserve every unrelated custom directive and site block.
3. Add these imports to the root Caddyfile:

```caddyfile
import /etc/caddy/hostforge.d/control-plane.caddy
import /etc/caddy/hostforge.d/routes.caddy
```

The control-plane snippet for the current production hostname should contain:

```caddyfile
https://hostforge.mrfury.dev {
    tls
    reverse_proxy 127.0.0.1:8080
}
```

Do not leave a duplicate `hostforge.mrfury.dev` site block in the root Caddyfile. After editing both files, run:

```bash
usermod -aG caddy hostforge
install -d -m 2770 -o root -g caddy /etc/caddy/hostforge.d
test -f /etc/caddy/hostforge.d/control-plane.caddy
touch /etc/caddy/hostforge.d/routes.caddy
chown root:caddy /etc/caddy/hostforge.d/control-plane.caddy /etc/caddy/hostforge.d/routes.caddy
chmod 0640 /etc/caddy/hostforge.d/control-plane.caddy /etc/caddy/hostforge.d/routes.caddy
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
systemctl restart hostforge-server
```

For a manual update, keep every command connected with `&&` so execution stops after the first failure:

```bash
cd /opt/hostforge &&
git pull --ff-only origin main &&
./scripts/install.sh --with-systemd &&
systemctl restart hostforge-server &&
systemctl --no-pager --full status hostforge-server
```

After either path, hard-refresh the dashboard with `Ctrl+Shift+R`.

## Database-services upgrade and restore drill

Before restarting into a release that adds database migrations, copy the SQLite database and confirm the encryption key remains present:

```bash
install -m 0600 -o root -g root /var/lib/hostforge/hostforge.db "/var/lib/hostforge/hostforge.db.pre-database-services.$(date +%Y%m%d%H%M%S)"
grep -q '^HOSTFORGE_ENV_ENCRYPTION_KEY=' /etc/hostforge/hostforge.env
grep -q '^HOSTFORGE_DATABASE_MIN_FREE_DISK_BYTES=' /etc/hostforge/hostforge.env || printf '%s\n' 'HOSTFORGE_DATABASE_MIN_FREE_DISK_BYTES=5368709120' >> /etc/hostforge/hostforge.env
grep -q '^HOSTFORGE_DATABASE_OPERATION_CONCURRENCY=' /etc/hostforge/hostforge.env || printf '%s\n' 'HOSTFORGE_DATABASE_OPERATION_CONCURRENCY=1' >> /etc/hostforge/hostforge.env
grep -q '^HOSTFORGE_DATABASE_TRANSFER_MAX_PER_HOUR=' /etc/hostforge/hostforge.env || printf '%s\n' 'HOSTFORGE_DATABASE_TRANSFER_MAX_PER_HOUR=60' >> /etc/hostforge/hostforge.env
```

Run the read-only capacity and exposure audit before provisioning, and again after each engine is added:

```bash
cd /opt/hostforge
bash ./scripts/database-services-vps-audit.sh
```

The audit fails when Docker storage is below the configured reserve, a managed database publishes a host port, or a managed database lacks CPU or memory enforcement. It reports allocated bytes for every labelled HostForge database volume and their combined total, plus Docker image/volume usage, private environment networks, and any standard database ports listening on the host. Run it as root when possible so the volume mountpoints can be measured; inaccessible mountpoints are reported as `unavailable` without weakening the exposure and resource-limit checks. The listener list is diagnostic because the VPS may contain an independently managed database; HostForge ownership and port-publication checks remain label-scoped.

For the complete six-engine drill, start with at least 15 GiB free in Docker storage and preferably 20 GiB. Image layers are shared, so `docker system df -v` on the target host is authoritative; do not estimate total usage by adding every image's virtual size. Running every engine concurrently also requires enough memory for their configured limits plus Docker, HostForge, BuildKit, Caddy, and the operating system.

As a planning allowance, reserve roughly 4–7 GiB for the six initial engine images and freshly initialized staging volumes, in addition to HostForge's 5 GiB safety floor. This is deliberately conservative rather than a quota: real database rows, indexes, temporary files, retained seven-day volumes, Docker build cache, and old image digests determine ongoing usage. Remote backups stream to R2/S3 and are not retained as a second full local archive. Use the audit output before and after each engine to record the actual delta on this VPS.

The default Development presets total 4.5 GiB of enforceable database memory across all six engines. Prefer an 8 GiB or larger VPS for the simultaneous acceptance matrix; on a smaller host, provision and verify engines sequentially and stop completed instances before starting the next one, while still completing the restart and persistence checks for each.

After update, provision one disposable instance of each enabled engine and verify the pinned image, private network, health, CPU/memory limits, and observed volume usage. Connect R2 or S3, create a manual backup, restore it as a new service, and verify known data before testing replace-current. Replace-current must show a successful safety backup and must restart bound application containers.

When a later release changes only a digest within the same catalog version, open the database instance and run the patch-upgrade preflight. It must refuse an unhealthy instance or one without a successful backup from the previous 24 hours. After a qualifying backup, verify the upgrade reuses the same volume, alias, engine version, and limits. A deliberately failing candidate must recreate the previous digest and return the database to healthy state.

Rollback the server binary only after stopping `hostforge-server`; restore the pre-upgrade SQLite copy only when the newer process cannot be recovered. Never remove a `hostforge-db-*` volume as part of application rollback. Keep remote backup objects and the encryption key so a fresh HostForge install can recreate an isolated service and restore the logical backup.

## If the update fails

Check the service logs first:

```bash
systemctl status hostforge-server --no-pager
journalctl -u hostforge-server -n 200 --no-pager
journalctl -u caddy -n 200 --no-pager
```

If the Caddy migration fails, the script leaves the existing root config in place or restores it after a failed reload. Inspect the validation error before retrying:

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl status caddy --no-pager
```

If the migrated layout must be reverted manually, restore the one-time backup and reload Caddy:

```bash
cp -a /etc/caddy/Caddyfile.hostforge-before-managed-imports /etc/caddy/Caddyfile
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
systemctl restart hostforge-server
```

If Git reports that `web/tsconfig.app.tsbuildinfo` would be overwritten, discard only that generated build artifact and retry the pull:

```bash
cd /opt/hostforge
git restore -- web/tsconfig.app.tsbuildinfo
git pull --ff-only origin main
```

Do not use a broad restore command when other files are modified. Run `git status --short` and inspect unexpected VPS changes before discarding them.

To return the repository to the last known Git commit, identify the previous commit and check it out explicitly, rebuild, and restart:

```bash
cd /opt/hostforge
git log --oneline -5
git checkout <previous-good-commit>
./scripts/install.sh --with-systemd
systemctl restart hostforge-server
```

After recovery, switch back to `main` before applying a later fixed release.
