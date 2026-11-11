#!/usr/bin/env sh
set -eu

BACKUP_FILE="${1:?uso: $0 BACKUP_FILE}"
[ -f "$BACKUP_FILE" ] || { echo "arquivo não encontrado: $BACKUP_FILE" >&2; exit 1; }
[ -s "$BACKUP_FILE" ] || { echo "arquivo vazio: $BACKUP_FILE" >&2; exit 1; }
docker compose run --rm -T --no-deps postgres pg_restore --list < "$BACKUP_FILE" >/dev/null
printf 'backup válido: %s\n' "$BACKUP_FILE"
