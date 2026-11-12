import { test, expect, type Page } from "@playwright/test";
import { mockScript } from "./mockBundle";
import type { MockChild, MockOptions } from "../src/__mocks__/backend";
import { DEFAULT_VIDEO_SITES } from "../src/lib/sites";
import { G15_NOW, g15Events, g15Fields } from "../src/__mocks__/g15";
import type { SiteDTO } from "../src/types";

// Окно родителя в браузере с подменной программой (src/__mocks__/backend.ts).
// Снимки лежат в frontend/screens/ и уходят в ветку screenshots.

const SCREENS = "screens";

async function mockProgram(page: Page, options: MockOptions = {}) {
  await page.addInitScript({ content: await mockScript(options) });
}

/**
 * Ни окно, ни прокручиваемая область страницы не уходят вбок; само окно (документ) не прокручивается
 * и по высоте: прокрутка только внутри main.content, шапка стоит на месте. Иначе прокрутка к полю
 * (фокус, scrollIntoView) сдвигает всё окно вместе с шапкой, и блок прилипает к названию.
 */
async function noHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => {
    const wider = (el: Element | null) => (el ? el.scrollWidth > el.clientWidth : false);
    const doc = document.documentElement;
    const out: string[] = [];
    if (wider(doc) || wider(document.querySelector(".content"))) out.push("прокрутка вбок");
    if (doc.scrollHeight - doc.clientHeight > 1) {
      const below = [...document.querySelectorAll("body *")]
        .filter((el) => el.getBoundingClientRect().bottom > doc.clientHeight + 1 && !el.closest(".content, .menu"))
        .map((el) => `${el.tagName.toLowerCase()}.${el.className} «${(el.textContent ?? "").slice(0, 30)}»`);
      const leaked = [...document.querySelectorAll(".content *")]
        .filter((el) => el.getBoundingClientRect().bottom > doc.clientHeight + 1 && getComputedStyle(el).position === "absolute" && (el as HTMLElement).offsetParent === document.body)
        .map((el) => `${el.tagName.toLowerCase()}.${el.className} «${(el.textContent ?? "").slice(0, 30)}»`);
      out.push(`окно прокручивается на ${doc.scrollHeight - doc.clientHeight} px: ${[...below, ...leaked].slice(0, 3).join(", ")}`);
    }
    return out;
  });
  expect(overflow).toEqual([]);
}

/**
 * Окно приложения не растёт вместе со страницей: длинные страницы прокручиваются внутри.
 * Для снимка длинной страницы целиком окно временно делаем выше.
 */
async function tallWindow(page: Page, height: number) {
  await page.setViewportSize({ width: 960, height });
}

/**
 * Снимок страницы целиком (приёмка P2 п.6): окно растёт ровно на невидимый остаток прокручиваемой
 * области, после чего прокрутки нет и низ страницы в кадре. Ширина окна не меняется.
 */
async function wholePage(page: Page) {
  // Своя область прокрутки экрана («Программы», B-43) тоже считается: окно растёт на её остаток.
  const hiddenNow = () =>
    page.evaluate(() =>
      Math.max(
        0,
        ...[...document.querySelectorAll("main.content, main.content .programs-scroll")].map((el) => el.scrollHeight - el.clientHeight),
      ),
    );
  const hidden = Math.ceil(await hiddenNow());
  const size = page.viewportSize()!;
  if (hidden > 0) await page.setViewportSize({ width: size.width, height: size.height + hidden });
  expect(await hiddenNow(), "часть страницы не в кадре").toBeLessThanOrEqual(1);
}

/** Элемент целиком в окне (низ не обрезан). */
async function inFrame(page: Page, l: ReturnType<Page["locator"]>) {
  const bottom = await l.evaluate((el) => el.getBoundingClientRect().bottom);
  expect(bottom, "элемент ниже края кадра").toBeLessThanOrEqual(page.viewportSize()!.height + 0.5);
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
  await expect(page.getByText("Шаг 1 из 5")).toBeVisible();
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

// s04 — журнал (FR-24, 8.3.7 п.2): вкладки периода, фильтры «Ребёнок» и «Что», события простыми словами.
const journalEvents = (now: number) => [
  { atMs: now - 600_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "bypass", text: "Миша: попытка обойти защиту. Закрыт значок программы" },
  { atMs: now - 1_800_000, tzOffsetMin: 180, childId: 2, childName: "Даша", kind: "site_forbidden_try", text: "Даша: попытка открыть запрещённое: сайт example.com" },
  { atMs: now - 3_600_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "extra_time", text: "Миша: доп. время +30 мин" },
  { atMs: now - 5_400_000, tzOffsetMin: 180, childId: 0, childName: "", kind: "password_failed", text: "Неверный пароль родителя" },
];

/** Часы журнала: середина дня, «Сегодня» на снимках не зависит от времени сборки. */
const JOURNAL_NOW = new Date(2026, 10, 12, 15, 0, 0);

async function openJournal(page: Page, over: MockOptions = {}) {
  await page.clock.setFixedTime(JOURNAL_NOW);
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({}), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })],
    events: journalEvents(JOURNAL_NOW.getTime()),
    ...over,
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Журнал" }).click();
}

for (const scheme of ["light", "dark"] as const) {
  test(`s04 журнал${scheme === "dark" ? ", тёмная тема" : ""}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await openJournal(page);
    await expect(page.getByText("Миша: доп. время +30 мин")).toBeVisible();
    await expect(page.getByRole("tab", { name: "Всё" })).toHaveAttribute("aria-selected", "true");
    await expect(page.getByLabel("Ребёнок")).toHaveValue("0");
    await expect(page.getByLabel("Что")).toHaveValue("");
    // При «Все дети» кнопки очистки нет (8.3.7 п.3).
    await expect(page.getByRole("button", { name: "Очистить журнал" })).toHaveCount(0);
    await noHorizontalScroll(page);
    await page.screenshot({ path: `${SCREENS}/s04-journal${scheme === "dark" ? "-dark" : ""}.png`, fullPage: true });
  });
}

// s04 — ссылка «Нарушения сегодня» с главной: «Что: Нарушения», вкладка «Сегодня», выбран ребёнок -
// справа «Очистить журнал». На 800 px строка фильтров не уходит вбок.
test("s04 журнал: нарушения сегодня у одного ребёнка", async ({ page }) => {
  await page.setViewportSize({ width: 800, height: 640 });
  await openJournal(page);
  await page.getByRole("button", { name: "Сегодня", exact: true }).click();
  await page.getByRole("button", { name: /^Нарушения сегодня: / }).click();
  await expect(page.getByLabel("Что")).toHaveValue("violations");
  await expect(page.getByRole("tab", { name: "Сегодня" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByText("Даша: попытка открыть запрещённое: сайт example.com")).toBeVisible();
  await expect(page.getByText("Миша: доп. время +30 мин")).toHaveCount(0);
  await page.getByLabel("Ребёнок").selectOption({ label: "Миша" });
  await expect(page.getByText("Даша: попытка открыть запрещённое: сайт example.com")).toHaveCount(0);
  await expect(page.locator(".journal-item")).toHaveCount(1);
  await expect(page.getByRole("button", { name: "Очистить журнал" })).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s04-journal-violations.png`, fullPage: true });
});

// s04 — «Показать ещё» (RM5-5): 230 записей, на странице 200, кнопка дописывает старые.
test("s04 журнал: «Показать ещё»", async ({ page }) => {
  const base = JOURNAL_NOW.getTime() - 86_400_000;
  const many = Array.from({ length: 230 }, (_, i) => ({
    atMs: base - i * 60_000,
    tzOffsetMin: 180,
    childId: 1,
    childName: "Миша",
    kind: "extra_time",
    text: `Миша: доп. время +5 мин (запись ${i + 1})`,
  }));
  await openJournal(page, { events: many });
  await expect(page.getByText("Миша: доп. время +5 мин (запись 1)", { exact: true })).toBeVisible();
  await expect(page.locator(".journal-item")).toHaveCount(200);
  const moreBtn = page.getByRole("button", { name: "Показать ещё" });
  await moreBtn.scrollIntoViewIfNeeded();
  await expect(moreBtn).toBeInViewport();
  await noHorizontalScroll(page);
  await moreBtn.click();
  await expect(page.locator(".journal-item")).toHaveCount(230);
  await expect(page.getByText("Миша: доп. время +5 мин (запись 230)", { exact: true })).toBeVisible();
  await expect(moreBtn).toHaveCount(0);
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
  await wholePage(page);
  await inFrame(page, page.getByRole("region", { name: "Итоги" }));
  await inFrame(page, page.getByRole("button", { name: "Настройки для родителя" }));
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
  await expect(page.getByRole("heading", { level: 3 }).first()).toHaveText("Версия 0.5.0. Статистика по дням, очистка и промокоды");
  await expect(page.locator("main.content").getByRole("button")).toHaveCount(0);
  await noHorizontalScroll(page);
  await wholePage(page);
  await inFrame(page, page.getByRole("heading", { name: /^Версия 0\.2\.1\./, level: 3 }));
  await inFrame(page, page.getByRole("heading", { level: 3 }).last());
  await page.screenshot({ path: `${SCREENS}/s07-whatsnew.png`, fullPage: true });
});

// --- Фидбек-раунд 0.2.2 ---------------------------------------------------------------

const AUTO_BONUS = "Бонус за свободное время: 15 мин";
const ledger = (now: number) => [
  { atMs: now - 10 * 3600_000, tzOffsetMin: 180, deltaMin: 15, reason: AUTO_BONUS, balanceAfter: 45 },
  { atMs: now - 30 * 3600_000, tzOffsetMin: 180, deltaMin: 30, reason: "за уборку", balanceAfter: 30 },
];

// s09 — копилка: обнулить и убавить с подтверждением (D-46); вкладка «Бонусы» — автобонусы и ссылка на «Промокоды» (R25, RM5-13).
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

// s09 — вкладка «Бонусы»: автобонусы и одна строка со ссылкой на раздел «Промокоды» (RM5-13);
// формы промокодов здесь больше нет, «Открыть» ведёт в раздел.
test("s09 бонусы: ссылка на промокоды", async ({ page }) => {
  await tallWindow(page, 1000);
  const now = Date.now();
  await mockProgram(page, {
    mode: "parent",
    sessionActive: true,
    children: [child({ bonusBalance: 45, bonusLedger: ledger(now) }), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", bonusBalance: 0, bonusLedger: [] })],
  });
  await page.goto("/");
  await page.getByRole("button", { name: "Копилка", exact: true }).click();
  await page.getByRole("tab", { name: "Бонусы" }).click();
  await expect(page.getByText("+15 мин за неизрасходованное время прошлого дня")).toBeVisible();
  const link = page.getByRole("region", { name: "Промокоды" });
  await expect(link.getByText("Промокоды теперь в отдельном разделе «Промокоды»", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Код", { exact: true })).toHaveCount(0);
  // Строка и кнопка в одну линию: кнопка по вертикали в пределах строки текста.
  const box = await link.evaluate((el) => {
    const r = (q: string) => el.querySelector(q)!.getBoundingClientRect();
    const t = r(".panel-text");
    const b = r("button");
    return { textMid: t.top + t.height / 2, btnTop: b.top, btnBottom: b.bottom, btnLeft: b.left, textRight: t.right };
  });
  expect(box.textMid, "текст и «Открыть» не на одной линии").toBeGreaterThan(box.btnTop);
  expect(box.textMid, "текст и «Открыть» не на одной линии").toBeLessThan(box.btnBottom);
  expect(box.btnLeft, "«Открыть» наезжает на текст").toBeGreaterThan(box.textRight);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s09-bonuses-link.png`, fullPage: true });
  await link.getByRole("button", { name: "Открыть" }).click();
  await expect(page.getByRole("heading", { name: "Промокоды", level: 2 })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Промокоды" })).toHaveAttribute("aria-current", "page");
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

// Вкладки «Итогов» (D-58, S39): game 10 мин, game2 5 мин, study 3 мин; длинное название игры, чтобы
// замер видел обрезку, и «Остальное время» (время без программы) последней строкой.
const S39_LONG = "Очень длинное название игры с пробелами и цифрами 2026";
const s39Programs = [
  { path: "c:\\games\\game.exe", name: "game", category: "games" as const, seconds: 600 },
  { path: "c:\\games\\game2.exe", name: "game2", category: "games" as const, seconds: 300 },
  { path: "c:\\games\\long.exe", name: S39_LONG, category: "games" as const, seconds: 120 },
  { path: "c:\\school\\study.exe", name: "study", category: "study" as const, seconds: 180 },
  { path: "", name: "Остальное время", category: "other" as const, seconds: 300 },
];
const s39Child = () => child({ usedS: 1320, gamesUsedS: 1020, studyUsedS: 180, programsToday: s39Programs });

/**
 * Замер списка выбранной вкладки (8.2.6 п.3): строки не налезают друг на друга, название, время и доля
 * внутри своей строки и не пересекаются, время и доля выровнены по правому краю во всех строках,
 * список не шире панели. При сбое все размеры в одной строке ошибки (журналы Actions недоступны).
 */
async function tabListFits(page: Page, where: string) {
  // «Итоги» лежат в панели периода (тоже tabpanel): панель категорий ищем по классу.
  const report = await page.getByRole("region", { name: "Итоги" }).locator(CATEGORY_PANEL).evaluate((panel) => {
    const r = (el: Element | null) => {
      const b = el!.getBoundingClientRect();
      return { l: Math.round(b.left * 10) / 10, r: Math.round(b.right * 10) / 10, t: Math.round(b.top * 10) / 10, b: Math.round(b.bottom * 10) / 10 };
    };
    const items = [...panel.querySelectorAll(".totals-item")].map((li) => ({
      text: li.textContent ?? "",
      row: r(li),
      name: r(li.querySelector(".totals-name")),
      nameScroll: (li.querySelector(".totals-name") as HTMLElement).scrollWidth,
      nameClient: (li.querySelector(".totals-name") as HTMLElement).clientWidth,
      time: r(li.querySelector(".totals-time")),
      share: r(li.querySelector(".totals-share")),
    }));
    return { panel: r(panel), items };
  });
  const problems: string[] = [];
  const { panel, items } = report;
  if (items.length === 0) problems.push("в списке нет строк");
  items.forEach((it, i) => {
    if (it.row.l < panel.l - 0.5 || it.row.r > panel.r + 0.5) problems.push(`строка ${i} шире панели`);
    for (const [n, c] of [["название", it.name], ["время", it.time], ["доля", it.share]] as const) {
      if (c.l < it.row.l - 0.5 || c.r > it.row.r + 0.5 || c.t < it.row.t - 0.5 || c.b > it.row.b + 0.5) problems.push(`строка ${i}: ${n} вне строки`);
    }
    if (it.name.r > it.time.l + 0.5) problems.push(`строка ${i}: название налезает на время`);
    if (it.time.r > it.share.l + 0.5) problems.push(`строка ${i}: время налезает на долю`);
    if (it.nameScroll > it.nameClient + 2) problems.push(`строка ${i}: название не помещается (${it.nameScroll} > ${it.nameClient})`);
    if (i > 0 && it.row.t < items[i - 1].row.b - 0.5) problems.push(`строки ${i - 1} и ${i} налезают`);
    if (i > 0 && Math.abs(it.share.r - items[0].share.r) > 0.5) problems.push(`строка ${i}: доля не выровнена`);
    if (i > 0 && Math.abs(it.time.r - items[0].time.r) > 0.5) problems.push(`строка ${i}: время не выровнено`);
  });
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report)}`).toEqual([]);
}

/**
 * Ждёт конца переходов CSS в «Итогах» (заливка вкладки идёт 150 мс): замер и снимок сразу после клика
 * видят промежуточный серый, и выбранная вкладка выглядит невыбранной (возврат 2 FB-10).
 */
async function settled(page: Page) {
  const left = await page.getByRole("region", { name: "Итоги" }).evaluate(async (box) => {
    const all = box.getAnimations({ subtree: true });
    const done = Promise.all(all.map((a) => a.finished.catch(() => undefined)));
    await Promise.race([done, new Promise((r) => setTimeout(r, 2000))]);
    return box.getAnimations({ subtree: true }).filter((a) => a.playState === "running").length;
  });
  expect(left, "переходы в «Итогах» не закончились за 2 с").toBe(0);
}

/**
 * Состояние вкладок «Итогов» замером (возврат 1 FB-10): заливку основным цветом, жирную подпись и черту
 * снизу имеет ровно та вкладка, у которой aria-selected, у остальных фон панели; панель подписана
 * выбранной вкладкой. Указатель уводится, чтобы наведение не мешало замеру. Всё в одной строке ошибки.
 */
async function tabsState(page: Page, want: string, where: string, sumWord = "за сегодня") {
  await page.mouse.move(1, 1);
  await settled(page);
  const totals = page.getByRole("region", { name: "Итоги" });
  const report = await totals.evaluate((box) => {
    const tabs = [...box.querySelectorAll('.totals-tab[role="tab"]')].map((t) => {
      const cs = getComputedStyle(t);
      return {
        id: t.id,
        label: t.querySelector(".totals-tab-label")?.textContent ?? "",
        selected: t.getAttribute("aria-selected"),
        bg: cs.backgroundColor,
        shadow: cs.boxShadow,
        weight: getComputedStyle(t.querySelector(".totals-tab-label")!).fontWeight,
      };
    });
    const panel = box.querySelector('.totals-panel[role="tabpanel"]');
    const sum = box.querySelector(".totals-sum")?.textContent ?? "";
    const surface = getComputedStyle(box).backgroundColor;
    return { tabs, labelledBy: panel?.getAttribute("aria-labelledby"), sum, surface };
  });
  const problems: string[] = [];
  const sel = report.tabs.filter((t) => t.selected === "true");
  if (sel.length !== 1 || sel[0].label !== want) problems.push(`выбрана не одна «${want}»`);
  for (const t of report.tabs) {
    const on = t.selected === "true";
    if (on && (t.bg === report.surface || t.shadow === "none" || Number(t.weight) < 700)) problems.push(`«${t.label}» выбрана, но выглядит невыбранной`);
    if (!on && (t.bg !== report.surface || t.shadow !== "none" || Number(t.weight) >= 700)) problems.push(`«${t.label}» не выбрана, но выглядит выбранной`);
  }
  if (!report.sum.startsWith(`${want} ${sumWord}`)) problems.push(`заголовок списка «${report.sum}»`);
  if (report.labelledBy !== sel[0]?.id) problems.push("панель подписана не выбранной вкладкой");
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report)}`).toEqual([]);
}

/**
 * Контраст текста вкладок и строк списка к фону (WCAG 2.1 AA, не меньше 4,5:1): фон ищется вверх по
 * предкам до непрозрачного, у строк учитывается полоса доли (::before с прозрачностью). Подписи и суммы
 * каждой вкладки замеряются в выбранном и невыбранном виде: перебор всех вкладок кликом.
 */
async function textContrast(page: Page, where: string) {
  await page.mouse.move(1, 1);
  const totals = page.getByRole("region", { name: "Итоги" });
  const bad: string[] = [];
  const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" }).getByRole("tab");
  const n = await tabs.count();
  for (let i = 0; i < n; i++) {
    await tabs.nth(i).click();
    await page.mouse.move(1, 1);
    await settled(page);
    const found = await totals.evaluate((box) => {
      const rgba = (c: string) => {
        const m = c.match(/[\d.]+/g)!.map(Number);
        return [m[0], m[1], m[2], m.length > 3 ? m[3] : 1];
      };
      const over = (top: number[], under: number[]) => top.slice(0, 3).map((v, k) => v * top[3] + under[k] * (1 - top[3]));
      const bgOf = (el: Element | null): number[] => {
        const chain: number[][] = [];
        for (let e = el; e; e = e.parentElement) {
          const c = rgba(getComputedStyle(e).backgroundColor);
          if (c[3] > 0) chain.push(c);
          if (c[3] >= 1) break;
        }
        let acc = [255, 255, 255];
        for (const c of chain.reverse()) acc = over(c, acc);
        return acc;
      };
      const lum = (c: number[]) => {
        const [r, g, b] = c.map((v) => {
          const x = v / 255;
          return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
        });
        return 0.2126 * r + 0.7152 * g + 0.0722 * b;
      };
      const ratio = (a: number[], b: number[]) => {
        const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
        return (x + 0.05) / (y + 0.05);
      };
      const out: string[] = [];
      const check = (el: Element, bg: number[], what: string) => {
        const fg = over(rgba(getComputedStyle(el).color), bg);
        const r = ratio(fg, bg);
        if (r < 4.5) out.push(`${what} «${el.textContent}» ${r.toFixed(2)}:1 (${getComputedStyle(el).color} на rgb(${bg.map(Math.round).join(",")}))`);
      };
      for (const t of box.querySelectorAll('.totals-tab[role="tab"]')) {
        const state = t.getAttribute("aria-selected") === "true" ? "выбранная" : "невыбранная";
        for (const part of t.querySelectorAll(".totals-tab-label, .totals-tab-value")) check(part, bgOf(t), `вкладка ${state}`);
      }
      // Вкладки периодов теперь на фоне «Итогов» (UX-аудит FB-13): их контраст тоже к этому фону.
      for (const t of box.querySelectorAll('[role="tablist"][aria-label="Период"] [role="tab"]')) {
        check(t, bgOf(t), `период ${t.getAttribute("aria-selected") === "true" ? "выбранный" : "невыбранный"}`);
      }
      for (const el of box.querySelectorAll(".totals-sum, .totals-note, .totals-empty")) check(el, bgOf(el), "панель");
      for (const li of box.querySelectorAll(".totals-item")) {
        const row = bgOf(li);
        const bar = getComputedStyle(li, "::before");
        const barC = rgba(bar.backgroundColor);
        const withBar = over([barC[0], barC[1], barC[2], barC[3] * Number(bar.opacity)], row);
        for (const part of li.querySelectorAll(".totals-name, .totals-time, .totals-share")) {
          check(part, row, "строка");
          check(part, withBar, "строка на полосе");
        }
      }
      return out;
    });
    bad.push(...found);
  }
  expect(bad, `${where}: ${bad.join("; ")}`).toEqual([]);
}

// s06 — вкладка «Игры» в окне ребёнка (D-58, S39): сумма игр и каждая игра «название, время, доля»;
// замер на всех размерах окна, включая самые узкие 800 x 480 и 640 x 520.
for (const [w, h] of [
  [800, 640],
  [800, 480],
  [640, 520],
]) {
  test(`s06 моя статистика: вкладка «Игры» в окне ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "mystats", children: [s39Child()], myChildId: 1 });
    await page.goto("/");
    const totals = page.getByRole("region", { name: "Итоги" });
    const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
    await expect(tabs.getByRole("tab")).toHaveCount(5);
    await tabs.getByRole("tab", { name: "Игры", exact: true }).click();
    await expect(tabs.getByRole("tab", { name: "Игры", exact: true })).toHaveAttribute("aria-selected", "true");
    const panel = totals.locator(CATEGORY_PANEL);
    await expect(panel.getByText("Игры за сегодня: 17 мин", { exact: true })).toBeVisible();
    const list = panel.getByRole("list", { name: "Игры: по программам" });
    await expect(list.getByRole("listitem")).toHaveCount(3);
    await expect(list.getByRole("listitem").nth(0)).toHaveText("game10 мин59%");
    await expect(list.getByRole("listitem").nth(1)).toHaveText("game25 мин29%");
    // Длинное название одной строкой (fitName, PL-22): целиком или обрезано одним «…» с полным в подсказке.
    const long = await list.getByRole("listitem").nth(2).locator(".totals-name").evaluate((el) => ({
      title: el.getAttribute("title"),
      shown: el.querySelector('[aria-hidden="true"]')?.textContent ?? el.textContent,
      width: el.clientWidth,
    }));
    const longOk = long.title === null ? long.shown === S39_LONG : long.title === S39_LONG && long.shown!.endsWith("…") && long.shown!.split("…").length === 2;
    expect(longOk, `длинное название в окне ${w} x ${h}: ${JSON.stringify(long)}`).toBe(true);
    await tabListFits(page, `окно ${w} x ${h}`);
    await tabsState(page, "Игры", `окно ${w} x ${h}`);
    await noHorizontalScroll(page);
    await totals.scrollIntoViewIfNeeded();
    // Кадр - окно (прокручивается область main.content, не документ): «Бонусы» ниже «Итогов» в той же
    // прокрутке и достижимы ею, а не обрезаны вёрсткой.
    const below = await page.evaluate(() => {
      const sc = document.querySelector("main.content")!;
      const tot = document.querySelector('[aria-labelledby="my-totals"]')!.getBoundingClientRect();
      const bon = document.querySelector('[aria-labelledby="my-bonuses"]')!.getBoundingClientRect();
      return { totalsBottom: tot.bottom, bonusesTop: bon.top, scrollH: sc.scrollHeight, clientH: sc.clientHeight, docScroll: document.documentElement.scrollHeight - document.documentElement.clientHeight };
    });
    expect(below.bonusesTop >= below.totalsBottom - 0.5 && below.docScroll <= 1, `«Бонусы» в окне ${w} x ${h}: ${JSON.stringify(below)}`).toBe(true);
    await page.getByRole("region", { name: "Бонусы" }).scrollIntoViewIfNeeded();
    await expect(page.getByRole("region", { name: "Бонусы" })).toBeInViewport();
    await totals.scrollIntoViewIfNeeded();
    await settled(page);
    if (w === 800 && h === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-games.png`, fullPage: true });
    if (w === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-games-small.png` });
  });
}

