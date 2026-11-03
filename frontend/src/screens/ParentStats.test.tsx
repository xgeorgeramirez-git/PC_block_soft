import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ParentStats from "./ParentStats";
import { installBackend, mockChild, S42_NOW, s42Child, uninstallBackend, type MockBackend } from "../__mocks__/backend";

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
    expect(screen.getByRole("tabpanel", { name: "Год" })).toBeTruthy();
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
