# k-ai — Agent Context

## Mission

Build and maintain **k-ai**, KπX sovereign LLM gateway: 100% Go, OpenAI-compatible downstream API, flexible upstream routing.

## Non-negotiables

- Work only inside this repository unless deployment testing requires otherwise.
- Do **not** modify `~/.agents/` until the user explicitly asks at project closure.
- English only in code, comments, docs.
- No secrets in git; use `.env` + GitLab CI variables.
- Model resolution must support **exact, glob, regex** — never hardcode rigid `"*"`.
- Alias rewrite supports `${model}`, `${provider}`, `${1}`, `${name}` capture groups.
- Read **CONTRACT.md** before structural changes.

## Architecture map

1. `internal/store` — SQLite source of truth after bootstrap
2. `internal/resolver` — provider + model rule matching (exact > regex > glob)
3. `internal/alias` — client-facing slug rewriting before resolver
4. `internal/proxy` — `/v1/*` OpenAI-compatible proxy + streaming
5. `internal/admin` — `/admin/api/v1/*` CRUD
6. `internal/auth` — downstream API keys + admin token
7. `internal/web` — embedded dashboard + playground

## Dev commands

```bash
make init
make check
make live-test
make up
```

## Deployment target

Homelab via Traefik at `ai.kpihx-labs.com` (see `docker-compose.yml` labels, `CONTRACT.md` §1.8).

## References

- Skeleton governance: `Fluid/ts_proxy` (`CONTRACT.md`, `TODO.md`, `CHANGELOG.md`)
- Deploy pattern: `Homelab/kpihx_labs` (`make init`, `make up`, `.gitlab-ci.yml`)

## v0.2 backlog

See **TODO.md** — OAuth sidecars, health probes, encrypted secrets at rest.
