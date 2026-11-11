#!/usr/bin/env sh
set -eu
REDIS_URL="${REDIS_URL:-redis://localhost:6379}"
CHANNEL="${HA_CHANNEL:-phase22-ha-smoke}"
LOCK="vod2live:channel:${CHANNEL}:lock"
FENCE="vod2live:channel:${CHANNEL}:fence"
redis-cli -u "$REDIS_URL" DEL "$LOCK" "$FENCE" >/dev/null
first=$(redis-cli -u "$REDIS_URL" INCR "$FENCE")
second=$(redis-cli -u "$REDIS_URL" INCR "$FENCE")
[ "$second" -gt "$first" ]
[ "$(redis-cli -u "$REDIS_URL" SET "$LOCK" "worker-a:$first" NX EX 15)" = "OK" ]
[ "$(redis-cli -u "$REDIS_URL" SET "$LOCK" "worker-b:$second" NX EX 15)" = "" ]
[ "$(redis-cli -u "$REDIS_URL" GET "$LOCK")" = "worker-a:$first" ]
redis-cli -u "$REDIS_URL" DEL "$LOCK" "$FENCE" >/dev/null
printf 'HA lease/fencing smoke passed\n'