// s31 — статистика родителя, вкладка «Игры» (FR-23, D-58, S39): те же строки, что у ребёнка.
test("s31 статистика родителя: вкладка «Игры»", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [s39Child(), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })] });
  await page.goto("/");
  await page.getByRole("article", { name: "Миша" }).getByRole("button", { name: "Статистика" }).click();
  const totals = page.getByRole("region", { name: "Итоги" });
  const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
  await tabs.getByRole("tab", { name: "Игры", exact: true }).click();
  const panel = totals.locator(CATEGORY_PANEL);
  await expect(panel.getByText("Игры за сегодня: 17 мин", { exact: true })).toBeVisible();
  await expect(panel.getByRole("list", { name: "Игры: по программам" }).getByRole("listitem").nth(0)).toHaveText("game10 мин59%");
  await tabListFits(page, "статистика родителя");
  await tabsState(page, "Игры", "статистика родителя");
  await noHorizontalScroll(page);
  await settled(page);
  await page.screenshot({ path: `${SCREENS}/s31-parent-stats-games.png`, fullPage: true });
  // Учёба: study 3 мин.
  await tabs.getByRole("tab", { name: "Учёба", exact: true }).click();
  await expect(panel.getByRole("list", { name: "Учёба: по программам" }).getByRole("listitem")).toHaveText(["study3 мин100%"]);
});

// S42 (D-61): «сейчас» - среда 21.10.2026 15:00; прошлые дни как у службы (day_state и usage_daily): пн этой
// недели игры 30 мин, 1-е число игры 20 мин, 2 января учёба 40 мин, прошлый год прочее 50 мин; сегодня game 10 мин.
const S42_NOW = new Date(2026, 9, 21, 15, 0, 0);
const S42_GAME = "c:\\games\\game.exe";
const s42Child = (over: Partial<MockChild> = {}) =>
  child({
    usedS: 600,
    gamesUsedS: 600,
    studyUsedS: 0,
    programsToday: [{ path: S42_GAME, name: "game", category: "games", seconds: 600 }],
    history: [
      { day: "2026-10-19", totalS: 1800, gamesS: 1800, extraMin: 30, programs: [{ path: S42_GAME, name: "game", category: "games", seconds: 1800 }] },
      { day: "2026-10-01", totalS: 1200, gamesS: 1200, programs: [{ path: S42_GAME, name: "game", category: "games", seconds: 1200 }] },
      { day: "2026-01-02", studyS: 2400, programs: [{ path: "c:\\school\\study.exe", name: "study", category: "study", seconds: 2400 }] },
      { day: "2025-06-15", totalS: 3000 },
    ],
    ...over,
  });

/** Панель вкладки категорий «Итогов»; в окне ребёнка она внутри панели периода (тоже tabpanel). */
const CATEGORY_PANEL = '.totals-panel[role="tabpanel"]';

/**
 * Вкладки периодов (D-61) помещаются: ни одна не налезает на соседнюю, подпись целиком внутри своей
 * кнопки, ряд не шире области окна. При сбое все размеры в одной строке ошибки.
 */
async function periodTabsFit(page: Page, where: string) {
  const report = await page.getByRole("tablist", { name: "Период" }).evaluate((list) => {
    const r = (el: Element) => {
      const b = el.getBoundingClientRect();
      return { l: Math.round(b.left * 10) / 10, r: Math.round(b.right * 10) / 10, t: Math.round(b.top * 10) / 10, b: Math.round(b.bottom * 10) / 10 };
    };
    const content = document.querySelector("main.content")!;
    return {
      list: r(list),
      content: r(content),
      tabs: [...list.querySelectorAll('[role="tab"]')].map((t) => ({ text: t.textContent ?? "", box: r(t), scroll: (t as HTMLElement).scrollWidth, client: (t as HTMLElement).clientWidth })),
    };
  });
  const problems: string[] = [];
  report.tabs.forEach((t, i) => {
    if (t.box.l < report.content.l - 0.5 || t.box.r > report.content.r + 0.5) problems.push(`«${t.text}» вне окна`);
    if (t.scroll > t.client + 1) problems.push(`«${t.text}»: подпись не помещается (${t.scroll} > ${t.client})`);
    for (const o of report.tabs.slice(i + 1)) {
      const apart = t.box.r <= o.box.l + 0.5 || o.box.r <= t.box.l + 0.5 || t.box.b <= o.box.t + 0.5 || o.box.b <= t.box.t + 0.5;
      if (!apart) problems.push(`«${t.text}» и «${o.text}» налезают`);
    }
  });
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report)}`).toEqual([]);
}

// s06 — периоды «Итогов» в окне ребёнка (S42, D-61): «Неделя» - игры 40 мин, строка game за неделю; окно
// не прокручивается целиком и строки не налезают на всех размерах, включая самые узкие.
for (const [w, h] of [
  [800, 640],
  [800, 480],
  [640, 520],
]) {
  test(`s06 моя статистика: неделя в окне ${w} x ${h}`, async ({ page }) => {
    await page.clock.setFixedTime(S42_NOW);
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "mystats", children: [s42Child()], myChildId: 1 });
    await page.goto("/");
    const totals = page.getByRole("region", { name: "Итоги" });
    await expect(totals.getByText("Использовано сегодня", { exact: true })).toBeVisible();
    await page.getByRole("tablist", { name: "Период" }).getByRole("tab", { name: "Неделя", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Неделя", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(totals.getByText("Использовано за неделю", { exact: true })).toBeVisible();
    await expect(totals.locator(".stat-row").filter({ hasText: "Использовано за неделю" })).toHaveText("Использовано за неделю40 мин");
    await expect(totals.locator(".stat-row").filter({ hasText: "Доп. время" })).toHaveText("Доп. время30 мин");
    await expect(totals.getByRole("status")).toHaveText("");
    const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
    await tabs.getByRole("tab", { name: "Игры", exact: true }).click();
    const panel = totals.locator(CATEGORY_PANEL);
    await expect(panel.getByText("Игры за неделю: 40 мин", { exact: true })).toBeVisible();
    await expect(panel.getByRole("list", { name: "Игры: по программам" }).getByRole("listitem")).toHaveText(["game40 мин100%"]);
    // «Сколько осталось» - за сегодня, от периода не зависит.
    await expect(page.getByRole("region", { name: "Сколько осталось" }).locator(".stat-row").first()).toHaveText("Осталось всего1 ч 50 мин");
    await periodTabsFit(page, `окно ${w} x ${h}`);
    await tabListFits(page, `неделя, окно ${w} x ${h}`);
    await tabsState(page, "Игры", `неделя, окно ${w} x ${h}`, "за неделю");
    await noHorizontalScroll(page);
    // Вкладки периодов - внутри «Итогов», под заголовком, над вкладками категорий (UX-аудит FB-13).
    const periods = page.getByRole("tablist", { name: "Период" });
    await expect(totals.getByRole("tablist", { name: "Период" })).toHaveCount(1);
    const [head, list, cats] = await Promise.all([
      boxOf(totals.getByRole("heading", { name: "Итоги" })),
      boxOf(periods),
      boxOf(tabs),
    ]);
    expect(head.bottom, "вкладки периодов не под заголовком «Итогов»").toBeLessThanOrEqual(list.top + 0.5);
    expect(list.bottom, "вкладки периодов не над вкладками категорий").toBeLessThanOrEqual(cats.top + 0.5);
    if (w === 800 && h === 640) {
      // Снимок до прокрутки к спискам: начало «Итогов» с выбранной вкладкой периода.
      await totals.evaluate((el) => el.scrollIntoView({ block: "start" }));
      await settled(page);
      await expect(periods.getByRole("tab", { name: "Неделя", exact: true })).toBeInViewport();
      await page.screenshot({ path: `${SCREENS}/s06-my-stats-week-top.png` });
    }
    await totals.scrollIntoViewIfNeeded();
    await settled(page);
    if (w === 800 && h === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-week.png`, fullPage: true });
    if (w === 640) await page.screenshot({ path: `${SCREENS}/s06-my-stats-week-small.png` });
  });
}

// s31 — статистика родителя за год (S42, D-61): игры 1 ч, учёба 40 мин со строкой study, «Использовано за
// год»; те же цифры, что в окне ребёнка; окно не прокручивается целиком.
for (const [w, h] of [
  [960, 900],
  [800, 480],
]) {
  test(`s31 статистика родителя: год в окне ${w} x ${h}`, async ({ page }) => {
    await page.clock.setFixedTime(S42_NOW);
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "parent", sessionActive: true, children: [s42Child(), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })] });
    await page.goto("/");
    await page.getByRole("article", { name: "Миша" }).getByRole("button", { name: "Статистика" }).click();
    const totals = page.getByRole("region", { name: "Итоги" });
    await page.getByRole("tablist", { name: "Период" }).getByRole("tab", { name: "Год", exact: true }).click();
    await expect(totals.locator(".stat-row").filter({ hasText: "Использовано за год" })).toHaveText("Использовано за год1 ч 00 мин");
    const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
    await expect(tabs.getByRole("tab", { name: "Игры", exact: true }).locator(".totals-tab-value")).toHaveText("1 ч 00 мин");
    await expect(tabs.getByRole("tab", { name: "Всего", exact: true }).locator(".totals-tab-value")).toHaveText("1 ч 40 мин");
    await tabs.getByRole("tab", { name: "Учёба", exact: true }).click();
    const panel = totals.locator(CATEGORY_PANEL);
    await expect(panel.getByText("Учёба за год: 40 мин", { exact: true })).toBeVisible();
    await expect(panel.getByRole("list", { name: "Учёба: по программам" }).getByRole("listitem")).toHaveText(["study40 мин100%"]);
    await periodTabsFit(page, `статистика родителя ${w} x ${h}`);
    await tabListFits(page, `статистика родителя за год ${w} x ${h}`);
    await tabsState(page, "Учёба", `статистика родителя за год ${w} x ${h}`, "за год");
    await noHorizontalScroll(page);
    await settled(page);
    // Снимок года у родителя - s21-parent-stats-year (неделя G15, «По дням» по месяцам): один снимок года.
    // «Всё время»: «Всего» 2 ч 30 мин (50 + 60 + 40), как в S42.
    await page.getByRole("tablist", { name: "Период" }).getByRole("tab", { name: "Всё время", exact: true }).click();
    await expect(tabs.getByRole("tab", { name: "Всего", exact: true }).locator(".totals-tab-value")).toHaveText("2 ч 30 мин");
  });
}

