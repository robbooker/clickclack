package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestStoreValidationAndAdminHelpers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

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

	if _, err := st.CreateWorkspace(ctx, store.CreateWorkspaceInput{Name: "ClickClack", Slug: workspace.Slug}, owner.ID); err == nil {
		t.Fatal("expected duplicate workspace slug error")
	}
	if _, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID}); err == nil {
		t.Fatal("expected empty message error")
	}
	if _, _, _, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: channel.ID, AuthorID: owner.ID, Body: "nope"}); err == nil {
		t.Fatal("expected missing root message error")
	}
	if results, err := st.SearchMessages(ctx, workspace.ID, "", owner.ID, "", 10); err != nil || len(results) != 0 {
		t.Fatalf("expected empty search results, got %#v err=%v", results, err)
	}
	if _, err := st.CreateInvite(ctx, workspace.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	link, err := st.CreateMagicLink(ctx, "magic@example.com", "Magic User")
	if err != nil {
		t.Fatal(err)
	}
	magicUser, session, err := st.ConsumeMagicLink(ctx, link.Token)
	if err != nil {
		t.Fatal(err)
	}
	if magicUser.DisplayName != "Magic User" || session.Token == "" {
		t.Fatalf("unexpected magic auth result: %#v %#v", magicUser, session)
	}
	sessionUser, err := st.GetSessionUser(ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if sessionUser.ID != magicUser.ID {
		t.Fatalf("expected session user %s, got %s", magicUser.ID, sessionUser.ID)
	}
	if _, _, err := st.ConsumeMagicLink(ctx, link.Token); err == nil {
		t.Fatal("expected consumed magic link error")
	}
	if _, err := st.CreateMagicLink(ctx, "", "No Email"); err == nil {
		t.Fatal("expected missing email error")
	}
	bot, botToken, err := st.CreateBot(ctx, store.CreateBotInput{
		WorkspaceID: workspace.ID,
		OwnerUserID: owner.ID,
		DisplayName: "Owner Bot",
		Handle:      "owner-bot",
		Scopes:      []string{"messages:write", "realtime:read"},
		CreatedBy:   owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bot.Kind != "bot" || bot.OwnerUserID != owner.ID || botToken.Token == "" {
		t.Fatalf("unexpected bot/token: %#v %#v", bot, botToken)
	}
	botAuth, err := st.GetBotTokenAuth(ctx, botToken.Token)
	if err != nil {
		t.Fatal(err)
	}
	if botAuth.User.ID != bot.ID || botAuth.WorkspaceID != workspace.ID {
		t.Fatalf("unexpected bot auth: %#v", botAuth)
	}
	if _, _, err := st.CreateBot(ctx, store.CreateBotInput{WorkspaceID: workspace.ID, OwnerUserID: bot.ID, DisplayName: "Nested"}); err == nil {
		t.Fatal("expected bot owner rejection")
	}
	if _, _, err := st.CreateBot(ctx, store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Bad Scope", Scopes: []string{"bad:scope"}}); err == nil {
		t.Fatal("expected bad scope rejection")
	}
	if _, _, err := st.CreateBot(ctx, store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Duplicate Handle", Handle: "owner-bot"}); err == nil {
		t.Fatal("expected duplicate bot handle rejection")
	}
	var exported bytes.Buffer
	if err := st.ExportJSON(ctx, &exported); err != nil {
		t.Fatal(err)
	}
	var exportBody map[string][]map[string]any
	if err := json.Unmarshal(exported.Bytes(), &exportBody); err != nil {
		t.Fatal(err)
	}
	if len(exportBody["auth_magic_links"]) == 0 || len(exportBody["sessions"]) == 0 {
		t.Fatalf("expected auth tables in export, got keys %#v", exportBody)
	}
	if len(exportBody["bot_tokens"]) == 0 {
		t.Fatalf("expected bot_tokens in export, got keys %#v", exportBody)
	}
	if err := st.Backup(ctx, filepath.Join(t.TempDir(), "backup.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE sessions`); err != nil {
		t.Fatal(err)
	}
	if err := st.ExportJSON(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected export failure")
	}

	second, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Second", Email: "second@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspace.ID, UserID: owner.ID, MemberIDs: []string{second.ID}}); err == nil {
		t.Fatal("expected dm membership error for second user")
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, second.ID, "member"); err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspace.ID, UserID: owner.ID, MemberIDs: []string{second.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dms, err := st.ListDirectConversations(ctx, workspace.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dms) != 1 || dms[0].ID != dm.ID {
		t.Fatalf("unexpected dm list: %#v", dms)
	}
	if _, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{ConversationID: dm.ID, AuthorID: second.ID}); err == nil {
		t.Fatal("expected empty dm message error")
	}
}

func TestBotStoreValidationAndAuthEdges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	outsider, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Outsider", Email: "outsider@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	createFailures := []struct {
		name  string
		input store.CreateBotInput
	}{
		{"missing workspace", store.CreateBotInput{DisplayName: "Missing Workspace"}},
		{"missing display name", store.CreateBotInput{WorkspaceID: workspace.ID}},
		{"display name too long", store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: strings.Repeat("a", 81)}},
		{"invalid handle", store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Bad Handle", Handle: "!"}},
		{"invalid avatar", store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Bad Avatar", AvatarURL: "ftp://example.com/avatar.png"}},
		{"missing owner", store.CreateBotInput{WorkspaceID: workspace.ID, OwnerUserID: "usr_missing", DisplayName: "Missing Owner"}},
		{"owner not member", store.CreateBotInput{WorkspaceID: workspace.ID, OwnerUserID: outsider.ID, DisplayName: "Outsider Bot"}},
		{"unknown scope", store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Unknown Scope", Scopes: []string{"bad:scope"}}},
	}
	for _, tc := range createFailures {
		t.Run("create_"+tc.name, func(t *testing.T) {
			if _, _, err := st.CreateBot(ctx, tc.input); err == nil {
				t.Fatal("expected CreateBot error")
			}
		})
	}

	bot, token, err := st.CreateBot(ctx, store.CreateBotInput{
		WorkspaceID: workspace.ID,
		OwnerUserID: owner.ID,
		DisplayName: "Default Scope Bot",
		TokenName:   " ",
		CreatedBy:   owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.Name != "default" || len(token.Scopes) == 0 {
		t.Fatalf("expected default token metadata, got %#v", token)
	}
	auth, err := st.GetBotTokenAuth(ctx, " "+token.Token+" ")
	if err != nil {
		t.Fatal(err)
	}
	if auth.User.ID != bot.ID || auth.TokenID != token.ID || auth.WorkspaceID != workspace.ID {
		t.Fatalf("unexpected bot token auth: %#v", auth)
	}

	if _, err := st.GetBotTokenAuth(ctx, "not-a-bot-token"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected non-bot token miss, got %v", err)
	}
	if _, err := st.GetBotTokenAuth(ctx, "ccb_missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing bot token miss, got %v", err)
	}

	_, revokedToken, err := st.CreateBot(ctx, store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Revoked Bot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE bot_tokens SET revoked_at = ? WHERE id = ?`, now(), revokedToken.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBotTokenAuth(ctx, revokedToken.Token); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected revoked token miss, got %v", err)
	}

	_, malformedToken, err := st.CreateBot(ctx, store.CreateBotInput{WorkspaceID: workspace.ID, DisplayName: "Malformed Scope Bot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE bot_tokens SET scopes_json = ? WHERE id = ?`, `{`, malformedToken.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBotTokenAuth(ctx, malformedToken.Token); err == nil {
		t.Fatal("expected malformed scope JSON error")
	}

	ownerMembershipBot, ownerMembershipToken, err := st.CreateBot(ctx, store.CreateBotInput{
		WorkspaceID: workspace.ID,
		OwnerUserID: owner.ID,
		DisplayName: "Owner Membership Bot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspace.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBotTokenAuth(ctx, ownerMembershipToken.Token); err == nil {
		t.Fatal("expected missing bot owner membership error")
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, owner.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspace.ID, ownerMembershipBot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBotTokenAuth(ctx, ownerMembershipToken.Token); err == nil {
		t.Fatal("expected missing bot membership error")
	}
}

func TestOpenRejectsBadDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open("sqlite://" + filepath.Join(path, "db.sqlite")); err == nil {
		t.Fatal("expected bad directory error")
	}
}

func TestStoreClosedDatabaseErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.db.Close(); err != nil {
		t.Fatal(err)
	}

	errorCases := []struct {
		name string
		fn   func() error
	}{
		{"migrate", func() error { return st.Migrate(ctx) }},
		{"create user", func() error {
			_, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "x"})
			return err
		}},
		{"first user", func() error {
			_, err := st.FirstUser(ctx)
			return err
		}},
		{"get user", func() error {
			_, err := st.GetUser(ctx, "usr_missing")
			return err
		}},
		{"list workspaces", func() error {
			_, err := st.ListWorkspaces(ctx, "usr_missing")
			return err
		}},
		{"create workspace", func() error {
			_, err := st.CreateWorkspace(ctx, store.CreateWorkspaceInput{Name: "x"}, "usr_missing")
			return err
		}},
		{"create channel", func() error {
			_, _, err := st.CreateChannel(ctx, store.CreateChannelInput{})
			return err
		}},
		{"create message", func() error {
			_, _, err := st.CreateMessage(ctx, store.CreateMessageInput{})
			return err
		}},
		{"create reply", func() error {
			_, _, _, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{})
			return err
		}},
		{"add reaction", func() error {
			_, err := st.AddReaction(ctx, store.CreateReactionInput{})
			return err
		}},
		{"remove reaction", func() error {
			_, err := st.RemoveReaction(ctx, store.CreateReactionInput{})
			return err
		}},
		{"create upload", func() error {
			_, err := st.CreateUpload(ctx, store.CreateUploadInput{})
			return err
		}},
		{"attach upload", func() error {
			return st.AttachUpload(ctx, store.AttachUploadInput{})
		}},
		{"create dm", func() error {
			_, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{})
			return err
		}},
		{"create dm message", func() error {
			_, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{})
			return err
		}},
		{"magic link", func() error {
			_, err := st.CreateMagicLink(ctx, "x@example.com", "x")
			return err
		}},
		{"identity", func() error {
			_, err := st.UpsertIdentityUser(ctx, store.UpsertIdentityUserInput{Provider: "github", ProviderSubject: "1"})
			return err
		}},
		{"session", func() error {
			_, err := st.CreateSession(ctx, "usr_missing")
			return err
		}},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected closed database error")
			}
		})
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open("sqlite://" + filepath.Join(t.TempDir(), "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st
}
