import { test, expect, type Page } from "@playwright/test";
import { mockScript } from "./mockBundle";
import type { MockChild, MockOptions } from "../src/__mocks__/backend";

// Окно родителя в браузере с подменной программой (src/__mocks__/backend.ts).
// Снимки лежат в frontend/screens/ и уходят в ветку screenshots.

const SCREENS = "screens";

async function mockProgram(page: Page, options: MockOptions = {}) {
  await page.addInitScript({ content: await mockScript(options) });
}

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
  expect(overflow).toBe(false);
}

/** Лимит 2 ч на каждый день: снимок не зависит от дня недели, в который идёт сборка. */
const everyDay = (totalMin: number) =>
  [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin, unlimited: false }));

const child = (over: Partial<MockChild>): MockChild => ({
  childId: 1,
  sid: "S-1-5-21-111-222-333-1001",
  name: "Миша",
  limits: everyDay(120),
  usedS: 6600,
  extraMin: 0,
  blocked: false,
  blockDetail: "Изменено системное время",
  ...over,
});

const family = (): MockChild[] => [
  child({}),
  child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 2100, blocked: true }),
];

// s00 — вход родителя: название, версия от программы, без горизонтальной прокрутки.
test("s00 вход родителя", async ({ page }) => {
  await mockProgram(page, { version: "0.0.1" });
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("PC Block");
  await expect(page.getByText("Версия 0.0.1")).toBeVisible();
  await expect(page.getByLabel("Пароль родителя")).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s00-login.png`, fullPage: true });
});

// s00-dark — тот же вход в тёмной теме Windows.
test("s00 вход родителя, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await mockProgram(page, { version: "0.0.1" });
  await page.goto("/");
  await expect(page.getByLabel("Пароль родителя")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s00-login-dark.png`, fullPage: true });
});

// s01 — мастер, шаг 1 (S1).
test("s01 мастер, шаг пароля", async ({ page }) => {
  await mockProgram(page, { configured: false, mode: "wizard" });
  await page.goto("/");
  await expect(page.getByText("Шаг 1 из 3")).toBeVisible();
  await page.getByLabel("Пароль родителя").fill("123");
  await page.getByLabel("Повторите пароль").fill("123");
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Не меньше 6 символов", { exact: true }).last()).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s01-wizard-password.png`, fullPage: true });
});

// s02 — «Сегодня»: статус словом и цветом, «Снять блокировку» при обходе (S3, S7).
test("s02 сегодня", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Сегодня" })).toBeVisible();
  await expect(page.getByRole("article", { name: "Миша" }).getByText("Осталось 10 мин")).toBeVisible();
  await expect(page.getByRole("button", { name: "Снять блокировку" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s02-today.png`, fullPage: true });
});

test("s02 сегодня, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await expect(page.getByRole("article", { name: "Даша" })).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s02-today-dark.png`, fullPage: true });
});

// s03 — лимиты ребёнка: правка показывает нижнюю панель «Сохранить» (FR-19).
test("s03 лимиты, несохранённые изменения", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({})] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await expect(page.getByRole("table")).toBeVisible();
  await page.getByLabel("Часы, понедельник").fill("1");
  await expect(page.getByText("Есть несохранённые изменения")).toBeVisible();
  await expect(page.getByRole("button", { name: "Сохранить" })).toHaveCount(1);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s03-limits.png`, fullPage: true });
});

// s04 — журнал и настройки.
test("s04 журнал", async ({ page }) => {
  const now = Date.now();
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({})],
    events: [
      { atMs: now - 600_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "bypass", text: "Миша: попытка обойти защиту. Экран заблокирован" },
      { atMs: now - 3_600_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "extra_time", text: "Миша: доп. время +30 мин" },
    ],
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Журнал" }).click();
  await expect(page.getByText("Миша: доп. время +30 мин")).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s04-journal.png`, fullPage: true });
});

test("s05 настройки", async ({ page }) => {
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({})],
    version: "0.1.0",
    notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Настройки" }).click();
  await expect(page.getByRole("heading", { name: "Детские учётки" })).toBeVisible();
  await expect(page.getByText("Версия 0.1.0").last()).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s05-settings.png`, fullPage: true });
});
