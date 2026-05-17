package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/store/sqlite/storedb"
)

func (s *Store) CreateUpload(ctx context.Context, input store.CreateUploadInput) (store.Upload, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Upload{}, err
	}
	defer tx.Rollback()
	if err := canCreateUploadTx(ctx, tx, input.WorkspaceID, input.OwnerID, input.ByteSize); err != nil {
		return store.Upload{}, err
	}
	upload := store.Upload{
		ID:          newID("upl"),
		WorkspaceID: input.WorkspaceID,
		OwnerID:     input.OwnerID,
		Filename:    input.Filename,
		ContentType: input.ContentType,
		ByteSize:    input.ByteSize,
		Width:       input.Width,
		Height:      input.Height,
		DurationMS:  input.DurationMS,
		StoragePath: input.StoragePath,
		CreatedAt:   now(),
	}
	if err := s.q.WithTx(tx).InsertUpload(ctx, storedb.InsertUploadParams{
		ID:          upload.ID,
		WorkspaceID: upload.WorkspaceID,
		OwnerID:     upload.OwnerID,
		Filename:    upload.Filename,
		ContentType: upload.ContentType,
		ByteSize:    upload.ByteSize,
		Width:       int64(upload.Width),
		Height:      int64(upload.Height),
		DurationMs:  int64(upload.DurationMS),
		StoragePath: upload.StoragePath,
		CreatedAt:   upload.CreatedAt,
	}); err != nil {
		return store.Upload{}, err
	}
	return upload, tx.Commit()
}

func (s *Store) UploadQuota(ctx context.Context, workspaceID, userID string) (store.UploadQuota, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.UploadQuota{}, err
	}
	defer tx.Rollback()
	if err := requireMembershipTx(ctx, tx, workspaceID, userID); err != nil {
		return store.UploadQuota{}, err
	}
	if err := requireNoModerationBlockTx(ctx, tx, workspaceID, userID); err != nil {
		return store.UploadQuota{}, err
	}
	return uploadQuotaTx(ctx, tx, workspaceID, userID)
}

func (s *Store) CanCreateUpload(ctx context.Context, workspaceID, userID string, byteSize int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return canCreateUploadTx(ctx, tx, workspaceID, userID, byteSize)
}

func canCreateUploadTx(ctx context.Context, tx *sql.Tx, workspaceID, userID string, byteSize int64) error {
	if byteSize < 0 {
		return errors.New("upload byte size must be non-negative")
	}
	if err := requireMembershipTx(ctx, tx, workspaceID, userID); err != nil {
		return err
	}
	if err := requireNoModerationBlockTx(ctx, tx, workspaceID, userID); err != nil {
		return err
	}
	quota, err := uploadQuotaTx(ctx, tx, workspaceID, userID)
	if err != nil {
		return err
	}
	return quota.CanFit(byteSize)
}

func uploadQuotaTx(ctx context.Context, tx *sql.Tx, workspaceID, userID string) (store.UploadQuota, error) {
	quota := store.UploadQuota{
		MaxBytes: store.UploadQuotaBytesPerUserWorkspace,
		MaxCount: store.UploadQuotaCountPerUserWorkspace,
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(byte_size), 0)
		FROM uploads
		WHERE workspace_id = ? AND owner_id = ?`,
		workspaceID, userID,
	).Scan(&quota.UsedCount, &quota.UsedBytes); err != nil {
		return store.UploadQuota{}, err
	}
	quota.RemainingCount = max(quota.MaxCount-quota.UsedCount, 0)
	quota.RemainingBytes = max(quota.MaxBytes-quota.UsedBytes, 0)
	return quota, nil
}

func (s *Store) GetUpload(ctx context.Context, uploadID, userID string) (store.Upload, error) {
	row, err := s.q.GetUpload(ctx, uploadID)
	if err != nil {
		return store.Upload{}, err
	}
	upload := storeUploadFromGetUpload(row)
	if err := s.requireMembership(ctx, upload.WorkspaceID, userID); err != nil {
		return store.Upload{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ma.message_id
		FROM message_attachments ma
		JOIN messages m ON m.id = ma.message_id
		WHERE ma.upload_id = ? AND m.deleted_at IS NULL`, uploadID)
	if err != nil {
		return store.Upload{}, err
	}
	defer rows.Close()
	messageIDs := []string{}
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			return store.Upload{}, err
		}
		messageIDs = append(messageIDs, messageID)
	}
	if err := rows.Err(); err != nil {
		return store.Upload{}, err
	}
	if err := rows.Close(); err != nil {
		return store.Upload{}, err
	}
	for _, messageID := range messageIDs {
		if _, err := s.GetMessage(ctx, messageID, userID); err == nil {
			return upload, nil
		}
	}
	if len(messageIDs) > 0 {
		return store.Upload{}, errors.New("upload is not visible")
	}
	if upload.OwnerID != userID {
		return store.Upload{}, errors.New("upload is not visible")
	}
	return upload, nil
}

