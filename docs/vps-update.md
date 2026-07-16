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

For a manual update, keep every command connected with `&&` so execution stops after the first failure:

```bash
cd /opt/hostforge &&
git pull --ff-only origin main &&
./scripts/install.sh --with-systemd &&
systemctl restart hostforge-server &&
systemctl --no-pager --full status hostforge-server
```

After either path, hard-refresh the dashboard with `Ctrl+Shift+R`.

## If the update fails

Check the service logs first:

```bash
systemctl status hostforge-server --no-pager
journalctl -u hostforge-server -n 200 --no-pager
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
