# CONTRACT.md — k-ai

> **Sovereign LLM Gateway — usage contract & development guide**
> **0% hardcode · 100% flexibility · OpenAI-compatible downstream**

---

## Facet 1: PROD (Usage Contract)

This facet defines how operators and AI agents interact with **k-ai** at `ai.kpihx-labs.com` or locally.

### 1.1 Installation flows (symmetric)

#### A. Homelab / Docker (primary)

```bash
git clone git@gitlab.com:kpihx-labs/k-ai.git   # or GitHub mirror
cd k-ai
make init          # .env + docker-compose.override.yml
make sync-env      # optional: pull cloud keys from OpenCode/Hermes runtimes
make up            # docker compose up -d --build
curl -sf http://127.0.0.1:8081/health         # via override port in dev
```

Teardown:

```bash
make down
```

#### B. Native Go (dev / debug)

```bash
make init
make sync-env
make run           # listens on K_AI_PORT (default 8080)
make live-test     # full smoke + Ollama + streaming (port 18080)
```

### 1.2 Public surfaces

| Surface | Path | Auth | Role |
|---------|------|------|------|
| Health | `GET /health` | none | Liveness |
| OpenAI API | `GET /v1/models` | Bearer API key | Model catalog |
| OpenAI API | `POST /v1/chat/completions` | Bearer API key | Chat (+ SSE stream) |
| Admin REST | `/admin/api/v1/*` | `X-Admin-Token` | CRUD providers, aliases, keys |
| Dashboard | `GET /` | none (admin token in UI) | Operators UI + playground |

Downstream clients (OpenCode, Cursor, Hermes, curl) MUST use:

```http
Authorization: Bearer kai_<secret>
```

Admin automation MUST use:

```http
X-Admin-Token: <K_AI_ADMIN_TOKEN>
```

### 1.3 Routing contract (non-negotiable)

Resolution order for `model` in chat requests:

1. **Alias engine** — rewrite slug → upstream model + optional `provider_id`
2. **Provider resolver** — match upstream model against provider rules
3. **Rule priority** — `exact` > `regex` > `glob` (catch-all `*` never wins over exact)

Alias rewrite templates:

| Token | Meaning |
|-------|---------|
| `${model}` | Requested slug after prefix/suffix strip |
| `${provider}` | Target provider id |
| `${1}`, `${2}` | Regex capture groups |
| `${name}` | Named regex group |

Configured alias prefixes (bootstrap defaults):

| Slug pattern | Provider |
|--------------|----------|
| `local-*` / `local-<model>` (regex) | `ollama-local` |
| `ollama-*` | `ollama-local` |
| `or-*` | `openrouter` |
| `oc-*` | `opencode` |
| `ocgo-*` | `opencode-go` |
| `v-*` | `venice` |

Direct upstream model ids also work when they match a provider rule.

### 1.4 Provider registry (bootstrap)

| ID | Upstream base | Key env var |
|----|---------------|-------------|
| `ollama-local` | `K_AI_OLLAMA_BASE_URL` | — |
| `mock` | `{K_AI_BASE_URL}/mock/v1` | — |
| `openrouter` | `https://openrouter.ai/api/v1` | `K_AI_OPENROUTER_API_KEY` |
| `opencode` | `https://api.opencode.ai/v1` | `K_AI_OPENCODE_API_KEY` |
| `opencode-go` | `https://opencode.ai/zen/go/v1` | `K_AI_OPENCODE_GO_API_KEY` |
| `venice` | `https://api.venice.ai/api/v1` | `K_AI_VENICE_API_KEY` |
| `mistral` | `https://api.mistral.ai/v1` | `K_AI_MISTRAL_API_KEY` |

Cloud providers without a key remain in DB but upstream calls fail until `make sync-env` or manual admin update.

### 1.5 Data lifecycle & storage

| Data | Path | Permission policy |
|------|------|-------------------|
| Runtime DB | `$K_AI_DATA_DIR/k-ai.db` (default `./data/k-ai.db`) | gitignored |
| Bootstrap YAML | `config/config.yaml` | committed, `{env:VAR}` placeholders |
| Secrets | `.env` | gitignored, never committed |
| API keys (downstream) | SQLite `api_keys` table | hashed at rest |

Bootstrap behaviour: on every start, providers + aliases from YAML are **upserted** into SQLite (config is source of truth for infra defaults; admin API overrides persist).

### 1.6 Admin API registry

