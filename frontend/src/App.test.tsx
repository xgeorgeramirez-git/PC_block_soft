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

  // Ловит (FR-4): выбор в окне удаления не доходит до установщика: код выхода остаётся пустым,
  // и галочка «Удалить статистику» ничего не меняет. 0 - удалить и оставить статистику, 2 - вместе с ней.
  it.each([
    ["без галочки", false, 0],
    ["с галочкой «Удалить статистику»", true, 2],
  ])("uninstall-gate %s: окно отдаёт установщику код %d", async (_name, tick, code) => {
    const user = setup({ mode: "uninstall-gate", configured: false });
    await screen.findByText(/пароль не нужен/);
    expect(mock.state.finishCode).toBeNull();
    if (tick) {
      await user.click(screen.getByRole("checkbox", { name: "Удалить статистику" }));
    }
    await user.click(screen.getByRole("button", { name: "Продолжить удаление" }));
    await vi.waitFor(() => expect(mock.state.finishCode).toBe(code));
  });

  // Ловит: отказ службы (неверный пароль) закрывает окно кодом «удалять»: пароль ничего не защищает.
  it("uninstall-gate: неверный пароль не завершает окно", async () => {
    const user = setup({ mode: "uninstall-gate" });
    await user.type(await screen.findByLabelText("Пароль родителя"), "не-тот-пароль{Enter}");
    await screen.findByText(/Неверный пароль/);
    expect(mock.state.finishCode).toBeNull();
  });

  // Ловит: окно ребёнка осталось заглушкой «появится в следующем обновлении» (FR-12, FR-32).
  it.each([
    ["mystats", "Моя статистика", "Только просмотр"],
    ["firstrun", "Теперь время за компьютером считается", "Понятно"],
  ])("%s: настоящий экран ребёнка без входа родителя", async (mode, title, mark) => {
    setup({ mode });
    expect(await screen.findByRole("heading", { name: title })).toBeTruthy();
    expect(screen.getAllByText(mark).length).toBeGreaterThan(0);
    expect(screen.queryByText(/появится в следующем обновлении/)).toBeNull();
    expect(screen.queryByLabelText("Пароль родителя")).toBeNull();
    // Ребёнку не пишут «Родительский контроль» в заголовке окна и не показывают меню родителя.
    expect(screen.queryByText("Родительский контроль")).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Разделы" })).toBeNull();
  });

  it("неизвестный режим ведёт себя как обычное окно", async () => {
    setup({ mode: "что-то-новое" });
    expect(await screen.findByLabelText("Пароль родителя")).toBeTruthy();
  });
});

