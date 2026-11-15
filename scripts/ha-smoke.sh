#!/usr/bin/env sh
set -eu
REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
CHANNEL="${HA_CHANNEL:-phase22-ha-smoke}"
LOCK="vod2live:channel:${CHANNEL}:lock"
FENCE="vod2live:channel:${CHANNEL}:fence"
if command -v redis-cli >/dev/null 2>&1; then
  redis() { redis-cli "$@"; }
elif command -v docker >/dev/null 2>&1; then
  redis() { docker compose exec -T redis redis-cli "$@"; }
else
  echo "redis-cli ou Docker é necessário para o smoke de HA" >&2
  exit 127
fi
redis -u "$REDIS_URL" DEL "$LOCK" "$FENCE" >/dev/null
first=$(redis -u "$REDIS_URL" INCR "$FENCE")
second=$(redis -u "$REDIS_URL" INCR "$FENCE")
[ "$second" -gt "$first" ]
[ "$(redis -u "$REDIS_URL" SET "$LOCK" "worker-a:$first" NX EX 15)" = "OK" ]
[ "$(redis -u "$REDIS_URL" SET "$LOCK" "worker-b:$second" NX EX 15)" = "" ]
[ "$(redis -u "$REDIS_URL" GET "$LOCK")" = "worker-a:$first" ]
redis -u "$REDIS_URL" DEL "$LOCK" "$FENCE" >/dev/null
printf 'HA lease/fencing smoke passed\n'
