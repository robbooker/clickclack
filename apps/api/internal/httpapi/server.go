package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/uploadstore"
	"github.com/openclaw/clickclack/apps/api/internal/webassets"
)

type Server struct {
	store          store.Store
	hub            *realtime.Hub
	uploadDir      string
	uploadStorage  uploadstore.Store
	githubOAuth    GitHubOAuthConfig
	disableDevAuth bool
	pushNotifier   PushNotifier
}

const websocketBearerProtocolPrefix = "clickclack.bearer."

type actor struct {
	user        store.User
	botTokenID  string
	workspaceID string
	scopes      []string
}

type Options struct {
	UploadDir      string
	UploadStorage  uploadstore.Store
	GitHubOAuth    GitHubOAuthConfig
	DisableDevAuth bool
	PushNotifier   PushNotifier
}

func New(st store.Store, hub *realtime.Hub, options Options) *Server {
	uploadStorage := options.UploadStorage
	if uploadStorage == nil && options.UploadDir != "" {
		uploadStorage = uploadstore.NewLocal(options.UploadDir)
	}
	return &Server{
		store:          st,
		hub:            hub,
		uploadDir:      options.UploadDir,
		uploadStorage:  uploadStorage,
		githubOAuth:    options.GitHubOAuth.withDefaults(),
		disableDevAuth: options.DisableDevAuth,
		pushNotifier:   options.PushNotifier,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/magic/request", s.requestMagicLink)
		r.Post("/auth/magic/consume", s.consumeMagicLink)
		r.Get("/auth/github/start", s.githubStart)
		r.Get("/auth/github/callback", s.githubCallback)
		r.Get("/me", s.me)
		r.Patch("/me", s.updateMe)
		r.Get("/workspaces", s.listWorkspaces)
		r.Post("/workspaces", s.createWorkspace)
		r.Get("/routes/{workspace_route_id}/{target_route_id}", s.resolveRoute)
		r.Get("/workspaces/{workspace_id}", s.getWorkspace)
		r.Get("/workspaces/{workspace_id}/channels", s.listChannels)
		r.Post("/workspaces/{workspace_id}/channels", s.createChannel)
		r.Patch("/channels/{channel_id}", s.updateChannel)
		r.Get("/channels/{channel_id}/messages", s.listMessages)
		r.Post("/channels/{channel_id}/messages", s.createMessage)
		r.Post("/channels/{channel_id}/read", s.markChannelRead)
		r.Get("/messages/{message_id}", s.getMessage)
		r.Patch("/messages/{message_id}", s.updateMessage)
		r.Delete("/messages/{message_id}", s.deleteMessage)
		r.Get("/messages/{message_id}/thread", s.getThread)
		r.Post("/messages/{message_id}/thread/replies", s.createThreadReply)
		r.Post("/messages/{message_id}/reactions", s.addReaction)
		r.Delete("/messages/{message_id}/reactions/{emoji}", s.removeReaction)
		r.Get("/realtime/events", s.listEvents)
		r.Post("/realtime/ephemeral", s.publishEphemeral)
		r.Get("/realtime/ws", s.websocket)
		r.Get("/search", s.search)
		r.Post("/uploads", s.createUpload)
		r.Get("/uploads/{upload_id}", s.getUpload)
		r.Post("/messages/{message_id}/attachments", s.attachUpload)
		r.Get("/dms", s.listDirectConversations)
		r.Post("/dms", s.createDirectConversation)
		r.Get("/dms/{conversation_id}/messages", s.listDirectMessages)
		r.Post("/dms/{conversation_id}/messages", s.createDirectMessage)
		r.Post("/dms/{conversation_id}/read", s.markDirectRead)
		r.Post("/hooks/mattermost/{channel_id}", s.mattermostWebhook)
		r.Post("/hooks/slash/{channel_id}", s.slashCommand)
	})

	r.NotFound(s.serveSPA)
	r.Head("/*", s.serveSPA)
	r.Get("/*", s.serveSPA)
	return r
}

