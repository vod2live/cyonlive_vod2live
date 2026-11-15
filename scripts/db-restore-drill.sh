#!/usr/bin/env sh
set -eu

BACKUP_FILE="${1:?uso: $0 BACKUP_FILE}"
[ -f "$BACKUP_FILE" ] || { echo "arquivo não encontrado: $BACKUP_FILE" >&2; exit 1; }
[ -s "$BACKUP_FILE" ] || { echo "arquivo vazio: $BACKUP_FILE" >&2; exit 1; }

POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-vod2live}"
DB_NAME="${RESTORE_DRILL_DB:-vod2live_restore_drill_$(date -u +%Y%m%d%H%M%S)}"
case "$DB_NAME" in
  ''|*[!A-Za-z0-9_]*|[0-9]*) echo "RESTORE_DRILL_DB deve ser um identificador PostgreSQL simples" >&2; exit 1 ;;
esac

created=0
started=$(date +%s)
cleanup() {
  if [ "$created" -eq 1 ] && [ "${RESTORE_DRILL_KEEP_DB:-0}" != "1" ]; then
    docker compose exec -T "$POSTGRES_SERVICE" dropdb -U "$POSTGRES_USER" --if-exists "$DB_NAME" >/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

./scripts/db-verify-backup.sh "$BACKUP_FILE"
docker compose exec -T "$POSTGRES_SERVICE" createdb -U "$POSTGRES_USER" "$DB_NAME"
created=1
docker compose exec -T "$POSTGRES_SERVICE" pg_restore --no-owner --exit-on-error --single-transaction \
  -U "$POSTGRES_USER" -d "$DB_NAME" < "$BACKUP_FILE"
docker compose exec -T "$POSTGRES_SERVICE" psql -X -v ON_ERROR_STOP=1 -At \
  -U "$POSTGRES_USER" -d "$DB_NAME" -c 'SELECT 1' | grep -qx 1
finished=$(date +%s)
backup_epoch=$(stat -c %Y "$BACKUP_FILE")
rto_seconds=$((finished - started))
rpo_age_seconds=$((finished - backup_epoch))
printf 'restore drill passed: database=%s rto_seconds=%s rpo_age_seconds=%s\n' \
  "$DB_NAME" "$rto_seconds" "$rpo_age_seconds"
