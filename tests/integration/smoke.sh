#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
printf 'Checking %s/healthz\n' "$BASE_URL"
curl --fail --silent --show-error "$BASE_URL/healthz" >/dev/null
printf 'API health check passed\n'
