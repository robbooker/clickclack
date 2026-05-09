package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
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
	profile := patchJSON[struct {
		User store.User `json:"user"`
	}](t, server.URL+"/api/me", map[string]string{
		"display_name": "Peter Steinberger",
		"handle":       "@steipete",
		"avatar_url":   "https://example.com/avatar.png",
	})
	if profile.User.DisplayName != "Peter Steinberger" || profile.User.Handle != "steipete" || profile.User.AvatarURL == "" {
		t.Fatalf("unexpected profile response: %#v", profile.User)
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
	} else if payload, ok := event.Payload.(map[string]any); !ok || payload["author_id"] != owner.ID || event.Seq == nil || *event.Seq != 1 {
		t.Fatalf("unexpected message.created event payload: %#v", event)
	}
	nonceCreated, nonceStatus := postJSONWithStatus[struct {
		Message store.Message `json:"message"`
		Event   *store.Event  `json:"event,omitempty"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages", map[string]string{"body": "idempotent post", "nonce": "http-nonce-1"})
	if nonceStatus != http.StatusCreated || nonceCreated.Event == nil || nonceCreated.Message.Nonce != "http-nonce-1" {
		t.Fatalf("unexpected nonce create response: status=%d payload=%#v", nonceStatus, nonceCreated)
	}
	nonceReplay, replayStatus := postJSONWithStatus[struct {
		Message store.Message `json:"message"`
		Event   *store.Event  `json:"event,omitempty"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages", map[string]string{"body": "idempotent post", "nonce": "http-nonce-1"})
	if replayStatus != http.StatusOK || nonceReplay.Message.ID != nonceCreated.Message.ID || nonceReplay.Event != nil {
		t.Fatalf("unexpected nonce replay response: status=%d payload=%#v", replayStatus, nonceReplay)
	}

	messages := getJSON[struct {
		Messages []store.Message `json:"messages"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages")
	if len(messages.Messages) != 2 {
		t.Fatalf("expected two root messages, got %d", len(messages.Messages))
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
	rawUpload := uploadFileWithoutPartContentType(t, server.URL+"/api/uploads", workspace.ID)
	if rawUpload.ContentType != "application/octet-stream" || rawUpload.Width != 33 || rawUpload.Height != 0 || rawUpload.DurationMS != 0 {
		t.Fatalf("unexpected raw upload metadata: %#v", rawUpload)
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

func TestMessagePageHTTPCursors(t *testing.T) {
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
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx, workspaces[0].ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel := channels[0]
	for i := 1; i <= 12; i++ {
		if _, _, err := st.CreateMessage(ctx, store.CreateMessageInput{
			ChannelID: channel.ID,
			AuthorID:  owner.ID,
			Body:      fmt.Sprintf("http page %02d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(New(st, realtime.NewHub(), Options{}).Handler())
	t.Cleanup(server.Close)
	base := server.URL + "/api/channels/" + channel.ID + "/messages"

	latest := getJSON[store.MessagePage](t, base+"?limit=5")
	expectHTTPSeqs(t, latest.Messages, 8, 12)
	if !latest.HasOlder || latest.HasNewer {
		t.Fatalf("unexpected latest metadata: %#v", latest)
	}

	after := getJSON[store.MessagePage](t, base+"?after_seq=3&limit=4")
	expectHTTPSeqs(t, after.Messages, 4, 7)
	if !after.HasOlder || !after.HasNewer {
		t.Fatalf("unexpected after metadata: %#v", after)
	}

	before := getJSON[store.MessagePage](t, base+"?before_seq=8&limit=3")
	expectHTTPSeqs(t, before.Messages, 5, 7)
	if !before.HasOlder || !before.HasNewer {
		t.Fatalf("unexpected before metadata: %#v", before)
	}

	around := getJSON[store.MessagePage](t, base+"?around_seq=6&limit=5")
	expectHTTPSeqs(t, around.Messages, 4, 8)
	if !around.HasOlder || !around.HasNewer {
		t.Fatalf("unexpected around metadata: %#v", around)
	}

	expectStatus(t, http.MethodGet, base+"?before_seq=4&after_seq=7", nil, http.StatusBadRequest)
}

func TestReadEventsArePrivateAcrossWebSocketAndHTTPReplay(t *testing.T) {
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
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	if err := st.AddWorkspaceMember(ctx, workspace.ID, second.ID, "member"); err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx, workspace.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel := channels[0]

	hub := realtime.NewHub()
	server := httptest.NewServer(New(st, hub, Options{UploadDir: filepath.Join(dataDir, "uploads")}).Handler())
	t.Cleanup(server.Close)

	created := postJSON[struct {
		Message store.Message `json:"message"`
		Event   store.Event   `json:"event"`
	}](t, server.URL+"/api/channels/"+channel.ID+"/messages", map[string]string{"body": "mark me read"})
	if created.Message.ChannelSeq == nil {
		t.Fatalf("expected channel seq: %#v", created.Message)
	}

	ownerRead := postJSONAsUser[struct {
		Receipt store.ReadReceipt `json:"receipt"`
	}](t, owner.ID, server.URL+"/api/channels/"+channel.ID+"/read", map[string]int64{"seq": *created.Message.ChannelSeq})
	if ownerRead.Receipt.LastReadSeq != *created.Message.ChannelSeq {
		t.Fatalf("unexpected read receipt: %#v", ownerRead.Receipt)
	}
	ownerReadAgain := postJSONAsUser[struct {
		Receipt store.ReadReceipt `json:"receipt"`
	}](t, owner.ID, server.URL+"/api/channels/"+channel.ID+"/read", map[string]int64{"seq": *created.Message.ChannelSeq})
	if ownerReadAgain.Receipt.LastReadSeq != *created.Message.ChannelSeq {
		t.Fatalf("unexpected idempotent read receipt: %#v", ownerReadAgain.Receipt)
	}
	expectStatus(t, http.MethodPost, server.URL+"/api/channels/"+channel.ID+"/read", strings.NewReader("{"), http.StatusBadRequest)

	ownerEvents := getJSONAsUser[struct {
		Events []store.Event `json:"events"`
	}](t, owner.ID, server.URL+"/api/realtime/events?workspace_id="+url.QueryEscape(workspace.ID)+"&after_cursor="+url.QueryEscape(created.Event.Cursor))
	if len(ownerEvents.Events) != 1 || ownerEvents.Events[0].Type != "channel.read" {
		t.Fatalf("owner should receive own read event, got %#v", ownerEvents.Events)
	}

	secondEvents := getJSONAsUser[struct {
		Events []store.Event `json:"events"`
	}](t, second.ID, server.URL+"/api/realtime/events?workspace_id="+url.QueryEscape(workspace.ID)+"&after_cursor="+url.QueryEscape(created.Event.Cursor))
	for _, event := range secondEvents.Events {
		if event.Type == "channel.read" {
			t.Fatalf("second user received private read event: %#v", secondEvents.Events)
		}
	}

	dm := postJSONAsUser[struct {
		Conversation store.DirectConversation `json:"conversation"`
	}](t, owner.ID, server.URL+"/api/dms", map[string]any{"workspace_id": workspace.ID, "member_ids": []string{second.ID}})
	dmMessage := postJSONAsUser[struct {
		Message store.Message `json:"message"`
		Event   store.Event   `json:"event"`
	}](t, owner.ID, server.URL+"/api/dms/"+dm.Conversation.ID+"/messages", map[string]string{"body": "dm read"})
	if dmMessage.Message.ChannelSeq == nil {
		t.Fatalf("expected dm seq: %#v", dmMessage.Message)
	}
	ownerDMRead := postJSONAsUser[struct {
		Receipt store.ReadReceipt `json:"receipt"`
	}](t, owner.ID, server.URL+"/api/dms/"+dm.Conversation.ID+"/read", map[string]int64{"seq": *dmMessage.Message.ChannelSeq})
	if ownerDMRead.Receipt.LastReadSeq != *dmMessage.Message.ChannelSeq {
		t.Fatalf("unexpected dm read receipt: %#v", ownerDMRead.Receipt)
	}
	ownerDMReadAgain := postJSONAsUser[struct {
		Receipt store.ReadReceipt `json:"receipt"`
	}](t, owner.ID, server.URL+"/api/dms/"+dm.Conversation.ID+"/read", map[string]int64{"seq": *dmMessage.Message.ChannelSeq})
	if ownerDMReadAgain.Receipt.LastReadSeq != *dmMessage.Message.ChannelSeq {
		t.Fatalf("unexpected idempotent dm read receipt: %#v", ownerDMReadAgain.Receipt)
	}
	expectStatus(t, http.MethodPost, server.URL+"/api/dms/"+dm.Conversation.ID+"/read", strings.NewReader("{"), http.StatusBadRequest)
	secondDMEvents := getJSONAsUser[struct {
		Events []store.Event `json:"events"`
	}](t, second.ID, server.URL+"/api/realtime/events?workspace_id="+url.QueryEscape(workspace.ID)+"&after_cursor="+url.QueryEscape(dmMessage.Event.Cursor))
	for _, event := range secondDMEvents.Events {
		if event.Type == "dm.read" {
			t.Fatalf("second user received private dm read event: %#v", secondDMEvents.Events)
		}
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
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	channels, err := st.ListChannels(ctx, workspace.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel := channels[0]
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

	bot, botToken, err := st.CreateBot(context.Background(), store.CreateBotInput{
		WorkspaceID: workspace.ID,
		OwnerUserID: owner.ID,
		DisplayName: "HTTP Bot",
		Handle:      "http-bot",
		Scopes:      []string{"messages:write", "realtime:read", "profile:read"},
		CreatedBy:   owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+botToken.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected bot bearer auth success, got %s %s", resp.Status, string(body))
	}
	resp.Body.Close()
	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/channels/"+channel.ID+"/messages", strings.NewReader(`{"body":"bot hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+botToken.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected bot message success, got %s %s", resp.Status, string(body))
	}
	resp.Body.Close()
	page, err := st.ListMessages(context.Background(), channel.ID, owner.ID, store.MessagePageRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	messages := page.Messages
	if messages[len(messages)-1].AuthorID != bot.ID || messages[len(messages)-1].Author == nil || messages[len(messages)-1].Author.Kind != "bot" {
		t.Fatalf("expected bot-authored message, got %#v", messages[len(messages)-1])
	}
	readOnlyBot, readOnlyToken, err := st.CreateBot(context.Background(), store.CreateBotInput{
		WorkspaceID: workspace.ID,
		DisplayName: "Read Bot",
		Scopes:      []string{"profile:read"},
		CreatedBy:   owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if readOnlyBot.Kind != "bot" {
		t.Fatalf("expected bot kind, got %#v", readOnlyBot)
	}
	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/channels/"+channel.ID+"/messages", strings.NewReader(`{"body":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+readOnlyToken.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected bot scope failure, got %s %s", resp.Status, string(body))
	}
	resp.Body.Close()

	expectStatus(t, http.MethodPatch, server.URL+"/api/me", strings.NewReader("{"), http.StatusBadRequest)
	expectStatus(t, http.MethodPatch, server.URL+"/api/me", strings.NewReader(`{"display_name":"Owner","handle":"x"}`), http.StatusBadRequest)
	expectStatus(t, http.MethodPatch, server.URL+"/api/me", strings.NewReader(`{"display_name":"Owner","avatar_url":"ftp://example.com/a.png"}`), http.StatusBadRequest)
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

func getJSONAsUser[T any](t *testing.T, userID, endpoint string) T {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-ClickClack-User", userID)
	resp, err := http.DefaultClient.Do(req)
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
	out, _ := postJSONWithStatus[T](t, endpoint, body)
	return out
}

func postJSONWithStatus[T any](t *testing.T, endpoint string, body any) (T, int) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: %s %s", endpoint, resp.Status, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out, resp.StatusCode
}

func postJSONAsUser[T any](t *testing.T, userID, endpoint string, body any) T {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ClickClack-User", userID)
	resp, err := http.DefaultClient.Do(req)
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

func uploadFileWithoutPartContentType(t *testing.T, endpoint, workspaceID string) store.Upload {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"workspace_id": workspaceID,
		"width":        "33",
		"height":       "-1",
		"duration_ms":  "bad",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="raw.bin"`)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("raw")); err != nil {
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
		t.Fatalf("raw upload: %s %s", resp.Status, string(body))
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
	expectStatus(t, http.MethodPatch, server.URL+"/api/me", strings.NewReader(`{"display_name":"Nope"}`), http.StatusUnauthorized)
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

func expectHTTPSeqs(t *testing.T, messages []store.Message, first, last int64) {
	t.Helper()
	wantLen := int(last - first + 1)
	if len(messages) != wantLen {
		t.Fatalf("expected %d messages from seq %d to %d, got %d: %#v", wantLen, first, last, len(messages), messages)
	}
	for i, message := range messages {
		want := first + int64(i)
		if message.ChannelSeq == nil || *message.ChannelSeq != want {
			t.Fatalf("message %d: expected seq %d, got %#v", i, want, message.ChannelSeq)
		}
	}
}
