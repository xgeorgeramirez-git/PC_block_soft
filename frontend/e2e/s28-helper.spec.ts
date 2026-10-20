import { test, expect, type Locator, type Page } from "@playwright/test";
import { mockScript } from "./mockBundle";
import type { MockChild, MockOptions } from "../src/__mocks__/backend";

// S28 (ТЗ 11): Помощник в окне родителя, сквозной сценарий в браузере с подменной программой.
// Дано: мастер пройден, окно родителя открыто впервые. Тогда: одно приглашение в тур и больше ничего.
// Когда: «Позже», затем «Дети → Лимиты». Тогда: карточка у таблицы лимитов; «Позже» убирает её до следующего
// открытия окна, «Больше не напоминать» (у напоминаний о риске) убирает навсегда; у ребёнка «Без лимита»
// во все дни напоминание с кнопкой «Задать лимиты». Когда: подсказки выключены. Тогда: карточек нет,
// а «Помощь» и тур открываются.
//
// Здесь только недостающее после e2e/screens.spec.ts (s08: приглашение, тур, напоминание на карточке).
// «Следующее открытие окна» в браузере это новая страница с тем же сохранённым состоянием Помощника,
// которое программа хранит в базе (GetHelperState / SaveHelperState).

const SCREENS = "screens";

async function openWindow(page: Page, options: MockOptions = {}) {
  await page.addInitScript({
    content: await mockScript({ mode: "parent", sessionActive: true, children: [child({})], helper: {}, ...options }),
  });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Сегодня" })).toBeVisible();
}

const everyDay = (totalMin: number, unlimited = false) =>
  [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin, unlimited }));

const child = (over: Partial<MockChild>): MockChild => ({
  childId: 1,
  sid: "S-1-5-21-111-222-333-1001",
  name: "Миша",
  limits: everyDay(120),
  usedS: 1800,
  extraMin: 0,
  blocked: false,
  blockDetail: "",
  ...over,
});

/** Меню разделов; имя точное: «Нарушения сегодня» на главной тоже содержит слово «Сегодня». */
const menu = (page: Page, name: string): Locator =>
  page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name, exact: true });

/** Все карточки подсказок на экране. */
const cards = (page: Page): Locator => page.locator("aside.hint-card");
const card = (page: Page, id: string): Locator => page.locator(`aside.hint-card[data-hint-id="${id}"]`);

/** Карточки появляются через короткую паузу после открытия экрана: «ничего нет» проверяем после неё. */
const SETTLE_MS = 1500;

async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => {
    const wider = (el: Element | null) => (el ? el.scrollWidth > el.clientWidth : false);
    return wider(document.documentElement) || wider(document.querySelector(".content"));
  });
  expect(overflow).toBe(false);
}

/** Состояние Помощника так, как его сохранила программа (то, что переживёт закрытие окна). */
async function helperState(page: Page): Promise<Record<string, unknown>> {
  const raw = await page.evaluate(async () => {
    const w = window as unknown as { go: { ui: { App: { Call(command: string, payload: string): Promise<string> } } } };
    return w.go.ui.App.Call("GetHelperState", "{}");
  });
  const reply = JSON.parse(raw) as { ok: boolean; payload: { state: Record<string, unknown> } };
  expect(reply.ok).toBe(true);
  return reply.payload.state;
}

/** Ждёт, пока в сохранённом состоянии появится запись: сохранение идёт после нажатия, не вместе с ним. */
async function savedState(page: Page, field: string, key: string): Promise<Record<string, unknown>> {
  await expect
    .poll(async () => {
      const s = await helperState(page);
      const v = s[field];
      return Array.isArray(v) ? v.includes(key) : typeof v === "object" && v !== null && key in (v as object);
    })
    .toBe(true);
  return helperState(page);
}

