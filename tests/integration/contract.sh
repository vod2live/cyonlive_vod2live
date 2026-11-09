#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-phase2-local-key}"
ADMIN_EMAIL="${PHASE14_ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${PHASE14_ADMIN_PASSWORD:-phase14-test-password}"
headers=$(mktemp)
trap 'rm -f "$headers"' EXIT
curl --fail --silent --show-error -D "$headers" "$BASE_URL/healthz" >/dev/null
grep -qi '^x-request-id: ' "$headers"
grep -qi '^x-content-type-options: nosniff' "$headers"
if curl --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL/v1/channels" | grep -q '^401$'; then :; else echo 'endpoint protegido não retornou 401' >&2; exit 1; fi
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' -H "X-API-Key: $API_KEY" "$BASE_URL/v1/channels")" != "401" ]; then echo 'API key legada ainda acessa endpoints de negócio' >&2; exit 1; fi
ORG_ID="${ORG_ID:-}"
if [ -z "$ORG_ID" ] && [ -f /tmp/vod2live-phase14-org-id ]; then ORG_ID=$(cat /tmp/vod2live-phase14-org-id); fi
if [ -n "$ORG_ID" ]; then
  login_body=$(printf '{"email":"%s","password":"%s","organization_id":"%s"}' "$ADMIN_EMAIL" "$ADMIN_PASSWORD" "$ORG_ID")
else
  login_body=$(printf '{"email":"%s","password":"%s"}' "$ADMIN_EMAIL" "$ADMIN_PASSWORD")
fi
login=$(curl --fail --silent -X POST "$BASE_URL/v1/auth/login" -H 'Content-Type: application/json' -d "$login_body")
TOKEN=$(printf '%s' "$login" | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
curl --fail --silent -H "Authorization: Bearer $TOKEN" "$BASE_URL/v1/channels" >/dev/null
curl --fail --silent "$BASE_URL/metrics" | grep -q 'vod2live_http_requests_total'
curl --fail --silent "$BASE_URL/version" | grep -q '"version"'
printf 'API contract checks passed\n'
