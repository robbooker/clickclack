<script lang="ts">
  import { groupMessages } from "../../lib/chat/messages";
  import { dmTitle } from "../../lib/chat/people";
  import type { Channel, DirectConversation, Message } from "../../lib/types";
  import MessageGroup from "./MessageGroup.svelte";

  type Props = {
    messages: Message[];
    selectedDirect?: DirectConversation;
    selectedChannel?: Channel;
    selectedThreadID?: string;
    currentUserID?: string;
    onListRef: (node: HTMLElement | null) => void;
    onActivateMessageComposer: () => void;
    onInlineImagePointerUp: (event: PointerEvent) => void;
    onOpenProfile: (profile?: Message["author"]) => void;
    onReply: (message: Message, context: "channel" | "dm") => void;
    onOpenThread: (message: Message) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
  };

  let {
    messages,
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
  }: Props = $props();

  let listNode: HTMLElement | null = $state(null);
  let groupedMessages = $derived(groupMessages(messages));
  let replyContext = $derived(selectedDirect ? "dm" : "channel");

  $effect(() => {
    onListRef(listNode);
    return () => onListRef(null);
  });
</script>

<div
  class="messages"
  role="log"
  aria-live="polite"
  bind:this={listNode}
  onpointerdown={onActivateMessageComposer}
  onpointerup={onInlineImagePointerUp}
>
  {#if messages.length === 0}
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
  {/if}
  {#each groupedMessages as group (group.key)}
    {#if group.dayLabel}
      <div class="day-divider"><span>{group.dayLabel}</span></div>
    {/if}
    <MessageGroup
      {group}
      {selectedThreadID}
      {replyContext}
      {onOpenProfile}
      {onReply}
      {onOpenThread}
      {onJumpToQuote}
      {onOpenImage}
    />
  {/each}
</div>
