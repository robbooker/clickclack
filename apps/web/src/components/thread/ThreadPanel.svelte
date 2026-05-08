<script lang="ts">
  import { avatarHue, avatarInitial, handleLabel } from "../../lib/chat/people";
  import { markdown, time } from "../../lib/format";
  import { uploadURL } from "../../lib/uploads";
  import type { Message, ThreadState } from "../../lib/types";
  import ChatComposer from "../composer/ChatComposer.svelte";
  import MediaAttachment from "../MediaAttachment.svelte";
  import QuoteBlock from "../messages/QuoteBlock.svelte";

  type Props = {
    root: Message;
    replies: Message[];
    threadState: ThreadState | null;
    replyBody: string;
    replyTarget: Message | null;
    onClose: () => void;
    onReplyBody: (value: string) => void;
    onSubmitReply: () => void;
    onReplyKeydown: (event: KeyboardEvent) => void;
    onReplyFocus: () => void;
    onReplyInputRef: (node: HTMLTextAreaElement | null) => void;
    onSetReplyTarget: (message: Message, context: "thread") => void;
    onClearReply: () => void;
    onActivateThreadComposer: () => void;
    onInlineImagePointerUp: (event: PointerEvent) => void;
    onJumpToQuote: (message: Message) => void;
    onOpenImage: (url: string, title: string) => void;
  };

  let {
    root,
    replies,
    threadState,
    replyBody,
    replyTarget,
    onClose,
    onReplyBody,
    onSubmitReply,
    onReplyKeydown,
    onReplyFocus,
    onReplyInputRef,
    onSetReplyTarget,
    onClearReply,
    onActivateThreadComposer,
    onInlineImagePointerUp,
    onJumpToQuote,
    onOpenImage,
  }: Props = $props();
</script>

<header>
  <div>
    <p>Thread</p>
    <strong>{threadState?.reply_count ?? replies.length} {(threadState?.reply_count ?? replies.length) === 1 ? "reply" : "replies"}</strong>
  </div>
  <button
    class="close"
    aria-label="Close thread"
    onclick={onClose}
  >×</button>
</header>
<div
  class="thread-scroll"
  role="region"
  aria-label="Thread messages"
  onpointerdown={onActivateThreadComposer}
  onpointerup={onInlineImagePointerUp}
>
  <article class="thread-root" data-message-id={root.id}>
    <div class="avatar" style="--hue: {avatarHue(root.author?.id || root.author_id || 'x')}deg">
      {#if root.author?.avatar_url}
        <img src={root.author.avatar_url} alt="" loading="lazy" />
      {:else}
        {avatarInitial(root.author?.display_name)}
      {/if}
    </div>
    <div class="group-body">
      <header>
        <strong>{root.author?.display_name || "Local User"}</strong>
        {#if root.author?.handle}<span>{handleLabel(root.author.handle)}</span>{/if}
        <time>{time(root.created_at)}</time>
        <button
          type="button"
          class="reply-quote-btn"
          aria-label="Reply"
          data-tooltip="Reply"
          onclick={() => onSetReplyTarget(root, "thread")}
        >↩</button>
      </header>
      <div class="markdown">{@html markdown(root.body)}</div>
      {#if root.attachments?.length}
        <div class="attachment-grid compact" aria-label="Attachments">
          {#each root.attachments as attachment (attachment.id)}
            <MediaAttachment
              upload={attachment}
              url={uploadURL(attachment)}
              onOpenImage={onOpenImage}
            />
          {/each}
        </div>
      {/if}
    </div>
  </article>
  <div class="thread-divider"><span>{replies.length} {replies.length === 1 ? "reply" : "replies"}</span></div>
  <div class="reply-list">
    {#each replies as reply (reply.id)}
      <article class="reply" data-message-id={reply.id}>
        <div class="avatar small" style="--hue: {avatarHue(reply.author?.id || reply.author_id || 'x')}deg">
          {#if reply.author?.avatar_url}
            <img src={reply.author.avatar_url} alt="" loading="lazy" />
          {:else}
            {avatarInitial(reply.author?.display_name)}
          {/if}
        </div>
        <div class="group-body">
          <header>
            <strong>{reply.author?.display_name || "Local User"}</strong>
            {#if reply.author?.handle}<span>{handleLabel(reply.author.handle)}</span>{/if}
            <time>{time(reply.created_at)}</time>
            <button
              type="button"
              class="reply-quote-btn"
              aria-label="Reply"
              data-tooltip="Reply"
              onclick={() => onSetReplyTarget(reply, "thread")}
            >↩</button>
          </header>
          <QuoteBlock message={reply} onJump={onJumpToQuote} />
          <div class="markdown">{@html markdown(reply.body)}</div>
          {#if reply.attachments?.length}
            <div class="attachment-grid compact" aria-label="Attachments">
              {#each reply.attachments as attachment (attachment.id)}
                <MediaAttachment
                  upload={attachment}
                  url={uploadURL(attachment)}
                  onOpenImage={onOpenImage}
                />
              {/each}
            </div>
          {/if}
        </div>
      </article>
    {/each}
  </div>
</div>
<ChatComposer
  value={replyBody}
  placeholder="Reply in thread"
  ariaLabel="Reply body"
  submitLabel="Reply"
  formClass="composer reply-composer"
  replyTarget={replyTarget}
  onValue={onReplyBody}
  onSubmit={onSubmitReply}
  onKeydown={onReplyKeydown}
  onFocus={onReplyFocus}
  onInputRef={onReplyInputRef}
  onClearReply={onClearReply}
/>
