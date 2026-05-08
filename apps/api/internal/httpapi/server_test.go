package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	"github.com/openclaw/clickclack/apps/api/internal/store"
	sqlitestore "github.com/openclaw/clickclack/apps/api/internal/store/sqlite"
)

func TestChatAPIVerticalSlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := sqlitestore.Open("sqlite://" + filepath.Join(dataDir, "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Second", Email: "second@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub()
	server := httptest.NewServer(New(st, hub, Options{UploadDir: filepath.Join(dataDir, "uploads")}).Handler())
	t.Cleanup(server.Close)

	expectStatus(t, http.MethodHead, server.URL+"/", nil, http.StatusOK)

	me := getJSON[struct {
		User store.User `json:"user"`
	}](t, server.URL+"/api/me")
	if me.User.ID != owner.ID {
		t.Fatalf("expected owner %s, got %s", owner.ID, me.User.ID)
	}

	workspaces := getJSON[struct {
		Workspaces []store.Workspace `json:"workspaces"`
	}](t, server.URL+"/api/workspaces")
	workspace := workspaces.Workspaces[0]
	createdWorkspace := postJSON[struct {
		Workspace store.Workspace `json:"workspace"`
	}](t, server.URL+"/api/workspaces", map[string]string{"name": "Side Dock"})
	if createdWorkspace.Workspace.Slug != "side-dock" {
		t.Fatalf("unexpected workspace slug %q", createdWorkspace.Workspace.Slug)
	}
	gotWorkspace := getJSON[struct {
		Workspace store.Workspace `json:"workspace"`
	}](t, server.URL+"/api/workspaces/"+workspace.ID)
	if gotWorkspace.Workspace.ID != workspace.ID {
		t.Fatalf("unexpected workspace response: %#v", gotWorkspace.Workspace)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, second.ID, "member"); err != nil {
		t.Fatal(err)
	}

	channels := getJSON[struct {
		Channels []store.Channel `json:"channels"`
	}](t, server.URL+"/api/workspaces/"+workspace.ID+"/channels")
	channel := channels.Channels[0]
	createdChannel := postJSON[struct {
		Channel store.Channel `json:"channel"`
	}](t, server.URL+"/api/workspaces/"+workspace.ID+"/channels", map[string]string{"name": "random"})
	if createdChannel.Channel.Name != "random" {
		t.Fatalf("unexpected channel: %#v", createdChannel.Channel)
	}

	wsURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/api/realtime/ws?workspace_id=" + url.QueryEscape(workspace.ID)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	created := postJSON[struct {
		Message store.Message `json:"message"`
		Event   store.Event   `json:"event"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages", map[string]string{"body": "findable **lobster**"})
	if created.Message.ChannelSeq == nil || *created.Message.ChannelSeq != 1 {
		t.Fatalf("unexpected channel seq: %#v", created.Message.ChannelSeq)
	}
	if event := readEventType(t, conn, "message.created"); event.Type != "message.created" {
		t.Fatalf("unexpected websocket event %s", event.Type)
	}

	messages := getJSON[struct {
		Messages []store.Message `json:"messages"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages")
	if len(messages.Messages) != 1 {
		t.Fatalf("expected one root message, got %d", len(messages.Messages))
	}

	reply := postJSON[struct {
		Message     store.Message     `json:"message"`
		ThreadState store.ThreadState `json:"thread_state"`
	}](t, server.URL+"/api/messages/"+created.Message.ID+"/thread/replies", map[string]string{"body": "thread _reply_"})
	if reply.ThreadState.ReplyCount != 1 {
		t.Fatalf("expected reply count 1, got %d", reply.ThreadState.ReplyCount)
	}

	thread := getJSON[struct {
		Root        store.Message     `json:"root"`
		Replies     []store.Message   `json:"replies"`
		ThreadState store.ThreadState `json:"thread_state"`
	}](t, server.URL+"/api/messages/"+created.Message.ID+"/thread")
	if thread.Root.ID != created.Message.ID || len(thread.Replies) != 1 {
		t.Fatalf("unexpected thread payload: %#v", thread)
	}

	search := getJSON[struct {
		Results []store.SearchResult `json:"results"`
	}](t, server.URL+"/api/search?workspace_id="+url.QueryEscape(workspace.ID)+"&q=lobster")
	if len(search.Results) != 1 || search.Results[0].Message.ID != created.Message.ID {
		t.Fatalf("unexpected search results: %#v", search.Results)
	}

	upload := uploadFile(t, server.URL+"/api/uploads", workspace.ID, "note.txt", "hello upload")
	attach := postJSON[map[string]bool](t, server.URL+"/api/messages/"+created.Message.ID+"/attachments", map[string]string{"upload_id": upload.ID})
	if !attach["ok"] {
		t.Fatal("expected attachment success")
	}
	body := getBody(t, server.URL+"/api/uploads/"+upload.ID)
	if body != "hello upload" {
		t.Fatalf("unexpected upload body %q", body)
	}

	reaction := postJSON[struct {
		Event store.Event `json:"event"`
	}](t, server.URL+"/api/messages/"+created.Message.ID+"/reactions", map[string]string{"emoji": "lobster"})
	if reaction.Event.Type != "reaction.added" {
		t.Fatalf("unexpected reaction event: %s", reaction.Event.Type)
	}
	deleteJSON(t, server.URL+"/api/messages/"+created.Message.ID+"/reactions/lobster")

	dm := postJSON[struct {
		Conversation store.DirectConversation `json:"conversation"`
	}](t, server.URL+"/api/dms", map[string]any{"workspace_id": workspace.ID, "member_ids": []string{second.ID}})
	if len(dm.Conversation.Members) != 2 {
		t.Fatalf("expected two dm members, got %d", len(dm.Conversation.Members))
	}
	dmMessage := postJSON[struct {
		Message store.Message `json:"message"`
	}](t, server.URL+"/api/dms/"+dm.Conversation.ID+"/messages", map[string]string{"body": "private click"})
	if dmMessage.Message.DirectConversationID != dm.Conversation.ID {
		t.Fatalf("unexpected dm message: %#v", dmMessage.Message)
	}
	dms := getJSON[struct {
		Conversations []store.DirectConversation `json:"conversations"`
	}](t, server.URL+"/api/dms?workspace_id="+url.QueryEscape(workspace.ID))
	if len(dms.Conversations) != 1 {
		t.Fatalf("expected one dm conversation, got %d", len(dms.Conversations))
	}
	dmMessages := getJSON[struct {
		Messages []store.Message `json:"messages"`
	}](t, server.URL+"/api/dms/"+dm.Conversation.ID+"/messages")
	if len(dmMessages.Messages) != 1 {
		t.Fatalf("expected one dm message, got %d", len(dmMessages.Messages))
	}

	webhook := postJSON[struct {
		Message store.Message `json:"message"`
	}](t, server.URL+"/api/hooks/mattermost/"+channel.ID, map[string]string{"text": "from webhook"})
	if webhook.Message.Body != "from webhook" {
		t.Fatalf("unexpected webhook body %q", webhook.Message.Body)
	}
	slash := postForm[struct {
		Text    string        `json:"text"`
		Message store.Message `json:"message"`
	}](t, server.URL+"/api/hooks/slash/"+channel.ID, url.Values{"command": {"/clack"}, "text": {"from slash"}})
	if slash.Text != "/clack from slash" || slash.Message.Body != slash.Text {
		t.Fatalf("unexpected slash response: %#v", slash)
	}

	events := getJSON[struct {
		Events []store.Event `json:"events"`
	}](t, server.URL+"/api/realtime/events?workspace_id="+url.QueryEscape(workspace.ID)+"&after_cursor="+url.QueryEscape(created.Event.Cursor))
	if len(events.Events) == 0 {
		t.Fatal("expected recoverable events after cursor")
	}
}

func TestHTTPErrorPathsAndSPA(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := sqlitestore.Open("sqlite://" + filepath.Join(dataDir, "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(st, realtime.NewHub(), Options{UploadDir: filepath.Join(dataDir, "uploads")}).Handler())
	t.Cleanup(server.Close)

	index := getBody(t, server.URL+"/")
	if !strings.Contains(index, "ClickClack") {
		t.Fatalf("expected embedded app shell, got %q", index)
	}
	fallback := getBody(t, server.URL+"/not-a-real-route")
	if !strings.Contains(fallback, "ClickClack") {
		t.Fatal("expected SPA fallback")
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-ClickClack-User", owner.ID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected header auth success, got %s", resp.Status)
	}
	resp.Body.Close()

	link := postJSON[struct {
		Token string `json:"token"`
	}](t, server.URL+"/api/auth/magic/request", map[string]string{"email": "auth@example.com", "display_name": "Auth User"})
	auth := postJSON[struct {
		User    store.User    `json:"user"`
		Session store.Session `json:"session"`
	}](t, server.URL+"/api/auth/magic/consume", map[string]string{"token": link.Token})
	if auth.User.DisplayName != "Auth User" || auth.Session.Token == "" {
		t.Fatalf("unexpected auth payload: %#v", auth)
	}
	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+auth.Session.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected bearer auth success, got %s %s", resp.Status, string(body))
	}

	expectStatus(t, http.MethodPost, server.URL+"/api/workspaces", strings.NewReader("{"), http.StatusBadRequest)
	expectStatus(t, http.MethodPost, server.URL+"/api/auth/magic/request", strings.NewReader(`{"email":""}`), http.StatusBadRequest)
	expectStatus(t, http.MethodPost, server.URL+"/api/auth/magic/consume", strings.NewReader(`{"token":"missing"}`), http.StatusBadRequest)
	expectStatus(t, http.MethodPost, server.URL+"/api/workspaces/missing/channels", strings.NewReader(`{"name":"x"}`), http.StatusBadRequest)
	expectStatus(t, http.MethodGet, server.URL+"/api/realtime/ws", nil, http.StatusBadRequest)
	expectStatus(t, http.MethodPost, server.URL+"/api/uploads", strings.NewReader("not multipart"), http.StatusBadRequest)
	expectStatus(t, http.MethodGet, server.URL+"/api/uploads/missing", nil, http.StatusNotFound)
	expectStatus(t, http.MethodGet, server.URL+"/api/search?workspace_id=missing&q=x", nil, http.StatusBadRequest)
}

func getJSON[T any](t *testing.T, endpoint string) T {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: %s %s", endpoint, resp.Status, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postJSON[T any](t *testing.T, endpoint string, body any) T {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: %s %s", endpoint, resp.Status, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postForm[T any](t *testing.T, endpoint string, form url.Values) T {
	t.Helper()
	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: %s %s", endpoint, resp.Status, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func deleteJSON(t *testing.T, endpoint string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("DELETE %s: %s %s", endpoint, resp.Status, string(body))
	}
}

func expectStatus(t *testing.T, method, endpoint string, body io.Reader, status int) {
	t.Helper()
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: expected %d, got %s %s", method, endpoint, status, resp.Status, string(payload))
	}
}

