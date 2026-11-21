#!/bin/sh
. "$(dirname -- "$0")/common.sh"
node "$BUILD_SCRIPT_DIR/artifacts.cjs" verify
node "$BUILD_SCRIPT_DIR/artifacts.cjs" served
cd "$ABA_WEB_ROOT"
exec node node_modules/@playwright/test/cli.js test "$@"
