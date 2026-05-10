ALTER TABLE messages ADD COLUMN client_nonce TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_author_client_nonce
  ON messages(author_id, client_nonce)
  WHERE client_nonce <> '';
