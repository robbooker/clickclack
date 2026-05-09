<script lang="ts" module>
  export type MessageListState = {
    atBottom: boolean;
    anchorMessageID?: string;
    anchorPixelOffset?: number;
  };

  export type MessageListViewportState = {
    atBottom: boolean;
    nearOlder: boolean;
    nearNewer: boolean;
  };

  export type MessageListHandle = {
    scrollToBottom: () => Promise<void>;
    scrollToMessage: (id: string) => boolean;
    scrollToDivider: (fallbackToAround?: boolean) => boolean;
    captureState: () => MessageListState | null;
    isAtBottom: () => boolean;
  };
</script>

<script lang="ts">
  import { onDestroy, tick } from "svelte";
  import { Virtualizer, type VirtualizerHandle } from "virtua/svelte";
  import { groupMessages, type MessageGroup as Group } from "../../lib/chat/messages";
  import { dmTitle } from "../../lib/chat/people";
  import type { Channel, DirectConversation, Message } from "../../lib/types";
  import HistoryLoader from "./HistoryLoader.svelte";
  import MessageGroup from "./MessageGroup.svelte";

  type Item =
    | { kind: "loader"; id: string; direction: "older" | "newer"; rows: number }
    | { kind: "day"; id: string; label: string }
    | { kind: "group"; id: string; group: Group }
    | { kind: "divider"; id: string };

  type Props = {
    messages: Message[];
    viewKey: string;
    loading?: boolean;
    unreadCount?: number;
    unreadBoundarySeq?: number;
    unreadBoundaryLoaded?: boolean;
    unreadSince?: string;
    hasOlder?: boolean;
    hasNewer?: boolean;
    loadingOlder?: boolean;
    loadingNewer?: boolean;
    prepending?: boolean;
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
    onLoadNewer?: () => void;
    onJumpToUnread?: () => void;
    onHistorySettled?: (state: MessageListViewportState) => void;
    onReachedBottom?: () => void;
    onMarkRead?: (readThroughSeq?: number) => void;
    onRetry?: (message: Message) => void;
    onDiscard?: (message: Message) => void;
  };

  let {
    messages,
    viewKey,
    loading = false,
    unreadCount = 0,
    unreadBoundarySeq = 0,
    unreadBoundaryLoaded = false,
    unreadSince = "",
    hasOlder = false,
    hasNewer = false,
    loadingOlder = false,
    loadingNewer = false,
    prepending = false,
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
    onLoadNewer,
    onJumpToUnread,
    onHistorySettled,
    onReachedBottom,
    onMarkRead,
    onRetry,
    onDiscard,
  }: Props = $props();

  // Sub-pixel tolerance — matches virtua's official chat example (FIXME comment
  // in their source notes devicePixelRatio rounding can prevent reaching exactly 0).
  const ANCHOR_THRESHOLD_PX = 1.5;
  const OLDER_LOAD_THRESHOLD_PX = 160;
  const NEWER_LOAD_THRESHOLD_PX = 260;
  const UNREAD_EXIT_MS = 180;

  let virtualizer: VirtualizerHandle | undefined = $state();
  let scrollEl: HTMLDivElement | undefined = $state();
  let historyLoaderEl: HTMLDivElement | undefined = $state();
  let historyLoaderHeight = $state(0);
  let viewportHeight = $state(0);
  let replyContext = $derived(selectedDirect ? "dm" : "channel");
  let dismissedUnreadViewKey = $state("");
  let dismissedUnreadBoundarySeq = $state(-1);
  let dismissedUnreadCount = $state(0);
  let targetUnreadCount = $derived(
    dismissedUnreadViewKey === viewKey &&
      dismissedUnreadBoundarySeq === unreadBoundarySeq &&
      unreadCount <= dismissedUnreadCount
      ? 0
      : unreadCount,
  );
  let displayUnreadViewKey = $state("");
  let displayUnreadCount = $state(0);
  let unreadClearing = $state(false);
  let unreadExitTimer: number | undefined;

  function clearUnreadExitTimer() {
    if (!unreadExitTimer) return;
    window.clearTimeout(unreadExitTimer);
    unreadExitTimer = undefined;
  }

  $effect(() => {
    if (displayUnreadViewKey && displayUnreadViewKey !== viewKey) {
      clearUnreadExitTimer();
      displayUnreadCount = 0;
      displayUnreadViewKey = "";
      unreadClearing = false;
      return;
    }

    if (targetUnreadCount > 0) {
      clearUnreadExitTimer();
      displayUnreadViewKey = viewKey;
      displayUnreadCount = targetUnreadCount;
      unreadClearing = false;
      return;
    }

    if (displayUnreadCount > 0 && !unreadClearing) {
      unreadClearing = true;
      unreadExitTimer = window.setTimeout(() => {
        unreadExitTimer = undefined;
        if (targetUnreadCount > 0) return;
        displayUnreadCount = 0;
        displayUnreadViewKey = "";
        unreadClearing = false;
      }, UNREAD_EXIT_MS);
    }
  });

  onDestroy(clearUnreadExitTimer);
  let listSpansUnreadBoundary = $derived.by(() => {
    const targetSeq = unreadBoundarySeq + 1;
    if (displayUnreadCount <= 0 || targetSeq <= 0) return false;
    let oldestSeq = Number.POSITIVE_INFINITY;
    let newestSeq = 0;
    for (const message of messages) {
      if (message.parent_message_id) continue;
      const seq = message.channel_seq || 0;
      if (seq <= 0) continue;
      oldestSeq = Math.min(oldestSeq, seq);
      newestSeq = Math.max(newestSeq, seq);
    }
    return oldestSeq <= targetSeq && newestSeq >= targetSeq;
  });
  let canUseUnreadDivider = $derived(unreadBoundaryLoaded && listSpansUnreadBoundary);

  let items = $derived.by<Item[]>(() => {
    const out: Item[] = [];
    let inserted = false;
    const crosses = (m: Message): boolean => {
      if (!canUseUnreadDivider) return false;
      if (m.parent_message_id) return false;
      if (m.author?.id === currentUserID || m.author_id === currentUserID) return false;
      return displayUnreadCount > 0 && (m.channel_seq || 0) > unreadBoundarySeq;
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
    if (loadingNewer) out.push({ kind: "loader", id: "loader-newer", direction: "newer", rows: 3 });
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
  let lastRestoreState: MessageListState | undefined;
  let pendingRestore = false;
  let suppressPagination = false;
  let suppressPaginationGeneration = 0;

  function suppressProgrammaticPagination(frames = 2) {
    const generation = ++suppressPaginationGeneration;
    suppressPagination = true;
    const release = (remaining: number) => {
      requestAnimationFrame(() => {
        if (generation !== suppressPaginationGeneration) return;
        if (remaining <= 1) {
          suppressPagination = false;
          return;
        }
        release(remaining - 1);
      });
    };
    release(frames);
  }

  function checkAtBottom(): boolean {
    if (!scrollEl) return true;
    // Read directly from the DOM. virtua's getScrollSize reflects its internal
    // (sometimes estimated) measurements; scrollEl.scrollHeight is ground truth.
    return scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight <= ANCHOR_THRESHOLD_PX;
  }

  function viewportState(): MessageListViewportState {
    if (!scrollEl) return { atBottom: true, nearOlder: false, nearNewer: false };
    const distance = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
    return {
      atBottom: distance <= ANCHOR_THRESHOLD_PX,
      nearOlder: scrollEl.scrollTop - historyLoaderHeight <= OLDER_LOAD_THRESHOLD_PX,
      nearNewer: distance <= NEWER_LOAD_THRESHOLD_PX,
    };
  }

  function emitHistorySettled() {
    onHistorySettled?.(viewportState());
  }

  function notifyReachedBottom() {
    if (hasNewer) return;
    onReachedBottom?.();
  }

  async function scrollToBottom() {
    if (!scrollEl || items.length === 0) return;
    shouldStickToBottom = true;
    await pinToBottom();
  }

  // Robust pin: write directly to scrollEl.scrollTop. The scroll container is
  // ours (the wrapper div), not virtua's internal one — so the DOM is always
  // ground truth even while virtua is mid-measurement. The ResizeObserver
  // hook below catches late layout shifts (images loading, markdown expanding)
  // and re-pins, which is what makes this work for variable-height items.
  async function pinToBottom() {
    if (!scrollEl) return;
    suppressProgrammaticPagination(12);
    let stableFrames = 0;
    for (let attempt = 0; attempt < 12; attempt++) {
      if (!scrollEl) return;
      const heightBefore = scrollEl.scrollHeight;
      scrollEl.scrollTop = scrollEl.scrollHeight;
      await new Promise((r) => requestAnimationFrame(r));
      if (!scrollEl) return;
      const heightStable = Math.abs(scrollEl.scrollHeight - heightBefore) < 1;
      if (checkAtBottom() && heightStable) {
        stableFrames++;
        if (stableFrames >= 2) break;
      } else {
        stableFrames = 0;
      }
    }
    if (checkAtBottom()) {
      atBottom = true;
      notifyReachedBottom();
    }
    emitHistorySettled();
  }

  $effect(() => {
    if (!scrollEl) return;
    const el = scrollEl;
    const onScroll = () => handleScroll(el.scrollTop);
    const onWheel = (event: WheelEvent) => {
      if (event.deltaY > 0 && !pendingRestore && !suppressPagination && hasNewer && checkAtBottom()) {
        onLoadNewer?.();
      }
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    el.addEventListener("wheel", onWheel, { passive: true });
    return () => {
      el.removeEventListener("scroll", onScroll);
      el.removeEventListener("wheel", onWheel);
    };
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
    const inner = scrollEl.lastElementChild as HTMLElement | null;
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

  // Discord-style scroll skeleton: when older history exists we render a
  // skeleton block ABOVE the virtualizer (outside `items` so prepends always
  // happen at index 0 — keeping virtua's cache aligned). Its height is fed
  // back into virtua via `startMargin` so all scroll math accounts for it.
  //
  // Sizing: we scale row count to roughly fill the viewport while loading
  // (so the user can keep dragging the scrollbar through "content" while a
  // fetch is in flight) and shrink to a small idle hint between fetches.
  //
  // Critical: when the skeleton grows or shrinks (e.g., loading→idle), we
  // adjust scrollTop by the delta so messages below stay at the same pixel.
  // Without this, the user's view jumps every time the skeleton resizes.
  const SKELETON_ROW_PX = 52;
  const SKELETON_IDLE_ROWS = 3;
  let skeletonRows = $derived.by(() => {
    if (!hasOlder || loading) return 0;
    if (!prepending) return SKELETON_IDLE_ROWS;
    const target = Math.max(viewportHeight, 480);
    return Math.max(SKELETON_IDLE_ROWS + 1, Math.ceil(target / SKELETON_ROW_PX));
  });

  $effect(() => {
    if (!scrollEl) return;
    const measure = () => {
      if (scrollEl) viewportHeight = scrollEl.clientHeight;
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(scrollEl);
    return () => ro.disconnect();
  });

  let prevSkeletonHeight = 0;
  $effect(() => {
    if (!historyLoaderEl || !scrollEl) {
      historyLoaderHeight = 0;
      prevSkeletonHeight = 0;
      return;
    }
    const root = scrollEl;
    const el = historyLoaderEl;
    const apply = () => {
      const next = el.offsetHeight;
      const prev = prevSkeletonHeight;
      historyLoaderHeight = next;
      prevSkeletonHeight = next;
      // Once the skeleton has been measured at least once, absorb any size
      // delta into scrollTop so content below the skeleton stays put. Skip
      // the very first measurement (that's the initial mount; scroll
      // restoration handles initial position).
      if (prev > 0 && next !== prev) {
        const delta = next - prev;
        // If the user is at the absolute top, keep them at the top instead
        // of dragging them into the freshly-grown skeleton.
        if (root.scrollTop > 0) root.scrollTop += delta;
      }
    };
    apply();
    const ro = new ResizeObserver(apply);
    ro.observe(el);
    return () => {
      ro.disconnect();
      historyLoaderHeight = 0;
      prevSkeletonHeight = 0;
    };
  });

  // Bulletproof anchor pinning for prepends — Discord/GitHub/Slack pattern.
  //
  // The hard truth: virtua's shift={true} only approximately compensates
  // scrollTop, and ResizeObserver remeasures keep firing for hundreds of ms
  // after a prepend (real row heights replacing estimates, fonts settling,
  // images, etc.). A one-shot fix is never enough — the anchor drifts later.
  //
  // Strategy:
  //   1. On detected prepend, save `scrollHeight - scrollTop` (the user's
  //      distance from the very bottom of loaded content). This invariant is
  //      pixel-perfect regardless of where height changes happen above.
  //   2. After commit, restore `scrollTop = scrollHeight - savedDistance`.
  //   3. Keep restoring on every ResizeObserver fire AND every animation frame
  //      for a short window (~800ms) so any late remeasure that grows/shrinks
  //      content above the viewport is absorbed without moving the user.
  //   4. If the user scrolls during the session, abort immediately.
  //
  // This makes scroll position invariant under content-size mutations above
  // the viewport — exactly what Discord does.
  let prevOldestMessageId = "";
  let prevAnchorViewKey = "";
  type PinSession = { savedBottomDistance: number; deadline: number };
  let pendingPin: PinSession | null = null;

  $effect.pre(() => {
    const oldestId = messages[0]?.id ?? "";
    const sameView = viewKey === prevAnchorViewKey;
    const isPrepend =
      sameView &&
      oldestId &&
      prevOldestMessageId &&
      oldestId !== prevOldestMessageId &&
      messages.some((m) => m.id === prevOldestMessageId);
    if (isPrepend && scrollEl) {
      pendingPin = {
        savedBottomDistance: scrollEl.scrollHeight - scrollEl.scrollTop,
        deadline: performance.now() + 800,
      };
    }
    prevOldestMessageId = oldestId;
    prevAnchorViewKey = viewKey;
  });

  $effect(() => {
    void messages;
    if (!pendingPin || !scrollEl) return;
    const session = pendingPin;
    pendingPin = null;
    const root = scrollEl;
    const inner = root.lastElementChild as HTMLElement | null;

    let lastAppliedTop = -1;
    let applying = false;

    const apply = () => {
      if (applying) return;
      // User-initiated scroll detection: if scrollTop drifted from what we
      // last applied (by more than rounding), the user took control. Stop.
      if (lastAppliedTop >= 0 && Math.abs(root.scrollTop - lastAppliedTop) > 2) {
        cleanup();
        return;
      }
      const target = root.scrollHeight - session.savedBottomDistance;
      if (Math.abs(root.scrollTop - target) > 0.5) {
        applying = true;
        root.scrollTop = target;
        applying = false;
      }
      lastAppliedTop = root.scrollTop;
    };

    let raf = 0;
    const tick = () => {
      apply();
      if (performance.now() < session.deadline) {
        raf = requestAnimationFrame(tick);
      } else {
        cleanup();
      }
    };

    const ro = inner ? new ResizeObserver(apply) : null;
    if (ro && inner) ro.observe(inner);

    function cleanup() {
      if (raf) cancelAnimationFrame(raf);
      ro?.disconnect();
    }

    apply();
    raf = requestAnimationFrame(tick);

    return cleanup;
  });

  function findDividerIndex(): number {
    return items.findIndex((it) => it.kind === "divider");
  }

  function firstUnreadMessageID(): string {
    if (!canUseUnreadDivider) return "";
    if (displayUnreadCount <= 0) return "";
    for (const message of messages) {
      if (message.parent_message_id) continue;
      if (message.author?.id === currentUserID || message.author_id === currentUserID) continue;
      if ((message.channel_seq || 0) > unreadBoundarySeq) return message.id;
    }
    return "";
  }

  function maxLoadedSeq(): number {
    let seq = 0;
    for (const message of messages) {
      seq = Math.max(seq, message.channel_seq || 0);
    }
    return seq;
  }

  function markUnreadRead() {
    dismissedUnreadViewKey = viewKey;
    dismissedUnreadBoundarySeq = unreadBoundarySeq;
    dismissedUnreadCount = unreadCount;
    onMarkRead?.(maxLoadedSeq());
  }

  function nextFrame(): Promise<void> {
    return new Promise((resolve) => requestAnimationFrame(() => resolve()));
  }

  function alignRenderedTarget(selector: string): boolean {
    if (!scrollEl) return false;
    const target = scrollEl.querySelector<HTMLElement>(selector);
    if (!target) return false;
    const delta = target.getBoundingClientRect().top - scrollEl.getBoundingClientRect().top;
    if (Math.abs(delta) <= ANCHOR_THRESHOLD_PX) return true;
    scrollEl.scrollTop += delta;
    return false;
  }

  function visibleMessageBounds(): { first: number; last: number } | null {
    if (!scrollEl) return null;
    const viewport = scrollEl.getBoundingClientRect();
    const byID = new Map(messages.map((message, index) => [message.id, index]));
    let first = Number.POSITIVE_INFINITY;
    let last = -1;
    for (const row of scrollEl.querySelectorAll<HTMLElement>("[data-message-id]")) {
      const rect = row.getBoundingClientRect();
      if (rect.bottom < viewport.top || rect.top > viewport.bottom) continue;
      const idx = byID.get(row.dataset.messageId || "");
      if (idx === undefined) continue;
      first = Math.min(first, idx);
      last = Math.max(last, idx);
    }
    return last < 0 ? null : { first, last };
  }

  function nudgeTowardMessage(messageID: string): boolean {
    if (!scrollEl) return false;
    const targetIndex = messages.findIndex((message) => message.id === messageID);
    const visible = visibleMessageBounds();
    if (targetIndex < 0 || !visible) return false;
    if (visible.first > targetIndex) {
      scrollEl.scrollTop -= Math.max(80, scrollEl.clientHeight * 0.85);
      return true;
    }
    if (visible.last < targetIndex) {
      scrollEl.scrollTop += Math.max(80, scrollEl.clientHeight * 0.85);
      return true;
    }
    return false;
  }

  function isCurrentView(key: string): boolean {
    return key === viewKey && key === lastViewKey;
  }

  async function settleVirtualTarget(
    key: string,
    indexForTarget: () => number,
    selector: string,
    targetMessageID = "",
  ): Promise<boolean> {
    suppressProgrammaticPagination(3);
    for (let attempt = 0; attempt < 24; attempt++) {
      if (!isCurrentView(key)) return false;
      if (!virtualizer || !scrollEl) return false;
      if (alignRenderedTarget(selector)) return true;
      const idx = indexForTarget();
      if (idx < 0) return false;
      virtualizer.scrollToIndex(idx, { align: "start" });
      virtualizer.scrollTo(Math.max(0, virtualizer.getItemOffset(idx)));
      await nextFrame();
      if (!isCurrentView(key)) return false;
      if (alignRenderedTarget(selector)) return true;
      if (targetMessageID && nudgeTowardMessage(targetMessageID)) await nextFrame();
    }
    if (!isCurrentView(key)) return false;
    return alignRenderedTarget(selector);
  }

  function scrollToMessage(messageID: string): boolean {
    if (!virtualizer) return false;
    const idx = findMessageIndex(messageID);
    if (idx < 0) return false;
    shouldStickToBottom = false;
    const key = viewKey;
    void settleVirtualTarget(
      key,
      () => findMessageIndex(messageID),
      `[data-message-id="${CSS.escape(messageID)}"]`,
      messageID,
    );
    return true;
  }

  function scrollToDivider(fallbackToAround = true): boolean {
    if (!virtualizer) return false;
    const idx = findDividerIndex();
    if (idx < 0) return false;
    shouldStickToBottom = false;
    const key = viewKey;
    void settleVirtualTarget(
      key,
      () => findDividerIndex(),
      "[data-unread-divider='true']",
      firstUnreadMessageID(),
    ).then((settled) => {
      if (fallbackToAround && !settled && isCurrentView(key)) onJumpToUnread?.();
    });
    return true;
  }

  function jumpToUnreadBoundary() {
    if (!hasNewer && checkAtBottom()) {
      markUnreadRead();
      return;
    }
    if (onJumpToUnread) {
      onJumpToUnread();
      return;
    }
    if (canUseUnreadDivider && scrollToDivider(false)) return;
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
      lastRestoreState = restoreState;
      shouldStickToBottom = true;
      atBottom = true;
      revealed = false;
      pendingRestore = true;
      void runRestore(key, restoreState, true);
      return;
    }

    const target = restoreState;
    if (target && target !== lastRestoreState) {
      lastRestoreState = target;
      if (!target.atBottom && target.anchorMessageID) {
        lastItemCount = count;
        pendingRestore = true;
        void runRestore(key, target, false);
        return;
      }
    }

    if (count > lastItemCount && shouldStickToBottom && !pendingRestore) {
      // Match official pattern: scroll on the same render as the data change.
      // pinToBottom handles the variable-height correction (scrollToIndex uses
      // an estimated size before measurement; we re-scroll once measured).
      void pinToBottom();
    } else if (count !== lastItemCount && !pendingRestore) {
      void emitSettledAfterFrames(key);
    }
    lastItemCount = count;
  });

  async function emitSettledAfterFrames(key: string) {
    await tick();
    await nextFrame();
    if (key === lastViewKey) emitHistorySettled();
  }

  async function runRestore(key: string, target: MessageListState | undefined, fallbackToBottom: boolean) {
    await tick();
    await new Promise((r) => requestAnimationFrame(r));
    if (key !== lastViewKey) return;
    if (target && !target.atBottom && target.anchorMessageID) {
      const restored = await restoreToAnchor(
        key,
        target.anchorMessageID,
        target.anchorPixelOffset ?? 0,
      );
      if (key !== lastViewKey) return;
      if (!restored && fallbackToBottom) await scrollToBottom();
      else shouldStickToBottom = false;
    } else {
      const dividerIdx = items.findIndex((it) => it.kind === "divider");
      if (dividerIdx >= 0 && virtualizer) {
        await settleVirtualTarget(
          key,
          () => findDividerIndex(),
          "[data-unread-divider='true']",
          firstUnreadMessageID(),
        );
        shouldStickToBottom = false;
      } else {
        await scrollToBottom();
      }
    }
    await new Promise((r) => requestAnimationFrame(r));
    if (key !== lastViewKey) return;
    pendingRestore = false;
    revealed = true;
    atBottom = checkAtBottom();
    shouldStickToBottom = atBottom;
    if (atBottom) notifyReachedBottom();
    if (scrollEl) handleScroll(scrollEl.scrollTop);
    emitHistorySettled();
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
      suppressProgrammaticPagination();
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
    atBottom = sticky;
    if (!sticky && hasOlder && scrollEl.scrollTop - historyLoaderHeight <= OLDER_LOAD_THRESHOLD_PX) onLoadOlder?.();
    if (!suppressPagination && hasNewer && (sticky || distance <= NEWER_LOAD_THRESHOLD_PX)) onLoadNewer?.();
    if (sticky) notifyReachedBottom();
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
      {#if hasOlder && !loading}
        <div
          class="messages-history-pad"
          bind:this={historyLoaderEl}
          aria-hidden={loadingOlder ? "false" : "true"}
        >
          <HistoryLoader direction="older" rows={skeletonRows} />
        </div>
      {/if}
      <Virtualizer
        bind:this={virtualizer}
        data={items}
        getKey={(item: Item) => item.id}
        shift={prepending}
        startMargin={historyLoaderHeight}
      >
        {#snippet children(item: Item, _index: number)}
          {#if item.kind === "loader"}
            <HistoryLoader direction={item.direction} rows={item.rows} />
          {:else if item.kind === "day"}
            <div class="day-divider"><span>{item.label}</span></div>
          {:else if item.kind === "divider"}
            <div
              class="new-messages-divider"
              class:is-clearing={unreadClearing}
              data-unread-divider="true"
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
  {#if !loading && messages.length > 0 && displayUnreadCount > 0}
    <div class="unread-bar" class:is-clearing={unreadClearing} role="status">
      <button
        type="button"
        class="unread-bar__jump"
        onclick={jumpToUnreadBoundary}
        aria-label={`Jump to ${displayUnreadCount > 0 ? displayUnreadCount : ""} new message${displayUnreadCount === 1 ? "" : "s"}`.replace(/  +/g, " ")}
      >
        <span class="unread-bar__label">
          {displayUnreadCount > 99 ? "99+" : displayUnreadCount} new message{displayUnreadCount === 1 ? "" : "s"}{unreadSince ? ` since ${unreadSince}` : ""}
        </span>
      </button>
      <button
        type="button"
        class="unread-bar__mark"
        onclick={markUnreadRead}
        aria-label="Mark as read"
      >
        <span>Mark read</span>
      </button>
    </div>
  {/if}
</div>
