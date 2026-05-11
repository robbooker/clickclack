<script lang="ts">
  import type { Channel } from "../../lib/types";

  type Props = {
    channels: Channel[];
    selectedChannelID: string;
    selectedDirectID: string;
    hrefForChannel: (channelID: string) => string;
    onSelectChannel: (channelID: string) => void;
    onCreateChannel: () => void;
  };

  let {
    channels,
    selectedChannelID,
    selectedDirectID,
    hrefForChannel,
    onSelectChannel,
    onCreateChannel,
  }: Props = $props();

  function shouldHandleClientNavigation(event: MouseEvent): boolean {
    return event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
  }
</script>

<section class="nav-section">
  <div class="section-title">
    <span class="caret" aria-hidden="true">▾</span>
    <span class="label">Channels</span>
    <button
      type="button"
      class="add-button"
      aria-label="Create channel"
      title="Create channel"
      onclick={onCreateChannel}
    >＋</button>
  </div>
  <div class="nav-list">
    {#each channels as channel (channel.id)}
      {@const unread = channel.unread_count || 0}
      <a
        href={hrefForChannel(channel.id)}
        class="nav-item channel"
        class:active={channel.id === selectedChannelID && !selectedDirectID}
        class:has-unread={unread > 0 && !(channel.id === selectedChannelID && !selectedDirectID)}
        onclick={(event) => {
          if (!shouldHandleClientNavigation(event)) return;
          event.preventDefault();
          onSelectChannel(channel.id);
        }}
      >
        <span class="hash">#</span> <span class="nav-label">{channel.name}</span>
        {#if unread > 0 && !(channel.id === selectedChannelID && !selectedDirectID)}
          <span class="unread-badge" aria-label={`${unread} unread`}>{unread > 99 ? "99+" : unread}</span>
        {/if}
      </a>
    {/each}
    {#if channels.length === 0}
      <p class="nav-empty">No channels yet</p>
    {/if}
  </div>
</section>
