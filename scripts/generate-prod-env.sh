#!/usr/bin/env bash
# Generate .env for homelab deploy (GitLab CI or manual). Never echo secrets.
set -euo pipefail

ENV_FILE="${1:-.env}"

write() {
  local key="$1"
  local val="${2:-}"
  if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
    sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
  else
    echo "${key}=${val}" >> "$ENV_FILE"
  fi
}

if [[ ! -f "$ENV_FILE" ]]; then
  cp .env.example "$ENV_FILE"
fi

write "K_AI_HOST" "0.0.0.0"
write "K_AI_PORT" "8080"
write "K_AI_BASE_URL" "http://127.0.0.1:8080"
write "K_AI_CONFIG_PATH" "./config/config.yaml"
write "K_AI_DATA_DIR" "./data"
write "K_AI_OLLAMA_BASE_URL" "${K_AI_OLLAMA_BASE_URL:-http://host.docker.internal:11434/v1}"
write "K_AI_ADMIN_TOKEN" "${K_AI_ADMIN_TOKEN:-change-me-admin-token}"
write "K_AI_OPENROUTER_API_KEY" "${K_AI_OPENROUTER_API_KEY:-}"
write "K_AI_OPENCODE_API_KEY" "${K_AI_OPENCODE_API_KEY:-}"
write "K_AI_OPENCODE_GO_API_KEY" "${K_AI_OPENCODE_GO_API_KEY:-}"
write "K_AI_VENICE_API_KEY" "${K_AI_VENICE_API_KEY:-}"
write "K_AI_MISTRAL_API_KEY" "${K_AI_MISTRAL_API_KEY:-}"

echo "Wrote deploy env to $ENV_FILE (secrets not printed)."
