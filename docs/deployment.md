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

The embedded frontend is a SvelteKit static SPA. Reverse proxies should pass
unknown paths through to the ClickClack binary, because direct visits to app
routes such as `/app/T.../C...`, `/app/T.../D...`, and `/app/T.../M...` are
resolved by the frontend fallback. Older internal-ID links such as
`/app/wsp_.../chn_...`, `/app/wsp_.../dm_...`, and `/app/wsp_.../msg_...`
are still accepted and canonicalized by the app after API permission checks.

`pnpm build` defaults the SvelteKit app version to `dev` so repeated local
builds do not rewrite embedded asset filenames when source code has not
changed. Release automation should set `CLICKCLACK_WEB_VERSION` to the commit
or tag being shipped so long-lived open browser tabs can detect a newly
deployed frontend bundle:

```sh
CLICKCLACK_WEB_VERSION="$(git rev-parse --short=12 HEAD)" pnpm build
```

## Releases

GoReleaser is configured in `.goreleaser.yml`. It builds `clickclack` for
Linux, macOS, Windows, and FreeBSD on `amd64` and `arm64`, with Windows
archives emitted as `.zip` and the others as `.tar.gz`. Linux `.deb` and
`.rpm` packages are generated through nfpm.

```sh
pnpm install
CLICKCLACK_WEB_VERSION="$(git rev-parse --short=12 HEAD)" \
  goreleaser release --snapshot --clean
```

The GoReleaser config runs `pnpm build` before compiling so the embedded SPA
is refreshed. The GitHub release workflow sets `CLICKCLACK_WEB_VERSION` from
the checked-out commit before invoking GoReleaser. Publishing is handled by
`.github/workflows/release.yml` on `v*` tags or manual dispatch with an
existing tag.

## Docker

The provided `Dockerfile` is multi-stage:

```sh
docker build \
  --build-arg CLICKCLACK_WEB_VERSION="$(git rev-parse --short=12 HEAD)" \
  -t clickclack .
docker run --rm -p 8080:8080 -v clickclack-data:/app/data clickclack
```

Stages:

1. `node:24-alpine` — installs pnpm dependencies and runs `pnpm build`.
2. `golang:1.26.5-alpine` — builds the Go binary, importing the SPA dist.
3. `alpine:3.23` — runtime image, runs as the `clickclack` user, exposes
   `8080`, mounts `/app/data` as a volume.

Override the entrypoint command to run admin tasks:

```sh
docker run --rm -v clickclack-data:/app/data clickclack \
  admin bootstrap --name "Peter" --email steipete@gmail.com
```

For the isolated non-production small-VM topology, deterministic synthetic
seed, OpenClaw/ClawRouter SecretRef contract, canary, and teardown, see
[FakeCo staging](fakeco.md). It uses this same Docker image and SQLite adapter;
it does not add a second ClickClack cloud runtime. The guarded FakeCo-only AWS
owner is documented in [deploy/fakeco/aws/README.md](../deploy/fakeco/aws/README.md).

## Health and telemetry

`GET /healthz` reports process liveness. `GET /readyz` checks database
connectivity and returns `503` without the database error when unavailable.
Both responses include an `X-Correlation-ID`; callers may supply a safe ID or
let the server generate one.

Set `CLICKCLACK_METRICS_ENABLED=true` only on a private operator network to
expose Prometheus text at `/metrics`. Metrics use normalized route patterns and
status classes; they do not label users, workspaces, channels, messages, query
values, or body content. When disabled, `/metrics` returns `404`.

## Data layout

SQLite layout:

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

Postgres layout:

```sh
CLICKCLACK_DB='postgres://user:pass@db.example.com:5432/clickclack?sslmode=require' \
clickclack serve --addr :8080 --data /var/lib/clickclack
```

The Postgres adapter stores users, messages, events, auth, search, and chat
metadata in Postgres. Use provider snapshots or `pg_dump` for Postgres
backups; `clickclack backup` is SQLite-only.

R2 upload layout:

```sh
CLICKCLACK_DB='postgres://user:pass@db.example.com:5432/clickclack?sslmode=require' \
CLICKCLACK_UPLOADS='r2://clickclack-uploads/prod' \
CLICKCLACK_R2_ACCOUNT_ID='91b59577e757131d68d55a471fe32aca' \
CLICKCLACK_R2_ACCESS_KEY_ID='...' \
CLICKCLACK_R2_SECRET_ACCESS_KEY='...' \
CLICKCLACK_DEV_BOOTSTRAP=false \
clickclack serve --addr :8080 --data /var/lib/clickclack
```

