import { test, expect, type Page } from "@playwright/test";
import { mockScript } from "./mockBundle";
import type { MockChild, MockOptions } from "../src/__mocks__/backend";

// Окно родителя в браузере с подменной программой (src/__mocks__/backend.ts).
// Снимки лежат в frontend/screens/ и уходят в ветку screenshots.

const SCREENS = "screens";

async function mockProgram(page: Page, options: MockOptions = {}) {
  await page.addInitScript({ content: await mockScript(options) });
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
  await expect(page.getByText("Шаг 1 из 4")).toBeVisible();
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
  // Ни обещаний на будущее, ни раздела Помощника (D-56).
  await expect(page.getByText(/в следующем обновлении/)).toHaveCount(0);
  await expect(page.getByRole("region", { name: "Помощник" })).toHaveCount(0);
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

// s07 — «Что нового» (FR-36): внизу меню, версии от новой к старой, только текст.
test("s07 что нового", async ({ page }) => {
  await tallWindow(page, 1100);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family() });
  await page.goto("/");
  await page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Что нового" }).click();
  await expect(page.getByRole("heading", { name: "Что нового", level: 2 })).toBeVisible();
  await expect(page.getByRole("heading", { level: 3 }).first()).toHaveText("Версия 0.3.0. Программы и игры");
  await expect(page.locator("main.content").getByRole("button")).toHaveCount(0);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s07-whatsnew.png`, fullPage: true });
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
    await expect(page.getByText("Когда время вышло: экран блокировки", { exact: true })).toBeInViewport({ ratio: 1 });
    await noHorizontalScroll(page);
    if (w === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-small.png` });
  });
}

