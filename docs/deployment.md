---
read_when:
  - shipping a release, building Docker, or planning a new install
  - changing how data is laid out on disk
---

# Deployment

ClickClack ships as one Go binary that embeds the Svelte SPA and the SQL
migrations. The deployment story is "drop a binary on a box, point it at a
data directory, run it behind a reverse proxy."

Public surfaces:

- `clickclack.chat` — product website.
- `app.clickclack.chat` — chat app. The same app is also available at `/app`
  for local development and simple single-host deployments.
- `docs.clickclack.chat` — documentation site built by `pnpm docs:site`.

## Single binary

```sh
pnpm install
pnpm build                                          # builds the SPA into apps/api/internal/webassets/dist
go build -o clickclack ./apps/api/cmd/clickclack
./clickclack serve --addr :8080 --data /var/lib/clickclack
```

The Go build step requires the SPA `dist/` to be present because `webassets`
uses `go:embed`. The `pnpm build` script copies `apps/web/dist` into
`apps/api/internal/webassets/dist`; CI must run it before `go build`.

## Docker

The provided `Dockerfile` is multi-stage:

```sh
docker build -t clickclack .
docker run --rm -p 8080:8080 -v clickclack-data:/app/data clickclack
```

Stages:

1. `node:25-alpine` — installs pnpm dependencies and runs `pnpm build`.
2. `golang:1.26-alpine` — builds the Go binary, importing the SPA dist.
3. `alpine:3.23` — runtime image, runs as the `clickclack` user, exposes
   `8080`, mounts `/app/data` as a volume.

Override the entrypoint command to run admin tasks:

```sh
docker run --rm -v clickclack-data:/app/data clickclack \
  admin bootstrap --name "Peter" --email steipete@gmail.com
```

## Data layout

```
<data>/
  clickclack.db                  # SQLite database (WAL files alongside)
  uploads/                       # local files for /api/uploads
  logs/                          # reserved; nothing writes here today
```

Back this directory up. SQLite WAL means a snapshot of the directory is
consistent enough, but prefer the online backup:

```sh
clickclack backup --data /var/lib/clickclack --out /var/backups/clickclack-$(date +%F).db
```

## Reverse proxy

Required for TLS, `Origin` enforcement, and request size limits. The
WebSocket endpoint accepts upgrades with `InsecureSkipVerify=true` today, so
the proxy is the right place to enforce origin policy.

A minimal nginx block:

```nginx
location / {
  proxy_pass http://127.0.0.1:8080;
  proxy_http_version 1.1;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
  proxy_read_timeout 300s;
}
```

## GitHub OAuth

If you want GitHub login, set:

```sh
CLICKCLACK_PUBLIC_URL=https://chat.example.com
CLICKCLACK_GITHUB_CLIENT_ID=...
CLICKCLACK_GITHUB_CLIENT_SECRET=...
```

Configure the GitHub OAuth app callback to
`<public-url>/api/auth/github/callback`. See [features/auth.md](features/auth.md).

## Migrations

`clickclack serve` applies migrations on boot. For zero-downtime deploys, run
`clickclack migrate` ahead of the new binary so the old binary doesn't see
unexpected tables. Migrations live in
`apps/api/internal/store/sqlite/migrations/` and are append-only.

## Backups and restore

```sh
# hot backup
clickclack backup --out /var/backups/clickclack-$(date +%F).db

# JSON dump (good for sanity, not for restore)
clickclack export --out /var/backups/clickclack-$(date +%F).json
```

Restore is a file swap: stop `clickclack`, replace `<data>/clickclack.db`,
delete any stale `*.db-wal`/`*.db-shm`, start it back up.
