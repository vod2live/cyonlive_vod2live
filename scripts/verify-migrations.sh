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

if [ -n "${DATABASE_URL:-}" ]; then
  if ! command -v psql >/dev/null 2>&1; then
    echo "psql não está disponível para DATABASE_URL" >&2
    exit 1
  fi
  psql_query() {
    psql -X -v ON_ERROR_STOP=1 -At "$DATABASE_URL" -c "$1"
  }
else
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker ou DATABASE_URL com psql é necessário para verificar migrations" >&2
    exit 1
  fi
  psql_query() {
    docker compose -f "$COMPOSE_FILE" exec -T "$POSTGRES_SERVICE" \
      psql -X -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$1"
  }
fi

if ! psql_query "SELECT 1 FROM pg_catalog.pg_class WHERE relname='schema_migrations' AND relkind='r'" | grep -qx 1; then
  echo "schema_migrations não existe; execute scripts/migrate.sh primeiro" >&2
  exit 1
fi

found=0
for migration in "$MIGRATIONS_DIR"/*.sql; do
  [ -f "$migration" ] || continue
  found=1
  version=$(basename "$migration")
  case "$version" in
    *[!A-Za-z0-9_.-]*) echo "nome de migration inválido: $version" >&2; exit 1 ;;
  esac
  checksum=$(sha256sum "$migration" | awk '{print $1}')
  applied_checksum=$(psql_query "SELECT checksum FROM schema_migrations WHERE version='$version'")
  if [ -z "$applied_checksum" ]; then
    echo "migration pendente: $version" >&2
    exit 1
  fi
  if [ "$applied_checksum" != "$checksum" ]; then
    echo "checksum divergente para $version" >&2
    exit 1
  fi
  printf 'verificada: %s\n' "$version"
done

if [ "$found" -eq 0 ]; then
  echo "nenhuma migration SQL encontrada em $MIGRATIONS_DIR" >&2
  exit 1
fi
printf 'migrações versionadas e checksums: OK (somente leitura)\n'
