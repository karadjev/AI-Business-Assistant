#!/bin/sh
set -eu
export LLM_EXTERNAL_REQUESTS_AUTHORIZED=false LLM_KILL_SWITCH=true LLM_PLATFORM_BUDGET_MICRORUB=0
export TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED=false TELEGRAM_SENDING_ENABLED=false TELEGRAM_SENDING_KILL_SWITCH=true
export LLM_BASE_URL= LLM_ALLOWED_BASE_URLS= TELEGRAM_WEBHOOK_BASE_URL= TELEGRAM_APPROVED_WEBHOOK_BASE_URL= TELEGRAM_TEST_USER_IDS=
export LLM_PROVIDER=mock LLM_ENABLE_LIVE=false LLM_API_KEY= LLM_MODEL=
export TELEGRAM_TRANSPORT=mock TELEGRAM_ENCRYPTION_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$PROJECT_ROOT/scripts/go-env.sh"
mkdir -p "$PROJECT_ROOT/.local"
cd "$PROJECT_ROOT/apps/api"
go build -o "$PROJECT_ROOT/.local/api" ./cmd/api
go build -o "$PROJECT_ROOT/.local/worker" ./cmd/worker
API_PID=''; WORKER_PID=''; WEB_PID=''
cleanup() {
 for proc in "$API_PID" "$WORKER_PID" "$WEB_PID"; do
  if [ -n "$proc" ]; then kill -TERM "$proc" 2>/dev/null || true; fi
 done
 for proc in "$API_PID" "$WORKER_PID" "$WEB_PID"; do
  if [ -n "$proc" ]; then wait "$proc" 2>/dev/null || true; fi
 done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
: "${DATABASE_URL:?Set DATABASE_URL to aba_app (never a privileged role)}"
export HTTP_ADDR=127.0.0.1:18080
export PUBLIC_ORIGIN=http://127.0.0.1:13000
export API_BASE_URL=http://127.0.0.1:18080
export SMOKE_API_URL=http://127.0.0.1:18080
export SMOKE_WEB_URL=http://127.0.0.1:13000
"$PROJECT_ROOT/.local/api" > "$PROJECT_ROOT/.local/api-smoke.log" 2>&1 & API_PID=$!
"$PROJECT_ROOT/.local/worker" > "$PROJECT_ROOT/.local/worker-smoke.log" 2>&1 & WORKER_PID=$!
cd "$PROJECT_ROOT/apps/web"
node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port 13000 > "$PROJECT_ROOT/.local/web-smoke.log" 2>&1 & WEB_PID=$!
node --input-type=module <<'JS'
const endpoints=[process.env.SMOKE_API_URL+"/health/live",process.env.SMOKE_WEB_URL+"/"];
for(const url of endpoints){let ready=false;for(let i=0;i<40;i++){try{const r=await fetch(url,{signal:AbortSignal.timeout(1000)});if(r.ok){ready=true;break;}}catch{}await new Promise(r=>setTimeout(r,250));}if(!ready)throw new Error("Smoke service did not start");}
JS
npm run test:smoke
# Stop backend for a REAL server-to-server unavailable check (not only browser mocking).
kill -TERM "$API_PID"
wait "$API_PID"
API_PID=''
SMOKE_BACKEND_DOWN=1 npm run test:smoke -- --grep 'реальный остановленный'
node --input-type=module <<'JS'
const response=await fetch(process.env.SMOKE_WEB_URL+"/api/backend-status");
const data=await response.json();
if(response.status!==503||data.kind!=="unavailable")throw new Error("Unavailable backend returned fabricated success");
console.log("PASS: real backend shutdown -> frontend 503 unavailable");
JS
kill -TERM "$WORKER_PID"
wait "$WORKER_PID"
WORKER_PID=''
echo 'PASS: API and worker exit cleanly after SIGTERM'