// Контраст текста вкладок и строк (WCAG AA 4,5:1) в светлой и тёмной теме, все вкладки по очереди.
for (const scheme of ["light", "dark"] as const) {
  test(`s06 моя статистика: контраст вкладок «Итогов», тема ${scheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await mockProgram(page, { mode: "mystats", children: [s39Child()], myChildId: 1 });
    await page.goto("/");
    await expect(page.getByRole("region", { name: "Итоги" }).getByRole("tablist", { name: "Итоги по категориям" }).getByRole("tab")).toHaveCount(5);
    await textContrast(page, `тема ${scheme}`);
    await tabsState(page, "Прочее", `тема ${scheme}, после перебора`);
  });
}

// FB-10 (0.3.1): «Использовано сегодня 22 мин» и плитка «Всего 25 мин» видны всегда, поэтому пояснение
// с числами стоит под рядом вкладок при любой выбранной вкладке и связано с вкладкой «Всего».
test("s06 моя статистика: пояснение «Всего» про учёбу", async ({ page }) => {
  await mockProgram(page, { mode: "mystats", children: [s39Child()], myChildId: 1 });
  await page.goto("/");
  const totals = page.getByRole("region", { name: "Итоги" });
  const panel = totals.locator(CATEGORY_PANEL);
  const text = "Всего = 22 мин в счёт лимита + 3 мин учёбы вне лимита";
  await expect(panel.getByText("Всего за сегодня: 25 мин", { exact: true })).toBeVisible();
  for (const name of ["Всего", "Игры", "Видео и соцсети", "Учёба", "Прочее"]) {
    await totals.getByRole("tab", { name, exact: true }).click();
    const note = totals.getByText(text, { exact: true });
    await expect(note).toBeVisible();
    await expect(panel.getByText(text, { exact: true })).toHaveCount(0);
    // Замер после щелчка: щелчок прокручивает страницу к вкладке.
    const tablist = await boxOf(totals.getByRole("tablist", { name: "Итоги по категориям" }));
    const nb = await boxOf(note);
    const pb = await boxOf(panel);
    expect(nb.top, "пояснение не под вкладками").toBeGreaterThanOrEqual(tablist.bottom - 0.5);
    expect(nb.bottom, "пояснение не над панелью").toBeLessThanOrEqual(pb.top + 0.5);
  }
  await expect(totals.getByRole("tab", { name: "Всего", exact: true })).toHaveAccessibleDescription(new RegExp(text.replace(/[+=]/g, "\\$&")));
  await noHorizontalScroll(page);
});

// Пустая вкладка (8.2.6 п.3): «Пока ничего не запускалось», списка нет; снимок s06-my-stats-empty-tab.
test("s06 моя статистика: пустая вкладка", async ({ page }) => {
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ usedS: 600, gamesUsedS: 0, studyUsedS: 0, programsToday: [{ path: "c:\\tools\\brave.exe", name: "Brave", category: "other", seconds: 600 }] })],
    myChildId: 1,
  });
  await page.goto("/");
  const totals = page.getByRole("region", { name: "Итоги" });
  await totals.getByRole("tab", { name: "Игры", exact: true }).click();
  const panel = totals.locator(CATEGORY_PANEL);
  await expect(panel.getByText("Игры за сегодня: 0 мин", { exact: true })).toBeVisible();
  await expect(panel.getByText("Пока ничего не запускалось", { exact: true })).toBeVisible();
  await expect(panel.getByRole("list")).toHaveCount(0);
  await expect(totals.getByText(/учёбы вне лимита/)).toHaveCount(0);
  await tabsState(page, "Игры", "пустая вкладка");
  await totals.scrollIntoViewIfNeeded();
  await settled(page);
  await page.screenshot({ path: `${SCREENS}/s06-my-stats-empty-tab.png` });
});

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
// Замечание заказчика к 0.2.4: «Ввести промокод» касается названия «PC Block» у шапки. Причина -
// скрытая подпись таблицы правил вне области прокрутки растягивала само окно; переход к полю
// промокода (Tab, щелчок) прокручивал окно вместе с шапкой. Шапка стоит на месте, окно не прокручено.
for (const [w, h] of [
  [800, 480],
  [640, 520],
]) {
  test(`s06 промокод: шапка на месте после перехода к полю, окно ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "mystats", children: [child({ bonusBalance: 45 })], myChildId: 1 });
    await page.goto("/");
    const field = page.getByRole("textbox", { name: "Промокод" });
    await field.focus();
    await expect(field).toBeFocused();
    await expect(field).toBeInViewport();
    // Самая «жадная» прокрутка к полю (так прокручивают переход по ссылке и часть движков окна):
    // поле к верху видимой области, все прокручиваемые предки вплоть до окна.
    await field.evaluate((el) => el.scrollIntoView({ block: "start" }));
    const state = await page.evaluate(() => ({
      docTop: document.scrollingElement!.scrollTop,
      barTop: document.querySelector("header.topbar")!.getBoundingClientRect().top,
    }));
    expect(state, `окно ${w} x ${h}: ${JSON.stringify(state)}`).toEqual({ docTop: 0, barTop: 0 });
    await noHorizontalScroll(page);
  });
}

test("s06 бонусы ребёнка и промокод", async ({ page }) => {
  await tallWindow(page, 1000);
  // Полдень: автобонус (10 ч назад) - сегодня, он виден в «Копилке» периода «Сегодня» (по умолчанию).
  const nowDate = new Date(2026, 9, 19, 12, 0, 0);
  await page.clock.setFixedTime(nowDate);
  const now = nowDate.getTime();
  await mockProgram(page, {
    mode: "mystats",
    children: [child({ bonusBalance: 45, bonusLedger: ledger(now) })],
    myChildId: 1,
    promos: [{ codeDisplay: "A1234", minutes: 30, createdAtMs: now - 3600_000 }],
  });
  await page.goto("/");
  const box = page.getByRole("region", { name: "Бонусы" });
  await expect(box.getByText("Копилка: 45 мин")).toBeVisible();
  // История копилки - одно место: раздел «Копилка» панели периода (M5), в «Бонусах» её нет.
  await expect(box.getByRole("list")).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "Сегодня", exact: true })).toHaveAttribute("aria-selected", "true");
  const piggy = page.getByRole("tabpanel", { name: "Сегодня" }).getByRole("region", { name: "Копилка" });
  await expect(piggy.getByText("+15 мин за неизрасходованное время прошлого дня")).toBeVisible();
  await expect(page.getByText("+15 мин за неизрасходованное время прошлого дня")).toHaveCount(1);
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

/**
 * Зазор между соседними блоками страницы не меньше 16 px (замер, не на глаз). Блоки сравниваются в
 * своей системе прокрутки (B-43): снаружи - заголовок, сообщение (если есть) и область списков
 * целиком; внутри области списков - её блоки между собой. Блоки внутри прокрученной области
 * сдвинуты все вместе, поэтому сравнивать их с заголовком снаружи нельзя.
 */
async function blocksApart(page: Page) {
  const gapsOk = async (parts: ReturnType<Page["locator"]>, where: string) => {
    const n = await parts.count();
    const boxes: Box[] = [];
    for (let i = 0; i < n; i++) {
      const b = await boxOf(parts.nth(i));
      if (b.height > 0) boxes.push(b);
    }
    for (let i = 1; i < boxes.length; i++) {
      expect(boxes[i].top - boxes[i - 1].bottom, `${where}: между блоками ${i - 1} и ${i} меньше 16 px`).toBeGreaterThanOrEqual(16 - 0.5);
    }
  };
  await gapsOk(
    page.locator("main.content .programs-page > .page-head, main.content .programs-message > *, main.content .programs-scroll"),
    "экран",
  );
  await gapsOk(page.locator("main.content .programs-scroll > *"), "область списков");
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

/** Путь в строках одной строкой, без переноса посреди имени файла; имя файла видно целиком. */
async function pathsOneLine(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    for (const el of Array.from(document.querySelectorAll<HTMLElement>(".program-path .path-short"))) {
      const line = parseFloat(getComputedStyle(el).lineHeight) || 20;
      if (el.getBoundingClientRect().height > line * 1.5) out.push(`перенос: ${el.textContent}`);
      const tail = el.querySelector<HTMLElement>(".path-tail");
      if (tail && tail.scrollWidth > tail.clientWidth + 1) out.push(`имя файла обрезано: ${tail.textContent}`);
    }
    return out;
  });
  expect(bad, `пути в одну строку: ${bad.join("; ")}`).toEqual([]);
}

// s11 — «Программы»: два ждущих решения (одна с пометкой «Файл изменился, проверьте решение. Было:
// игра», другая с длинным путём и без издателя) и список всех программ.
test("s11 программы: ждут решения и список", async ({ page }) => {
  await tallWindow(page, 1500);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  await expect(page.getByRole("heading", { name: "Ждут решения: 2" })).toBeVisible();
  await expect(page.getByText("Файл изменился, проверьте решение. Было: игра")).toBeVisible();
  const longPath = page.locator(".pending-item").nth(1).locator(".path-short");
  await expect(longPath).toContainText("…");
  await expect(longPath).toContainText("тренажёр таблицы умножения.exe");
  await expect(page.getByText("Блокнот")).toHaveCount(0);
  await decisionButtonsEven(page);
  await blocksApart(page);
  await nothingSticksOut(page);
  await pathsOneLine(page);
  await rowsEven(page);
  const longName = page.getByRole("region", { name: "Все программы" }).locator('.program-name[title="Очень длинное название игры с пробелами и цифрами 2026"]');
  await expect(longName).toHaveCount(1);
  await expect(longName).toContainText("…");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs.png`, fullPage: true });
});

// s11 — B-43: одно место сообщения без sticky (PL-25). Область списков прокручена вниз, категория
// меняется в нижней строке: сообщение с «Отменить» и отсчётом стоит над областью списков, на виду;
// окно целиком не прокручивается.
test("s11 программы: «Отменить» на виду при прокрутке списка", async ({ page }) => {
  await tallWindow(page, 640);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  const select = page.getByRole("combobox", { name: "Категория для «Учи.ру»" });
  await select.scrollIntoViewIfNeeded();
  await select.selectOption("other");
  const msg = page.locator(".programs-message .programs-feedback");
  await expect(msg).toContainText("Учи.ру");
  await expect(msg.getByRole("button", { name: "Отменить" })).toBeVisible();
  await expect(msg.locator(".undo-left")).toHaveText(/^\d+ с$/);
  const scrolled = await page.locator(".programs-scroll").evaluate((el) => el.scrollTop);
  expect(scrolled, "список не прокручен: проверка ничего не проверяет").toBeGreaterThan(0);
  expect(await page.locator("main.content").evaluate((el) => el.scrollTop), "прокрутилось окно, а не область списков").toBe(0);
  const content = await boxOf(page.locator("main.content"));
  const box = await boxOf(msg);
  const list = await boxOf(page.locator(".programs-scroll"));
  expect(box.top, "сообщение выше области окна").toBeGreaterThanOrEqual(content.top - 0.5);
  expect(box.bottom, "сообщение не над областью списков").toBeLessThanOrEqual(list.top + 0.5);
  // Выбор категории, к которому вернулся фокус, виден в области списков.
  const sel = await boxOf(select);
  expect(sel.top).toBeGreaterThanOrEqual(list.top - 0.5);
  expect(sel.bottom).toBeLessThanOrEqual(list.bottom + 0.5);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-undo-scroll.png` });
});

// s11 — после «Игра»: сообщение «SuperGame: игра» с «Отменить», фильтр «Игры» и отмеченные строки.
test("s11 программы: решение, фильтр и массовая смена", async ({ page }) => {
  await tallWindow(page, 1300);
  await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
  await page.locator(".pending-item").first().getByRole("button", { name: "Игра" }).click();
  await expect(page.getByText("SuperGame: игра")).toBeVisible();
  await expect(page.getByRole("button", { name: "Отменить" })).toBeVisible();
  const all = page.getByRole("region", { name: "Все программы" });
  // Точное имя: у Playwright имя ищется подстрокой, а «Скрыть «Очень длинное название игры…»» содержит «игры».
  const games = all.getByRole("group", { name: "Показать" }).getByRole("button", { name: "Игры", exact: true });
  await games.click();
  await expect(games).toHaveAttribute("aria-pressed", "true");
  await all.getByRole("checkbox", { name: "Выбрать всё в списке" }).check();
  await expect(all.getByText("Выбрано: 3")).toBeVisible();
  await rowsEven(page);
  await decisionButtonsEven(page);
  await blocksApart(page);
  await nothingSticksOut(page);
  await pathsOneLine(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-filtered.png`, fullPage: true });
});

/**
 * Строки таблицы программ ровные (замер, не глазом): название, «из файла: …» и издатель в одну
 * строку с одним «…» (полное в title), видимый текст не шире строки; строка с длинным названием не
 * выше обычной (±2 px); у каждой строки центры по вертикали времени за неделю, списка категории (или
 * текста у скрытой) и кнопки «Скрыть» совпадают (±2 px). Обычная строка — без своего названия и не в
 * режиме правки: подпись «из файла» и поле правки добавляют строку по задаче. При сбое все
 * подробности — в тексте ошибки одной строкой (пометки сборки видят только её, PL-1).
 */
async function rowsEven(page: Page) {
  const problems = await page.evaluate(() => {
    const out: { row: number; name: string; what: string; px?: Record<string, number | null> }[] = [];
    const rows = Array.from(document.querySelectorAll<HTMLElement>(".programs-table tbody tr"));
    const info = rows.map((row, i) => {
      const name = row.getAttribute("aria-label") ?? "";
      const editing = row.querySelector(".name-editor") !== null;
      const renamed = row.querySelector(".program-filename") !== null;
      for (const line of Array.from(row.querySelectorAll<HTMLElement>(".line-fit"))) {
        const visible = line.querySelector<HTMLElement>('[aria-hidden="true"]');
        const shown = (visible ?? line).textContent ?? "";
        const lh = parseFloat(getComputedStyle(line).lineHeight) || 20;
        const lr = line.getBoundingClientRect();
        // Название — не больше двух строк (data-lines), остальные строки — одна строка.
        const allowed = Number(line.dataset.lines ?? "1");
        if (lr.height > lh * (allowed + 0.5)) out.push({ row: i, name, what: `перенос «${shown}»`, px: { height: lr.height, line: lh, allowed } });
        if ((shown.match(/…/g) ?? []).length > 1) out.push({ row: i, name, what: `два «…» в «${shown}»` });
        if (shown.endsWith("…") && !line.title) out.push({ row: i, name, what: `обрезано без подсказки «${shown}»` });
        // Видимый текст (без копии для диктора) не шире строки и строка не нулевой ширины.
        const range = document.createRange();
        range.selectNodeContents(visible ?? line);
        const tw = range.getBoundingClientRect().width;
        if (lr.width < 8) out.push({ row: i, name, what: `строка названия сжата до нуля «${shown}»`, px: { width: lr.width } });
        else if (tw > lr.width + 1) out.push({ row: i, name, what: `текст шире строки «${shown}»`, px: { text: tw, box: lr.width } });
      }
      const centerOf = (el: Element | null) => {
        if (!el) return null;
        const r = el.getBoundingClientRect();
        return r.height > 0 ? r.top + r.height / 2 : null;
      };
      const weekCell = row.querySelector<HTMLElement>(".col-week");
      let week: number | null = null;
      if (weekCell && weekCell.textContent?.trim()) {
        const range = document.createRange();
        range.selectNodeContents(weekCell);
        const r = range.getBoundingClientRect();
        week = r.top + r.height / 2;
      }
      const cat = centerOf(row.querySelector(".col-category select, .col-category .program-category"));
      const btn = centerOf(row.querySelector(".col-action button"));
      const centers = [week, cat, btn].filter((v): v is number => v !== null);
      if (centers.length >= 2 && Math.max(...centers) - Math.min(...centers) > 2) {
        out.push({ row: i, name, what: "центры ячеек на разной высоте", px: { week, category: cat, button: btn } });
      }
      const long = row.querySelector(".program-name[title]") !== null;
      // Лишние строки: название во вторую строку (D-59, не больше двух) и путь, у которого имя файла
      // не помещается в строку даже после «диск…» (имя файла не режется). Строка выше обычной ровно на них.
      const linesOf = (el: HTMLElement | null) => {
        if (!el) return 0;
        const lh = parseFloat(getComputedStyle(el).lineHeight) || 20;
        return Math.max(0, Math.round(el.getBoundingClientRect().height / lh) - 1) * lh;
      };
      // Название стоит в ряд со значком «Переименовать» (24 px): ряд растёт на высоту названия сверх
      // одной строки или значка, смотря что выше.
      const title = row.querySelector<HTMLElement>(".program-title");
      const nameEl = row.querySelector<HTMLElement>(".program-name");
      const pencil = title?.querySelector<HTMLElement>(".program-rename");
      const nameExtra =
        title && nameEl && linesOf(nameEl) > 0
          ? title.getBoundingClientRect().height - Math.max(parseFloat(getComputedStyle(nameEl).lineHeight) || 20, pencil?.getBoundingClientRect().height ?? 0)
          : 0;
      const extra = nameExtra + linesOf(row.querySelector<HTMLElement>(".path-short"));
      return { i, name, h: row.getBoundingClientRect().height, plain: !editing && !renamed, long, extra };
    });
    const plain = info.filter((x) => x.plain && !x.long && x.extra === 0);
    if (plain.length > 0) {
      const base = Math.min(...plain.map((x) => x.h));
      for (const x of info.filter((y) => y.plain)) {
        if (Math.abs(x.h - base - x.extra) > 2) out.push({ row: x.i, name: x.name, what: "строка выше обычной", px: { height: x.h, base, extra: x.extra } });
      }
    }
    return out;
  });
  if (problems.length > 0) throw new Error(`строки таблицы программ неровные: ${JSON.stringify(problems)}`);
}

/**
 * Список категории в строке программ вмещает самую длинную подпись («Запрещено») целиком (ревью M4):
 * ширина того же списка без ограничения (width: auto, Chromium берёт самую длинную подпись, поля и
 * стрелку) не больше настоящей. Текст категории у скрытой не шире своей ячейки. При сбое в тексте
 * ошибки — строка, нужная и настоящая ширина (PL-1).
 */
async function categoriesFit(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    for (const row of Array.from(document.querySelectorAll<HTMLElement>(".programs-table tbody tr"))) {
      const name = row.getAttribute("aria-label") ?? "";
      const select = row.querySelector<HTMLSelectElement>(".col-category select");
      if (select) {
        const probe = select.cloneNode(true) as HTMLSelectElement;
        probe.style.cssText = "width:auto;min-width:0;max-width:none;position:fixed;left:0;top:0;visibility:hidden";
        select.parentElement!.appendChild(probe);
        const need = probe.getBoundingClientRect().width;
        probe.remove();
        const has = select.getBoundingClientRect().width;
        if (need > has + 0.5) out.push(`«${name}»: списку категории нужно ${need.toFixed(1)} px, есть ${has.toFixed(1)} px`);
        const cell = select.closest("td")!.getBoundingClientRect();
        const box = select.getBoundingClientRect();
        if (box.right > cell.right + 0.5) out.push(`«${name}»: список категории за краем ячейки`);
      }
      const text = row.querySelector<HTMLElement>(".col-category .program-category");
      if (text && text.scrollWidth > text.clientWidth + 1) out.push(`«${name}»: «${text.textContent}» не помещается (${text.scrollWidth} > ${text.clientWidth})`);
    }
    return out;
  });
  expect(bad, bad.join("; ")).toEqual([]);
}

/**
 * Кнопка «Скрыть» / «Показать» в своей колонке: не налезает на категорию и время за неделю (D-57).
 * У запрещённой (видимой) кнопки нет: запрещённую скрыть нельзя (RF9-5).
 */
async function hideButtonsInColumn(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    for (const row of Array.from(document.querySelectorAll<HTMLElement>(".programs-table tbody tr"))) {
      const btn = row.querySelector<HTMLElement>(".col-action button");
      const blocked = row.querySelector<HTMLSelectElement>(".col-category select")?.value === "blocked";
      if (blocked) {
        if (btn) out.push(`у запрещённой есть «Скрыть»: ${row.getAttribute("aria-label")}`);
        continue;
      }
      const cat = row.querySelector<HTMLElement>(".col-category")?.getBoundingClientRect();
      const week = row.querySelector<HTMLElement>(".col-week")?.getBoundingClientRect();
      if (!btn || !cat || !week) {
        out.push(`нет кнопки или колонок: ${row.getAttribute("aria-label")}`);
        continue;
      }
      const b = btn.getBoundingClientRect();
      if (b.left < week.right - 0.5 || b.left < cat.right - 0.5) out.push(`кнопка налезает: ${row.getAttribute("aria-label")}`);
      if (b.height > 48) out.push(`кнопка в две строки: ${row.getAttribute("aria-label")}`);
    }
    return out;
  });
  expect(bad).toEqual([]);
}

// s11 — фильтр «Скрытые» (D-57, 8.3.4 п.4): скрыта запускалка Stalker2, в строке категория текстом
// и «Показать»; в «Все» её нет. Фильтры помещаются в ряд с поиском.
test("s11 программы: фильтр «Скрытые»", async ({ page }) => {
  await tallWindow(page, 1300);
  const list = [
    ...programList(),
    { path: "c:\\program files\\epic games\\stalker2\\stalker2.exe", name: "Stalker2", publisher: "Epic Games", category: "games" as const, source: "parent" as const, weekS: 0, hidden: true },
    { path: "c:\\program files\\epic games\\stalker2\\binaries\\win64\\stalker2-win64-shipping.exe", name: "Stalker2-Win64-Shipping", publisher: "GSC Game World", category: "games" as const, source: "catalog" as const, weekS: 9000 },
  ];
  await openPrograms(page, { pending: [], programs: list });
  const all = page.getByRole("region", { name: "Все программы" });
  await expect(all.getByRole("row", { name: "Stalker2-Win64-Shipping", exact: true })).toBeVisible();
  await expect(all.getByRole("row", { name: "Stalker2", exact: true })).toHaveCount(0);
  await hideButtonsInColumn(page);
  await rowsEven(page);
  await nothingSticksOut(page);
  const filters = all.getByRole("group", { name: "Показать" });
  const first = await boxOf(filters.getByRole("button", { name: "Все", exact: true }));
  // Число скрытых у фильтра (B-43 п.2): «Скрытые (1)» тоже помещается в ряд в окне 960.
  const last = await boxOf(filters.getByRole("button", { name: "Скрытые (1)", exact: true }));
  expect(Math.abs(last.top - first.top), "фильтр «Скрытые (1)» перенёсся на новую строку").toBeLessThanOrEqual(0.5);
  await filters.getByRole("button", { name: "Скрытые (1)", exact: true }).click();
  await expect(all.getByRole("row", { name: "Stalker2", exact: true }).getByText("не считается", { exact: true })).toBeVisible();
  await expect(all.getByRole("row", { name: "Stalker2", exact: true })).toBeVisible();
  await expect(all.getByRole("button", { name: "Показать «Stalker2»", exact: true })).toBeVisible();
  await hideButtonsInColumn(page);
  await rowsEven(page);
  await blocksApart(page);
  await nothingSticksOut(page);
  await pathsOneLine(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-hidden.png`, fullPage: true });
});

