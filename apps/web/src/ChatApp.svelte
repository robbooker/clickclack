<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import { APIError, api } from "./lib/api";
  import { gifLibrary } from "./lib/gifs";
  import { collectRecentPeople, dmTitle } from "./lib/chat/people";
  import { redirectTypingToComposer } from "./lib/chat/typeToFocus";
  import { connectRealtime, type RealtimeConnection } from "./lib/realtime.svelte";
  import ChatComposer from "./components/composer/ChatComposer.svelte";
  import ImageViewer from "./components/media/ImageViewer.svelte";
  import MessageList from "./components/messages/MessageList.svelte";
  import GuildRail from "./components/navigation/GuildRail.svelte";
  import Sidebar from "./components/navigation/Sidebar.svelte";
  import ProfilePane from "./components/profile/ProfilePane.svelte";
  import ProfileSettingsModal from "./components/profile/ProfileSettingsModal.svelte";
  import SearchResults from "./components/search/SearchResults.svelte";
  import ThreadEmptyState from "./components/thread/ThreadEmptyState.svelte";
  import ThreadPanel from "./components/thread/ThreadPanel.svelte";
  import Topbar from "./components/topbar/Topbar.svelte";
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
  let socket: RealtimeConnection | null = null;
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
  $: connected = socket?.connected ?? false;
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
    socket?.close();
    socket = null;
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
    connectRealtimeSocket();
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
    connectRealtimeSocket();
  }

  async function selectWorkspace(workspaceID: string) {
    selectedWorkspaceID = workspaceID;
    await loadChannels();
    await loadDirectConversations();
    connectRealtimeSocket();
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

  async function selectChannel(channelID: string) {
    selectedChannelID = channelID;
    selectedDirectID = "";
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    mobileNavOpen = false;
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

  function resetSearch() {
    searchQuery = "";
    searchResults = [];
  }

  async function openSearchResult(result: SearchResult) {
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

  async function selectDirectConversation(conversationID: string) {
    selectedDirectID = conversationID;
    selectedChannelID = "";
    selectedThread = null;
    selectedProfile = null;
    activeComposerContext = "message";
    mobileNavOpen = false;
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

  function connectRealtimeSocket() {
    socket?.close();
    socket = null;
    if (!selectedWorkspaceID) return;
    socket = connectRealtime({
      workspaceID: selectedWorkspaceID,
      onEvent: (event) => void handleEvent(event),
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

  function toggleSidePanelFromTopbar() {
    if (sidePanelOpen) closeSidePanel();
    else status = "pick a message to open its thread";
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

  <GuildRail
    {workspaces}
    {selectedWorkspaceID}
    {workspaceName}
    {showWorkspaceCreate}
    onSelectWorkspace={(workspaceID) => void selectWorkspace(workspaceID)}
    onToggleWorkspaceCreate={() => (showWorkspaceCreate = !showWorkspaceCreate)}
    onWorkspaceName={(value) => (workspaceName = value)}
    onCreateWorkspace={() => void createWorkspace()}
  />

  <Sidebar
    workspaceName={selectedWorkspace?.name}
    {status}
    {connected}
    {sidebarCollapsed}
    {channels}
    {directConversations}
    {recentPeople}
    currentUser={user}
    {selectedChannelID}
    {selectedDirectID}
    {selectedProfile}
    {channelName}
    {directMemberID}
    onToggleCollapse={() => (sidebarCollapsed = !sidebarCollapsed)}
    onSelectChannel={(channelID) => void selectChannel(channelID)}
    onChannelName={(value) => (channelName = value)}
    onCreateChannel={() => void createChannel()}
    onSelectDirect={(conversationID) => void selectDirectConversation(conversationID)}
    onDirectMemberID={(value) => (directMemberID = value)}
    onCreateDirect={() => void createDirectConversation()}
    onOpenProfile={openUserProfile}
    onOpenSettings={openProfileSettings}
  />

  <main class="timeline">
    <Topbar
      {selectedDirect}
      {selectedChannel}
      workspaceName={selectedWorkspace?.name}
      currentUserID={user?.id}
      {searchQuery}
      {sidePanelOpen}
      threadOpen={selectedThread !== null}
      onSearchQuery={(value) => (searchQuery = value)}
      onSearch={() => void searchMessages()}
      onResetSearch={resetSearch}
      onToggleThread={toggleSidePanelFromTopbar}
      onPinnedItems={() => (status = "no pinned items")}
    />

    <SearchResults
      results={searchResults}
      onClose={() => (searchResults = [])}
      onOpenResult={(result) => void openSearchResult(result)}
    />

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
      <ThreadPanel
        root={selectedThread}
        {replies}
        threadState={selectedThreadState}
        {replyBody}
        replyTarget={replyTarget && replyContext === "thread" ? replyTarget : null}
        onClose={closeSidePanel}
        onReplyBody={(value) => (replyBody = value)}
        onSubmitReply={() => void sendReply()}
        onReplyKeydown={handleReplyKey}
        onReplyFocus={() => (activeComposerContext = "thread")}
        onReplyInputRef={(node) => (replyInput = node)}
        onSetReplyTarget={setReplyTarget}
        onClearReply={clearReplyTarget}
        onActivateThreadComposer={() => (activeComposerContext = "thread")}
        onInlineImagePointerUp={handleInlineImagePointerUp}
        onJumpToQuote={(message) => void jumpToQuotedMessage(message)}
        onOpenImage={openImageViewer}
      />
    {:else if selectedProfile}
      <ProfilePane
        profile={selectedProfile}
        currentUser={user}
        workspaceName={selectedWorkspace?.name}
        onClose={closeSidePanel}
        onEdit={openProfileSettings}
        onMessage={(memberID) => void startDirectWithUser(memberID)}
        onSetStatus={() => (status = "status messages are coming soon")}
      />
    {:else}
      <ThreadEmptyState />
    {/if}
  </aside>
</div>
{#if showProfileSettings && user}
  <ProfileSettingsModal
    {user}
    displayName={profileDisplayName}
    handle={profileHandle}
    avatarURL={profileAvatarURL}
    status={profileStatus}
    statusError={profileStatusError}
    onDisplayName={(value) => (profileDisplayName = value)}
    onHandle={(value) => (profileHandle = value)}
    onAvatarURL={(value) => (profileAvatarURL = value)}
    onClose={closeModal}
    onSave={() => void saveProfile()}
  />
{/if}
{#if selectedImage}
  <ImageViewer url={selectedImage.url} title={selectedImage.title} onClose={closeModal} />
{/if}
{/if}
