import { expect, test } from "@playwright/test";
import { execFile, execFileSync } from "node:child_process";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";

const serverURL = "http://127.0.0.1:18082";
const execFileAsync = promisify(execFile);
const goCacheEnv = {
  GOCACHE: execFileSync("go", ["env", "GOCACHE"], { cwd: process.cwd(), encoding: "utf8" }).trim(),
  GOMODCACHE: execFileSync("go", ["env", "GOMODCACHE"], {
    cwd: process.cwd(),
    encoding: "utf8",
  }).trim(),
};

function clickclack(args: string[], input?: string, env: NodeJS.ProcessEnv = {}): string {
  return execFileSync("go", ["run", "./apps/api/cmd/clickclack", ...args], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, ...env },
    input,
  }).trim();
}

async function clickclackAsync(args: string[], env: NodeJS.ProcessEnv = {}): Promise<string> {
  const { stdout } = await execFileAsync("go", ["run", "./apps/api/cmd/clickclack", ...args], {
    cwd: process.cwd(),
    encoding: "utf8",
    env: { ...process.env, ...env },
  });
  return stdout.trim();
}

function isolatedHome(): NodeJS.ProcessEnv {
  const root = mkdtempSync(join(tmpdir(), "clickclack-e2e-"));
  return {
    ...goCacheEnv,
    HOME: root,
    XDG_CONFIG_HOME: join(root, ".config"),
  };
}

test("product website links to app and docs", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "ClickClack" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open app" })).toHaveAttribute("href", "/app");
  await expect(page.getByRole("link", { name: "Read docs" })).toHaveAttribute(
    "href",
    "https://docs.clickclack.chat",
  );
  await expect(page.getByText("Self-hostable chat. Serious tool. Mild brine.")).toBeVisible();
});

test("sends messages, searches, uploads, opens a thread, and creates a DM", async ({ page }) => {
  const consoleMessages: string[] = [];
  page.on("console", (message) => consoleMessages.push(`${message.type()}: ${message.text()}`));
  page.on("pageerror", (error) => consoleMessages.push(`pageerror: ${error.message}`));
  const workspacesResponse = await page.request.get("/api/workspaces");
  const workspaces = (await workspacesResponse.json()) as { workspaces: { id: string }[] };
  const workspaceId = workspaces.workspaces[0].id;
  const secondUserId = execFileSync(
    "go",
    [
      "run",
      "./apps/api/cmd/clickclack",
      "admin",
      "user",
      "create",
      "--data",
      "./data/e2e",
      "--workspace",
      workspaceId,
      "--name",
      "Second User",
      "--email",
      "second@example.com",
    ],
    { cwd: process.cwd(), encoding: "utf8" },
  ).trim();

  await page.goto("/app");

  await page.getByRole("button", { name: "# general" }).click();
  await expect(page.getByRole("heading", { name: "#general" })).toBeVisible();

  await page.getByLabel("Message body").fill("hello **playwright**");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(
    page.locator(".markdown").filter({ hasText: "hello playwright" }),
    consoleMessages.join("\n"),
  ).toBeVisible({
    timeout: 5_000,
  });

  await page.getByLabel("Search messages").fill("playwright");
  await page.getByRole("button", { name: "Search" }).click();
  await expect(page.getByLabel("Search results").getByText("hello **playwright**")).toBeVisible();

  await page.getByLabel("Upload file").setInputFiles({
    name: "note.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("uploaded from playwright"),
  });
  await expect(page.getByText("note.txt")).toBeVisible();
  await page.getByLabel("Message body").fill("message with upload");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator(".markdown").filter({ hasText: "message with upload" })).toBeVisible();

  await page.getByRole("button", { name: "Open thread" }).first().click();
  await expect(page.getByText("Thread", { exact: true })).toBeVisible();

  await page.getByLabel("Reply body").fill("thread _reply_");
  await page.getByRole("button", { name: "Reply" }).click();
  await expect(page.locator(".reply .markdown").filter({ hasText: "thread reply" })).toBeVisible();

  await page.reload();
  await expect(page.locator(".markdown").filter({ hasText: "hello playwright" })).toBeVisible();

  await page.getByLabel("DM member user ID").fill(secondUserId);
  await page.getByLabel("DM member user ID").press("Enter");
  await expect(page.getByRole("heading", { name: /Second User/ })).toBeVisible();
  await page.getByLabel("Message body").fill("private playwright");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.locator(".markdown").filter({ hasText: "private playwright" })).toBeVisible();
});

