package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclaw/clickclack/apps/api/internal/store"
)

// quotesFixture spins up a workspace with two channels, a thread, and a DM so
// every test gets a consistent set of contexts to mix and match.
type quotesFixture struct {
	store     *Store
	owner     store.User
	other     store.User
	wsID      string
	channelA  store.Channel
	channelB  store.Channel
	rootA     store.Message // root message in channel A (thread root)
	otherRoot store.Message // root message in channel B
	dm        store.DirectConversation
	dmMsg     store.Message
}

func newQuotesFixture(t *testing.T) quotesFixture {
	t.Helper()
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
	wsID := workspaces[0].ID

	other, err := st.CreateUser(ctx, store.CreateUserInput{DisplayName: "Other", Email: "other@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(ctx, wsID, other.ID, "member"); err != nil {
		t.Fatal(err)
	}

	channels, err := st.ListChannels(ctx, wsID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	channelA := channels[0]
	channelB, _, err := st.CreateChannel(ctx, store.CreateChannelInput{WorkspaceID: wsID, Name: "second", Kind: "public", UserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	rootA, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channelA.ID, AuthorID: owner.ID, Body: "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	otherRoot, _, err := st.CreateMessage(ctx, store.CreateMessageInput{ChannelID: channelB.ID, AuthorID: owner.ID, Body: "from another channel"})
	if err != nil {
		t.Fatal(err)
	}
	dm, err := st.CreateDirectConversation(ctx, store.CreateDirectConversationInput{WorkspaceID: wsID, UserID: owner.ID, MemberIDs: []string{other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dmMsg, _, err := st.CreateDirectMessage(ctx, store.CreateDirectMessageInput{ConversationID: dm.ID, AuthorID: owner.ID, Body: "dm hi"})
	if err != nil {
		t.Fatal(err)
	}
	return quotesFixture{
		store: st, owner: owner, other: other, wsID: wsID,
		channelA: channelA, channelB: channelB,
		rootA: rootA, otherRoot: otherRoot, dm: dm, dmMsg: dmMsg,
	}
}

func ptr(s string) *string { return &s }

func TestCreateMessageWithQuotePersistsSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	reply, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding",
		QuotedMessageID: ptr(f.rootA.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.QuotedMessageID == nil || *reply.QuotedMessageID != f.rootA.ID {
		t.Fatalf("expected quoted_message_id %q, got %#v", f.rootA.ID, reply.QuotedMessageID)
	}
	if reply.QuotedBodySnapshot != "hello world" {
		t.Fatalf("expected snapshot %q, got %q", "hello world", reply.QuotedBodySnapshot)
	}
	if reply.QuotedAuthorID == nil || *reply.QuotedAuthorID != f.owner.ID {
		t.Fatalf("expected quoted_author_id %q, got %#v", f.owner.ID, reply.QuotedAuthorID)
	}
	if reply.QuotedAuthor == nil || reply.QuotedAuthor.ID != f.owner.ID {
		t.Fatalf("expected hydrated quoted author, got %#v", reply.QuotedAuthor)
	}

	page, err := f.store.ListMessages(ctx, f.channelA.ID, f.owner.ID, store.MessagePageRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	listed := page.Messages
	var found bool
	for _, m := range listed {
		if m.ID == reply.ID {
			found = true
			if m.QuotedMessageID == nil || *m.QuotedMessageID != f.rootA.ID {
				t.Fatalf("list: expected quoted_message_id, got %#v", m.QuotedMessageID)
			}
			if m.QuotedAuthor == nil {
				t.Fatalf("list: expected hydrated quoted author")
			}
		}
	}
	if !found {
		t.Fatalf("did not find reply in listed messages")
	}
}

func TestCreateMessageNonceReplayWithQuote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	first, event, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "quoted retry",
		QuotedMessageID: ptr(f.rootA.ID),
		Nonce:           "quoted-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == "" {
		t.Fatal("expected first quoted nonce send to emit an event")
	}
	if _, _, err := f.store.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: f.rootA.ID, UserID: f.owner.ID}); err != nil {
		t.Fatal(err)
	}
	replayed, replayEvent, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "quoted retry",
		QuotedMessageID: ptr(f.rootA.ID),
		Nonce:           "quoted-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != first.ID || replayEvent.ID != "" {
		t.Fatalf("expected quoted nonce replay without event, got %#v / %#v", replayed, replayEvent)
	}
	_, _, err = f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID: f.channelA.ID,
		AuthorID:  f.owner.ID,
		Body:      "quoted retry",
		Nonce:     "quoted-nonce-1",
	})
	if !errors.Is(err, store.ErrClientNonceConflict) {
		t.Fatalf("expected quote mismatch nonce conflict, got %v", err)
	}
}

func TestCreateThreadReplyNonceReplayWithQuote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	first, firstState, events, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID:   f.rootA.ID,
		AuthorID:        f.owner.ID,
		Body:            "thread retry",
		QuotedMessageID: ptr(f.rootA.ID),
		Nonce:           "thread-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstState.ReplyCount != 1 || len(events) != 2 {
		t.Fatalf("expected first reply event pair, got %#v / %#v", firstState, events)
	}
	if _, _, err := f.store.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: f.rootA.ID, UserID: f.owner.ID}); err != nil {
		t.Fatal(err)
	}
	replayed, replayState, replayEvents, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID:   f.rootA.ID,
		AuthorID:        f.owner.ID,
		Body:            "thread retry",
		QuotedMessageID: ptr(f.rootA.ID),
		Nonce:           "thread-nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ID != first.ID || replayState.ReplyCount != 1 || len(replayEvents) != 0 {
		t.Fatalf("expected thread nonce replay without events, got %#v / %#v / %#v", replayed, replayState, replayEvents)
	}
	_, _, _, err = f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID: f.rootA.ID,
		AuthorID:      f.owner.ID,
		Body:          "thread retry changed",
		Nonce:         "thread-nonce-1",
	})
	if !errors.Is(err, store.ErrClientNonceConflict) {
		t.Fatalf("expected thread nonce conflict, got %v", err)
	}
}

