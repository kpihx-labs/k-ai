# k-ai

```text
   _  _
  | |/ /__ _ _ __   ___ 
  | ' // _` | '_ \ / _ \
  |_|\_\__,_| .__/ \___/
            |_|  Sovereign LLM Gateway
```

> **100% Go · OpenAI-compatible · Homelab-first**
> **0% hardcode · flexible routing · alias rewrite engine**

KπX sovereign LLM gateway — unified downstream API for OpenCode, agents, and tools, with upstream routing to Ollama, OpenRouter, OpenCode Zen, Venice, Mistral, and more.

## Documentation stack

| Document | Role |
|----------|------|
| **[CONTRACT.md](CONTRACT.md)** | Source of truth — prod usage + dev rules |
| **[TODO.md](TODO.md)** | Roadmap and completion tracking |
| **[CHANGELOG.md](CHANGELOG.md)** | Version history |
| **[.agents/AGENTS.md](.agents/AGENTS.md)** | Agent context for this repo |

## Quick start

```bash
cd ~/KpihX-Labs/k_ai
make init
make sync-env      # optional: import keys from ~/.local/share/opencode/auth.json
make check         # build + tests
make run           # :8080
make live-test     # full real smoke (port 18080)
```

Dashboard: **http://127.0.0.1:8080/** — enter `K_AI_ADMIN_TOKEN` from `.env`.

## Docker (kpihx_labs pattern)

```bash
make init          # also copies docker-compose.override.yml
make up            # build + run (8081→8080 in dev override)
curl -sf http://127.0.0.1:8081/health
make down
```

Production serves via Traefik:

- `https://ai.kpihx-labs.com`
- `https://ai.homelab`

## API summary

| Endpoint | Auth |
|----------|------|
| `GET /health` | none |
| `GET /v1/models` | Bearer `kai_…` |
| `POST /v1/chat/completions` | Bearer `kai_…` |
| `/admin/api/v1/*` | `X-Admin-Token` |

See **CONTRACT.md** for alias prefixes, provider table, and streaming rules.

## Project layout

```
cmd/k-ai/              CLI entrypoint
internal/alias/        Alias engine
internal/admin/        Admin REST
internal/proxy/        OpenAI gateway + streaming
internal/resolver/     Provider model matching
internal/store/        SQLite
internal/web/          Embedded dashboard
config/config.yaml     Bootstrap defaults
scripts/               Tests + env sync + entrypoint
```

## License

Private — KπX Labs.