func (s *Server) resolveRoute(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceRouteID := chi.URLParam(r, "workspace_route_id")
	targetRouteID := chi.URLParam(r, "target_route_id")
	scope := routeScopeForParam(targetRouteID)
	if scope == "" {
		writeError(w, http.StatusNotFound, errors.New("route not found"))
		return
	}
	if err := act.requireScope(scope); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var target store.RouteTarget
	if isLegacyRouteParam(workspaceRouteID) || isLegacyRouteParam(targetRouteID) {
		target, err = s.store.ResolveLegacyRouteTarget(r.Context(), act.user.ID, workspaceRouteID, targetRouteID)
	} else {
		target, err = s.store.ResolveRouteTarget(r.Context(), act.user.ID, workspaceRouteID, targetRouteID)
	}
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("route not found"))
		return
	}
	if err := act.requireWorkspace(target.WorkspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if scope != routeScopeForTargetType(target.TargetType) {
		writeError(w, http.StatusNotFound, errors.New("route not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"route": target})
}

func routeScopeForParam(value string) string {
	switch {
	case strings.HasPrefix(value, "C"), strings.HasPrefix(value, "chn_"):
		return "channels:read"
	case strings.HasPrefix(value, "D"), strings.HasPrefix(value, "dm_"):
		return "dms:read"
	case strings.HasPrefix(value, "M"), strings.HasPrefix(value, "msg_"):
		return "threads:read"
	default:
		return ""
	}
}

func routeScopeForTargetType(targetType string) string {
	switch targetType {
	case "channel":
		return "channels:read"
	case "direct":
		return "dms:read"
	case "thread":
		return "threads:read"
	default:
		return ""
	}
}

func isLegacyRouteParam(value string) bool {
	return strings.HasPrefix(value, "wsp_") ||
		strings.HasPrefix(value, "chn_") ||
		strings.HasPrefix(value, "dm_") ||
		strings.HasPrefix(value, "msg_")
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("profile:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": act.user})
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if act.botTokenID != "" {
		writeError(w, http.StatusForbidden, errors.New("bot tokens cannot update profiles"))
		return
	}
	var body struct {
		DisplayName          string                      `json:"display_name"`
		Handle               string                      `json:"handle"`
		AvatarURL            string                      `json:"avatar_url"`
		NotificationSettings *store.NotificationSettings `json:"notification_settings"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.store.UpdateUserProfile(r.Context(), store.UpdateUserProfileInput{
		UserID:      act.user.ID,
		DisplayName: body.DisplayName,
		Handle:      body.Handle,
		AvatarURL:   body.AvatarURL,
	})
	if err == nil && body.NotificationSettings != nil {
		_, err = s.store.UpdateNotificationSettings(r.Context(), store.UpdateNotificationSettingsInput{
			UserID:          act.user.ID,
			PushoverEnabled: body.NotificationSettings.PushoverEnabled,
			PushoverUserKey: body.NotificationSettings.PushoverUserKey,
		})
		if err == nil {
			updated, err = s.store.GetUser(r.Context(), act.user.ID)
		}
	}
	writeResult(w, map[string]any{"user": updated}, err)
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("workspaces:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	items, err := s.store.ListWorkspaces(r.Context(), act.user.ID)
	if err == nil && act.botTokenID != "" {
		filtered := items[:0]
		for _, item := range items {
			if item.ID == act.workspaceID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	writeResult(w, map[string]any{"workspaces": items}, err)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if act.botTokenID != "" {
		writeError(w, http.StatusForbidden, errors.New("bot tokens cannot create workspaces"))
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workspace, err := s.store.CreateWorkspace(r.Context(), store.CreateWorkspaceInput{Name: body.Name, Slug: body.Slug}, act.user.ID)
	writeResultStatus(w, http.StatusCreated, map[string]any{"workspace": workspace}, err)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := chi.URLParam(r, "workspace_id")
	if err := act.requireScope("workspaces:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	workspace, err := s.store.GetWorkspace(r.Context(), workspaceID, act.user.ID)
	writeResult(w, map[string]any{"workspace": workspace}, err)
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := chi.URLParam(r, "workspace_id")
	if err := act.requireScope("channels:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	channels, err := s.store.ListChannels(r.Context(), workspaceID, act.user.ID)
	writeResult(w, map[string]any{"channels": channels}, err)
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("channels:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(chi.URLParam(r, "workspace_id")); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var body struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	channel, event, err := s.store.CreateChannel(r.Context(), store.CreateChannelInput{WorkspaceID: chi.URLParam(r, "workspace_id"), Name: body.Name, Kind: body.Kind, UserID: act.user.ID})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"channel": channel, "event": event}, err)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	page, err := parseMessagePageRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	messages, err := s.store.ListMessages(r.Context(), chi.URLParam(r, "channel_id"), act.user.ID, page)
	writeMessagePage(w, messages, err)
}

func (s *Server) createMessage(w http.ResponseWriter, r *http.Request) {
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
		Body            string `json:"body"`
		QuotedMessageID string `json:"quoted_message_id"`
		Nonce           string `json:"nonce"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !s.requireBotChannelWorkspace(w, r, act, chi.URLParam(r, "channel_id")) {
		return
	}
	message, event, err := s.store.CreateMessage(r.Context(), store.CreateMessageInput{ChannelID: chi.URLParam(r, "channel_id"), AuthorID: act.user.ID, Body: body.Body, QuotedMessageID: optionalString(body.QuotedMessageID), Nonce: body.Nonce})
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
		s.notifyMessageCreated(r.Context(), message)
	}
	writeMessageCreateResult(w, message, event, err)
}

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("messages:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	message, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id"))
	if !ok {
		return
	}
	if act.botTokenID == "" {
		message, err = s.store.GetMessage(r.Context(), chi.URLParam(r, "message_id"), act.user.ID)
	}
	writeResult(w, map[string]any{"message": message}, err)
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("threads:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if _, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id")); !ok {
		return
	}
	root, replies, state, err := s.store.GetThread(r.Context(), chi.URLParam(r, "message_id"), act.user.ID, queryInt(r, "limit", 100))
	writeResult(w, map[string]any{"root": root, "replies": replies, "thread_state": state}, err)
}