// s06 — зазор между шапкой «PC Block» и первым блоком окна ребёнка (замечание заказчика: «Ввести
// промокод» сливается с названием): не меньше 16 px при масштабе 1 и 1,5, в трёх высотах окна и на любой прокрутке.
for (const dsf of [1, 1.5]) {
  test.describe(`зазор под шапкой, масштаб ${dsf}`, () => {
    test.use({ deviceScaleFactor: dsf });
    for (const h of [1200, 640, 480]) {
      test(`s06 моя статистика: под шапкой зазор 16 px в окне 800 x ${h}`, async ({ page }) => {
        await page.setViewportSize({ width: 800, height: h });
        await mockProgram(page, {
          mode: "mystats",
          children: [child({ bonusBalance: 45, schedule: nightEveryDay })],
          myChildId: 1,
        });
        await page.goto("/");
        const head = page.locator("header.topbar");
        await expect(head).toBeVisible();
        expect(await head.evaluate((el) => getComputedStyle(el).borderBottomWidth)).not.toBe("0px");
        expect(await head.evaluate((el) => getComputedStyle(el).boxShadow)).not.toBe("none");
        const scroller = page.locator("main.content");
        const gap = async () =>
          scroller.evaluate((el) => {
            const bottom = document.querySelector("header.topbar")!.getBoundingClientRect().bottom;
            return el.getBoundingClientRect().top - bottom;
          });
        expect(await gap()).toBeGreaterThanOrEqual(16);
        // Первый блок содержимого в покое отстоит от шапки не меньше чем на 16 px.
        const first = await page.locator("main.content .my-stats .page-head").evaluate((el) => el.getBoundingClientRect().top);
        const bottom = await head.evaluate((el) => el.getBoundingClientRect().bottom);
        expect(first - bottom).toBeGreaterThanOrEqual(16);
        const max = await scroller.evaluate((el) => el.scrollHeight - el.clientHeight);
        for (const y of [0, Math.round(max / 2), max]) {
          await scroller.evaluate((el, top) => el.scrollTo(0, top), y);
          expect(await gap()).toBeGreaterThanOrEqual(16);
        }
        await scroller.evaluate((el) => el.scrollTo(0, 0));
        if (h === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-640-top${dsf === 1 ? "" : "-150"}.png` });
      });
    }
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

// --- M3: экран «Программы» (FR-20, 8.3.4) -----------------------------------------------

const H = 3600_000;
const pendingPrograms = (now: number) => [
  {
    path: "c:\\users\\миша\\downloads\\supergame.exe",
    name: "SuperGame",
    publisher: "Без подписи издателя",
    description: "Super Game Deluxe",
    attempts: 4,
    lastAttemptAt: now - 2 * H,
    wasCategory: "games",
    byChild: [
      { childId: 1, name: "Миша", attempts: 3 },
      { childId: 2, name: "Даша", attempts: 1 },
    ],
  },
  {
    path: "c:\\users\\даша\\appdata\\local\\programs\\очень длинное название папки тренажёра по математике\\версия 2026.10.1\\тренажёр таблицы умножения.exe",
    name: "Тренажёр таблицы умножения для начальной школы",
    publisher: "",
    description: "",
    attempts: 2,
    lastAttemptAt: now - 26 * H,
    wasCategory: "",
    byChild: [{ childId: 2, name: "Даша", attempts: 2 }],
  },
];

const programList = () => [
  { path: "c:\\program files\\bravesoftware\\brave-browser\\application\\brave.exe", name: "Brave", publisher: "Brave Software, Inc.", category: "other" as const, source: "snapshot" as const, weekS: 5400 },
  { path: "c:\\program files\\minecraft launcher\\minecraftlauncher.exe", name: "Minecraft Launcher", publisher: "Mojang", category: "games" as const, source: "catalog" as const, weekS: 7500 },
  { path: "c:\\program files (x86)\\steam\\steamapps\\common\\очень длинное название игры с пробелами\\bin\\win64\\game.exe", name: "Очень длинное название игры с пробелами и цифрами 2026", publisher: "", category: "games" as const, source: "parent" as const, weekS: 0 },
  { path: "c:\\program files\\uchi\\uchi.exe", name: "Учи.ру", publisher: "Учи.ру", category: "study" as const, source: "parent" as const, weekS: 1800 },
  { path: "c:\\users\\миша\\downloads\\bad.exe", name: "Bad", publisher: "", category: "blocked" as const, source: "parent" as const, weekS: 0 },
  { path: "c:\\windows\\system32\\notepad.exe", name: "Блокнот", publisher: "Microsoft", category: "system" as const, source: "snapshot" as const, weekS: 600 },
];

async function openPrograms(page: Page, options: MockOptions) {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: family(), ...options });
  await page.goto("/");
  await page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Программы" }).click();
  await expect(page.getByRole("heading", { name: "Программы", level: 2 })).toBeVisible();
}

type Box = { left: number; right: number; top: number; bottom: number; width: number; height: number };
const boxOf = (l: ReturnType<Page["locator"]>) =>
  l.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return { left: r.left, right: r.right, top: r.top, bottom: r.bottom, width: r.width, height: r.height } as Box;
  });

/** Кнопки решения одной высоты, в одну строку и с равными промежутками. */
async function decisionButtonsEven(page: Page) {
  const groups = page.locator(".pending-actions");
  const n = await groups.count();
  expect(n).toBeGreaterThan(0);
  for (let g = 0; g < n; g++) {
    const buttons = groups.nth(g).getByRole("button");
    await expect(buttons).toHaveText(["Игра", "Учёба", "Прочее", "Запретить"]);
    const boxes: Box[] = [];
    for (let i = 0; i < 4; i++) boxes.push(await boxOf(buttons.nth(i)));
    for (const b of boxes) {
      expect(Math.abs(b.height - boxes[0].height), "кнопки решения разной высоты").toBeLessThanOrEqual(0.5);
      expect(Math.abs(b.top - boxes[0].top), "кнопки решения не в одну строку").toBeLessThanOrEqual(0.5);
    }
    const gaps = [1, 2, 3].map((i) => boxes[i].left - boxes[i - 1].right);
    for (const gap of gaps) {
      expect(Math.abs(gap - gaps[0]), `промежутки между кнопками разные: ${gaps.join(", ")}`).toBeLessThanOrEqual(0.5);
      expect(gap).toBeGreaterThanOrEqual(8 - 0.5);
    }
  }
}

/** Зазор между соседними блоками страницы не меньше 16 px (замер, не на глаз). */
async function blocksApart(page: Page) {
  const parts = page.locator("main.content .programs-page > *");
  const n = await parts.count();
  const boxes: Box[] = [];
  for (let i = 0; i < n; i++) boxes.push(await boxOf(parts.nth(i)));
  for (let i = 1; i < boxes.length; i++) {
    expect(boxes[i].top - boxes[i - 1].bottom, `между блоками ${i - 1} и ${i} меньше 16 px`).toBeGreaterThanOrEqual(16 - 0.5);
  }
}

/** Ничего не вылезает из ячеек таблицы и из строк «Ждут решения». */
async function nothingSticksOut(page: Page) {
  const out = await page.evaluate(() => {
    const bad: string[] = [];
    for (const cell of Array.from(document.querySelectorAll<HTMLElement>(".programs-table td, .pending-item"))) {
      const c = cell.getBoundingClientRect();
      for (const child of Array.from(cell.querySelectorAll<HTMLElement>("select, button, input, p, .program-path"))) {
        const r = child.getBoundingClientRect();
        if (r.width === 0) continue;
        if (r.right > c.right + 0.5 || r.left < c.left - 0.5) bad.push(`${child.className || child.tagName}: ${child.textContent?.slice(0, 40)}`);
      }
    }
    return bad;
  });
  expect(out, "элементы вылезают за край").toEqual([]);
}

// s11 — «Программы»: два ждущих решения (одна с пометкой «Раньше было: игра», другая с длинным
// путём и без издателя) и список всех программ.
test("s11 программы: ждут решения и список", async ({ page }) => {
  await tallWindow(page, 1500);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  await expect(page.getByRole("heading", { name: "Ждут решения: 2" })).toBeVisible();
  await expect(page.getByText("Раньше было: игра")).toBeVisible();
  const longPath = page.locator(".pending-item").nth(1).locator(".path-short");
  await expect(longPath).toContainText("…");
  await expect(longPath).toContainText("тренажёр таблицы умножения.exe");
  await expect(page.getByText("Блокнот")).toHaveCount(0);
  await decisionButtonsEven(page);
  await blocksApart(page);
  await nothingSticksOut(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs.png`, fullPage: true });
});

// s11 — после «Игра»: сообщение «SuperGame: игра» с «Отменить», фильтр «Игры» и отмеченные строки.
test("s11 программы: решение, фильтр и массовая смена", async ({ page }) => {
  await tallWindow(page, 1300);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  await page.locator(".pending-item").first().getByRole("button", { name: "Игра" }).click();
  await expect(page.getByText("SuperGame: игра")).toBeVisible();
  await expect(page.getByRole("button", { name: "Отменить" })).toBeVisible();
  const all = page.getByRole("region", { name: "Все программы" });
  await all.getByRole("group", { name: "Показать" }).getByRole("button", { name: "Игры" }).click();
  await expect(all.getByRole("button", { name: "Игры" })).toHaveAttribute("aria-pressed", "true");
  await all.getByRole("checkbox", { name: "Выбрать всё в списке" }).check();
  await expect(all.getByText("Выбрано: 3")).toBeVisible();
  await decisionButtonsEven(page);
  await blocksApart(page);
  await nothingSticksOut(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-filtered.png`, fullPage: true });
});

// s11 — «Программы» пустой: никто ничего не запускал.
test("s11 программы: пусто", async ({ page }) => {
  await openPrograms(page, { pending: [], programs: [] });
  await expect(page.getByText("Программ пока нет: они появятся, когда ребёнок запустит что-то новое")).toBeVisible();
  await expect(page.getByRole("heading", { name: /Ждут решения/ })).toHaveCount(0);
  await blocksApart(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-empty.png`, fullPage: true });
});

// s11 — тёмная тема Windows: тот же экран читается.
test("s11 программы, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await tallWindow(page, 1500);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  await expect(page.getByRole("heading", { name: "Ждут решения: 2" })).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s11-programs-dark.png`, fullPage: true });
});

// --- M3: лимит игр, «Когда время вышло», шаг мастера «Программы» (T6b) -----------------

/** Неделя с играми: будни 2 ч и игры 1 ч, суббота 4 ч и игры 2 ч, среда игры 0, воскресенье игры без лимита. */
const gamesWeek = () =>
  [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({
    weekday,
    totalMin: weekday >= 6 ? 240 : 120,
    unlimited: false,
    gamesMin: weekday === 3 ? 0 : weekday === 6 ? 120 : 60,
    gamesUnlimited: weekday === 7,
  }));

/** Внутри ячейки таблицы лимитов ничего не вылезает за её край, поля одной ячейки в одну строку. */
async function limitCellsFit(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    for (const cell of Array.from(document.querySelectorAll<HTMLElement>(".limits-cell"))) {
      const c = cell.getBoundingClientRect();
      const inputs = Array.from(cell.querySelectorAll<HTMLElement>("input.limits-num"));
      if (inputs.length === 2 && Math.abs(inputs[0].getBoundingClientRect().top - inputs[1].getBoundingClientRect().top) > 0.5) {
        out.push(`часы и минуты не в одну строку: ${inputs[0].getAttribute("aria-label")}`);
      }
      for (const el of Array.from(cell.querySelectorAll<HTMLElement>("input, label, p, span"))) {
        const r = el.getBoundingClientRect();
        if (r.width === 0) continue;
        if (r.right > c.right + 0.5 || r.left < c.left - 0.5) out.push(`${el.className || el.tagName} вылезает: ${el.getAttribute("aria-label") ?? el.textContent?.slice(0, 30)}`);
      }
    }
    return out;
  });
  expect(bad, "в ячейках лимитов что-то вылезает").toEqual([]);
}

