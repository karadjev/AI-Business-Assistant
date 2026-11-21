#!/bin/sh
. "$(dirname -- "$0")/common.sh"
exec node "$BUILD_SCRIPT_DIR/../check-next-offline.cjs"
