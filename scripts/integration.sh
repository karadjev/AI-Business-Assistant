#!/bin/sh
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# Unique project name keeps concurrent test stacks isolated. Only ephemeral test resources are removed.
TEST_PROJECT="aba-test-$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')"
cleanup() { docker compose -p "$TEST_PROJECT" -f "$PROJECT_ROOT/infrastructure/compose.test.yml" down --volumes >/dev/null; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
docker compose -p "$TEST_PROJECT" -f "$PROJECT_ROOT/infrastructure/compose.test.yml" up --abort-on-container-exit --exit-code-from integration integration
