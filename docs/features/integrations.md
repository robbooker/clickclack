---
read_when:
  - touching webhook handlers or slash command callbacks
  - planning Mattermost-compatible behavior
---

# Integrations

ClickClack ships two integration surfaces designed to look like a small subset
of Mattermost so existing scripts can post messages without rewriting.

## App installations

An app installation binds a named integration to a workspace and to the bot user
that will act for it. It is control-plane metadata, not a runtime plugin loader.

```http
POST /api/workspaces/{workspace_id}/app-installations
Content-Type: application/json

{
  "app_slug": "openclaw",
  "display_name": "OpenClaw",
  "bot_user_id": "usr_...",
  "config": {
    "default_channel_id": "chn_..."
  }
}
```

Behavior:

- Requires a human session. Bot tokens cannot create, list, or revoke app
  installations.
- `bot_user_id` must be a bot member of the same workspace.
- `GET /api/workspaces/{workspace_id}/app-installations` lists active
  installations.
- `POST /api/app-installations/{installation_id}/revoke` revokes the binding
  without deleting historical metadata.

The TypeScript SDK exposes this as `client.apps.list(workspaceId)`,
`client.apps.install(workspaceId, input)`, and `client.apps.revoke(id)`.

## Incoming webhook

```http
POST /api/hooks/mattermost/{channel_id}
Content-Type: application/json
{ "text": "deploy ✅" }
```

Behavior:

- Authenticates the caller like any other API request. Use bearer session
  tokens for real integrations; reserve `X-ClickClack-User` for local/dev
  setups.
- Posts the `text` field as a Markdown channel message authored by the
  current user.
- Emits a `message.created` durable event.

Mattermost's full incoming webhook schema (`username`, `icon_emoji`,
`attachments`, etc.) is not honored — only `text`. Field names match so
existing senders don't crash.

## Registered slash commands

Use registered slash commands when an app owns the command and needs a signed
callback:

```http
POST /api/workspaces/{workspace_id}/slash-commands
Content-Type: application/json

{
  "app_installation_id": "app_...",
  "command": "/deploy",
  "description": "Deploy an environment",
  "callback_url": "https://example.com/clickclack/deploy",
  "bot_user_id": "usr_..."
}
```

Behavior:

- Requires a human session. Bot tokens cannot create, list, or revoke slash
  command registrations.
- `bot_user_id` must be a bot member of the same workspace.
- `command` is normalized to a lowercase leading-slash command such as
  `/deploy`.
- `POST /api/workspaces/{workspace_id}/slash-commands` returns a one-time
  `signing_secret`; list calls redact it.
- Invoking the command through `/api/hooks/slash/{channel_id}` sends JSON to
  `callback_url` with `X-ClickClack-Timestamp` and
  `X-ClickClack-Signature: sha256=<hex hmac>`, where the signed string is
  `<timestamp>.<raw-json-body>`.
- If the callback returns `{"response_type":"in_channel","text":"..."}`, the
  server posts that text as the command's bot user. `response_type` defaults to
  `in_channel`.

The callback payload includes `command_id`, `command`, `text`, `workspace_id`,
`channel_id`, `user_id`, `bot_user_id`, and `trigger_id`. Each invocation is
stored with callback status/body/error metadata for later audit.

The TypeScript SDK exposes this as `client.slashCommands.list(workspaceId)`,
`client.slashCommands.create(workspaceId, input)`, and
`client.slashCommands.revoke(id)`.

In the web app, channel composers discover registered commands inline: type
`/` at the start of a draft to open the slash-command menu, keep typing to
filter by command name, then use the arrow keys plus `Enter` or `Tab` to insert
the selected command. The menu overlays the timeline instead of resizing the
message list.

The same composer menu supports `@` mentions for workspace people and bots.
Mention suggestions work in channel, DM, and thread composers; selection uses
the same mouse, arrow-key, `Enter`, and `Tab` controls as slash commands.

## Outgoing event subscriptions

Outgoing event subscriptions push durable event-log entries to app callbacks.
They are intentionally built on the same event records used by realtime
WebSocket replay.

```http
POST /api/workspaces/{workspace_id}/event-subscriptions
Content-Type: application/json

{
  "app_installation_id": "app_...",
  "event_types": ["message.created", "reaction.added"],
  "callback_url": "https://example.com/clickclack/events"
}
```

Behavior:

- Requires a human session. Bot tokens cannot create, list, or revoke
  subscriptions.
- `event_types` accepts exact durable event types or `*`.
- Creation returns a one-time `signing_secret`; list calls redact it.
- Delivery posts `{"subscription_id":"sub_...","event": Event}` as JSON to the
  callback URL.
- Delivery uses the same signature headers as slash commands, plus
  `X-ClickClack-Event-ID`.
- Every delivery attempt is stored with response status, response body, error,
  and attempt number.

The TypeScript SDK exposes this as
`client.eventSubscriptions.list(workspaceId)`,
`client.eventSubscriptions.create(workspaceId, input)`,
`client.eventSubscriptions.revoke(id)`, and
`client.eventSubscriptions.deliveries(id)`.

## Connected accounts and audit log

Connected accounts record external identities or app-side account bindings for a
workspace user:

```http
POST /api/workspaces/{workspace_id}/connected-accounts
Content-Type: application/json

{
  "user_id": "usr_...",
  "provider": "github",
  "provider_account_id": "123456",
  "display_name": "octocat",
  "scopes": ["repo:read"],
  "metadata": {"login": "octocat"}
}
```

Behavior:

- Requires a human session.
- The target `user_id` must be a member of the workspace.
- `GET /api/workspaces/{workspace_id}/connected-accounts` lists active
  connected accounts.
- `POST /api/connected-accounts/{account_id}/revoke` revokes the account
  binding.
- Create and revoke write audit entries.

Audit entries are available from `GET /api/workspaces/{workspace_id}/audit-log`.
The SDK exposes these as `client.connectedAccounts.*` and
`client.auditLog.list(workspaceId)`.

## Compatibility slash command callback

```http
POST /api/hooks/slash/{channel_id}
Content-Type: application/x-www-form-urlencoded
command=/deploy&text=staging
```

Behavior:

- Posts `<command> <text>` (trimmed) as a channel message authored by the
  caller.
- Returns a Mattermost-shaped response:

```jsonc
{
  "response_type": "in_channel",
  "text":          "<command> <text>",
  "message":       Message,
  "event":         Event
}
```

If the posted `command` is not registered, ClickClack falls back to the legacy
Mattermost-shaped behavior above and posts `<command> <text>` directly as the
caller. This keeps simple scripts working while registered commands handle real
app callbacks.

## What is intentionally missing

- Retry scheduling for failed outgoing deliveries.
- Mattermost client compatibility (REST, WebSocket protocol).
