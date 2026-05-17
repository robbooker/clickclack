package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/uploadstore"
)

const (
	maxUploadBytes       = 64 << 20
	uploadCleanupTimeout = 5 * time.Second
)

func formInt(values map[string]string, key string) int {
	v, err := strconv.Atoi(values[key])
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func (s *Server) markChannelRead(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	receipt, event, err := s.store.MarkChannelRead(r.Context(), chi.URLParam(r, "channel_id"), act.user.ID, body.Seq)
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"receipt": receipt}, err)
}

func (s *Server) markDirectRead(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Seq int64 `json:"seq"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("dms:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotDirectWorkspace(w, r, act, chi.URLParam(r, "conversation_id")) {
		return
	}
	receipt, event, err := s.store.MarkDirectRead(r.Context(), chi.URLParam(r, "conversation_id"), act.user.ID, body.Seq)
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"receipt": receipt}, err)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	results, err := s.store.SearchMessages(r.Context(), workspaceID, r.URL.Query().Get("channel_id"), act.user.ID, r.URL.Query().Get("q"), queryInt(r, "limit", 50))
	writeResult(w, map[string]any{"results": results}, err)
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if s.uploadStorage == nil {
		writeError(w, http.StatusInternalServerError, errors.New("uploads are not configured"))
		return
	}
	if err := act.requireScope("uploads:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	fields := map[string]string{}
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if workspaceID != "" && !s.authorizeUploadWorkspace(w, r, act, workspaceID) {
		return
	}
	var upload store.CreateUploadInput
	var savedPath string
	committed := false
	defer func() {
		if savedPath != "" && !committed {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), uploadCleanupTimeout)
			defer cancel()
			_ = s.uploadStorage.Delete(cleanupCtx, savedPath)
		}
	}()
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeUploadBodyError(w, err, http.StatusBadRequest)
			return
		}
		name := part.FormName()
		if name == "" {
			continue
		}
		if name != "file" {
			body, err := io.ReadAll(io.LimitReader(part, 1024))
			if err != nil {
				writeUploadBodyError(w, err, http.StatusBadRequest)
				return
			}
			fields[name] = string(body)
			if name == "workspace_id" && workspaceID == "" {
				workspaceID = strings.TrimSpace(fields[name])
				if !s.authorizeUploadWorkspace(w, r, act, workspaceID) {
					return
				}
			} else if name == "workspace_id" && strings.TrimSpace(fields[name]) != "" && strings.TrimSpace(fields[name]) != workspaceID {
				writeError(w, http.StatusBadRequest, errors.New("workspace_id does not match query"))
				return
			}
			continue
		}
		if workspaceID == "" {
			writeError(w, http.StatusBadRequest, errors.New("workspace_id must precede file or be provided as a query parameter"))
			return
		}
		if upload.StoragePath != "" {
			writeError(w, http.StatusBadRequest, errors.New("only one file is supported"))
			return
		}
		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		saved, err := s.uploadStorage.Save(r.Context(), part, uploadstore.SaveOptions{ContentType: contentType})
		if err != nil {
			writeUploadBodyError(w, err, http.StatusInternalServerError)
			return
		}
		savedPath = saved.Path
		upload = store.CreateUploadInput{
			WorkspaceID: workspaceID,
			OwnerID:     act.user.ID,
			Filename:    filepath.Base(part.FileName()),
			ContentType: contentType,
			ByteSize:    saved.ByteSize,
			Width:       formInt(fields, "width"),
			Height:      formInt(fields, "height"),
			DurationMS:  formInt(fields, "duration_ms"),
			StoragePath: saved.Path,
		}
	}
	if upload.StoragePath == "" {
		writeError(w, http.StatusBadRequest, errors.New("file is required"))
		return
	}
	upload.Width = formInt(fields, "width")
	upload.Height = formInt(fields, "height")
	upload.DurationMS = formInt(fields, "duration_ms")
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	created, err := s.store.CreateUpload(r.Context(), upload)
	if err == nil {
		committed = true
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"upload": created}, err)
}

func (s *Server) authorizeUploadWorkspace(w http.ResponseWriter, r *http.Request, act actor, workspaceID string) bool {
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return false
	}
	if _, err := s.store.GetWorkspace(r.Context(), workspaceID, act.user.ID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return false
	}
	if err := s.store.CanCreateUpload(r.Context(), workspaceID, act.user.ID); err != nil {
		writeStoreError(w, err)
		return false
	}
	return true
}

func writeUploadBodyError(w http.ResponseWriter, err error, fallbackStatus int) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	writeError(w, fallbackStatus, err)
}

func (s *Server) getUpload(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if s.uploadStorage == nil {
		writeError(w, http.StatusInternalServerError, errors.New("uploads are not configured"))
		return
	}
	upload, err := s.store.GetUpload(r.Context(), chi.URLParam(r, "upload_id"), act.user.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !s.requireBotWorkspace(w, act, upload.WorkspaceID, nil) {
		return
	}
	setUploadResponseHeaders(w, upload)
	err = s.uploadStorage.ServeHTTP(w, r, uploadstore.Object{
		Path:        upload.StoragePath,
		Filename:    upload.Filename,
		ContentType: safeUploadContentType(upload.ContentType),
		ByteSize:    upload.ByteSize,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, uploadstore.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
	}
}

func setUploadResponseHeaders(w http.ResponseWriter, upload store.Upload) {
	contentType := safeUploadContentType(upload.ContentType)
	disposition := "attachment"
	if isInlineUploadContentType(contentType) {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": upload.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
}

func safeUploadContentType(value string) string {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	if strings.HasPrefix(contentType, "audio/") {
		return contentType
	}
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "video/mp4", "video/webm", "text/plain", "application/pdf":
		return contentType
	default:
		return "application/octet-stream"
	}
}

func isInlineUploadContentType(contentType string) bool {
	return strings.HasPrefix(contentType, "image/") || strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/") || contentType == "application/pdf" || contentType == "text/plain"
}

func (s *Server) attachUpload(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		UploadID string `json:"upload_id"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("uploads:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	upload, err := s.store.GetUpload(r.Context(), body.UploadID, act.user.ID)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotWorkspace(w, act, upload.WorkspaceID, nil) {
		return
	}
	message, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id"))
	if !ok {
		return
	}
	if message.AuthorID != act.user.ID {
		writeError(w, http.StatusForbidden, errors.New("message attachments can only be changed by the message author"))
		return
	}
	err = s.store.AttachUpload(r.Context(), store.AttachUploadInput{MessageID: chi.URLParam(r, "message_id"), UploadID: body.UploadID, UserID: act.user.ID})
	writeResult(w, map[string]any{"ok": true}, err)
}

