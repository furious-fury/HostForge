FROM docker.io/percona/percona-pgbouncer@sha256:2176f8efc705bfd2cb332dfabf3a29e41b45ef82254049e0a4a8cff80d3e51dc

LABEL org.opencontainers.image.title="HostForge PostgreSQL Gateway" \
      org.opencontainers.image.description="PgBouncer with the psql client required for HostForge TLS/auth probes" \
      org.opencontainers.image.source="https://github.com/furious-fury/HostForge" \
      org.opencontainers.image.licenses="Apache-2.0"

USER 0

RUN set -eux; \
    microdnf install -y percona-postgresql16; \
    microdnf clean all; \
    rm -rf /var/cache/dnf /var/cache/yum; \
    pgbouncer --version; \
    psql --version; \
    test "$(pgbouncer --version | sed -n '1s/^PgBouncer //p')" = "1.25.2"; \
    test "$(psql --version | sed -n 's/^psql (PostgreSQL) \([0-9][0-9]*\).*/\1/p')" -ge 16

USER 1001:1001
