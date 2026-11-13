#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vod2live}"
POSTGRES_DB="${POSTGRES_DB:-vod2live}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-migrations}"

psql() {
  docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$@"
}

if ! command -v docker >/dev/null 2>&1; then
  echo "docker não está disponível" >&2
  exit 1
fi

psql -c 'CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());' >/dev/null
psql -c 'SELECT pg_advisory_lock(hashtext('\''vod2live:migrations'\''));' >/dev/null
trap 'psql -c '\''SELECT pg_advisory_unlock(hashtext('\''vod2live:migrations'\''));'\'' >/dev/null 2>&1 || true' EXIT INT TERM

for migration in "$MIGRATIONS_DIR"/*.sql; do
  version=$(basename "$migration")
  checksum=$(sha256sum "$migration" | awk '{print $1}')
  applied=$(psql -tAc "SELECT checksum FROM schema_migrations WHERE version=$(printf "'%s'" "$version");" | tr -d '[:space:]')
  if [ -n "$applied" ]; then
    if [ "$applied" != "$checksum" ]; then
      echo "checksum divergente para $version" >&2
      exit 1
    fi
    echo "já aplicada: $version"
    continue
  fi
  echo "aplicando: $version"
  docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$migration"
  psql -c "INSERT INTO schema_migrations(version, checksum) VALUES ($(printf "'%s'" "$version"), $(printf "'%s'" "$checksum"));" >/dev/null
done

echo "migrações versionadas: OK"
