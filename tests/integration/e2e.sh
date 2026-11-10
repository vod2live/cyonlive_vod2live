#!/usr/bin/env sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8080}"
HLS_URL="${HLS_URL:-http://localhost:8000/master.m3u8}"
API_KEY="${API_KEY:-phase2-local-key}"
ADMIN_EMAIL="${PHASE14_ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${PHASE14_ADMIN_PASSWORD:-phase14-test-password}"
json_value() { python3 -c 'import json,sys; d=json.load(sys.stdin); print(d[sys.argv[1]])' "$1"; }
wait_ready() { i=0; while [ "$i" -lt 30 ]; do if curl --fail --silent "$BASE_URL/readyz" >/dev/null; then return 0; fi; i=$((i+1)); sleep 2; done; echo "API não ficou pronta" >&2; return 1; }
wait_ready
bootstrap=$(curl --fail --silent -X POST "$BASE_URL/v1/auth/bootstrap" -H "X-API-Key: $API_KEY" -H 'Content-Type: application/json' -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\",\"organization_name\":\"Organização E2E\"}")
ADMIN_TOKEN=$(printf '%s' "$bootstrap" | json_value access_token)
ORG_ID=$(printf '%s' "$bootstrap" | json_value organization_id)
AUTH_HEADER="Authorization: Bearer $ADMIN_TOKEN"
printf '%s' "$ORG_ID" > /tmp/vod2live-phase14-org-id

asset=$(curl --fail --silent -X POST "$BASE_URL/v1/assets" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d "{\"title\":\"Fixture HLS\",\"hls_url\":\"$HLS_URL\"}")
asset_id=$(printf '%s' "$asset" | json_value id)
inspection=$(curl --fail --silent -X POST "$BASE_URL/v1/assets/$asset_id/validate" -H "$AUTH_HEADER")
printf '%s' "$inspection" | grep -q '"duration_ms":6000'
channel=$(curl --fail --silent -X POST "$BASE_URL/v1/channels" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d '{"name":"Canal E2E","slug":"canal-e2e"}')
channel_id=$(printf '%s' "$channel" | json_value id)
start=$(date -u -d '-1 minute' '+%Y-%m-%dT%H:%M:%SZ')
item=$(curl --fail --silent -X POST "$BASE_URL/v1/channels/$channel_id/schedule" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d "{\"asset_id\":\"$asset_id\",\"starts_at\":\"$start\",\"position\":1}")
item_id=$(printf '%s' "$item" | json_value id)
curl --fail --silent -X POST "$BASE_URL/v1/channels/$channel_id/start" -H "$AUTH_HEADER" >/dev/null
curl --fail --silent "$BASE_URL/v1/channels/$channel_id/status" -H "$AUTH_HEADER" | grep -q '"status":"running"'
master=$(curl --fail --silent "$BASE_URL/v1/channels/$channel_id/master.m3u8" -H "$AUTH_HEADER")
printf '%s' "$master" | grep -q '#EXTM3U'
media=$(curl --fail --silent "$BASE_URL/v1/channels/$channel_id/media/360p.m3u8" -H "$AUTH_HEADER")
printf '%s' "$media" | grep -q '#EXT-X-MEDIA-SEQUENCE'
playback=$(curl --fail --silent -X POST "$BASE_URL/v1/channels/$channel_id/playback" -H "$AUTH_HEADER")
playback_url=$(printf '%s' "$playback" | json_value playback_url)
playback_master=$(curl --fail --silent "$BASE_URL$playback_url")
printf '%s' "$playback_master" | grep -q '#EXTM3U'
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL/playback/$channel_id/master.m3u8")" != "403" ]; then echo 'playback sem token deveria retornar 403' >&2; exit 1; fi
curl --fail --silent -X PATCH "$BASE_URL/v1/schedule/$item_id" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d "{\"starts_at\":\"$start\",\"position\":2,\"enabled\":true}" | grep -q '"position":2'
curl --fail --silent -X DELETE "$BASE_URL/v1/schedule/$item_id" -H "$AUTH_HEADER" >/dev/null

viewer=$(curl --fail --silent -X POST "$BASE_URL/v1/users" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d '{"email":"viewer@example.com","password":"viewer-phase14-password","role":"visualizador"}')
viewer_id=$(printf '%s' "$viewer" | json_value id)
team=$(curl --fail --silent -X POST "$BASE_URL/v1/teams" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d '{"name":"Operações E2E"}')
team_id=$(printf '%s' "$team" | json_value id)
curl --fail --silent -X POST "$BASE_URL/v1/teams/$team_id/members" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d "{\"user_id\":\"$viewer_id\"}" >/dev/null
viewer_login=$(curl --fail --silent -X POST "$BASE_URL/v1/auth/login" -H 'Content-Type: application/json' -d '{"email":"viewer@example.com","password":"viewer-phase14-password"}')
VIEWER_TOKEN=$(printf '%s' "$viewer_login" | json_value access_token)
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "$BASE_URL/v1/channels" -H "Authorization: Bearer $VIEWER_TOKEN" -H 'Content-Type: application/json' -d '{"name":"Negado"}')" != "403" ]; then echo 'visualizador conseguiu criar canal' >&2; exit 1; fi

other_org=$(curl --fail --silent -X POST "$BASE_URL/v1/organizations" -H "$AUTH_HEADER" -H 'Content-Type: application/json' -d '{"name":"Outra organização E2E","slug":"outra-organizacao-e2e"}')
other_org_id=$(printf '%s' "$other_org" | json_value id)
other_login=$(curl --fail --silent -X POST "$BASE_URL/v1/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\",\"organization_id\":\"$other_org_id\"}")
OTHER_TOKEN=$(printf '%s' "$other_login" | json_value access_token)
curl --fail --silent -X POST "$BASE_URL/v1/users" -H "Authorization: Bearer $OTHER_TOKEN" -H 'Content-Type: application/json' -d '{"email":"viewer@example.com","role":"visualizador"}' >/dev/null
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "$BASE_URL/v1/auth/login" -H 'Content-Type: application/json' -d '{"email":"viewer@example.com","password":"viewer-phase14-password"}')" != "400" ]; then echo 'login multi-organização deveria solicitar organization_id' >&2; exit 1; fi
other_viewer_login=$(curl --fail --silent -X POST "$BASE_URL/v1/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"viewer@example.com\",\"password\":\"viewer-phase14-password\",\"organization_id\":\"$other_org_id\"}")
OTHER_VIEWER_TOKEN=$(printf '%s' "$other_viewer_login" | json_value access_token)
if [ "$(curl --silent --output /dev/null --write-out '%{http_code}' "$BASE_URL/v1/channels/$channel_id" -H "Authorization: Bearer $OTHER_VIEWER_TOKEN")" != "404" ]; then echo 'isolamento entre organizações falhou' >&2; exit 1; fi
printf 'Fase 14 identity, RBAC e tenant E2E passed\n'