// s12 — «Дети → Лимиты» со столбцом «Игры» (FR-16): подсказка «0 мин: в этот день играть нельзя»
// в среде, игры без лимита в воскресенье, пояснение про игры и учёбу под таблицей. Окно 960x640.
test("s12 лимиты с играми", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ limits: gamesWeek() })] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await expect(page.getByRole("columnheader", { name: "Игры" })).toBeVisible();
  await expect(page.getByLabel("Часы игр, понедельник")).toHaveValue("1");
  await expect(page.getByRole("row", { name: /Среда/ }).getByText("0 мин: в этот день играть нельзя")).toBeVisible();
  await expect(page.getByLabel("Игры без лимита, воскресенье")).toBeChecked();
  const note = page.getByText("Время игр входит в общее. Учёба в общее время не входит");
  await expect(note).toBeVisible();
  // Пояснение стоит под таблицей с зазором, а не прижато к ней.
  const table = await boxOf(page.locator(".limits-table"));
  const noteBox = await boxOf(note);
  expect(noteBox.top - table.bottom).toBeGreaterThanOrEqual(4 - 0.5);
  // Столбцы «Общее» и «Игры» одинаковой ширины по содержимому не обязаны, но оба целиком в окне.
  for (const name of ["Общее", "Игры"]) {
    const h = await boxOf(page.getByRole("columnheader", { name }));
    expect(h.right).toBeLessThanOrEqual(960);
  }
  await limitCellsFit(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-limits-games.png`, fullPage: true });
});

// s12 — шаг «Время» мастера в самом узком окне мастера (720x560): два столбца без прокрутки вбок.
test("s12 мастер, шаг времени в узком окне", async ({ page }) => {
  await page.setViewportSize({ width: 720, height: 560 });
  await mockProgram(page, { configured: false, mode: "wizard" });
  await page.goto("/");
  await page.getByLabel("Пароль родителя").fill("секрет-123");
  await page.getByLabel("Повторите пароль").fill("секрет-123");
  await page.getByRole("button", { name: "Далее" }).click();
  await page.getByRole("checkbox", { name: /Миша/ }).check();
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByRole("columnheader", { name: "Игры" })).toBeVisible();
  await limitCellsFit(page);
  await noHorizontalScroll(page);
  const table = await boxOf(page.locator(".limits-table"));
  const panel = await boxOf(page.locator(".wizard"));
  expect(table.right, "таблица шире панели мастера").toBeLessThanOrEqual(panel.right - 16 + 0.5);
  await page.screenshot({ path: `${SCREENS}/s12-wizard-time-720.png`, fullPage: true });
});

/** Вкладка «Когда время вышло»: карточки вариантов не слипаются (зазор ≥ 12 px). */
async function endModesApart(page: Page) {
  const cards = page.locator(".end-mode");
  await expect(cards).toHaveCount(2);
  const a = await boxOf(cards.nth(0));
  const b = await boxOf(cards.nth(1));
  expect(b.top - a.bottom, "варианты слиплись").toBeGreaterThanOrEqual(12 - 0.5);
}

// s12 — «Когда время вышло» (FR-18) без учебных программ: у «Оставить только учёбу» предупреждение
// и кнопка «Программы»; выбран «Оставить только учёбу», внизу панель «Сохранить».
test("s12 когда время вышло: нет учебных программ", async ({ page }) => {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ limits: gamesWeek() })], programs: programList().filter((p) => p.category !== "study") });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await page.getByRole("tab", { name: "Когда время вышло" }).click();
  await page.getByRole("radio", { name: /Оставить только учёбу/ }).check();
  const warn = page.locator(".end-mode-warn");
  await expect(warn).toContainText("Учебных программ и сайтов пока нет: в этом режиме будет закрыто всё");
  await expect(warn.getByRole("button", { name: "Программы" })).toBeVisible();
  // Предупреждение внутри карточки «Оставить только учёбу», с отступом от её краёв.
  const card = await boxOf(page.locator(".end-mode").nth(1));
  const w = await boxOf(warn);
  expect(w.bottom).toBeLessThanOrEqual(card.bottom - 8 + 0.5);
  expect(w.right).toBeLessThanOrEqual(card.right - 8 + 0.5);
  await endModesApart(page);
  await expect(page.getByRole("region", { name: "Сохранение изменений" }).getByText("Есть несохранённые изменения")).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-end-mode-no-study.png`, fullPage: true });
});

