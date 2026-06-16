package sqlite

import (
	"context"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestCreateDirectConversationReusesOneToOneButNotGroups(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Other", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Third", Email: "third@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	for _, userID := range []string{other.ID, third.ID} {
		if err := st.AddWorkspaceMember(ctx, workspace.ID, userID, store.WorkspaceRoleMember); err != nil {
			t.Fatal(err)
		}
	}

	first, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{other.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	reused, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      other.ID,
		MemberIDs:   []string{owner.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != first.ID || reused.RouteID != first.RouteID {
		t.Fatalf("expected canonical one-to-one DM reuse, first=%#v reused=%#v", first, reused)
	}

	groupA, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{other.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{other.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if groupA.ID == groupB.ID {
		t.Fatalf("expected group DMs to remain independently creatable, got %s", groupA.ID)
	}
}

func TestCreateDirectConversationReusesLegacyUnkeyedPair(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Other", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := st.ListWorkspaces(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces[0]
	if err := st.AddWorkspaceMember(ctx, workspace.ID, other.ID, store.WorkspaceRoleMember); err != nil {
		t.Fatal(err)
	}

	const legacyID = "dm_legacy_pair"
	mustExecSQL(t, ctx, st, `INSERT INTO direct_conversations (id, workspace_id, created_at) VALUES (?, ?, '2026-01-01T00:00:00Z')`, legacyID, workspace.ID)
	for _, userID := range []string{owner.ID, other.ID} {
		mustExecSQL(t, ctx, st, `INSERT INTO direct_conversation_members (conversation_id, user_id, created_at) VALUES (?, ?, '2026-01-01T00:00:00Z')`, legacyID, userID)
	}

	reused, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{other.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != legacyID {
		t.Fatalf("expected legacy unkeyed pair reuse, got %#v", reused)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM direct_conversations WHERE workspace_id = ?`, workspace.ID); got != 1 {
		t.Fatalf("expected no duplicate conversation row, got %d", got)
	}
}