/** Строка в режиме правки названия не налезает на колонки: поле и кнопки внутри своей ячейки (D-59). */
async function editorInCell(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    const editor = document.querySelector<HTMLElement>(".name-editor");
    const cell = editor?.closest("td")?.getBoundingClientRect();
    const cat = editor?.closest("tr")?.querySelector<HTMLElement>(".col-category")?.getBoundingClientRect();
    if (!editor || !cell || !cat) return ["нет поля названия"];
    for (const el of Array.from(editor.querySelectorAll<HTMLElement>("input, button, p"))) {
      const r = el.getBoundingClientRect();
      if (r.right > cell.right + 0.5 || r.left < cell.left - 0.5) out.push(`вылезает из ячейки: ${el.tagName} ${el.textContent?.slice(0, 30)}`);
      if (r.right > cat.left + 0.5) out.push(`налезает на категорию: ${el.tagName} ${el.textContent?.slice(0, 30)}`);
    }
    return out;
  });
  expect(bad).toEqual([]);
}

// s11 — «Переименовать» (D-59, 8.3.4 п.5): поле на месте названия с «Сохранить»/«Отмена», затем
// своё название с «из файла: …» и «Название изменено» с «Отменить».
test("s11 программы: переименование", async ({ page }) => {
  await tallWindow(page, 1300);
  const list = [
    ...programList(),
    { path: "c:\\program files\\epic games\\stalker2\\binaries\\win64\\stalker2-win64-shipping.exe", name: "Stalker2-Win64-Shipping", publisher: "GSC Game World", category: "games" as const, source: "catalog" as const, weekS: 9000 },
  ];
  await openPrograms(page, { pending: [], programs: list });
  const all = page.getByRole("region", { name: "Все программы" });
  const row = all.getByRole("row", { name: "Stalker2-Win64-Shipping", exact: true });
  // Точное имя внутри своей строки: имена соседних кнопок содержат название программы.
  await row.getByRole("button", { name: "Переименовать «Stalker2-Win64-Shipping»", exact: true }).click();
  const field = row.getByRole("textbox", { name: "Новое название для «Stalker2-Win64-Shipping»", exact: true });
  await expect(field).toBeFocused();
  await editorInCell(page);
  await hideButtonsInColumn(page);
  await rowsEven(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-rename-edit.png`, fullPage: true });

  await field.fill("Сталкер 2");
  await row.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByText("Название изменено")).toBeVisible();
  await expect(page.getByRole("button", { name: "Отменить", exact: true })).toBeVisible();
  const renamed = all.getByRole("row", { name: "Сталкер 2", exact: true });
  await expect(renamed.getByText("из файла: Stalker2-Win64-Shipping", { exact: true })).toBeVisible();
  await hideButtonsInColumn(page);
  await rowsEven(page);
  await nothingSticksOut(page);
  await pathsOneLine(page);
  await blocksApart(page);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s11-programs-rename.png`, fullPage: true });
});

// FB-9/FB-11 (UX-аудит 0.3.1): в окне 960 название резалось на 13 знаках («Minecraft Lau…»), а путь
// терял имя файла («c:\\program…ncher.exe»), хотя справа в строке было пусто: узкие колонки «Категория»,
// «За неделю», «Скрыть» были шире своего содержимого, а путь резал имя файла при коротком начале.
// Окно родителя не уже 960 x 620 (app.go): проверяем 960 x 640 и 960 x 620.
const fitList = () => [
  ...programList(),
  { path: "c:\\program files\\epic games\\stalker2\\binaries\\win64\\stalker2-win64-shipping.exe", name: "Сталкер 2", fileName: "Stalker2-Win64-Shipping", publisher: "GSC Game World", category: "games" as const, source: "catalog" as const, weekS: 9000 },
  { path: "c:\\program files\\epic games\\fortnite\\fortnitegame\\binaries\\win64\\fortniteclient-win64-shipping.exe", name: "Fortnite", publisher: "Epic Games, Inc.", category: "games" as const, source: "catalog" as const, weekS: 600 },
];

for (const [w, h] of [
  [960, 640],
  [960, 620],
]) {
  test(`s11 программы: название и имя файла целиком в окне ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await openPrograms(page, { pending: [], programs: fitList() });
    const all = page.getByRole("region", { name: "Все программы" });
    await expect(all.getByRole("row", { name: "Minecraft Launcher", exact: true })).toBeVisible();
    const report = await page.evaluate(() => {
      const out: Record<string, unknown>[] = [];
      for (const row of Array.from(document.querySelectorAll<HTMLElement>(".programs-table tbody tr"))) {
        const label = row.getAttribute("aria-label") ?? "";
        const path = row.dataset.path ?? "";
        const name = row.querySelector<HTMLElement>(".program-name")!;
        const nameShown = (name.querySelector('[aria-hidden="true"]') ?? name).textContent ?? "";
        const lh = parseFloat(getComputedStyle(name).lineHeight) || 20;
        const short = row.querySelector<HTMLElement>(".path-short")!;
        const tail = row.querySelector<HTMLElement>(".path-tail")!;
        const sb = short.getBoundingClientRect();
        const tb = tail.getBoundingClientRect();
        const fileName = row.querySelector<HTMLElement>(".program-filename");
        out.push({
          label,
          nameShown,
          nameLines: Math.round(name.getBoundingClientRect().height / lh),
          file: path.slice(path.lastIndexOf("\\")),
          tail: tail.textContent,
          tailInside: tb.left >= sb.left - 0.5 && tb.right <= sb.right + 0.5 && tb.bottom <= sb.bottom + 0.5 && tail.scrollWidth <= tail.clientWidth + 1,
          fromFile: fileName ? (fileName.querySelector('[aria-hidden="true"]') ?? fileName).textContent : null,
        });
      }
      return out;
    });
    const msg = JSON.stringify(report);
    const row = (label: string) => report.find((r) => r.label === label)!;
    // Название «Minecraft Launcher» видно целиком.
    expect(row("Minecraft Launcher").nameShown, msg).toBe("Minecraft Launcher");
    // Имя файла в пути у каждой строки целиком и в своей рамке (не обрезано и не спрятано).
    for (const r of report) {
      expect(r.tail, `имя файла обрезано: ${msg}`).toBe(r.file);
      expect(r.tailInside, `имя файла не помещается в рамку: ${msg}`).toBe(true);
    }
    expect(row("Minecraft Launcher").tail).toBe("\\minecraftlauncher.exe");
    expect(row("Сталкер 2").tail).toBe("\\stalker2-win64-shipping.exe");
    expect(row("Сталкер 2").fromFile, msg).toBe("из файла: Stalker2-Win64-Shipping");
    // Длинное название переносится на вторую строку, а не режется на первой: не больше двух строк.
    const long = row("Очень длинное название игры с пробелами и цифрами 2026");
    expect(long.nameLines, msg).toBe(2);
    expect(String(long.nameShown).startsWith("Очень длинное название игры"), msg).toBe(true);
    // «Запрещено» в списке категории целиком (ревью M4): строка с запрещённой программой есть.
    await expect(all.locator(".col-category select").filter({ has: page.locator('option[value="blocked"]:checked') })).not.toHaveCount(0);
    await categoriesFit(page);
    await hideButtonsInColumn(page);
    await rowsEven(page);
    await nothingSticksOut(page);
    await noHorizontalScroll(page);
    await page.screenshot({ path: `${SCREENS}/s11-programs-fit-${w}x${h}.png`, fullPage: true });
  });
}

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

// m4-programs-hints — пояснения на «Программах» (B-39, B-43): легенда кнопок решения под «Решение
// действует для всех детей», подсказка у неизвестного издателя, пояснение «Скрыть», «Скрыть нельзя»
// у запрещённой в своей ячейке, «Применить» без категории недоступна с причиной в той же строке
// панели. Самое узкое окно родителя 960 x 620 (PL-26) и 1200 x 820; окно не прокручивается вбок.
for (const [w, h] of [
  [960, 620],
  [1200, 820],
]) {
  test(`m4 программы: пояснения в окне ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await openPrograms(page, { pending: pendingPrograms(Date.now()), programs: programList() });
    const pending = page.getByRole("region", { name: /^Ждут решения/ });
    const legend = pending.getByRole("list", { name: "Что значат кнопки" });
    // Заголовок легенды виден и стоит над её строками (ревью M4).
    const legendTitle = pending.getByRole("heading", { name: "Что значат кнопки", level: 4 });
    await expect(legendTitle).toBeVisible();
    expect((await boxOf(legend)).top, "легенда не под своим заголовком").toBeGreaterThanOrEqual((await boxOf(legendTitle)).bottom - 0.5);
    await expect(legend.getByRole("listitem")).toHaveText([
      "Игра: время идёт в лимит игр и в общее время",
      "Учёба: время не тратит лимит",
      "Прочее: время идёт в общее время",
      "Запретить: программа будет закрываться сразу после запуска",
    ]);
    const lead = await boxOf(pending.getByText("Решение действует для всех детей", { exact: true }));
    const legendBox = await boxOf(legend);
    const firstItem = await boxOf(page.locator(".pending-item").first());
    expect(legendBox.top, "легенда не под «Решение действует для всех детей»").toBeGreaterThanOrEqual(lead.bottom - 0.5);
    expect(firstItem.top, "легенда не над списком ждущих").toBeGreaterThanOrEqual(legendBox.bottom - 0.5);
    await expect(page.locator(".pending-item").nth(1).getByText("Издатель неизвестен: проверьте, откуда этот файл", { exact: true })).toBeVisible();

    const all = page.getByRole("region", { name: "Все программы" });
    await expect(
      all.getByText(
        "Скрытая программа пропадает из списка и статистики, не закрывается и не тратит время. Вернуть её можно в фильтре «Скрытые». Запрещённую скрыть нельзя",
        { exact: true },
      ),
    ).toBeVisible();
    const bad = all.getByRole("row", { name: "Bad", exact: true });
    const nohide = await boxOf(bad.getByText("Скрыть нельзя", { exact: true }));
    const cell = await boxOf(bad.locator("td.col-action"));
    expect(nohide.right, "«Скрыть нельзя» за правым краем ячейки").toBeLessThanOrEqual(cell.right + 0.5);
    expect(nohide.left, "«Скрыть нельзя» за левым краем ячейки").toBeGreaterThanOrEqual(cell.left - 0.5);

    await all.getByRole("checkbox", { name: "Отметить «Brave»", exact: true }).check();
    const apply = all.getByRole("button", { name: "Применить", exact: true });
    await expect(apply).toHaveAttribute("aria-disabled", "true");
    const hint = all.getByText("Сначала выберите категорию", { exact: true });
    await expect(hint).toBeVisible();
    await expect(apply).toHaveAttribute("aria-describedby", "bulk-apply-hint");
    // Подсказка одной строкой и в той же строке панели, что «Применить» (рядом с выбором категории).
    const hb = await boxOf(hint);
    const ab = await boxOf(apply);
    const lh = await hint.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight) || 20);
    expect(hb.height, "подсказка «Применить» в две строки").toBeLessThanOrEqual(lh * 1.5);
    expect(Math.abs(hb.top + hb.height / 2 - (ab.top + ab.height / 2)), "подсказка не в строке «Применить»").toBeLessThanOrEqual(2);

    await decisionButtonsEven(page);
    await blocksApart(page);
    await nothingSticksOut(page);
    await categoriesFit(page);
    await hideButtonsInColumn(page);
    await rowsEven(page);
    await noHorizontalScroll(page);
    await wholePage(page);
    await page.screenshot({ path: `${SCREENS}/${w === 960 ? "m4-programs-hints.png" : "m4-programs-hints-1200.png"}`, fullPage: true });
  });
}

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

