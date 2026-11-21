#!/bin/sh
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ ! -f "$PROJECT_ROOT/.env" ]; then echo 'Create .env from .env.example first' >&2; exit 1; fi
set -a
. "$PROJECT_ROOT/.env"
set +a
. "$PROJECT_ROOT/scripts/go-env.sh"
cd "$PROJECT_ROOT/apps/api"
exec go run ./cmd/worker
