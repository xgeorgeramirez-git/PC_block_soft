import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ParentStats from "./ParentStats";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-23, D-44: базовая статистика родителя за сегодня с выбором ребёнка, только просмотр.

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
      expect(stats.at(-1)?.payload).toEqual({ childId: 2 });
    });
  });

  it("открывается сразу на выбранном ребёнке (с карточки «Сегодня»)", async () => {
    setup(2);
    await screen.findByText("Только просмотр");
    await vi.waitFor(() => {
      const stats = mock.calls.filter((c) => c.command === "ParentStats");
      expect(stats[0]?.payload).toEqual({ childId: 2 });
    });
  });
});