func TestCreateMessageRejectsCrossChannelQuote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	_, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding",
		QuotedMessageID: ptr(f.otherRoot.ID),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected ErrQuotedMessageOutOfScope, got %v", err)
	}
}

func TestCreateMessageRejectsQuotingDeletedMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	if _, _, err := f.store.DeleteMessage(ctx, store.DeleteMessageInput{MessageID: f.rootA.ID, UserID: f.owner.ID}); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding",
		QuotedMessageID: ptr(f.rootA.ID),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected ErrQuotedMessageOutOfScope, got %v", err)
	}
}

func TestCreateMessageRejectsUnknownQuote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	_, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding",
		QuotedMessageID: ptr("msg_does_not_exist"),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected ErrQuotedMessageOutOfScope for unknown id, got %v", err)
	}
}

func TestCreateMessageRejectsQuotingThreadReply(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	reply, _, _, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: f.rootA.ID, AuthorID: f.owner.ID, Body: "in thread"})
	if err != nil {
		t.Fatal(err)
	}
	// channel-timeline send may not quote a thread reply (parent_message_id is set)
	_, _, err = f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "trying",
		QuotedMessageID: ptr(reply.ID),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected scope rejection when quoting thread reply from channel timeline, got %v", err)
	}
}

func TestCreateThreadReplyWithQuoteAllowsRootAndSiblings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	first, _, _, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID:   f.rootA.ID,
		AuthorID:        f.owner.ID,
		Body:            "quoting root",
		QuotedMessageID: ptr(f.rootA.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.QuotedBodySnapshot != "hello world" {
		t.Fatalf("expected root snapshot, got %q", first.QuotedBodySnapshot)
	}
	second, _, _, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID:   f.rootA.ID,
		AuthorID:        f.owner.ID,
		Body:            "quoting sibling",
		QuotedMessageID: ptr(first.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.QuotedMessageID == nil || *second.QuotedMessageID != first.ID {
		t.Fatalf("expected sibling quote, got %#v", second.QuotedMessageID)
	}

	// Quoting a message from a different thread root must fail.
	otherRootReply, _, _, err := f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{RootMessageID: f.otherRoot.ID, AuthorID: f.owner.ID, Body: "noise"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = f.store.CreateThreadReply(ctx, store.CreateThreadReplyInput{
		RootMessageID:   f.rootA.ID,
		AuthorID:        f.owner.ID,
		Body:            "should fail",
		QuotedMessageID: ptr(otherRootReply.ID),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected scope rejection across threads, got %v", err)
	}
}

func TestCreateDirectMessageWithQuoteScopeIsConversation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	reply, _, err := f.store.CreateDirectMessage(ctx, store.CreateDirectMessageInput{
		ConversationID:  f.dm.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding in DM",
		QuotedMessageID: ptr(f.dmMsg.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.QuotedMessageID == nil || *reply.QuotedMessageID != f.dmMsg.ID {
		t.Fatalf("expected dm quote, got %#v", reply.QuotedMessageID)
	}

	// quoting a channel message from a DM conversation must fail
	_, _, err = f.store.CreateDirectMessage(ctx, store.CreateDirectMessageInput{
		ConversationID:  f.dm.ID,
		AuthorID:        f.owner.ID,
		Body:            "leak",
		QuotedMessageID: ptr(f.rootA.ID),
	})
	if !errors.Is(err, store.ErrQuotedMessageOutOfScope) {
		t.Fatalf("expected DM scope rejection, got %v", err)
	}
}

func TestQuoteSnapshotTrimAndTruncate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	long := "  " + strings.Repeat("x", maxQuoteSnapshotChars+50) + "  "
	root, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{ChannelID: f.channelA.ID, AuthorID: f.owner.ID, Body: long})
	if err != nil {
		t.Fatal(err)
	}
	reply, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "ok",
		QuotedMessageID: ptr(root.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(reply.QuotedBodySnapshot)); got != maxQuoteSnapshotChars {
		t.Fatalf("expected snapshot to be %d runes, got %d", maxQuoteSnapshotChars, got)
	}
	if strings.HasPrefix(reply.QuotedBodySnapshot, " ") || strings.HasSuffix(reply.QuotedBodySnapshot, " ") {
		t.Fatalf("snapshot was not trimmed: %q", reply.QuotedBodySnapshot)
	}
}

func TestDeletingQuotedMessageNullsRefButKeepsSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	reply, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "responding",
		QuotedMessageID: ptr(f.rootA.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Hard-delete the quoted row so the FK ON DELETE SET NULL fires. The app
	// soft-deletes today, but FK behaviour is what we care about here.
	if _, err := f.store.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, f.rootA.ID); err != nil {
		t.Fatal(err)
	}
	got, err := getMessage(ctx, f.store.db, reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.QuotedMessageID != nil {
		t.Fatalf("expected quoted_message_id to be NULL after delete, got %v", *got.QuotedMessageID)
	}
	if got.QuotedBodySnapshot != "hello world" {
		t.Fatalf("expected snapshot preserved, got %q", got.QuotedBodySnapshot)
	}
	if got.QuotedAuthorID == nil {
		t.Fatalf("expected quoted_author_id preserved (author still exists)")
	}
}

func TestEmptyQuotedMessageIDIsTreatedAsAbsent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newQuotesFixture(t)

	reply, _, err := f.store.CreateMessage(ctx, store.CreateMessageInput{
		ChannelID:       f.channelA.ID,
		AuthorID:        f.owner.ID,
		Body:            "no quote please",
		QuotedMessageID: ptr("   "),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.QuotedMessageID != nil {
		t.Fatalf("expected absent quote, got %v", *reply.QuotedMessageID)
	}
	if reply.QuotedBodySnapshot != "" {
		t.Fatalf("expected empty snapshot, got %q", reply.QuotedBodySnapshot)
	}
}
