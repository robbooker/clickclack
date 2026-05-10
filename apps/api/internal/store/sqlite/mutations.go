package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func (s *Store) UpdateChannel(ctx context.Context, input store.UpdateChannelInput) (store.Channel, store.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Channel{}, store.Event{}, err
	}
	defer tx.Rollback()
	ch, err := scanChannel(tx.QueryRowContext(ctx, `SELECT id, workspace_id, name, kind, created_at, archived_at FROM channels WHERE id = ?`, input.ChannelID))
	if err != nil {
		return store.Channel{}, store.Event{}, err
	}
	if err := requireMembershipTx(ctx, tx, ch.WorkspaceID, input.UserID); err != nil {
		return store.Channel{}, store.Event{}, err
	}
	name := slug(input.Name)
	if name == "" {
		name = ch.Name
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		kind = ch.Kind
	}
	archivedValue := ch.ArchivedAt
	if input.Archived != nil {
		archivedValue = nil
		if *input.Archived {
			value := now()
			archivedValue = &value
		}
	}
	var archivedAt any
	if archivedValue != nil {
		archivedAt = *archivedValue
	}
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET name = ?, kind = ?, archived_at = ? WHERE id = ?`, name, kind, archivedAt, ch.ID); err != nil {
		return store.Channel{}, store.Event{}, err
	}
	event, err := insertEvent(ctx, tx, ch.WorkspaceID, ch.ID, "channel.updated", nil, map[string]string{"channel_id": ch.ID})
	if err != nil {
		return store.Channel{}, store.Event{}, err
	}
	ch.Name = name
	ch.Kind = kind
	ch.ArchivedAt = archivedValue
	return ch, event, tx.Commit()
}

func (s *Store) UpdateMessage(ctx context.Context, input store.UpdateMessageInput) (store.Message, store.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	defer tx.Rollback()
	msg, err := getMessageTx(ctx, tx, input.MessageID)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	if err := requireMessageAccessTx(ctx, tx, msg, input.UserID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	if msg.AuthorID != input.UserID {
		return store.Message{}, store.Event{}, errors.New("only the author can edit a message")
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return store.Message{}, store.Event{}, errors.New("message body is required")
	}
	editedAt := now()
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET body = ?, edited_at = ? WHERE id = ?`, body, editedAt, msg.ID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	payload := messagePayload(msg)
	recipients, err := eventRecipientsForMessageTx(ctx, tx, msg)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, msg.WorkspaceID, msg.ChannelID, "message.updated", msg.ChannelSeq, payload, recipients)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	msg.Body = body
	msg.EditedAt = &editedAt
	return msg, event, tx.Commit()
}

func (s *Store) DeleteMessage(ctx context.Context, input store.DeleteMessageInput) (store.Message, store.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	defer tx.Rollback()
	msg, err := getMessageTx(ctx, tx, input.MessageID)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	if err := requireMessageAccessTx(ctx, tx, msg, input.UserID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	if msg.AuthorID != input.UserID {
		return store.Message{}, store.Event{}, errors.New("only the author can delete a message")
	}
	deletedAt := now()
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET body = '', deleted_at = ? WHERE id = ?`, deletedAt, msg.ID); err != nil {
		return store.Message{}, store.Event{}, err
	}
	recipients, err := eventRecipientsForMessageTx(ctx, tx, msg)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, msg.WorkspaceID, msg.ChannelID, "message.deleted", msg.ChannelSeq, messagePayload(msg), recipients)
	if err != nil {
		return store.Message{}, store.Event{}, err
	}
	msg.Body = ""
	msg.DeletedAt = &deletedAt
	return msg, event, tx.Commit()
}

func messagePayload(msg store.Message) map[string]string {
	payload := map[string]string{"message_id": msg.ID, "root_message_id": msg.ThreadRootID}
	if msg.DirectConversationID != "" {
		payload["direct_conversation_id"] = msg.DirectConversationID
	}
	return payload
}

func eventRecipientsForMessageTx(ctx context.Context, tx *sql.Tx, msg store.Message) ([]string, error) {
	if msg.DirectConversationID == "" {
		return nil, nil
	}
	return directConversationMemberIDsTx(ctx, tx, msg.DirectConversationID)
}
