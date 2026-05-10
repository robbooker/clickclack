package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

const directConversationMemberHydrationBatchSize = 500

func (s *Store) ListDirectConversations(ctx context.Context, workspaceID, userID string) ([]store.DirectConversation, error) {
	if err := s.requireMembership(ctx, workspaceID, userID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT dc.id, dc.workspace_id, dc.created_at,
		       COALESCE((SELECT MAX(channel_seq) FROM messages WHERE direct_conversation_id = dc.id AND parent_message_id IS NULL), 0) AS last_seq,
		       COALESCE((SELECT last_read_seq FROM direct_reads WHERE conversation_id = dc.id AND user_id = ?), 0) AS last_read_seq
		FROM direct_conversations dc
		JOIN direct_conversation_members dcm ON dcm.conversation_id = dc.id
		WHERE dc.workspace_id = ? AND dcm.user_id = ?
		ORDER BY dc.created_at`, userID, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	out := []store.DirectConversation{}
	for rows.Next() {
		var dm store.DirectConversation
		if err := rows.Scan(&dm.ID, &dm.WorkspaceID, &dm.CreatedAt, &dm.LastSeq, &dm.LastReadSeq); err != nil {
			return nil, err
		}
		if dm.LastSeq > dm.LastReadSeq {
			dm.UnreadCount = dm.LastSeq - dm.LastReadSeq
		}
		out = append(out, dm)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out))
	for _, dm := range out {
		ids = append(ids, dm.ID)
	}
	membersByConversation, err := s.directConversationMembersByConversationIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Members = membersByConversation[out[i].ID]
	}
	return out, nil
}

func (s *Store) GetDirectConversation(ctx context.Context, conversationID, userID string) (store.DirectConversation, error) {
	var dm store.DirectConversation
	if err := s.db.QueryRowContext(ctx, `
		SELECT dc.id, dc.workspace_id, dc.created_at,
		       COALESCE((SELECT MAX(channel_seq) FROM messages WHERE direct_conversation_id = dc.id AND parent_message_id IS NULL), 0) AS last_seq,
		       COALESCE((SELECT last_read_seq FROM direct_reads WHERE conversation_id = dc.id AND user_id = ?), 0) AS last_read_seq
		FROM direct_conversations dc
		JOIN direct_conversation_members dcm ON dcm.conversation_id = dc.id
		WHERE dc.id = ? AND dcm.user_id = ?`, userID, conversationID, userID).
		Scan(&dm.ID, &dm.WorkspaceID, &dm.CreatedAt, &dm.LastSeq, &dm.LastReadSeq); err != nil {
		return store.DirectConversation{}, err
	}
	if dm.LastSeq > dm.LastReadSeq {
		dm.UnreadCount = dm.LastSeq - dm.LastReadSeq
	}
	members, err := s.directConversationMembers(ctx, dm.ID)
	if err != nil {
		return store.DirectConversation{}, err
	}
	dm.Members = members
	return dm, nil
}

func (s *Store) CreateDirectConversation(ctx context.Context, input store.CreateDirectConversationInput) (store.DirectConversation, error) {
	if err := s.requireMembership(ctx, input.WorkspaceID, input.UserID); err != nil {
		return store.DirectConversation{}, err
	}
	memberIDs := append([]string{input.UserID}, input.MemberIDs...)
	memberIDs = compactStrings(memberIDs)
	if len(memberIDs) < 2 {
		return store.DirectConversation{}, errors.New("direct conversation needs at least two members")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.DirectConversation{}, err
	}
	defer tx.Rollback()
	for _, memberID := range memberIDs {
		if err := requireMembershipTx(ctx, tx, input.WorkspaceID, memberID); err != nil {
			return store.DirectConversation{}, err
		}
	}
	dm := store.DirectConversation{ID: newID("dm"), WorkspaceID: input.WorkspaceID, CreatedAt: now()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO direct_conversations (id, workspace_id, created_at) VALUES (?, ?, ?)`, dm.ID, dm.WorkspaceID, dm.CreatedAt); err != nil {
		return store.DirectConversation{}, err
	}
	for _, memberID := range memberIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO direct_conversation_members (conversation_id, user_id, created_at) VALUES (?, ?, ?)`, dm.ID, memberID, dm.CreatedAt); err != nil {
			return store.DirectConversation{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return store.DirectConversation{}, err
	}
	members, err := s.directConversationMembers(ctx, dm.ID)
	if err != nil {
		return store.DirectConversation{}, err
	}
	dm.Members = members
	return dm, nil
}

func (s *Store) ListDirectMessages(ctx context.Context, conversationID, userID string, page store.MessagePageRequest) (store.MessagePage, error) {
	if err := s.requireDirectMembership(ctx, conversationID, userID); err != nil {
		return store.MessagePage{}, err
	}
	return s.listMessagePage(ctx, messagePageScope{
		where: "m.direct_conversation_id = ? AND m.parent_message_id IS NULL",
		args:  []any{conversationID},
	}, page)
}

func (s *Store) CreateDirectMessage(ctx context.Context, input store.CreateDirectMessageInput) (store.Message, store.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	defer tx.Rollback()
	var workspaceID string
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id FROM direct_conversations WHERE id = ?`, input.ConversationID).Scan(&workspaceID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	if err := requireDirectMembershipTx(ctx, tx, input.ConversationID, input.AuthorID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(channel_seq), 0) + 1 FROM messages WHERE direct_conversation_id = ?`, input.ConversationID).Scan(&seq); err != nil {
		return store.Message{}, store.Event{}, err
	}
	id := newID("msg")
	createdAt := now()
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return store.Message{}, store.Event{}, errors.New("message body is required")
	}
	nonce, err := normalizeClientNonce(input.Nonce)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	var quotedID, quotedAuthorID, quotedSnapshot string
	if input.QuotedMessageID != nil && strings.TrimSpace(*input.QuotedMessageID) != "" {
		quotedID = strings.TrimSpace(*input.QuotedMessageID)
		snap, authorID, err := resolveQuoteRefTx(ctx, tx, quotedID, quoteScope{kind: "dm", directConversationID: input.ConversationID})
		if err != nil {
			return store.Message{}, store.Event{}, err
		}
		quotedSnapshot = snap
		quotedAuthorID = authorID
	}
	if existing, err := getMessageByClientNonceTx(ctx, tx, input.AuthorID, nonce); err == nil {
		if existing.DirectConversationID != input.ConversationID || existing.ChannelID != "" || existing.ParentMessageID != nil || existing.Body != body || !sameQuotedMessageID(existing, quotedID) {
			return store.Message{}, store.Event{}, store.ErrClientNonceConflict
		}
		return existing, store.Event{}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return store.Message{}, store.Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at, quoted_message_id, quoted_body_snapshot, quoted_author_id, client_nonce)
		VALUES (?, ?, NULL, ?, ?, NULL, ?, ?, NULL, ?, 'markdown', ?, ?, ?, ?, ?)`, id, workspaceID, input.ConversationID, input.AuthorID, id, seq, body, createdAt, nullableQuotedID(quotedID), quotedSnapshot, nullableQuotedID(quotedAuthorID), nonce); err != nil {
		if existing, lookupErr := getMessageByClientNonceTx(ctx, tx, input.AuthorID, nonce); lookupErr == nil {
			if existing.DirectConversationID == input.ConversationID && existing.ChannelID == "" && existing.ParentMessageID == nil && existing.Body == body && sameQuotedMessageID(existing, quotedID) {
				return existing, store.Event{}, nil
			}
			return store.Message{}, store.Event{}, store.ErrClientNonceConflict
		}
		return store.Message{}, store.Event{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO thread_state (root_message_id) VALUES (?)`, id); err != nil {
		return store.Message{}, store.Event{}, err
	}
	recipients, err := directConversationMemberIDsTx(ctx, tx, input.ConversationID)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, workspaceID, "", "message.created", &seq, eventPayload(map[string]string{"message_id": id, "direct_conversation_id": input.ConversationID, "author_id": input.AuthorID}, nonce), recipients)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	msg, err := getMessageTx(ctx, tx, id)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	return msg, event, tx.Commit()
}

func (s *Store) requireDirectMembership(ctx context.Context, conversationID, userID string) error {
	var one int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM direct_conversation_members WHERE conversation_id = ? AND user_id = ?`, conversationID, userID).Scan(&one)
}

