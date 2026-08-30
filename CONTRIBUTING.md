# Contributing

Thanks for your interest in HostForge.

## License

HostForge is licensed under Apache-2.0 (see [LICENSE](./LICENSE)). By
submitting a contribution — a pull request, patch, or any other form of
proposed change — you agree it is licensed under the same terms, per
section 5 of the Apache License, Version 2.0.

## Getting set up

See [Local development](./docs/development.md) for prerequisites and how
to run the server and web UI locally.

## Before opening a pull request

Run the checks CI runs:

```bash
go build ./...
go vet ./...
go test ./...
npm --prefix web run lint
npm --prefix web run build
npm --prefix web run test
```

## Pull requests

- Keep a PR to one reviewable change. A feature, a bug fix, and a
  refactor are separate PRs, even if you did them in one sitting.
- Write a clear PR description: what changed and why, not just what.
- Add or update tests for behavior you change.
