# ObserveX PostgreSQL migrations (interim mechanism; the migration framework is
# open decision D-03). Applies internal/db/migrations/*.sql in order with
# scripts/db-migrate.sh; every migration is idempotent.
# Build from the repository root:
#   docker build -f services/db-migrate/Dockerfile -t observex/db-migrate .
# Run:
#   docker run --rm -e POSTGRES_DSN='postgres://…' observex/db-migrate
FROM postgres:16-alpine

COPY scripts/db-migrate.sh /usr/local/bin/db-migrate.sh
COPY internal/db/migrations/ /migrations/

RUN chmod 0755 /usr/local/bin/db-migrate.sh && chmod -R a+rX /migrations

# The image's unprivileged postgres user (uid 70); no server is started.
USER 70:70

ENTRYPOINT ["/usr/local/bin/db-migrate.sh", "/migrations"]
