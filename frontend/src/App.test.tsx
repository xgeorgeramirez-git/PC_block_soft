import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { DEFAULT_PASSWORD, installBackend, uninstallBackend, type MockBackend, type MockOptions } from "./__mocks__/backend";

// Окно целиком: выбор экрана по режиму, вход, меню, сессия родителя (R19).

let mock: MockBackend;

function setup(opts: MockOptions = {}) {
  mock = installBackend(opts);
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<App />);
  return user;
}

async function login(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
  await screen.findByRole("heading", { name: "Сегодня" });
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: new Date(2026, 9, 19, 12, 0, 0) });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("окно: название и версия", () => {
  // Ловит: окно не показывает название программы (пустое окно или чужой заголовок).
  it("показывает название программы", async () => {
    setup();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("PC Block");
    await screen.findByLabelText("Пароль родителя");
  });

  // Ловит: версия не берётся из программы (захардкожена во фронте или не выводится).
  it("показывает версию, полученную от программы", async () => {
    setup({ version: "9.8.7" });
    expect(await screen.findByText("Версия 9.8.7")).toBeTruthy();
  });

  // Ловит: сбой связи с программой роняет окно или показывает технический текст.
  it("при сбое говорит простыми словами, что версию узнать не удалось", async () => {
    mock = installBackend();
    window.go!.ui!.App!.Version = () => Promise.reject(new Error("window.go is undefined"));
    render(<App />);
    expect(await screen.findByText("Версию узнать не удалось")).toBeTruthy();
    expect(screen.queryByText(/undefined/)).toBeNull();
  });

  it("нет связи с программой: простыми словами и «Повторить»", async () => {
    mock = installBackend({ linkDown: true });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<App />);
    // Mode() отвечает, а Hello падает.
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    mock.state.linkDown = false;
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });
});

describe("выбор экрана по режиму", () => {
  it("window, программа не настроена: мастер", async () => {
    setup({ configured: false });
    expect(await screen.findByText("Шаг 1 из 3")).toBeTruthy();
  });

  it("window, настроена: вход, потом «Сегодня»", async () => {
    const user = setup();
    await login(user);
    expect(await screen.findByRole("article", { name: "Миша" })).toBeTruthy();
  });

  it("wizard у уже настроенной программы: вход, не мастер", async () => {
    setup({ mode: "wizard" });
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
    expect(screen.queryByText(/Шаг \d из 3/)).toBeNull();
  });

  it("parent с открытой сессией: сразу «Сегодня», пароль не спрашивают", async () => {
    setup({ mode: "parent", sessionActive: true });
    expect(await screen.findByRole("heading", { name: "Сегодня" })).toBeTruthy();
    expect(screen.queryByLabelText("Пароль родителя")).toBeNull();
  });

  it("parent без сессии: вход", async () => {
    setup({ mode: "parent" });
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });

  it("reset-password: сброс пароля", async () => {
    setup({ mode: "reset-password" });
    expect(await screen.findByRole("heading", { name: "Сброс пароля родителя" })).toBeTruthy();
  });

  it("uninstall-gate: окно подтверждения удаления", async () => {
    setup({ mode: "uninstall-gate" });
    expect(await screen.findByRole("heading", { name: "Удаление PC Block" })).toBeTruthy();
  });

  it.each([
    ["mystats", "Моя статистика"],
    ["firstrun", "Добро пожаловать"],
  ])("%s: заглушка «появится»", async (mode, title) => {
    setup({ mode });
    expect(await screen.findByRole("heading", { name: title })).toBeTruthy();
    expect(screen.getByText(/появится в следующем обновлении/)).toBeTruthy();
  });

  it("неизвестный режим ведёт себя как обычное окно", async () => {
    setup({ mode: "что-то-новое" });
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });
});

describe("мастер и вход", () => {
  // Ловит: после мастера родителя заставляют второй раз вводить только что заданный пароль.
  it("после мастера «Открыть настройки» входит сразу", async () => {
    const user = setup({ configured: false });
    await user.type(await screen.findByLabelText("Пароль родителя"), "секрет-123");
    await user.type(screen.getByLabelText("Повторите пароль"), "секрет-123");
    await user.click(screen.getByRole("button", { name: "Далее" }));
    await user.click(await screen.findByRole("checkbox", { name: /Миша/ }));
    await user.click(screen.getByRole("button", { name: "Далее" }));
    await user.click(await screen.findByRole("button", { name: "Готово" }));
    await user.click(await screen.findByRole("button", { name: "Открыть настройки" }));
    expect(await screen.findByRole("heading", { name: "Сегодня" })).toBeTruthy();
    expect(mock.count("Login")).toBe(1);
  });

  it("если сразу войти не вышло, просит пароль", async () => {
    const user = setup({ configured: false });
    mock.override("Login", "reject", 1);
    await user.type(await screen.findByLabelText("Пароль родителя"), "секрет-123");
    await user.type(screen.getByLabelText("Повторите пароль"), "секрет-123");
    await user.click(screen.getByRole("button", { name: "Далее" }));
    await user.click(await screen.findByRole("checkbox", { name: /Миша/ }));
    await user.click(screen.getByRole("button", { name: "Далее" }));
    await user.click(await screen.findByRole("button", { name: "Готово" }));
    await user.click(await screen.findByRole("button", { name: "Открыть настройки" }));
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Сегодня" })).toBeNull();
    await login(user);
  });

  it("«Выйти» закрывает сессию и возвращает ко входу", async () => {
    const user = setup();
    await login(user);
    await user.click(screen.getByRole("button", { name: "Выйти" }));
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
    expect(mock.state.sessionActive).toBe(false);
  });
});

