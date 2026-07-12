import { expect, test } from "@playwright/test";

type RGB = [number, number, number];

function parseColor(value: string): RGB {
  const color = value.trim();
  if (/^#[0-9a-f]{3}$/i.test(color)) {
    return [1, 2, 3].map((index) => Number.parseInt(`${color[index]}${color[index]}`, 16)) as RGB;
  }
  if (/^#[0-9a-f]{6}$/i.test(color)) {
    return [
      Number.parseInt(color.slice(1, 3), 16),
      Number.parseInt(color.slice(3, 5), 16),
      Number.parseInt(color.slice(5, 7), 16),
    ];
  }
  const rgb = color.match(/^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/i);
  if (!rgb) throw new Error(`unsupported color: ${value}`);
  return [Number(rgb[1]), Number(rgb[2]), Number(rgb[3])];
}

function relativeLuminance(color: RGB): number {
  const [red, green, blue] = color.map((channel) => {
    const value = channel / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

function contrastRatio(foreground: string, background: string): number {
  const values = [
    relativeLuminance(parseColor(foreground)),
    relativeLuminance(parseColor(background)),
  ];
  const lighter = Math.max(...values);
  const darker = Math.min(...values);
  return (lighter + 0.05) / (darker + 0.05);
}

for (const colorScheme of ["light", "dark"] as const) {
  test(`brand gradient text meets WCAG contrast in ${colorScheme} mode`, async ({ page }) => {
    await page.emulateMedia({ colorScheme });
    await page.goto("/app");
    const home = page.locator(".guild-rail .guild.home");
    await expect(home).toBeVisible();
    const readAppearance = () =>
      home.evaluate((element) => {
        const root = getComputedStyle(document.documentElement);
        const style = getComputedStyle(element);
        return {
          backgroundImage: style.backgroundImage,
          color: style.color,
          filter: style.filter,
          foreground: root.getPropertyValue("--brand-contrast"),
          firstStop: root.getPropertyValue("--brand-a"),
          secondStop: root.getPropertyValue("--brand-b"),
        };
      });
    type Appearance = Awaited<ReturnType<typeof readAppearance>>;
    let appearance: Appearance | undefined;
    await expect
      .poll(async () => {
        const candidate = await readAppearance();
        const ready = [
          candidate.backgroundImage,
          candidate.color,
          candidate.foreground,
          candidate.firstStop,
          candidate.secondStop,
        ].every((value) => value.trim().length > 0);
        if (ready) appearance = candidate;
        return ready;
      })
      .toBe(true);
    if (!appearance) throw new Error("brand appearance did not settle");

    expect(contrastRatio(appearance.foreground, appearance.firstStop)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(appearance.foreground, appearance.secondStop)).toBeGreaterThanOrEqual(4.5);
    expect(parseColor(appearance.color)).toEqual(parseColor(appearance.foreground));

    let hoverAppearance: Appearance | undefined;
    await expect
      .poll(async () => {
        await home.hover();
        const candidate = await readAppearance();
        const ready =
          candidate.backgroundImage.trim().length > 0 && candidate.color.trim().length > 0;
        if (ready) hoverAppearance = candidate;
        return ready;
      })
      .toBe(true);
    if (!hoverAppearance) throw new Error("brand hover appearance did not settle");
    expect(hoverAppearance.backgroundImage).toBe(appearance.backgroundImage);
    expect(parseColor(hoverAppearance.color)).toEqual(parseColor(appearance.foreground));
    expect(hoverAppearance.filter).toBe("none");
  });
}
