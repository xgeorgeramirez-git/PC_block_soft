import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ParentStats from "./ParentStats";
import { G15_NOW, g15Child, g15Events, installBackend, mockChild, S42_NOW, s42Child, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-23, D-44, D-61: статистика родителя с выбором ребёнка и периода «Итогов», только просмотр.

let mock: MockBackend;
const NOW = new Date(2026, 9, 19, 12, 0, 0);

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

function setup(initialChildId?: number) {
  mock = installBackend({
    sessionActive: true,
    children: [mockChild({ usedS: 2100 }), mockChild({ childId: 2, sid: "S-2", name: "Даша", usedS: 600 })],
  });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<ParentStats initialChildId={initialChildId} />);
  return user;
}

describe("ParentStats", () => {
  it("показывает статистику первого ребёнка за сегодня, только просмотр", async () => {
    setup();
    expect(await screen.findByText("Только просмотр")).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Итоги" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Мои правила" })).toBeTruthy();
    // Нет полей ввода — только чтение.
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.queryByRole("spinbutton")).toBeNull();
  });

  // Ловит: родитель видит у игр и учёбы заглушку, а не время за сегодня (S13).
  it("итоги игр и учёбы за сегодня", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild({ gamesUsedS: 1200, studyUsedS: 2400 })] });
    render(<ParentStats />);
    const box = await screen.findByRole("region", { name: "Итоги" });
    const tabs = within(within(box).getByRole("tablist", { name: "Итоги по категориям" }));
    const value = (label: string) => tabs.getByRole("tab", { name: label }).querySelector(".totals-tab-value")?.textContent;
    expect(value("Игры")).toBe("20 мин");
    expect(value("Учёба")).toBe("40 мин");
  });

  // Ловит (S39, D-58): у родителя нет вкладок или в них другие строки, чем у ребёнка.
  it("«Игры» у родителя: сумма и каждая игра с долей", async () => {
    mock = installBackend({
      sessionActive: true,
      children: [
        mockChild({
          usedS: 900,
          gamesUsedS: 900,
          programsToday: [
            { path: "c:\\games\\game.exe", name: "game", category: "games", seconds: 600 },
            { path: "c:\\games\\game2.exe", name: "game2", category: "games", seconds: 300 },
          ],
        }),
      ],
    });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<ParentStats />);
    const box = await screen.findByRole("region", { name: "Итоги" });
    await user.click(within(box).getByRole("tab", { name: "Игры" }));
    expect(within(box).getByText("Игры за сегодня: 15 мин")).toBeTruthy();
    const rows = within(within(box).getByRole("list", { name: "Игры: по программам" }))
      .getAllByRole("listitem")
      .map((li) => li.textContent);
    expect(rows).toEqual(["game10 мин67%", "game25 мин33%"]);
  });

  it("переключение ребёнка перезапрашивает его статистику", async () => {
    const user = setup(1);
    await screen.findByText("Только просмотр");
    const picker = screen.getByRole("group", { name: "Ребёнок" });
    await user.click(within(picker).getByRole("button", { name: "Даша" }));
    await vi.waitFor(() => {
      const stats = mock.calls.filter((c) => c.command === "ParentStats");
      expect(stats.at(-1)?.payload).toEqual({ childId: 2, period: "today" });
    });
  });

  it("открывается сразу на выбранном ребёнке (с карточки «Сегодня»)", async () => {
    setup(2);
    await screen.findByText("Только просмотр");
    await vi.waitFor(() => {
      const stats = mock.calls.filter((c) => c.command === "ParentStats");
      expect(stats[0]?.payload).toEqual({ childId: 2, period: "today" });
    });
  });

  // Ловит (S42, D-61): у родителя нет периодов (только «Пока показываем сегодня»), период не уходит в
  // запрос, цифры периода не те, что у ребёнка.
  it("S42 у родителя: год - игры 1 ч 00 мин, учёба 40 мин, «Использовано за год»", async () => {
    vi.setSystemTime(S42_NOW);
    mock = installBackend({ sessionActive: true, children: [s42Child()] });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<ParentStats />);
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(screen.queryByText(/Пока показываем сегодня/)).toBeNull();
    const periods = within(screen.getByRole("tablist", { name: "Период" })).getAllByRole("tab");
    expect(periods.map((t) => t.textContent)).toEqual(["Сегодня", "Неделя", "Месяц", "Год", "Всё время"]);
    await user.click(screen.getByRole("tab", { name: "Год" }));
    await within(box).findByText("Использовано за год");
    const value = (label: string) =>
      within(within(box).getByRole("tablist", { name: "Итоги по категориям" })).getByRole("tab", { name: label }).querySelector(".totals-tab-value")?.textContent;
    expect([value("Игры"), value("Учёба"), value("Всего")]).toEqual(["1 ч 00 мин", "40 мин", "1 ч 40 мин"]);
    await user.click(within(box).getByRole("tab", { name: "Учёба" }));
    expect(within(box).getByText("Учёба за год: 40 мин")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "ParentStats").at(-1)?.payload).toEqual({ childId: 1, period: "year" });
    const panel = screen.getByRole("tabpanel", { name: "Год" });
    // Вкладки периода - внутри «Итогов», как у ребёнка; «Мои правила» всегда за сегодня (ревью FB-13).
    expect(box.contains(panel)).toBe(true);
    expect(panel.contains(screen.getByRole("region", { name: "Мои правила" }))).toBe(false);
    for (const t of periods) expect(t.getAttribute("aria-controls")).toBe(panel.id);
  });

  // Ловит (B-46): пока данные идут, вкладки периодов ссылаются aria-controls на панель, которой нет
  // в документе. Теперь вкладки - внутри «Итогов» и появляются вместе с данными.
  it("пока данные идут: ни вкладок, ни висячей ссылки, только «Загружаю данные»", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild({ usedS: 2100 })] });
    mock.override("ParentStats", () => new Promise<never>(() => {}));
    render(<ParentStats />);
    expect(await screen.findByText("Загружаю данные…")).toBeTruthy();
    await vi.waitFor(() => expect(mock.count("ParentStats")).toBe(1));
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(screen.queryByRole("tab")).toBeNull();
    expect(document.querySelector("[aria-controls]")).toBeNull();
    expect(screen.queryByRole("region", { name: "Мои правила" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Очистить статистику" })).toBeNull();
  });

  // Ловит: переключение ребёнка сбрасывает выбранный период на «Сегодня».
  it("переключение ребёнка сохраняет период", async () => {
    const user = setup(1);
    await user.click(await screen.findByRole("tab", { name: "Месяц" }));
    await user.click(within(screen.getByRole("group", { name: "Ребёнок" })).getByRole("button", { name: "Даша" }));
    await vi.waitFor(() => {
      expect(mock.calls.filter((c) => c.command === "ParentStats").at(-1)?.payload).toEqual({ childId: 2, period: "month" });
    });
    expect(screen.getByRole("tab", { name: "Месяц" }).getAttribute("aria-selected")).toBe("true");
  });
});

