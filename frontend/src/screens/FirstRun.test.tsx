import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import FirstRun from "./FirstRun";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-32: один раз при первом входе после установки окно с пересказом правил на сегодня и кнопкой «Понятно».

let mock: MockBackend;

// Ребёнку на «ты» (A4). \b в JS не видит кириллицу, поэтому границы слов через \p{L}.
const YOU_FORM = /(?<!\p{L})(вы|вам|вас|ваш\p{L}*|попробуйте)(?!\p{L})/iu;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ mode: "firstrun", ...opts });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<FirstRun />);
  return user;
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("FirstRun: пересказ правил на сегодня", () => {
  it("заголовок и правила из программы", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Теперь время за компьютером считается" })).toBeTruthy();
    expect(screen.getByText("Сегодня можно 2 ч.")).toBeTruthy();
    expect(screen.getByText("Сколько осталось, видно в значке у часов.")).toBeTruthy();
  });

  // Ловит: запретные часы в M1 упомянуты, хотя их ещё нельзя задать.
  it("запретные часы не упоминаются", async () => {
    setup();
    await screen.findByText("Сегодня можно 2 ч.");
    expect(document.body.textContent).not.toMatch(/запретн|\d\d:\d\d/i);
  });

  it("без лимита сегодня: так и сказано", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: true }));
    setup({ children: [mockChild({ limits })] });
    expect(await screen.findByText("Сегодня без ограничений по времени.")).toBeTruthy();
  });

  it("доп. время, уже добавленное родителем, входит в «можно»", async () => {
    setup({ children: [mockChild({ extraMin: 30 })] });
    expect(await screen.findByText("Сегодня можно 2 ч 30 мин.")).toBeTruthy();
  });

  it("нет полей ввода, обращение на «ты», без длинных тире", async () => {
    setup();
    await screen.findByText("Сегодня можно 2 ч.");
    expect(document.querySelectorAll("input, textarea, select")).toHaveLength(0);
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(YOU_FORM);
  });
});

describe("FirstRun: «Понятно»", () => {
  // Ловит: окно не запоминает, что показано, и выскакивает при каждом входе (FR-32: один раз).
  it("отмечает первый вход и закрывает окно", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Понятно" }));
    await vi.waitFor(() => expect(mock.state.finishCode).toBe(0));
    expect(mock.count("FirstRunSeen")).toBe(1);
    expect(mock.state.firstRunSeen).toBe(true);
  });

  // Ловит: двойной щелчок отправляет отметку дважды.
  it("двойной щелчок не повторяет отметку", async () => {
    const user = setup();
    const button = await screen.findByRole("button", { name: "Понятно" });
    await user.dblClick(button);
    await vi.waitFor(() => expect(mock.state.finishCode).toBe(0));
    expect(mock.count("FirstRunSeen")).toBe(1);
  });

  it("после «Понятно» кнопки нет, а окно просит закрыть, если само не закрылось", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Понятно" }));
    expect(await screen.findByText("Готово. Это окно можно закрыть.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Понятно" })).toBeNull();
  });

  // Ловит: отметка не записалась, а окно закрылось: завтра оно появится снова без причины.
  it("отметка не записалась: окно остаётся, объясняет простыми словами и даёт повторить", async () => {
    const user = setup();
    mock.override("FirstRunSeen", "reject", 1);
    await user.click(await screen.findByRole("button", { name: "Понятно" }));
    expect(await screen.findByText("Программа не отвечает. Попробуй ещё раз через минуту")).toBeTruthy();
    expect(mock.state.finishCode).toBeNull();
    await user.click(screen.getByRole("button", { name: "Понятно" }));
    await vi.waitFor(() => expect(mock.state.finishCode).toBe(0));
    expect(mock.count("FirstRunSeen")).toBe(2);
  });

  // Ловит: тупик (L-119): правила не получены, а выйти из окна нечем.
  it("правила не получены: сказано словами, «Понятно» всё равно есть", async () => {
    mock = installBackend({ mode: "firstrun" });
    mock.override("MyToday", "reject");
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<FirstRun />);
    expect(await screen.findByText("Не удалось получить правила. Попробуй через минуту")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/ipc|pipe|undefined/i);
    await user.click(screen.getByRole("button", { name: "Понятно" }));
    await vi.waitFor(() => expect(mock.count("FirstRunSeen")).toBe(1));
  });
});