test("CLI supports multiple accounts chatting in one channel", async ({ page }) => {
  const workspacesResponse = await page.request.get("/api/workspaces");
  const workspaces = (await workspacesResponse.json()) as { workspaces: { id: string }[] };
  const workspaceId = workspaces.workspaces[0].id;

  const channelsResponse = await page.request.get(`/api/workspaces/${workspaceId}/channels`);
  const channels = (await channelsResponse.json()) as { channels: { id: string; name: string }[] };
  const general =
    channels.channels.find((channel) => channel.name === "general") ?? channels.channels[0];

  const ownerMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "local@clickclack.chat",
    "--name",
    "Local Captain",
  ]);
  const ownerSessionToken = clickclack([
    "--server",
    serverURL,
    "login",
    "--magic-token",
    ownerMagicToken,
    "--plain",
    "--no-store",
  ]);

  clickclack([
    "admin",
    "user",
    "create",
    "--data",
    "./data/e2e",
    "--workspace",
    workspaceId,
    "--name",
    "CLI Second",
    "--email",
    "cli-second@example.com",
  ]);
  const secondMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "cli-second@example.com",
    "--name",
    "CLI Second",
  ]);
  const secondSessionToken = clickclack([
    "--server",
    serverURL,
    "login",
    "--magic-token",
    secondMagicToken,
    "--plain",
    "--no-store",
  ]);

  const ownerMessageId = clickclack([
    "--server",
    serverURL,
    "--token",
    ownerSessionToken,
    "send",
    "--workspace",
    workspaceId,
    "--channel",
    general.id,
    "--plain",
    "owner from cli",
  ]);
  const secondMessageId = clickclack(
    [
      "--server",
      serverURL,
      "--token",
      secondSessionToken,
      "send",
      "--workspace",
      workspaceId,
      "--channel",
      general.id,
      "--stdin",
      "--plain",
    ],
    "second from cli",
  );

  const listJSON = clickclack([
    "--server",
    serverURL,
    "--token",
    ownerSessionToken,
    "messages",
    "list",
    "--workspace",
    workspaceId,
    "--channel",
    general.id,
    "--limit",
    "20",
    "--json",
  ]);
  const listed = JSON.parse(listJSON) as {
    messages: { id: string; body: string; author?: { display_name: string } }[];
  };
  expect(listed.messages).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: ownerMessageId, body: "owner from cli" }),
      expect.objectContaining({ id: secondMessageId, body: "second from cli" }),
    ]),
  );
  expect(
    listed.messages.find((message) => message.id === ownerMessageId)?.author?.display_name,
  ).toBe("Local Captain");
  expect(
    listed.messages.find((message) => message.id === secondMessageId)?.author?.display_name,
  ).toBe("CLI Second");

  const replyId = clickclack(
    [
      "--server",
      serverURL,
      "--token",
      secondSessionToken,
      "threads",
      "reply",
      ownerMessageId,
      "--stdin",
      "--plain",
    ],
    "thread reply from second cli",
  );
  const threadJSON = clickclack([
    "--server",
    serverURL,
    "--token",
    ownerSessionToken,
    "threads",
    "open",
    ownerMessageId,
    "--json",
  ]);
  const thread = JSON.parse(threadJSON) as {
    replies: { id: string; body: string; author?: { display_name: string } }[];
  };
  expect(thread.replies).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: replyId,
        body: "thread reply from second cli",
        author: expect.objectContaining({ display_name: "CLI Second" }),
      }),
    ]),
  );
});

