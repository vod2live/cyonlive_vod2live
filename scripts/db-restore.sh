#!/usr/bin/env sh
set -eu

BACKUP_FILE="${1:?uso: $0 BACKUP_FILE}"
[ -f "$BACKUP_FILE" ] || { echo "arquivo não encontrado: $BACKUP_FILE" >&2; exit 1; }
[ "${CONFIRM_RESTORE:-}" = "YES" ] || { echo "operação destrutiva: defina CONFIRM_RESTORE=YES" >&2; exit 1; }
docker compose exec -T postgres pg_restore --clean --if-exists --no-owner --exit-on-error -U "${POSTGRES_USER:-vod2live}" -d "${POSTGRES_DB:-vod2live}" < "$BACKUP_FILE"
printf 'restore concluído: %s\n' "$BACKUP_FILE"
