package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("channels:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Archived *bool  `json:"archived"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	channel, event, err := s.store.UpdateChannel(r.Context(), store.UpdateChannelInput{ChannelID: chi.URLParam(r, "channel_id"), UserID: act.user.ID, Name: body.Name, Kind: body.Kind, Archived: body.Archived})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"channel": channel, "event": event}, err)
}

func (s *Server) updateMessage(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id")); !ok {
		return
	}
	message, event, err := s.store.UpdateMessage(r.Context(), store.UpdateMessageInput{MessageID: chi.URLParam(r, "message_id"), UserID: act.user.ID, Body: body.Body})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"message": message, "event": event}, err)
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if _, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id")); !ok {
		return
	}
	message, event, err := s.store.DeleteMessage(r.Context(), store.DeleteMessageInput{MessageID: chi.URLParam(r, "message_id"), UserID: act.user.ID})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"message": message, "event": event}, err)
}

func (s *Server) publishEphemeral(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var body struct {
		WorkspaceID          string         `json:"workspace_id"`
		ChannelID            string         `json:"channel_id"`
		DirectConversationID string         `json:"direct_conversation_id"`
		Type                 string         `json:"type"`
		Payload              map[string]any `json:"payload"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Type != "typing.started" && body.Type != "typing.stopped" && body.Type != "presence.changed" {
		writeError(w, http.StatusBadRequest, errors.New("unsupported ephemeral event type"))
		return
	}
	if err := act.requireWorkspace(body.WorkspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if _, err := s.store.GetWorkspace(r.Context(), body.WorkspaceID, act.user.ID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if body.Payload == nil {
		body.Payload = map[string]any{}
	}
	directConversationID := body.DirectConversationID
	if directConversationID == "" {
		if payloadID, _ := body.Payload["direct_conversation_id"].(string); payloadID != "" {
			directConversationID = payloadID
		}
	}
	var recipientUserIDs []string
	if directConversationID != "" {
		if body.ChannelID != "" {
			writeError(w, http.StatusBadRequest, errors.New("channel_id and direct_conversation_id are mutually exclusive"))
			return
		}
		dm, err := s.store.GetDirectConversation(r.Context(), directConversationID, act.user.ID)
		if err != nil || dm.WorkspaceID != body.WorkspaceID {
			if err == nil {
				err = errors.New("direct conversation is not in this workspace")
			}
			writeError(w, http.StatusForbidden, err)
			return
		}
		recipientUserIDs = make([]string, 0, len(dm.Members))
		for _, member := range dm.Members {
			recipientUserIDs = append(recipientUserIDs, member.ID)
		}
		body.Payload["direct_conversation_id"] = directConversationID
	}
	body.Payload["user_id"] = act.user.ID
	event := store.Event{
		ID:               "eph_" + time.Now().UTC().Format("20060102150405.000000000"),
		Type:             body.Type,
		WorkspaceID:      body.WorkspaceID,
		ChannelID:        body.ChannelID,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		Payload:          body.Payload,
		RecipientUserIDs: recipientUserIDs,
	}
	s.hub.Publish(event)
	writeJSON(w, http.StatusAccepted, map[string]any{"event": event})
}
