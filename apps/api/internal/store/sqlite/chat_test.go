package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestStoreChatThreadsSearchUploadsAndEvents(t *testing.T) {
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

	createdChannel, channelEvent, err := st.CreateChannel(ctx, store.CreateChannelInput{
		WorkspaceID: workspace.ID,
		Name:        "Store Room",
		UserID:      owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if createdChannel.Name != "store-room" || channelEvent.Type != "channel.created" {
		t.Fatalf("unexpected channel create result: %#v %#v", createdChannel, channelEvent)
	}

	root, event, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: channel.ID,
		AuthorID:  owner.ID,
		Body:      "searchable **message**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "message.created" || event.ChannelID != channel.ID {
		t.Fatalf("unexpected message event: %#v", event)
	}
	if root.ChannelSeq == nil || *root.ChannelSeq != 1 {
		t.Fatalf("expected first channel sequence, got %#v", root.ChannelSeq)
	}
	idempotent, idempotentEvent, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: channel.ID,
		AuthorID:  owner.ID,
		Body:      "idempotent body",
		Nonce:     "client-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if idempotent.Nonce != "client-nonce-1" || idempotentEvent.Type != "message.created" {
		t.Fatalf("unexpected idempotent create result: %#v %#v", idempotent, idempotentEvent)
	}
	replayed, replayEvent, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: channel.ID,
		AuthorID:  owner.ID,
		Body:      "idempotent body",
		Nonce:     "client-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != idempotent.ID || replayEvent.ID != "" {
		t.Fatalf("expected idempotent replay without event, got %#v %#v", replayed, replayEvent)
	}
	if _, _, err := st.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: channel.ID,
		AuthorID:  owner.ID,
		Body:      "different body",
		Nonce:     "client-nonce-1",
	}); !errors.Is(err, store.ErrClientNonceConflict) {
		t.Fatalf("expected nonce conflict, got %v", err)
	}

	messages, err := st.ListMessages(ctx, channel.ID, owner.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != root.ID || messages[1].ID != idempotent.ID || messages[1].Nonce != "client-nonce-1" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	after, err := st.ListMessages(ctx, channel.ID, owner.ID, *root.ChannelSeq, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != idempotent.ID {
		t.Fatalf("expected idempotent message after root seq, got %#v", after)
	}

	reply, state, events, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID: root.ID,
		AuthorID:      owner.ID,
		Body:          "reply body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ThreadSeq == nil || *reply.ThreadSeq != 1 || state.ReplyCount != 1 || len(events) != 2 {
		t.Fatalf("unexpected reply result: %#v %#v %#v", reply, state, events)
	}
	threadRoot, replies, threadState, err := st.GetThread(ctx, root.ID, owner.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if threadRoot.ID != root.ID || len(replies) != 1 || threadState.ReplyCount != 1 {
		t.Fatalf("unexpected thread: %#v %#v %#v", threadRoot, replies, threadState)
	}

	results, err := st.SearchMessages(ctx, workspace.ID, owner.ID, "searchable", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Message.ID != root.ID {
		t.Fatalf("unexpected search results: %#v", results)
	}
	eventsAfter, err := st.ListEventsAfter(ctx, workspace.ID, owner.ID, channelEvent.Cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsAfter) == 0 {
		t.Fatal("expected events after channel cursor")
	}
	allEvents, err := st.ListEventsAfter(ctx, workspace.ID, owner.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(allEvents) == 0 {
		t.Fatal("expected events with empty cursor")
	}

	upload, err := st.CreateUpload(ctx, store.CreateUploadInput{
		WorkspaceID: workspace.ID,
		OwnerID:     owner.ID,
		Filename:    "note.txt",
		ContentType: "text/plain",
		ByteSize:    4,
		StoragePath: "/tmp/note.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	gotUpload, err := st.GetUpload(ctx, upload.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotUpload.ID != upload.ID || gotUpload.Filename != "note.txt" {
		t.Fatalf("unexpected upload: %#v", gotUpload)
	}
	if err := st.AttachUpload(ctx, store.AttachUploadInput{MessageID: root.ID, UploadID: upload.ID, UserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	withAttachment, err := st.ListMessages(ctx, channel.ID, owner.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(withAttachment[0].Attachments) != 1 {
		t.Fatalf("expected attachment on message, got %#v", withAttachment[0])
	}

	added, err := st.AddReaction(ctx, store.CreateReactionInput{MessageID: root.ID, UserID: owner.ID, Emoji: "claw"})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := st.RemoveReaction(ctx, store.CreateReactionInput{MessageID: root.ID, UserID: owner.ID, Emoji: "claw"})
	if err != nil {
		t.Fatal(err)
	}
	if added.Type != "reaction.added" || removed.Type != "reaction.removed" {
		t.Fatalf("unexpected reaction events: %#v %#v", added, removed)
	}
}

func TestStoreAccessErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Outsider", Email: "out@example.com"})
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
	root, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channels[0].ID, AuthorID: owner.ID, Body: "private"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := st.CreateUpload(ctx, store.CreateUploadInput{WorkspaceID: workspaces[0].ID, OwnerID: owner.ID, Filename: "x", ContentType: "text/plain", ByteSize: 1, StoragePath: "/tmp/x"})
	if err != nil {
		t.Fatal(err)
	}

	errorCases := []struct {
		name string
		fn   func() error
	}{
		{"list workspaces outsider empty ok", func() error {
			items, err := st.ListWorkspaces(ctx, outsider.ID)
			if err != nil {
				return err
			}
			if len(items) != 0 {
				t.Fatalf("expected no workspaces for outsider, got %#v", items)
			}
			return nil
		}},
		{"get workspace denied", func() error {
			_, err := st.GetWorkspace(ctx, workspaces[0].ID, outsider.ID)
			return err
		}},
		{"list channels denied", func() error {
			_, err := st.ListChannels(ctx, workspaces[0].ID, outsider.ID)
			return err
		}},
		{"list messages denied", func() error {
			_, err := st.ListMessages(ctx, channels[0].ID, outsider.ID, 0, 10)
			return err
		}},
		{"thread denied", func() error {
			_, _, _, err := st.GetThread(ctx, root.ID, outsider.ID, 10)
			return err
		}},
		{"events denied", func() error {
			_, err := st.ListEventsAfter(ctx, workspaces[0].ID, outsider.ID, "", 10)
			return err
		}},
		{"upload denied", func() error {
			_, err := st.GetUpload(ctx, upload.ID, outsider.ID)
			return err
		}},
		{"attach denied", func() error {
			return st.AttachUpload(ctx, store.AttachUploadInput{MessageID: root.ID, UploadID: upload.ID, UserID: outsider.ID})
		}},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if tc.name == "list workspaces outsider empty ok" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestStoreDirectMessagesAndUserLookup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)

	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetUser(ctx, owner.ID); err != nil || got.ID != owner.ID {
		t.Fatalf("unexpected user lookup: %#v err=%v", got, err)
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
	if err := st.AddWorkspaceMember(ctx, workspace.ID, other.ID, "member"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspace.ID, third.ID, "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{"", other.ID, other.ID},
	}); err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{
		WorkspaceID: workspace.ID,
		UserID:      owner.ID,
		MemberIDs:   []string{other.ID, third.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dm.Members) != 3 {
		t.Fatalf("expected three dm members, got %#v", dm.Members)
	}
	list, err := st.ListDirectConversations(ctx, workspace.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected two dm conversations for other member, got %#v", list)
	}
	msg, event, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{
		ConversationID: dm.ID,
		AuthorID:       other.ID,
		Body:           " direct hello ",
		Nonce:          "dm-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.DirectConversationID != dm.ID || event.Type != "message.created" || event.ChannelID != "" {
		t.Fatalf("unexpected direct message result: %#v %#v", msg, event)
	}
	replayedDM, replayedDMEvent, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{
		ConversationID: dm.ID,
		AuthorID:       other.ID,
		Body:           "direct hello",
		Nonce:          "dm-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayedDM.ID != msg.ID || replayedDMEvent.ID != "" {
		t.Fatalf("expected idempotent dm replay without event, got %#v %#v", replayedDM, replayedDMEvent)
	}
	messages, err := st.ListDirectMessages(ctx, dm.ID, third.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "direct hello" {
		t.Fatalf("unexpected direct messages: %#v", messages)
	}
	after, err := st.ListDirectMessages(ctx, dm.ID, third.ID, *messages[0].ChannelSeq, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("expected no direct messages after seq, got %#v", after)
	}

	errorCases := []struct {
		name string
		fn   func() error
	}{
		{"single member", func() error {
			_, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspace.ID, UserID: owner.ID})
			return err
		}},
		{"nonmember create dm", func() error {
			outside, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Outside", Email: "outside@example.com"})
			if err != nil {
				return err
			}
			_, err = st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspace.ID, UserID: owner.ID, MemberIDs: []string{outside.ID}})
			return err
		}},
		{"empty dm body", func() error {
			_, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{ConversationID: dm.ID, AuthorID: owner.ID})
			return err
		}},
		{"missing dm", func() error {
			_, err := st.ListDirectMessages(ctx, "dm_missing", owner.ID, 0, 10)
			return err
		}},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestStoreBranchCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := st.EnsureBootstrap(ctx, "Owner", "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	again, err := st.EnsureBootstrap(ctx, "Ignored", "ignored@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != owner.ID {
		t.Fatalf("expected existing bootstrap user, got %#v", again)
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

	secondWorkspace, err := st.CreateWorkspace(ctx, store.CreateWorkspaceInput{Name: "Other"}, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	defaultChannel, _, err := st.CreateChannel(ctx, store.CreateChannelInput{WorkspaceID: secondWorkspace.ID, UserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	if defaultChannel.Name != "general" || defaultChannel.Kind != "public" {
		t.Fatalf("unexpected default channel: %#v", defaultChannel)
	}
	otherUpload, err := st.CreateUpload(ctx, store.CreateUploadInput{WorkspaceID: secondWorkspace.ID, OwnerID: owner.ID, Filename: "other", ContentType: "text/plain", ByteSize: 1, StoragePath: "/tmp/other"})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "root for branches"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AttachUpload(ctx, store.AttachUploadInput{MessageID: root.ID, UploadID: otherUpload.ID, UserID: owner.ID}); err == nil {
		t.Fatal("expected mismatched upload workspace error")
	}
	reply, _, _, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: root.ID, AuthorID: owner.ID, Body: "reply"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.GetThread(ctx, reply.ID, owner.ID, 10); err == nil {
		t.Fatal("expected reply-as-root error")
	}
	if _, _, _, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: reply.ID, AuthorID: owner.ID, Body: "nested"}); err == nil {
		t.Fatal("expected nested reply error")
	}
	if _, _, _, err := st.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: root.ID, AuthorID: owner.ID}); err == nil {
		t.Fatal("expected empty reply body error")
	}
	if _, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: "chn_missing", AuthorID: owner.ID, Body: "x"}); err == nil {
		t.Fatal("expected missing channel error")
	}
	if results, err := st.SearchMessages(ctx, workspace.ID, owner.ID, "missingterm", 999); err != nil || len(results) != 0 {
		t.Fatalf("expected no search results, got %#v err=%v", results, err)
	}
	if _, err := st.ListEventsAfter(ctx, workspace.ID, owner.ID, "", 999); err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Branch Outsider", Email: "branch-out@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInvite(ctx, workspace.ID, outsider.ID); err == nil {
		t.Fatal("expected invite membership error")
	}
	if _, err := st.SearchMessages(ctx, workspace.ID, outsider.ID, "root", 10); err == nil {
		t.Fatal("expected search membership error")
	}

	firstLink, err := st.CreateMagicLink(ctx, "reuse@example.com", "Reuse One")
	if err != nil {
		t.Fatal(err)
	}
	firstUser, _, err := st.ConsumeMagicLink(ctx, firstLink.Token)
	if err != nil {
		t.Fatal(err)
	}
	secondLink, err := st.CreateMagicLink(ctx, "reuse@example.com", "Reuse Two")
	if err != nil {
		t.Fatal(err)
	}
	secondUser, _, err := st.ConsumeMagicLink(ctx, secondLink.Token)
	if err != nil {
		t.Fatal(err)
	}
	if firstUser.ID != secondUser.ID {
		t.Fatalf("expected reused magic user, got %s and %s", firstUser.ID, secondUser.ID)
	}
	expired, err := st.CreateMagicLink(ctx, "expired@example.com", "Expired")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE auth_magic_links SET expires_at = '2000-01-01T00:00:00Z' WHERE token = ?`, expired.Token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ConsumeMagicLink(ctx, expired.Token); err == nil {
		t.Fatal("expected expired magic link error")
	}
}