func (s *Server) createThreadReply(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("threads:write"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var body struct {
		Body            string `json:"body"`
		QuotedMessageID string `json:"quoted_message_id"`
		Nonce           string `json:"nonce"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id")); !ok {
		return
	}
	message, state, events, err := s.store.CreateThreadReply(r.Context(), store.CreateThreadReplyInput{RootMessageID: chi.URLParam(r, "message_id"), AuthorID: act.user.ID, Body: body.Body, QuotedMessageID: optionalString(body.QuotedMessageID), Nonce: body.Nonce})
	if err == nil && len(events) > 0 {
		s.hub.PublishMany(events)
		s.notifyMessageCreated(r.Context(), message)
	}
	writeThreadReplyCreateResult(w, message, state, events, err)
}

func (s *Server) addReaction(w http.ResponseWriter, r *http.Request) {
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
		Emoji string `json:"emoji"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, ok := s.requireBotMessageWorkspace(w, r, act, chi.URLParam(r, "message_id")); !ok {
		return
	}
	event, err := s.store.AddReaction(r.Context(), store.CreateReactionInput{MessageID: chi.URLParam(r, "message_id"), UserID: act.user.ID, Emoji: body.Emoji})
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
	}
	writeEventMutationResult(w, http.StatusCreated, event, err)
}

func (s *Server) removeReaction(w http.ResponseWriter, r *http.Request) {
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
	event, err := s.store.RemoveReaction(r.Context(), store.CreateReactionInput{MessageID: chi.URLParam(r, "message_id"), UserID: act.user.ID, Emoji: chi.URLParam(r, "emoji")})
	if err == nil && event.ID != "" {
		s.hub.Publish(event)
	}
	writeEventMutationResult(w, http.StatusOK, event, err)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if err := act.requireScope("realtime:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	events, err := s.store.ListEventsAfter(r.Context(), workspaceID, act.user.ID, r.URL.Query().Get("after_cursor"), queryInt(r, "limit", 200))
	if err == nil {
		events = filterEventsForUser(events, act.user.ID)
	}
	writeResult(w, map[string]any{"events": events}, err)
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	bearerProtocol := websocketBearerProtocol(r)
	if r.Header.Get("Authorization") == "" {
		if bearerProtocol != "" {
			r.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(bearerProtocol, websocketBearerProtocolPrefix))
		}
	}
	act, err := s.currentActor(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if err := act.requireScope("realtime:read"); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, errors.New("workspace_id is required"))
		return
	}
	if err := act.requireWorkspace(workspaceID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	if _, err := s.store.GetWorkspace(r.Context(), workspaceID, act.user.ID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	events, unsubscribe := s.hub.Subscribe(workspaceID)
	defer unsubscribe()
	acceptOptions := &websocket.AcceptOptions{OriginPatterns: s.websocketOriginPatterns(r)}
	if bearerProtocol != "" {
		acceptOptions.Subprotocols = []string{bearerProtocol}
	}
	conn, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	backlog, err := s.store.ListEventsAfter(ctx, workspaceID, act.user.ID, r.URL.Query().Get("after_cursor"), 500)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	sent := make(map[string]struct{}, len(backlog))
	for _, event := range backlog {
		if event.ID != "" {
			sent[event.ID] = struct{}{}
		}
		if !shouldDeliverEvent(event, act.user.ID) {
			continue
		}
		if err := writeWS(ctx, conn, event); err != nil {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if event.ID != "" {
				if _, ok := sent[event.ID]; ok {
					continue
				}
			}
			if !shouldDeliverEvent(event, act.user.ID) {
				continue
			}
			if err := writeWS(ctx, conn, event); err != nil {
				return
			}
		}
	}
}

func websocketBearerToken(r *http.Request) string {
	return strings.TrimPrefix(websocketBearerProtocol(r), websocketBearerProtocolPrefix)
}

func websocketBearerProtocol(r *http.Request) string {
	for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, websocketBearerProtocolPrefix) {
			return protocol
		}
	}
	return ""
}

