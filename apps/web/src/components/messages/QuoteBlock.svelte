<script lang="ts">
  import { quoteSnippet, quotedAuthorName } from "../../lib/chat/messages";
  import type { Message } from "../../lib/types";

  type Props = {
    message: Message;
    onJump: (message: Message) => void;
  };

  let { message, onJump }: Props = $props();
</script>

{#if message.quoted_message_id || message.quoted_body_snapshot}
  <button
    type="button"
    class="quote-block"
    class:dangling={!message.quoted_message_id}
    onclick={() => onJump(message)}
    disabled={!message.quoted_message_id}
    aria-label={message.quoted_message_id ? `Jump to quoted message from ${quotedAuthorName(message)}` : "Original message was deleted"}
  >
    <span class="quote-bar" aria-hidden="true"></span>
    <span class="quote-content">
      <span class="quote-author">{quotedAuthorName(message)}</span>
      {#if message.quoted_message_id}
        <span class="quote-snippet">{quoteSnippet(message.quoted_body_snapshot)}</span>
      {:else}
        <span class="quote-snippet muted">[original deleted] {quoteSnippet(message.quoted_body_snapshot)}</span>
      {/if}
    </span>
  </button>
{/if}