// s12 — «Когда время вышло» с учебной программой: предупреждения нет, выбран сохранённый «Блокировать экран».
test("s12 когда время вышло: учебные есть", async ({ page }) => {
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({ limits: gamesWeek() })],
    programs: programList(),
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await page.getByRole("tab", { name: "Когда время вышло" }).click();
  await expect(page.getByRole("radio", { name: /Блокировать экран/ })).toBeChecked();
  await expect(page.getByText(/Учебных программ и сайтов пока нет/)).toHaveCount(0);
  await endModesApart(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-end-mode.png`, fullPage: true });
});

test("s12 когда время вышло, тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ limits: gamesWeek(), endMode: "study_only" })], programs: [] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await page.getByRole("tab", { name: "Когда время вышло" }).click();
  await expect(page.locator(".end-mode-warn")).toBeVisible();
  await page.screenshot({ path: `${SCREENS}/s12-end-mode-dark.png`, fullPage: true });
});

// s12 — мастер, шаг 4 «Программы» (FR-2): окно мастера 880x680, список найденных программ с выбором.
test("s12 мастер, шаг программ", async ({ page }) => {
  await page.setViewportSize({ width: 880, height: 680 });
  await mockProgram(page, { configured: false, mode: "wizard" });
  await page.goto("/");
  await page.getByLabel("Пароль родителя").fill("секрет-123");
  await page.getByLabel("Повторите пароль").fill("секрет-123");
  await page.getByRole("button", { name: "Далее" }).click();
  await page.getByRole("checkbox", { name: /Миша/ }).check();
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Шаг 3 из 4")).toBeVisible();
  await limitCellsFit(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-wizard-time.png`, fullPage: true });
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Шаг 4 из 4")).toBeVisible();
  await expect(page.getByRole("table", { name: "Программы на этом компьютере" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Что это: Minecraft Launcher" })).toHaveValue("games");
  await expect(page.getByText(/Незнакомые программы будут ждать вашего решения/)).toBeVisible();
  // Кнопки шага в одну строку, одной высоты, под списком.
  const buttons = ["Назад", "Пропустить", "Готово"].map((name) => page.getByRole("button", { name, exact: true }));
  const boxes = await Promise.all(buttons.map((b) => boxOf(b)));
  for (const b of boxes) {
    expect(Math.abs(b.top - boxes[0].top)).toBeLessThanOrEqual(0.5);
    expect(Math.abs(b.height - boxes[0].height)).toBeLessThanOrEqual(0.5);
  }
  const list = await boxOf(page.locator(".wizard-programs"));
  expect(boxes[0].top - list.bottom).toBeGreaterThanOrEqual(8 - 0.5);
  await nothingSticksOut(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-wizard-programs.png`, fullPage: true });
});

// s12 — «Сегодня» с играми (8.3.2 п.3): у Миши «Игры 40 мин из 1 ч» под «Использовано».
test("s12 сегодня с играми", async ({ page }) => {
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({ limits: gamesWeek().map((l) => ({ ...l, gamesMin: 60, gamesUnlimited: false })), usedS: 3600, gamesUsedS: 2400 })],
  });
  await page.goto("/");
  const misha = page.getByRole("article", { name: "Миша" });
  const games = misha.getByText("Игры 40 мин из 1 ч", { exact: true });
  await expect(games).toBeVisible();
  const used = await boxOf(misha.getByText(/^Использовано /));
  const g = await boxOf(games);
  expect(g.top).toBeGreaterThanOrEqual(used.bottom - 0.5);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s12-today-games.png`, fullPage: true });
});