func requireDirectMembershipTx(ctx context.Context, tx *sql.Tx, conversationID, userID string) error {
	var one int
	return tx.QueryRowContext(ctx, `SELECT 1 FROM direct_conversation_members WHERE conversation_id = ? AND user_id = ?`, conversationID, userID).Scan(&one)
}

func directConversationMemberIDsTx(ctx context.Context, tx *sql.Tx, conversationID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id
		FROM direct_conversation_members
		WHERE conversation_id = ?
		ORDER BY user_id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) directConversationMembers(ctx context.Context, conversationID string) ([]store.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at
		FROM users u
		JOIN direct_conversation_members dcm ON dcm.user_id = u.id
		WHERE dcm.conversation_id = ?
		ORDER BY u.display_name`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []store.User{}
	for rows.Next() {
		member, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (s *Store) directConversationMembersByConversationIDs(ctx context.Context, conversationIDs []string) (map[string][]store.User, error) {
	out := map[string][]store.User{}
	if len(conversationIDs) == 0 {
		return out, nil
	}
	for start := 0; start < len(conversationIDs); start += directConversationMemberHydrationBatchSize {
		end := min(start+directConversationMemberHydrationBatchSize, len(conversationIDs))
		batch := conversationIDs[start:end]
		placeholders := strings.TrimRight(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, 0, len(batch))
		for _, id := range batch {
			args = append(args, id)
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT dcm.conversation_id, u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at
			FROM direct_conversation_members dcm
			JOIN users u ON u.id = dcm.user_id
			WHERE dcm.conversation_id IN (`+placeholders+`)
			ORDER BY dcm.conversation_id, u.display_name`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var conversationID string
			var owner sql.NullString
			var member store.User
			if err := rows.Scan(&conversationID, &member.ID, &member.Kind, &owner, &member.DisplayName, &member.Handle, &member.AvatarURL, &member.CreatedAt); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if owner.Valid {
				member.OwnerUserID = owner.String
			}
			out[conversationID] = append(out[conversationID], member)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func compactStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}
