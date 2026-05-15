-- name: InsertMagicLink :exec
INSERT INTO auth_magic_links (id, token, email, display_name, created_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(token), sqlc.arg(email), sqlc.arg(display_name), sqlc.arg(created_at), sqlc.arg(expires_at));

-- name: GetMagicLinkByToken :one
SELECT id, token, email, display_name, created_at, expires_at, used_at
FROM auth_magic_links
WHERE token = sqlc.arg(token);

-- name: MarkMagicLinkUsed :exec
UPDATE auth_magic_links
SET used_at = sqlc.arg(used_at)
WHERE id = sqlc.arg(id);

-- name: GetSessionUser :one
SELECT u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = sqlc.arg(token)
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg(now);

-- name: InsertSession :exec
INSERT INTO sessions (id, token, user_id, created_at, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(token), sqlc.arg(user_id), sqlc.arg(created_at), sqlc.arg(expires_at));

-- name: GetUserByIdentityEmail :one
SELECT u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at
FROM identities i
JOIN users u ON u.id = i.user_id
WHERE i.email = sqlc.arg(email)
ORDER BY u.created_at
LIMIT 1;

-- name: GetUserByIdentityProviderSubject :one
SELECT u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at
FROM identities i
JOIN users u ON u.id = i.user_id
WHERE i.provider = sqlc.arg(provider)
  AND i.provider_subject = sqlc.arg(provider_subject);

-- name: InsertHumanUser :exec
INSERT INTO users (id, display_name, avatar_url, created_at)
VALUES (sqlc.arg(id), sqlc.arg(display_name), sqlc.arg(avatar_url), sqlc.arg(created_at));

-- name: InsertIdentity :exec
INSERT INTO identities (id, user_id, provider, provider_subject, email, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(provider), sqlc.arg(provider_subject), sqlc.arg(email), sqlc.arg(created_at));

-- name: FirstUser :one
SELECT id, kind, owner_user_id, display_name, handle, avatar_url, created_at
FROM users
ORDER BY created_at
LIMIT 1;

-- name: GetUser :one
SELECT id, kind, owner_user_id, display_name, handle, avatar_url, created_at
FROM users
WHERE id = sqlc.arg(id);

-- name: UpdateUserProfile :exec
UPDATE users
SET display_name = sqlc.arg(display_name),
    handle = sqlc.arg(handle),
    avatar_url = sqlc.arg(avatar_url)
WHERE id = sqlc.arg(id);

-- name: InsertInvite :exec
INSERT INTO invites (id, workspace_id, token, created_by, created_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(token), sqlc.arg(created_by), sqlc.arg(created_at));

-- name: InsertWorkspaceMember :exec
INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), sqlc.arg(role), sqlc.arg(created_at));

-- name: InsertDefaultWorkspaceMember :exec
INSERT OR IGNORE INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES (sqlc.arg(workspace_id), sqlc.arg(user_id), 'member', sqlc.arg(created_at));

-- name: FirstWorkspace :one
SELECT id, COALESCE(route_id, '') AS route_id, name, slug, created_at
FROM workspaces
ORDER BY created_at
LIMIT 1;

-- name: InsertWorkspace :exec
INSERT INTO workspaces (id, route_id, name, slug, created_at)
VALUES (sqlc.arg(id), sqlc.arg(route_id), sqlc.arg(name), sqlc.arg(slug), sqlc.arg(created_at));

-- name: InsertDefaultChannel :exec
INSERT INTO channels (id, route_id, workspace_id, name, kind, created_at)
VALUES (sqlc.arg(id), sqlc.arg(route_id), sqlc.arg(workspace_id), 'general', 'public', sqlc.arg(created_at));

-- name: RequireMembership :one
SELECT 1
FROM workspace_members
WHERE workspace_id = sqlc.arg(workspace_id)
  AND user_id = sqlc.arg(user_id);

-- name: InsertUpload :exec
INSERT INTO uploads (id, workspace_id, owner_id, filename, content_type, byte_size, width, height, duration_ms, storage_path, created_at)
VALUES (sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(owner_id), sqlc.arg(filename), sqlc.arg(content_type), sqlc.arg(byte_size), sqlc.arg(width), sqlc.arg(height), sqlc.arg(duration_ms), sqlc.arg(storage_path), sqlc.arg(created_at));

-- name: GetUpload :one
SELECT id, workspace_id, owner_id, filename, content_type, byte_size, width, height, duration_ms, storage_path, created_at
FROM uploads
WHERE id = sqlc.arg(id);

-- name: GetUploadWorkspace :one
SELECT workspace_id
FROM uploads
WHERE id = sqlc.arg(id);

-- name: AttachUpload :exec
INSERT OR IGNORE INTO message_attachments (message_id, upload_id, created_at)
VALUES (sqlc.arg(message_id), sqlc.arg(upload_id), sqlc.arg(created_at));

-- name: GetChannelWorkspace :one
SELECT workspace_id
FROM channels
WHERE id = sqlc.arg(id);

-- name: GetDirectConversationWorkspace :one
SELECT workspace_id
FROM direct_conversations
WHERE id = sqlc.arg(id);

-- name: ChannelLastSeq :one
SELECT CAST(COALESCE(MAX(channel_seq), 0) AS INTEGER) AS last_seq
FROM messages
WHERE channel_id = CAST(sqlc.arg(channel_id) AS TEXT)
  AND parent_message_id IS NULL;

-- name: DirectLastSeq :one
SELECT CAST(COALESCE(MAX(channel_seq), 0) AS INTEGER) AS last_seq
FROM messages
WHERE direct_conversation_id = CAST(sqlc.arg(conversation_id) AS TEXT)
  AND parent_message_id IS NULL;

-- name: ReadChannelRead :one
SELECT last_read_seq, last_read_at
FROM channel_reads
WHERE channel_id = sqlc.arg(channel_id)
  AND user_id = sqlc.arg(user_id);

-- name: ReadDirectRead :one
SELECT last_read_seq, last_read_at
FROM direct_reads
WHERE conversation_id = sqlc.arg(conversation_id)
  AND user_id = sqlc.arg(user_id);

-- name: UpsertChannelRead :exec
INSERT INTO channel_reads (channel_id, user_id, last_read_seq, last_read_at)
VALUES (sqlc.arg(channel_id), sqlc.arg(user_id), sqlc.arg(last_read_seq), sqlc.arg(last_read_at))
ON CONFLICT(channel_id, user_id) DO UPDATE SET
  last_read_seq = excluded.last_read_seq,
  last_read_at = excluded.last_read_at;

-- name: UpsertDirectRead :exec
INSERT INTO direct_reads (conversation_id, user_id, last_read_seq, last_read_at)
VALUES (sqlc.arg(conversation_id), sqlc.arg(user_id), sqlc.arg(last_read_seq), sqlc.arg(last_read_at))
ON CONFLICT(conversation_id, user_id) DO UPDATE SET
  last_read_seq = excluded.last_read_seq,
  last_read_at = excluded.last_read_at;
