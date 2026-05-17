---
read_when:
  - changing upload storage, attachment endpoints, or upload limits
  - moving uploads off local disk
---

# Uploads

Uploads are files keyed by an `upl_...` ID and attached to messages through a
join table. The default backend is local disk; Cloudflare R2 can store bytes
for ephemeral container deployments. The server streams uploads back to
authenticated workspace members.

## Endpoints

```http
POST /api/uploads                                  # multipart: file, workspace_id
GET  /api/uploads/{upload_id}                      # streams the file
POST /api/messages/{message_id}/attachments        # { upload_id }
```

- Upload size cap: `64 MiB` per request.
- The file is written to the configured upload backend under a random
  `upload-*` key. The original filename is recorded in the `uploads` row but
  not used as the storage key.
- `Content-Type` falls back to `application/octet-stream` when the client
  doesn't send one.

## Attaching to a message

`POST /api/messages/{message_id}/attachments` records a row in
`message_attachments`. The store hydrates attachments on
`ListMessages`/`GetThread`, so subsequent reads include the attachment list
without an extra round-trip.

The handler checks that the requesting user is a member of the message's
workspace before linking. There is no "upload owner must equal message
author" rule today.

The web client renders `image/*` uploads inline in timelines and threads, and
renders `video/*` uploads as inline native players with controls. Other content
types appear as authenticated download cards that link to
`/api/uploads/{upload_id}`.

## Storage layout

Local disk:

```
<data>/
  clickclack.db
  uploads/
    upload-XXXXXXXX
    ...
  logs/
```

Configure `<data>` with `--data` or `CLICKCLACK_DATA`. The server creates
`uploads/` on demand when the first request arrives.

R2:

```sh
CLICKCLACK_UPLOADS=r2://clickclack-uploads/prod
CLICKCLACK_R2_ACCOUNT_ID=...
CLICKCLACK_R2_ACCESS_KEY_ID=...
CLICKCLACK_R2_SECRET_ACCESS_KEY=...
```

R2 keys are stored in the database as `r2://bucket/prefix/upload-...`.
Download requests are still authenticated by ClickClack before the object is
fetched from R2.

## What is intentionally missing

- Image thumbnailing/transcoding.
- Virus scanning.
- Per-workspace quotas.
