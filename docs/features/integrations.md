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

## Slash command callback

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

This is intentionally thin: there is no command registry, no permission
matrix, no ephemeral response mode. Bots that need richer behavior should
post directly through `/api/channels/{id}/messages` with the SDK.

## What is intentionally missing

- Outgoing webhooks.
- Mattermost client compatibility (REST, WebSocket protocol).
