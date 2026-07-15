# Updating HostForge on the VPS

This runbook applies to the current production installation:

- Repository: `/opt/hostforge`
- Service: `hostforge-server.service`
- VPS IP: `169.58.1.87`
- Public URL: `https://hostforge.mrfury.dev/`
- HostForge listener: `127.0.0.1:8080` behind Caddy

## Deploy a committed update

SSH to the VPS as root, then pull the committed `main` branch, rebuild the binaries and web UI, and restart the service:

```bash
cd /opt/hostforge
git pull --ff-only origin main
./scripts/install.sh --with-systemd
systemctl restart hostforge-server
systemctl --no-pager --full status hostforge-server
```

`install.sh --with-systemd` keeps the existing `/etc/hostforge/hostforge.env` file. Do not run `bootstrap-ubuntu.sh` for routine updates; it is only for first-time VPS provisioning.

## Verify the update

```bash
journalctl -u hostforge-server -n 100 --no-pager
curl -I https://hostforge.mrfury.dev/
```

Then hard-refresh the dashboard in the browser with `Ctrl+Shift+R`.

## If the update fails

Check the service logs first:

```bash
systemctl status hostforge-server --no-pager
journalctl -u hostforge-server -n 200 --no-pager
```

To return the repository to the last known Git commit, identify the previous commit and check it out explicitly, rebuild, and restart:

```bash
cd /opt/hostforge
git log --oneline -5
git checkout <previous-good-commit>
./scripts/install.sh --with-systemd
systemctl restart hostforge-server
```

After recovery, switch back to `main` before applying a later fixed release.
