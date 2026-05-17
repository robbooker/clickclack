CREATE TABLE IF NOT EXISTS workspace_member_moderation (
  workspace_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  timeout_until TEXT,
  blocked_at TEXT,
  moderation_note TEXT NOT NULL DEFAULT '',
  moderation_by TEXT REFERENCES users(id) ON DELETE SET NULL,
  moderation_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, user_id),
  FOREIGN KEY (workspace_id, user_id) REFERENCES workspace_members(workspace_id, user_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_workspace_member_moderation_timeout
  ON workspace_member_moderation(workspace_id, timeout_until);
