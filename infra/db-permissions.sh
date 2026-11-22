#!/bin/sh
set -eu

case "$POSTGRES_RUNTIME_PASSWORD" in
  ''|*[!A-Za-z0-9_-]*)
    echo 'POSTGRES_RUNTIME_PASSWORD must use only letters, digits, underscores and hyphens' >&2
    exit 1
    ;;
esac
if [ "${#POSTGRES_RUNTIME_PASSWORD}" -lt 32 ] || [ "$POSTGRES_RUNTIME_PASSWORD" = "$POSTGRES_PASSWORD" ]; then
  echo 'POSTGRES_RUNTIME_PASSWORD must be at least 32 characters and distinct from POSTGRES_PASSWORD' >&2
  exit 1
fi

export PGPASSWORD="$POSTGRES_PASSWORD"
export PGCLIENTENCODING=UTF8
export PGCONNECT_TIMEOUT=10

# psql reads both prompts from stdin and sends a SCRAM verifier to PostgreSQL.
# Keep its output private because a failed password-setting command can include it.
if ! printf '%s\n%s\n' "$POSTGRES_RUNTIME_PASSWORD" "$POSTGRES_RUNTIME_PASSWORD" |
  psql -X --no-password --single-transaction -q -v ON_ERROR_STOP=1 \
    -h postgres -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f /db-permissions.sql >/dev/null 2>&1; then
  echo 'Database role setup failed. Check migration status, owner credentials, and the runtime role ownership and membership.' >&2
  exit 1
fi

export PGPASSWORD="$POSTGRES_RUNTIME_PASSWORD"
if ! psql -X --no-password -qAt -v ON_ERROR_STOP=1 -h postgres -U anonymus_runtime -d "$POSTGRES_DB" \
  -c 'SELECT 1 FROM public.users LIMIT 0' >/dev/null 2>&1; then
  echo 'Cannot connect with POSTGRES_RUNTIME_PASSWORD and read the application schema' >&2
  exit 1
fi
echo 'Database runtime role ready'
