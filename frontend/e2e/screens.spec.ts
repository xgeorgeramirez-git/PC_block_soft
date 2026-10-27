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
// Даша (время вышло, красный), Егор (можно, зелёный).
const family = (): MockChild[] => [
  child({}),
  child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 7200 }),
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

// s02 — «Сегодня»: статус словом и цветом (зелёный, жёлтый, красный); обход больше не
// блокирует экран, кнопки сгруппированы, «Снять блокировку» нет (D-41, FB-5).
test("s02 сегодня", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Сегодня" })).toBeVisible();
  await expect(page.getByRole("article", { name: "Миша" }).getByText("Осталось 10 мин")).toBeVisible();
  await expect(page.getByRole("article", { name: "Егор" }).getByText("Можно", { exact: true })).toBeVisible();
  // Даша: время вышло (красный), «+время» доступно, «Снять блокировку» нигде нет (D-41).
  const dasha = page.getByRole("article", { name: "Даша" });
  await expect(dasha.getByText("Время вышло")).toBeVisible();
  await expect(dasha.getByRole("button", { name: "+15" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Снять блокировку" })).toHaveCount(0);
  // Кнопки «Сегодня» сгруппированы: у Миши есть группа «Доп. время», «Пауза», «Статистика» (FB-5).
  const misha = page.getByRole("article", { name: "Миша" });
  await expect(misha.getByText("Доп. время")).toBeVisible();
  await expect(misha.getByRole("button", { name: "−30" })).toBeVisible();
  await expect(misha.getByRole("button", { name: "Своё время…" })).toBeVisible();
  await expect(misha.getByRole("button", { name: "Пауза" })).toBeVisible();
  await expect(misha.getByRole("button", { name: "Статистика" })).toBeVisible();
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
      { atMs: now - 600_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "bypass", text: "Миша: попытка обойти защиту. Закрыт значок программы" },
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
  // Ребёнок видит только своё: единственное поле — промокод в «Бонусах» (R26), ни слов
  // родительского окна, ни других детей.
  await expect(page.getByRole("textbox")).toHaveCount(1);
  await expect(page.getByRole("region", { name: "Бонусы" }).getByRole("textbox", { name: "Промокод" })).toBeVisible();
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

// --- Фидбек-раунд 0.2.2 ---------------------------------------------------------------

const AUTO_BONUS = "Бонус за свободное время: 15 мин";
const ledger = (now: number) => [
  { atMs: now - 10 * 3600_000, tzOffsetMin: 180, deltaMin: 15, reason: AUTO_BONUS, balanceAfter: 45 },
  { atMs: now - 30 * 3600_000, tzOffsetMin: 180, deltaMin: 30, reason: "за уборку", balanceAfter: 30 },
];

// s09 — копилка: обнулить и убавить с подтверждением (D-46); вкладка «Бонусы» — автобонусы и промокоды (R25, R26).
test("s09 копилка: убавить с подтверждением", async ({ page }) => {
  await tallWindow(page, 900);
  const now = Date.now();
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ bonusBalance: 45, bonusLedger: ledger(now) })] });
  await page.goto("/");
  await page.getByRole("button", { name: "Копилка", exact: true }).click();
  const card = page.getByRole("article", { name: "Копилка, Миша" });
  await expect(card.locator(".bonus-balance")).toHaveText("Баланс: 45 мин");
  await expect(card.getByRole("button", { name: "Обнулить копилку" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s09-bonus.png`, fullPage: true });
  await card.getByRole("button", { name: "Убавить" }).click();
  const dialog = page.getByRole("dialog", { name: "Убавить копилку?" });
  await dialog.getByText("−30").click();
  await page.screenshot({ path: `${SCREENS}/s09-bonus-reduce-dialog.png` });
  await dialog.getByRole("button", { name: "Убавить на 30 мин" }).click();
  await expect(card.getByText("Убавлено 30 минут. Баланс 15 мин")).toBeVisible();
  await expect(card.getByText(/-30 мин: Убавлено родителем/)).toBeVisible();
});

test("s09 бонусы и промокоды", async ({ page }) => {
  await tallWindow(page, 1000);
  const now = Date.now();
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({ bonusBalance: 45, bonusLedger: ledger(now) }), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", bonusBalance: 0, bonusLedger: [] })],
    promos: [
      { codeDisplay: "pizza", minutes: 20, createdAtMs: now - 86400_000, usedAtMs: now - 3600_000, usedByChild: 2 },
    ],
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Копилка", exact: true }).click();
  await page.getByRole("tab", { name: "Бонусы" }).click();
  await expect(page.getByText("+15 мин за неизрасходованное время прошлого дня")).toBeVisible();
  await page.getByLabel("Код", { exact: true }).fill("a1234");
  await page.getByLabel("Минут", { exact: true }).fill("30");
  await page.getByRole("button", { name: "Создать промокод" }).click();
  await expect(page.getByRole("list", { name: "Действующие промокоды" }).getByText("a1234")).toBeVisible();
  await expect(page.getByRole("list", { name: "Использованные промокоды" }).getByText(/Даша/)).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s09-bonuses-promos.png`, fullPage: true });
});

