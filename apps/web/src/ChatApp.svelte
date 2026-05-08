<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import { APIError, api } from "./lib/api";
  import { markdown, time } from "./lib/format";
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

  $: selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceID);
  $: selectedChannel = channels.find((channel) => channel.id === selectedChannelID);
  $: selectedDirect = directConversations.find((conversation) => conversation.id === selectedDirectID);
  $: groupedMessages = groupMessages(messages);
  $: filteredGifs = gifLibrary.filter((gif) => {
    const query = gifQuery.trim().toLowerCase();
    return !query || gif.title.toLowerCase().includes(query) || gif.tags.some((tag) => tag.includes(query));
  });

  const gifLibrary = [
    {
      title: "Ship it",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExYjJ1bm1meHE4N2x3bnN0djJkMWtjNGc5bXYzZDFiOHBsbG16M3F0ZSZlcD12MV9naWZzX3NlYXJjaCZjdD1n/l0HlHFRbmaZtBRhXG/giphy.gif",
      tags: ["ship", "launch", "done"],
    },
    {
      title: "Approved",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExazBpbzJ6ODZ3bXQ3OHBvNGJidWZoajc0cHV6YnVub3MzZ3c1a2Z2dSZlcD12MV9naWZzX3NlYXJjaCZjdD1n/111ebonMs90YLu/giphy.gif",
      tags: ["yes", "approved", "nice"],
    },
    {
      title: "Deploy dance",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExY3NkaTVmZW9ydWNnZnl0ZWQ5aHQyeGNrd2k3NG4wZWNqYzNmd3k1ZCZlcD12MV9naWZzX3NlYXJjaCZjdD1n/GeimqsH0TLDt4tScGw/giphy.gif",
      tags: ["deploy", "dance", "celebrate"],
    },
    {
      title: "Looking",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExYWZ3emE0dm5mN2h0bGVsY2w0OXBodGd2cGJlNDRiZXo1YWNtdWRmZyZlcD12MV9naWZzX3NlYXJjaCZjdD1n/26n6WywJyh39n1pBu/giphy.gif",
      tags: ["search", "looking", "debug"],
    },
    {
      title: "Typing faster",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExOWFlbnJnbnIzbHYxcDIzdXZ3NGF3N2FocHNvMmR5enU3bHpycHBlZSZlcD12MV9naWZzX3NlYXJjaCZjdD1n/13HgwGsXF0aiGY/giphy.gif",
      tags: ["typing", "code", "work"],
    },
    {
      title: "Tiny victory",
      url: "https://media.giphy.com/media/v1.Y2lkPTc5MGI3NjExdjJ2b2tqNmF4dG16NjE0eXhuc3h5bTlvamgwNTR0Zmd6ZjhtM2JuaSZlcD12MV9naWZzX3NlYXJjaCZjdD1n/3o7abKhOpu0NwenH3O/giphy.gif",
      tags: ["win", "victory", "celebrate"],
    },
  ];

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
    messageBody = "";
    const path = selectedDirectID ? `/api/dms/${selectedDirectID}/messages` : `/api/channels/${selectedChannelID}/messages`;
    const data = await api<{ message: Message }>(path, {
      method: "POST",
      body: JSON.stringify({ body })
    });
    if (pendingUpload) {
      await api(`/api/messages/${data.message.id}/attachments`, {
        method: "POST",
        body: JSON.stringify({ upload_id: pendingUpload.id })
      });
      pendingUpload = null;
    }
    if (!messages.some((message) => message.id === data.message.id)) {
      messages = [...messages, data.message];
    }
    await scrollMessagesToBottom();
  }

  async function openThread(message: Message) {
    selectedThread = message;
    const data = await api<{ root: Message; replies: Message[]; thread_state: ThreadState }>(`/api/messages/${message.id}/thread`);
    selectedThread = data.root;
    replies = data.replies;
    selectedThreadState = data.thread_state;
  }

  async function sendReply() {
    const body = replyBody.trim();
    if (!body || !selectedThread) return;
    replyBody = "";
    const data = await api<{ message: Message; thread_state: ThreadState }>(`/api/messages/${selectedThread.id}/thread/replies`, {
      method: "POST",
      body: JSON.stringify({ body })
    });
    if (!replies.some((reply) => reply.id === data.message.id)) {
      replies = [...replies, data.message];
    }
    selectedThreadState = data.thread_state;
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

  function workspaceInitial(name: string) {
    const trimmed = name.trim();
    if (!trimmed) return "?";
    const parts = trimmed.split(/\s+/);
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
    return trimmed.slice(0, 2).toUpperCase();
  }

  function avatarInitial(name?: string | null) {
    if (!name) return "?";
    const trimmed = name.trim();
    return trimmed ? trimmed[0].toUpperCase() : "?";
  }

  function handleLabel(value?: string | null) {
    return value ? `@${value}` : "";
  }

  function dmAvatarUser(conversation: DirectConversation) {
    return conversation.members.find((member) => member.id !== user?.id) || conversation.members[0];
  }

  function avatarHue(seed: string) {
    let hash = 0;
    for (let i = 0; i < seed.length; i++) hash = (hash * 31 + seed.charCodeAt(i)) >>> 0;
    return hash % 360;
  }

  function dayLabel(value: string) {
    const date = new Date(value);
    const today = new Date();
    const yesterday = new Date();
    yesterday.setDate(today.getDate() - 1);
    const sameDay = (a: Date, b: Date) =>
      a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
    if (sameDay(date, today)) return "Today";
    if (sameDay(date, yesterday)) return "Yesterday";
    return new Intl.DateTimeFormat(undefined, { weekday: "long", month: "long", day: "numeric" }).format(date);
  }

  type Group = {
    key: string;
    dayLabel: string | null;
    messages: Message[];
    authorName: string;
    authorHandle: string;
    authorAvatarURL: string;
    authorID: string;
    timestamp: string;
  };

  function groupMessages(list: Message[]): Group[] {
    const groups: Group[] = [];
    let lastDay = "";
    let lastAuthor = "";
    let lastTime = 0;
    for (const message of list) {
      const created = new Date(message.created_at);
      const dayKey = created.toDateString();
      const authorID = message.author?.id || message.author_id || "local";
      const dayChanged = dayKey !== lastDay;
      const newAuthor = authorID !== lastAuthor;
      const tooFarApart = created.getTime() - lastTime > 5 * 60 * 1000;
      if (dayChanged || newAuthor || tooFarApart || groups.length === 0) {
        groups.push({
          key: message.id,
          dayLabel: dayChanged ? dayLabel(message.created_at) : null,
          messages: [message],
          authorName: message.author?.display_name || "Local User",
          authorHandle: message.author?.handle || "",
          authorAvatarURL: message.author?.avatar_url || "",
          authorID,
          timestamp: message.created_at,
        });
      } else {
        groups[groups.length - 1].messages.push(message);
      }
      lastDay = dayKey;
      lastAuthor = authorID;
      lastTime = created.getTime();
    }
    return groups;
  }

  function dmTitle(conversation: DirectConversation) {
    const others = conversation.members.filter((member) => member.id !== user?.id);
    const list = others.length > 0 ? others : conversation.members;
    return list.map((member) => member.display_name).join(", ");
  }

  function handleComposerKey(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendMessage();
    }
  }

  function handleReplyKey(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void sendReply();
    }
  }

  function uploadURL(upload: Upload) {
    return `/api/uploads/${encodeURIComponent(upload.id)}`;
  }

  function isImageUpload(upload: Upload) {
    return upload.content_type.startsWith("image/");
  }

  function isVideoUpload(upload: Upload) {
    return upload.content_type.startsWith("video/");
  }

  function formatBytes(size: number) {
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`;
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
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

  function threadSummary(message: Message) {
    if (selectedThread?.id === message.id) return "Open";
    return "Reply";
  }
</script>

<svelte:head>
  <meta name="color-scheme" content="light dark" />
</svelte:head>

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
  class:thread-open={selectedThread !== null}
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
            <button
              class="nav-item dm"
              class:active={conversation.id === selectedDirectID}
              onclick={async () => {
                selectedDirectID = conversation.id;
                selectedChannelID = "";
                selectedThread = null;
                mobileNavOpen = false;
                await loadMessages();
              }}
            >
              <span class="dm-avatar" style="--hue: {avatarHue(dmAvatarUser(conversation)?.id || conversation.id)}deg">
                {#if dmAvatarUser(conversation)?.avatar_url}
                  <img src={dmAvatarUser(conversation)?.avatar_url} alt="" loading="lazy" />
                {:else}
                  {avatarInitial(dmAvatarUser(conversation)?.display_name)}
                {/if}
              </span>
              <span class="nav-label">{dmTitle(conversation)}</span>
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
          <h1 class="with-glyph dm">{`@${dmTitle(selectedDirect)}`}</h1>
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
          class:active={selectedThread !== null}
          onclick={() => {
            if (selectedThread) selectedThread = null;
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

    <div class="messages" aria-live="polite" bind:this={messageList}>
      {#if messages.length === 0}
        <div class="empty">
          <div class="empty-icon">
            {#if selectedDirect}@{:else}#{/if}
          </div>
          <strong>
            {#if selectedDirect}
              This is the start of your conversation with {dmTitle(selectedDirect)}.
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
        <article class="message-group">
          <div class="avatar" style="--hue: {avatarHue(group.authorID)}deg">
            {#if group.authorAvatarURL}
              <img src={group.authorAvatarURL} alt="" loading="lazy" />
            {:else}
              {avatarInitial(group.authorName)}
            {/if}
          </div>
          <div class="group-body">
            <header>
              <strong>{group.authorName}</strong>
              {#if group.authorHandle}<span>{handleLabel(group.authorHandle)}</span>{/if}
              <time>{time(group.timestamp)}</time>
            </header>
            {#each group.messages as message, index (message.id)}
              <div class="message-row" class:selected={selectedThread?.id === message.id}>
                <span class="row-stamp" aria-hidden="true">{index === 0 ? "" : time(message.created_at)}</span>
                <div class="message-content">
                  <div class="markdown">{@html markdown(message.body)}</div>
                  {#if message.attachments?.length}
                    <div class="attachment-grid" aria-label="Attachments">
                      {#each message.attachments as attachment (attachment.id)}
                        {#if isImageUpload(attachment)}
                          <a class="image-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                            <img src={uploadURL(attachment)} alt={attachment.filename} loading="lazy" />
                            <span>{attachment.filename}</span>
                          </a>
                        {:else if isVideoUpload(attachment)}
                          <div class="video-attachment">
                            <video controls preload="metadata" aria-label={attachment.filename}>
                              <source src={uploadURL(attachment)} type={attachment.content_type} />
                            </video>
                            <a href={uploadURL(attachment)} target="_blank" rel="noreferrer">{attachment.filename}</a>
                          </div>
                        {:else}
                          <a class="file-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                            <span class="file-icon" aria-hidden="true">↧</span>
                            <span>
                              <strong>{attachment.filename}</strong>
                              <small>{formatBytes(attachment.byte_size)}</small>
                            </span>
                          </a>
                        {/if}
                      {/each}
                    </div>
                  {/if}
                </div>
                <div class="message-actions" aria-label="Message actions">
                  <button type="button" aria-label="Open thread" title={threadSummary(message)} onclick={() => openThread(message)}>
                    <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
                      <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M21 12a8 8 0 0 1-11.6 7.16L3 21l1.84-6.4A8 8 0 1 1 21 12Z"/>
                    </svg>
                  </button>
                </div>
              </div>
            {/each}
          </div>
        </article>
      {/each}
    </div>

    <form
      class="composer"
      onsubmit={(event) => {
        event.preventDefault();
        void sendMessage();
      }}
    >
      <div class="composer-toolbar" aria-label="Message tools">
        <button type="button" title="Bold" aria-label="Bold" onclick={() => applyMarkdownWrap("**")}>
          <strong>B</strong>
        </button>
        <button type="button" title="Italic" aria-label="Italic" onclick={() => applyMarkdownWrap("_")}>
          <em>I</em>
        </button>
        <button type="button" title="Code" aria-label="Code" onclick={() => applyMarkdownWrap("`")}>
          <span>{`<>`}</span>
        </button>
        <button type="button" title="Code block" aria-label="Code block" onclick={() => applyMarkdownWrap("```", "\n```")}>
          <span>{`{}`}</span>
        </button>
        <button type="button" title="Link" aria-label="Link" onclick={() => appendToComposer("[label](https://)")}>
          <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" d="M10 13a5 5 0 0 0 7.07 0l2.12-2.12a5 5 0 0 0-7.07-7.07L11 4.93M14 11a5 5 0 0 0-7.07 0L4.81 13.12a5 5 0 0 0 7.07 7.07L13 19.07"/>
          </svg>
        </button>
        <button
          type="button"
          title="GIF picker"
          aria-label="GIF picker"
          class:active={showGifPicker}
          onclick={() => (showGifPicker = !showGifPicker)}
        >
          GIF
        </button>
      </div>
      {#if showGifPicker}
        <section class="gif-picker" aria-label="GIF picker panel">
          <div class="gif-picker-head">
            <strong>GIFs</strong>
            <input bind:value={gifQuery} placeholder="Search reactions" aria-label="Search GIFs" />
          </div>
          <div class="gif-grid">
            {#each filteredGifs as gif (gif.url)}
              <button type="button" onclick={() => pickGif(gif.url, gif.title)}>
                <img src={gif.url} alt={gif.title} loading="lazy" />
                <span>{gif.title}</span>
              </button>
            {/each}
          </div>
        </section>
      {/if}
      {#if pendingUpload}
        <div class="composer-attachment">
          <span class="attachment-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="14" height="14"><path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/></svg>
          </span>
          {#if isImageUpload(pendingUpload)}
            <img class="pending-image" src={uploadURL(pendingUpload)} alt={pendingUpload.filename} />
          {/if}
          <span class="attachment-name">{pendingUpload.filename} · {formatBytes(pendingUpload.byte_size)}</span>
          <button type="button" class="attachment-remove" aria-label="Remove attachment" onclick={() => (pendingUpload = null)}>×</button>
        </div>
      {/if}
      <div class="composer-row">
        <label class="composer-icon" title="Upload file">
          <input type="file" aria-label="Upload file" onchange={uploadFile} />
          <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
            <path fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" d="M21.44 11.05 12.5 20a6 6 0 0 1-8.49-8.49l8.49-8.48a4 4 0 0 1 5.66 5.66l-8.49 8.49a2 2 0 0 1-2.83-2.83L13.41 7.5"/>
          </svg>
        </label>
        <textarea
          bind:value={messageBody}
          rows="1"
          placeholder={selectedDirect ? `Message ${dmTitle(selectedDirect)}` : selectedChannel ? `Message #${selectedChannel.name}` : "Pick a channel to start"}
          aria-label="Message body"
          onkeydown={handleComposerKey}
        ></textarea>
        <button type="submit" class="send" aria-label="Send" disabled={!messageBody.trim()}>
          <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
            <path fill="currentColor" d="M3 3.5 21 12 3 20.5l3.6-7.5L15 12 6.6 11l-3.6-7.5Z"/>
          </svg>
        </button>
      </div>
      <div class="composer-hint">
        <span><kbd>Enter</kbd> to send · <kbd>Shift</kbd>+<kbd>Enter</kbd> for newline · Markdown supported</span>
      </div>
    </form>
  </main>

  <aside class="thread" class:open={selectedThread !== null} aria-label="Thread pane">
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
            selectedThread = null;
            replies = [];
          }}
        >×</button>
      </header>
      <div class="thread-scroll">
        <article class="thread-root">
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
            </header>
            <div class="markdown">{@html markdown(selectedThread.body)}</div>
            {#if selectedThread.attachments?.length}
              <div class="attachment-grid compact" aria-label="Attachments">
                {#each selectedThread.attachments as attachment (attachment.id)}
                  {#if isImageUpload(attachment)}
                    <a class="image-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                      <img src={uploadURL(attachment)} alt={attachment.filename} loading="lazy" />
                      <span>{attachment.filename}</span>
                    </a>
                  {:else if isVideoUpload(attachment)}
                    <div class="video-attachment">
                      <video controls preload="metadata" aria-label={attachment.filename}>
                        <source src={uploadURL(attachment)} type={attachment.content_type} />
                      </video>
                      <a href={uploadURL(attachment)} target="_blank" rel="noreferrer">{attachment.filename}</a>
                    </div>
                  {:else}
                    <a class="file-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                      <span class="file-icon" aria-hidden="true">↧</span>
                      <span>
                        <strong>{attachment.filename}</strong>
                        <small>{formatBytes(attachment.byte_size)}</small>
                      </span>
                    </a>
                  {/if}
                {/each}
              </div>
            {/if}
          </div>
        </article>
        <div class="thread-divider"><span>{replies.length} {replies.length === 1 ? "reply" : "replies"}</span></div>
        <div class="reply-list">
          {#each replies as reply (reply.id)}
            <article class="reply">
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
                </header>
                <div class="markdown">{@html markdown(reply.body)}</div>
                {#if reply.attachments?.length}
                  <div class="attachment-grid compact" aria-label="Attachments">
                    {#each reply.attachments as attachment (attachment.id)}
                      {#if isImageUpload(attachment)}
                        <a class="image-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                          <img src={uploadURL(attachment)} alt={attachment.filename} loading="lazy" />
                          <span>{attachment.filename}</span>
                        </a>
                      {:else if isVideoUpload(attachment)}
                        <div class="video-attachment">
                          <video controls preload="metadata" aria-label={attachment.filename}>
                            <source src={uploadURL(attachment)} type={attachment.content_type} />
                          </video>
                          <a href={uploadURL(attachment)} target="_blank" rel="noreferrer">{attachment.filename}</a>
                        </div>
                      {:else}
                        <a class="file-attachment" href={uploadURL(attachment)} target="_blank" rel="noreferrer">
                          <span class="file-icon" aria-hidden="true">↧</span>
                          <span>
                            <strong>{attachment.filename}</strong>
                            <small>{formatBytes(attachment.byte_size)}</small>
                          </span>
                        </a>
                      {/if}
                    {/each}
                  </div>
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </div>
      <form
        class="composer reply-composer"
        onsubmit={(event) => {
          event.preventDefault();
          void sendReply();
        }}
      >
        <div class="composer-row">
          <textarea
            bind:value={replyBody}
            rows="1"
            placeholder="Reply in thread"
            aria-label="Reply body"
            onkeydown={handleReplyKey}
          ></textarea>
          <button type="submit" class="send" aria-label="Reply" disabled={!replyBody.trim()}>
            <svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
              <path fill="currentColor" d="M3 3.5 21 12 3 20.5l3.6-7.5L15 12 6.6 11l-3.6-7.5Z"/>
            </svg>
          </button>
        </div>
      </form>
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
    <button class="modal-backdrop" type="button" aria-label="Close account settings" onclick={() => (showProfileSettings = false)}></button>
    <section class="profile-modal" aria-label="Account settings">
      <header>
        <div>
          <p>Account</p>
          <h2>Profile settings</h2>
        </div>
        <button type="button" aria-label="Close account settings" onclick={() => (showProfileSettings = false)}>×</button>
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
          <button type="button" class="ghost-action" onclick={() => (showProfileSettings = false)}>Cancel</button>
          <button type="submit" class="primary-action">Save profile</button>
        </div>
      </form>
    </section>
  </div>
{/if}
{/if}
