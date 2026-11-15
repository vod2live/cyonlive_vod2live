#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for the staging smoke test" >&2
  exit 1
fi

COMPOSE=(docker compose --profile local -f docker-compose.yml -f deployments/staging/docker-compose.staging.yml)
SERVICES=(postgres redis minio hls-fixture api playout media-worker)

cleanup() {
  if [ "${STAGING_SMOKE_KEEP_SERVICES:-0}" != "1" ]; then
    "${COMPOSE[@]}" down -v
  fi
}
trap cleanup EXIT

"${COMPOSE[@]}" up -d --wait --wait-timeout "${STAGING_SMOKE_WAIT_TIMEOUT:-120}" "${SERVICES[@]}"

for service in api playout media-worker; do
  expected="/usr/local/bin/service-$service"
  actual=$("${COMPOSE[@]}" exec -T "$service" sh -ec 'readlink /proc/1/exe')
  if [ "$actual" != "$expected" ]; then
    echo "staging service $service started $actual, expected $expected" >&2
    exit 1
  fi
  printf 'staging service %s healthy with %s\n' "$service" "$actual"
done

printf 'staging smoke passed\n'
