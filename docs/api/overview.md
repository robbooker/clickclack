---
read_when:
  - changing REST endpoints, websocket behavior, SDK methods, or OpenAPI
  - adding integrations or bots
---

# API Overview

`packages/protocol/openapi.yaml` is the API contract source of truth. Server
handlers live in `apps/api/internal/httpapi`; the TypeScript SDK in
`packages/sdk-ts` is the typed client.

## Auth

The server resolves callers in this order (see
[../features/auth.md](../features/auth.md)):

1. `Authorization: Bearer <token>` (session token, magic-link result).
2. `cc_session` cookie.
3. `X-ClickClack-User: usr_...` header (local/dev impersonation for tests).
4. Dev fallback to the first user in the DB (disable in production via
   `--dev-bootstrap=false`).

## Endpoint groups

| Group         | Endpoints | Doc |
|---------------|-----------|-----|
| Auth          | `/api/auth/magic/{request,consume}`, `/api/auth/github/{start,callback}` | [auth](../features/auth.md) |
| Workspaces    | `/api/workspaces`, `/api/workspaces/{id}` | [workspaces](../features/workspaces.md) |
| Channels      | `/api/workspaces/{id}/channels`, `/api/channels/{id}` | [workspaces](../features/workspaces.md) |
| Messages      | `/api/channels/{id}/messages`, `/api/messages/{id}` | [messages](../features/messages.md) |
| Threads       | `/api/messages/{id}/thread`, `/api/messages/{id}/thread/replies` | [threads](../features/threads.md) |
| Reactions     | `/api/messages/{id}/reactions` | [reactions](../features/reactions.md) |
| Realtime      | `/api/realtime/{ws,events,ephemeral}` | [realtime](../features/realtime.md) |
| Search        | `/api/search` | [search](../features/search.md) |
| Uploads       | `/api/uploads`, `/api/messages/{id}/attachments` | [uploads](../features/uploads.md) |
| DMs           | `/api/dms`, `/api/dms/{id}/messages` | [dms](../features/dms.md) |
| Integrations  | `/api/hooks/mattermost/{channel}`, `/api/hooks/slash/{channel}` | [integrations](../features/integrations.md) |

## Conventions

- All payloads are JSON unless explicitly multipart (uploads) or
  form-encoded (slash commands).
- Mutating endpoints return both the affected resource and the durable event
  emitted (`{message, event}`), so clients can reconcile optimistically.
- Pagination on listings uses cursor-style sequence numbers (`after_seq` for
  channel/DM messages, `after_cursor` for events).
- Errors come back as `{ "error": "<message>" }` with an HTTP status code.

## SDK

TypeScript consumers should use `@clickclack/sdk-ts`. It depends on neither
Svelte nor any HTTP framework. See [../sdk.md](../sdk.md).
