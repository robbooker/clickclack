---
read_when:
  - changing how requests are authenticated
  - touching magic-link, GitHub OAuth, or session cookie code
  - adding a new auth provider or revising the bootstrap flow
---

# Auth

ClickClack accepts four ways to identify a caller, in order of precedence. The
resolver lives in `apps/api/internal/httpapi/server.go` (`currentUser`).

1. `Authorization: Bearer <token>` — bearer session token.
2. `cc_session` cookie — HTTP-only session cookie set by magic-link consume and
   GitHub OAuth callback.
3. `X-ClickClack-User: usr_...` header — explicit user impersonation for local
   development and tests.
4. Dev fallback — the very first user in the database. Enabled by default by
   `clickclack serve --dev-bootstrap=true` so a fresh checkout boots into a
   working app without any token plumbing.

The dev fallback is the one to disable in any non-local deployment. Set
`--dev-bootstrap=false` and require real sessions.

## Local owner bootstrap

`clickclack serve` calls `Store.EnsureBootstrap` when `--dev-bootstrap` is on.
That helper:

- Returns the first user if one exists.
- Otherwise creates a `Local Captain` user, a `ClickClack` workspace, and a
  `general` channel, then returns the new user.

To pin the owner identity instead, run the CLI before serving:

```sh
clickclack admin bootstrap --name "Peter" --email steipete@gmail.com
```

That prints the new `usr_...` ID. Pass it back via `X-ClickClack-User` or use
the magic-link flow to mint a session.

## Magic links

Magic-link tokens are short-lived bearer credentials. They can be created over
HTTP or from the CLI; the consume endpoint exchanges them for a durable
session.

```http
POST /api/auth/magic/request
{ "email": "steipete@gmail.com", "display_name": "Peter" }

POST /api/auth/magic/consume
{ "token": "<token>" }
```

Or from the CLI, which is the V0 delivery path:

```sh
clickclack admin magic-link create --email steipete@gmail.com --name "Peter"
```

The client CLI can consume that token directly:

```sh
clickclack login --magic-token mgt_...
```

For remote agents and bots, use the resulting bearer session token. The CLI
will not send a stored bearer token to a different `--server`, and it skips
stored bearer tokens when `--user` / `CLICKCLACK_USER_ID` is set without an
explicit `--token`.

`ConsumeMagicLink` returns `{user, session, token}` and sets `cc_session` as an
HTTP-only cookie. Browsers can drop the body; bots should hold the
`session.token` for the `Authorization` header.

## GitHub OAuth (optional)

GitHub OAuth is opt-in. Set all three env vars (or the equivalent config keys)
before serving:

```sh
CLICKCLACK_PUBLIC_URL=https://chat.example.com
CLICKCLACK_GITHUB_CLIENT_ID=...
CLICKCLACK_GITHUB_CLIENT_SECRET=...
```

Without those, `GET /api/auth/github/start` returns `501`.

Flow:

1. `GET /api/auth/github/start` sets a state cookie and redirects to GitHub.
2. GitHub redirects back to `GET /api/auth/github/callback?code&state`.
3. The handler exchanges the code, fetches `/user` and primary `/user/emails`,
   upserts a user keyed by `(provider="github", provider_subject=<github id>)`,
   creates a session, sets `cc_session`, redirects to `/`.

The redirect URL is derived from `CLICKCLACK_PUBLIC_URL` when set, otherwise
from the request scheme/host. Configure GitHub with `<public-url>/api/auth/github/callback`.

## Authorization

Every store mutation that touches a workspace runs `requireMembership` (or the
in-tx variant). API handlers do not duplicate that check — trust the store
layer for it. WebSocket subscriptions revalidate `GetWorkspace` before
upgrading.

Roles today are limited to `owner` and `member`, used only by the bootstrap
helper. There is no role enforcement on writes yet beyond membership.

## Sessions

`sessions` are bearer tokens with an `expires_at`. `GetSessionUser` resolves the
token to a `User`. There is no refresh flow — issue a new session when one
expires.

## What is intentionally missing

- Email/password login.
- Password reset.
- SMTP delivery for magic links (V0 prints the token; V1 will add delivery).
- Per-channel ACLs, role-based permissions, audit logs.
