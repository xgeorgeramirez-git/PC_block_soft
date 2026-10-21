import { test, expect, type Page } from "@playwright/test";
import { mockScript } from "./mockBundle";
import type { MockChild, MockOptions } from "../src/__mocks__/backend";

// Окно родителя в браузере с подменной программой (src/__mocks__/backend.ts).
// Снимки лежат в frontend/screens/ и уходят в ветку screenshots.

const SCREENS = "screens";

async function mockProgram(page: Page, options: MockOptions = {}) {
  // Помощник по умолчанию выключен: его карточки появляются с задержкой и сдвигают страницу, а снимки и
  // проверки остальных экранов должны быть одинаковыми при каждом прогоне. Свои снимки Помощника
  // (s07, s08) включают его явно.
  await page.addInitScript({ content: await mockScript({ helper: { enabled: false }, ...options }) });
}

/** Ни окно, ни прокручиваемая область страницы не уходят вбок. */
async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => {
    const wider = (el: Element | null) => (el ? el.scrollWidth > el.clientWidth : false);
    return wider(document.documentElement) || wider(document.querySelector(".content"));
  });
  expect(overflow).toBe(false);
}

/**
 * Окно приложения не растёт вместе со страницей: длинные страницы прокручиваются внутри.
 * Для снимка длинной страницы целиком окно временно делаем выше.
 */
async function tallWindow(page: Page, height: number) {
  await page.setViewportSize({ width: 960, height });
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

// Один и тот же набор детей на «Сегодня», в «Детях» и в «Настройках»: Миша (мало времени, жёлтый),
// Даша (обход защиты, красный), Егор (можно, зелёный).
const family = (): MockChild[] => [
  child({}),
  child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 2100, blocked: true }),
  child({ childId: 3, sid: "S-1-5-21-111-222-333-1003", name: "Егор", usedS: 1800 }),
];

