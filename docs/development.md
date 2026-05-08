---
read_when:
  - setting up a fresh checkout
  - changing the gate (lint/typecheck/test/build/coverage)
  - adding a new package or tool to the monorepo
---

# Development

The repo is a Go module plus a pnpm workspace. The Go binary embeds the
built SPA, so a full local build runs both toolchains.

## Prerequisites

- Go (matching `go.mod`).
- pnpm 11 (auto-managed via `corepack`).
- TypeScript runs via `tsgo` from `@typescript/native-preview` — installed
  through pnpm.
- Lint/format use `oxlint` and `oxfmt` — installed through pnpm.

## First run

```sh
pnpm install
pnpm build                                          # builds SPA + SDK and copies dist into apps/api
go run ./apps/api/cmd/clickclack serve
open http://localhost:8080
```

The dev fallback creates `Local Captain` as the first user, a `ClickClack`
workspace, and a `general` channel, so the SPA loads into a working state on
first hit.

## Two-process dev loop

```sh
# terminal 1
pnpm dev:api                                        # go run ./apps/api/cmd/clickclack serve

# terminal 2
pnpm dev:web                                        # vite dev server with API proxy
```

The Vite dev server proxies `/api` and `/api/realtime/ws` to `localhost:8080`.

## Scripts

| Command                | What it does |
|------------------------|--------------|
| `pnpm build`           | Builds the Svelte app and the SDK, then copies `apps/web/dist` into `apps/api/internal/webassets/dist`. |
| `pnpm check`           | Full local gate: `go test ./...`, root/workspace `tsgo`, `oxlint`, and format checks. |
| `pnpm coverage`        | Go tests with coverage; fails under 90% line coverage. |
| `pnpm dev:api`         | `go run ./apps/api/cmd/clickclack serve`. |
| `pnpm dev:web`         | `vite dev` for the SPA. |
| `pnpm fmt`             | `gofmt` + `oxfmt` over Go and TS/Svelte. |
| `pnpm fmt:check`       | CI-compatible formatting check with `gofmt -l` and `oxfmt --check`. |
| `pnpm lint`            | `oxlint` over web, SDK, examples, and tests. |
| `goreleaser release --snapshot --clean` | Local release smoke test for all configured OS/arch targets. |
| `pnpm typecheck`       | `tsgo --noEmit -p tsconfig.json` for root Playwright config/tests. |
| `pnpm test`            | `go test ./... && pnpm build`. |
| `pnpm test:e2e`        | Playwright suite in `tests/e2e`. |

## Layout

```
apps/
  api/                  # Go backend, single-binary entrypoint
    cmd/clickclack/     # CLI main
    internal/
      auth/             # placeholder
      config/           # flag/env/file resolution
      httpapi/          # chi router, handlers, auth resolution
      realtime/         # in-process pub/sub hub
      store/            # store interface + types
        sqlite/         # SQLite implementation, migrations, backup, export
      webassets/        # go:embed for the built SPA
  web/                  # Svelte 5 SPA
packages/
  protocol/             # OpenAPI spec, source of truth for the wire shape
  sdk-ts/               # TypeScript SDK (generated types + friendly wrapper)
examples/
  bot-ts/               # SDK usage example
infra/
  migrations/sqlite     # mirror of embedded migrations for tooling
  migrations/postgres   # placeholder for future Postgres support
tests/
  e2e/                  # Playwright tests
docs/                   # this directory
```

## Adding a feature

1. Update `packages/protocol/openapi.yaml` first when the wire shape
   changes. It is the contract.
2. Add the store method on `apps/api/internal/store/types.go` and implement
   it in `apps/api/internal/store/sqlite`.
3. Wire the handler in `apps/api/internal/httpapi`.
4. Update the SDK in `packages/sdk-ts/src/index.ts` so TS clients have a
   typed surface.
5. Update or add a `docs/features/<thing>.md`.
6. Run `pnpm check` and `pnpm coverage`.

## Testing

- `apps/api/internal/...` is the bulk of the test suite. Coverage gate is
  90%.
- `tests/e2e/chat.spec.ts` exercises the SPA end-to-end via Playwright.
- The SDK has no test target yet — the bot example is the smoke test.

## Coding rules

- IDs are sortable ULID-style with semantic prefixes (`usr_`, `wsp_`, `chn_`,
  `msg_`, `evt_`, `upl_`, `idn_`).
- Keep transactions short. Outbox events are inserted in the same tx as the
  durable write that produced them.
- Avoid Postgres-only SQL. Postgres support is planned to live behind the
  store interface, not by leaking dialect-specific SQL into handlers.
- TypeScript: no Svelte imports in `packages/sdk-ts`. The SDK must stay
  framework-neutral.
