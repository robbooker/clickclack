CREATE INDEX IF NOT EXISTS idx_direct_conversation_members_user
  ON direct_conversation_members(user_id, conversation_id);

CREATE INDEX IF NOT EXISTS idx_message_attachments_message_created
  ON message_attachments(message_id, created_at, upload_id);

DROP INDEX IF EXISTS idx_messages_direct_page;

CREATE INDEX IF NOT EXISTS idx_messages_direct_page
  ON messages(direct_conversation_id, parent_message_id, channel_seq)
  WHERE direct_conversation_id IS NOT NULL
    AND parent_message_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_channel_root_unique_seq
  ON messages(channel_id, channel_seq)
  WHERE channel_id IS NOT NULL
    AND parent_message_id IS NULL
    AND channel_seq IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_direct_unique_seq
  ON messages(direct_conversation_id, channel_seq)
  WHERE direct_conversation_id IS NOT NULL
    AND parent_message_id IS NULL
    AND channel_seq IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_thread_unique_seq
  ON messages(thread_root_id, thread_seq)
  WHERE parent_message_id IS NOT NULL
    AND thread_seq IS NOT NULL;

ALTER TABLE events ADD COLUMN is_private INTEGER NOT NULL DEFAULT 0 CHECK (is_private IN (0, 1));

