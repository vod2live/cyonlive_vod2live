#!/usr/bin/env sh
set -eu
LC_ALL=C
export LC_ALL

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vod2live}"
POSTGRES_DB="${POSTGRES_DB:-vod2live}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-migrations}"

if [ ! -d "$MIGRATIONS_DIR" ]; then
  echo "diretório de migrações não encontrado: $MIGRATIONS_DIR" >&2
  exit 1
fi

SQL_FILE=$(mktemp)
trap 'rm -f "$SQL_FILE"' EXIT INT TERM
{
  cat <<'SQL'
\set ON_ERROR_STOP on
SELECT pg_advisory_lock(hashtext('vod2live:migrations'));
CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  checksum TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL
  found=0
  for migration in "$MIGRATIONS_DIR"/*.sql; do
    [ -f "$migration" ] || continue
    found=1
    version=$(basename "$migration")
    case "$version" in
      *[!A-Za-z0-9_.-]*) echo "nome de migration inválido: $version" >&2; exit 1 ;;
    esac
    checksum=$(sha256sum "$migration" | awk '{print $1}')
    cat <<SQL
SELECT CASE WHEN EXISTS(SELECT 1 FROM schema_migrations WHERE version='$version') THEN 'true' ELSE 'false' END AS already_applied,
       CASE WHEN (SELECT checksum FROM schema_migrations WHERE version='$version') = '$checksum' THEN 'true' ELSE 'false' END AS checksum_matches \gset
\if :already_applied
\if :checksum_matches
\echo já aplicada: $version
\else
\echo checksum divergente para $version
\quit 3
\endif
\else
\echo aplicando: $version
SQL
    cat "$migration"
    cat <<SQL
INSERT INTO schema_migrations(version, checksum) VALUES ('$version', '$checksum');
\endif
SQL
  done
  if [ "$found" -eq 0 ]; then
    echo "nenhuma migration SQL encontrada em $MIGRATIONS_DIR" >&2
    exit 1
  fi
  cat <<'SQL'
SELECT pg_advisory_unlock(hashtext('vod2live:migrations'));
SQL
} > "$SQL_FILE"

if [ -n "${DATABASE_URL:-}" ]; then
  if ! command -v psql >/dev/null 2>&1; then
    echo "psql não está disponível para DATABASE_URL" >&2
    exit 1
  fi
  psql -X -v ON_ERROR_STOP=1 "$DATABASE_URL" -f "$SQL_FILE"
else
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker ou DATABASE_URL com psql é necessário para executar migrations" >&2
    exit 1
  fi
  docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
    psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" < "$SQL_FILE"
fi

echo "migrações versionadas e checksums: OK"