// s12 — «Дети → Лимиты» со столбцами «Игры» и «Видео и соцсети» (FR-16): подсказка «0 мин: в этот день
// играть нельзя» в среде, игры без лимита в воскресенье, пояснение про игры, видео и учёбу под таблицей.
test("s12 лимиты с играми", async ({ page }) => {
  await tallWindow(page, 900);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [child({ limits: gamesWeek() })] });
  await page.goto("/");
  await page.getByRole("button", { name: "Дети" }).click();
  await expect(page.getByRole("columnheader", { name: "Игры" })).toBeVisible();
  await expect(page.getByLabel("Часы игр, понедельник")).toHaveValue("1");
  await expect(page.getByRole("row", { name: /Среда/ }).getByText("0 мин: в этот день играть нельзя")).toBeVisible();
  await expect(page.getByLabel("Игры без лимита, воскресенье")).toBeChecked();
  const note = page.getByText("Время игр и видео входит в общее. Учёба в общее время не входит");
  await expect(note).toBeVisible();
  // Пояснение стоит под таблицей с зазором, а не прижато к ней.
  const table = await boxOf(page.locator(".limits-table"));
  const noteBox = await boxOf(note);
  expect(noteBox.top - table.bottom).toBeGreaterThanOrEqual(4 - 0.5);
  // Столбцы одинаковой ширины по содержимому не обязаны, но все целиком в окне.
  for (const name of ["Общее", "Игры", "Видео и соцсети"]) {
    const h = await boxOf(page.getByRole("columnheader", { name, exact: true }));
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

/** Блоки вкладки «Отдых» не налезают друг на друга и целиком в окне по ширине (PL-26). */
async function restBlocksApart(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    const sel = ".rest-tab .field-label, .rest-tab .field-input, .rest-tab .limits-unit, .rest-tab .field-hint, .rest-tab .field-error, .rest-count, .rest-sample, .rest-summary, .rest-switch";
    const els = [...document.querySelectorAll(sel)].filter((el) => !el.closest(".rest-sample-screen") || el.classList.contains("rest-sample"));
    const boxes = els.map((el) => ({ el, r: el.getBoundingClientRect(), name: `${el.className} «${(el.textContent ?? (el as HTMLInputElement).value ?? "").slice(0, 24)}»` }));
    const width = document.documentElement.clientWidth;
    for (const b of boxes) {
      if (b.r.width < 1) out.push(`${b.name}: сжато до нуля`);
      if (b.r.right > width + 0.5) out.push(`${b.name}: за правым краем окна`);
    }
    for (let i = 0; i < boxes.length; i++) {
      for (let k = i + 1; k < boxes.length; k++) {
        const a = boxes[i];
        const b = boxes[k];
        if (a.el.contains(b.el) || b.el.contains(a.el)) continue;
        const x = Math.min(a.r.right, b.r.right) - Math.max(a.r.left, b.r.left);
        const y = Math.min(a.r.bottom, b.r.bottom) - Math.max(a.r.top, b.r.top);
        if (x > 0.5 && y > 0.5) out.push(`${a.name} налезает на ${b.name}`);
      }
    }
    return out;
  });
  expect(bad, "на вкладке «Отдых» что-то налезает или вылезает").toEqual([]);
}

// s12 — вкладка «Отдых» (FR-37, 8.3.3 п.7): включено, «Только игры», порог 30, своё сообщение;
// итог словами совпадает с полями, образец «Как увидит ребёнок» с сообщением; светлая и тёмная
// тема, обычное окно родителя и самое узкое (960x620).
for (const v of [
  { name: "s12-rest-tab", scheme: "light" as const, w: 1200, h: 820 },
  { name: "s12-rest-tab-dark", scheme: "dark" as const, w: 1200, h: 820 },
  { name: "s12-rest-tab-960", scheme: "light" as const, w: 960, h: 620 },
]) {
  test(`s12 вкладка «Отдых»: ${v.name}`, async ({ page }) => {
    await page.setViewportSize({ width: v.w, height: v.h });
    await page.emulateMedia({ colorScheme: v.scheme });
    await mockProgram(page, {
      mode: "parent",
      sessionActive: true,
      children: [child({ limits: gamesWeek(), break: { enabled: true, afterMin: 60, counts: "games", warnMin: 5, restMin: 15, selfRestMin: 30, message: "Пойди погуляй с собакой" } })],
    });
    await page.goto("/");
    await page.getByRole("button", { name: "Дети" }).click();
    await page.getByRole("tablist", { name: "Правила ребёнка" }).getByRole("tab", { name: "Перерыв на отдых", exact: true }).click();
    await expect(page.getByRole("checkbox", { name: "Перерыв на отдых" })).toBeChecked();
    await expect(page.getByLabel("Засчитать как отдых, если не играл подряд")).toHaveValue("30");
    await expect(
      page.getByText("После 60 мин игр ребёнок увидит «Пора отдохнуть», через 5 мин компьютер закроется на 15 мин. Если сам не играл 30 мин подряд, это засчитается как отдых"),
    ).toBeVisible();
    // «Всё время, кроме учёбы»: учёба тоже засчитывается как отдых (R27), итог говорит это прямо.
    await page.getByRole("radio", { name: "Всё время, кроме учёбы" }).check();
    await expect(
      page.getByText("После 60 мин за компьютером ребёнок увидит «Пора отдохнуть», через 5 мин компьютер закроется на 15 мин. Если сам 30 мин подряд не был за компьютером или занимался только учёбой, это засчитается как отдых"),
    ).toBeVisible();
    await page.getByRole("radio", { name: "Только игры" }).check();
    const sample = page.getByRole("figure", { name: "Как увидит ребёнок" });
    await expect(sample.getByText("Пойди погуляй с собакой")).toBeVisible();
    await expect(sample.getByRole("img", { name: "Таймер 00:15:00" })).toBeVisible();
    // Картинка образца загрузилась (не пустой значок битой картинки).
    expect(await sample.locator("img.rest-sample-cat").evaluate((el) => (el as HTMLImageElement).naturalWidth)).toBeGreaterThan(0);
    // Ошибка у поля: порог меньше отдыха.
    await page.getByLabel("Засчитать как отдых, если не играл подряд").fill("10");
    await page.getByRole("button", { name: "Сохранить" }).click();
    await expect(page.getByText("Не меньше 15 мин: столько длится отдых")).toBeVisible();
    await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished)));
    await restBlocksApart(page);
    await noHorizontalScroll(page);
    await wholePage(page);
    await page.screenshot({ path: `${SCREENS}/${v.name}.png`, fullPage: true });
  });
}

/** Вкладка «Когда время вышло»: карточки вариантов не слипаются (зазор ≥ 16 px, приёмка P2 п.2). */
async function endModesApart(page: Page) {
  const cards = page.locator(".end-mode");
  await expect(cards).toHaveCount(2);
  const a = await boxOf(cards.nth(0));
  const b = await boxOf(cards.nth(1));
  expect(b.top - a.bottom, "варианты слиплись").toBeGreaterThanOrEqual(16 - 0.5);
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
  // Приёмка P2 п.3: на панели ровно одна фраза, одной строкой, без склейки со второй.
  const bar = page.getByRole("region", { name: "Сохранение изменений" });
  await expect(bar.locator(".savebar-main")).toHaveText("Есть несохранённые изменения");
  await expect(bar.getByRole("status")).toHaveCount(0);
  const barText = bar.locator(".savebar-text");
  const lines = await barText.evaluate((el) => el.getBoundingClientRect().height / (parseFloat(getComputedStyle(el).lineHeight) || 20));
  expect(lines, "фраза на панели переносится").toBeLessThan(1.5);
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
  await expect(page.getByText("Шаг 3 из 5")).toBeVisible();
  await limitCellsFit(page);
  await noHorizontalScroll(page);
  // Шаг целиком, с кнопками внизу; дальше снова окно мастера 880x680.
  await wholePage(page);
  await inFrame(page, page.getByRole("button", { name: "Далее" }));
  await page.screenshot({ path: `${SCREENS}/s12-wizard-time.png`, fullPage: true });
  await page.setViewportSize({ width: 880, height: 680 });
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Шаг 4 из 5")).toBeVisible();
  await expect(page.getByRole("table", { name: "Программы на этом компьютере" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Что это: Minecraft Launcher" })).toHaveValue("games");
  await expect(page.getByText(/Незнакомые программы будут ждать вашего решения/)).toBeVisible();
  // Кнопки шага в одну строку, одной высоты, под списком.
  const buttons = ["Назад", "Настрою позже", "Далее"].map((name) => page.getByRole("button", { name, exact: true }));
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

// --- M4: экран «Сайты» (FR-21, 8.3.5, T8) ------------------------------------------------

/** Домен в 120 знаков: переносится в строке, а не съедает «Удалить» (PL-26). */
const LONG_SITE = "ochen-dlinnyy-poddomen-dlya-proverki-perenosa.eshche-odin-dlinnyy-uroven-domena-v-spiske.zapreshchennyh-saytov-mama.test";

const siteSet = (): SiteDTO[] => [
  ...["example.com", "casino.example", LONG_SITE, "ads.example"].map((domain) => ({ domain, kind: "blocked" as const })),
  ...["uchi.ru", "resh.edu.ru"].map((domain) => ({ domain, kind: "study" as const })),
  ...DEFAULT_VIDEO_SITES.map((domain) => ({ domain, kind: "video_social" as const })),
];

/** Дети снимков: у Даши «18+» выключено. */
const sitesFamily = () => family().map((c) => (c.childId === 2 ? { ...c, adultFilter: false } : c));

async function openSites(page: Page, options: MockOptions = {}) {
  await mockProgram(page, { mode: "parent", sessionActive: true, children: sitesFamily(), sites: siteSet(), ...options });
  await page.goto("/");
  await page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Сайты", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Сайты", level: 2 })).toBeVisible();
  await expect(page.getByRole("tablist", { name: "Списки сайтов" })).toBeVisible();
}

/** Подсказка поля «Добавить сайт» в мастере. */
const SITE_HINT = "Можно вставить адрес страницы целиком: программа возьмёт из него только сайт. Например: youtube.com";
/** Та же подсказка на экране «Сайты»: там можно вставить сразу несколько адресов (B-49). */
const SITES_HINT =
  "Можно вставить адрес страницы целиком: программа возьмёт из него только сайт. Можно сразу несколько, по одному в строке. Например: youtube.com";

const sitesTab = (page: Page, title: string) =>
  page.getByRole("tablist", { name: "Списки сайтов" }).getByRole("tab", { name: new RegExp(`^${title} \\d+$`) });

/**
 * «лицензия CC BY-SA 4.0» в подписи источника не рвётся (ни на пробеле, ни на дефисе): все её
 * символы на одной строке. Замер по прямоугольникам символов фрагмента (Range.getClientRects).
 */
async function licenseOneLine(page: Page) {
  const report = await page.locator(".sites-source").first().evaluate((p) => {
    const node = p.firstChild as Text;
    const text = node.textContent ?? "";
    const start = text.indexOf("лицензия");
    if (start < 0) return { text, tops: [] as number[] };
    const tops: number[] = [];
    for (let i = start; i < text.length; i++) {
      const r = document.createRange();
      r.setStart(node, i);
      r.setEnd(node, i + 1);
      const box = r.getBoundingClientRect();
      if (box.width > 0) tops.push(Math.round(box.top));
    }
    return { text, tops };
  });
  expect(report.tops.length, `нет фрагмента «лицензия …»: ${report.text}`).toBeGreaterThan(0);
  expect(new Set(report.tops).size, `«лицензия CC BY-SA 4.0» на разных строках: ${JSON.stringify(report)}`).toBe(1);
}

/** Домены целиком внутри строк, кнопки «Удалить» в одну колонку справа. */
async function siteRowsFit(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    const list = document.querySelector<HTMLElement>(".sites-list")?.getBoundingClientRect();
    const rights: number[] = [];
    for (const row of Array.from(document.querySelectorAll<HTMLElement>(".sites-row"))) {
      const d = row.querySelector<HTMLElement>(".sites-domain")!.getBoundingClientRect();
      const b = row.querySelector<HTMLElement>("button")!.getBoundingClientRect();
      if (!list || d.right > list.right + 0.5 || d.right > b.left + 0.5) out.push(`домен вылезает: ${row.dataset.domain}`);
      if (d.width < 100) out.push(`домен сжат до ${d.width} px: ${row.dataset.domain}`);
      if (b.height > 48) out.push(`«Удалить» в две строки: ${row.dataset.domain}`);
      rights.push(b.right);
    }
    if (rights.some((r) => Math.abs(r - rights[0]) > 0.5)) out.push(`«Удалить» не в одну колонку: ${rights.join(", ")}`);
    return out;
  });
  expect(bad).toEqual([]);
}

// m4-sites — «Сайты» (8.3.5): переключатели «18+» по детям, под ними мелко источник списка;
// вкладки со счётчиками, «Сайтов: 4 из 1000», длинный домен переносится; «CC BY-SA 4.0» одной строкой.
test("m4 сайты: списки и 18+", async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 820 });
  await openSites(page);
  const group = page.getByRole("group", { name: "Блокировать сайты 18+" });
  await expect(group.getByRole("checkbox", { name: "Миша", exact: true })).toBeChecked();
  await expect(group.getByRole("checkbox", { name: "Даша", exact: true })).not.toBeChecked();
  const source = page.getByText("Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY\u2011SA 4.0");
  await expect(source).toBeVisible();
  expect((await boxOf(source)).top, "подпись источника не под переключателями").toBeGreaterThanOrEqual((await boxOf(group)).bottom - 0.5);
  await expect(page.getByRole("tablist", { name: "Списки сайтов" }).getByRole("tab")).toHaveText([
    "Запрещённые 4",
    "Учёба 2",
    "Видео и соцсети 9",
  ]);
  await expect(page.getByText("Сайтов: 4 из 1000", { exact: true })).toBeVisible();
  await expect(page.getByRole("list", { name: "Запрещённые: сайты" }).getByText(LONG_SITE, { exact: true })).toBeVisible();
  await siteRowsFit(page);
  await noHorizontalScroll(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m4-sites.png`, fullPage: true });
});

// m4-sites-dark — тот же экран в тёмной теме Windows.
test("m4 сайты: тёмная тема", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.setViewportSize({ width: 1200, height: 820 });
  await openSites(page);
  await siteRowsFit(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m4-sites-dark.png`, fullPage: true });
});

// m5-checkboxes-dark — флажки своим оформлением в тёмной теме (ревью окна M5): пустые «18+» у Даши и строк
// списка, отмеченные «18+» у Миши и строка casino.example, «выбрано не всё» у «Выбрать всё в списке».
// Рамка 2 px цветом рамки поля, а не рамка браузера (в тёмной теме сливалась с карточкой).
test("m5 флажки: тёмная тема, пустые и отмеченные", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await page.setViewportSize({ width: 1200, height: 820 });
  await openSites(page);
  const group = page.getByRole("group", { name: "Блокировать сайты 18+" });
  await expect(group.getByRole("checkbox", { name: "Миша", exact: true })).toBeChecked();
  await expect(group.getByRole("checkbox", { name: "Даша", exact: true })).not.toBeChecked();
  // Щелчок по названию сайта отмечает строку (флажок и домен в одной подписи).
  await page.locator(".sites-row", { hasText: "casino.example" }).locator(".sites-domain").click();
  await expect(page.getByRole("checkbox", { name: "Отметить «casino.example»", exact: true })).toBeChecked();
  await expect(page.getByText("Выбрано: 1", { exact: true })).toBeVisible();
  const look = await page.evaluate(() =>
    [...document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].map((el) => {
      const cs = getComputedStyle(el);
      // Ширина из стиля, а не из рамки: строки списка вне экрана не рисуются (content-visibility, B-51).
      return { appearance: cs.appearance, border: cs.borderTopWidth, width: cs.width };
    }),
  );
  expect(look.length).toBeGreaterThan(3);
  for (const l of look) expect(l).toEqual({ appearance: "none", border: "2px", width: "18px" });
  await siteRowsFit(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m5-checkboxes-dark.png`, fullPage: true });
});

// m4-sites-960 — наименьшее окно 960 x 620: без прокрутки вбок, поле и «Добавить» в одну строку.
test("m4 сайты: узкое окно", async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 620 });
  await openSites(page);
  const input = await boxOf(page.getByRole("textbox", { name: "Добавить сайт" }));
  const button = await boxOf(page.getByRole("button", { name: "Добавить", exact: true }));
  expect(Math.abs(input.top - button.top), "поле и «Добавить» не в одну строку").toBeLessThanOrEqual(0.5);
  expect(button.left).toBeGreaterThanOrEqual(input.right - 0.5);
  await licenseOneLine(page);
  await siteRowsFit(page);
  await noHorizontalScroll(page);
  // Снимок страницы целиком: окно растёт ровно на невидимый остаток, ширина 960 прежняя.
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m4-sites-960.png`, fullPage: true });
});

