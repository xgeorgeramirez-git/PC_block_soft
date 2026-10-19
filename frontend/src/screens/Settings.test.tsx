import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Settings from "./Settings";
import { DEFAULT_PASSWORD, installBackend, mockAccounts, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-25, сценарий 8.3.8: пароль, детские учётки, Помощник, О программе.

let mock: MockBackend;

async function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ sessionActive: true, ...opts });
  const user = userEvent.setup();
  render(<Settings />);
  await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
  return user;
}

const section = (name: string) => screen.getByRole("region", { name });

afterEach(() => {
  uninstallBackend();
});

describe("Settings: предупреждения", () => {
  // Ловит: испорченная база (Ruling 25) не видна родителю на экране настроек.
  it("при испорченной базе предупреждение стоит сверху", async () => {
    await setup({ notices: ["База данных была повреждена и восстановлена. Ограничения не действовали"] });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/База данных была повреждена и восстановлена/);
    const top = screen.getByRole("heading", { level: 2, name: "Настройки" });
    expect(top.compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("без предупреждений тишина", async () => {
    await setup();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("Settings: сменить пароль", () => {
  const old = () => screen.getByLabelText("Старый пароль");
  const fresh = () => screen.getByLabelText("Новый пароль");
  const again = () => screen.getByLabelText("Повторите новый пароль");
  const submit = () => within(section("Сменить пароль")).getByRole("button", { name: "Сменить пароль" });

  it("три поля пароля", async () => {
    await setup();
    for (const f of [old(), fresh(), again()]) {
      expect((f as HTMLInputElement).type).toBe("password");
    }
  });

  it("ошибки у полей: пустой старый, короткий новый, несовпадение", async () => {
    const user = await setup();
    await user.click(submit());
    expect(within(section("Сменить пароль")).getAllByRole("alert").map((a) => a.textContent)).toEqual([
      "Введите старый пароль",
      "Не меньше 6 символов",
    ]);

    await user.type(old(), DEFAULT_PASSWORD);
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет-2");
    await user.click(submit());
    const alerts = within(section("Сменить пароль")).getAllByRole("alert");
    expect(alerts.map((a) => a.textContent)).toEqual(["Пароли не совпадают"]);
    expect(again().getAttribute("aria-describedby")).toContain(alerts[0].id);
    expect(mock.count("ChangePassword")).toBe(0);
  });

  // Ловит: ошибка службы про старый пароль показана не у его поля.
  it("служба: неверный старый пароль - у поля старого пароля", async () => {
    const user = await setup();
    await user.type(old(), "не тот");
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    const alert = await within(section("Сменить пароль")).findByRole("alert");
    expect(alert.textContent).toBe("Старый пароль неверный");
    expect(old().getAttribute("aria-describedby")).toContain(alert.id);
  });

  it("успех: «Пароль изменён», поля очищены, новый пароль действует", async () => {
    const user = await setup();
    await user.type(old(), DEFAULT_PASSWORD);
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    expect(await screen.findByText("Пароль изменён")).toBeTruthy();
    expect((old() as HTMLInputElement).value).toBe("");
    expect((fresh() as HTMLInputElement).value).toBe("");
    expect(mock.calls.find((c) => c.command === "ChangePassword")?.payload).toEqual({
      oldPassword: DEFAULT_PASSWORD,
      newPassword: "новый-секрет",
    });
    expect(mock.state.password).toBe("новый-секрет");
  });

  it("нет связи: простыми словами, поля остаются", async () => {
    const user = await setup();
    mock.override("ChangePassword", "reject", 1);
    await user.type(old(), DEFAULT_PASSWORD);
    await user.type(fresh(), "новый-секрет");
    await user.type(again(), "новый-секрет");
    await user.click(submit());
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect((fresh() as HTMLInputElement).value).toBe("новый-секрет");
  });
});

describe("Settings: детские учётки", () => {
  it("список детей и форма добавления", async () => {
    await setup();
    const sec = section("Детские учётки");
    expect(within(sec).getByText("Миша")).toBeTruthy();
    expect(within(sec).getByRole("button", { name: "Переименовать Миша" })).toBeTruthy();
    expect(within(sec).getByRole("button", { name: "Убрать Миша" })).toBeTruthy();
  });

  // Ловит: «Убрать» срабатывает сразу, без подтверждения (L-74: необратимое - подтверждение).
  it("убрать: подтверждение с текстом из ТЗ, потом убирает", async () => {
    const user = await setup();
    await user.click(screen.getByRole("button", { name: "Убрать Миша" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Ограничения для Миши перестанут действовать. Статистика сохранится. Убрать?")).toBeTruthy();
    expect(mock.count("SetChildren")).toBe(0);
    await user.click(within(dialog).getByRole("button", { name: "Убрать" }));
    await screen.findByText("Детей пока нет");
    expect(mock.calls.find((c) => c.command === "SetChildren")?.payload).toEqual({ add: [], remove: [1], rename: [] });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText("Убрано: Миша. Статистика сохранена")).toBeTruthy();
  });

  it("«Отмена» и Esc ничего не убирают", async () => {
    const user = await setup();
    await user.click(screen.getByRole("button", { name: "Убрать Миша" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Убрать Миша" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("SetChildren")).toBe(0);
    expect(within(section("Детские учётки")).getByText("Миша")).toBeTruthy();
  });

  it("переименовать", async () => {
    const user = await setup();
    await user.click(screen.getByRole("button", { name: "Переименовать Миша" }));
    const input = screen.getByLabelText("Новое имя");
    await user.clear(input);
    await user.type(input, "Михаил");
    await user.click(screen.getByRole("button", { name: "Сохранить имя" }));
    expect(await within(section("Детские учётки")).findByText("Михаил")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "SetChildren")?.payload).toEqual({
      add: [],
      remove: [],
      rename: [{ childId: 1, name: "Михаил" }],
    });
  });

  it("пустое имя - ошибка у поля, в программу не уходит", async () => {
    const user = await setup();
    await user.click(screen.getByRole("button", { name: "Переименовать Миша" }));
    await user.clear(screen.getByLabelText("Новое имя"));
    await user.click(screen.getByRole("button", { name: "Сохранить имя" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Введите имя");
    expect(mock.count("SetChildren")).toBe(0);
    await user.click(screen.getByRole("button", { name: "Отмена" }));
    expect(screen.queryByLabelText("Новое имя")).toBeNull();
  });

  // Ловит: в список для добавления попали администратор или уже добавленный ребёнок (A2, D-38).
  it("добавить можно только обычную учётку, которой ещё нет в детях", async () => {
    const user = await setup();
    const select = screen.getByLabelText("Учётная запись Windows") as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.text)).toEqual(["Даша"]);
    await user.click(screen.getByRole("button", { name: "Добавить ребёнка" }));
    expect(await within(section("Детские учётки")).findByText("Даша")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "SetChildren")?.payload).toEqual({
      add: [{ sid: "S-1-5-21-111-222-333-1002", name: "Даша" }],
      remove: [],
      rename: [],
    });
    expect(await screen.findByText("Добавлено: Даша. Задайте лимиты в разделе «Дети»")).toBeTruthy();
  });

  it("добавлять некого: объясняет про администратора и даёт обновить список", async () => {
    const accounts = mockAccounts().filter((a) => a.name !== "Даша");
    const user = await setup({ accounts });
    expect(screen.getByText(/Подходящих обычных учётных записей нет/)).toBeTruthy();
    expect(screen.getByText(/Администратора добавить нельзя: на такой учётке ограничения не работают/)).toBeTruthy();
    const before = mock.count("ListAccounts");
    await user.click(screen.getByRole("button", { name: "Обновить список" }));
    await screen.findByText(/Подходящих обычных учётных записей нет/);
    expect(mock.count("ListAccounts")).toBe(before + 1);
  });

  it("ошибка службы при добавлении - у формы, простыми словами", async () => {
    const user = await setup();
    mock.override("SetChildren", { ok: false, error: "Эту учётку нельзя сделать детской", field: "children" }, 1);
    await user.click(screen.getByRole("button", { name: "Добавить ребёнка" }));
    expect((await within(section("Детские учётки")).findByRole("alert")).textContent).toBe(
      "Эту учётку нельзя сделать детской",
    );
  });

  it("детей несколько - у каждого свои кнопки", async () => {
    await setup({ children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" })] });
    expect(screen.getByRole("button", { name: "Убрать Даша" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Убрать Миша" })).toBeTruthy();
  });
});

describe("Settings: Помощник", () => {
  it("по умолчанию подсказки включены", async () => {
    await setup();
    const toggle = within(section("Помощник")).getByRole("checkbox", { name: "Показывать подсказки" }) as HTMLInputElement;
    await vi.waitFor(() => expect(toggle.disabled).toBe(false));
    expect(screen.getByText(/Подсказки при первом открытии экранов/)).toBeTruthy();
    expect(toggle.checked).toBe(true);
  });

  it("выключено в сохранённом состоянии - переключатель выключен", async () => {
    await setup({ helper: { enabled: false } });
    const toggle = within(section("Помощник")).getByRole("checkbox", { name: "Показывать подсказки" }) as HTMLInputElement;
    await vi.waitFor(() => expect(toggle.checked).toBe(false));
    expect(toggle.disabled).toBe(false);
  });

  // Ловит: переключатель затирает остальное состояние Помощника (прочитанные подсказки).
  it("переключение сохраняет состояние целиком, меняя только «включено»", async () => {
    const user = await setup({ helper: { enabled: true, seen: ["H-M1-01"], snoozed: {} } });
    const toggle = within(section("Помощник")).getByRole("checkbox", { name: "Показывать подсказки" }) as HTMLInputElement;
    await vi.waitFor(() => expect(toggle.disabled).toBe(false));
    await user.click(toggle);
    await vi.waitFor(() => expect(mock.count("SaveHelperState")).toBe(1));
    expect(mock.calls.find((c) => c.command === "SaveHelperState")?.payload).toEqual({
      state: { enabled: false, seen: ["H-M1-01"], snoozed: {} },
    });
    expect(toggle.checked).toBe(false);
  });

  it("не сохранилось: переключатель возвращается, причина простыми словами", async () => {
    const user = await setup({ helper: { enabled: true } });
    const toggle = within(section("Помощник")).getByRole("checkbox", { name: "Показывать подсказки" }) as HTMLInputElement;
    await vi.waitFor(() => expect(toggle.disabled).toBe(false));
    mock.override("SaveHelperState", "reject", 1);
    await user.click(toggle);
    expect(await within(section("Помощник")).findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(toggle.checked).toBe(true);
  });
});

describe("Settings: о программе", () => {
  it("версия приходит от программы", async () => {
    await setup({ version: "0.4.2" });
    expect(await within(section("О программе")).findByText("Версия 0.4.2")).toBeTruthy();
    expect(within(section("О программе")).getByText("Данные хранятся только на этом компьютере")).toBeTruthy();
  });

  it("версию узнать не удалось - так и говорит", async () => {
    mock = installBackend({ sessionActive: true });
    mock.state.linkDown = false;
    window.go!.ui!.App!.Version = () => Promise.reject(new Error("window.go is undefined"));
    render(<Settings />);
    expect(await screen.findByText("Версию узнать не удалось")).toBeTruthy();
  });
});