R2 stores upload bytes; Postgres stores upload metadata and message attachment
links. Requests still go through `/api/uploads/{id}` so workspace/member
authorization stays server-side.

## Reverse proxy

Required for TLS and request size limits. The WebSocket endpoint enforces the
request host by default and also allows `CLICKCLACK_PUBLIC_URL` as an origin
when configured.

Terminate TLS only at infrastructure you trust, redirect HTTP to HTTPS, and
send HSTS from the public HTTPS origin after confirming every subdomain covered
by the policy is HTTPS-ready. Rate-limit the public GitHub OAuth start endpoints
at this edge. ClickClack intentionally does not trust arbitrary
`X-Forwarded-For` values for security decisions.

The checked example at
[`deploy/nginx/clickclack.conf.example`](../deploy/nginx/clickclack.conf.example)
includes TLS, WebSocket proxying, query-free access logs, trusted forwarding
headers, and enforced OAuth rate limits. Replace its hostname and certificate
paths, confirm the upstream address, run `nginx -t`, and reload only after the
syntax check succeeds. If a CDN or load balancer connects to nginx, configure
nginx's real-IP module with only that provider's published address ranges;
never accept an arbitrary client-supplied forwarding header as the rate-limit
key.

## GitHub OAuth

If you want GitHub login, set:

```sh
CLICKCLACK_PUBLIC_URL=https://chat.example.com
# Optional only when trusted ClickClack instances share this hostname:
# CLICKCLACK_COOKIE_NAMESPACE=production
CLICKCLACK_GITHUB_CLIENT_ID=...
CLICKCLACK_GITHUB_CLIENT_SECRET=...
# Optional org gate:
# CLICKCLACK_GITHUB_ALLOWED_ORG=openclaw
# Optional moderator org for open guest login:
# CLICKCLACK_GITHUB_MODERATOR_ORG=openclaw
```

Configure the GitHub OAuth app callback to
`<public-url>/api/auth/github/callback`. Without `CLICKCLACK_GITHUB_ALLOWED_ORG`,
any GitHub account can sign in and is joined to an isolated `Guests` workspace
with a `guest` channel. When `CLICKCLACK_GITHUB_MODERATOR_ORG` is set, members
of that org become guest-workspace moderators and non-members start as
post-limited guests until approved. When the org gate is set, ClickClack asks
GitHub for `read:org` and only accepts active members of that org. See
[features/auth.md](features/auth.md) and
[features/moderation.md](features/moderation.md).

`CLICKCLACK_PUBLIC_URL` is startup-validated as an exact origin. Non-loopback
origins must use HTTPS and cannot contain a path, credentials, query, or
fragment. GitHub OAuth credentials are rejected without it.

GitHub OAuth apps have one configured callback URL. Each ClickClack public
origin, including a distinct non-default port, therefore needs matching OAuth
app configuration. Never point multiple unrelated public origins at one
instance and rely on the request `Host` header to choose a callback.

HTTP cookies do not include ports in their scope. When multiple trusted
ClickClack instances share one hostname, assign each a unique, stable
`CLICKCLACK_COOKIE_NAMESPACE`; all replicas of one instance must use the same
value. This avoids accidental cookie-name collisions but does not isolate
mutually untrusted services. Use separate hostnames, or separate registrable
domains for stronger isolation.

OAuth state and desktop grants are stored in the configured database, so
callbacks can land on a different replica and survive process restarts.
Internet-facing proxies must rate-limit these exact method and path pairs:

```text
GET  /api/auth/github/start
GET  /api/auth/github/desktop/start
POST /api/auth/github/desktop/consume
```

The nginx example combines both start routes in one per-client bucket at one
request per second with a burst of eight, and gives desktop redemption a
separate five-request-per-second bucket with a burst of twenty. These are
enforced starting values, not universal capacity targets. They allow one
browser to use ClickClack's full eight-start concurrency while limiting a
single source that repeatedly abandons flows.

For a CDN or WAF, create separate rules:

```text
Start rule:
  expression:
    http.request.method eq "GET" and
    http.request.uri.path in {
      "/api/auth/github/start"
      "/api/auth/github/desktop/start"
    }
  count by: client identity and hostname
  initial rate: 60 requests per minute
  action: log, then managed challenge or block

Consume rule:
  expression:
    http.request.method eq "POST" and
    http.request.uri.path eq "/api/auth/github/desktop/consume"
  count by: client identity and hostname
  initial rate: 300 requests per minute
  action: log, then block
```