test("CLI does not reuse stored tokens for another server", async ({ page }) => {
  const workspacesResponse = await page.request.get("/api/workspaces");
  const workspaces = (await workspacesResponse.json()) as { workspaces: { id: string }[] };
  const workspaceId = workspaces.workspaces[0].id;
  const channelsResponse = await page.request.get(`/api/workspaces/${workspaceId}/channels`);
  const channels = (await channelsResponse.json()) as { channels: { id: string }[] };
  const channelId = channels.channels[0].id;

  const ownerMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "local@clickclack.chat",
    "--name",
    "Local Captain",
  ]);
  const env = isolatedHome();
  clickclack(["--server", serverURL, "login", "--magic-token", ownerMagicToken], undefined, env);

  let leakedAuth = "";
  const http = await import("node:http");
  const probeServer = http.createServer((request, response) => {
    leakedAuth = request.headers.authorization ?? "";
    response.writeHead(401, { "content-type": "application/json" });
    response.end(JSON.stringify({ error: "unauthorized" }));
  });
  await new Promise<void>((resolve) => probeServer.listen(0, "127.0.0.1", resolve));
  try {
    const address = probeServer.address();
    if (typeof address !== "object" || address === null) throw new Error("missing probe address");
    await expect(
      clickclackAsync(
        [
          "--server",
          `http://127.0.0.1:${address.port}`,
          "send",
          "--workspace",
          workspaceId,
          "--channel",
          channelId,
          "should not leak",
        ],
        env,
      ),
    ).rejects.toThrow();
    expect(leakedAuth).toBe("");
  } finally {
    await new Promise<void>((resolve) => probeServer.close(() => resolve()));
  }
});

test("CLI does not reuse saved workspace defaults for another server", async ({ page }) => {
  const workspacesResponse = await page.request.get("/api/workspaces");
  const workspaces = (await workspacesResponse.json()) as { workspaces: { id: string }[] };
  const oldWorkspaceId = workspaces.workspaces[0].id;
  const channelsResponse = await page.request.get(`/api/workspaces/${oldWorkspaceId}/channels`);
  const channels = (await channelsResponse.json()) as { channels: { id: string }[] };
  const oldChannelId = channels.channels[0].id;

  const ownerMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "local@clickclack.chat",
    "--name",
    "Local Captain",
  ]);
  const env = isolatedHome();
  clickclack(
    [
      "--server",
      serverURL,
      "--workspace",
      oldWorkspaceId,
      "--channel",
      oldChannelId,
      "login",
      "--magic-token",
      ownerMagicToken,
    ],
    undefined,
    env,
  );

  const requestedPaths: string[] = [];
  const http = await import("node:http");
  const probeServer = http.createServer((request, response) => {
    requestedPaths.push(request.url ?? "");
    response.setHeader("content-type", "application/json");
    if (request.url === "/api/workspaces") {
      response.end(
        JSON.stringify({
          workspaces: [{ id: "wsp_probe", slug: "probe", name: "Probe Workspace" }],
        }),
      );
      return;
    }
    if (request.url === "/api/workspaces/wsp_probe/channels") {
      response.end(
        JSON.stringify({
          workspace: { id: "wsp_probe", slug: "probe", name: "Probe Workspace" },
          channels: [{ id: "chn_probe", name: "general", kind: "public" }],
        }),
      );
      return;
    }
    response.writeHead(404);
    response.end(JSON.stringify({ error: "not found" }));
  });
  await new Promise<void>((resolve) => probeServer.listen(0, "127.0.0.1", resolve));
  try {
    const address = probeServer.address();
    if (typeof address !== "object" || address === null) throw new Error("missing probe address");
    const output = await clickclackAsync(
      ["--server", `http://127.0.0.1:${address.port}`, "channels", "list"],
      env,
    );
    expect(output).toContain("chn_probe");
    expect(requestedPaths).toContain("/api/workspaces/wsp_probe/channels");
    expect(requestedPaths).not.toContain(`/api/workspaces/${oldWorkspaceId}/channels`);
  } finally {
    await new Promise<void>((resolve) => probeServer.close(() => resolve()));
  }
});

