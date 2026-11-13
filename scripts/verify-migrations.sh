#!/usr/bin/env sh
set -eu

scripts/migrate.sh
printf 'migrações versionadas e checksums: OK\n'
