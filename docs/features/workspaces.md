---
read_when:
  - adding workspace or channel endpoints
  - changing membership, slugs, or channel kinds
---

# Workspaces & Channels

A workspace is the top-level container. It owns channels, direct conversations,
events, uploads, and invites. Membership lives in `workspace_members` with a
`role` of `owner` or `member`.

## Workspaces

```http
GET  /api/workspaces                          # workspaces the caller belongs to
POST /api/workspaces                          # create + add caller as owner
GET  /api/workspaces/{workspace_id}           # one workspace, must be a member
```

`POST /api/workspaces` accepts `{name, slug?}`. Slugs default to a slugified
form of `name` and must be unique.

The owner who creates the workspace is auto-added with role `owner`. Adding
other members today goes through `clickclack admin user create --workspace
wsp_...` — the HTTP API does not yet expose member management.

## Channels

```http
GET  /api/workspaces/{workspace_id}/channels  # list, ordered by name
POST /api/workspaces/{workspace_id}/channels  # create
PATCH /api/channels/{channel_id}              # rename, change kind, archive
```

Create body: `{name, kind?}`. `name` is slugified to keep `(workspace_id, name)`
unique. `kind` defaults to `public`.

`PATCH` accepts any subset of `{name, kind, archived}`. Setting `archived=true`
fills `archived_at`; `archived=false` clears it.

Both endpoints emit a durable `channel.created` or `channel.updated` event into
the workspace event stream so connected clients see the change without
polling.

## Web routes

The web app uses ID-based routes for conversation navigation:

```text
/app/{workspace_id}
/app/{workspace_id}/{target_id}
```

`target_id` is a channel ID (`chn_...`), direct conversation ID (`dm_...`), or
thread root message ID (`msg_...`). Thread URLs resolve through the root
message, inherit that message's channel or DM visibility, and then open the
thread panel in the parent conversation.

## Membership rules

- Every workspace mutation checks `requireMembership(workspace_id, user_id)`.
- Listing channels, sending messages, opening threads, posting reactions,
  uploading files, and subscribing over WebSocket all go through the same
  check.
- Channel listing returns archived channels too — the UI is expected to
  render them differently. Filter on the client if you want only active
  channels.

## What is intentionally missing

- Private channels with explicit member sets (planned but not modeled in V1).
- Workspace-level roles beyond owner/member.
- Channel topic, description, or pinned messages.
