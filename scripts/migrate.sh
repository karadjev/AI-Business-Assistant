#!/bin/sh
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$PROJECT_ROOT"
case "${1:-up}" in up|down|status) ;; *) echo 'Usage: scripts/migrate.sh up|down|status' >&2; exit 1;; esac
exec docker compose run --rm migrate "${1:-up}"