// s00 — вход родителя: название, версия от программы, без горизонтальной прокрутки.
test("s00 вход родителя", async ({ page }) => {
  await mockProgram(page);
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
  await mockProgram(page);
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

// s02 — «Сегодня»: статус словом и цветом (зелёный, жёлтый, красный), «Снять блокировку» при обходе (S3, S7).
test("s02 сегодня", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Сегодня" })).toBeVisible();
  await expect(page.getByRole("article", { name: "Миша" }).getByText("Осталось 10 мин")).toBeVisible();
  await expect(page.getByRole("article", { name: "Егор" }).getByText("Можно", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Снять блокировку" })).toBeVisible();
  // У ребёнка, заблокированного за обход, «+время» нет; «Снять блокировку», «Пауза» и
  // «В копилку» доступны всегда (ТЗ 8.3.2, M2).
  const blocked = page.getByRole("article", { name: "Даша" });
  await expect(blocked.getByRole("button", { name: "+15" })).toHaveCount(0);
  await expect(blocked.getByRole("button", { name: "Снять блокировку" })).toBeVisible();
  await expect(blocked.getByRole("button", { name: "Пауза" })).toBeVisible();
  await expect(page.getByRole("article", { name: "Егор" }).getByRole("button", { name: "+15" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s02-today.png`, fullPage: true });
});

test("s02 сегодня, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await expect(page.getByRole("article", { name: "Даша" })).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s02-today-dark.png`, fullPage: true });
});

// s02 — предупреждение о нечитаемых правилах (R18): человеческими словами и с кнопкой в «Дети».
test("s02 сегодня, правила не прочитаны", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: family(),
    notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
  });
  await page.goto("/");
  const alert = page.getByRole("alert");
  await expect(alert).toContainText("Не удалось прочитать правила, поэтому сейчас ограничения не действуют");
  await expect(alert).not.toContainText(/база данных|служб/i);
  await expect(alert.getByRole("button", { name: "Открыть «Дети»" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s02-today-notice.png`, fullPage: true });
});

// s03 — лимиты ребёнка: правка показывает нижнюю панель «Сохранить» (FR-19). Окно 960x640 как у родителя.
test("s03 лимиты, несохранённые изменения", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({})] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await expect(page.getByRole("table")).toBeVisible();
  await page.getByLabel("Часы, понедельник").fill("1");
  const bar = page.getByRole("region", { name: "Сохранение изменений" });
  await expect(bar.getByText("Есть несохранённые изменения")).toBeVisible();
  await expect(page.getByRole("button", { name: "Сохранить" })).toHaveCount(1);

  // Панель прижата к нижнему краю окна на всю ширину страницы и не накрывает таблицу (B-7):
  // каждая строка, доскролленная в область, не заходит под панель.
  const barBox = (await bar.boundingBox())!;
  expect(barBox.y + barBox.height).toBeCloseTo(640, 0);
  expect(barBox.x + barBox.width).toBeCloseTo(960, 0);
  expect(barBox.width).toBeGreaterThan(700);
  for (const day of ["Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"]) {
    const row = page.getByRole("row", { name: new RegExp(day) });
    await row.scrollIntoViewIfNeeded();
    const box = (await row.boundingBox())!;
    expect(box.y + box.height, `${day} закрыт панелью`).toBeLessThanOrEqual(barBox.y + 0.5);
  }
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s03-limits.png`, fullPage: true });

  // После «Сохранить» на той же панели - итог.
  await bar.getByRole("button", { name: "Сохранить" }).click();
  await expect(page.getByRole("region", { name: "Сохранение изменений" }).getByText("Сохранено. Новые правила уже действуют")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s03-limits-saved.png`, fullPage: true });
});

// s03 — несколько детей: вкладки отнимают место, страница прокручивается над панелью, суббота и воскресенье доступны.
test("s03 лимиты нескольких детей, панель не закрывает таблицу", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await expect(page.getByRole("tab", { name: "Миша" })).toBeVisible();
  await page.getByLabel("Часы, понедельник").fill("1");
  const bar = page.getByRole("region", { name: "Сохранение изменений" });
  await expect(bar).toBeVisible();
  const sunday = page.getByRole("row", { name: /Воскресенье/ });
  await sunday.scrollIntoViewIfNeeded();
  const barBox = (await bar.boundingBox())!;
  const box = (await sunday.boundingBox())!;
  expect(box.y + box.height).toBeLessThanOrEqual(barBox.y + 0.5);
  await expect(page.getByRole("row", { name: /Суббота/ })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: `${SCREENS}/s03-limits-family.png`, fullPage: true });
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
  // Длинная страница: окно выше, чтобы на снимке были все разделы.
  await tallWindow(page, 1500);
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: family(),
    // Учётки Windows: администратор, трое детей и одна свободная учётка для формы «Добавить».
    accounts: [
      { sid: "S-1-5-21-111-222-333-1000", name: "Анна", admin: true, child: false, current: true, canBeChild: false },
      ...["Миша", "Даша", "Егор", "Лиза"].map((name, i) => ({
        sid: `S-1-5-21-111-222-333-${1001 + i}`,
        name,
        admin: false,
        child: false,
        current: false,
        canBeChild: true,
      })),
    ],
    notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Настройки" }).click();
  const box = page.getByRole("region", { name: "Детские учётки" });
  await expect(box.getByRole("heading", { name: "Детские учётки" })).toBeVisible();
  // Те же трое детей, что на «Сегодня», каждый на своей карточке с учёткой Windows.
  const cards = box.getByRole("listitem");
  await expect(cards).toHaveCount(3);
  for (const name of ["Миша", "Даша", "Егор"]) {
    await expect(cards.filter({ hasText: name }).getByText(`Учётная запись Windows: ${name}`)).toBeVisible();
  }
  await expect(box.getByRole("heading", { name: "Добавить детскую учётку" })).toBeVisible();
  // В «Помощнике» только работающий переключатель.
  await expect(page.getByText(/в следующем обновлении/)).toHaveCount(0);
  await expect(page.getByRole("alert")).not.toContainText(/база данных|служб/i);
  await expect(page.getByText("Версия 0.0.1")).toHaveCount(2);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s05-settings.png`, fullPage: true });
});

// s06 — окна ребёнка (FR-12, FR-32): «Моя статистика» только для просмотра, «Первый вход» с правилами на сегодня.
test("s06 моя статистика ребёнка", async ({ page }) => {
  await mockProgram(page, { mode: "mystats", children: family(), myChildId: 1 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Моя статистика" })).toBeVisible();
  await expect(page.getByText("Только просмотр", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Итоги" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Мои правила" })).toBeVisible();
  // Ребёнок видит только своё: ни полей ввода, ни слов родительского окна, ни других детей.
  await expect(page.getByRole("textbox")).toHaveCount(0);
  await expect(page.getByText("Родительский контроль")).toHaveCount(0);
  await expect(page.getByText("Даша")).toHaveCount(0);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s06-my-stats.png`, fullPage: true });
});

test("s06 первый вход ребёнка", async ({ page }) => {
  await mockProgram(page, { mode: "firstrun", children: family(), myChildId: 1 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Теперь время за компьютером считается" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Понятно" })).toBeVisible();
  await expect(page.getByText("Родительский контроль")).toHaveCount(0);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s06-first-run.png`, fullPage: true });
});

