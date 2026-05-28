CREATE TABLE IF NOT EXISTS slash_commands (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  app_installation_id TEXT REFERENCES app_installations(id) ON DELETE SET NULL,
  command TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  callback_url TEXT NOT NULL,
  signing_secret TEXT NOT NULL,
  bot_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_slash_commands_active_command
  ON slash_commands(workspace_id, command)
  WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_slash_commands_workspace
  ON slash_commands(workspace_id, revoked_at, command);

CREATE TABLE IF NOT EXISTS slash_command_invocations (
  id TEXT PRIMARY KEY,
  command_id TEXT NOT NULL REFERENCES slash_commands(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  text TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  response_status BIGINT NOT NULL DEFAULT 0,
  response_body TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_slash_command_invocations_command
  ON slash_command_invocations(command_id, created_at);
