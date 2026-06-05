#!/usr/bin/env bash
# Real-time verification matrix — prints PASS/FAIL per scenario with evidence paths.
set -euo pipefail

BASE="${K_AI_BASE_URL:-http://127.0.0.1:18090}"
ADMIN="${K_AI_ADMIN_TOKEN:-change-me-admin-token}"
REPORT="/tmp/k-ai-realtime-report.txt"
: > "$REPORT"

pass=0
fail=0

note() { echo "$1" | tee -a "$REPORT"; }
ok() { pass=$((pass+1)); note "PASS: $1"; }
ko() { fail=$((fail+1)); note "FAIL: $1"; [ -n "${2:-}" ] && note "  -> $2"; }

# Create API key
KEY_JSON=$(curl -sf -X POST "$BASE/admin/api/v1/api-keys" \
  -H "X-Admin-Token: $ADMIN" \
  -H "Content-Type: application/json" \
  -d '{"name":"realtime-matrix","scopes":["chat","models"]}') || { ko "create api key" "admin unreachable"; exit 1; }
API_KEY=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["key"])' "$KEY_JSON")

chat() {
  local model="$1"
  local outfile="$2"
  local code
  code=$(curl -s -o "$outfile" -w '%{http_code}' -X POST "$BASE/v1/chat/completions" \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: OK\"}],\"max_tokens\":16}")
  echo "$code"
}

note "=== k-ai REALTIME TEST MATRIX ==="
note "Base: $BASE"
note "Time: $(date -Iseconds)"
note ""

# 1 Health
curl -sf "$BASE/health" >/dev/null && ok "GET /health" || ko "GET /health"

# 2 Dashboard
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/")
[ "$code" = "200" ] && ok "GET / dashboard HTML" || ko "GET / dashboard" "HTTP $code"

# 3 Admin providers
n=$(curl -sf "$BASE/admin/api/v1/providers" -H "X-Admin-Token: $ADMIN" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["providers"]))')
[ "$n" -ge 7 ] && ok "Admin list providers ($n)" || ko "Admin providers" "count=$n"

# 4 Mock
c=$(chat "mock-model" /tmp/kai-rt-mock.json)
if [ "$c" = "200" ] && grep -q 'mock response' /tmp/kai-rt-mock.json; then ok "Chat mock-model (direct)"; else ko "Chat mock-model" "HTTP $c $(head -c 120 /tmp/kai-rt-mock.json)"; fi

# 5 Mock stream
stream=$(curl -sf -X POST "$BASE/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"model":"mock-model","messages":[{"role":"user","content":"hi"}],"stream":true}')
echo "$stream" | grep -q 'chat.completion.chunk' && ok "Stream mock-model SSE" || ko "Stream mock-model" "${stream:0:120}"

# 6 Ollama direct model
OLLAMA_MODEL=$(curl -sf http://127.0.0.1:11434/v1/models | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d["data"][0]["id"])')
c=$(chat "$OLLAMA_MODEL" /tmp/kai-rt-ollama-direct.json)
[ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-ollama-direct.json && ok "Chat Ollama direct ($OLLAMA_MODEL)" || ko "Chat Ollama direct" "HTTP $c"

# 7 Ollama via alias local-
c=$(chat "local-${OLLAMA_MODEL}" /tmp/kai-rt-ollama-alias.json)
[ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-ollama-alias.json && ok "Chat Ollama alias local-${OLLAMA_MODEL}" || ko "Chat Ollama alias" "HTTP $c"

# 8 OpenRouter via alias (keys from opencode sync)
c=$(chat "or-openai/gpt-4o-mini" /tmp/kai-rt-or.json)
if [ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-or.json; then
  ok "Chat OpenRouter alias or-openai/gpt-4o-mini"
else
  ko "Chat OpenRouter alias" "HTTP $c $(head -c 150 /tmp/kai-rt-or.json)"
fi

# 9 OpenCode Go via alias
c=$(chat "ocgo-minimax-m2.5" /tmp/kai-rt-ocgo.json)
if [ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-ocgo.json; then
  ok "Chat OpenCode Go alias ocgo-minimax-m2.5"
else
  ko "Chat OpenCode Go alias" "HTTP $c $(head -c 150 /tmp/kai-rt-ocgo.json)"
fi

# 10 OpenCode Zen via alias
c=$(chat "oc-deepseek-v4-flash" /tmp/kai-rt-oc.json)
if [ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-oc.json; then
  ok "Chat OpenCode alias oc-deepseek-v4-flash"
else
  ko "Chat OpenCode alias" "HTTP $c $(head -c 150 /tmp/kai-rt-oc.json)"
fi

# 11 Venice via alias
c=$(chat "v-llama-3.3-70b" /tmp/kai-rt-venice.json)
if [ "$c" = "200" ] && grep -q '"content"' /tmp/kai-rt-venice.json; then
  ok "Chat Venice alias v-llama-3.3-70b"
else
  ko "Chat Venice alias" "HTTP $c $(head -c 150 /tmp/kai-rt-venice.json)"
fi

# 12 Models catalog has ollama + alias examples
models=$(curl -sf "$BASE/v1/models" -H "Authorization: Bearer $API_KEY")
echo "$models" | grep -q 'local-example-model' && ok "/v1/models alias examples present" || ko "/v1/models alias examples"
echo "$models" | grep -q "$OLLAMA_MODEL" && ok "/v1/models includes Ollama model" || ko "/v1/models Ollama aggregate"

note ""
note "=== SUMMARY: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ]
