package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	"github.com/openclaw/clickclack/apps/api/internal/store"
	"github.com/openclaw/clickclack/apps/api/internal/webassets"
)

type Server struct {
	store          store.Store
	hub            *realtime.Hub
	uploadDir      string
	githubOAuth    GitHubOAuthConfig
	disableDevAuth bool
}

type Options struct {
	UploadDir      string
	GitHubOAuth    GitHubOAuthConfig
	DisableDevAuth bool
}

func New(st store.Store, hub *realtime.Hub, options Options) *Server {
	return &Server{
		store:          st,
		hub:            hub,
		uploadDir:      options.UploadDir,
		githubOAuth:    options.GitHubOAuth.withDefaults(),
		disableDevAuth: options.DisableDevAuth,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/magic/request", s.requestMagicLink)
		r.Post("/auth/magic/consume", s.consumeMagicLink)
		r.Get("/auth/github/start", s.githubStart)
		r.Get("/auth/github/callback", s.githubCallback)
		r.Get("/me", s.me)
		r.Get("/workspaces", s.listWorkspaces)
		r.Post("/workspaces", s.createWorkspace)
		r.Get("/workspaces/{workspace_id}", s.getWorkspace)
		r.Get("/workspaces/{workspace_id}/channels", s.listChannels)
		r.Post("/workspaces/{workspace_id}/channels", s.createChannel)
		r.Patch("/channels/{channel_id}", s.updateChannel)
		r.Get("/channels/{channel_id}/messages", s.listMessages)
		r.Post("/channels/{channel_id}/messages", s.createMessage)
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
		r.Post("/hooks/mattermost/{channel_id}", s.mattermostWebhook)
		r.Post("/hooks/slash/{channel_id}", s.slashCommand)
	})

	r.NotFound(s.serveSPA)
	r.Head("/*", s.serveSPA)
	r.Get("/*", s.serveSPA)
	return r
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	items, err := s.store.ListWorkspaces(r.Context(), user.ID)
	writeResult(w, map[string]any{"workspaces": items}, err)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
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
	workspace, err := s.store.CreateWorkspace(r.Context(), store.CreateWorkspaceInput{Name: body.Name, Slug: body.Slug}, user.ID)
	writeResultStatus(w, http.StatusCreated, map[string]any{"workspace": workspace}, err)
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspace, err := s.store.GetWorkspace(r.Context(), chi.URLParam(r, "workspace_id"), user.ID)
	writeResult(w, map[string]any{"workspace": workspace}, err)
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	channels, err := s.store.ListChannels(r.Context(), chi.URLParam(r, "workspace_id"), user.ID)
	writeResult(w, map[string]any{"channels": channels}, err)
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
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
	channel, event, err := s.store.CreateChannel(r.Context(), store.CreateChannelInput{WorkspaceID: chi.URLParam(r, "workspace_id"), Name: body.Name, Kind: body.Kind, UserID: user.ID})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"channel": channel, "event": event}, err)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	messages, err := s.store.ListMessages(r.Context(), chi.URLParam(r, "channel_id"), user.ID, queryInt64(r, "after_seq", 0), queryInt(r, "limit", 100))
	writeResult(w, map[string]any{"messages": messages}, err)
}

func (s *Server) createMessage(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	message, event, err := s.store.CreateMessage(r.Context(), store.CreateMessageInput{ChannelID: chi.URLParam(r, "channel_id"), AuthorID: user.ID, Body: body.Body})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"message": message, "event": event}, err)
}

func (s *Server) getThread(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	root, replies, state, err := s.store.GetThread(r.Context(), chi.URLParam(r, "message_id"), user.ID, queryInt(r, "limit", 100))
	writeResult(w, map[string]any{"root": root, "replies": replies, "thread_state": state}, err)
}

func (s *Server) createThreadReply(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	message, state, events, err := s.store.CreateThreadReply(r.Context(), store.CreateThreadReplyInput{RootMessageID: chi.URLParam(r, "message_id"), AuthorID: user.ID, Body: body.Body})
	if err == nil {
		s.hub.PublishMany(events)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"message": message, "thread_state": state, "events": events}, err)
}

func (s *Server) addReaction(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	event, err := s.store.AddReaction(r.Context(), store.CreateReactionInput{MessageID: chi.URLParam(r, "message_id"), UserID: user.ID, Emoji: body.Emoji})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResultStatus(w, http.StatusCreated, map[string]any{"event": event}, err)
}

func (s *Server) removeReaction(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	event, err := s.store.RemoveReaction(r.Context(), store.CreateReactionInput{MessageID: chi.URLParam(r, "message_id"), UserID: user.ID, Emoji: chi.URLParam(r, "emoji")})
	if err == nil {
		s.hub.Publish(event)
	}
	writeResult(w, map[string]any{"event": event}, err)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	events, err := s.store.ListEventsAfter(r.Context(), r.URL.Query().Get("workspace_id"), user.ID, r.URL.Query().Get("after_cursor"), queryInt(r, "limit", 200))
	writeResult(w, map[string]any{"events": events}, err)
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, errors.New("workspace_id is required"))
		return
	}
	if _, err := s.store.GetWorkspace(r.Context(), workspaceID, user.ID); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx := r.Context()
	backlog, err := s.store.ListEventsAfter(ctx, workspaceID, user.ID, r.URL.Query().Get("after_cursor"), 500)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	for _, event := range backlog {
		if err := writeWS(ctx, conn, event); err != nil {
			return
		}
	}
	events, unsubscribe := s.hub.Subscribe(workspaceID)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			if err := writeWS(ctx, conn, event); err != nil {
				return
			}
		}
	}
}

func (s *Server) currentUser(r *http.Request) (store.User, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return s.store.GetSessionUser(r.Context(), strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")))
	}
	if cookie, err := r.Cookie("cc_session"); err == nil && cookie.Value != "" {
		return s.store.GetSessionUser(r.Context(), cookie.Value)
	}
	if s.disableDevAuth {
		return store.User{}, errors.New("authentication required")
	}
	if id := r.Header.Get("X-ClickClack-User"); id != "" {
		return s.store.GetUser(r.Context(), id)
	}
	return s.store.FirstUser(r.Context())
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
	index, err := fs.ReadFile(dist, "index.html")
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
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
