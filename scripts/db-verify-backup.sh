#!/usr/bin/env sh
set -eu
BACKUP_FILE="${1:?uso: $0 BACKUP_FILE}"
[ -f "$BACKUP_FILE" ] || { echo "arquivo não encontrado: $BACKUP_FILE" >&2; exit 1; }
[ -s "$BACKUP_FILE" ] || { echo "arquivo vazio: $BACKUP_FILE" >&2; exit 1; }
BACKUP_DIR=$(dirname "$BACKUP_FILE")
BACKUP_NAME=$(basename "$BACKUP_FILE")
CHECKSUM_FILE="$BACKUP_FILE.sha256"
if [ -f "$CHECKSUM_FILE" ]; then
  (cd "$BACKUP_DIR" && sha256sum -c "$BACKUP_NAME.sha256")
elif [ "${REQUIRE_BACKUP_CHECKSUM:-false}" = "true" ]; then
  echo "checksum ausente: $CHECKSUM_FILE" >&2
  exit 1
else
  echo "aviso: checksum ausente: $CHECKSUM_FILE" >&2
fi
docker compose run --rm -T --no-deps postgres pg_restore --list < "$BACKUP_FILE" >/dev/null
printf 'backup válido: %s\n' "$BACKUP_FILE"
