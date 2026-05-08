<script lang="ts" module>
  export type MessageListState = {
    atBottom: boolean;
    anchorMessageID?: string;
    anchorPixelOffset?: number;
  };

  export type MessageListHandle = {
    scrollToBottom: () => void;
    scrollToMessage: (id: string) => boolean;
    captureState: () => MessageListState | null;
  };
</script>

<script lang="ts">
  import { tick } from "svelte";
  import { VList, type VListHandle } from "virtua/svelte";
  import { groupMessages, type MessageGroup as Group } from "../../lib/chat/messages";
  import { dmTitle } from "../../lib/chat/people";
  import type { Channel, DirectConversation, Message } from "../../lib/types";
  import MessageGroup from "./MessageGroup.svelte";

  type Item =
    | { kind: "day"; id: string; label: string }
    | { kind: "group"; id: string; group: Group };

  type Props = {
    messages: Message[];
    viewKey: string;
    loading?: boolean;
    unreadCount?: number;
    restoreState?: MessageListState;
    selectedDirect?: DirectConversation;
    selectedChannel?: Channel;
    selectedThreadID?: string;
    currentUserID?: string;
    onListRef: (handle: MessageListHandle | null) => void;
    onActivateMessageComposer: () => void;
    onInlineImagePointerUp: (event: PointerEvent) => void;
    onOpenProfile: (profile?: Message["author"]) => void;
    onReply: (message: Message, context: "channel" | "dm") => void;
    onOpenThread: (message: Message) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
    onRetry?: (message: Message) => void;
    onDiscard?: (message: Message) => void;
  };

  let {
    messages,
    viewKey,
    loading = false,
    unreadCount = 0,
    restoreState,
    selectedDirect,
    selectedChannel,
    selectedThreadID,
    currentUserID,
    onListRef,
    onActivateMessageComposer,
    onInlineImagePointerUp,
    onOpenProfile,
    onReply,
    onOpenThread,
    onJumpToQuote,
    onOpenImage,
    onRetry,
    onDiscard,
  }: Props = $props();

  const ANCHOR_THRESHOLD_PX = 120;

  let vlist: VListHandle | undefined = $state();
  let replyContext = $derived(selectedDirect ? "dm" : "channel");

  let items = $derived.by<Item[]>(() => {
    const out: Item[] = [];
    for (const group of groupMessages(messages)) {
      if (group.dayLabel) {
        out.push({ kind: "day", id: `day-${group.key}`, label: group.dayLabel });
      }
      out.push({ kind: "group", id: group.key, group });
    }
    return out;
  });

  let atBottom = $state(true);
  let revealed = $state(false);
  let lastViewKey: string | undefined;
  let lastItemCount = 0;
  let pendingRestore = false;

  function checkAtBottom(): boolean {
    if (!vlist) return true;
    const offset = vlist.getScrollOffset();
    const total = vlist.getScrollSize();
    const viewport = vlist.getViewportSize();
    return total - offset - viewport <= ANCHOR_THRESHOLD_PX;
  }

  function scrollToBottom() {
    if (!vlist || items.length === 0) return;
    vlist.scrollToIndex(items.length - 1, { align: "end" });
  }

  function findMessageIndex(messageID: string): number {
    return items.findIndex(
      (it) => it.kind === "group" && it.group.messages.some((m) => m.id === messageID),
    );
  }

  function scrollToMessage(messageID: string): boolean {
    if (!vlist) return false;
    const idx = findMessageIndex(messageID);
    if (idx < 0) return false;
    vlist.scrollToIndex(idx, { align: "start" });
    return true;
  }

  function captureState(): MessageListState | null {
    if (!vlist) return null;
    const isAtBottom = checkAtBottom();
    if (isAtBottom) return { atBottom: true };
    const offset = vlist.getScrollOffset();
    const idx = vlist.findItemIndex(offset);
    for (let i = Math.max(0, idx); i < items.length; i++) {
      const it = items[i];
      if (it.kind !== "group") continue;
      const itemTop = vlist.getItemOffset(i);
      const anchorMessageID = it.group.messages[0]?.id;
      if (!anchorMessageID) continue;
      return {
        atBottom: false,
        anchorMessageID,
        anchorPixelOffset: Math.max(0, offset - itemTop),
      };
    }
    return { atBottom: false };
  }

  $effect(() => {
    onListRef({ scrollToBottom, scrollToMessage, captureState });
    return () => onListRef(null);
  });

  // Watch viewKey + items to drive: hide-on-switch, scroll-restore, autoscroll-on-new-message.
  $effect(() => {
    const key = viewKey;
    const count = items.length;

    if (key !== lastViewKey) {
      lastViewKey = key;
      lastItemCount = count;
      atBottom = true;
      revealed = false;
      pendingRestore = true;
      void runRestore(key);
      return;
    }

    if (count > lastItemCount && atBottom && !pendingRestore) {
      void pinAfterRender();
    }
    lastItemCount = count;
  });

  async function runRestore(key: string) {
    // Wait two frames so VList mounts/measures with the new data.
    await tick();
    await new Promise((r) => requestAnimationFrame(r));
    if (key !== lastViewKey) return;
    const target = restoreState;
    if (target && !target.atBottom && target.anchorMessageID) {
      const restored = await restoreToAnchor(
        key,
        target.anchorMessageID,
        target.anchorPixelOffset ?? 0,
      );
      if (key !== lastViewKey) return;
      if (!restored) scrollToBottom();
    } else {
      scrollToBottom();
    }
    // Allow virtua one more frame to settle measurements before revealing.
    await new Promise((r) => requestAnimationFrame(r));
    if (key !== lastViewKey) return;
    pendingRestore = false;
    revealed = true;
    atBottom = checkAtBottom();
  }

  // Iteratively converge on the target offset. virtua may not have measured
  // every item yet; getItemOffset returns an estimate that becomes accurate
  // as items render. We re-scroll on each frame until the offset is stable.
  async function restoreToAnchor(
    key: string,
    messageID: string,
    pixelOffset: number,
  ): Promise<boolean> {
    if (!vlist) return false;
    const idx = findMessageIndex(messageID);
    if (idx < 0) return false;
    for (let attempt = 0; attempt < 8; attempt++) {
      if (!vlist || key !== lastViewKey) return false;
      const desired = vlist.getItemOffset(idx) + pixelOffset;
      vlist.scrollTo(desired);
      await new Promise((r) => requestAnimationFrame(r));
      if (key !== lastViewKey) return false;
      const recheck = vlist.getItemOffset(idx) + pixelOffset;
      const current = vlist.getScrollOffset();
      if (Math.abs(recheck - desired) < 1 && Math.abs(current - desired) < 1) break;
    }
    return true;
  }

  async function pinAfterRender() {
    await tick();
    scrollToBottom();
  }

  function handleScroll(_offset: number) {
    if (pendingRestore) return;
    atBottom = checkAtBottom();
  }
