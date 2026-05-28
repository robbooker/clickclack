CREATE TABLE IF NOT EXISTS app_installations (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  app_slug TEXT NOT NULL,
  display_name TEXT NOT NULL,
  bot_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  config_json TEXT NOT NULL DEFAULT '{}',
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_installations_active_slug
  ON app_installations(workspace_id, app_slug)
  WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_app_installations_workspace
  ON app_installations(workspace_id, revoked_at, app_slug);

CREATE INDEX IF NOT EXISTS idx_app_installations_bot
  ON app_installations(bot_user_id);