func uploadFile(t *testing.T, endpoint, workspaceID, filename, content string) store.Upload {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("workspace_id", workspaceID); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(endpoint, writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: %s %s", resp.Status, string(body))
	}
	var out struct {
		Upload store.Upload `json:"upload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Upload
}

func getBody(t *testing.T, endpoint string) string {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s: %s %s", endpoint, resp.Status, string(body))
	}
	return string(body)
}

func TestDisableDevAuthRequiresSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataDir := t.TempDir()
	st, err := sqlitestore.Open("sqlite://" + filepath.Join(dataDir, "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(st, realtime.NewHub(), Options{DisableDevAuth: true}).Handler())
	t.Cleanup(server.Close)

	expectStatus(t, http.MethodGet, server.URL+"/api/me", nil, http.StatusUnauthorized)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-ClickClack-User", owner.ID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected dev user header to be ignored, got %s", resp.Status)
	}

	session, err := st.CreateSession(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "cc_session", Value: session.Token})
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected session auth, got %s", resp.Status)
	}
}

func TestQueryHelpersParseValues(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/?limit=42&after_seq=123", nil)
	if got := queryInt(req, "limit", 10); got != 42 {
		t.Fatalf("unexpected int query value %d", got)
	}
	if got := queryInt64(req, "after_seq", 10); got != 123 {
		t.Fatalf("unexpected int64 query value %d", got)
	}
}

func readEventType(t *testing.T, conn *websocket.Conn, eventType string) store.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, body, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var event store.Event
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type == eventType {
			return event
		}
	}
}
