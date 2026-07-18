# HostForge PostgreSQL gateway image

This image is the reviewed PostgreSQL v1 data-plane runtime. It extends the
digest-pinned Percona PgBouncer 1.25.2 image with Percona PostgreSQL 16's
`psql` client. HostForge needs both binaries in the same container for config
administration and an end-to-end `sslmode=verify-full` authentication probe.

The image contains no HostForge configuration, certificate, credential, or
secret. HostForge mounts the active `0600` generation read-only at runtime.

## Publish

The `postgresql-gateway-image.yml` workflow publishes an amd64 image to:

```text
ghcr.io/furious-fury/hostforge-postgresql-gateway:sha-<git-sha>
```

Run the workflow from the reviewed commit, then make the GHCR package public.
HostForge intentionally supplies no registry credential to Docker and will
fail closed if the immutable image cannot be pulled anonymously.

Resolve and record the published digest:

```bash
docker pull ghcr.io/furious-fury/hostforge-postgresql-gateway:sha-<git-sha>
docker image inspect ghcr.io/furious-fury/hostforge-postgresql-gateway:sha-<git-sha> \
  --format '{{index .RepoDigests 0}}'
```

Before setting the HostForge environment, verify the exact published image:

```bash
image='ghcr.io/furious-fury/hostforge-postgresql-gateway@sha256:<digest>'
docker pull "$image"
docker run --rm --entrypoint pgbouncer "$image" --version
docker run --rm --entrypoint psql "$image" --version
timeout 20 docker run --rm "$image" pgbouncer --version
docker image inspect "$image" \
  --format 'user={{json .Config.User}} entrypoint={{json .Config.Entrypoint}}'
```

The checks must report PgBouncer 1.25.2, PostgreSQL client 16 or newer, runtime
user `1001:1001`, and a successful normal-entrypoint command override. Configure
only the immutable `@sha256:` reference in `HOSTFORGE_POSTGRES_GATEWAY_IMAGE`.
