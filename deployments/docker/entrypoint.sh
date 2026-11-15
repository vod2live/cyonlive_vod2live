#!/bin/sh
set -eu

case "${SERVICE:-api}" in
  api)
    exec /usr/local/bin/service-api "$@"
    ;;
  playout)
    exec /usr/local/bin/service-playout "$@"
    ;;
  media-worker)
    exec /usr/local/bin/service-media-worker "$@"
    ;;
  *)
    echo "unsupported SERVICE: ${SERVICE}" >&2
    exit 64
    ;;
esac
