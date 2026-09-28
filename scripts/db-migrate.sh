#!/bin/sh
# ObserveX — apply the PostgreSQL migrations in order (interim mechanism).
#
# Usage:  POSTGRES_DSN=postgres://user:pass@host:5432/db?sslmode=require scripts/db-migrate.sh [DIR]
#
# DIR defaults to internal/db/migrations (or /migrations inside the
# db-migrate image). Every file is applied in lexical order, each in its own
# transaction, and the run stops at the first error (ON_ERROR_STOP). After a
# file succeeds its name is recorded in schema_migrations, which 001 defines.
#
# Every migration is idempotent, so the whole set is re-applied on every run.
# That is also how a database left part-initialised by the errors corrected in
# 001 and 005 is completed. This script formalises the repository's existing
# "apply the SQL files in order" behaviour; it does NOT decide D-03 (adopting a
# migration framework), and a framework chosen under D-03 can adopt the same
# files and the same schema_migrations table.
#
# The DSN is read from the environment only and is never printed.
set -eu
# Keep notices ("... does not exist, skipping") out of the log.
PGOPTIONS="${PGOPTIONS:-} -c client_min_messages=warning"
export PGOPTIONS

DIR="${1:-}"
if [ -z "$DIR" ]; then
  if [ -d /migrations ]; then DIR=/migrations; else DIR="$(dirname "$0")/../internal/db/migrations"; fi
fi
: "${POSTGRES_DSN:?POSTGRES_DSN must be set}"

WAIT="${DB_MIGRATE_WAIT_SECONDS:-120}"
i=0
until psql "$POSTGRES_DSN" -qtAc 'SELECT 1' >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge "$WAIT" ]; then
    echo "db-migrate: database not reachable after ${WAIT}s" >&2
    exit 1
  fi
  sleep 1
done

found=0
for f in "$DIR"/*.sql; do
  [ -e "$f" ] || continue
  found=1
  name="$(basename "$f" .sql)"
  echo "db-migrate: applying $name"
  psql "$POSTGRES_DSN" -q -v ON_ERROR_STOP=1 --single-transaction -f "$f"
  # Variables are interpolated only in script input, not in -c; the name is
  # passed as a quoted literal (:'name'), never spliced into SQL text.
  echo "INSERT INTO schema_migrations(version) VALUES (:'name') ON CONFLICT (version) DO NOTHING;" |
    psql "$POSTGRES_DSN" -q -v ON_ERROR_STOP=1 -v name="$name"
done
if [ "$found" -eq 0 ]; then
  echo "db-migrate: no migrations found in $DIR" >&2
  exit 1
fi
echo "db-migrate: done"
