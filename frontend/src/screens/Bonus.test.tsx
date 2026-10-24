import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Bonus from "./Bonus";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-22 (D-46: обнулить и убавить), вкладка «Бонусы» (R25, D-49), промокоды (FR-35, R26, D-50).

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

describe("Промокоды (FR-35, R26, D-50)", () => {
  // Ловит: созданный код не попадает в «Действующие».
  it("создать промокод: появляется в «Действующих»", async () => {
    const user = setup();
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    await user.type(screen.getByLabelText("Код"), "a1234");
    const minutes = screen.getByLabelText("Минут");
    await user.clear(minutes);
    await user.type(minutes, "30");
    await user.click(screen.getByRole("button", { name: "Создать промокод" }));
    expect(await screen.findByText("Промокод «a1234» на 30 минут готов")).toBeTruthy();
    const active = screen.getByRole("list", { name: "Действующие промокоды" });
    expect(within(active).getByText("a1234")).toBeTruthy();
    expect(within(active).getByText("30 мин")).toBeTruthy();
    expect((screen.getByLabelText("Код") as HTMLInputElement).value).toBe("");
  });

  // Ловит: повтор кода (другой регистр) создаёт второй такой же код.
  it("повтор кода — ошибка у поля «Код»", async () => {
    const user = setup({ promos: [{ codeDisplay: "A1234", minutes: 15, createdAtMs: NOW.getTime() }] });
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    await user.type(screen.getByLabelText("Код"), " a1234 ");
    await user.click(screen.getByRole("button", { name: "Создать промокод" }));
    expect(await screen.findByText("Такой код уже есть, придумайте другой")).toBeTruthy();
    expect(screen.getByLabelText("Код").getAttribute("aria-invalid")).toBe("true");
  });

  // Ловит: родитель не видит, кто и когда использовал код.
  it("«Использованные»: когда и каким ребёнком", async () => {
    const used = NOW.getTime() - 3600_000;
    const user = setup({
      promos: [{ codeDisplay: "pizza", minutes: 20, createdAtMs: NOW.getTime() - 86400_000, usedAtMs: used, usedByChild: 1 }],
    });
    await misha();
    await user.click(screen.getByRole("tab", { name: "Бонусы" }));
    const list = await screen.findByRole("list", { name: "Использованные промокоды" });
    expect(within(list).getByText("pizza")).toBeTruthy();
    expect(within(list).getByText(/Миша/)).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Промокоды" })).getByText("Действующих кодов нет")).toBeTruthy();
  });
});
