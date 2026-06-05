# TODO — k-ai

## Completed (v0.1.0)

- [x] Go gateway skeleton (`cmd/`, `internal/*`) inspired by ts-proxy governance layout
- [x] OpenAI-compatible `/v1/models` + `/v1/chat/completions`
- [x] Provider registry with `exact` / `glob` / `regex` model rules
- [x] Alias engine with rewrite templates (`${model}`, `${provider}`, captures)
- [x] Resolver priority: exact > regex > glob (fixes mock vs cloud `*`)
- [x] Admin REST API (`/admin/api/v1/*`)
- [x] SQLite persistence + YAML bootstrap upsert on every start
- [x] Env expansion `{env:VAR}` + `make sync-env` from OpenCode/Hermes auth
- [x] Streaming SSE passthrough + mock stream
- [x] Embedded dashboard + playground (`GET /`)
- [x] Docker + Traefik labels for `ai.kpihx-labs.com` / `ai.homelab`
- [x] Scripts: `http-test.sh`, `live-test.sh`
- [x] `.gitlab-ci.yml` (validate, deploy_homelab, sync_github)
- [x] Governance docs: `CONTRACT.md`, `CHANGELOG.md`, `TODO.md`

## In progress / deploy

- [ ] **Git repository init + first push** to GitLab (`main`) to trigger homelab deploy
- [ ] **Production verification** at `https://ai.kpihx-labs.com/health` with new image
- [ ] Register GitLab CI variables (`K_AI_ADMIN_TOKEN`, provider keys)

## Features & evolution (v0.2+)

- [ ] OAuth sidecars (Copilot, Google) — pattern TBD
- [ ] Provider health probes + circuit breaking
- [ ] Rate limiting per API key
- [ ] Admin: update alias via PUT (currently create/delete only)
- [ ] Upstream model cache TTL for `/v1/models` (avoid slow aggregate)
- [ ] Structured audit log (provider, model, latency, tokens)
- [ ] CLI admin tool (`k-ai admin …`) mirroring REST

## Security

- [ ] Encrypt provider API keys at rest in SQLite
- [ ] Rotate admin token workflow
- [ ] Scope enforcement audit (chat vs models vs admin paths)

## Distribution

- [ ] GitHub mirror repo `kpihx-labs/k-ai` via CI sync job
- [ ] Version tagging + release notes automation (`make git-release`)
