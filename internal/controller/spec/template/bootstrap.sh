#!/bin/sh
set -eu

# This script is intentionally restricted to a new, empty PGDATA. It never
# overwrites a target and never treats PG_VERSION as proof of completion.
parent=$(dirname "$PGDATA")
target=$PGDATA
stage="$parent/.kubegres-bootstrap-staging"
marker="$target/.kubegres-bootstrap-complete"
identity="$stage/identity"

fail() { echo "bootstrap failed: $1" >&2; exit 1; }

: "${BOOTSTRAP_CR_UID:?custom resource identity is required}"
: "${BOOTSTRAP_PVC_NAME:?PVC identity is required}"

if [ -e "$target" ] && { [ -L "$target" ] || [ ! -d "$target" ]; }; then fail 'PGDATA path is not a regular directory'; fi
if [ -e "$marker" ] && [ ! -f "$marker" ]; then fail 'completion marker is not a regular file'; fi
if [ -f "$marker" ]; then
  grep -qx 'version=kubegres-logical-bootstrap-v1' "$marker" || fail 'completion marker is invalid'
  grep -qx "cr_uid=$BOOTSTRAP_CR_UID" "$marker" || fail 'completion marker identity mismatch'
  grep -qx "pvc=$BOOTSTRAP_PVC_NAME" "$marker" || fail 'completion marker PVC mismatch'
  [ "$(wc -l < "$marker")" -eq 3 ] || fail 'completion marker has unexpected content'
  [ -f "$target/PG_VERSION" ] || { echo 'bootstrap marker has no PG_VERSION' >&2; exit 1; }
  exit 0
fi

if [ "${BOOTSTRAP_COMPLETED:-false}" = "true" ]; then fail 'completed bootstrap has no valid marker on this storage'; fi

[ -e "$target" ] && [ ! -d "$target" ] && { echo 'PGDATA is not a directory' >&2; exit 1; }
[ -L "$target" ] && { echo 'PGDATA must not be a symlink' >&2; exit 1; }
[ -d "$target" ] && [ -n "$(find "$target" -mindepth 1 -maxdepth 1 -print -quit)" ] && { echo 'PGDATA is non-empty and has no valid bootstrap marker' >&2; exit 1; }
[ -e "$stage" ] && {
  [ -f "$identity" ] || fail 'unrecognized bootstrap staging directory exists'
  grep -qx "cr_uid=$BOOTSTRAP_CR_UID" "$identity" || fail 'staging identity mismatch'
  grep -qx "pvc=$BOOTSTRAP_PVC_NAME" "$identity" || fail 'staging PVC mismatch'
  [ "$(wc -l < "$identity")" -eq 2 ] || fail 'staging identity is invalid'
  rm -rf "$stage"
}
mkdir -p "$stage/data"
trap 'rm -rf "$stage"' EXIT HUP INT TERM

: "${BOOTSTRAP_SOURCE_PASSWORD:?source password is required}"
: "${POSTGRES_PASSWORD:?destination password is required}"
: "${BOOTSTRAP_SOURCE_HOST:?source host is required}"
: "${BOOTSTRAP_SOURCE_DATABASE:?source database is required}"
: "${BOOTSTRAP_SOURCE_USERNAME:?source username is required}"
: "${BOOTSTRAP_DESTINATION_DATABASE:?destination database is required}"
: "${BOOTSTRAP_CA_FILE:?source CA is required}"
: "${BOOTSTRAP_TIMEOUT_SECONDS:?bootstrap timeout is required}"
printf '%s\n' "cr_uid=$BOOTSTRAP_CR_UID" "pvc=$BOOTSTRAP_PVC_NAME" > "$identity"
chmod 600 "$identity"

export PGPASSWORD="$BOOTSTRAP_SOURCE_PASSWORD"
if ! PGSSLMODE=verify-full PGSSLROOTCERT="$BOOTSTRAP_CA_FILE" timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" pg_dump --format=custom --no-owner --no-privileges --no-tablespaces --no-subscriptions \
  --host="$BOOTSTRAP_SOURCE_HOST" --port="${BOOTSTRAP_SOURCE_PORT:-5432}" \
  --username="$BOOTSTRAP_SOURCE_USERNAME" --dbname="$BOOTSTRAP_SOURCE_DATABASE" \
  --file="$stage/source.dump" >/dev/null 2>&1; then
  fail 'source export failed'
