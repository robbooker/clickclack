<script lang="ts" module>
  export type MessageListState = {
    atBottom: boolean;
    anchorMessageID?: string;
    anchorPixelOffset?: number;
  };

  export type MessageListHandle = {
    scrollToBottom: () => void;
    scrollToMessage: (id: string) => boolean;
    scrollToDivider: () => boolean;
    captureState: () => MessageListState | null;
    isAtBottom: () => boolean;
  };
</script>

<script lang="ts">
  import { tick } from "svelte";
  import { Virtualizer, type VirtualizerHandle } from "virtua/svelte";
  import { groupMessages, type MessageGroup as Group } from "../../lib/chat/messages";
  import { dmTitle } from "../../lib/chat/people";
  import type { Channel, DirectConversation, Message } from "../../lib/types";
  import MessageGroup from "./MessageGroup.svelte";

  type Item =
    | { kind: "day"; id: string; label: string }
    | { kind: "group"; id: string; group: Group }
    | { kind: "divider"; id: string };

  type Props = {
    messages: Message[];
    viewKey: string;
    loading?: boolean;
    unreadCount?: number;
    unreadAnchorMessageID?: string;
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
    onLoadOlder?: () => void;
    onReachedBottom?: () => void;
    onMarkRead?: () => void;
    onRetry?: (message: Message) => void;
    onDiscard?: (message: Message) => void;
  };

  let {
    messages,
    viewKey,
    loading = false,
    unreadCount = 0,
    unreadAnchorMessageID = "",
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
    onLoadOlder,
    onReachedBottom,
    onMarkRead,
    onRetry,
    onDiscard,
  }: Props = $props();

  // Sub-pixel tolerance — matches virtua's official chat example (FIXME comment
  // in their source notes devicePixelRatio rounding can prevent reaching exactly 0).
  const ANCHOR_THRESHOLD_PX = 1.5;

  let virtualizer: VirtualizerHandle | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();
  let replyContext = $derived(selectedDirect ? "dm" : "channel");

  let items = $derived.by<Item[]>(() => {
    const out: Item[] = [];
    const anchorID = unreadAnchorMessageID;
    let inserted = anchorID === "";
    const crosses = (m: Message): boolean => {
      if (m.parent_message_id) return false;
      return m.id === anchorID;
    };
    for (const group of groupMessages(messages)) {
      let splitIdx = -1;
      if (!inserted) {
        for (let i = 0; i < group.messages.length; i++) {
          if (crosses(group.messages[i])) {
            splitIdx = i;
            break;
          }
        }
      }
      if (group.dayLabel) {
        out.push({ kind: "day", id: `day-${group.key}`, label: group.dayLabel });
      }
      if (splitIdx === -1) {
        out.push({ kind: "group", id: group.key, group });
      } else if (splitIdx === 0) {
        out.push({ kind: "divider", id: `div-${group.key}` });
        out.push({ kind: "group", id: group.key, group });
        inserted = true;
      } else {
        const before: Group = {
          ...group,
          key: `${group.key}-pre`,
          messages: group.messages.slice(0, splitIdx),
        };
        const after: Group = {
          ...group,
          key: `${group.key}-post`,
          dayLabel: null,
          messages: group.messages.slice(splitIdx),
          timestamp: group.messages[splitIdx].created_at,
        };
        out.push({ kind: "group", id: before.key, group: before });
        out.push({ kind: "divider", id: `div-${group.key}` });
        out.push({ kind: "group", id: after.key, group: after });
        inserted = true;
      }
    }
    return out;
  });

  // shouldStickToBottom mirrors the official virtua chat example: a ref-style
  // flag updated SYNCHRONOUSLY in onscroll using the live offset/scrollSize.
  // The items effect reads this flag (not a derived state) — so when items
  // append (which does NOT fire onscroll), the previous user-driven value wins.
  let shouldStickToBottom = true;
  // Reactive mirror for the FAB visibility only. Never gates programmatic scroll.
  let atBottom = $state(true);
  let revealed = $state(false);
  let lastViewKey: string | undefined;
  let lastItemCount = 0;
  let pendingRestore = false;

  function checkAtBottom(): boolean {
    if (!scrollEl) return true;
    // Read directly from the DOM. virtua's getScrollSize reflects its internal
    // (sometimes estimated) measurements; scrollEl.scrollHeight is ground truth.
    return scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight <= ANCHOR_THRESHOLD_PX;
  }

  function notifyReachedBottom() {
    onReachedBottom?.();
  }

  function scrollToBottom() {
    if (!scrollEl || items.length === 0) return;
    shouldStickToBottom = true;
    pinToBottom();
  }

  // Robust pin: write directly to scrollEl.scrollTop. The scroll container is
  // ours (the wrapper div), not virtua's internal one — so the DOM is always
  // ground truth even while virtua is mid-measurement. The ResizeObserver
  // hook below catches late layout shifts (images loading, markdown expanding)
  // and re-pins, which is what makes this work for variable-height items.
  function pinToBottom() {
    if (!scrollEl) return;
    scrollEl.scrollTop = scrollEl.scrollHeight;
    requestAnimationFrame(() => {
      if (!scrollEl) return;
      scrollEl.scrollTop = scrollEl.scrollHeight;
      if (checkAtBottom()) {
        atBottom = true;
        notifyReachedBottom();
      }
    });
  }

  $effect(() => {
    if (!scrollEl) return;
    const el = scrollEl;
    const onScroll = () => handleScroll(el.scrollTop);
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  });

  // Watch the inner content for size changes. While stuck to bottom, every
  // layout shift (item measured, image loaded, font swap) grows scrollHeight
  // and would leave the user not-quite-at-bottom. We coalesce all observed
  // shifts into a single rAF write so a burst of measurements (common during
  // the initial render of a long history) costs one scrollTop write per frame.
  $effect(() => {
    if (!scrollEl) return;
    let raf = 0;
    const schedule = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        if (shouldStickToBottom && !pendingRestore && scrollEl) {
          scrollEl.scrollTop = scrollEl.scrollHeight;
        }
      });
    };
    const observer = new ResizeObserver(schedule);
    // Observe only the inner content (the Virtualizer's container). The outer
    // scrollEl rarely resizes; observing it would just add noise.
    const inner = scrollEl.firstElementChild?.nextElementSibling as HTMLElement | null;
    if (inner) observer.observe(inner);
    return () => {
      if (raf) cancelAnimationFrame(raf);
      observer.disconnect();
    };
  });

  function findMessageIndex(messageID: string): number {
    return items.findIndex(
      (it) => it.kind === "group" && it.group.messages.some((m) => m.id === messageID),
    );
  }

  function findDividerIndex(): number {
    return items.findIndex((it) => it.kind === "divider");
  }

  function scrollToMessage(messageID: string): boolean {
    if (!virtualizer) return false;
    const idx = findMessageIndex(messageID);
    if (idx < 0) return false;
    shouldStickToBottom = false;
    virtualizer.scrollToIndex(idx, { align: "start" });
    return true;
  }

  function scrollToDivider(): boolean {
    if (!virtualizer) return false;
    const idx = findDividerIndex();
    if (idx < 0) return false;
    shouldStickToBottom = false;
    // Align the divider to the top of the viewport — same as Discord. Smooth
    // scrolling so the user gets visual continuity.
    virtualizer.scrollToIndex(idx, { align: "start", smooth: true });
    return true;
  }

  function captureState(): MessageListState | null {
    if (!virtualizer) return null;
    const isAtBottom = checkAtBottom();
    if (isAtBottom) return { atBottom: true };
    const offset = virtualizer.getScrollOffset();
    const idx = virtualizer.findItemIndex(offset);
    for (let i = Math.max(0, idx); i < items.length; i++) {
      const it = items[i];
      if (it.kind !== "group") continue;
      const itemTop = virtualizer.getItemOffset(i);
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
    onListRef({ scrollToBottom, scrollToMessage, scrollToDivider, captureState, isAtBottom: () => checkAtBottom() });
    return () => onListRef(null);
  });

  // Drive scroll on data change. Mirrors the official chat example's
  // useEffect([items]): if shouldStickToBottom, pin to last index. The flag is
  // the source of truth — last user scroll set it, item-append doesn't move it.
  $effect(() => {
    const key = viewKey;
    const count = items.length;

    if (key !== lastViewKey) {
      lastViewKey = key;
      lastItemCount = count;
      shouldStickToBottom = true;
      atBottom = true;
      revealed = false;
      pendingRestore = true;
      void runRestore(key);
      return;
    }

    if (count > lastItemCount && shouldStickToBottom && !pendingRestore) {
      // Match official pattern: scroll on the same render as the data change.
      // pinToBottom handles the variable-height correction (scrollToIndex uses
      // an estimated size before measurement; we re-scroll once measured).
      pinToBottom();
    }
    lastItemCount = count;
  });

  async function runRestore(key: string) {
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
      else shouldStickToBottom = false;
    } else {
      const dividerIdx = items.findIndex((it) => it.kind === "divider");
      if (dividerIdx >= 0 && virtualizer) {
        virtualizer.scrollToIndex(dividerIdx, { align: "start" });
        shouldStickToBottom = false;
      } else {
        scrollToBottom();
      }
    }
    await new Promise((r) => requestAnimationFrame(r));
    if (key !== lastViewKey) return;
    pendingRestore = false;
    revealed = true;
    atBottom = checkAtBottom();
    shouldStickToBottom = atBottom;
    if (atBottom) notifyReachedBottom();
  }

  async function restoreToAnchor(
    key: string,
    messageID: string,
    pixelOffset: number,
  ): Promise<boolean> {
    if (!virtualizer) return false;
    const idx = findMessageIndex(messageID);
    if (idx < 0) return false;
    for (let attempt = 0; attempt < 8; attempt++) {
      if (!virtualizer || key !== lastViewKey) return false;
      const desired = virtualizer.getItemOffset(idx) + pixelOffset;
      virtualizer.scrollTo(desired);
      await new Promise((r) => requestAnimationFrame(r));
      if (key !== lastViewKey) return false;
      const recheck = virtualizer.getItemOffset(idx) + pixelOffset;
      const current = virtualizer.getScrollOffset();
      if (Math.abs(recheck - desired) < 1 && Math.abs(current - desired) < 1) break;
    }
    return true;
  }

  // Synchronously update shouldStickToBottom on every scroll. We read the DOM
  // directly so we're immune to virtua's measurement estimates.
  function handleScroll(_offset: number) {
    if (!scrollEl || pendingRestore) return;
    const distance = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
    const sticky = distance <= ANCHOR_THRESHOLD_PX;
    shouldStickToBottom = sticky;
    const wasAtBottom = atBottom;
    atBottom = sticky;
    if (!sticky && scrollEl.scrollTop <= 160) onLoadOlder?.();
    if (sticky && !wasAtBottom) notifyReachedBottom();
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
    <div class="messages-scroll" bind:this={scrollEl}>
      <div class="messages-spacer"></div>
      <Virtualizer
        bind:this={virtualizer}
        data={items}
        getKey={(item: Item) => item.id}
      >
        {#snippet children(item: Item, _index: number)}
          {#if item.kind === "day"}
            <div class="day-divider"><span>{item.label}</span></div>
          {:else if item.kind === "divider"}
            <div
              class="new-messages-divider"
              role="separator"
              aria-label="New messages"
            >
              <span>New</span>
            </div>
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
      </Virtualizer>
    </div>
  {/if}
  {#if !loading && messages.length > 0 && unreadAnchorMessageID}
    <div class="unread-bar" role="status">
      <button
        type="button"
        class="unread-bar__jump"
        onclick={() => scrollToDivider()}
        aria-label={`Jump to ${unreadCount > 0 ? unreadCount : ""} new message${unreadCount === 1 ? "" : "s"}`.replace(/  +/g, " ")}
      >
        <span class="unread-bar__label">
          {#if unreadCount > 0}
            {unreadCount > 99 ? "99+" : unreadCount} new message{unreadCount === 1 ? "" : "s"}
          {:else}
            New messages
          {/if}
        </span>
      </button>
      <button
        type="button"
        class="unread-bar__mark"
        onclick={() => onMarkRead?.()}
        aria-label="Mark as read"
      >
        <span>Mark As Read</span>
        <svg viewBox="0 0 16 16" aria-hidden="true" width="12" height="12">
          <path fill="currentColor" d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06z"/>
        </svg>
      </button>
    </div>
  {/if}
</div>