// s07 — «Помощь»: туры, первые шаги с отметками, «Что нового».
test("s07 помощь", async ({ page }) => {
  await tallWindow(page, 1100);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await page.getByRole("button", { name: "Помощь" }).click();
  await expect(page.getByRole("heading", { name: "Помощь" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Обучение" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Первые шаги" }).getByText("Пароль родителя задан")).toBeVisible();
  await expect(page.getByRole("region", { name: "Что нового" })).toBeVisible();
  await expect(page.getByText(/в следующем обновлении/)).toHaveCount(0);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s07-help.png`, fullPage: true });
});

// s08 — Помощник включён: приглашение в тур, не перекрывающее окно, и сам тур с подсветкой.
test("s08 приглашение и тур", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family(), helper: {} });
  await page.goto("/");
  const invite = page.getByRole("status").filter({ hasText: "Короткий тур по окну родителя" });
  await expect(invite).toBeVisible();
  // Карточка в потоке страницы: меню по-прежнему доступно. Имя кнопки берём точное и только из меню:
  // «Нарушения сегодня: N» на главной тоже содержит это слово.
  await expect(page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Сегодня", exact: true })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s08-helper-invite.png`, fullPage: true });
  await invite.getByRole("button", { name: "Начать" }).click();
  const tour = page.getByRole("dialog", { name: "Короткий тур по окну родителя" });
  await expect(tour).toBeVisible();
  await expect(tour.getByRole("button", { name: "Пропустить тур" })).toBeVisible();
  await expect(page.locator(".tour-spot")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s08-helper-tour.png`, fullPage: true });
  await tour.getByRole("button", { name: "Пропустить тур" }).click();
  await expect(tour).toHaveCount(0);
});

// s08 — напоминание о риске на карточке ребёнка: «Без лимита» во все дни.
test("s08 напоминание на карточке ребёнка", async ({ page }) => {
  await tallWindow(page, 900);
  const unlimited = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: true }));
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({ limits: unlimited, usedS: 600 })],
    helper: { done: ["H-M1-01", "H-M1-03"] },
  });
  await page.goto("/");
  const card = page.getByRole("article", { name: "Миша" }).getByRole("status");
  await expect(card).toContainText("Похоже, у Миши нет ограничений ни в один день");
  await expect(card.getByRole("button", { name: "Задать лимиты" })).toBeVisible();
  await expect(card.getByRole("button", { name: "Больше не напоминать" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s08-helper-reminder.png`, fullPage: true });
});