describe("окно с плашки «Попросить сейчас» (RM3-4)", () => {
  // Ловит: окно стража открывается на «Сегодня», родителю приходится искать программу.
  it("parent с открытой сессией и экраном programs: сразу «Программы»", async () => {
    setup({ mode: "parent", sessionActive: true, startScreen: "programs" });
    expect(await screen.findByRole("heading", { name: "Программы" })).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Ждут решения: 1" })).toBeTruthy();
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    expect(within(nav).getByRole("button", { name: "Программы" }).getAttribute("aria-current")).toBe("page");
    // Заголовок окна тот же, ничего лишнего.
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("PC Block");
  });

  it("сессии нет: сначала пароль, потом сразу «Программы»", async () => {
    const user = setup({ mode: "parent", startScreen: "programs" });
    await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
    expect(await screen.findByRole("heading", { name: "Программы" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Сегодня" })).toBeNull();
  });

  it("после «Выйти» и нового входа окно снова на «Сегодня»", async () => {
    const user = setup({ mode: "parent", sessionActive: true, startScreen: "programs" });
    await screen.findByRole("heading", { name: "Программы" });
    await user.click(screen.getByRole("button", { name: "Выйти" }));
    await user.type(await screen.findByLabelText("Пароль родителя"), `${DEFAULT_PASSWORD}{Enter}`);
    expect(await screen.findByRole("heading", { name: "Сегодня" })).toBeTruthy();
  });

  it("без экрана запуска: «Сегодня»", async () => {
    setup({ mode: "parent", sessionActive: true });
    expect(await screen.findByRole("heading", { name: "Сегодня" })).toBeTruthy();
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
  it("«Сегодня», «Дети», «Программы», «Копилка», «Статистика», «Журнал», «Настройки» и внизу «Что нового»", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    expect(within(nav).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "Сегодня",
      "Дети",
      "Программы",
      "Копилка",
      "Статистика",
      "Журнал",
      "Настройки",
      "Что нового",
    ]);
    expect(within(nav).getByRole("button", { name: "Сегодня" }).getAttribute("aria-current")).toBe("page");
  });

  it("каждый пункт открывает свой раздел", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });

    await user.click(within(nav).getByRole("button", { name: "Дети" }));
    expect(await screen.findByRole("heading", { name: /Правила, Миша/ })).toBeTruthy();
    expect(within(nav).getByRole("button", { name: "Дети" }).getAttribute("aria-current")).toBe("page");
    expect(within(nav).getByRole("button", { name: "Сегодня" }).getAttribute("aria-current")).toBeNull();

    await user.click(within(nav).getByRole("button", { name: "Программы" }));
    expect(await screen.findByRole("heading", { name: "Программы" })).toBeTruthy();
    expect(within(nav).getByRole("button", { name: "Программы" }).getAttribute("aria-current")).toBe("page");

    await user.click(within(nav).getByRole("button", { name: "Статистика" }));
    expect(await screen.findByRole("heading", { name: "Статистика" })).toBeTruthy();

    await user.click(within(nav).getByRole("button", { name: "Журнал" }));
    expect(await screen.findByRole("heading", { name: "Журнал" })).toBeTruthy();

    await user.click(within(nav).getByRole("button", { name: "Настройки" }));
    expect(await screen.findByRole("heading", { name: "Настройки" })).toBeTruthy();

    await user.click(within(nav).getByRole("button", { name: "Что нового" }));
    expect(await screen.findByRole("heading", { name: "Что нового" })).toBeTruthy();
  });

  // Ловит: «Что нового» стоит в основной группе разделов, а не отдельно внизу меню (ТЗ 8.3.2 п.2).
  it("«Что нового» внизу меню, отдельно от разделов, и открывает список с новой версией сверху", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    const list = within(nav).getByRole("list");
    expect(within(list).queryByRole("button", { name: "Что нового" })).toBeNull();
    const item = within(nav).getByRole("button", { name: "Что нового" });
    expect(within(nav).getAllByRole("button").at(-1)).toBe(item);

    await user.click(item);
    expect(item.getAttribute("aria-current")).toBe("page");
    const versions = (await screen.findAllByRole("heading", { level: 3 })).map((h) => h.textContent);
    expect(versions[0]).toBe("Версия 0.2.5. Предупреждения с табло и звуком");
    expect(screen.queryByRole("heading", { name: "Помощь" })).toBeNull();
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

  // Ловит: ссылка ведёт не туда или счётчик на «Сегодня» расходится с «Программами» (8.3.2 п.5).
  it("ссылка «Ждут решения: 1» открывает «Программы» с тем же счётчиком", async () => {
    const user = setup();
    await login(user);
    await user.click(await screen.findByRole("button", { name: "Ждут решения: 1" }));
    expect(await screen.findByRole("heading", { name: "Ждут решения: 1" })).toBeTruthy();
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    expect(within(nav).getByRole("button", { name: "Программы" }).getAttribute("aria-current")).toBe("page");
  });

  // Ловит: после решения на «Программах» «Сегодня» показывает старый счётчик.
  it("после решения на «Программах» ссылки «Ждут решения» на «Сегодня» нет", async () => {
    const user = setup();
    await login(user);
    const nav = screen.getByRole("navigation", { name: "Разделы" });
    await user.click(within(nav).getByRole("button", { name: "Программы" }));
    await screen.findByRole("heading", { name: "Ждут решения: 1" });
    await user.click(screen.getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    await user.click(within(nav).getByRole("button", { name: "Сегодня" }));
    await screen.findByRole("article", { name: "Миша" });
    expect(screen.queryByRole("button", { name: /^Ждут решения/ })).toBeNull();
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
    expect(screen.getByRole("heading", { name: /Правила/ })).toBeTruthy();

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

describe("окно: предупреждение о правилах и нижняя панель", () => {
  // Ловит: предупреждение о нечитаемых правилах (R18) без следующего шага: родитель не знает, куда идти.
  it("кнопка «Открыть «Дети»» у предупреждения ведёт к лимитам", async () => {
    const user = setup({
      mode: "parent",
      sessionActive: true,
      notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
    });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).not.toMatch(/баз[аыу] данных|служб/i);
    await user.click(within(alert).getByRole("button", { name: "Открыть «Дети»" }));
    expect(await screen.findByRole("heading", { name: "Правила, Миша" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Дети" }).getAttribute("aria-current")).toBe("page");
  });

  it("то же предупреждение в «Настройках» тоже ведёт в «Дети»", async () => {
    const user = setup({
      mode: "parent",
      sessionActive: true,
      notices: ["База данных была повреждена и восстановлена"],
    });
    await screen.findByRole("heading", { name: "Сегодня" });
    await user.click(screen.getByRole("button", { name: "Настройки" }));
    const alert = await screen.findByRole("alert");
    await user.click(within(alert).getByRole("button", { name: "Открыть «Дети»" }));
    expect(await screen.findByRole("heading", { name: "Правила, Миша" })).toBeTruthy();
  });

  // Ловит: панель «Сохранить» лежит внутри прокручиваемой страницы и накрывает строки таблицы (FR-19).
  it("панель сохранения стоит внизу окна, вне прокручиваемой страницы", async () => {
    const user = setup({ mode: "parent", sessionActive: true });
    await screen.findByRole("heading", { name: "Сегодня" });
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await user.click(await screen.findByLabelText("Без лимита, понедельник"));
    const bar = screen.getByRole("region", { name: "Сохранение изменений" });
    expect(bar.closest("main")).toBeNull();
    expect(bar.parentElement?.classList.contains("savebar-slot")).toBe(true);
    // Слот идёт после страницы: панель ниже таблицы и всегда у нижнего края.
    const main = document.querySelector("main.content")!;
    expect(main.compareDocumentPosition(bar) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // Одна кнопка «Сохранить» на всё окно.
    expect(screen.getAllByRole("button", { name: "Сохранить" })).toHaveLength(1);
    await user.click(within(bar).getByRole("button", { name: "Сохранить" }));
    const saved = await screen.findByText("Сохранено. Новые правила уже действуют");
    expect(saved.closest(".savebar-slot")).toBeTruthy();
  });

  it("при уходе с несохранёнными правками панель остаётся честной: вопрос и выбор", async () => {
    const user = setup({ mode: "parent", sessionActive: true });
    await screen.findByRole("heading", { name: "Сегодня" });
    await user.click(screen.getByRole("button", { name: "Дети" }));
    await user.click(await screen.findByLabelText("Без лимита, понедельник"));
    await user.click(screen.getByRole("button", { name: "Журнал" }));
    expect(await screen.findByRole("dialog", { name: "Сохранить изменения?" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Не сохранять" }));
    await vi.waitFor(() => expect(screen.queryByRole("region", { name: "Сохранение изменений" })).toBeNull());
  });
});