func (s *Server) websocketOriginPatterns(r *http.Request) []string {
	publicURL, err := url.Parse(strings.TrimSpace(s.githubOAuth.PublicURL))
	if err != nil || publicURL.Host == "" {
		return nil
	}
	return []string{publicURL.Scheme + "://" + publicURL.Host}
}

// shouldDeliverEvent gates per-user-private events so they only reach allowed
// sessions and never leak to other workspace members.
func shouldDeliverEvent(event store.Event, userID string) bool {
	if len(event.RecipientUserIDs) > 0 {
		for _, allowed := range event.RecipientUserIDs {
			if allowed == userID {
				return true
			}
		}
		return false
	}
	switch event.Type {
	case "channel.read", "dm.read":
		payload, ok := event.Payload.(map[string]string)
		if !ok {
			// Backlog payloads come back via ListEventsAfter as map[string]any.
			if anyPayload, ok := event.Payload.(map[string]any); ok {
				if v, _ := anyPayload["user_id"].(string); v != "" {
					return v == userID
				}
				return false
			}
			return false
		}
		return payload["user_id"] == userID
	}
	return true
}

func filterEventsForUser(events []store.Event, userID string) []store.Event {
	filtered := events[:0]
	for _, event := range events {
		if shouldDeliverEvent(event, userID) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func (s *Server) currentActor(r *http.Request) (actor, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if botAuth, err := s.store.GetBotTokenAuth(r.Context(), token); err == nil {
			return actor{
				user:        botAuth.User,
				botTokenID:  botAuth.TokenID,
				workspaceID: botAuth.WorkspaceID,
				scopes:      botAuth.Scopes,
			}, nil
		}
		user, err := s.store.GetSessionUser(r.Context(), token)
		return actor{user: user}, err
	}
	if cookie, err := r.Cookie("cc_session"); err == nil && cookie.Value != "" {
		user, err := s.store.GetSessionUser(r.Context(), cookie.Value)
		return actor{user: user}, err
	}
	if s.disableDevAuth {
		return actor{}, errors.New("authentication required")
	}
	if !isLocalDevRequest(r) {
		return actor{}, errors.New("authentication required")
	}
	if id := r.Header.Get("X-ClickClack-User"); id != "" {
		user, err := s.store.GetUser(r.Context(), id)
		return actor{user: user}, err
	}
	user, err := s.store.FirstUser(r.Context())
	return actor{user: user}, err
}

func (a actor) requireScope(scope string) error {
	if a.botTokenID == "" {
		return nil
	}
	for _, candidate := range a.scopes {
		if candidate == scope {
			return nil
		}
	}
	return errors.New("bot token is missing scope " + scope)
}

func (a actor) requireWorkspace(workspaceID string) error {
	if a.botTokenID == "" {
		return nil
	}
	if a.workspaceID == workspaceID {
		return nil
	}
	return errors.New("bot token cannot access this workspace")
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	dist, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if r.URL.Path != "/" {
		if file, err := dist.Open(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
			_ = file.Close()
			http.FileServer(http.FS(dist)).ServeHTTP(w, r)
			return
		}
	}
	fallback := "index.html"
	if r.URL.Path != "/" {
		if _, err := fs.Stat(dist, "200.html"); err == nil {
			fallback = "200.html"
		}
	}
	index, err := fs.ReadFile(dist, fallback)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(index)
}

func writeWS(ctx context.Context, conn *websocket.Conn, event store.Event) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, body)
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