// m4-sites-error, m4-sites-undo — ошибка у поля простыми словами; удаление с «Отменить» на 10 с;
// после прокрутки к полю шапка на месте (PL-25).
test("m4 сайты: ошибка у поля и отмена удаления", async ({ page }) => {
  await openSites(page);
  const field = page.getByRole("textbox", { name: "Добавить сайт" });
  await field.fill("просто текст");
  await field.press("Enter");
  const error = page.getByText("Это не похоже на адрес сайта", { exact: true });
  await expect(error).toBeVisible();
  await expect(field).toHaveAttribute("aria-invalid", "true");
  await expect(field).toBeFocused();
  // Подсказка с примером не пропадает при ошибке: под ошибкой, в той же форме (ревью M4).
  const hint = page.getByText(SITES_HINT, { exact: true });
  await expect(hint).toBeVisible();
  expect((await boxOf(hint)).top, "подсказка не под ошибкой").toBeGreaterThanOrEqual((await boxOf(error)).bottom - 0.5);
  await page.screenshot({ path: `${SCREENS}/m4-sites-error.png`, fullPage: true });

  await page.getByRole("button", { name: "Удалить example.com", exact: true }).click();
  await expect(page.getByText("Удалён example.com", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Отменить", exact: true })).toBeVisible();
  await expect(sitesTab(page, "Запрещённые")).toHaveText("Запрещённые 3");
  await page.screenshot({ path: `${SCREENS}/m4-sites-undo.png`, fullPage: true });

  await field.evaluate((el) => el.scrollIntoView({ block: "start" }));
  const state = await page.evaluate(() => ({
    docTop: document.scrollingElement!.scrollTop,
    barTop: document.querySelector("header.topbar")!.getBoundingClientRect().top,
  }));
  expect(state).toEqual({ docTop: 0, barTop: 0 });
  await noHorizontalScroll(page);
});

// m4-sites-video — вкладка «Видео и соцсети» со стартовым набором из 9 доменов (R14).
test("m4 сайты: вкладка «Видео и соцсети»", async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 820 });
  await openSites(page);
  await sitesTab(page, "Видео и соцсети").click();
  await expect(sitesTab(page, "Видео и соцсети")).toHaveAttribute("aria-selected", "true");
  const list = page.getByRole("list", { name: "Видео и соцсети: сайты" });
  await expect(list.getByRole("listitem")).toHaveCount(9);
  await expect(page.getByText(/из 1000/)).toHaveCount(0);
  await siteRowsFit(page);
  await noHorizontalScroll(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m4-sites-video.png`, fullPage: true });
});

// m5-sites-paste, m5-sites-picked — несколько адресов по одному в строке (не-адрес остаётся в поле
// с ошибкой) и отметка строк с «Выбрано: N» и «Удалить» (B-49); ссылка «Открыть лимиты» (B-50).
test("m5 сайты: вставка нескольких адресов и отметка строк", async ({ page }) => {
  await openSites(page);
  const field = page.getByRole("textbox", { name: "Добавить сайт" });
  await field.fill("kino.example\nhttps://www.New.example/page\nкотики видео");
  await expect(field).toHaveAttribute("rows", "3");
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m5-sites-paste.png`, fullPage: true });
  await field.press("Enter");
  await expect(page.getByText("Добавлено сайтов: 2", { exact: true })).toBeVisible();
  await expect(field).toHaveValue("котики видео");
  await expect(page.getByText("Это не похоже на адрес сайта", { exact: true })).toBeVisible();
  await expect(sitesTab(page, "Запрещённые")).toHaveText("Запрещённые 6");

  await page.getByRole("checkbox", { name: "Отметить «casino.example»", exact: true }).check();
  await page.getByRole("checkbox", { name: "Отметить «ads.example»", exact: true }).check();
  await expect(page.getByText("Выбрано: 2", { exact: true })).toBeVisible();
  await siteRowsFit(page);
  await noHorizontalScroll(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/m5-sites-picked.png`, fullPage: true });
  await page.getByRole("button", { name: "Удалить отмеченные", exact: true }).click();
  await expect(page.getByText("Удалено сайтов: 2", { exact: true })).toBeVisible();
  await expect(sitesTab(page, "Запрещённые")).toHaveText("Запрещённые 4");

  await sitesTab(page, "Видео и соцсети").click();
  await page.getByRole("button", { name: "Открыть лимиты", exact: true }).click();
  await expect(page.getByRole("tab", { name: "Лимиты", exact: true })).toHaveAttribute("aria-selected", "true");
});

// --- M4: видео-лимит, шаг мастера «Сайты», «Итоги» по сайтам (T9) --------------------------

/** Неделя с видео: как gamesWeek, видео пн 20 мин, ср 0 (закрыто), вс без лимита, остальные 30 мин. */
const videoWeek = () =>
  gamesWeek().map((l) => ({
    ...l,
    videoMin: l.weekday === 1 ? 20 : l.weekday === 3 ? 0 : 30,
    videoUnlimited: l.weekday === 7,
  }));

/**
 * Один день на всех снимках T9 (PL-28): понедельник 19.10.2026 15:00. Лимиты понедельника по videoWeek:
 * 2 ч, игры 1 ч, видео 20 мин; за день 35 мин, из них игры 10 мин и видео 15 мин, учёба 5 мин.
 */
const M4_NOW = new Date(2026, 9, 19, 15, 0, 0);

/** Сайты сегодня: youtube.com 10 мин, vk.com 5 мин. */
const videoSitesToday = () => [
  { path: "youtube.com", name: "youtube.com", category: "video_social" as const, seconds: 600, kind: "site" as const },
  { path: "vk.com", name: "vk.com", category: "video_social" as const, seconds: 300, kind: "site" as const },
];

const videoChild = () =>
  child({ limits: videoWeek(), usedS: 2100, gamesUsedS: 600, videoUsedS: 900, studyUsedS: 300, programsToday: videoSitesToday() });

/** Подписи вкладок «Итогов» целиком внутри своих кнопок (длинная «Видео и соцсети» переносится), вкладки не налезают. */
async function totalsTabsFit(page: Page, where: string) {
  const report = await page
    .getByRole("region", { name: "Итоги" })
    .getByRole("tablist", { name: "Итоги по категориям" })
    .evaluate((list) =>
      [...list.querySelectorAll<HTMLElement>('[role="tab"]')].map((t) => {
        const b = t.getBoundingClientRect();
        const l = t.querySelector(".totals-tab-label")!.getBoundingClientRect();
        return { text: t.textContent ?? "", l: b.left, r: b.right, t: b.top, b: b.bottom, labelL: l.left, labelR: l.right, labelB: l.bottom, scroll: t.scrollWidth, client: t.clientWidth };
      }),
    );
  const problems: string[] = [];
  report.forEach((x, i) => {
    if (x.labelL < x.l - 0.5 || x.labelR > x.r + 0.5 || x.labelB > x.b + 0.5) problems.push(`«${x.text}»: подпись вне вкладки`);
    if (x.scroll > x.client + 1) problems.push(`«${x.text}»: не помещается (${x.scroll} > ${x.client})`);
    for (const o of report.slice(i + 1)) {
      const apart = x.r <= o.l + 0.5 || o.r <= x.l + 0.5 || x.b <= o.t + 0.5 || o.b <= x.t + 0.5;
      if (!apart) problems.push(`«${x.text}» и «${o.text}» налезают`);
    }
  });
  // Ряды плиток: все пять в ряд или 3 + 2, но не 4 + 1 с одинокой плиткой (визуальная приёмка T9).
  const rows = [...new Set(report.map((x) => Math.round(x.t)))].map((top) => report.filter((x) => Math.round(x.t) === top).length);
  if (rows.length > 1 && rows.some((n) => n < 2)) problems.push(`плитки по рядам ${rows.join(" + ")}`);
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report)}`).toEqual([]);
}

// m4-limits-video — «Дети → Лимиты» с третьим столбцом «Видео и соцсети» (FR-16, T9-2): в самом узком
// окне родителя 960x640 три столбца без прокрутки вбок (PL-26), подсказка «закрыты» в среде.
for (const [w, h] of [
  [960, 640],
  [1200, 820],
]) {
  test(`m4 лимиты с видео в окне ${w} x ${h}`, async ({ page }) => {
    await page.clock.setFixedTime(M4_NOW);
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "parent", sessionActive: true, children: [videoChild()] });
    await page.goto("/");
    await page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Дети" }).click();
    await expect(page.getByRole("columnheader", { name: "Видео и соцсети", exact: true })).toBeVisible();
    await expect(page.getByLabel("Минуты видео, понедельник")).toHaveValue("20");
    await expect(page.getByLabel("Видео без лимита, воскресенье")).toBeChecked();
    await expect(page.getByRole("row", { name: /Среда/ }).getByText("0 мин: в этот день видео и соцсети закрыты", { exact: true })).toBeVisible();
    await expect(page.getByText("Время игр и видео входит в общее. Учёба в общее время не входит", { exact: true })).toBeVisible();
    const content = await boxOf(page.locator("main.content"));
    for (const name of ["День", "Общее", "Игры", "Видео и соцсети"]) {
      const head = await boxOf(page.getByRole("columnheader", { name, exact: true }));
      expect(head.right, `столбец «${name}» за краем окна ${w}`).toBeLessThanOrEqual(Math.min(w, content.right) + 0.5);
    }
    await limitCellsFit(page);
    await noHorizontalScroll(page);
    await wholePage(page);
    await page.screenshot({ path: `${SCREENS}/${w === 960 ? "m4-limits-video.png" : "m4-limits-video-1200.png"}`, fullPage: true });
  });
}

/** Мастер до шага 5 «Сайты»: пароль, Миша, шаблон времени, программы без правок. */
async function wizardToSites(page: Page, check3?: () => Promise<void>) {
  await mockProgram(page, { configured: false, mode: "wizard" });
  await page.goto("/");
  await page.getByLabel("Пароль родителя").fill("секрет-123");
  await page.getByLabel("Повторите пароль").fill("секрет-123");
  await page.getByRole("button", { name: "Далее" }).click();
  await page.getByRole("checkbox", { name: /Миша/ }).check();
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Шаг 3 из 5")).toBeVisible();
  if (check3) await check3();
  await page.getByRole("button", { name: "Далее" }).click();
  await expect(page.getByText("Шаг 4 из 5")).toBeVisible();
  await expect(page.getByRole("table", { name: "Программы на этом компьютере" })).toBeVisible();
  await page.getByRole("button", { name: "Далее", exact: true }).click();
  await expect(page.getByText("Шаг 5 из 5")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Сайты", level: 2 })).toBeVisible();
}

// m4-wizard-sites — шаг мастера 5 «Сайты» (FR-2, 8.3.1 п.5–7): «18+» по детям с подписью источника рядом,
// стартовый набор видео и поле добавления одним блоком (PL-36), кнопки шага в одну строку.
for (const [w, h] of [
  [880, 680],
  [720, 560],
]) {
  test(`m4 мастер, шаг сайтов в окне ${w} x ${h}`, async ({ page }) => {
    await page.setViewportSize({ width: w, height: h });
    await wizardToSites(page, async () => {
      // Шаг «Время» в мастере - два столбца, без «Видео и соцсетей» (T9-2).
      await expect(page.getByRole("columnheader")).toHaveText(["День", "Общее", "Игры"]);
    });
    const group = page.getByRole("group", { name: "Блокировать сайты 18+" });
    await expect(group.getByRole("checkbox", { name: "Миша" })).toBeChecked();
    const source = page.getByText("Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY\u2011SA 4.0", { exact: true });
    await expect(source).toBeVisible();
    const g = await boxOf(group);
    const s = await boxOf(source);
    expect(s.top - g.bottom, "источник списка не рядом с переключателями").toBeLessThanOrEqual(24);
    const list = page.getByRole("list", { name: "Видео и соцсети: сайты" });
    await expect(list.getByRole("listitem")).toHaveCount(9);
    const field = page.getByRole("textbox", { name: "Добавить сайт" });
    await field.fill("https://Pikabu.ru/hot");
    await field.press("Enter");
    await expect(list.getByRole("listitem")).toHaveCount(10);
    await expect(list.getByRole("listitem").last()).toContainText("pikabu.ru");
    await field.fill("котики видео");
    await field.press("Enter");
    await expect(page.getByText("Это не похоже на адрес сайта", { exact: true })).toBeVisible();
    await expect(page.getByText(SITE_HINT, { exact: true })).toBeVisible();
    await licenseOneLine(page);
    await siteRowsFit(page);
    // Кнопки шага в одну строку, одной высоты, под списком.
    const buttons = ["Назад", "Настрою позже", "Готово"].map((name) => page.getByRole("button", { name, exact: true }));
    const boxes = await Promise.all(buttons.map((b) => boxOf(b)));
    for (const b of boxes) {
      expect(Math.abs(b.top - boxes[0].top)).toBeLessThanOrEqual(0.5);
      expect(Math.abs(b.height - boxes[0].height)).toBeLessThanOrEqual(0.5);
    }
    expect(boxes[0].top).toBeGreaterThanOrEqual((await boxOf(list)).bottom - 0.5);
    const form = await boxOf(page.locator(".wizard .sites-add"));
    const panel = await boxOf(page.locator(".wizard"));
    expect(form.right, "поле шире панели мастера").toBeLessThanOrEqual(panel.right + 0.5);
    await noHorizontalScroll(page);
    await wholePage(page);
    await inFrame(page, buttons[2]);
    await page.screenshot({ path: `${SCREENS}/${w === 880 ? "m4-wizard-sites.png" : "m4-wizard-sites-720.png"}`, fullPage: true });
  });
}

// m4-today-video — «Сегодня» (8.3.2 п.3): «Видео 15 мин из 20 мин» от службы под строкой игр; цифры те же,
// что на снимках «Статистики» и «Лимитов» того же дня (M4_NOW).
test("m4 сегодня с видео", async ({ page }) => {
  await page.clock.setFixedTime(M4_NOW);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [videoChild()] });
  await page.goto("/");
  const misha = page.getByRole("article", { name: "Миша" });
  await expect(misha.getByText("Использовано 35 мин из 2 ч", { exact: true })).toBeVisible();
  const games = misha.getByText("Игры 10 мин из 1 ч", { exact: true });
  const video = misha.getByText("Видео 15 мин из 20 мин", { exact: true });
  await expect(video).toBeVisible();
  const gb = await boxOf(games);
  const vb = await boxOf(video);
  expect(vb.top, "строка видео не под строкой игр").toBeGreaterThanOrEqual(gb.bottom - 0.5);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/m4-today-video.png`, fullPage: true });
});

// m4-my-stats-video — окно ребёнка (8.2.6 п.1, п.3): «Видео и соцсети» в «Сколько осталось» и вкладка
// «Видео и соцсети» со строками сайтов; подпись вкладки переносится внутри неё в узком окне.
for (const [w, h] of [
  [800, 640],
  [640, 520],
]) {
  test(`m4 моя статистика: видео в окне ${w} x ${h}`, async ({ page }) => {
    await page.clock.setFixedTime(M4_NOW);
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "mystats", children: [videoChild()], myChildId: 1 });
    await page.goto("/");
    const left = page.getByRole("region", { name: "Сколько осталось" });
    await expect(left.locator(".stat-row").filter({ hasText: "Видео и соцсети" })).toHaveText("Видео и соцсетиОсталось 5 мин из 20 мин");
    const totals = page.getByRole("region", { name: "Итоги" });
    const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
    await expect(tabs.getByRole("tab")).toHaveCount(5);
    await expect(page.getByText("Пока не считается")).toHaveCount(0);
    await tabs.getByRole("tab", { name: "Видео и соцсети", exact: true }).click();
    const panel = totals.locator(CATEGORY_PANEL);
    await expect(panel.getByText("Видео и соцсети за сегодня: 15 мин", { exact: true })).toBeVisible();
    await expect(panel.getByRole("list", { name: "Видео и соцсети: по сайтам" }).getByRole("listitem")).toHaveText(["youtube.com10 мин67%", "vk.com5 мин33%"]);
    await tabListFits(page, `видео, окно ${w} x ${h}`);
    await totalsTabsFit(page, `видео, окно ${w} x ${h}`);
    await tabsState(page, "Видео и соцсети", `видео, окно ${w} x ${h}`);
    await noHorizontalScroll(page);
    await totals.scrollIntoViewIfNeeded();
    await settled(page);
    await page.screenshot({ path: `${SCREENS}/${w === 800 ? "m4-my-stats-video.png" : "m4-my-stats-video-small.png"}`, fullPage: w === 800 });
  });
}

// m4-parent-stats-video — статистика родителя (FR-23): те же строки сайтов, «Всего» включает сайты.
test("m4 статистика родителя: видео", async ({ page }) => {
  await page.clock.setFixedTime(M4_NOW);
  await page.setViewportSize({ width: 960, height: 620 });
  await mockProgram(page, { mode: "parent", sessionActive: true, children: [videoChild(), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })] });
  await page.goto("/");
  await page.getByRole("article", { name: "Миша" }).getByRole("button", { name: "Статистика" }).click();
  const totals = page.getByRole("region", { name: "Итоги" });
  const tabs = totals.getByRole("tablist", { name: "Итоги по категориям" });
  const panel = totals.locator(CATEGORY_PANEL);
  await expect(panel.getByRole("list", { name: "Всего: по программам и сайтам" }).getByRole("listitem").filter({ hasText: "youtube.com" })).toHaveCount(1);
  await tabs.getByRole("tab", { name: "Видео и соцсети", exact: true }).click();
  await expect(panel.getByRole("list", { name: "Видео и соцсети: по сайтам" }).getByRole("listitem")).toHaveText(["youtube.com10 мин67%", "vk.com5 мин33%"]);
  await tabListFits(page, "статистика родителя, видео");
  await totalsTabsFit(page, "статистика родителя, видео");
  await tabsState(page, "Видео и соцсети", "статистика родителя, видео");
  await noHorizontalScroll(page);
  await settled(page);
  await page.screenshot({ path: `${SCREENS}/m4-parent-stats-video.png`, fullPage: true });
});

// --- M5: разделы статистики за период, «По дням», очистка (S21, S22, S48; 8.2.6, 8.3.7) -----------

/** Ребёнок эталонной недели G15 (вс 22.11.2026): пн 16 … сб 21 из «базы», сегодня game.exe 10 мин. */
const g15 = (over: Partial<MockChild> = {}) => child({ ...g15Fields(), ...over });

/** Родитель: окно статистики Миши через карточку «Сегодня». */
async function openParentStats(page: Page, children: MockChild[], events = g15Events()) {
  await mockProgram(page, { mode: "parent", sessionActive: true, children, events });
  await page.goto("/");
  await page.getByRole("article", { name: "Миша" }).getByRole("button", { name: "Статистика" }).click();
  await expect(page.getByRole("region", { name: "Итоги" })).toBeVisible();
}

async function choosePeriod(page: Page, name: string, used: string) {
  await page.getByRole("tablist", { name: "Период" }).getByRole("tab", { name, exact: true }).click();
  await expect(page.getByRole("region", { name: "Итоги" }).locator(".stat-row").filter({ hasText: used })).toBeVisible();
}

const barHits = (page: Page) => page.getByRole("region", { name: "По дням" }).getByRole("group", { name: "Столбики: стрелки влево и вправо" }).getByRole("img");

/** Прокрутка вбок только у графика: сколько он шире своей области (0 - помещается). */
const chartOverflow = (page: Page) =>
  page.locator(".daybars-scroll").evaluate((el) => ({ scroll: el.scrollWidth, client: el.clientWidth, left: el.scrollLeft }));

/**
 * Столбики и подписи «По дням» не налезают друг на друга, подписи внутри области графика, подсказка (если
 * есть) целиком в кадре окна. Всё в одной строке ошибки (журналы Actions недоступны).
 */
async function barsFit(page: Page, where: string) {
  const report = await page.locator("figure.daybars").evaluate((fig) => {
    const r = (el: Element) => {
      const b = el.getBoundingClientRect();
      return { l: b.left, r: b.right, t: b.top, b: b.bottom };
    };
    const scroll = r(fig.querySelector(".daybars-scroll")!);
    const labels = [...fig.querySelectorAll(".daybars-label")]
      .filter((l) => (l.textContent ?? "") !== "")
      .map((l) => {
        const range = document.createRange();
        range.selectNodeContents(l);
        const b = range.getBoundingClientRect();
        return { text: l.textContent ?? "", l: b.left, r: b.right };
      });
    // Подписи оси: прямоугольник текста, его рамка (обрезка overflow) и карточка «По дням».
    const area = r(fig.querySelector(".daybars-area")!);
    const figBox = r(fig);
    const card = r(fig.closest(".card") ?? fig.closest("section") ?? fig);
    const ticks = [...fig.querySelectorAll(".daybars-tick")].map((t) => {
      const range = document.createRange();
      range.selectNodeContents(t);
      const b = range.getBoundingClientRect();
      const el = t as HTMLElement;
      return { text: t.textContent ?? "", l: b.left, r: b.right, mid: (b.top + b.bottom) / 2, scroll: el.scrollWidth, client: el.clientWidth };
    });
    const tip = fig.querySelector(".daybars-tip");
    const content = r(document.querySelector("main.content")!);
    return { scroll, area, fig: figBox, card, ticks, labels, tip: tip ? { ...r(tip), text: tip.textContent ?? "" } : null, content, vw: window.innerWidth, vh: window.innerHeight };
  });
  const problems: string[] = [];
  report.labels.forEach((l, i) => {
    const next = report.labels[i + 1];
    if (next && l.r > next.l + 0.5) problems.push(`подписи «${l.text}» и «${next.text}» налезают`);
  });
  for (const t of report.ticks) {
    // Ось стоит на месте слева от прокручиваемой части: правый край подписи - не дальше левого края
    // видимой области столбиков (.daybars-scroll), левый - внутри рисунка.
    if (t.l < report.fig.l - 0.5) problems.push(`подпись оси «${t.text}» обрезана слева`);
    if (t.r > report.scroll.l + 0.5) problems.push(`подпись оси «${t.text}» заходит на столбики`);
    if (t.l < report.card.l - 0.5 || t.r > report.card.r + 0.5) problems.push(`подпись оси «${t.text}» вне карточки`);
    if (t.scroll > t.client + 1) problems.push(`подпись оси «${t.text}» обрезана (${t.scroll} > ${t.client})`);
  }
  // Ось на одной высоте со столбиками: «0» - у низа области графика, верхнее деление - у её верха.
  const [zero, top] = [report.ticks[0], report.ticks[report.ticks.length - 1]];
  if (zero && Math.abs(zero.mid - report.area.b) > 1.5) problems.push(`деление «${zero.text}» не у низа столбиков`);
  if (top && Math.abs(top.mid - report.area.t) > 1.5) problems.push(`деление «${top.text}» не у верха области`);
  if (report.tip) {
    const t = report.tip;
    if (t.l < report.content.l - 0.5 || t.r > report.content.r + 0.5 || t.r > report.vw + 0.5) problems.push(`подсказка «${t.text}» за краем окна`);
    if (t.t < report.scroll.t - 0.5) problems.push(`подсказка «${t.text}» обрезана сверху`);
  }
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report)}`).toEqual([]);
}

