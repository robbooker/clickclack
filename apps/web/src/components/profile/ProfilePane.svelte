<script lang="ts">
  import { avatarHue, avatarInitial, handleLabel } from "../../lib/chat/people";
  import type { User } from "../../lib/types";

  type Props = {
    profile: User;
    currentUser: User | null;
    workspaceName?: string;
    onClose: () => void;
    onEdit: () => void;
    onMessage: (memberID: string) => void;
    onSetStatus: () => void;
  };

  let { profile, currentUser, workspaceName, onClose, onEdit, onMessage, onSetStatus }: Props = $props();
</script>

<header>
  <div>
    <p>Profile</p>
    <strong>{profile.display_name}</strong>
  </div>
  <button class="close" aria-label="Close profile" onclick={onClose}>×</button>
</header>
<div class="profile-pane">
  <div class="profile-hero" style="--hue: {avatarHue(profile.id)}deg">
    <span class="profile-avatar">
      {#if profile.avatar_url}
        <img src={profile.avatar_url} alt="" loading="lazy" />
      {:else}
        {avatarInitial(profile.display_name)}
      {/if}
    </span>
  </div>
  <section class="profile-pane-body">
    <div class="profile-pane-title">
      <div>
        <h2>{profile.display_name}</h2>
        {#if profile.handle}<span>{handleLabel(profile.handle)}</span>{/if}
      </div>
      {#if currentUser?.id === profile.id}
        <button type="button" class="text-action" onclick={onEdit}>Edit</button>
      {/if}
    </div>
    <div class="profile-presence">
      <span class="presence-dot active" aria-hidden="true"></span>
      <span>Active</span>
    </div>
    <div class="profile-actions-row">
      {#if currentUser?.id !== profile.id}
        <button type="button" class="primary-action" onclick={() => onMessage(profile.id)}>
          Message
        </button>
      {/if}
      <button type="button" class="ghost-action" onclick={onSetStatus}>
        Set a status
      </button>
    </div>
    <section class="profile-info">
      <header>
        <strong>Contact information</strong>
        {#if currentUser?.id === profile.id}
          <button type="button" class="text-action" onclick={onEdit}>Edit</button>
        {/if}
      </header>
      <div class="profile-info-row">
        <span class="info-icon" aria-hidden="true">@</span>
        <div>
          <small>Handle</small>
          <span>{profile.handle ? handleLabel(profile.handle) : "No handle set"}</span>
        </div>
      </div>
      <div class="profile-info-row">
        <span class="info-icon" aria-hidden="true">ID</span>
        <div>
          <small>User ID</small>
          <span>{profile.id}</span>
        </div>
      </div>
    </section>
    <section class="profile-info">
      <header>
        <strong>About</strong>
      </header>
      <p class="profile-note">Member of {workspaceName || "this workspace"}. Click Message to keep the conversation in your sidebar.</p>
    </section>
  </section>
</div>