func (s *Server) listDirectConversations(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if err := act.requireScope("dms:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	items, err := s.store.ListDirectConversations(r.Context(), workspaceID, act.user.ID)
	writeResult(w, map[string]any{"conversations": items}, err)
}

func (s *Server) createDirectConversation(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		WorkspaceID string   `json:"workspace_id"`
		MemberIDs   []string `json:"member_ids"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("dms:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(body.WorkspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	dm, err := s.store.CreateDirectConversation(r.Context(), store.CreateDirectConversationInput{WorkspaceID: body.WorkspaceID, UserID: act.user.ID, MemberIDs: body.MemberIDs})
	writeResultStatus(w, http.StatusCreated, map[string]any{"conversation": dm}, err)
}

func (s *Server) listDirectMessages(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("dms:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	page, err := parseMessagePageRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireBotDirectWorkspace(w, r, act, chi.URLParam(r, "conversation_id")) {
		return
	}
	messages, err := s.store.ListDirectMessages(r.Context(), chi.URLParam(r, "conversation_id"), act.user.ID, page)
	writeMessagePage(w, messages, err)
}

func (s *Server) createDirectMessage(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Body            string `json:"body"`
		QuotedMessageID string `json:"quoted_message_id"`
		Nonce           string `json:"nonce"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("dms:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotDirectWorkspace(w, r, act, chi.URLParam(r, "conversation_id")) {
		return
	}
	message, event, err := s.store.CreateDirectMessage(r.Context(), store.CreateDirectMessageInput{ConversationID: chi.URLParam(r, "conversation_id"), AuthorID: act.user.ID, Body: body.Body, QuotedMessageID: optionalString(body.QuotedMessageID), Nonce: body.Nonce})
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
		s.notifyMessageCreated(r.Context(), message)
	}
	writeMessageCreateResult(w, message, event, err)
}

func (s *Server) mattermostWebhook(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := readJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	message, event, err := s.store.CreateMessage(r.Context(), store.CreateMessageInput{ChannelID: chi.URLParam(r, "channel_id"), AuthorID: act.user.ID, Body: body.Text})
	if err == nil {
		s.hub.Publish(event)
		s.notifyMessageCreated(r.Context(), message)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"message": message, "event": event}, err)
}

func (s *Server) slashCommand(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	command := strings.TrimSpace(r.FormValue("command"))
	if text == "" && command == "" {
		writeError(w, http.StatusBadRequest, errors.New("slash command text is required"))
		return
	}
	body := strings.TrimSpace(command + " " + text)
	message, event, err := s.store.CreateMessage(r.Context(), store.CreateMessageInput{ChannelID: chi.URLParam(r, "channel_id"), AuthorID: act.user.ID, Body: body})
	if err == nil {
		s.hub.Publish(event)
		s.notifyMessageCreated(r.Context(), message)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{
		"response_type": "in_channel",
		"text":          message.Body,
		"message":       message,
		"event":         event,
	}, err)
}
