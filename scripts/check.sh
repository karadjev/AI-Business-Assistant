#!/bin/sh
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$PROJECT_ROOT/scripts/go-env.sh"
cd "$PROJECT_ROOT/apps/api"
UNFORMATTED=$(gofmt -l .)
if [ -n "$UNFORMATTED" ]; then echo "$UNFORMATTED"; exit 1; fi
go vet ./...
go test -race ./...
go build ./cmd/api ./cmd/worker ./cmd/migrate ./cmd/healthcheck
cd "$PROJECT_ROOT/apps/web"
npm run lint
npm run typecheck
npm run check:openapi
npm run build