func writeResult(w http.ResponseWriter, body any, err error) {
	writeResultStatus(w, http.StatusOK, body, err)
}

func writeResultStatus(w http.ResponseWriter, status int, body any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, status, body)
}

func writeMessageCreateResult(w http.ResponseWriter, message store.Message, event store.Event, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body := map[string]any{"message": message}
	status := http.StatusOK
	if event.ID != "" {
		body["event"] = event
		status = http.StatusCreated
	}
	writeJSON(w, status, body)
}

func writeThreadReplyCreateResult(w http.ResponseWriter, message store.Message, state store.ThreadState, events []store.Event, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusOK
	if len(events) > 0 {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"message": message, "thread_state": state, "events": events})
}

func writeEventMutationResult(w http.ResponseWriter, changedStatus int, event store.Event, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	status := http.StatusOK
	if event.ID != "" {
		status = changedStatus
	}
	writeJSON(w, status, map[string]any{"event": event})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

// optionalString returns a non-empty trimmed pointer or nil. Useful for JSON
// fields that should map to a nullable Go pointer when absent or blank.
func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	return value
}

func queryInt64(r *http.Request, key string, fallback int64) int64 {
	value, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func parseMessagePageRequest(r *http.Request) (store.MessagePageRequest, error) {
	values := r.URL.Query()
	req := store.MessagePageRequest{Limit: queryInt(r, "limit", 100)}
	cursorCount := 0
	for _, cursor := range []struct {
		key string
		set func(int64)
	}{
		{"before_seq", func(v int64) { req.BeforeSeq = &v }},
		{"after_seq", func(v int64) { req.AfterSeq = &v }},
		{"around_seq", func(v int64) { req.AroundSeq = &v }},
	} {
		raw, ok := values[cursor.key]
		if !ok {
			continue
		}
		cursorCount++
		if len(raw) == 0 || strings.TrimSpace(raw[0]) == "" {
			return req, fmt.Errorf("%w: %s is required", store.ErrInvalidMessagePage, cursor.key)
		}
		value, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil || value < 0 {
			return req, fmt.Errorf("%w: %s must be a non-negative integer", store.ErrInvalidMessagePage, cursor.key)
		}
		cursor.set(value)
	}
	if cursorCount > 1 {
		return req, fmt.Errorf("%w: before_seq, after_seq, and around_seq are mutually exclusive", store.ErrInvalidMessagePage)
	}
	if mode := values.Get("mode"); mode != "" {
		if mode != "latest" {
			return req, fmt.Errorf("%w: unsupported message page mode %q", store.ErrInvalidMessagePage, mode)
		}
		if cursorCount > 0 {
			return req, fmt.Errorf("%w: mode and cursor params are mutually exclusive", store.ErrInvalidMessagePage)
		}
	}
	return req, nil
}

func writeMessagePage(w http.ResponseWriter, page store.MessagePage, err error) {
	writeResult(w, map[string]any{
		"messages":   page.Messages,
		"oldest_seq": page.OldestSeq,
		"newest_seq": page.NewestSeq,
		"has_older":  page.HasOlder,
		"has_newer":  page.HasNewer,
	}, err)
}

func ListenAndServe(ctx context.Context, addr string, handler http.Handler) error {
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve %s: %w", addr, err)
}
