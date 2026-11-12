import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import MyStats from "./MyStats";
import {
  DEFAULT_PASSWORD,
  G15_NOW,
  g15Child,
  g15Events,
  installBackend,
  mockChild,
  S42_NOW,
  s42Child,
  uninstallBackend,
  type MockBackend,
} from "../__mocks__/backend";
import type { ProgramTime } from "../types";

// FR-12, R21, сценарий 8.2.6: окно «Моя статистика» только для чтения, в M1 - «Сегодня» и «Мои правила».

let mock: MockBackend;

// Ребёнку на «ты» (A4). \b в JS не видит кириллицу, поэтому границы слов через \p{L}.
const YOU_FORM = /(?<!\p{L})(вы|вам|вас|ваш\p{L}*|попробуйте)(?!\p{L})/iu;

const NOW = new Date(2026, 9, 19, 12, 0, 0); // пн, 19 октября 2026

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ mode: "mystats", ...opts });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<MyStats />);
  return user;
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("MyStats: окно только для чтения (R21)", () => {
  it("в заголовке пометка «Только просмотр»", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Моя статистика" })).toBeTruthy();
    expect(screen.getByText("Только просмотр")).toBeTruthy();
  });

  // Ловит: в окне ребёнка появилось поле или список для ввода: он ничего не меняет (R21).
  // Единственное поле — «Ввести промокод» в разделе «Бонусы» (R26, D-50).
  it("нет полей ввода, кроме промокода в «Бонусах»", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    const bonuses = await screen.findByRole("region", { name: "Бонусы" });
    const fields = Array.from(document.querySelectorAll("input, textarea, select, [contenteditable]"));
    expect(fields).toHaveLength(1);
    expect(bonuses.contains(fields[0])).toBe(true);
    expect(screen.getByRole("textbox", { name: "Промокод" })).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  // Ловит (S22, R21): в окне ребёнка появились выбор ребёнка, очистка статистики или поле статистики.
  it("S22: статистика за неделю без полей, выбора ребёнка и очистки", async () => {
    vi.setSystemTime(G15_NOW);
    const user = setup({ children: [g15Child()], events: g15Events() });
    await user.click(await screen.findByRole("tab", { name: "Неделя" }));
    await screen.findByRole("region", { name: "По дням" });
    const fields = Array.from(document.querySelectorAll("input, textarea, select, [contenteditable]"));
    expect(fields).toHaveLength(1);
    expect(screen.getByRole("region", { name: "Бонусы" }).contains(fields[0])).toBe(true);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: /Очистить/ })).toBeNull();
    expect(screen.queryByRole("group", { name: "Ребёнок" })).toBeNull();
    expect(new Set(mock.calls.map((c) => c.command))).toEqual(new Set(["MyToday", "GetBonus"]));
  });

  // Ловит: ребёнок видит статистику другого ребёнка или чужое имя (R21).
  it("только свои данные: чужого ребёнка нет нигде", async () => {
    setup({
      myChildId: 2,
      children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })],
    });
    await screen.findByRole("region", { name: "Мои правила" });
    expect(document.body.textContent).toContain("Даша");
    expect(document.body.textContent).not.toContain("Миша");
    // Окно спросило только свои данные и ничего из команд родителя: копилку — номером 0 («моя»).
    expect(new Set(mock.calls.map((c) => c.command))).toEqual(new Set(["MyToday", "GetBonus"]));
    expect(mock.calls.filter((c) => c.command === "GetBonus").every((c) => !c.payload.childId)).toBe(true);
  });

  it("не просит у программы ничего, кроме MyToday и своей копилки, и ничего не отправляет", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    await screen.findByRole("region", { name: "Бонусы" });
    expect(new Set(mock.calls.map((c) => c.command))).toEqual(new Set(["MyToday", "GetBonus"]));
  });
});

