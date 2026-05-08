import type { RealtimeEvent } from "./types";

export type RealtimeOptions = {
  workspaceID: string;
  onEvent: (event: RealtimeEvent) => void;
  reconnectDelayMs?: number;
};

export type RealtimeConnection = {
  readonly connected: boolean;
  close(): void;
};

const cursorKey = (workspaceID: string) => `clickclack:${workspaceID}:cursor`;

export function connectRealtime(options: RealtimeOptions): RealtimeConnection {
  const { workspaceID, onEvent } = options;
  const reconnectDelayMs = options.reconnectDelayMs ?? 1200;

  let socket: WebSocket | null = null;
  let reconnectTimer: number | undefined;
  let closed = false;
  const state = $state({ connected: false });

  function open() {
    if (closed) return;
    const url = new URL("/api/realtime/ws", window.location.href);
    url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    url.searchParams.set("workspace_id", workspaceID);
    const lastCursor = localStorage.getItem(cursorKey(workspaceID)) || "";
    if (lastCursor) url.searchParams.set("after_cursor", lastCursor);

    const current = new WebSocket(url);
    socket = current;

    current.addEventListener("open", () => {
      if (socket === current) state.connected = true;
    });

    current.addEventListener("message", (message) => {
      const event = JSON.parse(String(message.data)) as RealtimeEvent;
      if (event.cursor) localStorage.setItem(cursorKey(workspaceID), event.cursor);
      onEvent(event);
    });

    current.addEventListener("close", () => {
      if (socket !== current || closed) return;
      socket = null;
      state.connected = false;
      reconnectTimer = window.setTimeout(open, reconnectDelayMs);
    });
  }

  open();

  return {
    get connected() {
      return state.connected;
    },
    close() {
      closed = true;
      state.connected = false;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      socket?.close();
      socket = null;
    },
  };
}