test("CLI honors user override when stored credentials exist", async ({ page }) => {
  const workspacesResponse = await page.request.get("/api/workspaces");
  const workspaces = (await workspacesResponse.json()) as { workspaces: { id: string }[] };
  const workspaceId = workspaces.workspaces[0].id;
  const channelsResponse = await page.request.get(`/api/workspaces/${workspaceId}/channels`);
  const channels = (await channelsResponse.json()) as { channels: { id: string }[] };
  const channelId = channels.channels[0].id;

  const ownerMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "local@clickclack.chat",
    "--name",
    "Local Captain",
  ]);
  const env = isolatedHome();
  clickclack(["--server", serverURL, "login", "--magic-token", ownerMagicToken], undefined, env);

  const secondUserId = clickclack([
    "admin",
    "user",
    "create",
    "--data",
    "./data/e2e",
    "--workspace",
    workspaceId,
    "--name",
    "Override User",
    "--email",
    "override@example.com",
  ]);

  const messageId = clickclack(
    [
      "--server",
      serverURL,
      "--user",
      secondUserId,
      "send",
      "--workspace",
      workspaceId,
      "--channel",
      channelId,
      "--plain",
      "sent with user override",
    ],
    undefined,
    env,
  );

  const messagesJSON = clickclack(
    [
      "--server",
      serverURL,
      "--user",
      secondUserId,
      "messages",
      "list",
      "--workspace",
      workspaceId,
      "--channel",
      channelId,
      "--json",
    ],
    undefined,
    env,
  );
  const messages = JSON.parse(messagesJSON) as {
    messages: { id: string; body: string; author?: { display_name: string } }[];
  };
  expect(messages.messages).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: messageId,
        body: "sent with user override",
        author: expect.objectContaining({ display_name: "Override User" }),
      }),
    ]),
  );
});

test("CLI resolves channel IDs across visible workspaces", async ({ page }) => {
  const firstWorkspacesResponse = await page.request.get("/api/workspaces");
  const firstWorkspaces = (await firstWorkspacesResponse.json()) as {
    workspaces: { id: string }[];
  };
  const defaultWorkspaceId = firstWorkspaces.workspaces[0].id;

  const ownerMagicToken = clickclack([
    "admin",
    "magic-link",
    "create",
    "--data",
    "./data/e2e",
    "--email",
    "local@clickclack.chat",
    "--name",
    "Local Captain",
  ]);
  const ownerToken = clickclack([
    "--server",
    serverURL,
    "login",
    "--magic-token",
    ownerMagicToken,
    "--plain",
    "--no-store",
  ]);

  const workspaceResponse = await page.request.post("/api/workspaces", {
    data: { name: "Other CLI Workspace", slug: "other-cli" },
  });
  const workspace = (await workspaceResponse.json()) as { workspace: { id: string } };
  expect(workspace.workspace.id).not.toBe(defaultWorkspaceId);
  const channelResponse = await page.request.post(
    `/api/workspaces/${workspace.workspace.id}/channels`,
    {
      data: { name: "remote-room", kind: "public" },
    },
  );
  const channel = (await channelResponse.json()) as { channel: { id: string; name: string } };

  const messageId = clickclack([
    "--server",
    serverURL,
    "--token",
    ownerToken,
    "send",
    "--channel",
    channel.channel.id,
    "--plain",
    "cross workspace channel id",
  ]);
  const messagesJSON = clickclack([
    "--server",
    serverURL,
    "--token",
    ownerToken,
    "messages",
    "list",
    "--channel",
    channel.channel.id,
    "--json",
  ]);
  const messages = JSON.parse(messagesJSON) as { messages: { id: string; body: string }[] };
  expect(messages.messages).toEqual(
    expect.arrayContaining([
      expect.objectContaining({ id: messageId, body: "cross workspace channel id" }),
    ]),
  );
});
