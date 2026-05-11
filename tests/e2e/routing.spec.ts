import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";

function clickclack(args: string[]): string {
  return execFileSync("go", ["run", "./apps/api/cmd/clickclack", ...args], {
    cwd: process.cwd(),
    encoding: "utf8",
  }).trim();
}

test("app routes restore channels, DMs, threads, fallbacks, and history navigation", async ({
  page,
}) => {
  const workspacesResponse = await page.request.get("/api/workspaces");
  const { workspaces } = (await workspacesResponse.json()) as {
    workspaces: { id: string }[];
  };
  const workspaceID = workspaces[0].id;
  const stamp = Date.now();

  const channelResponse = await page.request.post(`/api/workspaces/${workspaceID}/channels`, {
    data: { name: `route-${stamp}`, kind: "public" },
  });
  const { channel } = (await channelResponse.json()) as {
    channel: { id: string; name: string };
  };

  const rootResponse = await page.request.post(`/api/channels/${channel.id}/messages`, {
    data: { body: `route thread root ${stamp}` },
  });
  const { message: root } = (await rootResponse.json()) as {
    message: { id: string; body: string };
  };
  await page.request.post(`/api/messages/${root.id}/thread/replies`, {
    data: { body: `route thread reply ${stamp}` },
  });

  const secondUserID = clickclack([
    "admin",
    "user",
    "create",
    "--data",
    "./data/e2e",
    "--workspace",
    workspaceID,
    "--name",
    `Route User ${stamp}`,
    "--email",
    `route-${stamp}@example.com`,
  ]);
  const dmResponse = await page.request.post("/api/dms", {
    data: { workspace_id: workspaceID, member_ids: [secondUserID] },
  });
  const { conversation } = (await dmResponse.json()) as {
    conversation: { id: string };
  };

  await page.goto(`/app/${workspaceID}/${channel.id}`);
  await expect(page.getByRole("heading", { name: `#${channel.name}` })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/app/${workspaceID}/${channel.id}$`));

  await page.goto(`/app/${workspaceID}/${conversation.id}`);
  await expect(page.getByRole("heading", { name: /Route User/ })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/app/${workspaceID}/${conversation.id}$`));

  await page.goBack();
  await expect(page.getByRole("heading", { name: `#${channel.name}` })).toBeVisible();
  await page.goForward();
  await expect(page.getByRole("heading", { name: /Route User/ })).toBeVisible();

  await page.goto(`/app/${workspaceID}/${root.id}`);
  await expect(page.getByRole("heading", { name: `#${channel.name}` })).toBeVisible();
  await expect(page.getByText("Thread", { exact: true })).toBeVisible();
  await expect(page.locator(".thread-root .markdown")).toContainText(root.body);
  await expect(page.locator(".reply .markdown")).toContainText(`route thread reply ${stamp}`);

  await page.goto(`/app/${workspaceID}`);
  await expect(page).toHaveURL(/\/app\/wsp_[^/]+\/(chn_|dm_)/);

  await page.goto(`/app/${workspaceID}/msg_missing_${stamp}`);
  await expect(page).toHaveURL(/\/app\/wsp_[^/]+\/(chn_|dm_)/);
  await expect(page.getByText("Could not load ClickClack")).toHaveCount(0);
});
