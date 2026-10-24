import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "../App";
import { DEFAULT_PASSWORD, installBackend, mockChild, uninstallBackend } from "../__mocks__/backend";
import { HINTS, hintById } from "./catalog";

// Подсказки фидбек-раунда 0.2.2 (раздел «Покрытие Помощника»): у каждой есть место на своём экране.
// Остальные подсказки помечены принятыми, чтобы в окне оставалась ровно проверяемая.

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

const EVERY_DAY = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, startMin: 21 * 60, endMin: 7 * 60 }));

async function openWindow(only: string, schedule: typeof EVERY_DAY = EVERY_DAY) {
  const done = HINTS.filter((h) => h.kind === "hint" && h.id !== only).map((h) => h.id);
  installBackend({ helper: { done }, children: [mockChild({ schedule, bonusBalance: 45 })] });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<App />);
  await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
  await screen.findByRole("heading", { name: "Сегодня" });
  return user;
}

const titleOf = (id: string) => hintById(id)!.title;

describe("подсказки 0.2.2 стоят на своих местах", () => {
  // Ловит: у кнопки «Снять запрет на сегодня» нет места для карточки H-M2-04.
  it("H-M2-04: на карточке ребёнка с запретными часами", async () => {
    await openWindow("H-M2-04");
    expect(await screen.findByText(titleOf("H-M2-04"))).toBeTruthy();
  });

  // Ловит: подсказка про снятие запрета появляется там, где снимать нечего (запретных часов нет).
  it("H-M2-04: без запретных часов не показывается", async () => {
    await openWindow("H-M2-04", []);
    await screen.findByRole("article", { name: "Миша" });
    await vi.advanceTimersByTimeAsync(1500);
    expect(screen.queryByText(titleOf("H-M2-04"))).toBeNull();
  });

  // Ловит: H-M2-05 про «Убавить» и «Обнулить» не показывается на экране «Копилка».
  it("H-M2-05: на вкладке «Копилка»", async () => {
    const user = await openWindow("H-M2-05");
    await user.click(screen.getByRole("button", { name: "Копилка" }));
    expect(await screen.findByText(titleOf("H-M2-05"))).toBeTruthy();
  });

  // Ловит: вкладка «Бонусы» не объяснена (H-M2-06).
  it("H-M2-06: у вкладок экрана «Копилка»", async () => {
    const user = await openWindow("H-M2-06");
    await user.click(screen.getByRole("button", { name: "Копилка" }));
    expect(await screen.findByText(titleOf("H-M2-06"))).toBeTruthy();
  });

  // Ловит: у формы создания промокода нет места для H-M2-07.
  it("H-M2-07: над формой промокода на вкладке «Бонусы»", async () => {
    const user = await openWindow("H-M2-07");
    await user.click(screen.getByRole("button", { name: "Копилка" }));
    await user.click(await screen.findByRole("tab", { name: "Бонусы" }));
    const card = await screen.findByText(titleOf("H-M2-07"));
    const form = screen.getByRole("button", { name: "Создать промокод" });
    // Карточка стоит выше формы, а не поверх неё.
    expect(card.compareDocumentPosition(form) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
