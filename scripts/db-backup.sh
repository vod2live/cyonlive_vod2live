#!/usr/bin/env sh
set -eu

OUTPUT="${1:-backups/vod2live-$(date -u +%Y%m%dT%H%M%SZ).dump}"
mkdir -p "$(dirname "$OUTPUT")"
umask 077
docker compose exec -T postgres pg_dump -Fc -U "${POSTGRES_USER:-vod2live}" -d "${POSTGRES_DB:-vod2live}" > "$OUTPUT"
[ -s "$OUTPUT" ] || { echo "backup vazio: $OUTPUT" >&2; exit 1; }
printf 'backup criado: %s\n' "$OUTPUT"
