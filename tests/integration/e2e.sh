#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
HLS_URL="${HLS_URL:-http://localhost:8000/master.m3u8}"
json_value() { python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[sys.argv[1]])' "$1"; }
wait_ready() { i=0; while [ "$i" -lt 30 ]; do if curl --fail --silent "$BASE_URL/readyz" >/dev/null; then return 0; fi; i=$((i+1)); sleep 2; done; echo "API não ficou pronta" >&2; return 1; }
wait_ready
asset=$(curl --fail --silent -X POST "$BASE_URL/v1/assets" -H 'Content-Type: application/json' -d "{\"title\":\"Fixture HLS\",\"hls_url\":\"$HLS_URL\"}")
asset_id=$(printf '%s' "$asset" | json_value id)
inspection=$(curl --fail --silent -X POST "$BASE_URL/v1/assets/$asset_id/validate")
printf '%s' "$inspection" | grep -q '"duration_ms":6000'
channel=$(curl --fail --silent -X POST "$BASE_URL/v1/channels" -H 'Content-Type: application/json' -d '{"name":"Canal E2E","slug":"canal-e2e"}')
channel_id=$(printf '%s' "$channel" | json_value id)
start=$(date -u -d '-1 minute' '+%Y-%m-%dT%H:%M:%SZ')
item=$(curl --fail --silent -X POST "$BASE_URL/v1/channels/$channel_id/schedule" -H 'Content-Type: application/json' -d "{\"asset_id\":\"$asset_id\",\"starts_at\":\"$start\",\"position\":1}")
item_id=$(printf '%s' "$item" | json_value id)
curl --fail --silent -X POST "$BASE_URL/v1/channels/$channel_id/start" >/dev/null
curl --fail --silent "$BASE_URL/v1/channels/$channel_id/status" | grep -q '"status":"running"'
master=$(curl --fail --silent "$BASE_URL/v1/channels/$channel_id/master.m3u8")
printf '%s' "$master" | grep -q '#EXTM3U'
media=$(curl --fail --silent "$BASE_URL/v1/channels/$channel_id/media/360p.m3u8")
printf '%s' "$media" | grep -q '#EXT-X-MEDIA-SEQUENCE'
curl --fail --silent -X PATCH "$BASE_URL/v1/schedule/$item_id" -H 'Content-Type: application/json' -d "{\"starts_at\":\"$start\",\"position\":2,\"enabled\":true}" | grep -q '"position":2'
curl --fail --silent -X DELETE "$BASE_URL/v1/schedule/$item_id" >/dev/null
printf 'Fase 1 E2E passed\n'
