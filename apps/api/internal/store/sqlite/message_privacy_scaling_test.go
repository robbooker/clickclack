package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestMessagePrivacyScalingPrivacyAndDMThreads(t *testing.T) {
	t.Parallel()
	ctx, st, owner, workspace, channel := seededStore(t)

	member, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "DM Member", Email: "dm-member@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceOnly, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Workspace Only", Email: "workspace-only-hardening@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, member.ID, "member"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, workspaceOnly.ID, "member"); err != nil {
		t.Fatal(err)
	}

	publicMessage, _, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: channel.ID,
		AuthorID:  owner.ID,
		Body:      "public searchable channel text",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherChannel, _, err := st.CreateChannel(ctx, store.CreateChannelInput{WorkspaceID: workspace.ID, UserID: owner.ID, Name: "other-hardening"})
	if err != nil {
		t.Fatal(err)
	}
	otherChannelMessage, _, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: otherChannel.ID,
		AuthorID:  owner.ID,
		Body:      "public searchable other channel text",
	})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspace.ID, UserID: owner.ID, MemberIDs: []string{member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dmMessage, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{
		ConversationID: dm.ID,
		AuthorID:       owner.ID,
		Body:           "private searchable dm secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := st.SearchMessages(ctx, workspace.ID, "", owner.ID, "searchable", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected workspace search to exclude DMs, got %#v", results)
	}
	scopedResults, err := st.SearchMessages(ctx, workspace.ID, channel.ID, owner.ID, "searchable", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(scopedResults) != 1 || scopedResults[0].Message.ID != publicMessage.ID {
		t.Fatalf("expected channel search to stay in channel, got %#v; other=%s", scopedResults, otherChannelMessage.ID)
	}
	dmOnlyResults, err := st.SearchMessages(ctx, workspace.ID, "", owner.ID, "secret", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dmOnlyResults) != 0 {
		t.Fatalf("expected DM-only search term to be hidden from workspace search, got %#v", dmOnlyResults)
	}

	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: dmMessage.ID, UserID: workspaceOnly.ID, Body: "blocked"}); err == nil {
		t.Fatal("expected non-DM member to be blocked from updating a DM message")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: dmMessage.ID, UserID: workspaceOnly.ID}); err == nil {
		t.Fatal("expected non-DM member to be blocked from deleting a DM message")
	}
	if _, err := st.AddReaction(ctx, store.CreateReactionInput{MessageID: dmMessage.ID, UserID: workspaceOnly.ID, Emoji: "nope"}); err == nil {
		t.Fatal("expected non-DM member to be blocked from reacting to a DM message")
	}
	upload, err := st.CreateUpload(ctx, store.CreateUploadInput{WorkspaceID: workspace.ID, OwnerID: owner.ID, Filename: "dm.txt", ContentType: "text/plain", ByteSize: 1, StoragePath: "/tmp/dm.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUpload(ctx, store.AttachUploadInput{MessageID: dmMessage.ID, UploadID: upload.ID, UserID: workspaceOnly.ID}); err == nil {
		t.Fatal("expected non-DM member to be blocked from attaching to a DM message")
	}

	reply, state, events, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID: dmMessage.ID,
		AuthorID:      member.ID,
		Body:          "dm thread reply secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.DirectConversationID != dm.ID || reply.ChannelID != "" || state.ReplyCount != 1 || len(events) != 2 {
		t.Fatalf("unexpected DM thread reply result: %#v %#v %#v", reply, state, events)
	}
	page, err := st.ListDirectMessages(ctx, dm.ID, owner.ID, store.MessagePageRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != dmMessage.ID || page.NewestSeq != *dmMessage.ChannelSeq {
		t.Fatalf("expected DM root timeline to exclude thread replies, got %#v", page)
	}
	receipt, readEvent, err := st.MarkDirectRead(ctx, dm.ID, owner.ID, 999)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.LastReadSeq != *dmMessage.ChannelSeq || len(readEvent.RecipientUserIDs) != 1 || readEvent.RecipientUserIDs[0] != owner.ID {
		t.Fatalf("expected DM read to cap to root sequence and target the reader, got %#v %#v", receipt, readEvent)
	}
	_, replies, _, err := st.GetThread(ctx, dmMessage.ID, owner.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 || replies[0].ID != reply.ID || replies[0].DirectConversationID != dm.ID {
		t.Fatalf("unexpected DM thread replies: %#v", replies)
	}
	if _, _, _, err := st.GetThread(ctx, dmMessage.ID, workspaceOnly.ID, 10); err == nil {
		t.Fatal("expected non-DM member to be blocked from DM thread")
	}

	ownerEvents, err := st.ListEventsAfter(ctx, workspace.ID, owner.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	memberEvents, err := st.ListEventsAfter(ctx, workspace.ID, member.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	workspaceOnlyEvents, err := st.ListEventsAfter(ctx, workspace.ID, workspaceOnly.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, events := range [][]store.Event{ownerEvents, memberEvents} {
		if !hasEventForMessage(events, "message.created", dmMessage.ID) {
			t.Fatalf("expected DM member event replay to include DM message event: %#v", events)
		}
		if !hasEventForMessage(events, "thread.reply_created", reply.ID) {
			t.Fatalf("expected DM member event replay to include DM thread event: %#v", events)
		}
	}
	if hasEventForMessage(workspaceOnlyEvents, "message.created", dmMessage.ID) || hasEventForMessage(workspaceOnlyEvents, "thread.reply_created", reply.ID) {
		t.Fatalf("expected workspace-only member replay to hide DM events, got %#v", workspaceOnlyEvents)
	}
	for _, event := range workspaceOnlyEvents {
		if event.Type == "dm.read" || event.Type == "channel.read" {
			t.Fatalf("expected private read receipts to be filtered at replay query, got %#v", workspaceOnlyEvents)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM event_recipients WHERE event_id IN (SELECT id FROM events WHERE type = 'message.created' AND json_extract(payload_json, '$.direct_conversation_id') = ?)`, dm.ID); err != nil {
		t.Fatal(err)
	}
	workspaceOnlyEvents, err = st.ListEventsAfter(ctx, workspace.ID, workspaceOnly.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if hasEventForMessage(workspaceOnlyEvents, "message.created", dmMessage.ID) {
		t.Fatalf("expected private event flag to hide DM event even after recipient rows are removed, got %#v", workspaceOnlyEvents)
	}
}

func TestMessagePrivacyScalingMessageShapeAndSequenceGuards(t *testing.T) {
	t.Parallel()
	ctx, st, owner, workspace, channel := seededStore(t)
	root, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at)
		VALUES ('msg_invalid_shape', ?, ?, 'dm_fake', ?, NULL, 'msg_invalid_shape', 999, NULL, 'bad', 'markdown', ?)`,
		workspace.ID, channel.ID, owner.ID, now()); err == nil {
		t.Fatal("expected invalid message shape to be rejected")
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at)
		VALUES ('msg_duplicate_seq', ?, ?, NULL, ?, NULL, 'msg_duplicate_seq', ?, NULL, 'dupe', 'markdown', ?)`,
		workspace.ID, channel.ID, owner.ID, *root.ChannelSeq, now()); err == nil {
		t.Fatal("expected duplicate channel sequence to be rejected")
	}
	otherRoot, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "other root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at)
		VALUES ('msg_mismatched_thread', ?, ?, NULL, ?, ?, ?, NULL, 1, 'bad reply', 'markdown', ?)`,
		workspace.ID, channel.ID, owner.ID, root.ID, otherRoot.ID, now()); err == nil {
		t.Fatal("expected mismatched parent and thread root to be rejected")
	}
	otherChannel, _, err := st.CreateChannel(ctx, store.CreateChannelInput{WorkspaceID: workspace.ID, UserID: owner.ID, Name: "wrong-thread-surface"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at)
		VALUES ('msg_wrong_channel_thread', ?, ?, NULL, ?, ?, ?, NULL, 1, 'bad reply', 'markdown', ?)`,
		workspace.ID, otherChannel.ID, owner.ID, root.ID, root.ID, now()); err == nil {
		t.Fatal("expected reply on the wrong channel surface to be rejected")
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at)
		VALUES ('msg_valid_shape_reply', ?, ?, NULL, ?, ?, ?, NULL, 1, 'valid reply', 'markdown', ?)`,
		workspace.ID, channel.ID, owner.ID, root.ID, root.ID, now()); err != nil {
		t.Fatalf("expected valid root reply shape to be accepted: %v", err)
	}
}

func TestMessagePrivacyScalingDirectMemberHydrationBatchesLargeInputs(t *testing.T) {
	t.Parallel()
	ctx, st, _, _, _ := seededStore(t)
	ids := make([]string, directConversationMemberHydrationBatchSize+25)
	for i := range ids {
		ids[i] = "dm_missing_" + strings.Repeat("x", i%8)
	}
	members, err := st.directConversationMembersByConversationIDs(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("expected no members for missing conversations, got %#v", members)
	}
}

func TestMessagePrivacyScalingMigrationBackfillsCurrentMainDMThreadPrivacy(t *testing.T) {
	ctx := context.Background()
	st, err := Open("sqlite://" + filepath.Join(t.TempDir(), "clickclack.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	applySQLiteMigrations(t, ctx, st, "0001_initial.sql", "0002_auth.sql", "0003_user_profiles.sql", "0004_message_quotes.sql", "0005_upload_dimensions.sql", "0006_message_client_nonce.sql", "0007_unread.sql", "0008_message_paging_indexes.sql", "0008_user_notification_settings.sql", "0009_bots.sql")

	const (
		workspaceID     = "wsp_migration"
		ownerID         = "usr_owner"
		memberID        = "usr_member"
		workspaceOnlyID = "usr_workspace_only"
		dmID            = "dm_private"
		rootID          = "msg_dm_root"
		replyID         = "msg_dm_reply"
	)
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	mustExecSQL(t, ctx, st, `INSERT INTO users (id, display_name, avatar_url, created_at) VALUES (?, 'Owner', '', '2026-01-01T00:00:00Z')`, ownerID)
	mustExecSQL(t, ctx, st, `INSERT INTO users (id, display_name, avatar_url, created_at) VALUES (?, 'Member', '', '2026-01-01T00:00:00Z')`, memberID)
	mustExecSQL(t, ctx, st, `INSERT INTO users (id, display_name, avatar_url, created_at) VALUES (?, 'Workspace Only', '', '2026-01-01T00:00:00Z')`, workspaceOnlyID)
	mustExecSQL(t, ctx, st, `INSERT INTO workspaces (id, name, slug, created_at) VALUES (?, 'Workspace', 'workspace', '2026-01-01T00:00:00Z')`, workspaceID)
	mustExecSQL(t, ctx, st, `INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, 'owner', '2026-01-01T00:00:00Z')`, workspaceID, ownerID)
	mustExecSQL(t, ctx, st, `INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, 'member', '2026-01-01T00:00:00Z')`, workspaceID, memberID)
	mustExecSQL(t, ctx, st, `INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, 'member', '2026-01-01T00:00:00Z')`, workspaceID, workspaceOnlyID)
	mustExecSQL(t, ctx, st, `INSERT INTO direct_conversations (id, workspace_id, created_at) VALUES (?, ?, '2026-01-01T00:00:00Z')`, dmID, workspaceID)
	mustExecSQL(t, ctx, st, `INSERT INTO direct_conversation_members (conversation_id, user_id, created_at) VALUES (?, ?, '2026-01-01T00:00:00Z')`, dmID, ownerID)
	mustExecSQL(t, ctx, st, `INSERT INTO direct_conversation_members (conversation_id, user_id, created_at) VALUES (?, ?, '2026-01-01T00:00:00Z')`, dmID, memberID)
	mustExecSQL(t, ctx, st, `INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, 1, NULL, 'private root', 'markdown', '2026-01-01T00:00:01Z')`, rootID, workspaceID, dmID, ownerID, rootID)
	mustExecSQL(t, ctx, st, `INSERT INTO thread_state (root_message_id, reply_count, last_reply_at, last_reply_author_ids_json) VALUES (?, 1, '2026-01-01T00:00:02Z', '["usr_member"]')`, rootID)
	mustExecSQL(t, ctx, st, `INSERT INTO messages (id, workspace_id, channel_id, direct_conversation_id, author_id, parent_message_id, thread_root_id, channel_seq, thread_seq, body, body_format, created_at) VALUES (?, ?, '', NULL, ?, ?, ?, NULL, 1, 'private reply', 'markdown', '2026-01-01T00:00:02Z')`, replyID, workspaceID, memberID, rootID, rootID)
	mustExecSQL(t, ctx, st, `INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at) VALUES ('evt_dm_message', 'cur_001', ?, NULL, 'message.created', 1, '{"message_id":"msg_dm_root","direct_conversation_id":"dm_private","author_id":"usr_owner"}', '2026-01-01T00:00:01Z')`, workspaceID)
	mustExecSQL(t, ctx, st, `INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at) VALUES ('evt_thread_reply', 'cur_002', ?, NULL, 'thread.reply_created', NULL, '{"message_id":"msg_dm_reply","root_message_id":"msg_dm_root"}', '2026-01-01T00:00:02Z')`, workspaceID)
	mustExecSQL(t, ctx, st, `INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at) VALUES ('evt_thread_state', 'cur_003', ?, NULL, 'thread.state_updated', NULL, '{"root_message_id":"msg_dm_root"}', '2026-01-01T00:00:02Z')`, workspaceID)
	mustExecSQL(t, ctx, st, `INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at) VALUES ('evt_reply_reaction', 'cur_004', ?, NULL, 'reaction.added', NULL, '{"message_id":"msg_dm_reply","emoji":":ok:"}', '2026-01-01T00:00:03Z')`, workspaceID)
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	applySQLiteMigrations(t, ctx, st, "0010_message_privacy_scaling.sql")

	var replyDM string
	var replyChannel any
	if err := st.db.QueryRowContext(ctx, `SELECT direct_conversation_id, channel_id FROM messages WHERE id = ?`, replyID).Scan(&replyDM, &replyChannel); err != nil {
		t.Fatal(err)
	}
	if replyDM != dmID || replyChannel != nil {
		t.Fatalf("expected migration to attach old DM reply to DM surface, got dm=%q channel=%#v", replyDM, replyChannel)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE id IN ('evt_dm_message', 'evt_thread_reply', 'evt_thread_state', 'evt_reply_reaction') AND is_private = 1`); got != 4 {
		t.Fatalf("expected all historical DM events to be private, got %d", got)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM event_recipients WHERE event_id IN ('evt_thread_reply', 'evt_thread_state', 'evt_reply_reaction')`); got != 6 {
		t.Fatalf("expected historical DM thread/reaction events to be recipient-scoped, got %d", got)
	}
	workspaceOnlyEvents, err := st.ListEventsAfter(ctx, workspaceID, workspaceOnlyID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceOnlyEvents) != 0 {
		t.Fatalf("expected workspace-only member to see no private historical DM events, got %#v", workspaceOnlyEvents)
	}
}

func TestMessagePrivacyScalingHotPathQueryPlans(t *testing.T) {
	t.Parallel()
	ctx, st, _, _, _ := seededStore(t)
	for _, tc := range []struct {
		name      string
		query     string
		args      []any
		wantIndex string
	}{
		{
			name:      "channel page",
			query:     `EXPLAIN QUERY PLAN SELECT m.id FROM messages m WHERE m.channel_id = ? AND m.parent_message_id IS NULL ORDER BY m.channel_seq DESC LIMIT ?`,
			args:      []any{"chn_x", 100},
			wantIndex: "idx_messages_channel_root_page",
		},
		{
			name:      "dm page",
			query:     `EXPLAIN QUERY PLAN SELECT m.id FROM messages m WHERE m.direct_conversation_id = ? AND m.parent_message_id IS NULL ORDER BY m.channel_seq DESC LIMIT ?`,
			args:      []any{"dm_x", 100},
			wantIndex: "idx_messages_direct_page",
		},
		{
			name:      "dm sidebar membership",
			query:     `EXPLAIN QUERY PLAN SELECT dc.id FROM direct_conversation_members dcm JOIN direct_conversations dc ON dc.id = dcm.conversation_id WHERE dcm.user_id = ? ORDER BY dc.created_at`,
			args:      []any{"usr_x"},
			wantIndex: "idx_direct_conversation_members_user",
		},
		{
			name:      "attachment hydration",
			query:     `EXPLAIN QUERY PLAN SELECT u.id FROM message_attachments ma JOIN uploads u ON u.id = ma.upload_id WHERE ma.message_id = ? ORDER BY ma.created_at`,
			args:      []any{"msg_x"},
			wantIndex: "idx_message_attachments_message_created",
		},
		{
			name: "event replay",
			query: `EXPLAIN QUERY PLAN
					SELECT e.id
					FROM events e
					WHERE e.workspace_id = ? AND e.cursor > ?
					  AND (
					    e.is_private = 0
					    OR EXISTS (SELECT 1 FROM event_recipients er WHERE er.event_id = e.id AND er.user_id = ?)
					  )
					ORDER BY e.cursor
				LIMIT ?`,
			args:      []any{"wsp_x", "cur_x", "usr_x", 200},
			wantIndex: "idx_events_workspace_cursor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, ctx, st, tc.query, tc.args...)
			if !strings.Contains(plan, tc.wantIndex) {
				t.Fatalf("expected query plan to use %s, got:\n%s", tc.wantIndex, plan)
			}
		})
	}
}

func TestMessagePrivacyScalingEventPruning(t *testing.T) {
	t.Parallel()
	ctx, st, owner, workspace, channel := seededStore(t)
	message, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "event prune message"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.MarkChannelRead(ctx, channel.ID, owner.ID, *message.ChannelSeq); err != nil {
		t.Fatal(err)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM event_recipients`); got == 0 {
		t.Fatal("expected private read event recipients before pruning")
	}
	if _, err := st.PruneEvents(ctx, workspace.ID, 0, ""); err == nil {
		t.Fatal("expected prune without retention bounds to be rejected")
	}
	if _, err := st.PruneEvents(ctx, "", 1, ""); err == nil {
		t.Fatal("expected prune without workspace to be rejected")
	}
	if _, err := st.PruneEvents(ctx, workspace.ID, -1, ""); err == nil {
		t.Fatal("expected prune with negative keep_latest to be rejected")
	}
	if _, err := st.PruneEvents(ctx, workspace.ID, 0, "not-a-time"); err == nil {
		t.Fatal("expected prune with invalid cutoff to be rejected")
	}
	pruned, err := st.PruneEvents(ctx, workspace.ID, 0, "9999-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if pruned == 0 {
		t.Fatal("expected pruning to delete events")
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE workspace_id = ?`, workspace.ID); got != 0 {
		t.Fatalf("expected all workspace events to be pruned, got %d", got)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM event_recipients`); got != 0 {
		t.Fatalf("expected event recipient rows to cascade on prune, got %d", got)
	}

	for i := 0; i < 3; i++ {
		if _, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "event keep latest"}); err != nil {
			t.Fatal(err)
		}
	}
	pruned, err = st.PruneEvents(ctx, workspace.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 2 {
		t.Fatalf("expected keep_latest prune to delete 2 events, got %d", pruned)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE workspace_id = ?`, workspace.ID); got != 1 {
		t.Fatalf("expected latest event to be retained, got %d", got)
	}
}

func TestMessagePrivacyScalingEventPruningUsesTimestampCutoff(t *testing.T) {
	t.Parallel()
	ctx, st, _, workspace, _ := seededStore(t)
	mustExecSQL(t, ctx, st, `DELETE FROM event_recipients`)
	mustExecSQL(t, ctx, st, `DELETE FROM events WHERE workspace_id = ?`, workspace.ID)
	mustExecSQL(t, ctx, st, `
		INSERT INTO events (id, cursor, workspace_id, channel_id, type, seq, payload_json, created_at, is_private)
		VALUES
		  ('evt_before_cutoff', 'cur_prune_001', ?, NULL, 'test.before', NULL, '{}', '2025-12-31T23:59:59.999999999Z', 0),
		  ('evt_at_cutoff', 'cur_prune_002', ?, NULL, 'test.equal', NULL, '{}', '2026-01-01T00:00:00Z', 0),
		  ('evt_after_fractional_cutoff', 'cur_prune_003', ?, NULL, 'test.after', NULL, '{}', '2026-01-01T00:00:00.5Z', 0)
	`, workspace.ID, workspace.ID, workspace.ID)

	pruned, err := st.PruneEvents(ctx, workspace.ID, 0, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected only event before cutoff to be pruned, got %d", pruned)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE id = 'evt_before_cutoff'`); got != 0 {
		t.Fatalf("expected event before cutoff to be deleted, got %d", got)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE id = 'evt_at_cutoff'`); got != 1 {
		t.Fatalf("expected event exactly at cutoff to be retained, got %d", got)
	}
	if got := scalarCount(t, ctx, st, `SELECT COUNT(*) FROM events WHERE id = 'evt_after_fractional_cutoff'`); got != 1 {
		t.Fatalf("expected fractional event after cutoff to be retained, got %d", got)
	}
}

func hasEventForMessage(events []store.Event, eventType, messageID string) bool {
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		switch payload := event.Payload.(type) {
		case map[string]string:
			if payload["message_id"] == messageID {
				return true
			}
		case map[string]any:
			if got, _ := payload["message_id"].(string); got == messageID {
				return true
			}
		}
	}
	return false
}

func scalarCount(t *testing.T, ctx context.Context, st *Store, query string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := st.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func explainQueryPlan(t *testing.T, ctx context.Context, st *Store, query string, args ...any) string {
	t.Helper()
	rows, err := st.db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(details, "\n")
}

func applySQLiteMigrations(t *testing.T, ctx context.Context, st *Store, names ...string) {
	t.Helper()
	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func mustExecSQL(t *testing.T, ctx context.Context, st *Store, query string, args ...any) {
	t.Helper()
	if _, err := st.db.ExecContext(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}
