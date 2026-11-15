#!/usr/bin/env sh
set -eu
OUTPUT="${1:-backups/vod2live-$(date -u +%Y%m%dT%H%M%SZ).dump}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-0}"
OUTPUT_DIR=$(dirname "$OUTPUT")
OUTPUT_NAME=$(basename "$OUTPUT")
mkdir -p "$OUTPUT_DIR"
umask 077

case "$RETENTION_DAYS" in
  ''|*[!0-9]*) echo "BACKUP_RETENTION_DAYS deve ser um inteiro não negativo" >&2; exit 1 ;;
esac

TEMP_OUTPUT=$(mktemp "$OUTPUT_DIR/.${OUTPUT_NAME}.tmp.XXXXXX")
cleanup() { rm -f "$TEMP_OUTPUT"; }
trap cleanup EXIT INT TERM
docker compose exec -T postgres pg_dump -Fc -U "${POSTGRES_USER:-vod2live}" -d "${POSTGRES_DB:-vod2live}" > "$TEMP_OUTPUT"
[ -s "$TEMP_OUTPUT" ] || { echo "backup vazio: $OUTPUT" >&2; exit 1; }
mv -f "$TEMP_OUTPUT" "$OUTPUT"
(
  cd "$OUTPUT_DIR"
  sha256sum "$OUTPUT_NAME" > "$OUTPUT_NAME.sha256"
)

if [ "$RETENTION_DAYS" -gt 0 ]; then
  find "$OUTPUT_DIR" -maxdepth 1 -type f -name 'vod2live-*.dump' -mtime "+$RETENTION_DAYS" -print -delete | while IFS= read -r old; do
    rm -f "$old.sha256"
  done
fi

printf 'backup criado: %s\n' "$OUTPUT"
printf 'checksum criado: %s.sha256\n' "$OUTPUT"
