#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${K_AI_BASE_URL:-http://127.0.0.1:8080}"
ADMIN_TOKEN="${K_AI_ADMIN_TOKEN:-change-me-admin-token}"

echo "==> Health"
curl -sf "$BASE_URL/health" | tee /tmp/kai-health.json
echo

echo "==> Create API key"
KEY_JSON=$(curl -sf -X POST "$BASE_URL/admin/api/v1/api-keys" \
  -H "X-Admin-Token: $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"http-test","scopes":["chat","models"]}')
echo "$KEY_JSON"
API_KEY=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["key"])' "$KEY_JSON")

echo "==> List models"
curl -sf "$BASE_URL/v1/models" -H "Authorization: Bearer $API_KEY" | tee /tmp/kai-models.json
echo

echo "==> Chat completion (mock path via alias if configured)"
CHAT_BODY='{"model":"mock-model","messages":[{"role":"user","content":"ping"}]}'
curl -sf -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d "$CHAT_BODY" | tee /tmp/kai-chat.json
echo

echo "==> HTTP smoke tests passed"
