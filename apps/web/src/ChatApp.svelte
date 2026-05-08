<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import { APIError, api } from "./lib/api";
  import { gifLibrary } from "./lib/gifs";
  import { quoteSnippet, quotedAuthorName } from "./lib/chat/messages";
  import {
    avatarHue,
    avatarInitial,
    collectRecentPeople,
    directConversationForUser,
    dmAvatarUser,
    dmTitle,
    handleLabel,
    workspaceInitial,
  } from "./lib/chat/people";
  import { redirectTypingToComposer } from "./lib/chat/typeToFocus";
  import { markdown, time } from "./lib/format";
  import { uploadURL } from "./lib/uploads";
  import ChatComposer from "./components/composer/ChatComposer.svelte";
  import MediaAttachment from "./components/MediaAttachment.svelte";
  import MessageList from "./components/messages/MessageList.svelte";
  import type { Channel, DirectConversation, Message, RealtimeEvent, SearchResult, ThreadState, Upload, User, Workspace } from "./lib/types";

  let user: User | null = null;
  let workspaces: Workspace[] = [];
  let channels: Channel[] = [];
  let directConversations: DirectConversation[] = [];
  let messages: Message[] = [];
  let replies: Message[] = [];
  let selectedWorkspaceID = "";
  let selectedChannelID = "";
  let selectedDirectID = "";
  let selectedThread: Message | null = null;
  let selectedThreadState: ThreadState | null = null;
  let selectedProfile: User | null = null;
  let selectedImage: { url: string; title: string } | null = null;
  let messageBody = "";
  let replyBody = "";
  let workspaceName = "";
  let channelName = "";
  let directMemberID = "";
  let searchQuery = "";
  let searchResults: SearchResult[] = [];
  let pendingUpload: Upload | null = null;
  let showGifPicker = false;
  let showProfileSettings = false;
  let gifQuery = "";
  let profileDisplayName = "";
  let profileHandle = "";
  let profileAvatarURL = "";
  let profileStatus = "";
  let profileStatusError = false;
  let status = "loading";
  let authRequired = false;
  let socket: WebSocket | null = null;
  let connected = false;
  let reconnectTimer: number | undefined;
  let messageList: HTMLElement | null = null;
  let showWorkspaceCreate = false;
  let sidebarCollapsed = false;
  let mobileNavOpen = false;
  let replyTarget: Message | null = null;
  let replyContext: "channel" | "dm" | "thread" | null = null;
  let messageInput: HTMLTextAreaElement | null = null;
  let replyInput: HTMLTextAreaElement | null = null;
  let activeComposerContext: "message" | "thread" = "message";

  $: selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceID);
  $: selectedChannel = channels.find((channel) => channel.id === selectedChannelID);
  $: selectedDirect = directConversations.find((conversation) => conversation.id === selectedDirectID);
  $: sidePanelOpen = selectedThread !== null || selectedProfile !== null;
  $: recentPeople = collectRecentPeople(messages, directConversations, user?.id || "");
  $: if (replyContext === "channel" && replyTarget && !messages.some((m) => m.id === replyTarget?.id)) clearReplyTarget();
  $: if (replyContext === "dm" && replyTarget && !messages.some((m) => m.id === replyTarget?.id)) clearReplyTarget();
  $: if (replyContext === "thread" && replyTarget && selectedThread && replyTarget.id !== selectedThread.id && !replies.some((r) => r.id === replyTarget?.id)) clearReplyTarget();
  $: filteredGifs = gifLibrary.filter((gif) => {
    const query = gifQuery.trim().toLowerCase();
    return !query || gif.title.toLowerCase().includes(query) || gif.tags.some((tag) => tag.includes(query));
  });

  onMount(() => {
    void boot();
  });

  onDestroy(() => {
    const current = socket;
    socket = null;
    connected = false;
    current?.close();
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
  });

  async function boot() {
    try {
      const me = await api<{ user: User }>("/api/me");
      user = me.user;
      await loadWorkspaces();
      status = "ready";
    } catch (error) {
      if (error instanceof APIError && (error.status === 401 || error.status === 403)) {
        authRequired = true;
        status = "auth";
        return;
      }
      status = error instanceof Error ? error.message : "Could not load ClickClack";
    }
  }

  function openProfileSettings() {
    if (!user) return;
    profileDisplayName = user.display_name;
    profileHandle = user.handle ? `@${user.handle}` : "";
    profileAvatarURL = user.avatar_url;
    profileStatus = "";
    profileStatusError = false;
    showProfileSettings = true;
  }

  async function saveProfile() {
    profileStatus = "";
    profileStatusError = false;
    try {
      const data = await api<{ user: User }>("/api/me", {
        method: "PATCH",
        body: JSON.stringify({
          display_name: profileDisplayName,
          handle: profileHandle,
          avatar_url: profileAvatarURL,
        }),
      });
      user = data.user;
      messages = messages.map((message) =>
        message.author?.id === user?.id ? { ...message, author: data.user } : message,
      );
      replies = replies.map((reply) =>
        reply.author?.id === user?.id ? { ...reply, author: data.user } : reply,
      );
      if (selectedThread?.author?.id === user.id) selectedThread = { ...selectedThread, author: data.user };
      profileStatus = "Saved";
      showProfileSettings = false;
    } catch (error) {
      profileStatus = error instanceof Error ? error.message : "Could not save profile";
      profileStatusError = true;
    }
  }

  async function loadWorkspaces() {
    const data = await api<{ workspaces: Workspace[] }>("/api/workspaces");
    workspaces = data.workspaces;
    selectedWorkspaceID = selectedWorkspaceID || workspaces[0]?.id || "";
    await loadChannels();
    await loadDirectConversations();
    if (workspaces.length === 0) status = "create a workspace";
    connectRealtime();
  }

  async function createWorkspace() {
    if (!workspaceName.trim()) return;
    const data = await api<{ workspace: Workspace }>("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ name: workspaceName })
    });
    workspaceName = "";
    showWorkspaceCreate = false;
    workspaces = [...workspaces, data.workspace];
    selectedWorkspaceID = data.workspace.id;
    await loadChannels();
    await loadDirectConversations();
    connectRealtime();
  }

  async function loadChannels() {
    if (!selectedWorkspaceID) return;
    const data = await api<{ channels: Channel[] }>(`/api/workspaces/${selectedWorkspaceID}/channels`);
    channels = data.channels;
    selectedChannelID = channels.find((channel) => channel.id === selectedChannelID)?.id || channels[0]?.id || "";
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    replies = [];
    await loadMessages();
  }

  async function createChannel() {
    if (!selectedWorkspaceID || !channelName.trim()) return;
    const data = await api<{ channel: Channel }>(`/api/workspaces/${selectedWorkspaceID}/channels`, {
      method: "POST",
      body: JSON.stringify({ name: channelName, kind: "public" })
    });
    channelName = "";
    channels = [...channels, data.channel];
    selectedChannelID = data.channel.id;
    selectedDirectID = "";
    await loadMessages();
  }

  async function loadMessages() {
    if (selectedDirectID) {
      const data = await api<{ messages: Message[] }>(`/api/dms/${selectedDirectID}/messages`);
      messages = data.messages;
      await scrollMessagesToBottom();
      return;
    }
    if (!selectedChannelID) {
      messages = [];
      return;
    }
    const data = await api<{ messages: Message[] }>(`/api/channels/${selectedChannelID}/messages`);
    messages = data.messages;
    await scrollMessagesToBottom();
  }

  async function scrollMessagesToBottom() {
    await tick();
    if (messageList) messageList.scrollTop = messageList.scrollHeight;
  }

  async function sendMessage() {
    const body = messageBody.trim();
    if (!body) return;
    if (!selectedChannelID && !selectedDirectID) {
      status = "pick or create a channel";
      return;
    }
    const activeContext: "channel" | "dm" = selectedDirectID ? "dm" : "channel";
    const quote = replyTarget && replyContext === activeContext ? replyTarget : null;
    messageBody = "";
    const path = selectedDirectID ? `/api/dms/${selectedDirectID}/messages` : `/api/channels/${selectedChannelID}/messages`;
    const payload: Record<string, unknown> = { body };
    if (quote) payload.quoted_message_id = quote.id;
    const data = await api<{ message: Message }>(path, {
      method: "POST",
      body: JSON.stringify(payload)
    });
    let message = data.message;
    if (quote) clearReplyTarget();
    if (pendingUpload) {
      const upload = pendingUpload;
      await api(`/api/messages/${data.message.id}/attachments`, {
        method: "POST",
        body: JSON.stringify({ upload_id: upload.id })
      });
      pendingUpload = null;
      message = { ...message, attachments: [...(message.attachments || []), upload] };
    }
    if (messages.some((existing) => existing.id === message.id)) {
      messages = messages.map((existing) => (existing.id === message.id ? message : existing));
    } else {
      messages = [...messages, message];
    }
    await scrollMessagesToBottom();
  }

  async function openThread(message: Message) {
    selectedProfile = null;
    selectedThread = message;
    activeComposerContext = "thread";
    const data = await api<{ root: Message; replies: Message[]; thread_state: ThreadState }>(`/api/messages/${message.id}/thread`);
    selectedThread = data.root;
    replies = data.replies;
    selectedThreadState = data.thread_state;
  }

  async function sendReply() {
    const body = replyBody.trim();
    if (!body || !selectedThread) return;
    const quote = replyTarget && replyContext === "thread" ? replyTarget : null;
    replyBody = "";
    const payload: Record<string, unknown> = { body };
    if (quote) payload.quoted_message_id = quote.id;
    const data = await api<{ message: Message; thread_state: ThreadState }>(`/api/messages/${selectedThread.id}/thread/replies`, {
      method: "POST",
      body: JSON.stringify(payload)
    });
    if (quote) clearReplyTarget();
    if (!replies.some((reply) => reply.id === data.message.id)) {
      replies = [...replies, data.message];
    }
    selectedThreadState = data.thread_state;
  }

  function setReplyTarget(message: Message, context: "channel" | "dm" | "thread") {
    replyTarget = message;
    replyContext = context;
    activeComposerContext = context === "thread" ? "thread" : "message";
  }

  function isModalOpen(): boolean {
    return selectedImage !== null || showProfileSettings;
  }

  function activeComposerTarget(): HTMLTextAreaElement | null {
    if (activeComposerContext === "thread" && selectedThread && replyInput) return replyInput;
    return messageInput;
  }

  function clearReplyTarget() {
    replyTarget = null;
    replyContext = null;
  }

  async function jumpToQuotedMessage(message: Message) {
    const targetID = message.quoted_message_id;
    if (!targetID) return;
    await tick();
    const node = document.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(targetID)}"]`);
    if (!node) return;
    node.scrollIntoView({ behavior: "smooth", block: "center" });
    node.classList.add("highlight");
    window.setTimeout(() => node.classList.remove("highlight"), 1500);
  }

  async function searchMessages() {
    if (!selectedWorkspaceID || !searchQuery.trim()) {
      searchResults = [];
      return;
    }
    const data = await api<{ results: SearchResult[] }>(
      `/api/search?workspace_id=${encodeURIComponent(selectedWorkspaceID)}&q=${encodeURIComponent(searchQuery.trim())}`
    );
    searchResults = data.results;
  }

  async function uploadFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file || !selectedWorkspaceID) return;
    const form = new FormData();
    form.set("workspace_id", selectedWorkspaceID);
    form.set("file", file);
    const data = await api<{ upload: Upload }>("/api/uploads", { method: "POST", body: form });
    pendingUpload = data.upload;
    input.value = "";
  }

  async function loadDirectConversations() {
    if (!selectedWorkspaceID) return;
    const data = await api<{ conversations: DirectConversation[] }>(`/api/dms?workspace_id=${selectedWorkspaceID}`);
    directConversations = data.conversations;
  }

  async function createDirectConversation() {
    if (!selectedWorkspaceID || !directMemberID.trim()) return;
    const data = await api<{ conversation: DirectConversation }>("/api/dms", {
      method: "POST",
      body: JSON.stringify({ workspace_id: selectedWorkspaceID, member_ids: [directMemberID.trim()] })
    });
    directMemberID = "";
    directConversations = [...directConversations, data.conversation];
    selectedDirectID = data.conversation.id;
    selectedChannelID = "";
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    await loadMessages();
  }

  async function startDirectWithUser(memberID: string) {
    if (!selectedWorkspaceID || !memberID) return;
    const existing = directConversations.find((conversation) =>
      conversation.members.some((member) => member.id === memberID),
    );
    if (existing) {
      selectedDirectID = existing.id;
      selectedChannelID = "";
      selectedThread = null;
      selectedProfile = null;
      activeComposerContext = "message";
      await loadMessages();
      return;
    }
    const data = await api<{ conversation: DirectConversation }>("/api/dms", {
      method: "POST",
      body: JSON.stringify({ workspace_id: selectedWorkspaceID, member_ids: [memberID] })
    });
    directConversations = [...directConversations, data.conversation];
    selectedDirectID = data.conversation.id;
    selectedChannelID = "";
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    await loadMessages();
  }

  function connectRealtime() {
    if (reconnectTimer) window.clearTimeout(reconnectTimer);
    const previous = socket;
    socket = null;
    connected = false;
    previous?.close();
    if (!selectedWorkspaceID) return;
    const lastCursor = localStorage.getItem(`clickclack:${selectedWorkspaceID}:cursor`) || "";
    const url = new URL("/api/realtime/ws", window.location.href);
    url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("workspace_id", selectedWorkspaceID);
    if (lastCursor) url.searchParams.set("after_cursor", lastCursor);
    const current = new WebSocket(url);
    socket = current;
    current.addEventListener("open", () => {
      if (socket === current) connected = true;
    });
    current.addEventListener("message", (message) => {
      const event = JSON.parse(String(message.data)) as RealtimeEvent;
      if (event.cursor) localStorage.setItem(`clickclack:${selectedWorkspaceID}:cursor`, event.cursor);
      void handleEvent(event);
    });
    current.addEventListener("close", () => {
      if (socket !== current) return;
      connected = false;
      reconnectTimer = window.setTimeout(connectRealtime, 1200);
    });
  }

  async function handleEvent(event: RealtimeEvent) {
    if ((event.type === "channel.created" || event.type === "channel.updated") && event.workspace_id === selectedWorkspaceID) {
      await loadChannels();
      return;
    }
    if (
      (event.channel_id === selectedChannelID || event.payload.direct_conversation_id === selectedDirectID) &&
      (event.type === "message.created" || event.type === "message.updated" || event.type === "message.deleted")
    ) {
      await loadMessages();
    }
    const rootID = event.payload.root_message_id || event.payload.message_id;
    if (selectedThread && rootID === selectedThread.id) {
      await openThread(selectedThread);
    }
  }

  function openUserProfile(profile?: User | null) {
    if (!profile) return;
    selectedThread = null;
    selectedProfile = profile;
  }

  function handleComposerKey(event: KeyboardEvent) {
    if (event.key === "Escape" && replyTarget && replyContext !== "thread") {
      event.preventDefault();
      clearReplyTarget();
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  }

  function handleReplyKey(event: KeyboardEvent) {
    if (event.key === "Escape" && replyTarget && replyContext === "thread") {
      event.preventDefault();
      clearReplyTarget();
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendReply();
    }
  }

  function openImageViewer(url: string, title: string) {
    selectedImage = { url, title };
  }

  function handleInlineImagePointerUp(event: PointerEvent) {
    const target = event.target;
    if (!(target instanceof HTMLImageElement)) return;
    if (!target.closest(".markdown")) return;
    event.preventDefault();
    openImageViewer(target.currentSrc || target.src, target.alt || "Image");
  }

  function appendToComposer(snippet: string) {
    const prefix = messageBody && !messageBody.endsWith("\n") ? "\n" : "";
    messageBody = `${messageBody}${prefix}${snippet}`;
  }

  function applyMarkdownWrap(before: string, after = before) {
    const placeholder = before === "```" ? "\ncode\n" : "text";
    appendToComposer(`${before}${placeholder}${after}`);
  }

  function pickGif(url: string, title: string) {
    appendToComposer(`![${title}](${url})`);
    showGifPicker = false;
    gifQuery = "";
  }

  function closeSidePanel() {
    if (replyContext === "thread") clearReplyTarget();
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    replies = [];
  }

  function closeModal() {
    selectedImage = null;
    showProfileSettings = false;
  }
</script>

<svelte:head>
  <meta name="color-scheme" content="light dark" />
</svelte:head>

<svelte:window
  onkeydown={(event) => {
    if (event.key === "Escape") {
      if (isModalOpen()) {
        closeModal();
      } else if (replyTarget) {
        event.preventDefault();
        clearReplyTarget();
        return;
      }
    }
    redirectTypingToComposer(event, {
      authRequired,
      isModalOpen,
      messageInput,
      replyInput,
      target: activeComposerTarget,
    });
  }}
/>

{#if authRequired}
  <main class="auth-shell">
    <section class="auth-panel" aria-label="Sign in">
      <div class="auth-brand">
        <div class="mark">cc</div>
        <div class="brand-text">
          <strong>ClickClack</strong>
          <span>OpenClaw workspace chat</span>
        </div>
      </div>
      <div class="auth-copy">
        <h1>Welcome back.</h1>
        <p>Workspace chat for the OpenClaw crew. Sign in with the GitHub account that's a member of the org.</p>
      </div>
      <a class="github-login" href="/api/auth/github/start">
        <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
          <path fill="currentColor" d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56v-2c-3.2.69-3.87-1.37-3.87-1.37-.52-1.32-1.27-1.67-1.27-1.67-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.02 1.75 2.68 1.25 3.34.96.1-.74.4-1.25.73-1.54-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.29 1.18-3.1-.12-.29-.51-1.46.11-3.05 0 0 .96-.31 3.15 1.18a10.94 10.94 0 0 1 5.74 0c2.19-1.49 3.15-1.18 3.15-1.18.62 1.59.23 2.76.12 3.05.74.81 1.18 1.84 1.18 3.1 0 4.42-2.69 5.39-5.25 5.68.41.36.78 1.06.78 2.13v3.16c0 .31.21.67.8.56 4.56-1.52 7.85-5.83 7.85-10.91C23.5 5.65 18.35.5 12 .5z"/>
        </svg>
        Continue with GitHub
      </a>
      <p class="auth-foot">Limited to active members of the OpenClaw org.</p>
    </section>
  </main>
{:else}
<div
  class="shell"
  class:nav-open={mobileNavOpen}
  class:sidebar-collapsed={sidebarCollapsed}
  class:thread-open={sidePanelOpen}
>
  <button
    class="mobile-nav-toggle"
    type="button"
    aria-label="Toggle navigation"
    onclick={() => (mobileNavOpen = !mobileNavOpen)}
  >
    {#if mobileNavOpen}&times;{:else}<span class="bars"><i></i><i></i><i></i></span>{/if}
  </button>

  <nav class="guild-rail" aria-label="Workspaces">
    <a class="guild home" title="ClickClack home" href="/">
      <span>cc</span>
    </a>
    <div class="guild-divider" aria-hidden="true"></div>
    <div class="guild-list">
      {#each workspaces as workspace (workspace.id)}
        <div class="guild-wrap" class:active={workspace.id === selectedWorkspaceID}>
          <button
            class="guild"
            title={workspace.name}
            onclick={async () => {
              selectedWorkspaceID = workspace.id;
              await loadChannels();
              await loadDirectConversations();
              connectRealtime();
            }}
          >
            <span>{workspaceInitial(workspace.name)}</span>
          </button>
        </div>
      {/each}
      <button
        class="guild add"
        title="Create workspace"
        aria-label="Create workspace"
        onclick={() => (showWorkspaceCreate = !showWorkspaceCreate)}
      >+</button>
    </div>
    {#if showWorkspaceCreate}
      <form
        class="guild-create"
        onsubmit={(event) => {
          event.preventDefault();
          void createWorkspace();
        }}
      >
        <input bind:value={workspaceName} placeholder="Workspace name" aria-label="Workspace name" />
      </form>
    {/if}
  </nav>

  <aside class="sidebar" aria-label="Channels and DMs">
    <header class="workspace-header">
      <div class="workspace-name">
        <strong>{selectedWorkspace?.name || "Pick a workspace"}</strong>
        <span class="presence" class:online={connected}>{connected ? "Connected" : status}</span>
      </div>
      <button
        type="button"
        class="sidebar-collapse"
        aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
        title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
        onclick={() => (sidebarCollapsed = !sidebarCollapsed)}
      >
        <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
          <path
            fill="none"
            stroke="currentColor"
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d={sidebarCollapsed ? "m9 6 6 6-6 6" : "m15 6-6 6 6 6"}
          />
        </svg>
      </button>
    </header>

    <div class="sidebar-scroll">
      <section class="nav-section">
        <div class="section-title">
          <span class="caret" aria-hidden="true">▾</span>
          <span class="label">Channels</span>
        </div>
        <div class="nav-list">
          {#each channels as channel (channel.id)}
            <button
              class="nav-item channel"
              class:active={channel.id === selectedChannelID && !selectedDirectID}
              onclick={async () => {
                selectedChannelID = channel.id;
                selectedDirectID = "";
                selectedThread = null;
                selectedProfile = null;
                activeComposerContext = "message";
                mobileNavOpen = false;
                await loadMessages();
              }}
            >
              <span class="hash">#</span> <span class="nav-label">{channel.name}</span>
            </button>
          {/each}
          {#if channels.length === 0}
            <p class="nav-empty">No channels yet</p>
          {/if}
        </div>
        <form
          class="inline-create"
          onsubmit={(event) => {
            event.preventDefault();
            void createChannel();
          }}
        >
          <input bind:value={channelName} placeholder="add-channel" aria-label="New channel name" />
          <button type="submit" class="ghost" aria-label="Create channel">＋</button>
        </form>
      </section>

      <section class="nav-section">
        <div class="section-title">
          <span class="caret" aria-hidden="true">▾</span>
          <span class="label">Direct messages</span>
        </div>
        <div class="nav-list">
          {#each directConversations as conversation (conversation.id)}
            {@const dmUser = dmAvatarUser(conversation, user?.id)}
            <button
              class="nav-item dm"
              class:active={conversation.id === selectedDirectID}
              onclick={async () => {
                selectedDirectID = conversation.id;
                selectedChannelID = "";
                selectedThread = null;
                selectedProfile = null;
                activeComposerContext = "message";
                mobileNavOpen = false;
                await loadMessages();
              }}
            >
              <span class="dm-avatar" style="--hue: {avatarHue(dmUser?.id || conversation.id)}deg">
                {#if dmUser?.avatar_url}
                  <img src={dmUser.avatar_url} alt="" loading="lazy" />
                {:else}
                  {avatarInitial(dmUser?.display_name)}
                {/if}
              </span>
              <span class="nav-label">{dmTitle(conversation, user?.id)}</span>
              <span class="presence-dot" aria-hidden="true"></span>
            </button>
          {/each}
          {#if directConversations.length === 0}
            <p class="nav-empty">No direct messages yet</p>
          {/if}
        </div>
        <form
          class="inline-create"
          onsubmit={(event) => {
            event.preventDefault();
            void createDirectConversation();
          }}
        >
          <input bind:value={directMemberID} placeholder="user id" aria-label="DM member user ID" />
          <button type="submit" class="ghost" aria-label="Start DM">＋</button>
        </form>
      </section>

      <section class="nav-section">
        <div class="section-title">
          <span class="caret" aria-hidden="true">▾</span>
          <span class="label">People</span>
        </div>
        <div class="nav-list">
          {#each recentPeople as person (person.id)}
            {@const conversation = directConversationForUser(directConversations, person.id)}
            <button
              class="nav-item dm"
              class:active={conversation?.id === selectedDirectID || selectedProfile?.id === person.id}
              onclick={async () => {
                if (conversation) {
                  selectedDirectID = conversation.id;
                  selectedChannelID = "";
                  selectedThread = null;
                  selectedProfile = null;
                  activeComposerContext = "message";
                  mobileNavOpen = false;
                  await loadMessages();
                } else {
                  openUserProfile(person);
                }
              }}
            >
              <span class="dm-avatar" style="--hue: {avatarHue(person.id)}deg">
                {#if person.avatar_url}
                  <img src={person.avatar_url} alt="" loading="lazy" />
                {:else}
                  {avatarInitial(person.display_name)}
                {/if}
              </span>
              <span class="nav-label">{person.display_name}</span>
              <span class="presence-dot active" aria-hidden="true"></span>
            </button>
          {/each}
          {#if recentPeople.length === 0}
            <p class="nav-empty">People appear here as you chat</p>
          {/if}
        </div>
      </section>
    </div>

    {#if user}
      <button
        class="user-card"
        type="button"
        onclick={openProfileSettings}
        oncontextmenu={(event) => {
          event.preventDefault();
          openProfileSettings();
        }}
        aria-label={`Account settings for ${user.display_name} ${handleLabel(user.handle)}`}
      >
        <span class="dm-avatar" style="--hue: {avatarHue(user.id)}deg">
          {#if user.avatar_url}
            <img src={user.avatar_url} alt="" loading="lazy" />
          {:else}
            {avatarInitial(user.display_name)}
          {/if}
        </span>
        <div class="user-meta">
          <strong>{user.display_name}</strong>
          <span>{user.handle ? handleLabel(user.handle) : connected ? "Active" : "Reconnecting…"}</span>
        </div>
        <span class="presence-dot active" aria-hidden="true"></span>
      </button>
    {/if}
  </aside>

  <main class="timeline">
    <header class="topbar">
      <div class="topbar-title">
        {#if selectedDirect}
          <h1 class="with-glyph dm">{`@${dmTitle(selectedDirect, user?.id)}`}</h1>
        {:else if selectedChannel}
          <h1 class="with-glyph channel">{`#${selectedChannel.name}`}</h1>
        {:else}
          <h1 class="with-glyph">ClickClack</h1>
        {/if}
        <span class="topbar-divider" aria-hidden="true"></span>
        <p class="topbar-meta">{selectedWorkspace?.name || "no workspace"}</p>
      </div>
      <form
        class="search"
        onsubmit={(event) => {
          event.preventDefault();
          void searchMessages();
        }}
      >
        <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
          <circle cx="11" cy="11" r="7" fill="none" stroke="currentColor" stroke-width="2"/>
          <path d="m20 20-3.5-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
        <input bind:value={searchQuery} placeholder="Search messages" aria-label="Search messages" />
        {#if searchQuery}
          <button
            type="button"
            class="search-clear"
            aria-label="Reset"
            onclick={() => {
              searchQuery = "";
              searchResults = [];
            }}
          >×</button>
        {/if}
        <button type="submit" class="search-submit">Search</button>
      </form>
      <div class="topbar-actions" aria-label="Channel tools">
        <button
          type="button"
          title={selectedThread ? "Close thread" : "Open a message thread"}
          aria-label={selectedThread ? "Close thread" : "Open a message thread"}
          class:active={sidePanelOpen}
          onclick={() => {
            if (sidePanelOpen) closeSidePanel();
            else status = "pick a message to open its thread";
          }}
        >
          <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M21 12a8 8 0 0 1-11.6 7.16L3 21l1.84-6.4A8 8 0 1 1 21 12Z"/>
          </svg>
        </button>
        <button type="button" title="Pinned items" aria-label="Pinned items" onclick={() => (status = "no pinned items")}>
          <svg viewBox="0 0 24 24" width="15" height="15" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="m14 4 6 6-4 4v5l-2 2-5-5-4 4-1-1 4-4-5-5 2-2h5l4-4Z"/>
          </svg>
        </button>
      </div>
    </header>

    {#if searchResults.length > 0}
      <div class="search-results" aria-label="Search results">
        <div class="search-results-head">
          <strong>{searchResults.length} {searchResults.length === 1 ? "result" : "results"}</strong>
          <button
            type="button"
            onclick={() => {
              searchResults = [];
            }}
          >Close</button>
        </div>
        {#each searchResults as result (result.message.id)}
          <button
            class="search-result"
            onclick={async () => {
              searchResults = [];
              if (result.message.channel_id) {
                selectedChannelID = result.message.channel_id;
                selectedDirectID = "";
                await loadMessages();
              }
              if (result.message.direct_conversation_id) {
                selectedDirectID = result.message.direct_conversation_id;
                selectedChannelID = "";
                await loadMessages();
              }
            }}
          >
            <span class="dm-avatar" style="--hue: {avatarHue(result.message.author?.id || result.message.author_id || 'x')}deg">
              {#if result.message.author?.avatar_url}
                <img src={result.message.author.avatar_url} alt="" loading="lazy" />
              {:else}
                {avatarInitial(result.message.author?.display_name)}
              {/if}
            </span>
            <div class="search-result-body">
              <div>
                <strong>{result.message.author?.display_name || "Local User"}</strong>
                <time>{time(result.message.created_at)}</time>
              </div>
              <span>{result.message.body}</span>
            </div>
          </button>
        {/each}
      </div>
    {/if}

    <MessageList
      {messages}
      {selectedDirect}
      {selectedChannel}
      selectedThreadID={selectedThread?.id}
      currentUserID={user?.id}
      onListRef={(node) => (messageList = node)}
      onActivateMessageComposer={() => (activeComposerContext = "message")}
      onInlineImagePointerUp={handleInlineImagePointerUp}
      onOpenProfile={openUserProfile}
      onReply={setReplyTarget}
      onOpenThread={openThread}
      onJumpToQuote={(message) => void jumpToQuotedMessage(message)}
      onOpenImage={openImageViewer}
    />

    <ChatComposer
      value={messageBody}
      placeholder={selectedDirect ? `Message ${dmTitle(selectedDirect, user?.id)}` : selectedChannel ? `Message #${selectedChannel.name}` : "Pick a channel to start"}
      ariaLabel="Message body"
      submitLabel="Send"
      pendingUpload={pendingUpload}
      replyTarget={replyTarget && replyContext === (selectedDirectID ? "dm" : "channel") ? replyTarget : null}
      showUpload
      showToolbar
      showGifPicker={showGifPicker}
      gifQuery={gifQuery}
      filteredGifs={filteredGifs}
      onValue={(value) => (messageBody = value)}
      onSubmit={() => void sendMessage()}
      onKeydown={handleComposerKey}
      onFocus={() => (activeComposerContext = "message")}
      onInputRef={(node) => (messageInput = node)}
      onUploadFile={uploadFile}
      onRemoveUpload={() => (pendingUpload = null)}
      onClearReply={clearReplyTarget}
      onApplyMarkdownWrap={applyMarkdownWrap}
      onAppendToComposer={appendToComposer}
      onToggleGif={() => (showGifPicker = !showGifPicker)}
      onGifQuery={(value) => (gifQuery = value)}
      onPickGif={pickGif}
    />
  </main>

  <aside class="thread" class:open={sidePanelOpen} aria-label={selectedProfile ? "Profile pane" : "Thread pane"}>
    {#if selectedThread}
      <header>
        <div>
          <p>Thread</p>
          <strong>{selectedThreadState?.reply_count ?? replies.length} {(selectedThreadState?.reply_count ?? replies.length) === 1 ? "reply" : "replies"}</strong>
        </div>
        <button
          class="close"
          aria-label="Close thread"
          onclick={() => {
            closeSidePanel();
          }}
        >×</button>
      </header>
      <div
        class="thread-scroll"
        role="region"
        aria-label="Thread messages"
        onpointerdown={() => (activeComposerContext = "thread")}
        onpointerup={handleInlineImagePointerUp}
      >
        <article class="thread-root" data-message-id={selectedThread.id}>
          <div class="avatar" style="--hue: {avatarHue(selectedThread.author?.id || selectedThread.author_id || 'x')}deg">
            {#if selectedThread.author?.avatar_url}
              <img src={selectedThread.author.avatar_url} alt="" loading="lazy" />
            {:else}
              {avatarInitial(selectedThread.author?.display_name)}
            {/if}
          </div>
          <div class="group-body">
            <header>
              <strong>{selectedThread.author?.display_name || "Local User"}</strong>
              {#if selectedThread.author?.handle}<span>{handleLabel(selectedThread.author.handle)}</span>{/if}
              <time>{time(selectedThread.created_at)}</time>
              <button
                type="button"
	                class="reply-quote-btn"
	                aria-label="Reply"
	                data-tooltip="Reply"
	                onclick={() => selectedThread && setReplyTarget(selectedThread, "thread")}
	              >↩</button>
            </header>
            <div class="markdown">{@html markdown(selectedThread.body)}</div>
            {#if selectedThread.attachments?.length}
              <div class="attachment-grid compact" aria-label="Attachments">
                {#each selectedThread.attachments as attachment (attachment.id)}
                  <MediaAttachment
                    upload={attachment}
                    url={uploadURL(attachment)}
                    onOpenImage={openImageViewer}
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
	                    onclick={() => setReplyTarget(reply, "thread")}
	                  >↩</button>
                </header>
                {#if reply.quoted_message_id || reply.quoted_body_snapshot}
                  <button
                    type="button"
                    class="quote-block"
                    class:dangling={!reply.quoted_message_id}
                    onclick={() => jumpToQuotedMessage(reply)}
                    disabled={!reply.quoted_message_id}
                    aria-label={reply.quoted_message_id ? `Jump to quoted message from ${quotedAuthorName(reply)}` : "Original message was deleted"}
                  >
                    <span class="quote-bar" aria-hidden="true"></span>
                    <span class="quote-content">
                      <span class="quote-author">{quotedAuthorName(reply)}</span>
                      {#if reply.quoted_message_id}
                        <span class="quote-snippet">{quoteSnippet(reply.quoted_body_snapshot)}</span>
                      {:else}
                        <span class="quote-snippet muted">[original deleted] {quoteSnippet(reply.quoted_body_snapshot)}</span>
                      {/if}
                    </span>
                  </button>
                {/if}
                <div class="markdown">{@html markdown(reply.body)}</div>
                {#if reply.attachments?.length}
                  <div class="attachment-grid compact" aria-label="Attachments">
                    {#each reply.attachments as attachment (attachment.id)}
                      <MediaAttachment
                        upload={attachment}
                        url={uploadURL(attachment)}
                        onOpenImage={openImageViewer}
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
        replyTarget={replyTarget && replyContext === "thread" ? replyTarget : null}
        onValue={(value) => (replyBody = value)}
        onSubmit={() => void sendReply()}
        onKeydown={handleReplyKey}
        onFocus={() => (activeComposerContext = "thread")}
        onInputRef={(node) => (replyInput = node)}
        onClearReply={clearReplyTarget}
      />
    {:else if selectedProfile}
      <header>
        <div>
          <p>Profile</p>
          <strong>{selectedProfile.display_name}</strong>
        </div>
        <button class="close" aria-label="Close profile" onclick={closeSidePanel}>×</button>
      </header>
      <div class="profile-pane">
        <div class="profile-hero" style="--hue: {avatarHue(selectedProfile.id)}deg">
          <span class="profile-avatar">
            {#if selectedProfile.avatar_url}
              <img src={selectedProfile.avatar_url} alt="" loading="lazy" />
            {:else}
              {avatarInitial(selectedProfile.display_name)}
            {/if}
          </span>
        </div>
        <section class="profile-pane-body">
          <div class="profile-pane-title">
            <div>
              <h2>{selectedProfile.display_name}</h2>
              {#if selectedProfile.handle}<span>{handleLabel(selectedProfile.handle)}</span>{/if}
            </div>
            {#if user?.id === selectedProfile.id}
              <button type="button" class="text-action" onclick={openProfileSettings}>Edit</button>
            {/if}
          </div>
          <div class="profile-presence">
            <span class="presence-dot active" aria-hidden="true"></span>
            <span>Active</span>
          </div>
          <div class="profile-actions-row">
            {#if user?.id !== selectedProfile.id}
              <button type="button" class="primary-action" onclick={() => startDirectWithUser(selectedProfile?.id || "")}>
                Message
              </button>
            {/if}
            <button type="button" class="ghost-action" onclick={() => (status = "status messages are coming soon")}>
              Set a status
            </button>
          </div>
          <section class="profile-info">
            <header>
              <strong>Contact information</strong>
              {#if user?.id === selectedProfile.id}
                <button type="button" class="text-action" onclick={openProfileSettings}>Edit</button>
              {/if}
            </header>
            <div class="profile-info-row">
              <span class="info-icon" aria-hidden="true">@</span>
              <div>
                <small>Handle</small>
                <span>{selectedProfile.handle ? handleLabel(selectedProfile.handle) : "No handle set"}</span>
              </div>
            </div>
            <div class="profile-info-row">
              <span class="info-icon" aria-hidden="true">ID</span>
              <div>
                <small>User ID</small>
                <span>{selectedProfile.id}</span>
              </div>
            </div>
          </section>
          <section class="profile-info">
            <header>
              <strong>About</strong>
            </header>
            <p class="profile-note">Member of {selectedWorkspace?.name || "this workspace"}. Click Message to keep the conversation in your sidebar.</p>
          </section>
        </section>
      </div>
    {:else}
      <div class="thread-empty">
        <div class="thread-icon">
          <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M21 12a8 8 0 0 1-11.6 7.16L3 21l1.84-6.4A8 8 0 1 1 21 12Z"/>
          </svg>
        </div>
        <strong>No thread open</strong>
        <span>Hover any message and tap the bubble to keep side conversations tidy.</span>
      </div>
    {/if}
  </aside>
</div>
{#if showProfileSettings && user}
  <div class="modal-scrim" role="presentation">
    <button class="modal-backdrop" type="button" aria-label="Close account settings" onclick={closeModal}></button>
    <section class="profile-modal" aria-label="Account settings">
      <header>
        <div>
          <p>Account</p>
          <h2>Profile settings</h2>
        </div>
        <button type="button" aria-label="Close account settings" onclick={closeModal}>×</button>
      </header>
      <form
        class="profile-form"
        onsubmit={(event) => {
          event.preventDefault();
          void saveProfile();
        }}
      >
        <div class="profile-preview">
          <span class="avatar large" style="--hue: {avatarHue(user.id)}deg">
            {#if profileAvatarURL}
              <img src={profileAvatarURL} alt="" loading="lazy" />
            {:else}
              {avatarInitial(profileDisplayName)}
            {/if}
          </span>
          <div>
            <strong>{profileDisplayName || user.display_name}</strong>
            <span>{profileHandle || handleLabel(user.handle) || "No handle set"}</span>
          </div>
        </div>
        <label class="field">
          <span>Display name</span>
          <input bind:value={profileDisplayName} aria-label="Display name" maxlength="80" autocomplete="name" />
        </label>
        <label class="field">
          <span>Handle</span>
          <input bind:value={profileHandle} aria-label="Handle" placeholder="@steipete" autocomplete="username" />
        </label>
        <label class="field">
          <span>Avatar URL</span>
          <input bind:value={profileAvatarURL} aria-label="Avatar URL" placeholder="https://example.com/avatar.png" inputmode="url" />
        </label>
        {#if profileStatus}<p class="profile-status" class:error={profileStatusError}>{profileStatus}</p>{/if}
        <div class="profile-actions">
          <button type="button" class="ghost-action" onclick={closeModal}>Cancel</button>
          <button type="submit" class="primary-action">Save profile</button>
        </div>
      </form>
    </section>
  </div>
{/if}
{#if selectedImage}
  <div class="modal-scrim image-viewer-scrim" role="presentation">
    <button class="modal-backdrop" type="button" aria-label="Close image viewer" onclick={closeModal}></button>
    <section class="image-viewer" aria-label="Image viewer">
      <header>
        <strong>{selectedImage.title}</strong>
        <div>
          <a href={selectedImage.url} target="_blank" rel="noreferrer">Open original</a>
          <button type="button" aria-label="Close image viewer" onclick={closeModal}>×</button>
        </div>
      </header>
      <div class="image-viewer-stage">
        <img src={selectedImage.url} alt={selectedImage.title} />
      </div>
    </section>
  </div>
{/if}
{/if}
