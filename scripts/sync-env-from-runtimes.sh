#!/usr/bin/env bash
# Sync upstream API keys from local agent runtimes into .env (never prints secrets).
# Sources: opencode auth.json, openclaw .env, hermes auth.json — first non-empty wins.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$ROOT/.env"
EXAMPLE="$ROOT/.env.example"

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$EXAMPLE" "$ENV_FILE"
fi

set_kv() {
  local key="$1"
  local val="$2"
  if [[ -z "$val" ]]; then
    return
  fi
  if grep -q "^${key}=" "$ENV_FILE"; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
  else
    echo "${key}=${val}" >> "$ENV_FILE"
  fi
}

# Read API key from opencode auth.json
read_opencode_key() {
  local provider="$1"
  local auth="$HOME/.local/share/opencode/auth.json"
  [[ -f "$auth" ]] || return 0
  python3 -c "
import json, sys
with open('$auth') as f: data = json.load(f)
e = data.get('$provider', {})
k = e.get('key') or e.get('access') or ''
if k: print(k, end='')
" 2>/dev/null || true
}

# Read API key from openclaw .env
read_openclaw_key() {
  local var_name="$1"
  local env_file="$HOME/.openclaw/.env"
  [[ -f "$env_file" ]] || return 0
  while IFS='=' read -r k v; do
    if [[ "$k" == "$var_name" && -n "$v" ]]; then
      printf '%s' "$v"
      return
    fi
  done < "$env_file"
}

# Read API key from hermes auth.json
read_hermes_key() {
  local provider="$1"
  local auth="$HOME/.hermes/auth.json"
  [[ -f "$auth" ]] || return 0
  python3 -c "
import json, sys
with open('$auth') as f: data = json.load(f)
items = data.get('$provider', [])
if items and isinstance(items, list):
    k = items[0].get('api_key') or items[0].get('key') or ''
    if k: print(k, end='')
" 2>/dev/null || true
}

# Helper: set key from first non-empty source
set_first() {
  local env_key="$1"
  shift
  for val in "$@"; do
    if [[ -n "$val" ]]; then
      set_kv "$env_key" "$val"
      return
    fi
  done
}

set_first "K_AI_OPENROUTER_API_KEY" \
  "$(read_opencode_key openrouter)" \
  "$(read_openclaw_key OPENROUTER_API_KEY)" \
  "$(read_hermes_key openrouter)"

set_first "K_AI_OPENCODE_API_KEY" \
  "$(read_opencode_key opencode)" \
  "$(read_openclaw_key OPENCODE_API_KEY)"

set_first "K_AI_OPENCODE_GO_API_KEY" \
  "$(read_opencode_key opencode-go)" \
  "$(read_hermes_key opencode-go)"

set_first "K_AI_VENICE_API_KEY" \
  "$(read_opencode_key venice)"

set_first "K_AI_MISTRAL_API_KEY" \
  "$(read_opencode_key mistral)"

echo "Synced provider keys into $ENV_FILE (values not printed)."
# Audit: show which keys are set (lengths only)
python3 -c "
for line in open('$ENV_FILE'):
    if '=' in line and line.strip() and not line.startswith('#'):
        k, v = line.strip().split('=', 1)
        if 'KEY' in k or 'TOKEN' in k:
            print(f'  {k}: {\"SET\" if v else \"EMPTY\"} (len={len(v)})')
"
