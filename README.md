# HostForge

Self-hosted PaaS: deploy from Git to a single VPS. **Phase 0** implements the CLI spike: clone a repo and run [Nixpacks](https://github.com/railwayapp/nixpacks) with streamed logs.


## Prerequisites

- **Go** 1.22+
- **Git** (for upstream repos; the CLI uses [go-git](https://github.com/go-git/go-git) but host tooling may still matter)
- **Nixpacks CLI** on your `PATH` — [installation](https://github.com/railwayapp/nixpacks#installation)
- **Docker** is *not* required for Phase 0 when using `nixpacks build ... -o <dir>` (filesystem output)

### Windows

Nixpacks is most reliable under **WSL2** or a **Linux** environment. Native Windows may work if `nixpacks` is installed and on `PATH`, but if builds fail unexpectedly, try WSL2.

## Build

```bash
go build -o hostforge ./cmd/cli
```

## Usage

```bash
./hostforge deploy [flags] <repo_url>
./hostforge version
```

- **`<repo_url>`:** HTTPS URL only in Phase 0 (e.g. `https://github.com/org/repo`).
- **`--data-dir`:** Overrides the data root (default: `./.hostforge`). You can also set **`HOSTFORGE_DATA_DIR`**.
- **`--branch`:** Optional branch name; default is the remote’s default branch.

Data layout:

- Worktrees: `<data-dir>/worktrees/<hash>/`
- Nixpacks output: `<data-dir>/builds/<hash>/`

## Golden path (manual test)

Use a small public Node app with a root `package.json`:

```bash
go run ./cmd/cli deploy https://github.com/heroku/node-js-getting-started
```

**Expected:** Git clone progress on stderr, Nixpacks logs on stdout/stderr, exit code **0**, and artifacts under `.hostforge/builds/<hash>/`.

**If clone fails:** Check network, URL, and private-repo access (Phase 0 does not configure Git credentials beyond your environment).

**If Nixpacks fails:** Run `nixpacks plan .` inside the worktree path printed in logs, or install/upgrade Nixpacks. Ensure sufficient disk and that the stack is supported by Nixpacks.

## Server binary

`cmd/server` is a stub until later phases.
