---
read_when:
  - changing the TypeScript SDK or the OpenAPI shape
  - adding or maintaining bot examples
---

# TypeScript SDK

`@clickclack/sdk-ts` is the framework-neutral client. It wraps the HTTP API
and the realtime WebSocket without any Svelte dependency, so bots, CLIs, and
non-Svelte frontends can use it directly.

Source: `packages/sdk-ts/src/index.ts`.

## Install (workspace)

The SDK is published from the monorepo. Inside this repo, depend on it as
`@clickclack/sdk-ts` via pnpm workspaces. External consumers will install it
once it's published.

## Quick start

```ts
import { ClickClackClient } from "@clickclack/sdk-ts";

const client = new ClickClackClient({
  baseUrl: "http://localhost:8080",
  token: process.env.CLICKCLACK_TOKEN,        // optional, sets Authorization
  userId: process.env.CLICKCLACK_USER_ID,     // optional local/dev override
});

const me = await client.me();
const workspaces = await client.workspaces.list();
const channels = await client.channels.list(workspaces[0].id);
const message = await client.channels.sendMessage(channels[0].id, {
  body: "click clack",
});
```

## Auth

The client sends, in this order:

- `Authorization: Bearer <token>` if `token` was set or `auth.consumeMagicLink`
  succeeded (it stores the returned session token).
- `X-ClickClack-User: <userId>` if `userId` was set. Use this only for local
  development/test impersonation; hosted bots should use bearer tokens.

Helpers:

```ts
client.auth.requestMagicLink({ email });          // POST /api/auth/magic/request
client.auth.consumeMagicLink(token);              // POST /api/auth/magic/consume; sets bearer
client.auth.setToken(token);                      // store an externally-issued token
client.auth.githubStartUrl();                     // build the OAuth start URL for the browser
```

See [features/auth.md](features/auth.md).

## Surface

| Group         | Methods |
|---------------|---------|
| `me()`        | get the current user |
| `workspaces`  | `list`, `create` |
| `channels`    | `list`, `create`, `update`, `messages`, `sendMessage` |
| `messages`    | `update`, `delete` |
| `threads`     | `get`, `reply` |
| `search(workspaceId, q)` | full-text search |
| `uploads`     | `create(workspaceId, file)`, `attach(messageId, uploadId)` |
| `dms`         | `list`, `create`, `messages`, `sendMessage` |
| `events`      | `publishEphemeral`, `subscribe` |

## Realtime subscription

```ts
const socket = client.events.subscribe({
  workspaceId,
  afterCursor: lastSeenCursor,
  onEvent(event) {
    // event.type, event.payload, event.cursor
  },
  onClose() {
    // backoff and resubscribe with the latest cursor
  },
});
```

`subscribe` returns a raw `WebSocket`. Call `.close()` to disconnect. See
[features/realtime.md](features/realtime.md) for cursor recovery rules.

## Generated types

`packages/sdk-ts/src/generated/openapi.ts` is generated from
`packages/protocol/openapi.yaml`. Re-export at the top of `index.ts`:

```ts
export type { components, paths } from "./generated/openapi";
```

Use the friendly hand-written types (`User`, `Workspace`, `Message`, etc.)
for app code; reach into `components["schemas"]` only when you need the
exact OpenAPI shape.

## Bot example

`examples/bot-ts` is a minimal one-shot bot that sends a single message:

```sh
CLICKCLACK_URL=http://localhost:8080 \
CLICKCLACK_USER_ID=usr_dev \
CLICKCLACK_CHANNEL_ID=chn_... \
CLICKCLACK_TEXT="clack from bot" \
pnpm --filter @clickclack/example-bot start
```

Use `CLICKCLACK_TOKEN` instead of `CLICKCLACK_USER_ID` once you have a real
session token.