// s10 — запретные часы: правка интервала с заполненными полями (D-45); снять запрет на сегодня (R24).
test("s10 правка запретного интервала", async ({ page }) => {
  const schedule = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, startMin: 21 * 60, endMin: 7 * 60 }));
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ schedule })] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await page.getByRole("tab", { name: "Запретные часы" }).click();
  await page.getByRole("button", { name: /Изменить интервал 21:00 до 07:00 в Понедельник/i }).click();
  await expect(page.getByLabel(/Новое начало запретного времени/)).toHaveValue("21:00");
  await expect(page.getByLabel(/Новый конец запретного времени/)).toHaveValue("07:00");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s10-interval-edit.png`, fullPage: true });
});

test("s10 снять запрет на сегодня", async ({ page }) => {
  await tallWindow(page, 900);
  const schedule = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, startMin: 21 * 60, endMin: 7 * 60 }));
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ usedS: 1800, schedule })] });
  await page.goto("/");
  const misha = page.getByRole("article", { name: "Миша" });
  await misha.getByRole("button", { name: "Снять запрет на сегодня" }).click();
  await expect(misha.getByRole("button", { name: "Вернуть запретные часы" })).toBeVisible();
  await expect(misha.getByText("Сегодня запретные часы не действуют").first()).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s10-no-forbidden.png`, fullPage: true });
});

// s06 — «Сегодня» окна ребёнка (D-53, ТЗ 8.2.6 п.1): остатки, лимит дня, запретные часы, копилка, кнопки
// «Попросить ещё» и «Взять из копилки», внизу мелко «Настройки для родителя».
const nightEveryDay = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, startMin: 21 * 60, endMin: 7 * 60 }));