fi
unset PGPASSWORD

printf '%s\n' "$POSTGRES_PASSWORD" > "$stage/postgres-password"
chmod 600 "$stage/postgres-password"
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" initdb -D "$stage/data" --username="${POSTGRES_USER:-postgres}" --pwfile="$stage/postgres-password" --auth-local=trust --auth-host=scram-sha-256 >/dev/null 2>&1; then fail 'staging database initialization failed'; fi
mkdir -p "$stage/socket"
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" pg_ctl -D "$stage/data" -o "-c listen_addresses='' -c unix_socket_directories=$stage/socket" -w start >/dev/null 2>&1; then fail 'staging database start failed'; fi
cleanup_server() { pg_ctl -D "$stage/data" -m fast -w stop >/dev/null 2>&1 || true; }
stop_server() { pg_ctl -D "$stage/data" -m fast -w stop >/dev/null 2>&1; }
trap 'cleanup_server; rm -rf "$stage"' EXIT HUP INT TERM

export PGHOST="$stage/socket"
case "$BOOTSTRAP_DESTINATION_DATABASE" in *[!A-Za-z0-9_]*|'') echo 'destination database is not a safe identifier' >&2; exit 1;; esac
case "$BOOTSTRAP_SOURCE_DATABASE" in *[!A-Za-z0-9_]*|'') echo 'source database is not a safe identifier' >&2; exit 1;; esac
case "$BOOTSTRAP_SOURCE_USERNAME" in *[!A-Za-z0-9_]*|'') echo 'source username is not a safe identifier' >&2; exit 1;; esac
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" psql --username="${POSTGRES_USER:-postgres}" --dbname=postgres --set=ON_ERROR_STOP=1 --command="CREATE DATABASE \"$BOOTSTRAP_DESTINATION_DATABASE\";" >/dev/null 2>&1; then fail 'destination database creation failed'; fi
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" psql --username="${POSTGRES_USER:-postgres}" --dbname=postgres --set=ON_ERROR_STOP=1 --set=replication_password="$POSTGRES_REPLICATION_PASSWORD" --set=replication_role="${BOOTSTRAP_REPLICATION_USER:-replication}" >/dev/null 2>&1 <<'SQL'
CREATE ROLE :"replication_role" WITH REPLICATION LOGIN PASSWORD :'replication_password';
SQL
then fail 'replication role creation failed'; fi
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" psql --username="${POSTGRES_USER:-postgres}" --dbname="$BOOTSTRAP_DESTINATION_DATABASE" --set=ON_ERROR_STOP=1 --command="GRANT EXECUTE ON FUNCTION pg_promote(boolean, integer) TO \"${BOOTSTRAP_REPLICATION_USER:-replication}\";" >/dev/null 2>&1; then fail 'replication role privilege setup failed'; fi
if ! timeout --foreground "${BOOTSTRAP_TIMEOUT_SECONDS}s" pg_restore --exit-on-error --single-transaction --no-owner --no-privileges --no-tablespaces --no-subscriptions --username="${POSTGRES_USER:-postgres}" --dbname="$BOOTSTRAP_DESTINATION_DATABASE" "$stage/source.dump" >/dev/null 2>&1; then fail 'destination restore failed'; fi
unset PGHOST
if ! stop_server; then fail 'staging database did not stop cleanly'; fi
trap - EXIT HUP INT TERM
rm -f "$stage/source.dump" "$stage/postgres-password"
rm -f "$identity"
rm -rf "$stage/socket"
{
  printf '%s\n' 'version=kubegres-logical-bootstrap-v1' "cr_uid=$BOOTSTRAP_CR_UID" "pvc=$BOOTSTRAP_PVC_NAME"
} > "$stage/data/.kubegres-bootstrap-complete"
# Publish only after the staged database has stopped cleanly and the target is absent.
[ -d "$target" ] && [ -n "$(find "$target" -mindepth 1 -maxdepth 1 -print -quit)" ] && fail 'target changed during bootstrap'
rmdir "$target" 2>/dev/null || true
[ ! -e "$target" ] || fail 'target appeared during publication'
mv -T "$stage/data" "$target"
rmdir "$stage"