// Ревью окна M5: на экране всегда цифры выбранного ребёнка и выбранного периода, даже при сбое.
describe("ParentStats: ответ не того ребёнка и сбои", () => {
  const LINK_DOWN = "Программа не отвечает. Попробуйте ещё раз через минуту";

  // Ловит: до ответа о новом ребёнке видны цифры прежнего и «Очистить статистику», которая спросит про нового.
  it("смена ребёнка до ответа: «Загружаю данные», цифр прежнего и кнопки очистки нет", async () => {
    const user = setup(1);
    await screen.findByRole("region", { name: "Итоги" });
    expect(screen.getByRole("button", { name: "Очистить статистику" })).toBeTruthy();
    const real = mock.handler("ParentStats");
    mock.override("ParentStats", (p: { childId: number }) => (p.childId === 2 ? new Promise<never>(() => {}) : real(p)));
    await user.click(within(screen.getByRole("group", { name: "Ребёнок" })).getByRole("button", { name: "Даша" }));
    expect(await screen.findByText("Загружаю данные…")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Итоги" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Мои правила" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Очистить статистику" })).toBeNull();
  });

  // Ловит: сбой при смене ребёнка оставляет на экране цифры прежнего без «Повторить».
  it("сбой при смене ребёнка: уведомление и «Повторить» на месте цифр, кнопки очистки нет", async () => {
    const user = setup(1);
    await screen.findByRole("region", { name: "Итоги" });
    mock.override("ParentStats", "reject", 1);
    await user.click(within(screen.getByRole("group", { name: "Ребёнок" })).getByRole("button", { name: "Даша" }));
    expect(await screen.findByText(LINK_DOWN)).toBeTruthy();
    expect(screen.getAllByText(LINK_DOWN)).toHaveLength(1);
    expect(screen.queryByRole("region", { name: "Итоги" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Очистить статистику" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("region", { name: "Итоги" })).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "ParentStats").at(-1)?.payload).toEqual({ childId: 2, period: "today" });
    expect(screen.getByRole("button", { name: "Очистить статистику" })).toBeTruthy();
    expect(screen.queryByText(LINK_DOWN)).toBeNull();
  });

  // Ловит: при сбое периода вкладка показывает новый период, а под ней цифры прежнего.
  it("сбой периода - вкладка и цифры одного периода", async () => {
    const user = setup(1);
    const box = await screen.findByRole("region", { name: "Итоги" });
    await within(box).findByText("Использовано сегодня");
    mock.override("ParentStats", "reject", 1);
    await user.click(screen.getByRole("tab", { name: "Неделя" }));
    expect(await screen.findByText(LINK_DOWN)).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("tab", { name: "Неделя" }).getAttribute("aria-selected")).toBe("false");
    expect(within(box).getByText("Использовано сегодня")).toBeTruthy();
    // «Обновить» спрашивает показанный период, а не тот, что не загрузился.
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    await vi.waitFor(() => expect(mock.calls.filter((c) => c.command === "ParentStats").at(-1)?.payload).toEqual({ childId: 1, period: "today" }));
    await vi.waitFor(() => expect(screen.queryByText(LINK_DOWN)).toBeNull());
  });
});

// Разделы за период у родителя на неделе G15 (S21) и «Очистить статистику» (FR-38, 8.3.7 п.3, S48).
describe("ParentStats: G15 и очистка статистики", () => {
  function g15Setup(twoChildren = false) {
    vi.setSystemTime(G15_NOW);
    const kids = [g15Child()];
    if (twoChildren) kids.push(mockChild({ childId: 2, sid: "S-2", name: "Даша", usedS: 600 }));
    mock = installBackend({ sessionActive: true, children: kids, events: g15Events() });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<ParentStats />);
    return user;
  }
  const allValue = () =>
    within(within(screen.getByRole("region", { name: "Итоги" })).getByRole("tablist", { name: "Итоги по категориям" }))
      .getByRole("tab", { name: "Всего" })
      .querySelector(".totals-tab-value")?.textContent;
  const clearStatus = () => document.querySelector(".stat-clear-status") as HTMLElement;

  // Ловит (S21): у родителя другие разделы или цифры, чем у ребёнка.
  it("неделя: 7 столбиков, таблицы, нарушение и копилка", async () => {
    const user = g15Setup();
    await user.click(await screen.findByRole("tab", { name: "Неделя" }));
    await screen.findByText("Использовано за неделю");
    const days = screen.getByRole("region", { name: "По дням" });
    expect(within(within(days).getByRole("group")).getAllByRole("img")).toHaveLength(7);
    const programs = within(screen.getByRole("region", { name: "Программы" })).getByRole("table");
    expect(within(programs).getAllByRole("row").slice(1).map((tr) => tr.children[0].textContent)).toEqual(["game.exe", "other.exe", "study.exe"]);
    expect(within(screen.getByRole("region", { name: "Сайты" })).getByRole("table").textContent).toContain("youtube.com");
    expect(within(screen.getByRole("region", { name: "Нарушения" })).getAllByRole("listitem")).toHaveLength(1);
    expect(within(screen.getByRole("region", { name: "Копилка" })).getByText("Сейчас в копилке: 25 мин")).toBeTruthy();
    expect(screen.getByText("Статистика ведётся с 16 ноября 2026")).toBeTruthy();
  });

  // Ловит (8.3.7 п.3): кнопки нет при одном ребёнке (группа выбора скрыта вместе с ней).
  it("кнопка «Очистить статистику» при одном ребёнке (выбора ребёнка нет)", async () => {
    g15Setup();
    expect(await screen.findByRole("button", { name: "Очистить статистику" })).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Ребёнок" })).toBeNull();
  });

  it("кнопка «Очистить статистику» при двух детях - рядом с выбором ребёнка", async () => {
    g15Setup(true);
    const picker = await screen.findByRole("group", { name: "Ребёнок" });
    const btn = await screen.findByRole("button", { name: "Очистить статистику" });
    expect(picker.parentElement).toBe(btn.parentElement);
  });

  // Ловит (8.3.7 п.3): текст окна не по ТЗ; фокус на «Удалить» (Enter сразу удаляет всё).
  it("окно подтверждения: текст 8.3.7 п.3, фокус на «Отмена»", async () => {
    const user = g15Setup();
    await user.click(await screen.findByRole("button", { name: "Очистить статистику" }));
    const dialog = screen.getByRole("dialog", { name: "Удалить всю статистику Миши?" });
    expect(within(dialog).getByText("Остаток на сегодня и копилка не изменятся. Это нельзя отменить.")).toBeTruthy();
    expect(within(dialog).getAllByRole("button").map((b) => b.textContent)).toEqual(["Удалить", "Отмена"]);
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Отмена" }));
  });

  it("«Отмена» и Esc закрывают окно, ничего не удаляя", async () => {
    const user = g15Setup();
    await user.click(await screen.findByRole("button", { name: "Очистить статистику" }));
    await user.click(screen.getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Очистить статистику" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("ClearStats")).toBe(0);
    expect(clearStatus().textContent).toBe("");
  });

  // Ловит (FR-38, S48): очистка уходит без номера ребёнка или дважды; итоги и подпись не обновились.
  it("«Удалить»: один вызов, «Статистика Миши очищена», итоги нули, «Статистика ведётся с 22 ноября 2026»", async () => {
    const user = g15Setup();
    await screen.findByText("Статистика ведётся с 16 ноября 2026");
    expect(allValue()).toBe("10 мин");
    expect(clearStatus().getAttribute("role")).toBe("status");
    expect(clearStatus().textContent).toBe("");
    await user.click(screen.getByRole("button", { name: "Очистить статистику" }));
    await user.click(screen.getByRole("button", { name: "Удалить" }));
    await vi.waitFor(() => expect(clearStatus().textContent).toBe("Статистика Миши очищена"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.calls.filter((c) => c.command === "ClearStats").map((c) => c.payload)).toEqual([{ childId: 1 }]);
    await screen.findByText("Статистика ведётся с 22 ноября 2026");
    expect(allValue()).toBe("0 мин");
    // Остаток на сегодня (учёт лимита) не изменился (D-73): строка «Сегодня» от ядра.
    expect(mock.state.children[0].usedS).toBe(600);
  });

  it("ошибка очистки: уведомление, данные экрана прежние", async () => {
    const user = g15Setup();
    await screen.findByText("Статистика ведётся с 16 ноября 2026");
    mock.override("ClearStats", { ok: false, error: "Не удалось очистить статистику. Попробуйте ещё раз" });
    await user.click(screen.getByRole("button", { name: "Очистить статистику" }));
    await user.click(screen.getByRole("button", { name: "Удалить" }));
    expect(await screen.findByText("Не удалось очистить статистику. Попробуйте ещё раз")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("Статистика ведётся с 16 ноября 2026")).toBeTruthy();
    expect(allValue()).toBe("10 мин");
    expect(clearStatus().textContent).toBe("");
  });

  it("пока идёт очистка, «Удалить» недоступна: второй вызов не уходит", async () => {
    const user = g15Setup();
    await screen.findByText("Статистика ведётся с 16 ноября 2026");
    mock.override("ClearStats", () => new Promise<never>(() => {}));
    await user.click(screen.getByRole("button", { name: "Очистить статистику" }));
    const del = screen.getByRole("button", { name: "Удалить" });
    await user.click(del);
    expect((del as HTMLButtonElement).disabled).toBe(true);
    await user.click(del);
    expect(mock.count("ClearStats")).toBe(1);
  });

  it("смена ребёнка убирает сообщение об очистке прежнего", async () => {
    const user = g15Setup(true);
    await user.click(await screen.findByRole("button", { name: "Очистить статистику" }));
    await user.click(screen.getByRole("button", { name: "Удалить" }));
    await vi.waitFor(() => expect(clearStatus().textContent).toBe("Статистика Миши очищена"));
    await user.click(within(screen.getByRole("group", { name: "Ребёнок" })).getByRole("button", { name: "Даша" }));
    expect(clearStatus().textContent).toBe("");
  });
});
