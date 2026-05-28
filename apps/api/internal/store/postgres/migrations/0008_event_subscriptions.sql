CREATE TABLE IF NOT EXISTS event_subscriptions (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  app_installation_id TEXT REFERENCES app_installations(id) ON DELETE SET NULL,
  event_types_json TEXT NOT NULL,
  callback_url TEXT NOT NULL,
  signing_secret TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_event_subscriptions_workspace
  ON event_subscriptions(workspace_id, revoked_at);

CREATE TABLE IF NOT EXISTS event_delivery_attempts (
  id TEXT PRIMARY KEY,
  subscription_id TEXT NOT NULL REFERENCES event_subscriptions(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  attempt BIGINT NOT NULL DEFAULT 1,
  request_json TEXT NOT NULL,
  response_status BIGINT NOT NULL DEFAULT 0,
  response_body TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  completed_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_event_delivery_attempts_subscription
  ON event_delivery_attempts(subscription_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_event_delivery_attempts_once
  ON event_delivery_attempts(subscription_id, event_id, attempt);