func (s *Store) AttachUpload(ctx context.Context, input store.AttachUploadInput) (store.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.Event{}, err
	}
	defer tx.Rollback()
	qtx := s.q.WithTx(tx)
	msg, err := getMessageTx(ctx, tx, input.MessageID)
	if err != nil {
		return store.Event{}, err
	}
	if err := requireMessageAccessTx(ctx, tx, msg, input.UserID); err != nil {
		return store.Event{}, err
	}
	if msg.AuthorID != input.UserID {
		return store.ErrMessageNotWritable
	}
	if err := requireNoModerationBlockTx(ctx, tx, msg.WorkspaceID, input.UserID); err != nil {
		return store.Event{}, err
	}
	if msg.AuthorID != input.UserID {
		return store.Event{}, errors.New("message attachments can only be changed by the message author")
	}
	if msg.DeletedAt != nil {
		return store.Event{}, errors.New("deleted messages cannot have attachments")
	}
	uploadWorkspace, err := qtx.GetUploadWorkspace(ctx, input.UploadID)
	if err != nil {
		return store.Event{}, err
	}
	if uploadWorkspace != msg.WorkspaceID {
		return store.Event{}, errors.New("upload and message workspaces differ")
	}
	uploadRow, err := qtx.GetUpload(ctx, input.UploadID)
	if err != nil {
		return store.Event{}, err
	}
	upload := storeUploadFromGetUpload(uploadRow)
	if upload.OwnerID != input.UserID {
		rows, err := tx.QueryContext(ctx, `
			SELECT ma.message_id
			FROM message_attachments ma
			JOIN messages m ON m.id = ma.message_id
			WHERE ma.upload_id = ? AND m.deleted_at IS NULL`, input.UploadID)
		if err != nil {
			return store.Event{}, err
		}
		messageIDs := []string{}
		for rows.Next() {
			var messageID string
			if err := rows.Scan(&messageID); err != nil {
				rows.Close()
				return store.Event{}, err
			}
			messageIDs = append(messageIDs, messageID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return store.Event{}, err
		}
		if err := rows.Close(); err != nil {
			return store.Event{}, err
		}
		visible := false
		for _, messageID := range messageIDs {
			attachedMessage, err := getMessageTx(ctx, tx, messageID)
			if err == nil && attachedMessage.DeletedAt == nil && requireMessageAccessTx(ctx, tx, attachedMessage, input.UserID) == nil {
				visible = true
				break
			}
		}
		if !visible {
			return store.Event{}, errors.New("upload is not visible")
		}
	}
	rows, err := qtx.AttachUpload(ctx, storedb.AttachUploadParams{
		MessageID: input.MessageID,
		UploadID:  input.UploadID,
		CreatedAt: now(),
	})
	if err != nil {
		return store.Event{}, err
	}
	if rows == 0 {
		return store.Event{}, tx.Commit()
	}
	recipients, err := eventRecipientsForMessageTx(ctx, tx, msg)
	if err != nil {
		return store.Event{}, err
	}
	event, err := insertEventWithRecipients(ctx, tx, msg.WorkspaceID, msg.ChannelID, "message.updated", msg.ChannelSeq, messagePayload(msg), recipients)
	if err != nil {
		return store.Event{}, err
	}
	return event, tx.Commit()
}

func (s *Store) hydrateAttachments(ctx context.Context, messages []store.Message) ([]store.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}
	ids := make([]string, 0, len(messages))
	indexByID := make(map[string]int, len(messages))
	for i, message := range messages {
		if message.DeletedAt != nil {
			continue
		}
		ids = append(ids, message.ID)
		indexByID[message.ID] = i
	}
	if len(ids) == 0 {
		return messages, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ma.message_id, u.id, u.workspace_id, u.owner_id, u.filename, u.content_type, u.byte_size, u.width, u.height, u.duration_ms, u.storage_path, u.created_at
		FROM message_attachments ma
		JOIN uploads u ON u.id = ma.upload_id
		JOIN messages m ON m.id = ma.message_id
		WHERE ma.message_id IN (`+placeholders+`) AND m.deleted_at IS NULL
		ORDER BY ma.message_id, ma.created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID string
		var upload store.Upload
		if err := rows.Scan(&messageID, &upload.ID, &upload.WorkspaceID, &upload.OwnerID, &upload.Filename, &upload.ContentType, &upload.ByteSize, &upload.Width, &upload.Height, &upload.DurationMS, &upload.StoragePath, &upload.CreatedAt); err != nil {
			return nil, err
		}
		if index, ok := indexByID[messageID]; ok {
			messages[index].Attachments = append(messages[index].Attachments, upload)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}
