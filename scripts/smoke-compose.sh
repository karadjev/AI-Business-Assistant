#!/bin/sh
# Creates and deletes ONLY an isolated synthetic stack, never the user's dev volume.
set -eu
export LLM_EXTERNAL_REQUESTS_AUTHORIZED=false LLM_KILL_SWITCH=true LLM_PLATFORM_BUDGET_MICRORUB=0
export TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED=false TELEGRAM_SENDING_ENABLED=false TELEGRAM_SENDING_KILL_SWITCH=true
export LLM_BASE_URL= LLM_ALLOWED_BASE_URLS= TELEGRAM_WEBHOOK_BASE_URL= TELEGRAM_APPROVED_WEBHOOK_BASE_URL= TELEGRAM_TEST_USER_IDS=
export LLM_PROVIDER=mock LLM_ENABLE_LIVE=false LLM_API_KEY= LLM_MODEL=
export TELEGRAM_TRANSPORT=mock TELEGRAM_ENCRYPTION_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$PROJECT_ROOT"
TEST_PROJECT="aba-smoke-$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')"
export API_PORT=28080 WEB_PORT=23000 MAILPIT_PORT=28025 PUBLIC_ORIGIN=http://web:3000
compose() { docker compose --env-file .env.example -p "$TEST_PROJECT" "$@"; }
cleanup() { compose down --volumes >/dev/null; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose config --quiet
compose up --build --wait --wait-timeout 180
browser() {
 mkdir -p "$PROJECT_ROOT/.local/smoke/$TEST_PROJECT"
 docker run --rm --network "${TEST_PROJECT}_default" \
  --mount "type=bind,source=$PROJECT_ROOT/apps/web,target=/workspace,readonly" \
  --mount "type=bind,source=$PROJECT_ROOT/.local/smoke/$TEST_PROJECT,target=/tmp/results" \
  -w /workspace -e SMOKE_API_URL=http://api:8080 -e SMOKE_WEB_URL=http://web:3000 \
  -e SMOKE_REQUIRE_READY=1 -e TEST_MAILPIT_URL=http://mailpit:8025 -e "SMOKE_BACKEND_DOWN=${SMOKE_BACKEND_DOWN:-0}" \
  mcr.microsoft.com/playwright:v1.64.0-noble \
  node node_modules/@playwright/test/cli.js test --output=/tmp/results "$@"
}
browser --grep-invert 'реальный остановленный'
# Actual PostgreSQL outage: live stays 200, ready becomes 503.
compose stop postgres
compose exec -T web node --input-type=module <<'JS'
const ready=await fetch("http://api:8080/health/ready");
const live=await fetch("http://api:8080/health/live");
if(ready.status!==503||live.status!==200)throw new Error("DB outage health mismatch");
console.log("PASS: database outage ready=503, live=200");
JS
compose start postgres
compose stop api worker
for service in api worker; do
 container=$(compose ps -aq "$service")
 code=$(docker inspect --format '{{.State.ExitCode}}' "$container")
 if [ "$code" != 0 ]; then echo "$service shutdown failed: $code" >&2; exit 1; fi
done
SMOKE_BACKEND_DOWN=1 browser --grep 'реальный остановленный'
echo 'PASS: Compose smoke and graceful shutdown'
