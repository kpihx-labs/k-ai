#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="${K_AI_BASE_URL:-http://127.0.0.1:18080}"
PORT="${K_AI_PORT:-18080}"
ADMIN_TOKEN="${K_AI_ADMIN_TOKEN:-change-me-admin-token}"
LOG="/tmp/k-ai-live-test.log"
PIDFILE="/tmp/k-ai-live-test.pid"

cleanup() {
  if [[ -f "$PIDFILE" ]]; then
    kill "$(cat "$PIDFILE")" 2>/dev/null || true
    rm -f "$PIDFILE"
  fi
}
trap cleanup EXIT

echo "==> sync-env (optional keys from runtimes)"
chmod +x scripts/sync-env-from-runtimes.sh
./scripts/sync-env-from-runtimes.sh || true

echo "==> build"
make build

echo "==> start server on :$PORT"
mkdir -p data
set -a
[[ -f .env ]] && source .env
set +a
export K_AI_ADMIN_TOKEN="${K_AI_ADMIN_TOKEN:-$ADMIN_TOKEN}"
export K_AI_PORT="$PORT"
export K_AI_BASE_URL="$BASE_URL"
./bin/k-ai -config ./config/config.yaml >"$LOG" 2>&1 &
echo $! >"$PIDFILE"

for i in $(seq 1 30); do
  if curl -sf "$BASE_URL/health" >/dev/null; then
    break
  fi
  sleep 0.5
done
curl -sf "$BASE_URL/health" >/dev/null || { tail -20 "$LOG"; exit 1; }

echo "==> dashboard"
CODE=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/")
[[ "$CODE" == "200" ]] || { echo "dashboard HTTP $CODE"; exit 1; }

echo "==> http-test suite"
K_AI_BASE_URL="$BASE_URL" K_AI_ADMIN_TOKEN="$K_AI_ADMIN_TOKEN" ./scripts/http-test.sh

echo "==> streaming (mock)"
KEY_JSON=$(curl -sf -X POST "$BASE_URL/admin/api/v1/api-keys" \
  -H "X-Admin-Token: $K_AI_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"stream-test","scopes":["chat"]}')
API_KEY=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["key"])' "$KEY_JSON")
STREAM=$(curl -sf -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"mock-model","messages":[{"role":"user","content":"hi"}],"stream":true}')
echo "$STREAM" | grep -q 'chat.completion.chunk' || { echo "stream failed: $STREAM"; exit 1; }

if curl -sf "${K_AI_OLLAMA_BASE_URL:-http://127.0.0.1:11434/v1}/models" >/dev/null 2>&1; then
  echo "==> ollama alias route"
  OLLAMA_MODEL=$(curl -sf "${K_AI_OLLAMA_BASE_URL:-http://127.0.0.1:11434/v1}/models" | python3 -c 'import json,sys; data=json.load(sys.stdin); ids=[m["id"] for m in data.get("data",[])]; print(ids[0] if ids else "")')
  if [[ -n "$OLLAMA_MODEL" ]]; then
    curl -sf -X POST "$BASE_URL/v1/chat/completions" \
      -H "Authorization: Bearer $API_KEY" \
      -H "Content-Type: application/json" \
      -d "{\"model\":\"local-${OLLAMA_MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"say ok\"}],\"max_tokens\":8}" \
      | tee /tmp/kai-ollama-chat.json >/dev/null
    echo "ollama via alias local-${OLLAMA_MODEL}: OK"
  fi
else
  echo "==> ollama not reachable — skipped"
fi

echo "==> live tests passed"
