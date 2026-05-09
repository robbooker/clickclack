CREATE INDEX IF NOT EXISTS idx_messages_channel_root_page
  ON messages(channel_id, parent_message_id, channel_seq)
  WHERE channel_id IS NOT NULL AND parent_message_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_messages_direct_page
  ON messages(direct_conversation_id, channel_seq)
  WHERE direct_conversation_id IS NOT NULL;
