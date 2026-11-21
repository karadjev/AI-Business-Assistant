#!/bin/sh
. "$(dirname -- "$0")/common.sh"
sh "$BUILD_SCRIPT_DIR/check-go.sh"
export CGO_ENABLED=1
cd "$ABA_API_DIR"
exec go test -race -count=1 "$@" ./...
