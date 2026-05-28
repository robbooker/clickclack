CREATE TABLE IF NOT EXISTS topics (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  channel_id TEXT REFERENCES channels(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  archived_at TEXT,
  UNIQUE(workspace_id, channel_id, name)
);

CREATE INDEX IF NOT EXISTS idx_topics_workspace
  ON topics(workspace_id, archived_at, name);

ALTER TABLE messages ADD COLUMN topic_id TEXT REFERENCES topics(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_messages_topic
  ON messages(topic_id, channel_seq);