describe("MyStats: периоды", () => {
  it("пять периодов, выбран «Сегодня», все доступны", async () => {
    setup();
    const tabs = within(await screen.findByRole("tablist", { name: "Период" })).getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Сегодня", "Неделя", "Месяц", "Год", "Всё время"]);
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    expect(tabs.filter((t) => t.getAttribute("aria-disabled"))).toEqual([]);
    expect(screen.queryByText(/появятся/)).toBeNull();
    expect(mock.calls.find((c) => c.command === "MyToday")?.payload).toEqual({ period: "today" });
  });

  // Ловит (S42, D-61): период не меняет «Итоги»; неделя тянет 1-е число, месяц - 2 января; подпись
  // «Использовано сегодня» и «Игры за сегодня» на неделе; «Сколько осталось» зависит от периода.
  it("S42: игры 10 / 40 / 60 / 60 / 60 мин, учёба за год 40 мин, всего за всё время 2 ч 30 мин", async () => {
    vi.setSystemTime(S42_NOW);
    const user = setup({ children: [s42Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const value = (label: string) =>
      within(within(box).getByRole("tablist", { name: "Итоги по категориям" })).getByRole("tab", { name: label }).querySelector(".totals-tab-value")?.textContent;
    const left = () => within(screen.getByRole("region", { name: "Сколько осталось" })).getByText("Осталось всего").closest("div")?.textContent;
    const leftToday = left();
    const want: [string, string, string, string, string, string][] = [
      // период, «Использовано …», игры, учёба, всего, прочее
      ["Сегодня", "Использовано сегодня10 мин", "10 мин", "0 мин", "10 мин", "0 мин"],
      ["Неделя", "Использовано за неделю40 мин", "40 мин", "0 мин", "40 мин", "0 мин"],
      ["Месяц", "Использовано за месяц1 ч 00 мин", "1 ч 00 мин", "0 мин", "1 ч 00 мин", "0 мин"],
      ["Год", "Использовано за год1 ч 00 мин", "1 ч 00 мин", "40 мин", "1 ч 40 мин", "0 мин"],
      ["Всё время", "Использовано за всё время1 ч 50 мин", "1 ч 00 мин", "40 мин", "2 ч 30 мин", "50 мин"],
    ];
    for (const [name, used, games, study, all, other] of want) {
      await user.click(screen.getByRole("tab", { name }));
      await vi.waitFor(() => expect(within(box).getByText(used.replace(/\d.*$/, "")).closest("div")?.textContent).toBe(used));
      expect(screen.getByRole("tab", { name }).getAttribute("aria-selected")).toBe("true");
      expect([value("Игры"), value("Учёба"), value("Всего"), value("Прочее")]).toEqual([games, study, all, other]);
      expect(left()).toBe(leftToday);
    }
    await user.click(within(box).getByRole("tab", { name: "Игры" }));
    expect(within(box).getByText("Игры за всё время: 1 ч 00 мин")).toBeTruthy();
    expect(within(within(box).getByRole("list", { name: "Игры: по программам" })).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "game1 ч 00 мин100%",
    ]);
    await user.click(within(box).getByRole("tab", { name: "Прочее" }));
    expect(within(within(box).getByRole("list", { name: "Прочее: по программам" })).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Остальное время50 мин100%",
    ]);
    expect(mock.calls.filter((c) => c.command === "MyToday").map((c) => c.payload.period)).toEqual(["today", "today", "week", "month", "year", "all"]);
  });

  // Ловит (8.2.6 п.3): доп. время и взятое из копилки за период не показаны.
  it("доп. время и «Взято из копилки» за период", async () => {
    vi.setSystemTime(S42_NOW);
    const user = setup({ children: [s42Child({ extraMin: 10 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const row = (label: string) => within(box).getByText(label).closest("div")?.textContent;
    expect([row("Доп. время"), row("Взято из копилки")]).toEqual(["Доп. время10 мин", "Взято из копилки0 мин"]);
    await user.click(screen.getByRole("tab", { name: "Всё время" }));
    await vi.waitFor(() => expect(row("Доп. время")).toBe("Доп. время40 мин"));
    expect(row("Взято из копилки")).toBe("Взято из копилки15 мин");
  });

  // Ловит (8.2.6 п.9): пустой период показывает нули вместо «За этот период пока нет данных».
  it("пустой период: «За этот период пока нет данных»", async () => {
    const user = setup({ children: [mockChild({ usedS: 0 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).queryByText("За этот период пока нет данных")).toBeNull();
    await user.click(screen.getByRole("tab", { name: "Месяц" }));
    expect(await within(box).findByText("За этот период пока нет данных")).toBeTruthy();
    expect(within(box).queryByRole("tablist", { name: "Итоги по категориям" })).toBeNull();
  });

  // Ловит: стрелки не ходят по периодам (та же разметка, что у вкладок «Итогов»), фокус теряется.
  it("стрелки, Home и End переключают период и фокус", async () => {
    const user = setup();
    const today = await screen.findByRole("tab", { name: "Сегодня" });
    today.focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Неделя" }));
    expect(screen.getByRole("tab", { name: "Неделя" }).getAttribute("aria-selected")).toBe("true");
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Всё время" }));
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Сегодня" }));
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Всё время" }));
    await user.keyboard("{Home}");
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    const tabs = within(screen.getByRole("tablist", { name: "Период" })).getAllByRole("tab");
    expect(tabs.map((t) => t.tabIndex)).toEqual([0, -1, -1, -1, -1]);
    expect(screen.getByRole("tabpanel", { name: "Сегодня" })).toBeTruthy();
  });

  // Ловит (ревью и UX-аудит FB-13): вкладки периодов стоят над «Сколько осталось» и «Нужно больше
  // времени?» (всегда за сегодня), а «Итоги», которые они меняют, - ниже сгиба; панель периода
  // оборачивает и блоки за сегодня; aria-controls вкладок указывает не на панель.
  it("вкладки периодов внутри «Итогов» под заголовком, панель периода - только «Итоги»", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Итоги" });
    const periods = screen.getByRole("tablist", { name: "Период" });
    const title = within(box).getByRole("heading", { name: "Итоги" });
    const categories = within(box).getByRole("tablist", { name: "Итоги по категориям" });
    const before = (a: Node, b: Node) => Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);
    expect(box.contains(periods)).toBe(true);
    expect(before(title, periods) && before(periods, categories)).toBe(true);
    expect(before(screen.getByRole("region", { name: "Нужно больше времени?" }), box)).toBe(true);
    const panel = screen.getByRole("tabpanel", { name: "Сегодня" });
    expect(box.contains(panel) && panel.contains(categories)).toBe(true);
    for (const name of ["Сколько осталось", "Нужно больше времени?", "Бонусы", "Мои правила"]) {
      expect(panel.contains(screen.getByRole("region", { name })), name).toBe(false);
    }
    for (const t of within(periods).getAllByRole("tab")) expect(t.getAttribute("aria-controls")).toBe(panel.id);
  });

  // Ловит (8.2.6, PL-36): разделы за период не в панели периода или не в порядке ТЗ; «Бонусы» и
  // «Мои правила» (за сегодня) попали в панель периода.
  it("разделы за период по порядку внутри панели периода: Итоги, По дням, Программы, Сайты, Нарушения, Копилка", async () => {
    vi.setSystemTime(G15_NOW);
    const user = setup({ children: [g15Child()], events: g15Events() });
    await user.click(await screen.findByRole("tab", { name: "Неделя" }));
    const panel = await screen.findByRole("tabpanel", { name: "Неделя" });
    await within(panel).findByRole("region", { name: "По дням" });
    const heads = within(panel)
      .getAllByRole("heading")
      .map((h) => h.textContent);
    expect(heads).toEqual(["По дням", "Программы", "Сайты", "Нарушения", "Копилка"]);
    const before = (a: Node, b: Node) => Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);
    const box = screen.getByRole("region", { name: "Итоги" });
    expect(box.contains(panel)).toBe(true);
    expect(before(within(box).getByRole("tablist", { name: "Итоги по категориям" }), within(panel).getByRole("region", { name: "По дням" }))).toBe(true);
    for (const name of ["Бонусы", "Мои правила", "Сколько осталось"]) {
      expect(panel.contains(screen.getByRole("region", { name })), name).toBe(false);
    }
  });

  // Ловит: на время загрузки периода окно мигает («Загружаю данные» вместо всей страницы) или нет признака
  // загрузки; поздний ответ прошлого периода перекрывает выбранный.
  it("загрузка периода: страница на месте, короткий признак, поздний ответ не перекрывает выбранный", async () => {
    vi.setSystemTime(S42_NOW);
    const user = setup({ children: [s42Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    await within(box).findByText("Использовано сегодня");
    let releaseWeek: () => void = () => {};
    const weekGate = new Promise<void>((r) => (releaseWeek = r));
    const real = mock.handler("MyToday");
    mock.override("MyToday", async (payload) => {
      if ((payload as { period?: string }).period === "week") await weekGate;
      return real(payload);
    });
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    expect(within(box).getByRole("status").textContent).toBe("Загружаю…");
    expect(box.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("region", { name: "Сколько осталось" })).toBeTruthy();
    expect(within(box).getByText("Использовано сегодня")).toBeTruthy();
    await user.click(screen.getByRole("tab", { name: "Месяц" }));
    await within(box).findByText("Использовано за месяц");
    releaseWeek();
    await new Promise((r) => setTimeout(r, 20));
    expect(within(box).getByText("Использовано за месяц")).toBeTruthy();
    expect(within(box).queryByText("Использовано за неделю")).toBeNull();
    expect(within(box).getByRole("status").textContent).toBe("");
    expect(box.getAttribute("aria-busy")).toBe("false");
  });

  // Ловит: «Обновить» и возврат в окно сбрасывают выбранный период на «Сегодня».
  it("«Обновить» перезапрашивает выбранный период", async () => {
    const user = setup();
    await user.click(await screen.findByRole("tab", { name: "Год" }));
    await screen.findByText("Использовано за год");
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    await vi.waitFor(() => expect(mock.calls.filter((c) => c.command === "MyToday").at(-1)?.payload).toEqual({ period: "year" }));
    expect(screen.getByText("Использовано за год")).toBeTruthy();
  });
});

describe("MyStats: сбой периода", () => {
  // Ловит (ревью окна M5): при сбое загрузки вкладка показывает новый период, а под ней цифры прошлого.
  it("сбой периода - вкладка и цифры одного периода", async () => {
    vi.setSystemTime(S42_NOW);
    const user = setup({ children: [s42Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    await within(box).findByText("Использовано сегодня");
    mock.override("MyToday", "reject", 1);
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    expect(await screen.findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: "Неделя" }).getAttribute("aria-selected")).toBe("false");
    expect(screen.getByRole("tabpanel", { name: "Сегодня" })).toBeTruthy();
    expect(within(box).getByText("Использовано сегодня")).toBeTruthy();
    expect(within(box).queryByText("Использовано за неделю")).toBeNull();
    // Повтор той же вкладки идёт уже без сбоя.
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    await within(box).findByText("Использовано за неделю");
    expect(screen.getByRole("tab", { name: "Неделя" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByText("Не удалось получить данные. Попробуй через минуту")).toBeNull();
  });

  // Ловит: после сбоя «Обновить» спрашивает период, которого на экране нет.
  it("после сбоя «Обновить» спрашивает показанный период", async () => {
    const user = setup();
    await user.click(await screen.findByRole("tab", { name: "Год" }));
    await screen.findByText("Использовано за год");
    mock.override("MyToday", "reject", 1);
    await user.click(screen.getByRole("tab", { name: "Месяц" }));
    await screen.findByText("Не удалось получить данные. Попробуй через минуту");
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    await vi.waitFor(() => expect(mock.calls.filter((c) => c.command === "MyToday").at(-1)?.payload).toEqual({ period: "year" }));
  });
});

describe("MyStats: Итоги", () => {
  // Ловит: рядом стояли «Осталось всего 10 мин», «Сегодня осталось 10 мин из 2 ч» и «Всего 1 ч 50 мин»
  // без пояснения. Остаток называется один раз (сверху), в «Итогах» только «Использовано сегодня».
  it("в «Итогах» только «Использовано сегодня», остаток не повторяется", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Итоги" });
    const total = within(box).getByText("Использовано сегодня").closest("div");
    expect(total?.textContent).toContain("35 мин");
    // «Всего» - только вкладка (D-58) со своей подписью «Всего за сегодня», не безымянная строка.
    expect(within(box).getAllByText("Всего").every((el) => el.closest('[role="tab"]') !== null)).toBe(true);
    expect(within(box).queryByText(/осталось/i)).toBeNull();
    expect(within(box).queryByText("Использовано 35 мин")).toBeNull();
    const left = within(screen.getByRole("region", { name: "Сколько осталось" })).getByText("Осталось всего").closest("div");
    expect(left?.textContent).toContain("1 ч 25 мин");
  });

  // Ловит: итоги игр и учёбы не из ответа службы (S13: «в статистике Учёба 40 мин»).
  it("итоги: игры, видео и учёба за сегодня от службы, «Пока не считается» больше нет", async () => {
    setup({ children: [mockChild({ usedS: 2100, gamesUsedS: 1200, videoUsedS: 300, studyUsedS: 2400 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tabs = within(within(box).getByRole("tablist", { name: "Итоги по категориям" }));
    const value = (label: string) => tabs.getByRole("tab", { name: label }).querySelector(".totals-tab-value")?.textContent;
    expect(value("Всего")).toBe("1 ч 15 мин");
    expect(value("Игры")).toBe("20 мин");
    expect(value("Видео и соцсети")).toBe("5 мин");
    expect(value("Учёба")).toBe("40 мин");
    expect(value("Прочее")).toBe("10 мин");
    expect(screen.queryByText("Пока не считается")).toBeNull();
    expect(within(box).queryByText("Появится позже")).toBeNull();
  });

  it("время вышло: видно, когда можно снова", async () => {
    setup({ children: [mockChild({ usedS: 7200 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("Можно снова: завтра")).toBeTruthy();
  });

  // Ловит: статус на экране ребёнка расходится с остатком в значке и у родителя (Review Focus 5).
  it("остаток тот же, что у программы для значка и родителя", async () => {
    setup({ children: [mockChild({ usedS: 3600 })] });
    const left = (await screen.findByText("Осталось всего")).closest("div");
    expect(left?.textContent).toContain("1 ч 00 мин");
  });

  // Ловит: пустой период показывает пустоту или ноль вместо объяснения (8.2.6 п.9).
  it("нет данных: «За этот период пока нет данных»", async () => {
    setup({ myChildId: null });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("За этот период пока нет данных")).toBeTruthy();
    expect(within(box).queryByText("Использовано сегодня")).toBeNull();
  });
});

// Вкладки «Итогов» (D-58, S39): сумма категории и каждая программа «название, время, доля».
const S39_PROGRAMS: ProgramTime[] = [
  { path: "c:\\games\\game.exe", name: "game", category: "games", seconds: 600 },
  { path: "c:\\games\\game2.exe", name: "game2", category: "games", seconds: 300 },
  { path: "c:\\school\\study.exe", name: "study", category: "study", seconds: 180 },
  { path: "", name: "Остальное время", category: "other", seconds: 300 },
];

function s39Child() {
  return mockChild({ usedS: 1200, gamesUsedS: 900, studyUsedS: 180, programsToday: S39_PROGRAMS });
}

/** Строки списка выбранной вкладки: [название, время, доля]. */
function listRows(box: HTMLElement): string[][] {
  const list = within(box).queryByRole("list", { name: /: по / });
  if (!list) return [];
  return within(list)
    .getAllByRole("listitem")
    .map((li) => [".totals-name", ".totals-time", ".totals-share"].map((c) => li.querySelector(c)?.textContent ?? ""));
}

/** Панель вкладки категорий «Итогов» (внутри панели периода - своя). */
function categoryPanel(box: HTMLElement): HTMLElement {
  const panels = within(box)
    .getAllByRole("tabpanel")
    .filter((p) => p.getAttribute("aria-labelledby")?.startsWith("totals-tab-"));
  expect(panels).toHaveLength(1);
  return panels[0];
}

// T9-6, PL-28: вкладка «Видео и соцсети» со строками сайтов, «Всего» включает сайты.
describe("MyStats: «Итоги» по сайтам", () => {
  const SITES: ProgramTime[] = [
    { path: "youtube.com", name: "youtube.com", category: "video_social", seconds: 600, kind: "site" },
    { path: "vk.com", name: "vk.com", category: "video_social", seconds: 300, kind: "site" },
  ];
  const videoChild = () => mockChild({ usedS: 2100, gamesUsedS: 600, videoUsedS: 900, studyUsedS: 300, programsToday: SITES });

  it("плитки и вкладка «Видео и соцсети»: сайты по убыванию, доля от суммы видео", async () => {
    const user = setup({ children: [videoChild()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tabs = within(within(box).getByRole("tablist", { name: "Итоги по категориям" }));
    const value = (label: string) => tabs.getByRole("tab", { name: label }).querySelector(".totals-tab-value")?.textContent;
    expect(["Всего", "Игры", "Видео и соцсети", "Учёба", "Прочее"].map(value)).toEqual(["40 мин", "10 мин", "15 мин", "5 мин", "10 мин"]);
    await user.click(tabs.getByRole("tab", { name: "Видео и соцсети" }));
    const panel = categoryPanel(box);
    expect(panel.getAttribute("aria-labelledby")).toBe("totals-tab-video_social");
    expect(within(panel).getByText("Видео и соцсети за сегодня: 15 мин")).toBeTruthy();
    expect(within(panel).getByRole("list", { name: "Видео и соцсети: по сайтам" })).toBeTruthy();
    expect(listRows(panel)).toEqual([
      ["youtube.com", "10 мин", "67%"],
      ["vk.com", "5 мин", "33%"],
    ]);
  });

  // Ловит (PL-28): строки «Всего» без сайтов не сходятся с плиткой «Всего».
  it("«Всего» содержит сайты; список назван «по программам и сайтам»", async () => {
    setup({ children: [videoChild()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByRole("list", { name: "Всего: по программам и сайтам" })).toBeTruthy();
    const names = listRows(box).map((r) => r[0]);
    expect(names).toContain("youtube.com");
    expect(names).toContain("vk.com");
    expect(names.at(-1)).toBe("Остальное время");
  });

  it("видео без строк: «Пока ничего не запускалось»", async () => {
    const user = setup({ children: [mockChild({ usedS: 600, videoUsedS: 0 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    await user.click(within(box).getByRole("tab", { name: "Видео и соцсети" }));
    expect(within(categoryPanel(box)).getByText("Пока ничего не запускалось")).toBeTruthy();
  });

  // PL-31: «Неделя» = прошлые дни недели из базы + сегодня.
  it("«Неделя»: видео прошлых дней и сегодня", async () => {
    const history = [
      // Вторник этой недели: youtube.com 10 мин.
      { day: "2026-10-13", totalS: 1200, videoS: 600, programs: [{ path: "youtube.com", name: "youtube.com", category: "video_social" as const, seconds: 600, kind: "site" as const }] },
      // Прошлая неделя: в «Неделю» не входит.
      { day: "2026-10-05", totalS: 900, videoS: 900 },
    ];
    vi.setSystemTime(new Date(2026, 9, 14, 12, 0, 0)); // ср, 14 октября 2026
    const user = setup({ children: [mockChild({ usedS: 2100, videoUsedS: 900, programsToday: SITES, history })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    await within(box).findByText("Использовано за неделю");
    await user.click(within(box).getByRole("tab", { name: "Видео и соцсети" }));
    expect(within(box).getByText("Видео и соцсети за неделю: 25 мин")).toBeTruthy();
    expect(listRows(categoryPanel(box))[0]).toEqual(["youtube.com", "20 мин", "80%"]);
  });
});

describe("MyStats: вкладки «Итогов»", () => {
  // Ловит (S39): нажатие «Игры» не показывает сумму и игры по отдельности, доли не 67% и 33%.
  it("«Игры»: сумма 15 мин, game 10 мин 67%, game2 5 мин 33%", async () => {
    const user = setup({ children: [s39Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tabs = within(within(box).getByRole("tablist", { name: "Итоги по категориям" }));
    expect(tabs.getAllByRole("tab").map((t) => t.querySelector(".totals-tab-label")?.textContent)).toEqual(["Всего", "Игры", "Видео и соцсети", "Учёба", "Прочее"]);
    await user.click(tabs.getByRole("tab", { name: "Игры" }));
    expect(tabs.getByRole("tab", { name: "Игры" }).getAttribute("aria-selected")).toBe("true");
    expect(tabs.getByRole("tab", { name: "Всего" }).getAttribute("aria-selected")).toBe("false");
    const panel = categoryPanel(box);
    expect(panel.getAttribute("aria-labelledby")).toBe("totals-tab-games");
    expect(tabs.getByRole("tab", { name: "Игры" }).getAttribute("aria-controls")).toBe(panel.id);
    expect(within(panel).getByText("Игры за сегодня: 15 мин")).toBeTruthy();
    expect(listRows(panel)).toEqual([
      ["game", "10 мин", "67%"],
      ["game2", "5 мин", "33%"],
    ]);
  });

  // Ловит (ревью 0.3.1): в панели вкладки нет элементов фокуса, и без tabIndex Tab от вкладки
  // перескакивает через неё: список программ с клавиатуры не прочитать (шаблон вкладок WAI-ARIA).
  it("Tab от выбранной вкладки ведёт в панель", async () => {
    const user = setup({ children: [s39Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tab = within(box).getByRole("tab", { name: "Игры" });
    await user.click(tab);
    await user.tab();
    const panel = categoryPanel(box);
    expect(panel.getAttribute("tabindex")).toBe("0");
    expect(document.activeElement).toBe(panel);
  });

  it("«Учёба»: study 3 мин; «Всего» по умолчанию со всеми программами", async () => {
    const user = setup({ children: [s39Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("Всего за сегодня: 23 мин")).toBeTruthy();
    expect(listRows(box).map((r) => r[0])).toEqual(["game", "game2", "study", "Остальное время"]);
    await user.click(within(box).getByRole("tab", { name: "Учёба" }));
    expect(listRows(box)).toEqual([["study", "3 мин", "100%"]]);
  });

  // Ловит FB-10 (0.3.1): «Всего» больше «Использовано сегодня» без объяснения (учёба вне лимита, D-7):
  // пояснение было только на вкладке «Всего», а плитка «Всего 23 мин» видна всегда.
  it("«Всего» с учёбой: пояснение с числами у вкладок при любой вкладке, связано с «Всего»", async () => {
    const user = setup({ children: [s39Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const text = "Всего = 20 мин в счёт лимита + 3 мин учёбы вне лимита";
    for (const name of ["Всего", "Игры", "Видео и соцсети", "Учёба", "Прочее"]) {
      await user.click(within(box).getByRole("tab", { name }));
      const note = within(box).getByText(text);
      // Под рядом вкладок, не внутри панели: виден при любой выбранной вкладке и не дублируется.
      expect(note.closest("#totals-panel")).toBeNull();
      expect(within(box).getAllByText(text)).toHaveLength(1);
    }
    const note = within(box).getByText(text);
    const all = within(box).getByRole("tab", { name: "Всего" });
    expect((all.getAttribute("aria-describedby") ?? "").split(" ")).toContain(note.id);
    expect(within(box).getByRole("tab", { name: "Игры" }).getAttribute("aria-describedby") ?? "").not.toContain(note.id);
  });

  it("«Всего» без учёбы: пояснения нет", async () => {
    setup({ children: [mockChild({ usedS: 600, studyUsedS: 0 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("Всего за сегодня: 10 мин")).toBeTruthy();
    expect(within(box).queryByText(/учёбы вне лимита/)).toBeNull();
    expect(within(box).getByRole("tab", { name: "Всего" }).getAttribute("aria-describedby")).toBe("totals-value-all");
  });

  // Ловит: пустая категория показывает пустой список или ноль без объяснения (8.2.6 п.3).
  it("пустая категория: «Пока ничего не запускалось»", async () => {
    const user = setup({ children: [mockChild({ usedS: 0, gamesUsedS: 0, studyUsedS: 0 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    await user.click(within(box).getByRole("tab", { name: "Игры" }));
    expect(within(box).getByText("Пока ничего не запускалось")).toBeTruthy();
    expect(within(box).queryByRole("list", { name: /: по / })).toBeNull();
  });

  // Ловит: вкладки не переключаются стрелками, фокус не идёт за выбором (доступность вкладок).
  it("стрелки, Home и End переключают вкладки и переносят фокус", async () => {
    const user = setup({ children: [s39Child()] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tab = (name: string) => within(box).getByRole("tab", { name });
    expect(tab("Всего").tabIndex).toBe(0);
    expect(tab("Игры").tabIndex).toBe(-1);
    tab("Всего").focus();
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(tab("Игры"));
    expect(tab("Игры").getAttribute("aria-selected")).toBe("true");
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(document.activeElement).toBe(tab("Прочее"));
    await user.keyboard("{Home}");
    expect(tab("Всего").getAttribute("aria-selected")).toBe("true");
    await user.keyboard("{End}");
    expect(tab("Прочее").getAttribute("aria-selected")).toBe("true");
    expect(listRows(box)).toEqual([["Остальное время", "5 мин", "100%"]]);
  });
});

describe("MyStats: Мои правила", () => {
  it("лимиты по дням недели текстом, сегодняшний день отмечен", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    const rows = within(box).getAllByRole("row").slice(1);
    expect(rows.map((r) => r.textContent)).toEqual([
      "Понедельник2 ч (сегодня)",
      "Вторник2 ч",
      "Среда1 ч 30 мин",
      "Четверг2 ч",
      "Пятница2 ч",
      "Суббота4 ч",
      "ВоскресеньеБез лимита",
    ]);
    expect(rows[0].getAttribute("aria-current")).toBe("date");
    expect(rows.filter((r) => r.getAttribute("aria-current") === "date")).toHaveLength(1);
  });

  // R21: лимит игр и режим окончания - строками службы под таблицей дней.
  it("лимит игр и режим окончания текстом от службы", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    const general = within(box).getByRole("list", { name: "Общие правила" });
    expect(within(general).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Игры: пн до 1 ч, ср нельзя, сб до 2 ч",
      "Видео и соцсети: пн до 1 ч, ср нельзя",
      "Когда время вышло: экран блокировки",
    ]);
  });

  it("одинаковый лимит видео все 7 дней - «до 30 мин в день»", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false, videoMin: 30, videoUnlimited: false }));
    setup({ children: [mockChild({ limits })] });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    const general = within(box).getByRole("list", { name: "Общие правила" });
    expect(within(general).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Видео и соцсети: до 30 мин в день",
      "Когда время вышло: экран блокировки",
    ]);
  });

  it("режим «Оставить только учёбу» и игры без лимита", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));
    setup({ children: [mockChild({ limits, endMode: "study_only" })] });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    const general = within(box).getByRole("list", { name: "Общие правила" });
    expect(within(general).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Когда время вышло: остаётся только учёба"]);
    expect(within(box).queryByText(/экран закроется блокировкой/)).toBeNull();
  });

  // Ловит: «Мои правила» обещают запретные часы, которых у ребёнка нет (они живут в «Сегодня», D-53).
  it("в таблице правил нет запретных часов", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(box.textContent).not.toMatch(/запретн|с \d\d:\d\d до \d\d:\d\d/i);
  });

  it("ноль минут - «Нельзя»", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: false }));
    setup({ children: [mockChild({ limits })] });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(within(box).getAllByText("Нельзя")).toHaveLength(7);
  });

  // Ловит: правила не прочитаны (R18), а окно показывает пустую таблицу без слов.
  it("правил нет: говорит об этом и куда идти", async () => {
    setup();
    const base = (await window.go!.ui!.App!.Call("MyToday", "{}")) as string;
    const reply = JSON.parse(base);
    reply.payload.rules = [];
    mock.override("MyToday", reply);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Обновить" }));
    const box = await screen.findByRole("region", { name: "Мои правила" });
    await within(box).findByText("Правила сейчас недоступны. Спроси у родителя.");
    expect(within(box).queryByRole("table")).toBeNull();
  });

  it("сеанс не ребёнка: правил нет, окно не падает", async () => {
    setup({ myChildId: null });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(within(box).getByText("Эта учётная запись не детская: здесь нет правил.")).toBeTruthy();
  });
});

describe("MyStats: обновление и сбои", () => {
  // Ловит: сбой связи с программой показывает технический текст или молчит (L-112, L-114).
  it("нет связи: простыми словами и «Обновить», кнопка повторяет запрос", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Итоги" });
    mock.override("MyToday", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    expect(await screen.findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/ipc|connection|pipe|undefined/i);
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    expect(await screen.findByRole("region", { name: "Итоги" })).toBeTruthy();
    expect(screen.queryByText("Не удалось получить данные. Попробуй через минуту")).toBeNull();
  });

  it("первая загрузка без связи: сообщение и «Обновить»", async () => {
    mock = installBackend({ mode: "mystats", linkDown: true });
    render(<MyStats />);
    expect(await screen.findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Обновить" })).toBeTruthy();
  });

  // Ловит: окно лежит открытым и показывает остаток часовой давности.
  it("вернулись в окно: цифры перечитываются", async () => {
    setup();
    await screen.findByRole("region", { name: "Итоги" });
    const before = mock.count("MyToday");
    window.dispatchEvent(new Event("focus"));
    await vi.waitFor(() => expect(mock.count("MyToday")).toBe(before + 1));
  });

  it("пока данные грузятся, говорит об этом", () => {
    setup();
    expect(screen.getByText("Загружаю данные…")).toBeTruthy();
  });
});

describe("MyStats: тексты для ребёнка", () => {
  // Ловит: длинные тире (NFR-11) и обращение на «вы» в окне ребёнка (A4).
  it("без длинных тире и без «вы»", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(YOU_FORM);
  });
});

describe("MyStats: «Бонусы» (R25, R26, D-49, D-50)", () => {
  const AUTO = "Бонус за свободное время: 15 мин";
  const withBonus = () =>
    setup({
      children: [
        mockChild({
          bonusBalance: 45,
          bonusLedger: [
            { atMs: NOW.getTime() - 12 * 3600_000, tzOffsetMin: 180, deltaMin: 15, reason: AUTO, balanceAfter: 45 },
            { atMs: NOW.getTime() - 36 * 3600_000, tzOffsetMin: 180, deltaMin: 30, reason: "за уборку", balanceAfter: 30 },
          ],
        }),
      ],
      promos: [{ codeDisplay: "A1234", minutes: 30, createdAtMs: NOW.getTime() - 3600_000 }],
    });

  // Ловит: ребёнок не видит, за что начислен автобонус (D-49); история копилки в двух местах окна.
  it("баланс копилки и автобонус простыми словами", async () => {
    const user = withBonus();
    const box = await screen.findByRole("region", { name: "Бонусы" });
    expect(await within(box).findByText("Копилка: 45 мин")).toBeTruthy();
    expect(within(box).queryByRole("list")).toBeNull();
    expect(box.textContent ?? "").not.toMatch(YOU_FORM);
    await user.click(screen.getByRole("tab", { name: "Всё время" }));
    await screen.findByText("Использовано за всё время");
    const piggy = screen.getByRole("region", { name: "Копилка" });
    expect(within(piggy).getByText("Сейчас в копилке: 45 мин")).toBeTruthy();
    expect(within(piggy).getByText("+15 мин за неизрасходованное время прошлого дня")).toBeTruthy();
    expect(within(piggy).getByText("+30 мин: за уборку")).toBeTruthy();
    expect(piggy.textContent ?? "").not.toMatch(YOU_FORM);
  });

  // Ловит: верный код не добавляет время или сообщение не по ТЗ (FR-35: «Добавлено N минут»).
  it("верный промокод: «Добавлено 30 минут»", async () => {
    const user = withBonus();
    const field = await screen.findByRole("textbox", { name: "Промокод" });
    await user.type(field, " a1234 ");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(await screen.findByText("Добавлено 30 минут")).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(mock.calls.find((c) => c.command === "RedeemPromo")?.payload).toEqual({ childId: 0, code: " a1234 " });
    expect((field as HTMLInputElement).value).toBe("");
  });

  // Ловит: повторный ввод того же кода снова добавляет время (одноразовость, R26).
  it("неверный или использованный код: «Код не подходит», время не меняется", async () => {
    const user = withBonus();
    const field = await screen.findByRole("textbox", { name: "Промокод" });
    await user.type(field, "a1234");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    await screen.findByText("Добавлено 30 минут");
    await user.type(field, "a1234");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(await screen.findByText("Код не подходит")).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(30);
  });

  it("пустой код: подсказка у поля, в программу не уходит", async () => {
    const user = withBonus();
    await screen.findByRole("textbox", { name: "Промокод" });
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(screen.getByText("Введи код")).toBeTruthy();
    expect(mock.count("RedeemPromo")).toBe(0);
  });
});

async function myTodayStatus() {
  return JSON.parse((await window.go!.ui!.App!.Call("MyToday", "{}")) as string).payload.status as { tooltip: string };
}

// D-53, ТЗ 8.2.6 п.1: «Сегодня» с остатками, лимитами, запретными часами, копилкой, кнопками и ссылкой внизу.
describe("MyStats: «Сегодня» (D-53)", () => {
  const MONDAY_NIGHT = [{ weekday: 1, startMin: 21 * 60, endMin: 7 * 60 }];

  // Review Focus 5: остаток игр в окне = подсказка значка «Игры: 20 мин» (один ответ службы).
  it("остаток игр тот же, что в подсказке значка", async () => {
    setup({ children: [mockChild({ gamesUsedS: 2400 })] });
    const box = await screen.findByRole("region", { name: "Сколько осталось" });
    const games = within(box).getByText("Игры").closest("div")?.querySelector("dd")?.textContent;
    expect(games).toBe("Осталось 20 мин из 1 ч");
    const tooltip = (await myTodayStatus()).tooltip;
    expect(tooltip.split("\n")[1]).toBe("Игры: 20 мин");
  });

  // Review Focus плана п. 9: остаток видео в окне = подсказка значка «Видео: 1 ч 00 мин».
  it("остаток видео тот же, что в подсказке значка", async () => {
    setup({ children: [mockChild({ videoUsedS: 1200 })] });
    const box = await screen.findByRole("region", { name: "Сколько осталось" });
    const video = within(box).getByText("Видео и соцсети").closest("div")?.querySelector("dd")?.textContent;
    expect(video).toBe("Осталось 40 мин из 1 ч");
    const tooltip = (await myTodayStatus()).tooltip;
    expect(tooltip.split("\n")[2]).toBe("Видео: 40 мин");
  });

  it("подсказка значка по умолчанию: видео 1 ч 00 мин третьей строкой", async () => {
    setup();
    await screen.findByRole("region", { name: "Сколько осталось" });
    expect((await myTodayStatus()).tooltip.split("\n")[2]).toBe("Видео: 1 ч 00 мин");
  });

  // Ловит: на «Сегодня» нет остатка общего времени, игр, «Видео и соцсетей», лимита, запретных часов или копилки.
  it("остатки, лимит дня, запретные часы и копилка", async () => {
    setup({ children: [mockChild({ bonusBalance: 45, schedule: MONDAY_NIGHT })] });
    const box = await screen.findByRole("region", { name: "Сколько осталось" });
    const rows = Object.fromEntries(
      Array.from(box.querySelectorAll("dt")).map((dt) => [dt.textContent, dt.nextElementSibling?.textContent]),
    );
    expect(rows).toEqual({
      "Осталось всего": "1 ч 25 мин",
      "Игры": "Осталось 1 ч 00 мин из 1 ч",
      "Видео и соцсети": "Осталось 1 ч 00 мин из 1 ч",
      "Лимит дня": "2 ч",
      "Запретные часы": "Сегодня нельзя с 21:00 до 07:00",
      "Копилка": "45 мин",
    });
  });

  // Ловит: раздел остатка идёт после итогов и правил, и ребёнок ищет главное внизу.
  it("раздел остатка стоит первым, перед итогами", async () => {
    setup();
    const left = await screen.findByRole("region", { name: "Сколько осталось" });
    const totals = screen.getByRole("region", { name: "Итоги" });
    expect(left.compareDocumentPosition(totals) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("кнопки «Попросить ещё» и «Взять из копилки», а «Настройки для родителя» внизу мелко", async () => {
    setup({ children: [mockChild({ bonusBalance: 45 })] });
    const ask = await screen.findByRole("button", { name: "Попросить ещё" });
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    const parent = screen.getByRole("button", { name: "Настройки для родителя" });
    const rules = screen.getByRole("region", { name: "Мои правила" });
    expect(ask.compareDocumentPosition(spend) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(rules.compareDocumentPosition(parent) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(parent.className).toContain("link");
  });

  it("сеанс не ребёнка: кнопок времени нет", async () => {
    setup({ myChildId: null });
    await screen.findByRole("region", { name: "Мои правила" });
    expect(screen.queryByRole("button", { name: "Попросить ещё" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Взять из копилки" })).toBeNull();
  });

  // Ловит: ссылка внизу ничего не открывает или открывает не окно родителя.
  it("«Настройки для родителя» просит у службы окно родителя", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Настройки для родителя" }));
    expect(mock.calls.find((c) => c.command === "OpenWindow")?.payload).toEqual({ kind: "parent" });
    expect(mock.state.openedWindows).toEqual(["parent"]);
  });

  // Ловит: окно не открылось, а ребёнок не получил ни слова (L-114).
  it("не открылось: простыми словами", async () => {
    const user = setup();
    const link = await screen.findByRole("button", { name: "Настройки для родителя" });
    mock.override("OpenWindow", "reject", 1);
    await user.click(link);
    expect(await screen.findByText("Не получилось открыть окно. Попробуй ещё раз")).toBeTruthy();
  });
});

describe("MyStats: «Попросить ещё» (FR-10)", () => {
  async function openAsk() {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Попросить ещё" }));
    const dialog = await screen.findByRole("dialog", { name: "Позови родителя" });
    return { user, dialog };
  }

  // Ловит: панель без пароля родителя (ребёнок сам себе добавил бы время) или без быстрых кнопок.
  it("панель «Позови родителя»: пароль, +15 / +30 / +60 и своё число", async () => {
    const { dialog } = await openAsk();
    expect(within(dialog).getByLabelText("Пароль родителя")).toBeTruthy();
    expect((within(dialog).getByLabelText("Пароль родителя") as HTMLInputElement).type).toBe("password");
    for (const name of ["+15 мин", "+30 мин", "+60 мин"]) expect(within(dialog).getByRole("button", { name })).toBeTruthy();
    expect(within(dialog).getByLabelText("Своё число минут")).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Отмена" })).toBeTruthy();
  });

  // Ловит: служба не получает номер ребёнка (без него AskMore отвечает «Ребёнок не найден»), пароль или минуты.
  it("верный пароль и +30: время добавлено, «Добавлено 30 минут»", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.click(within(dialog).getByRole("button", { name: "+30 мин" }));
    expect(await screen.findByText("Добавлено 30 минут")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AskMore")?.payload).toEqual({
      childId: 1,
      minutes: 30,
      password: DEFAULT_PASSWORD,
    });
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(screen.queryByRole("dialog")).toBeNull();
    // Цифры окна перечитаны: лимит дня теперь 2 ч 30 мин.
    expect(await screen.findByText("2 ч 30 мин")).toBeTruthy();
  });

  // Ловит: неверный пароль закрывает панель или показывает ошибку не у поля (L-121).
  it("неверный пароль: ошибка у поля, панель открыта, время не добавлено", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), "не тот");
    await user.click(within(dialog).getByRole("button", { name: "+15 мин" }));
    expect(await within(dialog).findByText("Неверный пароль. Осталось попыток: 4")).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Позови родителя" })).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(0);
  });

  it("пустой пароль: подсказка у поля, в программу не уходит", async () => {
    const { user, dialog } = await openAsk();
    await user.click(within(dialog).getByRole("button", { name: "+60 мин" }));
    expect(within(dialog).getByText("Введи пароль родителя")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
  });

  // Ловит: своё число принимает 0 или больше 240 (граница службы).
  it("своё число: 45 минут проходит, 300 отклонено до отправки", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.type(within(dialog).getByLabelText("Своё число минут"), "300");
    await user.click(within(dialog).getByRole("button", { name: "Добавить" }));
    expect(within(dialog).getByText("Можно от 1 до 240 минут")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
    await user.clear(within(dialog).getByLabelText("Своё число минут"));
    await user.type(within(dialog).getByLabelText("Своё число минут"), "45");
    await user.click(within(dialog).getByRole("button", { name: "Добавить" }));
    expect(await screen.findByText("Добавлено 45 минут")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AskMore")?.payload).toMatchObject({ minutes: 45 });
  });

  it("«Отмена» и Esc закрывают панель, ничего не отправляя", async () => {
    const { user, dialog } = await openAsk();
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Попросить ещё" }));
    await screen.findByRole("dialog", { name: "Позови родителя" });
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("AskMore")).toBe(0);
  });

  // Ловит: не узнали номер ребёнка (копилка не прочиталась) и отправили пустой запрос.
  it("не знаем, чей сеанс: говорит об этом, а не шлёт пароль впустую", async () => {
    const { user, dialog } = await openAsk();
    mock.override("GetBonus", "reject");
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    await user.click(await screen.findByRole("button", { name: "Попросить ещё" }));
    const again = await screen.findByRole("dialog", { name: "Позови родителя" });
    await user.type(within(again).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.click(within(again).getByRole("button", { name: "+30 мин" }));
    expect(await within(again).findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
  });

  it("тексты панели ребёнку на «ты», без длинных тире", async () => {
    const { dialog } = await openAsk();
    expect(dialog.textContent).not.toMatch(/[—–]/);
    expect(dialog.textContent).not.toMatch(YOU_FORM);
  });
});

describe("MyStats: «Взять из копилки» (FR-11)", () => {
  async function openSpend(over: Parameters<typeof mockChild>[0] = {}) {
    const user = setup({ children: [mockChild({ bonusBalance: 45, ...over })] });
    await user.click(await screen.findByRole("button", { name: "Взять из копилки" }));
    const dialog = await screen.findByRole("dialog", { name: "Взять из копилки" });
    return { user, dialog };
  }

  it("варианты 15, 30 и весь баланс", async () => {
    const { dialog } = await openSpend();
    expect(within(dialog).getAllByRole("button").map((b) => b.textContent)).toEqual(["15 мин", "30 мин", "Всё: 45 мин", "Отмена"]);
  });

  // Ловит: списание без подтверждения или с неверным остатком копилки в тексте.
  it("30 минут: подтверждение с остатком, «Взять» списывает без пароля", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "30 мин" }));
    expect(await screen.findByText("Взять из копилки 30 мин? Останется 15 мин")).toBeTruthy();
    expect(mock.count("SpendBonus")).toBe(0);
    await user.click(screen.getByRole("button", { name: "Взять" }));
    expect(await screen.findByText("Взято из копилки 30 мин")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "SpendBonus")?.payload).toEqual({ childId: 0, minutes: 30 });
    expect(mock.state.children[0].bonusBalance).toBe(15);
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(screen.queryByRole("dialog")).toBeNull();
    // Копилка в окне перечитана.
    const left = await screen.findByRole("region", { name: "Сколько осталось" });
    expect(within(left).getByText("Копилка").nextElementSibling?.textContent).toBe("15 мин");
  });

  it("«Отмена» на подтверждении: ничего не списано", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "Всё: 45 мин" }));
    await screen.findByText("Взять из копилки 45 мин? Останется 0 мин");
    await user.click(screen.getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("SpendBonus")).toBe(0);
    expect(mock.state.children[0].bonusBalance).toBe(45);
  });

  // Ловит: отказ службы теряется или показан техническим текстом.
  it("отказ службы показан в панели", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "15 мин" }));
    mock.state.children[0].bonusBalance = 5; // копилку уже потратили в другом месте
    await user.click(await screen.findByRole("button", { name: "Взять" }));
    expect(await screen.findByText("В копилке 5 мин, столько взять нельзя")).toBeTruthy();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  // Ловит: в запретный интервал кнопка рабочая или недоступна молча (урок L-17).
  it("запретные часы: кнопка недоступна, причина и час названы", async () => {
    setup({ children: [mockChild({ bonusBalance: 45, forbiddenUntil: "Можно снова: сегодня в 07:00" })] });
    const user = userEvent.setup();
    const btn = await screen.findByRole("button", { name: "Взять из копилки" });
    expect(btn.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Из копилки можно взять с 07:00")).toBeTruthy();
    expect(btn.getAttribute("aria-describedby")).toBeTruthy();
    await user.click(btn);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // Ловит: причина недоступности дана мелкой серой строкой (field-hint), а строка «Запретные часы»
  // в тот же момент говорит «запретных часов нет» (кадр s06-spend-forbidden).
  it("запретные часы: причина заметна рядом с кнопкой, строка «Запретные часы» не спорит с ней", async () => {
    setup({ children: [mockChild({ bonusBalance: 45, forbiddenUntil: "Можно снова: сегодня в 07:00" })] });
    const note = await screen.findByText("Из копилки можно взять с 07:00");
    expect(note.className).toContain("spend-note");
    expect(note.className).not.toContain("field-hint");
    expect(note.getAttribute("role")).toBe("note");
    const row = within(screen.getByRole("region", { name: "Сколько осталось" })).getByText("Запретные часы").closest("div");
    expect(row?.textContent).not.toContain("запретных часов нет");
    expect(row?.textContent).toContain("07:00");
  });

  it("копилка пуста: кнопка недоступна, причина названа", async () => {
    setup();
    const btn = await screen.findByRole("button", { name: "Взять из копилки" });
    expect(btn.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("В копилке пока пусто")).toBeTruthy();
  });

  it("тексты панели ребёнку на «ты», без длинных тире", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "30 мин" }));
    await screen.findByText("Взять из копилки 30 мин? Останется 15 мин");
    const text = screen.getByRole("dialog").textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(YOU_FORM);
  });
});

// Разделы за период (8.2.6 п.4-10, M5) на эталонной неделе G15 (S21, S22): то же, что у родителя.
describe("MyStats: разделы за период (G15)", () => {
  async function weekG15(opts: Parameters<typeof installBackend>[0] = {}) {
    vi.setSystemTime(G15_NOW);
    const user = setup({ children: [g15Child()], events: g15Events(), ...opts });
    await user.click(await screen.findByRole("tab", { name: "Неделя" }));
    await screen.findByText("Использовано за неделю");
    return user;
  }
  const rowsOf = (table: HTMLElement) =>
    within(table)
      .getAllByRole("row")
      .slice(1)
      .map((tr) => Array.from(tr.children).map((c) => c.textContent));

  // Ловит (8.2.6 п.10, FR-38): подписи нет или она внутри карточки, а не над «Итогами».
  it("«Статистика ведётся с 16 ноября 2026» над карточкой «Итоги»", async () => {
    await weekG15();
    const since = screen.getByText("Статистика ведётся с 16 ноября 2026");
    const box = screen.getByRole("region", { name: "Итоги" });
    expect(box.contains(since)).toBe(false);
    expect(Boolean(since.compareDocumentPosition(box) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true);
  });

  // Ловит (S21): столбиков не 7 или подсказки не по эталону.
  it("«По дням»: 7 столбиков, итог графика как в «Итогах»", async () => {
    await weekG15();
    const days = screen.getByRole("region", { name: "По дням" });
    const bars = within(within(days).getByRole("group")).getAllByRole("img");
    expect(bars).toHaveLength(7);
    expect(bars[1].getAttribute("aria-label")).toBe("вт, 17 ноября: 2 ч 20 мин, учёба 0 мин");
    expect(days.querySelector("svg")?.getAttribute("aria-label")).toBe("По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
  });

  // Ловит (8.2.6 п.5, RM5-4): таблицы не по эталону, сайт в «Программах».
  it("«Программы» и «Сайты»: название, время, доля", async () => {
    await weekG15();
    const programs = within(screen.getByRole("region", { name: "Программы" })).getByRole("table", { name: "Программы" });
    expect(rowsOf(programs)).toEqual([
      ["game.exe", "4 ч 45 мин", "45%"],
      ["other.exe", "4 ч 08 мин", "39%"],
      ["study.exe", "1 ч 35 мин", "15%"],
    ]);
    const sites = within(screen.getByRole("region", { name: "Сайты" })).getByRole("table", { name: "Сайты" });
    expect(rowsOf(sites)).toEqual([["youtube.com", "3 ч 40 мин", "100%"]]);
    expect(within(programs).getAllByRole("columnheader").map((c) => c.textContent)).toEqual(["Название", "Время", "Доля"]);
  });

  // Ловит (8.2.6 п.6-7): нарушения не за период, обычное событие среди нарушений; копилка без истории.
  it("«Нарушения» - только нарушения; «Копилка» - баланс и история за неделю", async () => {
    await weekG15();
    const v = screen.getByRole("region", { name: "Нарушения" });
    expect(within(within(v).getByRole("list", { name: "Нарушения" })).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "19 ноя, 18:05Миша: попытка открыть запрещённую программу Steam. Программа закрыта",
    ]);
    const piggy = screen.getByRole("region", { name: "Копилка" });
    expect(within(piggy).getByText("Сейчас в копилке: 25 мин")).toBeTruthy();
    expect(within(within(piggy).getByRole("list", { name: "История копилки за период" })).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "21 ноя, 00:00+20 мин за неизрасходованное время прошлого дняБаланс: 25 мин",
      "19 ноя, 16:00-15 мин: потраченоБаланс: 5 мин",
    ]);
  });

  it("больше 20 нарушений: первые 20 и «Показать все (25)»", async () => {
    vi.setSystemTime(G15_NOW);
    const many = Array.from({ length: 25 }, (_, i) => ({
      atMs: G15_NOW.getTime() - (i + 1) * 60_000,
      tzOffsetMin: -G15_NOW.getTimezoneOffset(),
      childId: 1,
      childName: "Миша",
      kind: "bypass",
      text: `Миша: обход защиты ${i + 1}`,
    }));
    const user = setup({ children: [g15Child()], events: many });
    const v = await screen.findByRole("region", { name: "Нарушения" });
    expect(within(v).getAllByRole("listitem")).toHaveLength(20);
    await user.click(within(v).getByRole("button", { name: "Показать все (25)" }));
    expect(within(v).getAllByRole("listitem")).toHaveLength(25);
    expect(within(v).queryByRole("button")).toBeNull();
  });

  // Ловит (8.2.6 п.9): пустой период - нули в разделах вместо одной строки; нарушения и копилка пропали.
  it("пустой период: одна строка «За этот период пока нет данных», нарушения и копилка - своими словами", async () => {
    const user = setup({ children: [mockChild({ usedS: 0 })] });
    await user.click(await screen.findByRole("tab", { name: "Месяц" }));
    const box = screen.getByRole("region", { name: "Итоги" });
    expect(await within(box).findByText("За этот период пока нет данных")).toBeTruthy();
    expect(within(box).getAllByText("За этот период пока нет данных")).toHaveLength(1);
    for (const name of ["По дням", "Программы", "Сайты"]) expect(screen.queryByRole("region", { name }), name).toBeNull();
    expect(within(screen.getByRole("region", { name: "Нарушения" })).getByText("Нарушений за этот период нет")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Копилка" })).getByText("За этот период копилка не менялась")).toBeTruthy();
  });

  it("сеанс не ребёнка: ни разделов, ни подписи «Статистика ведётся с»", async () => {
    setup({ myChildId: null });
    await screen.findByRole("region", { name: "Итоги" });
    for (const name of ["По дням", "Программы", "Сайты", "Нарушения", "Копилка"]) expect(screen.queryByRole("region", { name }), name).toBeNull();
    expect(screen.queryByText(/Статистика ведётся/)).toBeNull();
  });

  // Ловит: поздний ответ прошлого периода перекрывает столбики выбранного (как у «Итогов»).
  it("поздний ответ недели не перекрывает столбики месяца", async () => {
    vi.setSystemTime(G15_NOW);
    const user = setup({ children: [g15Child()], events: g15Events() });
    await screen.findByRole("region", { name: "По дням" });
    let releaseWeek: () => void = () => {};
    const weekGate = new Promise<void>((r) => (releaseWeek = r));
    const real = mock.handler("MyToday");
    mock.override("MyToday", async (payload) => {
      if ((payload as { period?: string }).period === "week") await weekGate;
      return real(payload);
    });
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    await user.click(screen.getByRole("tab", { name: "Месяц" }));
    await screen.findByText("Использовано за месяц");
    releaseWeek();
    await new Promise((r) => setTimeout(r, 20));
    const svg = () => screen.getByRole("region", { name: "По дням" }).querySelector("svg");
    expect(svg()?.getAttribute("aria-label")).toBe("По дням за месяц: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
    expect(within(within(screen.getByRole("region", { name: "По дням" })).getByRole("group")).getAllByRole("img")).toHaveLength(22);
  });
});

describe("MyStats: значение плитки частями", () => {
  // Ловит (возврат 2 T5): длинное значение плитки не делится на части и не переносится; текст плитки изменился.
  it("«1 ч 15 мин» - две неразрывные части, текст прежний", async () => {
    setup({ children: [mockChild({ usedS: 2100, gamesUsedS: 1200, videoUsedS: 300, studyUsedS: 2400 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    const value = within(box).getByRole("tab", { name: "Всего" }).querySelector(".totals-tab-value")!;
    expect(value.textContent).toBe("1 ч 15 мин");
    expect(Array.from(value.querySelectorAll(".totals-tab-part")).map((p) => p.textContent)).toEqual(["1 ч", "15 мин"]);
    const short = within(box).getByRole("tab", { name: "Игры" }).querySelector(".totals-tab-value")!;
    expect(Array.from(short.querySelectorAll(".totals-tab-part")).map((p) => p.textContent)).toEqual(["20 мин"]);
  });
});
