<script lang="ts">
  import { onDestroy, onMount } from "svelte";
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
  let status = "loading";
  let authRequired = false;
  let socket: WebSocket | null = null;
  let reconnectTimer: number | undefined;

  $: selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceID);
  $: selectedChannel = channels.find((channel) => channel.id === selectedChannelID);
  $: selectedDirect = directConversations.find((conversation) => conversation.id === selectedDirectID);

  onMount(() => {
    void boot();
  });

  onDestroy(() => {
    socket?.close();
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

  async function loadWorkspaces() {
    const data = await api<{ workspaces: Workspace[] }>("/api/workspaces");
    workspaces = data.workspaces;
    selectedWorkspaceID = selectedWorkspaceID || workspaces[0]?.id || "";
    await loadChannels();
    await loadDirectConversations();
    connectRealtime();
  }

  async function createWorkspace() {
    if (!workspaceName.trim()) return;
    const data = await api<{ workspace: Workspace }>("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ name: workspaceName })
    });
    workspaceName = "";
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
    await loadMessages();
  }

  async function loadMessages() {
    if (selectedDirectID) {
      const data = await api<{ messages: Message[] }>(`/api/dms/${selectedDirectID}/messages`);
      messages = data.messages;
      return;
    }
    if (!selectedChannelID) {
      messages = [];
      return;
    }
    const data = await api<{ messages: Message[] }>(`/api/channels/${selectedChannelID}/messages`);
    messages = data.messages;
  }

  async function sendMessage() {
    const body = messageBody.trim();
    if (!body || (!selectedChannelID && !selectedDirectID)) return;
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
    socket?.close();
    if (!selectedWorkspaceID) return;
    const lastCursor = localStorage.getItem(`clickclack:${selectedWorkspaceID}:cursor`) || "";
    const url = new URL("/api/realtime/ws", window.location.href);
    url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("workspace_id", selectedWorkspaceID);
    if (lastCursor) url.searchParams.set("after_cursor", lastCursor);
    socket = new WebSocket(url);
    socket.addEventListener("message", (message) => {
      const event = JSON.parse(String(message.data)) as RealtimeEvent;
      if (event.cursor) localStorage.setItem(`clickclack:${selectedWorkspaceID}:cursor`, event.cursor);
      void handleEvent(event);
    });
    socket.addEventListener("close", () => {
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

</script>

<svelte:head>
  <meta name="color-scheme" content="light dark" />
</svelte:head>

{#if authRequired}
  <main class="auth-shell">
    <section class="auth-panel" aria-label="Sign in">
      <div class="brand">
        <div class="mark">cc</div>
        <div>
          <strong>ClickClack</strong>
          <span>OpenClaw workspace chat</span>
        </div>
      </div>
      <div class="auth-copy">
        <h1>Sign in to ClickClack</h1>
        <p>GitHub access is limited to active members of the OpenClaw organization.</p>
      </div>
      <a class="github-login" href="/api/auth/github/start">Continue with GitHub</a>
    </section>
  </main>
{:else}
<div class="shell">
  <aside class="sidebar" aria-label="Workspace and channel navigation">
    <div class="brand">
      <div class="mark">cc</div>
      <div>
        <strong>ClickClack</strong>
        <span>{user?.display_name || "local"}</span>
      </div>
    </div>

    <section>
      <div class="section-title">Workspaces</div>
      <div class="nav-list">
        {#each workspaces as workspace}
          <button
            class:active={workspace.id === selectedWorkspaceID}
            onclick={async () => {
              selectedWorkspaceID = workspace.id;
              await loadChannels();
              connectRealtime();
            }}
          >
            {workspace.name}
          </button>
        {/each}
      </div>
      <form
        class="inline-create"
        onsubmit={(event) => {
          event.preventDefault();
          void createWorkspace();
        }}
      >
        <input bind:value={workspaceName} placeholder="New workspace" aria-label="New workspace name" />
      </form>
    </section>

    <section>
      <div class="section-title">Channels</div>
      <div class="nav-list channels">
        {#each channels as channel}
          <button
            class:active={channel.id === selectedChannelID}
            onclick={async () => {
              selectedChannelID = channel.id;
              selectedThread = null;
              await loadMessages();
            }}
          >
            <span>#</span>{channel.name}
          </button>
        {/each}
      </div>
      <form
        class="inline-create"
        onsubmit={(event) => {
          event.preventDefault();
          void createChannel();
        }}
      >
        <input bind:value={channelName} placeholder="New channel" aria-label="New channel name" />
      </form>
    </section>

    <section>
      <div class="section-title">DMs</div>
      <div class="nav-list channels">
        {#each directConversations as conversation}
          <button
            class:active={conversation.id === selectedDirectID}
            onclick={async () => {
              selectedDirectID = conversation.id;
              selectedChannelID = "";
              selectedThread = null;
              await loadMessages();
            }}
          >
            <span>@</span>{conversation.members.map((member) => member.display_name).join(", ")}
          </button>
        {/each}
      </div>
      <form
        class="inline-create"
        onsubmit={(event) => {
          event.preventDefault();
          void createDirectConversation();
        }}
      >
        <input bind:value={directMemberID} placeholder="Member user ID" aria-label="DM member user ID" />
      </form>
    </section>
  </aside>

  <main class="timeline">
    <header class="topbar">
      <div>
        <p>{selectedWorkspace?.name || "Workspace"}</p>
        <h1>{selectedDirect ? "@" + selectedDirect.members.map((member) => member.display_name).join(", ") : "#" + (selectedChannel?.name || "general")}</h1>
      </div>
      <form
        class="search"
        onsubmit={(event) => {
          event.preventDefault();
          void searchMessages();
        }}
      >
        <input bind:value={searchQuery} placeholder="Search" aria-label="Search messages" />
        <button type="submit">Search</button>
      </form>
      <div class="connection" data-state={socket?.readyState === WebSocket.OPEN ? "live" : "idle"}>
        {socket?.readyState === WebSocket.OPEN ? "live" : status}
      </div>
    </header>

    {#if searchResults.length > 0}
      <div class="search-results" aria-label="Search results">
        {#each searchResults as result (result.message.id)}
          <button
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
            <strong>{result.message.author?.display_name || "Local User"}</strong>
            <span>{result.message.body}</span>
          </button>
        {/each}
      </div>
    {/if}

    <div class="messages" aria-live="polite">
      {#if messages.length === 0}
        <div class="empty">
          <strong>Quiet tide.</strong>
          <span>Start with Markdown. Threads open from any root message.</span>
        </div>
      {/if}
      {#each messages as message (message.id)}
        <article class="message" class:selected={selectedThread?.id === message.id}>
          <div class="avatar">{message.author?.display_name?.slice(0, 1) || "c"}</div>
          <div class="message-body">
            <header>
              <strong>{message.author?.display_name || "Local User"}</strong>
              <time>{time(message.created_at)}</time>
            </header>
            <div class="markdown">{@html markdown(message.body)}</div>
            <button class="thread-button" onclick={() => openThread(message)}>Open thread</button>
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
      <textarea bind:value={messageBody} rows="3" placeholder="Message with Markdown" aria-label="Message body"></textarea>
      <div class="composer-actions">
        <label class="upload-button">
          <input type="file" aria-label="Upload file" onchange={uploadFile} />
          Upload
        </label>
        {#if pendingUpload}
          <span class="pending-upload">{pendingUpload.filename}</span>
        {/if}
        <button type="button" onclick={() => void sendMessage()}>Send</button>
      </div>
    </form>
  </main>

  <aside class="thread" class:open={selectedThread} aria-label="Thread pane">
    {#if selectedThread}
      <header>
        <div>
          <p>Thread</p>
          <strong>{selectedThreadState?.reply_count || replies.length} replies</strong>
        </div>
        <button
          aria-label="Close thread"
          onclick={() => {
            selectedThread = null;
            replies = [];
          }}
        >
          x
        </button>
      </header>
      <article class="thread-root">
        <strong>{selectedThread.author?.display_name || "Local User"}</strong>
        <div class="markdown">{@html markdown(selectedThread.body)}</div>
      </article>
      <div class="reply-list">
        {#each replies as reply (reply.id)}
          <article class="reply">
            <header>
              <strong>{reply.author?.display_name || "Local User"}</strong>
              <time>{time(reply.created_at)}</time>
            </header>
            <div class="markdown">{@html markdown(reply.body)}</div>
          </article>
        {/each}
      </div>
      <form
        class="reply-composer"
        onsubmit={(event) => {
          event.preventDefault();
          void sendReply();
        }}
      >
        <textarea bind:value={replyBody} rows="3" placeholder="Reply in thread" aria-label="Reply body"></textarea>
        <button type="button" onclick={() => void sendReply()}>Reply</button>
      </form>
    {:else}
      <div class="thread-empty">
        <strong>No thread open</strong>
        <span>Pick a message to keep the side conversation tidy.</span>
      </div>
    {/if}
  </aside>
</div>
{/if}
