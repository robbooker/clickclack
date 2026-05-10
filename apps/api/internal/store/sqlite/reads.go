package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

// MarkChannelRead upserts the user's read pointer for a channel and emits a
// `channel.read` event. The pointer is monotonic — calls with a smaller seq
// are no-ops and return the existing receipt.
func (s *Store) MarkChannelRead(ctx context.Context, channelID, userID string, seq int64) (store.ReadReceipt, store.Event, error) {
	if seq < 0 {
		return store.ReadReceipt{}, store.Event{}, errors.New("seq must be non-negative")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	defer tx.Rollback()
	var workspaceID string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM channels WHERE id = ?`, channelID).Scan(&workspaceID); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if err := requireMembershipTx(ctx, tx, workspaceID, userID); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	// Cap to the current channel last_seq so a buggy client can't push the
	// pointer beyond reality.
	var lastSeq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(channel_seq), 0) FROM messages WHERE channel_id = ? AND parent_message_id IS NULL`, channelID).Scan(&lastSeq); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if seq > lastSeq {
		seq = lastSeq
	}
	current, currentAt, err := readChannelReadTx(ctx, tx, channelID, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if seq <= current {
		return store.ReadReceipt{ScopeID: channelID, UserID: userID, LastReadSeq: current, LastReadAt: currentAt}, store.Event{}, nil
	}
	at := now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO channel_reads (channel_id, user_id, last_read_seq, last_read_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(channel_id, user_id) DO UPDATE SET last_read_seq = excluded.last_read_seq, last_read_at = excluded.last_read_at`,
		channelID, userID, seq, at); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, workspaceID, channelID, "channel.read", &seq, map[string]string{
		"channel_id": channelID,
		"user_id":    userID,
	}, []string{userID})
	if err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	return store.ReadReceipt{ScopeID: channelID, UserID: userID, LastReadSeq: seq, LastReadAt: at}, event, tx.Commit()
}

// MarkDirectRead is the DM analogue of MarkChannelRead.
func (s *Store) MarkDirectRead(ctx context.Context, conversationID, userID string, seq int64) (store.ReadReceipt, store.Event, error) {
	if seq < 0 {
		return store.ReadReceipt{}, store.Event{}, errors.New("seq must be non-negative")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	defer tx.Rollback()
	var workspaceID string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM direct_conversations WHERE id = ?`, conversationID).Scan(&workspaceID); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if err := requireDirectMembershipTx(ctx, tx, conversationID, userID); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	var lastSeq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(channel_seq), 0) FROM messages WHERE direct_conversation_id = ? AND parent_message_id IS NULL`, conversationID).Scan(&lastSeq); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if seq > lastSeq {
		seq = lastSeq
	}
	current, currentAt, err := readDirectReadTx(ctx, tx, conversationID, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return store.ReadReceipt{}, store.Event{}, err
	}
	if seq <= current {
		return store.ReadReceipt{ScopeID: conversationID, UserID: userID, LastReadSeq: current, LastReadAt: currentAt}, store.Event{}, nil
	}
	at := now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO direct_reads (conversation_id, user_id, last_read_seq, last_read_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(conversation_id, user_id) DO UPDATE SET last_read_seq = excluded.last_read_seq, last_read_at = excluded.last_read_at`,
		conversationID, userID, seq, at); err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, workspaceID, "", "dm.read", &seq, map[string]string{
		"direct_conversation_id": conversationID,
		"user_id":                userID,
	}, []string{userID})
	if err != nil {
		return store.ReadReceipt{}, store.Event{}, err
	}
	return store.ReadReceipt{ScopeID: conversationID, UserID: userID, LastReadSeq: seq, LastReadAt: at}, event, tx.Commit()
}

func readChannelReadTx(ctx context.Context, tx *sql.Tx, channelID, userID string) (int64, string, error) {
	var seq int64
	var at string
	err := tx.QueryRowContext(ctx, `SELECT last_read_seq, last_read_at FROM channel_reads WHERE channel_id = ? AND user_id = ?`, channelID, userID).Scan(&seq, &at)
	return seq, at, err
}

func readDirectReadTx(ctx context.Context, tx *sql.Tx, conversationID, userID string) (int64, string, error) {
	var seq int64
	var at string
	err := tx.QueryRowContext(ctx, `SELECT last_read_seq, last_read_at FROM direct_reads WHERE conversation_id = ? AND user_id = ?`, conversationID, userID).Scan(&seq, &at)
	return seq, at, err
}
