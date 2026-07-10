package httpapi

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/realtime"
	"github.com/openclaw/clickclack/apps/api/internal/store"
	sqlitestore "github.com/openclaw/clickclack/apps/api/internal/store/sqlite"
)

func TestEventTailCursorUsesVisibleEventWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := sqlitestore.Open("sqlite://" + filepath.Join(t.TempDir(), "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "realtime-tail-owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	member, err := st.CreateUser(ctx, store.CreateUserInput{
		DisplayName: "Member",
		Email:       "realtime-tail-member@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	if err := st.AddWorkspaceMember(ctx, workspace.ID, member.ID, "member"); err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx, workspace.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(st, realtime.NewHub(), Options{}).Handler())
	t.Cleanup(server.Close)

	first := postJSONAsUser[struct {
		Message store.Message `json:"message"`
		Event   store.Event   `json:"event"`
	}](t, owner.ID, server.URL+"/api/channels/"+channels[0].ID+"/messages", map[string]string{
		"body": "first",
	})
	postJSONAsUser[struct {
		Receipt store.ReadReceipt `json:"receipt"`
	}](t, owner.ID, server.URL+"/api/channels/"+channels[0].ID+"/read", map[string]int64{
		"seq": *first.Message.ChannelSeq,
	})
	second := postJSONAsUser[struct {
		Message store.Message `json:"message"`
		Event   store.Event   `json:"event"`
	}](t, owner.ID, server.URL+"/api/channels/"+channels[0].ID+"/messages", map[string]string{
		"body": "second",
	})

	result := getJSONAsUser[struct {
		Events     []store.Event `json:"events"`
		TailCursor string        `json:"tail_cursor"`
	}](t, member.ID, server.URL+"/api/realtime/events?workspace_id="+
		url.QueryEscape(workspace.ID)+"&after_cursor="+url.QueryEscape(first.Event.Cursor)+
		"&limit=1&include_tail=true")

	if len(result.Events) != 1 || result.Events[0].ID != second.Event.ID {
		t.Fatalf("hidden read receipt consumed the visible page: %#v", result.Events)
	}
	if result.TailCursor != second.Event.Cursor {
		t.Fatalf("tail cursor = %q, want %q", result.TailCursor, second.Event.Cursor)
	}
}
