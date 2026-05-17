CREATE TABLE IF NOT EXISTS upload_quota_reservations (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  byte_size INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_upload_quota_reservations_workspace_owner
  ON upload_quota_reservations(workspace_id, owner_id, expires_at);
