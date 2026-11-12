import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Bonus from "./Bonus";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-22 (D-46: обнулить и убавить), вкладка «Бонусы» (R25, D-49) и ссылка на раздел «Промокоды» (RM5-13).

let mock: MockBackend;
const NOW = new Date(2026, 9, 23, 12, 0, 0);
const AUTO = "Бонус за свободное время: 15 мин";

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({
    sessionActive: true,
    children: [
      mockChild({
        bonusBalance: 45,
        bonusLedger: [
          { atMs: NOW.getTime() - 12 * 3600_000, tzOffsetMin: 180, deltaMin: 15, reason: AUTO, balanceAfter: 45 },
          { atMs: NOW.getTime() - 36 * 3600_000, tzOffsetMin: 180, deltaMin: 30, reason: "за уборку", balanceAfter: 30 },
        ],
      }),
    ],
    ...opts,
  });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<Bonus />);
  return user;
}

const misha = () => screen.findByRole("article", { name: "Копилка, Миша" });
const balanceOf = (card: HTMLElement) => card.querySelector(".bonus-balance")?.textContent;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("Копилка: обнулить и убавить (D-46)", () => {
  // Ловит: обнуление без подтверждения или баланс не меняется.
  it("«Обнулить копилку» спрашивает и обнуляет", async () => {
    const user = setup();
    const card = await misha();
    await user.click(within(card).getByRole("button", { name: "Обнулить копилку" }));
    const dialog = screen.getByRole("dialog", { name: "Обнулить копилку?" });
    expect(within(dialog).getByText(/Баланс Миши станет 0 мин/)).toBeTruthy();
    expect(mock.count("ClearBonus")).toBe(0);
    await user.click(within(dialog).getByRole("button", { name: "Обнулить" }));
    expect(await within(card).findByText("Копилка обнулена")).toBeTruthy();
    expect(balanceOf(card)).toBe("Баланс: 0 мин");
    expect(within(card).getByText(/-45 мин: Обнулено родителем/)).toBeTruthy();
  });

  // Ловит: «Отмена» всё равно списывает.
  it("«Отмена» в подтверждении ничего не меняет", async () => {
    const user = setup();
    const card = await misha();
    await user.click(within(card).getByRole("button", { name: "Обнулить копилку" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("ClearBonus")).toBe(0);
    expect(balanceOf(card)).toBe("Баланс: 45 мин");
  });

  // Ловит: «Убавить» не даёт выбрать −15/−30/−60 или списывает без подтверждения.
  it("«Убавить» на 30 минут с подтверждением", async () => {
    const user = setup();
    const card = await misha();
    await user.click(within(card).getByRole("button", { name: "Убавить" }));
    const dialog = screen.getByRole("dialog", { name: "Убавить копилку?" });
    await user.click(within(dialog).getByRole("radio", { name: "−30" }));
    await user.click(within(dialog).getByRole("button", { name: "Убавить на 30 мин" }));
    expect(mock.calls.find((c) => c.command === "ReduceBonus")?.payload).toEqual({ childId: 1, minutes: 30 });
    expect(await within(card).findByText("Убавлено 30 минут. Баланс 15 мин")).toBeTruthy();
    expect(balanceOf(card)).toBe("Баланс: 15 мин");
  });

  // Ловит: своё число больше баланса уводит копилку в минус.
  it("своё число больше баланса — копилка не ниже 0", async () => {
    const user = setup();
    const card = await misha();
    await user.click(within(card).getByRole("button", { name: "Убавить" }));
    const dialog = screen.getByRole("dialog", { name: "Убавить копилку?" });
    await user.click(within(dialog).getByRole("radio", { name: "Своё" }));
    const own = within(dialog).getByLabelText("Сколько минут убавить");
    await user.clear(own);
    await user.type(own, "100");
    await user.click(within(dialog).getByRole("button", { name: "Убавить на 100 мин" }));
    await vi.waitFor(() => expect(balanceOf(card)).toBe("Баланс: 0 мин"));
  });

  it("пустая копилка: кнопки недоступны с пояснением", async () => {
    setup({ children: [mockChild({ bonusBalance: 0 })] });
    const card = await misha();
    expect((within(card).getByRole("button", { name: "Обнулить копилку" }) as HTMLButtonElement).disabled).toBe(true);
    expect((within(card).getByRole("button", { name: "Убавить" }) as HTMLButtonElement).disabled).toBe(true);
    expect(within(card).getByText("Копилка пуста: убавлять нечего")).toBeTruthy();
  });
});

describe("Вкладка «Бонусы» (R25, D-49)", () => {
  // Ловит: автобонус не виден родителю или показан технической строкой.
  it("показывает автобонусы понятным языком и только их", async () => {
    const user = setup();
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    const panel = screen.getByRole("tabpanel", { name: "Бонусы" });
    const box = within(panel).getByRole("region", { name: "Автобонусы, Миша" });
    expect(within(box).getByText("+15 мин за неизрасходованное время прошлого дня")).toBeTruthy();
    expect(within(box).queryByText(/за уборку/)).toBeNull();
    expect(within(panel).getByText(/5 мин за каждые полные 10 минут/)).toBeTruthy();
  });

  it("автобонусов ещё не было — понятная заглушка", async () => {
    const user = setup({ children: [mockChild({ bonusBalance: 0, bonusLedger: [] })] });
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    expect(screen.getByText("Автобонусов пока не было. Они начисляются в полночь за прошедший день")).toBeTruthy();
  });
});

describe("Вкладка «Бонусы»: ссылка на «Промокоды» (RM5-13)", () => {
  // Ловит: форма промокодов осталась в «Копилке» вторым местом для кодов или кнопка никуда не ведёт.
  it("нет поля «Код», есть «Открыть», клик ведёт в раздел «Промокоды»", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild({ bonusBalance: 45 })] });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    const onOpenPromos = vi.fn();
    render(<Bonus onOpenPromos={onOpenPromos} />);
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    const panel = screen.getByRole("tabpanel", { name: "Бонусы" });
    expect(within(panel).queryByLabelText("Код")).toBeNull();
    expect(within(panel).queryByRole("list", { name: "Действующие промокоды" })).toBeNull();
    const link = within(panel).getByRole("region", { name: "Промокоды" });
    expect(within(link).getByText("Промокоды теперь в отдельном разделе «Промокоды»")).toBeTruthy();
    await user.click(within(link).getByRole("button", { name: "Открыть" }));
    expect(onOpenPromos).toHaveBeenCalledTimes(1);
    expect(mock.count("ListPromos")).toBe(0);
  });
});