</script>

<div
  class="messages"
  class:is-revealing={loading || (!revealed && messages.length > 0)}
  role="log"
  aria-live="polite"
  onpointerdown={onActivateMessageComposer}
  onpointerup={onInlineImagePointerUp}
>
  {#if !loading && messages.length === 0}
    <div class="empty">
      <div class="empty-icon">
        {#if selectedDirect}@{:else}#{/if}
      </div>
      <strong>
        {#if selectedDirect}
          This is the start of your conversation with {dmTitle(selectedDirect, currentUserID)}.
        {:else if selectedChannel}
          Welcome to #{selectedChannel.name}!
        {:else}
          Pick a channel to get started.
        {/if}
      </strong>
      <span>Send a message in Markdown — code fences, lists, links all work. Threads open from any message.</span>
    </div>
  {:else if messages.length > 0}
    <VList
      bind:this={vlist}
      data={items}
      getKey={(item: Item) => item.id}
      onscroll={handleScroll}
      class="messages-vlist"
      style="padding: 16px 4px 24px;"
    >
      {#snippet children(item: Item, _index: number)}
        {#if item.kind === "day"}
          <div class="day-divider"><span>{item.label}</span></div>
        {:else}
          <MessageGroup
            group={item.group}
            {selectedThreadID}
            {replyContext}
            {onOpenProfile}
            {onReply}
            {onOpenThread}
            {onJumpToQuote}
            {onOpenImage}
            {onRetry}
            {onDiscard}
          />
        {/if}
      {/snippet}
    </VList>
  {/if}
  {#if !loading && messages.length > 0}
    <button
      type="button"
      class="jump-to-bottom"
      class:visible={!atBottom && revealed}
      aria-label={unreadCount > 0 ? `Jump to ${unreadCount} new message${unreadCount === 1 ? "" : "s"}` : "Jump to most recent"}
      onclick={() => scrollToBottom()}
      tabindex={!atBottom && revealed ? 0 : -1}
    >
      {#if unreadCount > 0}
        <span class="jump-to-bottom__count">{unreadCount > 99 ? "99+" : unreadCount} new</span>
      {/if}
      <svg viewBox="0 0 16 16" aria-hidden="true" width="14" height="14">
        <path fill="currentColor" d="M8 11.5a1 1 0 0 1-.71-.29l-4-4a1 1 0 1 1 1.42-1.42L8 9.09l3.29-3.3a1 1 0 0 1 1.42 1.42l-4 4a1 1 0 0 1-.71.29z"/>
      </svg>
    </button>
  {/if}
</div>