/**
 * Плитки «Итогов» (возврат 2 T5): значение и подпись каждой плитки целиком внутри её рамки (ничего не
 * обрезано и не торчит), плитки внутри карточки «Итогов». Всё в одной строке ошибки.
 */
async function tilesFit(page: Page, where: string) {
  const report = await page.getByRole("region", { name: "Итоги" }).evaluate((box) => {
    const r = (el: Element) => {
      const b = el.getBoundingClientRect();
      return { l: b.left, r: b.right, t: b.top, b: b.bottom };
    };
    const card = r(box);
    return [...box.querySelectorAll('.totals-tab[role="tab"]')].map((t) => {
      const parts = [...t.querySelectorAll(".totals-tab-label, .totals-tab-value")].map((p) => ({
        text: p.textContent ?? "",
        box: r(p),
        scroll: (p as HTMLElement).scrollWidth,
        client: (p as HTMLElement).clientWidth,
      }));
      return { text: t.textContent ?? "", box: r(t), scroll: (t as HTMLElement).scrollWidth, client: (t as HTMLElement).clientWidth, parts, card };
    });
  });
  const problems: string[] = [];
  for (const t of report) {
    if (t.scroll > t.client + 1) problems.push(`плитка «${t.text}»: содержимое шире рамки (${t.scroll} > ${t.client})`);
    if (t.box.l < t.card.l - 0.5 || t.box.r > t.card.r + 0.5) problems.push(`плитка «${t.text}» вне карточки`);
    for (const p of t.parts) {
      if (p.scroll > p.client + 1) problems.push(`«${p.text}» обрезано (${p.scroll} > ${p.client})`);
      if (p.box.l < t.box.l - 0.5 || p.box.r > t.box.r + 0.5 || p.box.b > t.box.b + 0.5) problems.push(`«${p.text}» выходит за плитку «${t.text}»`);
    }
  }
  expect(problems, `${where}: ${problems.join("; ")} | ${JSON.stringify(report.map((t) => [t.text, t.box, t.parts]))}`).toEqual([]);
}

// s21 — статистика родителя за неделю G15 (S21): «Итоги», «По дням» 7 столбиков (среда пустая), «Программы»,
// «Сайты», «Нарушения», «Копилка»; подпись «Статистика ведётся с»; светлая и тёмная тема.
for (const scheme of ["light", "dark"] as const) {
  test(`s21 статистика родителя: неделя G15, тема ${scheme}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.clock.setFixedTime(G15_NOW);
    await page.setViewportSize({ width: 960, height: 900 });
    await openParentStats(page, [g15()]);
    await choosePeriod(page, "Неделя", "Использовано за неделю");
    await expect(page.getByText("Статистика ведётся с 16 ноября 2026", { exact: true })).toBeVisible();
    await expect(barHits(page)).toHaveCount(7);
    await expect(barHits(page).nth(1)).toHaveAttribute("aria-label", "вт, 17 ноября: 2 ч 20 мин, учёба 0 мин");
    await expect(page.locator("svg.daybars-svg")).toHaveAttribute("aria-label", "По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
    await expect(page.locator("svg.daybars-svg line.daybar-empty")).toHaveCount(1);
    await expect(page.getByRole("table", { name: "Программы" }).locator("tbody tr")).toHaveText(["game.exe4 ч 45 мин45%", "other.exe4 ч 08 мин39%", "study.exe1 ч 35 мин15%"]);
    await expect(page.getByRole("table", { name: "Сайты" }).locator("tbody tr")).toHaveText(["youtube.com3 ч 40 мин100%"]);
    await expect(page.getByRole("list", { name: "Нарушения" }).getByRole("listitem")).toHaveCount(1);
    await expect(page.getByRole("region", { name: "Копилка" }).getByText("Сейчас в копилке: 25 мин", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Очистить статистику" })).toBeVisible();
    expect((await chartOverflow(page)).scroll).toBeLessThanOrEqual((await chartOverflow(page)).client);
    await barsFit(page, `неделя, тема ${scheme}`);
    await tilesFit(page, `неделя 960, тема ${scheme}`);
    await noHorizontalScroll(page);
    await wholePage(page);
    await page.screenshot({ path: `${SCREENS}/s21-parent-stats-week${scheme === "dark" ? "-dark" : ""}.png`, fullPage: true });
  });
}

// s21 — год G15: столбики по месяцам январь…ноябрь, пустые месяцы - отметкой, ноябрь 12 ч 33 мин.
test("s21 статистика родителя: год G15 по месяцам", async ({ page }) => {
  await page.clock.setFixedTime(G15_NOW);
  await page.setViewportSize({ width: 960, height: 900 });
  await openParentStats(page, [g15()]);
  await choosePeriod(page, "Год", "Использовано за год");
  await expect(barHits(page)).toHaveCount(11);
  await expect(barHits(page).last()).toHaveAttribute("aria-label", "ноябрь 2026: 12 ч 33 мин, учёба 1 ч 35 мин");
  await expect(page.locator(".daybars-label")).toHaveText(["янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя"]);
  await expect(page.locator("svg.daybars-svg line.daybar-empty")).toHaveCount(10);
  await barsFit(page, "год");
  await noHorizontalScroll(page);
  await wholePage(page);
  await page.screenshot({ path: `${SCREENS}/s21-parent-stats-year.png`, fullPage: true });
});

/** Октябрь 2026 с временем через день: 31 столбик «Месяца» на 31.10.2026. */
const october = () =>
  Array.from({ length: 30 }, (_, i) => i + 1)
    .filter((d) => d % 2 === 1)
    .map((d) => ({ day: `2026-10-${String(d).padStart(2, "0")}`, totalS: (30 + (d % 7) * 20) * 60, gamesS: 1800 }));
const OCT31 = new Date(2026, 9, 31, 15, 0, 0);

// s21 — 31 столбик месяца помещается без прокрутки вбок и в 960, и в 800 px, и в окне ребёнка 640 px.
for (const [w, h, mode] of [
  [960, 900, "parent"],
  [800, 640, "parent"],
  [640, 520, "mystats"],
] as const) {
  test(`s21 статистика: месяц 31 столбик в окне ${w} x ${h} (${mode})`, async ({ page }) => {
    await page.clock.setFixedTime(OCT31);
    await page.setViewportSize({ width: w, height: h });
    const kid = child({ usedS: 1200, gamesUsedS: 0, history: october() });
    if (mode === "parent") {
      await openParentStats(page, [kid], []);
    } else {
      await mockProgram(page, { mode: "mystats", children: [kid], myChildId: 1 });
      await page.goto("/");
    }
    await choosePeriod(page, "Месяц", "Использовано за месяц");
    await expect(barHits(page)).toHaveCount(31);
    await expect(page.locator(".daybars-label").filter({ hasText: /\S/ })).toHaveText(["чт 1", "чт 8", "чт 15", "чт 22", "чт 29"]);
    const o = await chartOverflow(page);
    expect(o.scroll, `график шире своей области: ${JSON.stringify(o)}`).toBeLessThanOrEqual(o.client);
    await barHits(page).nth(0).hover();
    await barsFit(page, `месяц, первый столбик, ${w} x ${h}`);
    await barHits(page).nth(30).hover();
    await barsFit(page, `месяц, последний столбик, ${w} x ${h}`);
    await page.mouse.move(1, 1);
    await tilesFit(page, `месяц ${w} x ${h} (${mode})`);
    await noHorizontalScroll(page);
    if (w === 960) await page.screenshot({ path: `${SCREENS}/s21-parent-stats-month31.png`, fullPage: true });
  });
}

// s21 — подписи оси любой длины целиком внутри графика и карточки: деления по 30 мин («1 ч 30 мин») и
// по 3 ч («12 ч»), у родителя 960 px и в окне ребёнка 640 px. Столбики под подписями дней (barsFit).
for (const [peak, ticks] of [
  [7000, ["0", "30 мин", "1 ч", "1 ч 30 мин", "2 ч"]],
  [40000, ["0", "3 ч", "6 ч", "9 ч", "12 ч"]],
] as const) {
  for (const [w, h, mode] of [
    [960, 900, "parent"],
    [640, 520, "mystats"],
  ] as const) {
    test(`s21 статистика: подписи оси до «${ticks[ticks.length - 1]}» в окне ${w} x ${h} (${mode})`, async ({ page }) => {
      await page.clock.setFixedTime(OCT31);
      await page.setViewportSize({ width: w, height: h });
      const kid = child({ usedS: 1200, gamesUsedS: 0, history: [{ day: "2026-10-05", totalS: peak, gamesS: 0 }] });
      if (mode === "parent") {
        await openParentStats(page, [kid], []);
      } else {
        await mockProgram(page, { mode: "mystats", children: [kid], myChildId: 1 });
        await page.goto("/");
      }
      await choosePeriod(page, "Месяц", "Использовано за месяц");
      await expect(page.locator(".daybars-tick")).toHaveText([...ticks]);
      await barsFit(page, `подписи оси до ${peak} с, ${w} x ${h}`);
      await noHorizontalScroll(page);
    });
  }
}

// s21 — «Всё время» за 36 месяцев: прокручивается вбок только график (сегодня виден справа), окно - нет.
test("s21 статистика родителя: всё время 36 месяцев", async ({ page }) => {
  await page.clock.setFixedTime(G15_NOW);
  await page.setViewportSize({ width: 960, height: 900 });
  const months = Array.from({ length: 35 }, (_, i) => {
    const d = new Date(2023, 11 + i, 10, 12);
    return { day: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-10`, totalS: (600 + (i % 6) * 900) * 60, studyS: (i % 3) * 1200 };
  });
  await openParentStats(page, [g15({ history: months })], []);
  await choosePeriod(page, "Всё время", "Использовано за всё время");
  await expect(barHits(page)).toHaveCount(36);
  await expect(barHits(page).first()).toHaveAttribute("aria-label", /^декабрь 2023: /);
  await expect(page.locator(".daybars-label").first()).toHaveText("дек 2023");
  const o = await chartOverflow(page);
  expect(o.scroll, "36 месяцев должны прокручиваться внутри графика").toBeGreaterThan(o.client);
  expect(o.left + o.client, `график не прокручен к сегодняшнему месяцу: ${JSON.stringify(o)}`).toBeGreaterThanOrEqual(o.scroll - 1);
  await expect(barHits(page).last()).toBeInViewport();
  await barsFit(page, "всё время");
  await tilesFit(page, "всё время 36 месяцев");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s21-parent-stats-all36.png`, fullPage: true });
});

// s21 — подсказка с клавиатуры (WCAG 2.1.1, 1.4.13): Tab на «сегодня», стрелкой влево до вторника,
// подсказка «вт, 17 ноября: …» над графиком и целиком в окне; Esc прячет.
test("s21 статистика родителя: подсказка столбика с клавиатуры", async ({ page }) => {
  await page.clock.setFixedTime(G15_NOW);
  await page.setViewportSize({ width: 960, height: 900 });
  await openParentStats(page, [g15()]);
  await choosePeriod(page, "Неделя", "Использовано за неделю");
  // Раздел целиком в кадре: подсказка стоит над графиком, фокус не должен прокрутить её за край.
  await page.getByRole("region", { name: "По дням" }).scrollIntoViewIfNeeded();
  // Столбик «сегодня» - единственный столбик в порядке Tab (tabIndex 0).
  await expect(barHits(page).nth(6)).toHaveAttribute("tabindex", "0");
  await barHits(page).nth(6).focus();
  await expect(page.locator(".daybars-tip")).toHaveText("вс, 22 ноября: 10 мин, учёба 0 мин");
  for (let i = 0; i < 5; i++) await page.keyboard.press("ArrowLeft");
  await expect(barHits(page).nth(1)).toBeFocused();
  const tip = page.locator(".daybars-tip");
  await expect(tip).toHaveText("вт, 17 ноября: 2 ч 20 мин, учёба 0 мин");
  await expect(tip).toBeInViewport({ ratio: 1 });
  const [tb, area] = await Promise.all([boxOf(tip), boxOf(page.locator(".daybars-area"))]);
  expect(tb.bottom, "подсказка накрывает столбики").toBeLessThanOrEqual(area.top + 0.5);
  await barsFit(page, "подсказка вторника");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s21-parent-stats-tip.png` });
  await page.keyboard.press("Escape");
  await expect(tip).toHaveCount(0);
  await expect(barHits(page).nth(1)).toBeFocused();
});

