#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://localhost:8080}"
REQUESTS="${REQUESTS:-120}"
CONCURRENCY="${CONCURRENCY:-8}"

case "$REQUESTS" in ''|*[!0-9]*) echo 'REQUESTS deve ser inteiro positivo' >&2; exit 2;; esac
case "$CONCURRENCY" in ''|*[!0-9]*) echo 'CONCURRENCY deve ser inteiro positivo' >&2; exit 2;; esac
[ "$REQUESTS" -gt 0 ] || { echo 'REQUESTS deve ser positivo' >&2; exit 2; }
[ "$CONCURRENCY" -gt 0 ] || { echo 'CONCURRENCY deve ser positivo' >&2; exit 2; }

run_one() {
  endpoint="$1"
  code=$(curl --connect-timeout 3 --max-time 10 --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL$endpoint" || printf '000')
  [ "$code" = "200" ] || { printf '%s %s\n' "$endpoint" "$code"; return 1; }
}
export BASE_URL
export -f run_one 2>/dev/null || true

failures=0
for endpoint in /healthz /readyz /version; do
  if command -v xargs >/dev/null 2>&1; then
    failed=$(seq "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} sh -c 'code=$(curl --connect-timeout 3 --max-time 10 --silent --output /dev/null --write-out "%{http_code}" "$BASE_URL$0" || printf "000"); [ "$code" = "200" ] || printf "%s\n" "$code"' "$endpoint" | wc -l | tr -d ' ')
  else
    failed=0
    i=0
    while [ "$i" -lt "$REQUESTS" ]; do run_one "$endpoint" || failed=$((failed+1)); i=$((i+1)); done
  fi
  printf '%s requests=%s concurrency=%s failures=%s\n' "$endpoint" "$REQUESTS" "$CONCURRENCY" "$failed"
  failures=$((failures+failed))
done
[ "$failures" -eq 0 ] || { echo "load test failed: $failures request(s)" >&2; exit 1; }
printf 'load test passed\n'