test("s06 моя статистика: сегодня с кнопками", async ({ page }) => {
  // Окно прокручивается внутри, fullPage его не растягивает: высота с запасом, чтобы «Бонусы» и
  // поле промокода влезли в кадр целиком.
  await tallWindow(page, 2400); // страница целиком выше окна: липкая полоса стоит на своём месте, ничего не закрывает
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 45, schedule: nightEveryDay })],
    myChildId: 1,
  });
  await page.goto("/");
  const left = page.getByRole("region", { name: "Сколько осталось" });
  await expect(left.getByText("Осталось всего")).toBeVisible();
  await expect(left.getByText("Сегодня нельзя с 21:00 до 07:00")).toBeVisible();
  await expect(left.getByText("Копилка")).toBeVisible();
  await expect(page.getByRole("button", { name: "Попросить ещё" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Взять из копилки" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Настройки для родителя" })).toBeVisible();
  await expect(page.getByRole("textbox")).toHaveCount(1);
  await expect(page.getByRole("region", { name: "Бонусы" }).getByRole("textbox", { name: "Промокод" })).toBeInViewport({ ratio: 1 });
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s06-my-stats-today.png`, fullPage: true });
});

// s06 — окно ребёнка в размерах от 800 x 1200 до 800 x 480 и самом узком 640 x 520: оболочка это три части в
// потоке (шапка, область прокрутки, в ней же «Настройки для родителя» последним блоком), поэтому ничто
// не лежит поверх соседнего блока в верху, середине и низу прокрутки (замечание заказчика).
for (const [w, h] of [
  [800, 1200],
  [800, 800],
  [800, 640],
  [800, 480],
  [640, 520],
]) {
  test(`s06 моя статистика: блоки не пересекаются в окне ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, {
      mode: "mystats",
      children: [child({ bonusBalance: 45, schedule: nightEveryDay })],
      myChildId: 1,
    });
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Настройки для родителя" })).toBeVisible();
    const parts = [
      page.locator("header.topbar"),
      page.getByRole("region", { name: "Нужно больше времени?" }),
      page.getByRole("button", { name: "Попросить ещё" }),
      page.getByRole("button", { name: "Взять из копилки" }),
      page.getByRole("textbox", { name: "Промокод" }),
      page.getByRole("button", { name: "Ввести промокод" }),
      page.getByRole("button", { name: "Настройки для родителя" }),
    ];
    // Вложенные блоки (кнопка внутри карточки) пересекаться могут, как и положено: сравниваем только
    // блоки, из которых ни один не лежит внутри другого.
    const noOverlap = async () => {
      // Видимая часть блока: то, что осталось после обрезки областью прокрутки. Прокрученная за её край
      // часть не видна и шапку не перекрывает (шапка не входит в область прокрутки).
      const boxes = await Promise.all(
        parts.map((n, i) =>
          n.evaluate((el, isHead) => {
            const r = el.getBoundingClientRect();
            if (isHead) return { left: r.left, right: r.right, top: r.top, bottom: r.bottom };
            const c = document.querySelector("main.content")!.getBoundingClientRect();
            return {
              left: Math.max(r.left, c.left),
              right: Math.min(r.right, c.right),
              top: Math.max(r.top, c.top),
              bottom: Math.min(r.bottom, c.bottom),
            };
          }, i === 0),
        ),
      );
      type B = { left: number; right: number; top: number; bottom: number };
      const inside = (a: B, b: B) => a.left >= b.left && a.right <= b.right && a.top >= b.top && a.bottom <= b.bottom;
      for (let i = 0; i < boxes.length; i++) {
        for (let j = i + 1; j < boxes.length; j++) {
          const a = boxes[i];
          const b = boxes[j];
          if (inside(a, b) || inside(b, a)) continue;
          const apart = a.right <= b.left || b.right <= a.left || a.bottom <= b.top || b.bottom <= a.top;
          expect(apart, `блоки ${i} и ${j} пересекаются: ${JSON.stringify([a, b])}`).toBe(true);
        }
      }
    };
    const scroller = page.locator("main.content");
    // Область прокрутки начинается не выше низа шапки, шапка и страница не прокручиваются вместе.
    const head = await page.locator("header.topbar").evaluate((el) => el.getBoundingClientRect().bottom);
    expect(await scroller.evaluate((el) => el.getBoundingClientRect().top)).toBeGreaterThanOrEqual(head - 0.5);
    expect(await page.evaluate(() => document.scrollingElement?.scrollTop ?? 0)).toBe(0);
    const max = await scroller.evaluate((el) => el.scrollHeight - el.clientHeight);
    const shot = w === 800 && (h === 640 || h === 480) ? h : 0;
    for (const [label, frac] of [["top", 0], ["mid", 0.5], ["bottom", 1]] as const) {
      await scroller.evaluate((el, top) => el.scrollTo(0, top), Math.round(max * frac));
      await noOverlap();
      expect(await page.evaluate(() => document.scrollingElement?.scrollTop ?? 0)).toBe(0);
      if (shot) await page.screenshot({ path: `${SCREENS}/s06-my-stats-${shot}-${label}.png` });
    }
    // Низ: «Настройки для родителя» достижима прокруткой и стоит под всем содержимым.
    const link = page.getByRole("button", { name: "Настройки для родителя" });
    await expect(link).toBeInViewport({ ratio: 1 });
    await expect(page.getByText("Когда время выйдет, экран закроется блокировкой.", { exact: false })).toBeInViewport({ ratio: 1 });
    await noHorizontalScroll(page);
    if (w === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-small.png` });
  });
}

// s06 — панель «Позови родителя» (FR-10): пароль родителя и +15 / +30 / +60 / своё число.
test("s06 панель «Позови родителя»", async ({ page }) => {
  await mockProgram(page, { mode: "mystats", children: [child({ bonusBalance: 45 })], myChildId: 1 });
  await page.goto("/");
  await page.getByRole("button", { name: "Попросить ещё" }).click();
  const dialog = page.getByRole("dialog", { name: "Позови родителя" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("Пароль родителя")).toBeFocused();
  await expect(dialog.getByRole("button", { name: "+30" })).toBeVisible();
  await expect(dialog.getByLabel("Своё число минут")).toBeVisible();
  await dialog.getByLabel("Пароль родителя").fill("не тот");
  await dialog.getByRole("button", { name: "+15" }).click();
  await expect(dialog.getByText("Неверный пароль. Осталось попыток: 4")).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s06-ask-more.png` });
  await dialog.getByLabel("Пароль родителя").fill("секрет-123");
  await dialog.getByRole("button", { name: "+30" }).click();
  await expect(page.getByText("Добавлено 30 минут")).toBeVisible();
});