// s22 — «Моя статистика» за неделю G15 (S22): те же разделы о себе, без выбора ребёнка, очистки и полей
// (кроме промокода); окно 800 x 640 не прокручивается целиком, шапка на месте (PL-25).
for (const [scheme, w, h] of [
  ["light", 800, 640],
  ["dark", 800, 640],
  ["light", 640, 520],
] as const) {
  test(`s22 моя статистика: неделя G15, тема ${scheme}, окно ${w} x ${h}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.clock.setFixedTime(G15_NOW);
    await page.setViewportSize({ width: w, height: h });
    await mockProgram(page, { mode: "mystats", children: [g15()], myChildId: 1, events: g15Events() });
    await page.goto("/");
    await choosePeriod(page, "Неделя", "Использовано за неделю");
    await expect(page.getByText("Статистика ведётся с 16 ноября 2026", { exact: true })).toBeVisible();
    await expect(barHits(page)).toHaveCount(7);
    await expect(page.getByRole("table", { name: "Программы" }).locator("tbody tr")).toHaveCount(3);
    await expect(page.getByRole("list", { name: "Нарушения" }).getByRole("listitem")).toHaveCount(1);
    await expect(page.getByRole("textbox")).toHaveCount(1);
    await expect(page.getByRole("combobox")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Очистить статистику" })).toHaveCount(0);
    await expect(page.getByRole("group", { name: "Ребёнок" })).toHaveCount(0);
    const head = await page.locator("header.topbar").evaluate((el) => el.getBoundingClientRect().bottom);
    expect(await page.locator("main.content").evaluate((el) => el.getBoundingClientRect().top)).toBeGreaterThanOrEqual(head - 0.5);
    expect(await page.evaluate(() => document.scrollingElement?.scrollTop ?? 0)).toBe(0);
    await barsFit(page, `окно ребёнка ${w} x ${h}, тема ${scheme}`);
    await tilesFit(page, `окно ребёнка ${w} x ${h}, тема ${scheme}`);
    await noHorizontalScroll(page);
    if (w !== 800) return;
    await wholePage(page);
    await inFrame(page, page.getByRole("button", { name: "Настройки для родителя" }));
    await page.screenshot({ path: `${SCREENS}/s22-my-stats-week${scheme === "dark" ? "-dark" : ""}.png`, fullPage: true });
  });
}

// s48 — «Очистить статистику» (FR-38, 8.3.7 п.3): окно подтверждения с фокусом на «Отмена», затем
// «Статистика Миши очищена», «Итоги» нули и «Статистика ведётся с 22 ноября 2026».
test("s48 очистка статистики: подтверждение и результат", async ({ page }) => {
  await page.clock.setFixedTime(G15_NOW);
  await page.setViewportSize({ width: 960, height: 900 });
  await openParentStats(page, [g15(), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })]);
  await page.getByRole("group", { name: "Ребёнок" }).getByRole("button", { name: "Миша" }).click();
  await choosePeriod(page, "Неделя", "Использовано за неделю");
  // Окно открыто с клавиатуры: рамка фокуса (:focus-visible) на «Отмена» видна на снимке.
  await page.getByRole("button", { name: "Очистить статистику" }).focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "Удалить всю статистику Миши?" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Остаток на сегодня и копилка не изменятся. Это нельзя отменить.", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Отмена" })).toBeFocused();
  expect(await dialog.getByRole("button", { name: "Отмена" }).evaluate((el) => el.matches(":focus-visible")), "рамка фокуса на «Отмена» не видна").toBe(true);
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s48-clear-stats-dialog.png` });
  await dialog.getByRole("button", { name: "Удалить" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator(".stat-clear-status")).toHaveText("Статистика Миши очищена");
  await expect(page.getByText("Статистика ведётся с 22 ноября 2026", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Итоги" }).getByText("За этот период пока нет данных", { exact: true })).toBeVisible();
  await page.getByRole("tablist", { name: "Период" }).getByRole("tab", { name: "Сегодня", exact: true }).click();
  await expect(page.getByRole("region", { name: "Итоги" }).locator(".stat-row").filter({ hasText: "Использовано сегодня" })).toHaveText("Использовано сегодня0 мин");
  await barsFit(page, "после очистки, 960");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s48-clear-stats-done.png`, fullPage: true });
});

// s48 — «Очистить журнал» (FR-38, 8.3.7 п.3): окно открыто с клавиатуры, фокус и рамка на «Отмена»;
// после «Удалить» - «Журнал Миши очищен», в журнале Миши одна запись, у Даши записи на месте.
for (const scheme of ["light", "dark"] as const) {
  test(`s48 очистка журнала: подтверждение и результат${scheme === "dark" ? ", тёмная тема" : ""}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await openJournal(page);
    await page.getByLabel("Ребёнок").selectOption({ label: "Миша" });
    await expect(page.locator(".journal-item")).toHaveCount(2);
    await page.getByRole("button", { name: "Очистить журнал" }).focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Удалить весь журнал Миши?" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Это нельзя отменить.", { exact: true })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Отмена" })).toBeFocused();
    expect(await dialog.getByRole("button", { name: "Отмена" }).evaluate((el) => el.matches(":focus-visible")), "рамка фокуса на «Отмена» не видна").toBe(true);
    await noHorizontalScroll(page);
    const dark = scheme === "dark" ? "-dark" : "";
    await page.screenshot({ path: `${SCREENS}/s48-clear-journal-dialog${dark}.png` });
    // Esc закрывает окно, ничего не удаляя; фокус возвращается на кнопку.
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Очистить журнал" })).toBeFocused();
    await expect(page.locator(".journal-item")).toHaveCount(2);
    await page.keyboard.press("Enter");
    await dialog.getByRole("button", { name: "Удалить" }).click();
    await expect(dialog).toHaveCount(0);
    await expect(page.locator(".stat-clear-status")).toHaveText("Журнал Миши очищен");
    await expect(page.locator(".journal-item")).toHaveCount(1);
    await expect(page.locator(".journal-item .journal-text")).toHaveText("Миша: журнал очищен");
    await noHorizontalScroll(page);
    if (scheme === "light") await page.screenshot({ path: `${SCREENS}/s48-clear-journal-done.png`, fullPage: true });
    await page.getByLabel("Ребёнок").selectOption({ label: "Даша" });
    await expect(page.getByText("Даша: попытка открыть запрещённое: сайт example.com")).toBeVisible();
    await expect(page.locator(".stat-clear-status")).toHaveText("");
  });
}

// --- S49: раздел «Промокоды» (FR-35, R26, 8.3.10) ------------------------------------------

const S49_NOW = new Date(2026, 10, 11, 18, 40, 0).getTime();
const S49_DAY0 = new Date(2026, 10, 11, 9, 0, 0).getTime();
const S49_FAMILY = () => [child({}), child({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" })];

/** 20 действующих кодов, каждый третий - 16 знаков (предел R26); создавались раз в полчаса. */
const s49Active = () =>
  Array.from({ length: 20 }, (_, i) => ({
    codeDisplay: i % 3 === 0 ? `KANIKULY${String(i).padStart(2, "0")}MWQXYZ` : `LETO${String(i + 1).padStart(2, "0")}`,
    minutes: [30, 15, 60, 120, 600][i % 5],
    createdAtMs: S49_DAY0 + i * 30 * 60_000,
  }));

/** Использованные: у кого и когда; два кода по 16 знаков, один старый строчными (показ - заглавными). */
const s49Used = () => [
  { codeDisplay: "SUPERBONUS2026XY", minutes: 20, createdAtMs: S49_DAY0 - 86400_000, usedAtMs: S49_NOW - 10 * 60_000, usedByChild: 1 },
  { codeDisplay: "pizza", minutes: 30, createdAtMs: S49_DAY0 - 2 * 86400_000, usedAtMs: S49_NOW - 3 * 3600_000, usedByChild: 2 },
  { codeDisplay: "WWWWMMMMWWWWMMMM", minutes: 600, createdAtMs: S49_DAY0 - 3 * 86400_000, usedAtMs: S49_NOW - 26 * 3600_000, usedByChild: 1 },
  { codeDisplay: "A1234", minutes: 15, createdAtMs: S49_DAY0 - 4 * 86400_000, usedAtMs: S49_NOW - 50 * 3600_000, usedByChild: 2 },
];

async function openPromos(page: Page, promos = [...s49Active(), ...s49Used()]) {
  await page.clock.setFixedTime(S49_NOW);
  await mockProgram(page, { mode: "parent", sessionActive: true, children: S49_FAMILY(), promos });
  await page.goto("/");
  await page.getByRole("navigation", { name: "Разделы" }).getByRole("button", { name: "Промокоды" }).click();
  await expect(page.getByRole("heading", { name: "Промокоды", level: 2 })).toBeVisible();
}

/**
 * Значение не обрезано: код в строке списка и в списке новых кодов целиком в своей рамке
 * (scrollWidth <= clientWidth), строка не шире панели.
 */
async function codesNotClipped(page: Page) {
  const bad = await page.evaluate(() => {
    const out: string[] = [];
    const panel = document.querySelector("#promos-panel")!.getBoundingClientRect();
    for (const el of document.querySelectorAll<HTMLElement>(".promo-item .promo-code, .promo-codes li")) {
      if (el.scrollWidth > el.clientWidth + 1) out.push(`«${el.textContent}» обрезан: ${el.scrollWidth} > ${el.clientWidth}`);
    }
    for (const li of document.querySelectorAll<HTMLElement>(".promo-item")) {
      const r = li.getBoundingClientRect();
      if (r.right > panel.right + 0.5) out.push(`строка «${li.textContent}» шире панели`);
      for (const part of li.children) {
        const p = part.getBoundingClientRect();
        if (p.right > r.right + 0.5) out.push(`«${part.textContent}» за краем строки`);
      }
    }
    return out;
  });
  expect(bad).toEqual([]);
}

for (const scheme of ["light", "dark"] as const) {
  const dark = scheme === "dark" ? "-dark" : "";
  test(`s49 промокоды: действующие${scheme === "dark" ? ", тёмная тема" : ""}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.setViewportSize({ width: 960, height: 900 });
    await openPromos(page);
    const tabs = page.getByRole("tablist", { name: "Промокоды" });
    await expect(tabs.getByRole("tab", { name: "Действующие (20)" })).toHaveAttribute("aria-selected", "true");
    await expect(tabs.getByRole("tab", { name: "Использованные (4)" })).toBeVisible();
    const list = page.getByRole("list", { name: "Действующие промокоды" });
    await expect(list.getByRole("listitem")).toHaveCount(20);
    // Новые сверху: последний созданный «LETO20» на 600 мин в 18:30.
    await expect(list.getByRole("listitem").first()).toHaveText("LETO20600 минсоздан 11 ноября в 18:30");
    await expect(list.locator(".promo-code").first()).toHaveCSS("font-family", /Cascadia Mono|Consolas|Courier New|monospace/);
    await noHorizontalScroll(page);
    await codesNotClipped(page);
    await wholePage(page);
    await page.screenshot({ path: `${SCREENS}/s49-promos-active${dark}.png`, fullPage: true });
  });

  test(`s49 промокоды: придумать коды${scheme === "dark" ? ", тёмная тема" : ""}`, async ({ page }) => {
    await page.emulateMedia({ colorScheme: scheme });
    await page.setViewportSize({ width: 960, height: 900 });
    await openPromos(page);
    const gen = page.getByRole("region", { name: "Придумать коды" });
    await expect(gen.getByLabel("Сколько")).toHaveValue("10");
    await expect(gen.getByLabel("Длина")).toHaveValue("6");
    await expect(gen.getByRole("radio", { name: "Буквы и цифры" })).toBeChecked();
    await expect(gen.getByLabel("Минуты")).toHaveValue("30");
    await gen.getByRole("button", { name: "Придумать" }).click();
    await expect(page.locator(".promo-status")).toHaveText("Готово: 10 кодов на 30 мин");
    // Сообщение отделено от карточки «Создать код», как сообщения очистки статистики и журнала.
    const gap = await page.evaluate(() => {
      const st = document.querySelector(".promo-status")!.getBoundingClientRect();
      const card = document.querySelector(".promo-status ~ .panel")!.getBoundingClientRect();
      return card.top - st.bottom;
    });
    expect(gap, "«Готово» вплотную к карточке").toBeCloseTo(12, 0);
    const fresh = gen.getByRole("list", { name: "Новые коды" });
    await expect(fresh.getByRole("listitem")).toHaveCount(10);
    for (const code of await fresh.getByRole("listitem").allTextContents()) expect(code).toMatch(/^[A-HJ-NP-Z2-9]{6}$/);
    await expect(gen.getByRole("button", { name: "Скопировать все" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Действующие (30)" })).toHaveAttribute("aria-selected", "true");
    // Коды выделяются мышью (их можно скопировать вручную, если буфер обмена не дал).
    await expect(fresh).toHaveCSS("user-select", "text");
    await noHorizontalScroll(page);
    await codesNotClipped(page);
    await page.screenshot({ path: `${SCREENS}/s49-promos-generated${dark}.png` });
  });
}

test("s49 промокоды: использованные", async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 900 });
  await openPromos(page);
  await page.getByRole("tab", { name: "Использованные (4)" }).click();
  const list = page.getByRole("list", { name: "Использованные промокоды" });
  await expect(list.getByRole("listitem")).toHaveCount(4);
  await expect(list.getByRole("listitem").first()).toHaveText("SUPERBONUS2026XY20 миниспользован 11 ноября в 18:30, Миша");
  await expect(list.getByText("PIZZA", { exact: true })).toBeVisible();
  await expect(list.getByText("использован 11 ноября в 15:40, Даша", { exact: true })).toBeVisible();
  // «Очистить историю» сверху справа, над списком.
  const pos = await page.evaluate(() => {
    const b = [...document.querySelectorAll("button")].find((x) => x.textContent === "Очистить историю")!.getBoundingClientRect();
    const l = document.querySelector('[aria-label="Использованные промокоды"]')!.getBoundingClientRect();
    return { btnRight: b.right, btnBottom: b.bottom, listRight: l.right, listTop: l.top };
  });
  expect(pos.btnBottom, "«Очистить историю» не над списком").toBeLessThanOrEqual(pos.listTop);
  expect(Math.abs(pos.btnRight - pos.listRight), "«Очистить историю» не справа").toBeLessThan(2);
  await noHorizontalScroll(page);
  await codesNotClipped(page);
  await page.screenshot({ path: `${SCREENS}/s49-promos-used.png`, fullPage: true });
});

test("s49 промокоды: ошибка у поля «Код»", async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 900 });
  await openPromos(page);
  const create = page.getByRole("region", { name: "Создать код" });
  const code = create.getByLabel("Код", { exact: true });
  await expect(code).toHaveAttribute("placeholder", "A1234");
  // Ширина колонки «Код» не зависит от ошибки: «Минуты» и «Создать» стоят на месте.
  const minutesBox = create.getByLabel("Минуты", { exact: true });
  const submitBox = create.getByRole("button", { name: "Создать" });
  const xBefore = { minutes: (await minutesBox.boundingBox())!.x, submit: (await submitBox.boundingBox())!.x };
  await code.fill("код!");
  await create.getByRole("button", { name: "Создать" }).click();
  const error = create.getByText("Только латинские буквы A-Z и цифры", { exact: true });
  await expect(error).toBeVisible();
  expect((await minutesBox.boundingBox())!.x, "«Минуты» сдвинулись из-за ошибки").toBeCloseTo(xBefore.minutes, 0);
  expect((await submitBox.boundingBox())!.x, "«Создать» сдвинулась из-за ошибки").toBeCloseTo(xBefore.submit, 0);
  // Ошибка переносится в пределах ширины поля.
  const errBox = (await error.boundingBox())!;
  const codeBox = (await code.boundingBox())!;
  expect(errBox.x + errBox.width, "ошибка шире поля «Код»").toBeLessThanOrEqual(codeBox.x + codeBox.width + 1);
  await expect(code).toHaveAttribute("aria-invalid", "true");
  await expect(code).toBeFocused();
  // Ошибка под своим полем, а не общим абзацем под формой.
  const pos = await page.evaluate(() => {
    const input = document.querySelector<HTMLInputElement>('.promo-code-field input')!.getBoundingClientRect();
    const err = document.querySelector(".promo-code-field .field-error")!.getBoundingClientRect();
    return { inLeft: input.left, inBottom: input.bottom, errLeft: err.left, errTop: err.top };
  });
  expect(pos.errTop).toBeGreaterThanOrEqual(pos.inBottom);
  expect(Math.abs(pos.errLeft - pos.inLeft)).toBeLessThan(2);
  await expect(page.getByRole("tab", { name: "Действующие (20)" })).toBeVisible();
  // Enter в поле нажимает «Создать»: латинский код создаётся и встаёт первым в «Действующих».
  await code.fill("ab12");
  await code.press("Enter");
  await expect(page.locator(".promo-status")).toHaveText("Промокод «AB12» на 30 минут готов");
  await expect(page.getByRole("list", { name: "Действующие промокоды" }).getByRole("listitem").first()).toContainText("AB12");
  await code.fill("код!");
  await code.press("Enter");
  await expect(error).toBeVisible();
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s49-promos-code-error.png` });
});

test("s49 промокоды: очистить историю", async ({ page }) => {
  await page.setViewportSize({ width: 960, height: 900 });
  await openPromos(page);
  await page.getByRole("tab", { name: "Использованные (4)" }).click();
  // Окно открыто с клавиатуры: рамка фокуса (:focus-visible) на «Отмена» видна на снимке.
  const clear = page.getByRole("button", { name: "Очистить историю" });
  await clear.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "Удалить все использованные коды?" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Действующие останутся.", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Отмена" })).toBeFocused();
  expect(await dialog.getByRole("button", { name: "Отмена" }).evaluate((el) => el.matches(":focus-visible")), "рамка фокуса на «Отмена» не видна").toBe(true);
  // «Удалить» не красная (ТЗ 8.0 п.3): красный - только статус.
  const del = await dialog.getByRole("button", { name: "Удалить" }).evaluate((el) => {
    const css = getComputedStyle(el);
    return { bg: css.backgroundColor, color: css.color, border: css.borderTopColor, radius: css.borderRadius };
  });
  expect(del.radius).toBe("6px");
  for (const c of [del.bg, del.color, del.border]) expect(c, "«Удалить» красная").not.toBe("rgb(179, 38, 30)");
  await noHorizontalScroll(page);
  await page.screenshot({ path: `${SCREENS}/s49-promos-clear-dialog.png` });
  // Esc закрывает окно, ничего не удаляя; фокус возвращается на кнопку.
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(clear).toBeFocused();
  await expect(page.getByRole("list", { name: "Использованные промокоды" }).getByRole("listitem")).toHaveCount(4);
  await page.keyboard.press("Enter");
  await dialog.getByRole("button", { name: "Удалить" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator(".promo-status")).toHaveText("История промокодов очищена");
  await expect(page.getByText("Использованных кодов нет", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Очистить историю" })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "Использованные (0)" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Действующие (20)" })).toBeVisible();
});

// Узкое окно: длинные коды (16 знаков) целиком и в 640, и в 960 px; без прокрутки вбок.
for (const width of [640, 960]) {
  test(`s49 промокоды: длинные коды не обрезаны, ${width} px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 800 });
    await openPromos(page);
    await noHorizontalScroll(page);
    await codesNotClipped(page);
    const gen = page.getByRole("region", { name: "Придумать коды" });
    await gen.getByLabel("Длина").fill("12");
    await gen.getByRole("button", { name: "Придумать" }).click();
    await expect(gen.getByRole("list", { name: "Новые коды" }).getByRole("listitem")).toHaveCount(10);
    await noHorizontalScroll(page);
    await codesNotClipped(page);
    await page.getByRole("tab", { name: "Использованные (4)" }).click();
    await noHorizontalScroll(page);
    await codesNotClipped(page);
  });
}

// Окно 800×640: прокручивается только main.content, шапка на месте; стрелки переключают вкладки.
test("s49 промокоды: окно 800×640 и клавиатура", async ({ page }) => {
  await page.setViewportSize({ width: 800, height: 640 });
  await openPromos(page);
  await noHorizontalScroll(page);
  const scroller = page.locator("main.content");
  expect(await scroller.evaluate((el) => el.scrollHeight > el.clientHeight), "раздел помещается целиком: прокрутку не проверить").toBe(true);
  const active = page.getByRole("tab", { name: "Действующие (20)" });
  await active.focus();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("tab", { name: "Использованные (4)" })).toBeFocused();
  await expect(page.getByRole("tab", { name: "Использованные (4)" })).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Home");
  await expect(active).toBeFocused();
  await scroller.evaluate((el) => el.scrollTo(0, el.scrollHeight));
  await noHorizontalScroll(page);
  expect(await page.evaluate(() => document.documentElement.scrollTop)).toBe(0);
});
