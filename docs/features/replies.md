---
read_when:
  - changing message create flow or quoted_message_id semantics
  - touching the `quoted_*` columns on `messages`
  - changing scope rules for inline replies
---

# Inline replies (quotes)

ClickClack has two reply patterns:

1. **Threads** — a flat reply list anchored to a root message; see
   [threads.md](threads.md).
2. **Inline quote-replies** — Discord/Telegram-style. The reply lives in the
   same stream (channel timeline, DM, or thread pane) and renders a clickable
   quote of the message it answers.

This page covers (2).

## Wire format

`POST /api/channels/{channel_id}/messages`,
`POST /api/dms/{conversation_id}/messages`, and
`POST /api/messages/{message_id}/thread/replies` all accept an optional
`quoted_message_id` and `nonce`:

```json
{ "body": "responding", "quoted_message_id": "msg_...", "nonce": "client-uuid" }
```

Server-side, three columns are stored on the new row:

| column                 | nullable | notes                                     |
| ---------------------- | -------- | ----------------------------------------- |
| `quoted_message_id`    | yes      | FK to `messages.id`, `ON DELETE SET NULL` |
| `quoted_body_snapshot` | no (`''`) | trimmed, capped at 280 runes              |
| `quoted_author_id`     | yes      | FK to `users.id`, `ON DELETE SET NULL`    |

When the new message is read back, an additional `quoted_author` object is
hydrated alongside `author`.

## Scope rules

The quoted message must live in the same context as the new message:

- **Channel timeline:** same `channel_id`, and the quoted message must be a
  top-level message (`parent_message_id IS NULL`).
- **DM:** same `direct_conversation_id`, top-level only.
- **Thread:** same `thread_root_id` (the root or any reply in the same thread).

Cross-context quoting and quoting a soft-deleted message both fail with
`HTTP 400` and `ErrQuotedMessageOutOfScope`. An empty `quoted_message_id`
string is treated as absent.

## Soft-snapshot semantics

`quoted_body_snapshot` is captured **at send time** and never updated. If the
quoted message is later edited, the snapshot stays as-is — the UI shows the
historical preview but, on click, scrolls to the live (edited) message.

If the quoted source is hard-deleted, the FK becomes `NULL` while the
snapshot survives. The UI renders the snapshot prefixed with
`[original deleted]` and disables the click target.

Soft-delete (today's `DELETE /api/messages/...`) sets `deleted_at` but keeps
the row, so existing quote refs continue to resolve. Hard delete is
expected to be rare (admin/export purge paths) and is the only path that
trips `ON DELETE SET NULL`.

## CLI

`clickclack send` and `clickclack threads reply` both accept `--reply-to
msg_...`, which is forwarded as `quoted_message_id`.

## Realtime

No new event types. The existing `message.created` and
`thread.reply_created` events carry the new fields in the same payload they
already used.