describe("Копилка: «Отменить» начисление (B-33)", () => {
  // Ловит: копилка показывает отмену своим кодом - без живой области до сообщения, без отсчёта,
  // без удержания кнопки, пока на ней фокус.
  it("живая область есть до начисления; «Отменить» с отсчётом, держится в фокусе, отменяет", async () => {
    const user = setup();
    const card = await misha();
    const live = within(card).getByRole("status");
    expect(live.textContent).toBe("");
    await user.type(within(card).getByLabelText("За что"), "за уборку");
    await user.click(within(card).getByRole("button", { name: "Начислить" }));
    expect(await within(live).findByText("Начислено 30 минут. Баланс 75 мин")).toBeTruthy();
    expect(within(card).getAllByRole("status")).toHaveLength(1);
    const undo = within(live).getByRole("button", { name: "Отменить" });
    expect(live.querySelector(".undo-left")?.textContent).toBe("10 с");
    act(() => undo.focus());
    act(() => {
      vi.advanceTimersByTime(12_000);
    });
    expect(within(live).getByRole("button", { name: "Отменить" })).toBe(undo);
    expect(live.querySelector(".undo-left")).toBeNull();
    act(() => undo.blur());
    expect(within(live).queryByRole("button", { name: "Отменить" })).toBeNull();
  });

  it("«Отменить» в течение 10 с отменяет начисление", async () => {
    const user = setup();
    const card = await misha();
    await user.type(within(card).getByLabelText("За что"), "за уборку");
    await user.click(within(card).getByRole("button", { name: "Начислить" }));
    await user.click(await within(card).findByRole("button", { name: "Отменить" }));
    expect(await within(card).findByText("Начисление отменено")).toBeTruthy();
    expect(mock.count("UndoBonus")).toBe(1);
    await vi.waitFor(() => expect(balanceOf(card)).toBe("Баланс: 45 мин"));
  });
});