test.describe("S28 Помощник", () => {
  // Приглашение один раз и больше ничего; «Позже»; на «Дети» карточка стоит у таблицы лимитов.
  test("s28 приглашение в тур, «Позже», карточка у лимитов", async ({ page, context }) => {
    await openWindow(page);

    // Первое открытие окна: ровно одна карточка, приглашение в тур. Ни подсказки про кнопки «+15», ни других.
    await expect(cards(page)).toHaveCount(1);
    await expect(card(page, "H-M1-01")).toContainText("Короткий тур по окну родителя");
    await page.waitForTimeout(SETTLE_MS);
    await expect(cards(page)).toHaveCount(1);

    // «Позже»: приглашение уходит, окно по-прежнему работает.
    await card(page, "H-M1-01").getByRole("button", { name: "Позже" }).click();
    await expect(card(page, "H-M1-01")).toHaveCount(0);

    // «Дети → Лимиты»: подсказка про дни недели стоит над таблицей лимитов, а не поверх неё.
    await menu(page, "Дети").click();
    await expect(page.getByRole("heading", { name: /Лимиты времени/ })).toBeVisible();
    const hint = card(page, "H-M1-02");
    await expect(hint).toBeVisible();
    await expect(hint).toContainText("Лимиты по дням недели");
    const table = page.locator("table.limits-table");
    await expect(table).toBeVisible();
    const hintBox = await hint.boundingBox();
    const tableBox = await table.boundingBox();
    expect(hintBox).not.toBeNull();
    expect(tableBox).not.toBeNull();
    expect(hintBox!.y + hintBox!.height).toBeLessThanOrEqual(tableBox!.y + 1);
    await noHorizontalScroll(page);
    await page.screenshot({ path: `${SCREENS}/s28-helper-limits.png`, fullPage: true });

    // «Позже» на этой карточке: в этом же окне она не возвращается, ни на «Дети», ни после других разделов.
    await hint.getByRole("button", { name: "Позже" }).click();
    await expect(cards(page)).toHaveCount(0);
    await menu(page, "Сегодня").click();
    await menu(page, "Дети").click();
    await page.waitForTimeout(SETTLE_MS);
    await expect(card(page, "H-M1-02")).toHaveCount(0);

    // Следующее открытие окна: оба «Позже» кончились, приглашение и подсказка про лимиты возвращаются по разу.
    // Журнал показов (не больше 2 подсказок в час) очищаем: между окнами прошло больше часа.
    const state = await savedState(page, "snoozed", "H-M1-02");
    expect(Object.keys(state.snoozed as object).sort()).toEqual(["H-M1-01", "H-M1-02"]);
    const next = await context.newPage();
    await openWindow(next, { helper: { ...state, log: {} } });
    await expect(card(next, "H-M1-01")).toBeVisible();
    await card(next, "H-M1-01").getByRole("button", { name: "Позже" }).click();
    await menu(next, "Дети").click();
    await expect(card(next, "H-M1-02")).toBeVisible();
    await next.screenshot({ path: `${SCREENS}/s28-helper-limits-again.png`, fullPage: true });
  });

  // У ребёнка «Без лимита» во все дни: напоминание с кнопкой «Задать лимиты»; «Позже» и «Больше не напоминать».
  test("s28 «без лимитов»: напоминание, «Позже», «Больше не напоминать»", async ({ page, context }) => {
    const unlimited = child({ limits: everyDay(0, true), usedS: 600 });
    const done = { done: ["H-M1-01", "H-M1-03"] };
    await openWindow(page, { children: [unlimited], helper: done });

    const reminder = card(page, "D-M1-01");
    await expect(reminder).toContainText("Похоже, у Миши нет ограничений ни в один день");
    await expect(cards(page)).toHaveCount(1);
    await expect(reminder.getByRole("button", { name: "Задать лимиты" })).toBeVisible();
    await expect(reminder.getByRole("button", { name: "Позже" })).toBeVisible();
    await expect(reminder.getByRole("button", { name: "Больше не напоминать" })).toBeVisible();
    // Напоминание стоит на карточке этого ребёнка, а не над всем окном.
    await expect(page.getByRole("article", { name: "Миша" }).locator("aside.hint-card")).toHaveCount(1);
    await noHorizontalScroll(page);
    await page.screenshot({ path: `${SCREENS}/s28-helper-nolimits.png`, fullPage: true });

    // «Позже»: до следующего открытия окна; риск остаётся, поэтому в новом окне напоминание возвращается.
    await reminder.getByRole("button", { name: "Позже" }).click();
    await expect(cards(page)).toHaveCount(0);
    await page.waitForTimeout(SETTLE_MS);
    await expect(cards(page)).toHaveCount(0);
    const afterLater = await savedState(page, "snoozed", "D-M1-01:1");

    const second = await context.newPage();
    await openWindow(second, { children: [unlimited], helper: { ...afterLater, log: {} } });
    await expect(card(second, "D-M1-01")).toBeVisible();

    // «Больше не напоминать»: навсегда. Не возвращается ни в этом окне, ни в следующем.
    await card(second, "D-M1-01").getByRole("button", { name: "Больше не напоминать" }).click();
    await expect(cards(second)).toHaveCount(0);
    await menu(second, "Дети").click();
    await menu(second, "Сегодня").click();
    await second.waitForTimeout(SETTLE_MS);
    await expect(cards(second)).toHaveCount(0);
    const afterMute = await savedState(second, "muted", "D-M1-01:1");

    const third = await context.newPage();
    await openWindow(third, { children: [unlimited], helper: { ...afterMute, log: {} } });
    await third.waitForTimeout(SETTLE_MS);
    await expect(cards(third)).toHaveCount(0);

    // «Задать лимиты» в окне, где напоминание ещё не выключено, ведёт на «Дети» к нужному ребёнку.
    const fourth = await context.newPage();
    await openWindow(fourth, { children: [unlimited], helper: done });
    await card(fourth, "D-M1-01").getByRole("button", { name: "Задать лимиты" }).click();
    await expect(fourth.getByRole("heading", { name: "Лимиты времени, Миша" })).toBeVisible();
  });

  // Подсказки выключены: карточек нет нигде, а «Помощь» и тур открываются.
  test("s28 подсказки выключены: карточек нет, «Помощь» и тур работают", async ({ page }) => {
    await openWindow(page, { children: [child({ limits: everyDay(0, true) })], helper: { enabled: false } });
    await page.waitForTimeout(SETTLE_MS);
    await expect(cards(page)).toHaveCount(0);
    await menu(page, "Дети").click();
    await expect(page.getByRole("heading", { name: /Лимиты времени/ })).toBeVisible();
    await page.waitForTimeout(SETTLE_MS);
    await expect(cards(page)).toHaveCount(0);

    await page.getByRole("button", { name: "Помощь", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Помощь" })).toBeVisible();
    await expect(page.getByText(/Подсказки в окне сейчас выключены/)).toBeVisible();
    await noHorizontalScroll(page);
    await page.screenshot({ path: `${SCREENS}/s28-helper-off.png`, fullPage: true });

    await page.getByRole("region", { name: "Обучение" }).getByRole("button", { name: "Начать" }).click();
    const tour = page.getByRole("dialog", { name: "Короткий тур по окну родителя" });
    await expect(tour).toBeVisible();
    await expect(tour.getByRole("button", { name: "Пропустить тур" })).toBeVisible();
    await tour.getByRole("button", { name: "Пропустить тур" }).click();
    await expect(tour).toHaveCount(0);
    await expect(cards(page)).toHaveCount(0);
  });
});
