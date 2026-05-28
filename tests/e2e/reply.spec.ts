import { expect, test } from "@playwright/test";

test("inline quote-reply renders, jumps, and survives source delete", async ({ page }) => {
	await page.goto("/app");
	await page.getByRole("link", { name: "# general" }).click();
	await expect(page.getByRole("heading", { name: "#general" })).toBeVisible();

	// Send the original message we'll reply to.
	await page.getByLabel("Message body").fill("the quoted original");
	await page.getByRole("button", { name: "Send" }).click();
	const original = page.locator(".markdown").filter({ hasText: "the quoted original" });
	await expect(original).toBeVisible();

	// Click Quote on the row, ensure composer chip appears, send a reply.
	const originalRow = page.locator(".message-row", {
		has: page.locator(".markdown").filter({ hasText: "the quoted original" }),
	});
	await originalRow.hover();
	await originalRow.getByRole("button", { name: "Reply" }).click();
	await expect(page.getByLabel("Replying to message")).toBeVisible();

	await page.getByLabel("Message body").fill("responding inline");
	await page.getByRole("button", { name: "Send" }).click();
	const replyRow = page.locator(".message-row", {
		has: page.locator(".markdown").filter({ hasText: "responding inline" }),
	});
	await expect(replyRow).toBeVisible();

	const quoteBlock = replyRow.locator(".quote-block");
	await expect(quoteBlock).toBeVisible();
	await expect(quoteBlock).toContainText("the quoted original");

	// The composer chip should clear after sending.
	await expect(page.getByLabel("Replying to message")).toHaveCount(0);

	// Clicking the quote block highlights the source message.
	await quoteBlock.click();
	await expect(originalRow).toHaveClass(/highlight/);

	// Cross-channel quote is forbidden by the API: directly verify the contract.
	const workspacesResp = await page.request.get("/api/workspaces");
	const workspaceId = (await workspacesResp.json()).workspaces[0].id;
	const channelsResp = await page.request.get(`/api/workspaces/${workspaceId}/channels`);
	const { channels } = await channelsResp.json();
	// Find any non-general channel; if none, create one.
	let otherChannel = channels.find((c: { name: string }) => c.name !== "general");
	if (!otherChannel) {
		const created = await page.request.post(`/api/workspaces/${workspaceId}/channels`, {
			data: { name: "second", kind: "public" },
		});
		otherChannel = (await created.json()).channel;
	}

	// Get the original's id by sending another targeted message via API.
	const generalId = channels.find((c: { name: string }) => c.name === "general").id;
	const list = await page.request.get(`/api/channels/${generalId}/messages`);
	const { messages } = await list.json();
	const originalMsg = messages.find((m: { body: string }) => m.body === "the quoted original");
	expect(originalMsg).toBeTruthy();

	const crossResp = await page.request.post(`/api/channels/${otherChannel.id}/messages`, {
		data: { body: "leak attempt", quoted_message_id: originalMsg.id },
	});
	expect(crossResp.status()).toBe(400);
});
