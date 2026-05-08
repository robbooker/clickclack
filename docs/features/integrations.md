---
read_when:
  - touching webhook handlers or slash command callbacks
  - planning Mattermost-compatible behavior
---

# Integrations

ClickClack ships two integration surfaces designed to look like a small subset
of Mattermost so existing scripts can post messages without rewriting.

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
- Bot accounts with their own permissions.
- Mattermost client compatibility (REST, WebSocket protocol).
