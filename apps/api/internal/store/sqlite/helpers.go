package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/openclaw/clickclack/apps/api/internal/store"
)

type scanner interface {
	Scan(dest ...any) error
}

var (
	idMu      sync.Mutex
	idEntropy = ulid.Monotonic(rand.Reader, 0)
)

func scanUser(row scanner) (store.User, error) {
	var u store.User
	var owner sql.NullString
	err := row.Scan(&u.ID, &u.Kind, &owner, &u.DisplayName, &u.Handle, &u.AvatarURL, &u.CreatedAt)
	if owner.Valid {
		u.OwnerUserID = owner.String
	}
	return u, err
}

func scanWorkspace(row scanner) (store.Workspace, error) {
	var w store.Workspace
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedAt)
	return w, err
}

func scanChannel(row scanner) (store.Channel, error) {
	var ch store.Channel
	err := row.Scan(&ch.ID, &ch.WorkspaceID, &ch.Name, &ch.Kind, &ch.CreatedAt, &ch.ArchivedAt)
	return ch, err
}

func getMessage(ctx context.Context, db *sql.DB, id string) (store.Message, error) {
	return scanMessage(db.QueryRowContext(ctx, messageSelect()+` WHERE m.id = ?`, id))
}

func getMessageTx(ctx context.Context, tx *sql.Tx, id string) (store.Message, error) {
	return scanMessage(tx.QueryRowContext(ctx, messageSelect()+` WHERE m.id = ?`, id))
}

func messageSelect() string {
	return `SELECT m.id, m.workspace_id, COALESCE(m.channel_id, ''), COALESCE(m.direct_conversation_id, ''), m.author_id, m.parent_message_id, m.thread_root_id, m.channel_seq, m.thread_seq,
		       m.body, m.body_format, m.created_at, m.edited_at, m.deleted_at,
		       u.id, u.kind, u.owner_user_id, u.display_name, u.handle, u.avatar_url, u.created_at,
		       m.quoted_message_id, m.quoted_body_snapshot, m.quoted_author_id,
		       qu.id, qu.kind, qu.owner_user_id, qu.display_name, qu.handle, qu.avatar_url, qu.created_at,
		       m.client_nonce
		FROM messages m
		JOIN users u ON u.id = m.author_id
		LEFT JOIN users qu ON qu.id = m.quoted_author_id`
}

func scanMessage(row scanner) (store.Message, error) {
	var m store.Message
	var parent, edited, deleted sql.NullString
	var channelSeq, threadSeq sql.NullInt64
	var author store.User
	var quotedMessageID, quotedAuthorID sql.NullString
	var authorOwnerID sql.NullString
	var quAuthorID, quKind, quOwnerID, quDisplayName, quHandle, quAvatarURL, quCreatedAt sql.NullString
	var nonce string
	err := row.Scan(
		&m.ID, &m.WorkspaceID, &m.ChannelID, &m.DirectConversationID, &m.AuthorID, &parent, &m.ThreadRootID, &channelSeq, &threadSeq,
		&m.Body, &m.BodyFormat, &m.CreatedAt, &edited, &deleted,
		&author.ID, &author.Kind, &authorOwnerID, &author.DisplayName, &author.Handle, &author.AvatarURL, &author.CreatedAt,
		&quotedMessageID, &m.QuotedBodySnapshot, &quotedAuthorID,
		&quAuthorID, &quKind, &quOwnerID, &quDisplayName, &quHandle, &quAvatarURL, &quCreatedAt,
		&nonce,
	)
	if err != nil {
		return store.Message{}, err
	}
	if parent.Valid {
		m.ParentMessageID = &parent.String
	}
	if channelSeq.Valid {
		m.ChannelSeq = &channelSeq.Int64
	}
	if threadSeq.Valid {
		m.ThreadSeq = &threadSeq.Int64
	}
	if edited.Valid {
		m.EditedAt = &edited.String
	}
	if deleted.Valid {
		m.DeletedAt = &deleted.String
	}
	if authorOwnerID.Valid {
		author.OwnerUserID = authorOwnerID.String
	}
	m.Author = &author
	if quotedMessageID.Valid {
		m.QuotedMessageID = &quotedMessageID.String
	}
	if quotedAuthorID.Valid {
		m.QuotedAuthorID = &quotedAuthorID.String
	}
	if quAuthorID.Valid {
		m.QuotedAuthor = &store.User{
			ID:          quAuthorID.String,
			Kind:        quKind.String,
			OwnerUserID: quOwnerID.String,
			DisplayName: quDisplayName.String,
			Handle:      quHandle.String,
			AvatarURL:   quAvatarURL.String,
			CreatedAt:   quCreatedAt.String,
		}
	}
	m.Nonce = nonce
	return m, nil
}

func normalizeClientNonce(value string) (string, error) {
	nonce := strings.TrimSpace(value)
	if len(nonce) > 128 {
		return "", errors.New("nonce is too long")
	}
	return nonce, nil
}

func getMessageByClientNonceTx(ctx context.Context, tx *sql.Tx, authorID, nonce string) (store.Message, error) {
	if nonce == "" {
		return store.Message{}, sql.ErrNoRows
	}
	return scanMessage(tx.QueryRowContext(ctx, messageSelect()+` WHERE m.author_id = ? AND m.client_nonce = ?`, authorID, nonce))
}

func sameQuotedMessageID(message store.Message, quotedID string) bool {
	if quotedID == "" {
		return message.QuotedMessageID == nil || *message.QuotedMessageID == ""
	}
	return message.QuotedMessageID != nil && *message.QuotedMessageID == quotedID
}

var handlePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func normalizeHandle(value string) (string, error) {
	handle := strings.ToLower(strings.TrimSpace(value))
	handle = strings.TrimPrefix(handle, "@")
	if handle == "" {
		return "", nil
	}
	if !handlePattern.MatchString(handle) {
		return "", errors.New("handle must be 2-32 chars using letters, numbers, underscores, or dashes")
	}
	return handle, nil
}

func normalizeAvatarURL(value string) (string, error) {
	avatarURL := strings.TrimSpace(value)
	if avatarURL == "" {
		return "", nil
	}
	if len(avatarURL) > 500 {
		return "", errors.New("avatar_url is too long")
	}
	parsed, err := url.Parse(avatarURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", errors.New("avatar_url must be an http or https URL")
	}
	return avatarURL, nil
}

func scanMessages(rows *sql.Rows) ([]store.Message, error) {
	out := []store.Message{}
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func getThreadState(ctx context.Context, db *sql.DB, rootID string) (store.ThreadState, error) {
	return scanThreadState(db.QueryRowContext(ctx, `SELECT root_message_id, reply_count, last_reply_at, last_reply_author_ids_json FROM thread_state WHERE root_message_id = ?`, rootID))
}

func scanThreadState(row scanner) (store.ThreadState, error) {
	var state store.ThreadState
	var lastReply sql.NullString
	if err := row.Scan(&state.RootMessageID, &state.ReplyCount, &lastReply, &state.LastReplyAuthorIDsJSON); err != nil {
		return store.ThreadState{}, err
	}
	if lastReply.Valid {
		state.LastReplyAt = &lastReply.String
	}
	_ = json.Unmarshal([]byte(state.LastReplyAuthorIDsJSON), &state.LastReplyAuthorIDs)
	return state, nil
}

func updateThreadState(ctx context.Context, tx *sql.Tx, rootID, authorID, createdAt string) (store.ThreadState, error) {
	state, err := scanThreadState(tx.QueryRowContext(ctx, `SELECT root_message_id, reply_count, last_reply_at, last_reply_author_ids_json FROM thread_state WHERE root_message_id = ?`, rootID))
	if err != nil {
		return store.ThreadState{}, err
	}
	ids := append([]string{authorID}, state.LastReplyAuthorIDs...)
	seen := map[string]bool{}
	compact := make([]string, 0, 3)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		compact = append(compact, id)
		if len(compact) == 3 {
			break
		}
	}
	body, _ := json.Marshal(compact)
	if _, err := tx.ExecContext(ctx, `UPDATE thread_state SET reply_count = reply_count + 1, last_reply_at = ?, last_reply_author_ids_json = ? WHERE root_message_id = ?`, createdAt, string(body), rootID); err != nil {
		return store.ThreadState{}, err
	}
	return scanThreadState(tx.QueryRowContext(ctx, `SELECT root_message_id, reply_count, last_reply_at, last_reply_author_ids_json FROM thread_state WHERE root_message_id = ?`, rootID))
}

func insertEvent(ctx context.Context, tx *sql.Tx, workspaceID, channelID, eventType string, seq *int64, payload any) (store.Event, error) {
	return insertEventWithRecipients(ctx, tx, workspaceID, channelID, eventType, seq, payload, nil)
}

func insertEventWithRecipients(ctx context.Context, tx *sql.Tx, workspaceID, channelID, eventType string, seq *int64, payload any, recipientUserIDs []string) (store.Event, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return store.Event{}, err
	}
	recipients := compactStrings(recipientUserIDs)
	event := store.Event{
		ID:          newID("evt"),
		Cursor:      newID("cur"),
		Type:        eventType,
		WorkspaceID: workspaceID,
		ChannelID:   channelID,
		Seq:         seq,
		CreatedAt:   now(),
		PayloadJSON: string(payloadJSON),
		Payload:     payload,
	}
	isPrivate := 0
	if len(recipients) > 0 {
		isPrivate = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at, is_private) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.Cursor, event.WorkspaceID, nullableString(event.ChannelID), event.Type, event.Seq, event.PayloadJSON, event.CreatedAt, isPrivate); err != nil {
		return store.Event{}, err
	}
	for _, userID := range recipients {
		if _, err := tx.ExecContext(ctx, `INSERT INTO event_recipients (event_id, user_id) VALUES (?, ?)`, event.ID, userID); err != nil {
			return store.Event{}, err
		}
		event.RecipientUserIDs = append(event.RecipientUserIDs, userID)
	}
	return event, nil
}

// eventPayload returns the base payload with a "nonce" key only if non-empty.
// Used so optimistic clients can correlate WS message.created with their pending
// placeholder.
func eventPayload(base map[string]string, nonce string) map[string]string {
	if nonce == "" {
		return base
	}
	out := make(map[string]string, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out["nonce"] = nonce
	return out
}

func scanEvents(rows *sql.Rows) ([]store.Event, error) {
	out := []store.Event{}
	for rows.Next() {
		var event store.Event
		var seq sql.NullInt64
		if err := rows.Scan(&event.ID, &event.Cursor, &event.WorkspaceID, &event.ChannelID, &event.Type, &seq, &event.PayloadJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		if seq.Valid {
			event.Seq = &seq.Int64
		}
		var payload any
		_ = json.Unmarshal([]byte(event.PayloadJSON), &payload)
		event.Payload = payload
		out = append(out, event)
	}
	return out, rows.Err()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func newID(prefix string) string {
	idMu.Lock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), idEntropy)
	idMu.Unlock()
	return prefix + "_" + strings.ToLower(id.String())
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugRE.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
