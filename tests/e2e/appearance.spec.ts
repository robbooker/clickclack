import { expect, test } from "@playwright/test";

// Appearance prefs are device-local: the settings modal writes localStorage
// and data attributes on <html>; base.css maps those to color-scheme and
// board token overrides. These tests drive the real UI and assert the
// attribute + persistence contract.

async function openAppearanceSettings(page: import("@playwright/test").Page) {
  await page.goto("/app");
  await page.getByRole("button", { name: /account settings/i }).click();
  await page.getByRole("button", { name: "Appearance" }).click();
  await expect(page.getByRole("heading", { name: "Appearance" })).toBeVisible();
}

test("forced color mode applies instantly and survives reload", async ({ page }) => {
  await openAppearanceSettings(page);

  const html = page.locator("html");
  await expect(html).not.toHaveAttribute("data-color-mode");

  await page.getByRole("radio", { name: "Dark" }).click();
  await expect(html).toHaveAttribute("data-color-mode", "dark");
  // color-scheme pins to dark, so light-dark() resolves the dark background.
  await expect
    .poll(() => page.evaluate(() => getComputedStyle(document.documentElement).colorScheme))
    .toBe("dark");

  await page.reload();
  // The app.html boot script applies the stored mode before hydration.
  await expect(html).toHaveAttribute("data-color-mode", "dark");

  await openAppearanceSettings(page);
  await page.getByRole("radio", { name: "System" }).click();
  await expect(html).not.toHaveAttribute("data-color-mode");
});

test("board theme retunes the app palette and survives reload", async ({ page }) => {
  await openAppearanceSettings(page);

  const html = page.locator("html");
  const accentOf = () =>
    page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--accent").trim(),
    );
  const signalAccent = await accentOf();

  await page.getByRole("radio", { name: /^Ember/ }).click();
  await expect(html).toHaveAttribute("data-board", "ember");
  await expect.poll(accentOf).not.toBe(signalAccent);

  await page.reload();
  await expect(html).toHaveAttribute("data-board", "ember");

  await openAppearanceSettings(page);
  await page.getByRole("radio", { name: /^Signal/ }).click();
  await expect(html).not.toHaveAttribute("data-board");
  await expect.poll(accentOf).toBe(signalAccent);
});
