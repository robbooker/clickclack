<script lang="ts">
  import { avatarHue, avatarInitial, handleLabel } from "../../lib/chat/people";
  import { time } from "../../lib/format";
  import type { Message } from "../../lib/types";
  import type { MessageGroup as MessageGroupType } from "../../lib/chat/messages";
  import MessageRow from "./MessageRow.svelte";

  type Props = {
    group: MessageGroupType;
    selectedThreadID?: string;
    replyContext: "channel" | "dm";
    onOpenProfile: (profile?: Message["author"]) => void;
    onReply: (message: Message, context: "channel" | "dm") => void;
    onOpenThread: (message: Message) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
    onRetry?: (message: Message) => void;
    onDiscard?: (message: Message) => void;
  };

  let {
    group,
    selectedThreadID,
    replyContext,
    onOpenProfile,
    onReply,
    onOpenThread,
    onJumpToQuote,
    onOpenImage,
    onRetry,
    onDiscard,
  }: Props = $props();
</script>

<article class="message-group">
  <button
    type="button"
    class="avatar avatar-button"
    style="--hue: {avatarHue(group.authorID)}deg"
    aria-label={`View profile for ${group.authorName}`}
    onclick={() => onOpenProfile(group.messages[0]?.author)}
  >
    {#if group.authorAvatarURL}
      <img src={group.authorAvatarURL} alt="" loading="lazy" />
    {:else}
      {avatarInitial(group.authorName)}
    {/if}
  </button>
  <div class="group-body">
    <header>
      <button
        type="button"
        class="author-name"
        onclick={() => onOpenProfile(group.messages[0]?.author)}
      >{group.authorName}</button>
      {#if group.authorHandle}<span>{handleLabel(group.authorHandle)}</span>{/if}
      <time>{time(group.timestamp)}</time>
    </header>
    {#each group.messages as message, index (message.id)}
      <MessageRow
        {message}
        {index}
        selected={selectedThreadID === message.id}
        {replyContext}
        {selectedThreadID}
        {onReply}
        {onOpenThread}
        {onJumpToQuote}
        {onOpenImage}
        {onRetry}
        {onDiscard}
      />
    {/each}
  </div>
</article>
