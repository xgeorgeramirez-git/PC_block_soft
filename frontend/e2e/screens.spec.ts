import { test, expect, type Page } from "@playwright/test";

// Подмена программы: в браузере нет Wails, поэтому методы App отдаём сами.
async function mockProgram(page: Page, version: string) {
  await page.addInitScript((v) => {
    (window as unknown as { go: unknown }).go = {
      ui: { App: { Version: () => Promise.resolve(v) } },
    };
  }, version);
}

const SCREENS = "screens";

// s00 — окно-скелет (Фаза 0): название, версия от программы, без горизонтальной прокрутки.
test("s00 окно-скелет", async ({ page }) => {
  await mockProgram(page, "0.0.1");
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("PC Block");
  await expect(page.getByText("Версия 0.0.1")).toBeVisible();
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);
  await page.screenshot({ path: `${SCREENS}/s00-skeleton-window.png`, fullPage: true });
});

// s00-dark — то же окно в тёмной теме Windows.
test("s00 окно-скелет, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await mockProgram(page, "0.0.1");
  await page.goto("/");
  await expect(page.getByText("Версия 0.0.1")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s00-skeleton-window-dark.png`, fullPage: true });
});
