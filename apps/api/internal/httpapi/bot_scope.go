package httpapi

import (
	"net/http"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func (s *Server) requireBotChannelWorkspace(w http.ResponseWriter, r *http.Request, act actor, channelID string) bool {
	if act.botTokenID == "" {
		return true
	}
	channel, err := s.store.GetChannel(r.Context(), channelID, act.user.ID)
	return s.requireBotWorkspace(w, act, channel.WorkspaceID, err)
}

func (s *Server) requireBotMessageWorkspace(w http.ResponseWriter, r *http.Request, act actor, messageID string) (store.Message, bool) {
	message, err := s.store.GetMessage(r.Context(), messageID, act.user.ID)
	return message, s.requireBotWorkspace(w, act, message.WorkspaceID, err)
}

func (s *Server) requireBotDirectWorkspace(w http.ResponseWriter, r *http.Request, act actor, conversationID string) bool {
	if act.botTokenID == "" {
		return true
	}
	dm, err := s.store.GetDirectConversation(r.Context(), conversationID, act.user.ID)
	return s.requireBotWorkspace(w, act, dm.WorkspaceID, err)
}

func (s *Server) requireBotWorkspace(w http.ResponseWriter, act actor, workspaceID string, err error) bool {
	if act.botTokenID == "" {
		return true
	}
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return false
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return false
	}
	return true
}
