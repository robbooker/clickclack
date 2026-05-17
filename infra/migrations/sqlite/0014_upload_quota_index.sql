CREATE INDEX IF NOT EXISTS idx_uploads_workspace_owner
  ON uploads(workspace_id, owner_id);
