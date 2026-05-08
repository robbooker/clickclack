---
read_when:
  - changing upload storage, attachment endpoints, or upload limits
  - moving uploads off local disk
---

# Uploads

Uploads are local files keyed by an `upl_...` ID and attached to messages
through a join table. The server streams them back to authenticated workspace
members.

## Endpoints

```http
POST /api/uploads                                  # multipart: file, workspace_id
GET  /api/uploads/{upload_id}                      # streams the file
POST /api/messages/{message_id}/attachments        # { upload_id }
```

- Upload size cap: `32 MiB` per request (`ParseMultipartForm(32 << 20)`).
  Anything larger should use a future direct-to-storage flow.
- The file is written to the upload directory (`<data>/uploads`) under a
  random `upload-*` name. The original filename is recorded in the `uploads`
  row but not used on disk.
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

## What is intentionally missing

- Object storage (S3/GCS) — keep the local path single-node-only for V1.
- Image thumbnailing/transcoding.
- Virus scanning.
- Per-workspace quotas.
