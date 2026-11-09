#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-phase2-local-key}"
headers=$(mktemp)
trap 'rm -f "$headers"' EXIT
curl --fail --silent --show-error -D "$headers" "$BASE_URL/healthz" >/dev/null
grep -qi '^x-request-id: ' "$headers"
grep -qi '^x-content-type-options: nosniff' "$headers"
if curl --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL/v1/channels" | grep -q '^401$'; then :; else echo 'endpoint protegido não retornou 401' >&2; exit 1; fi
curl --fail --silent -H "X-API-Key: $API_KEY" "$BASE_URL/v1/channels" >/dev/null
curl --fail --silent "$BASE_URL/metrics" | grep -q 'vod2live_http_requests_total'
printf 'API contract checks passed\n'