| Method | Path | Action |
|--------|------|--------|
| GET | `/admin/api/v1/health` | Admin liveness |
| GET/POST | `/admin/api/v1/providers` | List / create |
| GET/PUT/DELETE | `/admin/api/v1/providers/{id}` | Read / update / delete |
| GET/POST | `/admin/api/v1/aliases` | List / create |
| DELETE | `/admin/api/v1/aliases/{id}` | Delete |
| GET/POST | `/admin/api/v1/api-keys` | List / create (returns raw key once) |
| DELETE | `/admin/api/v1/api-keys/{id}` | Revoke |

### 1.7 Streaming contract

When `"stream": true`:

- Request forwarded to upstream with same flag
- Response body streamed byte-for-byte (SSE)
- `Content-Type: text/event-stream` preserved
- Mock provider emits valid SSE chunks for offline tests

### 1.8 Production deployment (homelab)

- **Traefik** external network `proxy`
- **Hosts**: `ai.kpihx-labs.com` (Cloudflare TLS), `ai.homelab` (internal)
- **Container port**: 8080
- **Critical**: inside Docker, `K_AI_BASE_URL=http://127.0.0.1:8080` so mock self-routing stays in-container (set by `scripts/docker-entrypoint.sh` if unset)
- **Ollama**: `K_AI_OLLAMA_BASE_URL=http://host.docker.internal:11434/v1` (override on server if Ollama elsewhere)
- **CI**: GitLab `deploy_homelab` job on `main` → `docker compose up -d`

---

## Facet 2: DEV (Development Guide)

### 2.1 Project structure (target layout)

```text
k_ai/
├── cmd/k-ai/main.go           # Entrypoint
├── internal/
│   ├── admin/                 # Admin REST handlers
│   ├── alias/                 # Alias engine (exact/glob/regex)
│   ├── auth/                  # API key + admin middleware
│   ├── config/                # YAML + env expansion
│   ├── integration/           # httptest integration tests
│   ├── proxy/                 # OpenAI gateway + streaming
│   ├── resolver/              # Provider model rules
│   ├── server/                # HTTP mux wiring
│   ├── store/                 # SQLite persistence
│   └── web/static/            # Embedded dashboard
├── config/config.yaml         # Bootstrap providers + aliases
├── scripts/
│   ├── sync-env-from-runtimes.sh
│   ├── http-test.sh
│   ├── live-test.sh
│   └── docker-entrypoint.sh
├── docker-compose.yml         # Prod Traefik labels
├── docker-compose.override.example.yml
├── Dockerfile
├── Makefile                   # Command plane
├── .gitlab-ci.yml             # validate + deploy + GitHub sync
├── CONTRACT.md                # THIS DOCUMENT
├── CHANGELOG.md
├── TODO.md
├── README.md
└── .agents/AGENTS.md          # Agent context (repo-local)
```

### 2.2 Mandatory Make targets

| Command | Role |
|---------|------|
| `make init` | Copy `.env`, override compose, create dirs |
| `make check` | `go mod tidy` + build + `go test ./...` |
| `make run` | Native server with `.env` |
| `make sync-env` | Pull provider keys from local agent auth files |
| `make http-test` | curl smoke (server must be running) |
| `make live-test` | Start server, dashboard, stream, Ollama alias |
| `make up` / `make down` | Docker compose (kpihx_labs pattern) |
| `make deploy-check` | Docker build gate |
| `make push` | Push to `gitlab` remote (when configured) |

### 2.3 Immutable engineering rules

#### R1: No rigid catch-all routing bugs

Glob `*` providers MUST NOT steal models with exact rules elsewhere. Resolver scores: exact > regex > glob.

#### R2: No secrets in git

Use `{env:VAR}` in YAML, `.env` locally, GitLab CI variables in prod.

#### R3: Real tests before claiming done

`make check` + `make live-test` are mandatory. Static-only validation is insufficient.

#### R4: Config upsert on boot

`BootstrapFromConfig` always syncs providers/aliases from YAML — never skip when DB non-empty.

#### R5: English in code

Comments, identifiers, and agent docs in English. User-facing dashboard may use French labels.

### 2.4 CI/CD contract

| Stage | Job | Trigger |
|-------|-----|---------|
| validate | `make check` | all branches |
| deploy | `deploy_homelab` | `main`, runner tag `homelab` |
| sync | `sync_github` | `main`, requires `GITHUB_TOKEN` |

Required GitLab CI variables (homelab):

- `K_AI_ADMIN_TOKEN`
- Optional: `K_AI_OPENROUTER_API_KEY`, `K_AI_OPENCODE_API_KEY`, `K_AI_OPENCODE_GO_API_KEY`, `K_AI_VENICE_API_KEY`, `K_AI_MISTRAL_API_KEY`, `K_AI_OLLAMA_BASE_URL`
