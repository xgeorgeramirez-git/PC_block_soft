import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "../App";
import { DEFAULT_PASSWORD, installBackend, mockChild, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import type { WeekdayLimit } from "../types";
import { LABELS, hintById } from "./catalog";

// Помощник в настоящем окне родителя (модульный вариант S28; сквозной - приёмка вехи, T16):
// приглашение, «Позже», подсказка на «Дети», выключатель, тихие места, тур, переход из напоминания.

let mock: MockBackend;

const UNLIMITED: WeekdayLimit[] = Array.from({ length: 7 }, (_, i) => ({ weekday: i + 1, totalMin: 0, unlimited: true }));

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

async function openWindow(opts: MockOptions = {}) {
  mock = installBackend({ children: [mockChild()], ...opts });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<App />);
  await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
  await screen.findByRole("heading", { name: "Сегодня" });
  return user;
}

const menu = (name: string) => screen.getByRole("button", { name });
const hints = () => screen.queryAllByRole("status").filter((n) => n.classList.contains("hint-card"));
const saved = () => mock.state.helper as Record<string, unknown>;
const settle = async (ms: number) => {
  for (let spent = 0; spent < ms; spent += 50) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(50);
    });
  }
};

describe("окно родителя: сценарий подсказок", () => {
  it("первый вход: приглашение в тур над разделами, окно не заблокировано", async () => {
    const user = await openWindow();
    const card = await screen.findByRole("status");
    expect(card.getAttribute("data-hint-id")).toBe("H-M1-01");
    // Окно работает, пока карточка на экране.
    await user.click(menu("Дети"));
    expect(await screen.findByRole("heading", { name: /Правила/ })).toBeTruthy();
  });

  // Ловит: S28: «Позже» на приглашении, на «Дети» подсказка про лимиты, на «Сегодня» про кнопки.
  it("«Позже», потом «Дети»: подсказка про лимиты по дням", async () => {
    const user = await openWindow();
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: LABELS.later }));
    await user.click(menu("Дети"));
    const card = await screen.findByText(hintById("H-M1-02")!.title);
    expect(card.closest("aside")).toBeTruthy();
    // Карточка стоит над таблицей лимитов, а не поверх неё.
    const table = await screen.findByRole("table");
    expect(card.compareDocumentPosition(table) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("выключено в настройках: нигде ни одной карточки, а «Помощь» и тур работают", async () => {
    const user = await openWindow({ helper: { enabled: false } });
    await settle(1500);
    expect(hints()).toHaveLength(0);
    await user.click(menu("Дети"));
    await settle(1500);
    expect(hints()).toHaveLength(0);
    await user.click(menu("Помощь"));
    expect(await screen.findByText(/Подсказки в окне сейчас выключены/)).toBeTruthy();
    // Туров теперь несколько: берём «Начать» в строке нужного тура (не первую попавшуюся).
    const tourRow = within(screen.getByRole("region", { name: "Обучение" }))
      .getByText("Короткий тур по окну родителя")
      .closest("li") as HTMLElement;
    await user.click(within(tourRow).getByRole("button", { name: "Начать" }));
    expect(await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" })).toBeTruthy();
  });

  it("выключатель в «Настройках» убирает видимую карточку сразу", async () => {
    const user = await openWindow();
    await screen.findByRole("status");
    await user.click(menu("Настройки"));
    // Карточка могла уже стоять над разделами: после выключения её нет.
    const toggle = await screen.findByRole("checkbox", { name: "Показывать подсказки" });
    await waitFor(() => expect((toggle as HTMLInputElement).disabled).toBe(false));
    await user.click(toggle);
    expect(hints()).toHaveLength(0);
    await settle(1500);
    expect(hints()).toHaveLength(0);
    await user.click(toggle);
    expect((toggle as HTMLInputElement).checked).toBe(true);
  });
});

describe("окно родителя: тихие места", () => {
  // Ловит: подсказка поверх вопроса «Сохранить изменения?».
  it("несохранённые правки: окно вопроса заглушает карточки", async () => {
    const user = await openWindow({ helper: { done: ["H-M1-01"] } });
    await user.click(menu("Дети"));
    await screen.findByText(hintById("H-M1-02")!.title);
    const hours = await screen.findByLabelText("Часы, понедельник");
    await user.clear(hours);
    await user.type(hours, "3");
    await user.click(menu("Сегодня"));
    const ask = await screen.findByRole("dialog", { name: "Сохранить изменения?" });
    expect(hints()).toHaveLength(0);
    await user.click(within(ask).getByRole("button", { name: "Остаться" }));
    expect(await screen.findByText(hintById("H-M1-02")!.title)).toBeTruthy();
  });

  // Ловит: подсказка на экране, где вводят пароль.
  it("поле пароля в фокусе: карточки нет", async () => {
    const user = await openWindow();
    await screen.findByRole("status");
    await user.click(menu("Настройки"));
    const field = await screen.findByLabelText("Старый пароль");
    await user.click(field);
    expect(hints()).toHaveLength(0);
    await settle(1500);
    expect(hints()).toHaveLength(0);
  });
});

describe("окно родителя: напоминания о риске", () => {
  const two = (a: Partial<Parameters<typeof mockChild>[0]>, b: Partial<Parameters<typeof mockChild>[0]> = {}) => [
    mockChild({ childId: 1, name: "Миша", ...a }),
    mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", ...b }),
  ];

  // Ловит: напоминание не ведёт к цели (L-08): открывает «Дети» на другом ребёнке.
  it("«Задать лимиты» открывает «Дети» сразу на нужном ребёнке", async () => {
    const user = await openWindow({ helper: { done: ["H-M1-01", "H-M1-03"] }, children: two({}, { limits: UNLIMITED }) });
    const card = await screen.findByRole("status");
    expect(within(card).getByText("Похоже, у Даши нет ограничений ни в один день")).toBeTruthy();
    expect(card.closest("article")?.getAttribute("aria-labelledby")).toBe("child-2-name");
    await user.click(within(card).getByRole("button", { name: "Задать лимиты" }));
    expect(await screen.findByRole("heading", { name: "Правила, Даша" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Даша", selected: true })).toBeTruthy();
  });

  it("права администратора: «Как исправить» без технических слов", async () => {
    const user = await openWindow({ helper: { done: ["H-M1-01", "H-M1-03"] }, children: [mockChild({ admin: true })] });
    const card = await screen.findByRole("status");
    expect(card.textContent).not.toMatch(/SID|ipc|null|undefined|служб/i);
    await user.click(within(card).getByRole("button", { name: "Как исправить" }));
    const dialog = await screen.findByRole("dialog", { name: "Как вернуть обычные права" });
    expect(within(dialog).getAllByRole("listitem")).toHaveLength(4);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("напоминание стоит на карточке нужного ребёнка и не дублируется", async () => {
    await openWindow({ helper: { done: ["H-M1-01", "H-M1-03"] }, children: two({ admin: true }, { limits: UNLIMITED }) });
    const card = await screen.findByRole("status");
    // Одна карточка за раз: сначала про Мишу, про Дашу позже.
    expect(card.closest("article")?.getAttribute("aria-labelledby")).toBe("child-1-name");
    expect(hints()).toHaveLength(1);
  });
});

describe("окно родителя: тур", () => {
  // Ловит: тур не ведёт по экранам или подсвечивает пустоту.
  it("тур из приглашения проходит «Сегодня» и «Дети» до «Готово»", async () => {
    const user = await openWindow();
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    const tour = await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    const spot = () => document.querySelector(".tour-spot")?.getAttribute("data-anchor");
    await waitFor(() => expect(spot()).toBe("menu-today"));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await waitFor(() => expect(spot()).toBe("child-card"));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await waitFor(() => expect(spot()).toBe("extra-buttons"));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await waitFor(() => expect(spot()).toBe("menu-children"));
    expect(menu("Дети").getAttribute("aria-current")).toBe("page");
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await waitFor(() => expect(spot()).toBe("limits-table"));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await waitFor(() => expect(spot()).toBe("savebar"));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourDone }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(saved().tours).toEqual(["T-M1-01"]));
  });

  // Ловит: «Пропустить тур» на шаге не работает или записывает тур пройденным.
  it("«Пропустить тур» на середине закрывает тур, окно остаётся целым", async () => {
    const user = await openWindow();
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    const tour = await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    await user.click(within(tour).getByRole("button", { name: LABELS.tourNext }));
    await user.click(within(tour).getByRole("button", { name: LABELS.tourSkip }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("heading", { name: "Сегодня" })).toBeTruthy();
    expect(saved().tours ?? []).toEqual([]);
  });

  it("Esc во время тура пропускает его", async () => {
    const user = await openWindow();
    await user.click(within(await screen.findByRole("status")).getByRole("button", { name: "Начать" }));
    await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" });
    await act(async () => undefined);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("окно родителя: «Помощь»", () => {
  it("раздел «Помощь» - настоящая справка, а не обещание", async () => {
    const user = await openWindow({ helper: { enabled: false } });
    await user.click(menu("Помощь"));
    expect(await screen.findByRole("region", { name: "Первые шаги" })).toBeTruthy();
    expect(screen.queryByText(/следующем обновлении/)).toBeNull();
    expect(menu("Помощь").getAttribute("aria-current")).toBe("page");
  });
});
