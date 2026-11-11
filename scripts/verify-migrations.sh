#!/usr/bin/env sh
set -eu

for migration in /docker-entrypoint-initdb.d/*.sql; do
  echo "aplicando novamente: $migration"
  docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER:-vod2live}" -d "${POSTGRES_DB:-vod2live}" -f "$migration" >/dev/null
done
printf 'migrações reaplicáveis: OK\n'
