#!/usr/bin/env sh
set -eu

PLAYBACK_URL="${PLAYBACK_URL:?defina PLAYBACK_URL com uma URL assinada}"
REQUESTS="${REQUESTS:-60}"
CONCURRENCY="${CONCURRENCY:-6}"
case "$REQUESTS" in ''|*[!0-9]*) exit 2;; esac
case "$CONCURRENCY" in ''|*[!0-9]*) exit 2;; esac
[ "$REQUESTS" -gt 0 ] && [ "$CONCURRENCY" -gt 0 ] || exit 2
failed=$(seq "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} sh -c 'curl --connect-timeout 3 --max-time 10 --fail --silent "$0" >/dev/null || printf "1\n"' "$PLAYBACK_URL" | wc -l | tr -d ' ')
printf 'playback requests=%s concurrency=%s failures=%s\n' "$REQUESTS" "$CONCURRENCY" "$failed"
[ "$failed" -eq 0 ]
