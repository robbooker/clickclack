// Typing indicator state machine.
//
// Pattern (Zulip-style, see web/src/typing_status.ts):
//  - On first keystroke, immediately POST `typing.started`.
//  - While the user keeps typing, re-ping every PING_EVERY_MS so receivers
//    that joined late still see them and so the entry doesn't decay.
//  - After IDLE_MS of no keystrokes, POST `typing.stopped`.
//  - On send / blur / channel-switch, call stop() to fire stopped immediately.

import { api } from "./api";

const IDLE_MS = 5000;
const PING_EVERY_MS = 4000;

type Scope = {
  workspaceID: string;
  channelID?: string;
  directConversationID?: string;
};

type State = {
  scope: Scope;
  active: boolean;
  lastPingAt: number;
  idleTimer?: number;
};

let state: State | null = null;

function scopesEqual(a: Scope, b: Scope): boolean {
  return (
    a.workspaceID === b.workspaceID &&
    (a.channelID ?? "") === (b.channelID ?? "") &&
    (a.directConversationID ?? "") === (b.directConversationID ?? "")
  );
}

function payload(scope: Scope): Record<string, string> {
  const p: Record<string, string> = {};
  if (scope.channelID) p.channel_id = scope.channelID;
  if (scope.directConversationID) p.direct_conversation_id = scope.directConversationID;
  return p;
}

async function send(scope: Scope, type: "typing.started" | "typing.stopped"): Promise<void> {
  try {
    await api("/api/realtime/ephemeral", {
      method: "POST",
      body: JSON.stringify({
        workspace_id: scope.workspaceID,
        channel_id: scope.channelID || "",
        type,
        payload: payload(scope),
      }),
    });
  } catch {
    // Typing notifications are best-effort; failures are silent.
  }
}

export function notifyTyping(scope: Scope): void {
  if (!scope.workspaceID) return;
  if (!scope.channelID && !scope.directConversationID) return;

  const now = Date.now();
  if (state && !scopesEqual(state.scope, scope)) {
    void stopTypingFor(state);
    state = null;
  }
  if (!state) {
    state = { scope, active: false, lastPingAt: 0 };
  }
  if (!state.active || now - state.lastPingAt >= PING_EVERY_MS) {
    state.active = true;
    state.lastPingAt = now;
    void send(scope, "typing.started");
  }
  if (state.idleTimer) window.clearTimeout(state.idleTimer);
  state.idleTimer = window.setTimeout(() => {
    if (state) {
      void stopTypingFor(state);
      state = null;
    }
  }, IDLE_MS);
}

async function stopTypingFor(s: State): Promise<void> {
  if (s.idleTimer) window.clearTimeout(s.idleTimer);
  if (s.active) await send(s.scope, "typing.stopped");
}

export function stopTyping(): void {
  if (!state) return;
  void stopTypingFor(state);
  state = null;
}