// s06 — «Взять из копилки» (FR-11): выбор суммы и подтверждение с остатком.
test("s06 «Взять из копилки»: подтверждение", async ({ page }) => {
  // То же состояние, что в соседних кадрах: запретные часы 21:00 до 07:00 (сейчас день), копилка 45 мин.
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 45, schedule: nightEveryDay })],
    myChildId: 1,
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Взять из копилки" }).click();
  const dialog = page.getByRole("dialog", { name: "Взять из копилки" });
  await expect(dialog.getByRole("button", { name: "Всё: 45 мин" })).toBeVisible();
  // Варианты из FR-11: 15, 30 и весь баланс (60 больше баланса 45, его нет).
  await expect(dialog.getByRole("button", { name: "15 мин" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "30 мин" })).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s06-spend-choose.png` });
  await dialog.getByRole("button", { name: "30 мин" }).click();
  await expect(dialog.getByText("Взять из копилки 30 мин? Останется 15 мин")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s06-spend-bonus.png` });
  await dialog.getByRole("button", { name: "Взять" }).click();
  await expect(page.getByText("Взято из копилки 30 мин")).toBeVisible();
});

// s06 — выбор суммы при большой копилке: видны все четыре варианта 15 / 30 / 60 / всё.
test("s06 «Взять из копилки»: выбор суммы при копилке 90 мин", async ({ page }) => {
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 90, schedule: nightEveryDay })],
    myChildId: 1,
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Взять из копилки" }).click();
  const dialog = page.getByRole("dialog", { name: "Взять из копилки" });
  for (const name of ["15 мин", "30 мин", "60 мин", "Всё: 90 мин"]) {
    await expect(dialog.getByRole("button", { name })).toBeVisible();
  }
  await page.screenshot({ path: `${SCREENS}/s06-spend-choose-big.png` });
});

// s06 — запретные часы: «Взять из копилки» недоступна и называет час (урок L-17).
test("s06 копилка в запретные часы недоступна с пояснением", async ({ page }) => {
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 45, schedule: nightEveryDay, forbiddenUntil: "Можно снова: сегодня в 07:00" })],
    myChildId: 1,
  });
  await tallWindow(page, 2400);
  await page.goto("/");
  await expect(page.getByRole("button", { name: "Взять из копилки" })).toHaveAttribute("aria-disabled", "true");
  await expect(page.getByText("Из копилки можно взять с 07:00")).toBeVisible();
  await expect(page.getByText("Сегодня нельзя с 21:00 до 07:00")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s06-spend-forbidden.png`, fullPage: true });
});

// s06 — «Бонусы» у ребёнка: копилка, автобонус простыми словами, промокод (R25, R26).
test("s06 бонусы ребёнка и промокод", async ({ page }) => {
  await tallWindow(page, 1000);
  const now = Date.now();
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 45, bonusLedger: ledger(now) })],
    myChildId: 1,
    promos: [{ codeDisplay: "A1234", minutes: 30, createdAtMs: now - 3600_000 }],
  });
  await page.goto("/");
  const box = page.getByRole("region", { name: "Бонусы" });
  await expect(box.getByText("Копилка: 45 мин")).toBeVisible();
  await expect(box.getByText("+15 мин за неизрасходованное время прошлого дня")).toBeVisible();
  await box.getByRole("textbox", { name: "Промокод" }).fill("a1234");
  await box.getByRole("button", { name: "Ввести промокод" }).click();
  await expect(box.getByText("Добавлено 30 минут")).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s06-my-bonuses.png`, fullPage: true });
});
