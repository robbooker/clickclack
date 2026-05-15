package sqlite

import (
	"context"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

func TestMutationsCreateDurableEvents(t *testing.T) {
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
	channels, err := st.ListChannels(ctx, workspaces[0].ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channel := channels[0]

	archived := true
	updatedChannel, channelEvent, err := st.UpdateChannel(ctx, store.UpdateChannelInput{ChannelID: channel.ID, UserID: owner.ID, Name: "harbor", Archived: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if updatedChannel.Name != "harbor" || updatedChannel.ArchivedAt == nil || channelEvent.Type != "channel.updated" {
		t.Fatalf("unexpected channel update: %#v %#v", updatedChannel, channelEvent)
	}
	archived = false
	updatedChannel, _, err = st.UpdateChannel(ctx, store.UpdateChannelInput{ChannelID: channel.ID, UserID: owner.ID, Kind: "private", Archived: &archived})
	if err != nil {
		t.Fatal(err)
	}
	if updatedChannel.Kind != "private" || updatedChannel.ArchivedAt != nil {
		t.Fatalf("unexpected channel unarchive: %#v", updatedChannel)
	}

	message, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channel.ID, AuthorID: owner.ID, Body: "before"})
	if err != nil {
		t.Fatal(err)
	}
	updatedMessage, updateEvent, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: owner.ID, Body: "after"})
	if err != nil {
		t.Fatal(err)
	}
	if updatedMessage.Body != "after" || updatedMessage.EditedAt == nil || updateEvent.Type != "message.updated" {
		t.Fatalf("unexpected message update: %#v %#v", updatedMessage, updateEvent)
	}
	deletedMessage, deleteEvent, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: message.ID, UserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	if deletedMessage.DeletedAt == nil || deleteEvent.Type != "message.deleted" {
		t.Fatalf("unexpected message delete: %#v %#v", deletedMessage, deleteEvent)
	}
	second, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Second", Email: "second@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspaces[0].ID, second.ID, "member"); err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: workspaces[0].ID, UserID: owner.ID, MemberIDs: []string{second.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dmMessage, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{ConversationID: dm.ID, AuthorID: owner.ID, Body: "dm before"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: dmMessage.ID, UserID: second.ID, Body: "dm after"}); err == nil {
		t.Fatal("expected non-author DM member update to be rejected")
	}
	updatedDM, dmEvent, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: dmMessage.ID, UserID: owner.ID, Body: "dm after"})
	if err != nil {
		t.Fatal(err)
	}
	if updatedDM.DirectConversationID != dm.ID || dmEvent.ChannelID != "" {
		t.Fatalf("unexpected dm update: %#v %#v", updatedDM, dmEvent)
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: dmMessage.ID, UserID: second.ID}); err == nil {
		t.Fatal("expected non-author DM member delete to be rejected")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: dmMessage.ID, UserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListEventsAfter(ctx, workspaces[0].ID, owner.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, eventType := range []string{"channel.updated", "message.updated", "message.deleted"} {
		if !seen[eventType] {
			t.Fatalf("missing event %s in %#v", eventType, events)
		}
	}
}

func TestMutationsRejectInvalidInput(t *testing.T) {
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
	channels, err := st.ListChannels(ctx, workspaces[0].ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channels[0].ID, AuthorID: owner.ID, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: owner.ID, Body: " "}); err == nil {
		t.Fatal("expected empty message update error")
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: "missing", UserID: owner.ID, Body: "x"}); err == nil {
		t.Fatal("expected missing message update error")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: "missing", UserID: owner.ID}); err == nil {
		t.Fatal("expected missing message delete error")
	}
	if _, _, err := st.UpdateChannel(ctx, store.UpdateChannelInput{ChannelID: "missing", UserID: owner.ID}); err == nil {
		t.Fatal("expected missing channel error")
	}
	outsider, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Outsider", Email: "outsider@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateChannel(ctx, store.UpdateChannelInput{ChannelID: channels[0].ID, UserID: outsider.ID, Name: "nope"}); err == nil {
		t.Fatal("expected outsider channel update error")
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: outsider.ID, Body: "nope"}); err == nil {
		t.Fatal("expected outsider message update error")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: message.ID, UserID: outsider.ID}); err == nil {
		t.Fatal("expected outsider message delete error")
	}
	member, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Member", Email: "member-mutations@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, workspaces[0].ID, member.ID, "member"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: member.ID, Body: "nope"}); err == nil {
		t.Fatal("expected non-author member message update error")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: message.ID, UserID: member.ID}); err == nil {
		t.Fatal("expected non-author member message delete error")
	}
	reactionMessage, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channels[0].ID, AuthorID: owner.ID, Body: "react"})
	if err != nil {
		t.Fatal(err)
	}
	firstReaction, err := st.AddReaction(ctx, store.CreateReactionInput{MessageID: reactionMessage.ID, UserID: owner.ID, Emoji: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if firstReaction.ID == "" || firstReaction.Type != "reaction.added" {
		t.Fatalf("unexpected first reaction event: %#v", firstReaction)
	}
	duplicateReaction, err := st.AddReaction(ctx, store.CreateReactionInput{MessageID: reactionMessage.ID, UserID: owner.ID, Emoji: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if duplicateReaction.ID != "" {
		t.Fatalf("duplicate reaction emitted event: %#v", duplicateReaction)
	}
	removedReaction, err := st.RemoveReaction(ctx, store.CreateReactionInput{MessageID: reactionMessage.ID, UserID: owner.ID, Emoji: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if removedReaction.ID == "" || removedReaction.Type != "reaction.removed" {
		t.Fatalf("unexpected remove reaction event: %#v", removedReaction)
	}
	missingReaction, err := st.RemoveReaction(ctx, store.CreateReactionInput{MessageID: reactionMessage.ID, UserID: owner.ID, Emoji: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if missingReaction.ID != "" {
		t.Fatalf("missing reaction emitted event: %#v", missingReaction)
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: message.ID, UserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: owner.ID, Body: "deleted body returns"}); err == nil {
		t.Fatal("expected deleted message update error")
	}
	results, err := st.SearchMessages(ctx, workspaces[0].ID, channels[0].ID, owner.ID, "deleted body returns", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted message was searchable: %#v", results)
	}
}

func TestMutationsReturnOutboxErrors(t *testing.T) {
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
	channels, err := st.ListChannels(ctx, workspaces[0].ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channels[0].ID, AuthorID: owner.ID, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE events`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UpdateChannel(ctx, store.UpdateChannelInput{ChannelID: channels[0].ID, UserID: owner.ID, Name: "after-events"}); err == nil {
		t.Fatal("expected channel outbox error")
	}
	if _, _, err := st.UpdateMessage(ctx, store.UpdateMessageInput{MessageID: message.ID, UserID: owner.ID, Body: "after-events"}); err == nil {
		t.Fatal("expected message update outbox error")
	}
	if _, _, err := st.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: message.ID, UserID: owner.ID}); err == nil {
		t.Fatal("expected message delete outbox error")
	}
}