describe("меню окна родителя", () => {
  it("«Сегодня», «Дети», «Журнал», «Настройки» и внизу «Помощь»", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    expect(within(nav).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "Сегодня",
      "Дети",
      "Журнал",
      "Настройки",
      "Помощь",
    ]);
    expect(within(nav).getByRole("button", { name: "Сегодня" }).getAttribute("aria-current")).toBe("page");
  });

  it("каждый пункт открывает свой раздел", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });

    await user.click(within(nav).getByRole("button", { name: "Дети" }));
    expect(await screen.findByRole("heading", { name: /Лимиты времени, Миша/ })).toBeTruthy();
    expect(within(nav).getByRole("button", { name: "Дети" }).getAttribute("aria-current")).toBe("page");
    expect(within(nav).getByRole("button", { name: "Сегодня" }).getAttribute("aria-current")).toBeNull();

    await user.click(within(nav).getByRole("button", { name: "Журнал" }));
    expect(await screen.findByRole("heading", { name: "Журнал" })).toBeTruthy();

    await user.click(within(nav).getByRole("button", { name: "Настройки" }));
    expect(await screen.findByRole("heading", { name: "Настройки" })).toBeTruthy();

    await user.click(within(nav).getByRole("button", { name: "Помощь" }));
    expect(await screen.findByRole("heading", { name: "Помощь" })).toBeTruthy();
  });

  it("ссылка «Нарушения сегодня» открывает журнал нарушений", async () => {
    const user = setup({
      events: [
        { atMs: Date.now() - 60_000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind: "bypass", text: "Миша: попытка обойти защиту" },
      ],
    });
    await login(user);
    await user.click(await screen.findByRole("button", { name: "Нарушения сегодня: 1" }));
    expect(await screen.findByRole("heading", { name: "Журнал: нарушения" })).toBeTruthy();
    expect(screen.getByText("Миша: попытка обойти защиту")).toBeTruthy();
  });

  // Ловит: из раздела «Дети» можно уйти, потеряв правки, без вопроса (FR-19).
  it("уход из «Детей» с несохранёнными правками спрашивает «Сохранить изменения?»", async () => {
    const user = setup();
    await login(user);
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await user.click(await screen.findByLabelText("Без лимита, понедельник"));
    await user.click(screen.getByRole("button", { name: "Журнал" }));
    const dialog = screen.getByRole("dialog", { name: "Сохранить изменения?" });
    await user.click(within(dialog).getByRole("button", { name: "Остаться" }));
    expect(screen.getByRole("heading", { name: /Лимиты времени/ })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Журнал" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByRole("heading", { name: "Журнал" })).toBeTruthy();
    expect(mock.count("SaveLimits")).toBe(1);
  });

  it("«Выйти» без связи не оставляет окно открытым: снова просит пароль", async () => {
    const user = setup();
    await login(user);
    mock.override("Logout", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Выйти" }));
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });

  it("«Выйти» тоже спрашивает про несохранённые правки", async () => {
    const user = setup();
    await login(user);
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await user.click(await screen.findByLabelText("Без лимита, понедельник"));
    await user.click(screen.getByRole("button", { name: "Выйти" }));
    expect(screen.getByRole("dialog", { name: "Сохранить изменения?" })).toBeTruthy();
    expect(mock.state.sessionActive).toBe(true);
  });
});

describe("сессия родителя (R19)", () => {
  // Ловит: родитель не узнаёт, что сессия вот-вот кончится, и теряет работу.
  it("за минуту до конца: «Сессия закончится через 1 минуту» и «Продолжить»", async () => {
    const user = setup({ mode: "parent", sessionActive: true, sessionIdleS: 80 });
    await screen.findByRole("heading", { name: "Сегодня" });
    expect(screen.queryByText("Сессия закончится через 1 минуту")).toBeNull();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(screen.getByText("Сессия закончится через 1 минуту")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Продолжить" }));
    await vi.waitFor(() => expect(screen.queryByText("Сессия закончится через 1 минуту")).toBeNull());
    expect(mock.count("ExtendSession")).toBe(1);
  });

  // Ловит: после конца сессии окно остаётся рабочим без пароля, или правки пропадают.
  it("конец сессии: поверх окна ввод пароля, несохранённые правки остаются", async () => {
    const user = setup({ mode: "parent", sessionActive: true, sessionIdleS: 90 });
    await screen.findByRole("heading", { name: "Сегодня" });
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await user.click(await screen.findByLabelText("Без лимита, понедельник"));
    expect((screen.getByLabelText("Без лимита, понедельник") as HTMLInputElement).checked).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(95_000);
    });
    const overlay = await screen.findByRole("dialog", { name: "Сессия закончилась" });
    expect(within(overlay).getByLabelText("Пароль родителя")).toBeTruthy();
    expect(within(overlay).getByText(/несохранённые правки на месте/)).toBeTruthy();

    await user.type(within(overlay).getByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
    await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "Сессия закончилась" })).toBeNull());
    // Правка пережила повторный ввод пароля и её можно сохранить.
    expect((screen.getByLabelText("Без лимита, понедельник") as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Сохранить" }));
    expect(await screen.findByText("Сохранено. Новые правила уже действуют")).toBeTruthy();
  });

  it("после повторного входа сессия снова под присмотром", async () => {
    const user = setup({ mode: "parent", sessionActive: true, sessionIdleS: 70 });
    await screen.findByRole("heading", { name: "Сегодня" });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(75_000);
    });
    const overlay = await screen.findByRole("dialog", { name: "Сессия закончилась" });
    mock.state.sessionIdleS = 600; // новая сессия обычной длины
    await user.type(within(overlay).getByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
    await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "Сессия закончилась" })).toBeNull());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    // До конца новой сессии далеко, предупреждения нет.
    expect(screen.queryByText("Сессия закончится через 1 минуту")).toBeNull();
  });
});
