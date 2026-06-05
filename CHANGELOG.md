# CHANGELOG — k-ai

All notable changes to this project are documented here.

## [0.1.0] - 2026-06-05

### Added

- Initial **100% Go** sovereign LLM gateway for KπX homelab.
- OpenAI-compatible endpoints: `/v1/models`, `/v1/chat/completions` (incl. SSE streaming).
- Provider registry: Ollama, OpenRouter, OpenCode, OpenCode Go, Venice, Mistral, mock.
- Flexible model rules per provider: `exact`, `glob`, `regex`.
- Alias engine with prefix routes (`local-`, `or-`, `oc-`, `ocgo-`, `v-`, `ollama-`).
- Admin REST API for providers, aliases, and downstream API keys.
- SQLite runtime store with YAML bootstrap upsert on startup.
- Config env expansion `{env:VAR}` and `scripts/sync-env-from-runtimes.sh`.
- Embedded web dashboard + playground at `/`.
- Docker multi-stage build, Traefik compose for `ai.kpihx-labs.com`.
- Test suite: Go unit/integration + `make live-test` (Ollama alias, streaming, dashboard).
- Project governance: `CONTRACT.md`, `TODO.md`, `CHANGELOG.md` (ts-proxy skeleton alignment).

### Fixed

- Resolver no longer routes `mock-model` to cloud providers with glob `*`.
- Mock provider base URL follows `K_AI_BASE_URL` (port-safe for live-test).
- `/v1/models` exposes human alias examples instead of raw regex patterns.

### Known gaps

- Production deploy pending first GitLab push to `main`.
- OAuth sidecars not implemented (planned v0.2).
