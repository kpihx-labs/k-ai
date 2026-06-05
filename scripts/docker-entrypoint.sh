#!/bin/sh
set -e
mkdir -p /data
# Mock provider self-calls must stay in-container (see CONTRACT.md §1.8)
if [ -z "${K_AI_BASE_URL:-}" ]; then
  export K_AI_BASE_URL="http://127.0.0.1:8080"
fi
exec /app/k-ai "$@"
