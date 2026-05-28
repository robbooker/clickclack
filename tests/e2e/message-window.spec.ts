import { expect, test } from "@playwright/test";
import {
  MAX_PROTECTED_MESSAGE_WINDOW,
  RETAIN_MESSAGE_WINDOW,
  trimMessageWindow,
} from "../../apps/web/src/lib/chat/messageWindow";
import type { Message } from "../../apps/web/src/lib/types";

function message(seq: number): Message {
  return {
    id: `msg_${seq}`,
    workspace_id: "wsp_test",
    channel_id: "chn_test",
    author_id: "usr_test",
    thread_root_id: `msg_${seq}`,
    channel_seq: seq,
    body: `message ${seq}`,
    body_format: "markdown",
    created_at: "2026-05-09T00:00:00Z",
  };
}

function messages(count: number): Message[] {
  return Array.from({ length: count }, (_, index) => message(index + 1));
}

test("message window trim preserves protected rows near append and prepend edges", () => {
  const list = messages(950);
  const appendAnchor = list[99].id;
  const append = trimMessageWindow(list, "append", new Set([appendAnchor]));
  expect(append).toHaveLength(851);
  expect(append[0].id).toBe(appendAnchor);
  expect(append.at(-1)?.id).toBe(list.at(-1)?.id);

  const prependAnchor = list[849].id;
  const prepend = trimMessageWindow(list, "prepend", new Set([prependAnchor]));
  expect(prepend).toHaveLength(850);
  expect(prepend[0].id).toBe(list[0].id);
  expect(prepend.at(-1)?.id).toBe(prependAnchor);
});

test("message window trim stays bounded when protected rows span too far", () => {
  const list = messages(MAX_PROTECTED_MESSAGE_WINDOW + RETAIN_MESSAGE_WINDOW);
  const append = trimMessageWindow(list, "append", new Set([list[0].id]));
  expect(append).toHaveLength(MAX_PROTECTED_MESSAGE_WINDOW);
  expect(append.some((item) => item.id === list[0].id)).toBe(false);
  expect(append.at(-1)?.id).toBe(list.at(-1)?.id);
});
