<script lang="ts">
  import { threadSummary } from "../../lib/chat/messages";
  import { time, markdown } from "../../lib/format";
  import { uploadURL } from "../../lib/uploads";
  import type { Message } from "../../lib/types";
  import MediaAttachment from "../MediaAttachment.svelte";
  import QuoteBlock from "./QuoteBlock.svelte";

  type Props = {
    message: Message;
    index: number;
    selected: boolean;
    replyContext: "channel" | "dm";
    selectedThreadID?: string;
    onReply: (message: Message, context: "channel" | "dm") => void;
    onOpenThread: (message: Message) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
  };

  let {
    message,
    index,
    selected,
    replyContext,
    selectedThreadID,
    onReply,
    onOpenThread,
    onJumpToQuote,
    onOpenImage,
  }: Props = $props();
</script>

<div class="message-row" class:selected data-message-id={message.id}>
  <span class="row-stamp" aria-hidden="true">{index === 0 ? "" : time(message.created_at)}</span>
  <div class="message-content">
    <QuoteBlock {message} onJump={onJumpToQuote} />
    <div class="markdown">{@html markdown(message.body)}</div>
    {#if message.attachments?.length}
      <div class="attachment-grid" aria-label="Attachments">
        {#each message.attachments as attachment (attachment.id)}
          <MediaAttachment
            upload={attachment}
            url={uploadURL(attachment)}
            onOpenImage={onOpenImage}
          />
        {/each}
      </div>
    {/if}
  </div>
  <div class="message-actions" aria-label="Message actions">
    <button
      type="button"
      aria-label="Reply"
      class="tooltip"
      data-tooltip="Reply"
      onclick={() => onReply(message, replyContext)}
    >
      <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
        <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M9 17 4 12l5-5M4 12h11a5 5 0 0 1 5 5v3"/>
      </svg>
    </button>
    <button
      type="button"
      aria-label="Open thread"
      class="tooltip"
      data-tooltip={threadSummary(message, selectedThreadID)}
      onclick={() => onOpenThread(message)}
    >
      <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
        <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M21 12a8 8 0 0 1-11.6 7.16L3 21l1.84-6.4A8 8 0 1 1 21 12Z"/>
      </svg>
    </button>
  </div>
</div>