CREATE TABLE IF NOT EXISTS event_recipients (
  event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (event_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_event_recipients_user_event
  ON event_recipients(user_id, event_id);

UPDATE messages
SET channel_id = NULL,
    direct_conversation_id = (
      SELECT root.direct_conversation_id
      FROM messages root
      WHERE root.id = messages.thread_root_id
    )
WHERE parent_message_id IS NOT NULL
  AND (direct_conversation_id IS NULL OR direct_conversation_id = '')
  AND EXISTS (
    SELECT 1
    FROM messages root
    WHERE root.id = messages.thread_root_id
      AND root.parent_message_id IS NULL
      AND root.direct_conversation_id IS NOT NULL
      AND root.direct_conversation_id <> ''
  );

INSERT OR IGNORE INTO event_recipients (event_id, user_id)
SELECT e.id, dcm.user_id
FROM events e
JOIN direct_conversation_members dcm
  ON dcm.conversation_id = json_extract(e.payload_json, '$.direct_conversation_id')
WHERE json_extract(e.payload_json, '$.direct_conversation_id') IS NOT NULL
  AND json_extract(e.payload_json, '$.direct_conversation_id') <> '';

INSERT OR IGNORE INTO event_recipients (event_id, user_id)
SELECT e.id, dcm.user_id
FROM events e
JOIN messages m
  ON m.id = json_extract(e.payload_json, '$.message_id')
JOIN direct_conversation_members dcm
  ON dcm.conversation_id = m.direct_conversation_id
WHERE json_extract(e.payload_json, '$.message_id') IS NOT NULL
  AND json_extract(e.payload_json, '$.message_id') <> ''
  AND m.direct_conversation_id IS NOT NULL
  AND m.direct_conversation_id <> '';

INSERT OR IGNORE INTO event_recipients (event_id, user_id)
SELECT e.id, dcm.user_id
FROM events e
JOIN messages root
  ON root.id = json_extract(e.payload_json, '$.root_message_id')
JOIN direct_conversation_members dcm
  ON dcm.conversation_id = root.direct_conversation_id
WHERE json_extract(e.payload_json, '$.root_message_id') IS NOT NULL
  AND json_extract(e.payload_json, '$.root_message_id') <> ''
  AND root.parent_message_id IS NULL
  AND root.direct_conversation_id IS NOT NULL
  AND root.direct_conversation_id <> '';

INSERT OR IGNORE INTO event_recipients (event_id, user_id)
SELECT e.id, u.id
FROM events e
JOIN users u
  ON u.id = json_extract(e.payload_json, '$.user_id')
WHERE e.type IN ('channel.read', 'dm.read')
  AND json_extract(e.payload_json, '$.user_id') IS NOT NULL
  AND json_extract(e.payload_json, '$.user_id') <> '';

UPDATE events
SET is_private = 1
WHERE json_extract(payload_json, '$.direct_conversation_id') IS NOT NULL
  AND json_extract(payload_json, '$.direct_conversation_id') <> '';

UPDATE events
SET is_private = 1
WHERE type IN ('channel.read', 'dm.read')
  AND json_extract(payload_json, '$.user_id') IS NOT NULL
  AND json_extract(payload_json, '$.user_id') <> '';

UPDATE events
SET is_private = 1
WHERE EXISTS (
  SELECT 1
  FROM event_recipients er
  WHERE er.event_id = events.id
);

UPDATE events
SET is_private = 1
WHERE EXISTS (
  SELECT 1
  FROM messages m
  WHERE m.id = json_extract(events.payload_json, '$.message_id')
    AND m.direct_conversation_id IS NOT NULL
    AND m.direct_conversation_id <> ''
);

UPDATE events
SET is_private = 1
WHERE EXISTS (
  SELECT 1
  FROM messages root
  WHERE root.id = json_extract(events.payload_json, '$.root_message_id')
    AND root.parent_message_id IS NULL
    AND root.direct_conversation_id IS NOT NULL
    AND root.direct_conversation_id <> ''
);

CREATE TRIGGER IF NOT EXISTS event_recipients_mark_private
AFTER INSERT ON event_recipients
BEGIN
  UPDATE events SET is_private = 1 WHERE id = NEW.event_id;
END;

CREATE TRIGGER IF NOT EXISTS messages_shape_insert
BEFORE INSERT ON messages
WHEN NOT (
  (
    NEW.parent_message_id IS NULL
    AND NEW.channel_id IS NOT NULL
    AND NEW.direct_conversation_id IS NULL
    AND NEW.thread_root_id = NEW.id
    AND NEW.channel_seq IS NOT NULL
    AND NEW.thread_seq IS NULL
    AND EXISTS (
      SELECT 1
      FROM channels c
      WHERE c.id = NEW.channel_id
        AND c.workspace_id = NEW.workspace_id
    )
  )
  OR
  (
    NEW.parent_message_id IS NULL
    AND NEW.channel_id IS NULL
    AND NEW.direct_conversation_id IS NOT NULL
    AND NEW.thread_root_id = NEW.id
    AND NEW.channel_seq IS NOT NULL
    AND NEW.thread_seq IS NULL
    AND EXISTS (
      SELECT 1
      FROM direct_conversations dc
      WHERE dc.id = NEW.direct_conversation_id
        AND dc.workspace_id = NEW.workspace_id
    )
  )
  OR
  (
    NEW.parent_message_id IS NOT NULL
    AND NEW.parent_message_id = NEW.thread_root_id
    AND NEW.channel_seq IS NULL
    AND NEW.thread_seq IS NOT NULL
    AND (
      (
        NEW.channel_id IS NOT NULL
        AND NEW.direct_conversation_id IS NULL
        AND EXISTS (
          SELECT 1
          FROM messages root
          WHERE root.id = NEW.thread_root_id
            AND root.parent_message_id IS NULL
            AND root.thread_root_id = root.id
            AND root.workspace_id = NEW.workspace_id
            AND root.channel_id = NEW.channel_id
            AND root.direct_conversation_id IS NULL
        )
      )
      OR
      (
        NEW.channel_id IS NULL
        AND NEW.direct_conversation_id IS NOT NULL
        AND EXISTS (
          SELECT 1
          FROM messages root
          WHERE root.id = NEW.thread_root_id
            AND root.parent_message_id IS NULL
            AND root.thread_root_id = root.id
            AND root.workspace_id = NEW.workspace_id
            AND root.channel_id IS NULL
            AND root.direct_conversation_id = NEW.direct_conversation_id
        )
      )
    )
  )
)
BEGIN
  SELECT RAISE(ABORT, 'invalid message shape');
END;

CREATE TRIGGER IF NOT EXISTS messages_shape_update
BEFORE UPDATE OF channel_id, direct_conversation_id, parent_message_id, thread_root_id, channel_seq, thread_seq ON messages
WHEN NOT (
  (
    NEW.parent_message_id IS NULL
    AND NEW.channel_id IS NOT NULL
    AND NEW.direct_conversation_id IS NULL
    AND NEW.thread_root_id = NEW.id
    AND NEW.channel_seq IS NOT NULL
    AND NEW.thread_seq IS NULL
    AND EXISTS (
      SELECT 1
      FROM channels c
      WHERE c.id = NEW.channel_id
        AND c.workspace_id = NEW.workspace_id
    )
  )
  OR
  (
    NEW.parent_message_id IS NULL
    AND NEW.channel_id IS NULL
    AND NEW.direct_conversation_id IS NOT NULL
    AND NEW.thread_root_id = NEW.id
    AND NEW.channel_seq IS NOT NULL
    AND NEW.thread_seq IS NULL
    AND EXISTS (
      SELECT 1
      FROM direct_conversations dc
      WHERE dc.id = NEW.direct_conversation_id
        AND dc.workspace_id = NEW.workspace_id
    )
  )
  OR
  (
    NEW.parent_message_id IS NOT NULL
    AND NEW.parent_message_id = NEW.thread_root_id
    AND NEW.channel_seq IS NULL
    AND NEW.thread_seq IS NOT NULL
    AND (
      (
        NEW.channel_id IS NOT NULL
        AND NEW.direct_conversation_id IS NULL
        AND EXISTS (
          SELECT 1
          FROM messages root
          WHERE root.id = NEW.thread_root_id
            AND root.parent_message_id IS NULL
            AND root.thread_root_id = root.id
            AND root.workspace_id = NEW.workspace_id
            AND root.channel_id = NEW.channel_id
            AND root.direct_conversation_id IS NULL
        )
      )
      OR
      (
        NEW.channel_id IS NULL
        AND NEW.direct_conversation_id IS NOT NULL
        AND EXISTS (
          SELECT 1
          FROM messages root
          WHERE root.id = NEW.thread_root_id
            AND root.parent_message_id IS NULL
            AND root.thread_root_id = root.id
            AND root.workspace_id = NEW.workspace_id
            AND root.channel_id IS NULL
            AND root.direct_conversation_id = NEW.direct_conversation_id
        )
      )
    )
  )
)
BEGIN
  SELECT RAISE(ABORT, 'invalid message shape');
END;