Use the strongest stable client characteristic your provider supports. IP plus
JA4 is preferable to IP alone when available; shared NATs need enough headroom
for legitimate users. Do not put an interactive challenge on the desktop
consume POST. First evaluate the rules against ordinary and known-abusive
traffic in log-only mode, then enforce and continue tuning from measured
percentiles. Cloudflare documents this rollout in its
[rate-limit analysis guide](https://developers.cloudflare.com/waf/rate-limiting-rules/find-rate-limit/).

CDN limits are another layer, not the only layer. Cloudflare counters are
maintained per data center and can fail open during infrastructure overload, so
keep the nginx limits or an equivalent trusted-origin limit in place. See
Cloudflare's
[rate-limit troubleshooting notes](https://developers.cloudflare.com/waf/rate-limiting-rules/troubleshooting/)
and nginx's
[`limit_req` documentation](https://nginx.org/en/docs/http/ngx_http_limit_req_module.html).

The database also rejects new work at 8,192 pending OAuth transactions, eight
pending starts per browser binding, and 4,096 pending desktop grants. With the
current ten-minute transaction TTL and five-minute grant TTL, a distributed
attacker that never completes a flow can exhaust either global pool at about
13.6 creations per second. That figure is a capacity alarm threshold, not a
recommended global edge limit: successful callbacks and redemptions remove
rows early. Deployments expecting more than 8,192 simultaneous starts or 4,096
unredeemed desktop callbacks must raise and load-test those database limits
before launch. They are compiled constants today and must stay identical in
the SQLite and Postgres stores.

Monitor live pool occupancy directly until a bounded metric is available:

```sql
-- Postgres
SELECT
  (SELECT count(*) FROM oauth_transactions
   WHERE expires_at_unix > extract(epoch FROM now())) AS oauth_transactions,
  (SELECT count(*) FROM desktop_oauth_grants
   WHERE expires_at_unix > extract(epoch FROM now())) AS desktop_oauth_grants;

-- SQLite
SELECT
  (SELECT count(*) FROM oauth_transactions
   WHERE expires_at_unix > unixepoch()) AS oauth_transactions,
  (SELECT count(*) FROM desktop_oauth_grants
   WHERE expires_at_unix > unixepoch()) AS desktop_oauth_grants;
```

Alert on sustained `429`, `503`, or
`clickclack_github_oauth_events_total{event="capacity_rejected"}` activity, and
on pending-row utilization before it reaches the hard limit. Configure every
proxy and CDN log sink to omit query strings on all GitHub OAuth routes,
including `/api/auth/github/callback`: callbacks contain short-lived
authorization codes, while desktop starts contain verifier challenges. nginx
error entries include the full request line, so the checked example suppresses
per-location nginx error logging for these routes and relies on query-free
status access logs plus ClickClack's correlation-aware server logs. Never log
`Authorization`, `Cookie`, or `Set-Cookie` headers. ClickClack's own request
logger records route patterns without query strings.

## Migrations

`clickclack serve` applies migrations on boot. For zero-downtime deploys, run
`clickclack migrate` ahead of the new binary so the old binary doesn't see
unexpected tables. SQLite migrations live in
`apps/api/internal/store/sqlite/migrations/`; Postgres migrations live in
`apps/api/internal/store/postgres/migrations/`. Both are append-only.

## Event retention

The durable realtime event log is for reconnect recovery, not permanent
message history. Message history stays in `messages`; old events can be
removed after the offline-recovery window you operate against:

```sh
clickclack admin events prune --workspace wsp_... --older-than-days 30 --keep-latest 10000
```

Run this from maintenance automation after backups. Clients with cursors older
than the retained event window should resync through the message APIs.

## Backups and restore

```sh
# hot backup
clickclack backup --out /var/backups/clickclack-$(date +%F).db

# JSON dump (good for sanity, not for restore)
clickclack export --out /var/backups/clickclack-$(date +%F).json
```

SQLite restore is a file swap: stop `clickclack`, replace
`<data>/clickclack.db`, delete any stale `*.db-wal`/`*.db-shm`, start it back
up. Postgres restore uses your database provider's restore flow or `psql`
from a `pg_dump` output.
