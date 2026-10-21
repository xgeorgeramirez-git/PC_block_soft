import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Settings from "./Settings";
import GuidanceProvider from "../guidance/GuidanceProvider";
import { HelperEngine } from "../guidance/engine";
import { apiHelperIO } from "../guidance/io";
import { RULES_NOT_READ_TEXT } from "../lib/notices";
import { DEFAULT_PASSWORD, installBackend, mockAccounts, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-25, сценарий 8.3.8: пароль, детские учётки, Помощник, О программе.

let mock: MockBackend;
let openChildren: Mock<() => void>;

async function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ sessionActive: true, ...opts });
  openChildren = vi.fn<() => void>();
  const user = userEvent.setup();
  render(<Settings onOpenChildren={openChildren} />);
  await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
  return user;
}

const section = (name: string) => screen.getByRole("region", { name });

afterEach(() => {
  uninstallBackend();
});

describe("Settings: предупреждения", () => {
  // Ловит: испорченная база (Ruling 25) не видна родителю на экране настроек или показана
  // техническими словами (NFR-11, L-112).
  it("при испорченной базе предупреждение стоит сверху, человеческими словами, с кнопкой в «Дети»", async () => {
    const user = await setup({
      notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
    });
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(RULES_NOT_READ_TEXT);
    expect(alert.textContent).not.toMatch(/баз[аыу] данных|служб/i);
    const top = screen.getByRole("heading", { level: 2, name: "Настройки" });
    expect(top.compareDocumentPosition(alert) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await user.click(within(alert).getByRole("button", { name: "Открыть «Дети»" }));
    expect(openChildren).toHaveBeenCalledTimes(1);
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

  // Ловит: учётка Windows ребёнка не видна, а форма добавления выглядит частью карточки последнего ребёнка.
  it("у каждого ребёнка своя карточка с его учётной записью Windows", async () => {
    await setup({
      children: [
        mockChild({ name: "Мишаня" }),
        mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" }),
      ],
    });
    const list = within(section("Детские учётки")).getByRole("list");
    const cards = within(list).getAllByRole("listitem");
    expect(cards).toHaveLength(2);
    // Имя в программе может отличаться от имени учётки Windows: показываем обе.
    expect(within(cards[0]).getByText("Мишаня")).toBeTruthy();
    expect(within(cards[0]).getByText("Учётная запись Windows: Миша")).toBeTruthy();
    expect(within(cards[1]).getByText("Даша")).toBeTruthy();
    expect(within(cards[1]).getByText("Учётная запись Windows: Даша")).toBeTruthy();
    expect(within(cards[1]).getByRole("button", { name: "Убрать Даша" })).toBeTruthy();
    expect(within(cards[0]).queryByText(/Даша/)).toBeNull();
  });

  it("учётки Windows нет в списке: «не найдена на этом компьютере»", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("ListAccounts", { ok: true, payload: { accounts: [] } });
    render(<Settings />);
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    expect(await screen.findByText("Учётная запись Windows не найдена на этом компьютере")).toBeTruthy();
  });

  it("форма «Добавить» - отдельный блок со своим заголовком под списком детей", async () => {
    await setup({ children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" })], accounts: [
      ...mockAccounts(),
      { sid: "S-1-5-21-111-222-333-1003", name: "Егор", admin: false, child: false, current: false, canBeChild: true },
    ] });
    const box = section("Детские учётки");
    const list = within(box).getByRole("list");
    const heading = within(box).getByRole("heading", { level: 4, name: "Добавить детскую учётку" });
    expect(heading.closest("li")).toBeNull();
    expect(list.contains(heading)).toBe(false);
    expect(list.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const select = within(box).getByLabelText("Учётная запись Windows") as HTMLSelectElement;
    expect(heading.parentElement?.contains(select)).toBe(true);
    expect(list.contains(select)).toBe(false);
    expect(Array.from(select.options).map((o) => o.text)).toEqual(["Егор"]);
  });

  it("добавлять некого: заголовок блока остаётся, под ним объяснение", async () => {
    await setup({ accounts: mockAccounts().filter((a) => a.name !== "Даша") });
    const box = section("Детские учётки");
    expect(within(box).getByRole("heading", { level: 4, name: "Добавить детскую учётку" })).toBeTruthy();
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

  // Ловит: в окне родителя обещание «в следующем обновлении» вместо работающей функции; в M1 только переключатель.
  it("только работающий переключатель, без обещаний на будущее", async () => {
    await setup();
    const box = section("Помощник");
    await vi.waitFor(() => expect(within(box).getByRole("checkbox").hasAttribute("disabled")).toBe(false));
    expect(within(box).getAllByRole("checkbox")).toHaveLength(1);
    expect(within(box).queryByText(/следующем обновлении/)).toBeNull();
    expect(within(box).queryByText(/появится|появятся/)).toBeNull();
    expect(within(box).queryByRole("button")).toBeNull();
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

describe("Settings: Помощник в окне родителя", () => {
  async function setupWithHelper(helper: Record<string, unknown>) {
    mock = installBackend({ sessionActive: true, helper });
    const user = userEvent.setup();
    render(
      <GuidanceProvider engine={new HelperEngine(apiHelperIO)} screen="settings" navigate={() => undefined}>
        <div data-help-anchor="menu-today" />
        <Settings />
      </GuidanceProvider>,
    );
    await vi.waitFor(() => expect(screen.queryByText("Загружаю данные")).toBeNull());
    const box = section("Помощник");
    await vi.waitFor(() => expect(within(box).getByRole("checkbox").hasAttribute("disabled")).toBe(false));
    return { user, box };
  }

  // Ловит: «Вернуть скрытые» стирает и выключатель, и пройденные туры.
  it("«Вернуть скрытые подсказки» забывает скрытое, но не трогает выключатель и туры", async () => {
    const { user, box } = await setupWithHelper({
      enabled: false,
      seen: ["H-M1-02"],
      done: ["H-M1-01"],
      muted: ["D-M1-01:1"],
      tours: ["T-M1-01"],
    });
    await user.click(within(box).getByRole("button", { name: "Вернуть скрытые подсказки" }));
    expect(await within(box).findByText("Скрытые подсказки снова будут появляться.")).toBeTruthy();
    const state = (mock.state.helper ?? {}) as Record<string, unknown>;
    expect(state.enabled).toBe(false);
    expect(state.tours).toEqual(["T-M1-01"]);
    expect(state.seen ?? []).toEqual([]);
    expect(state.done ?? []).toEqual([]);
    expect(state.muted ?? []).toEqual([]);
  });

  it("«Пройти обучение заново» запускает тур", async () => {
    const { user, box } = await setupWithHelper({ enabled: false });
    await user.click(within(box).getByRole("button", { name: "Пройти обучение заново" }));
    expect(await screen.findByRole("dialog", { name: "Короткий тур по окну родителя" })).toBeTruthy();
  });

  // Ловит: выключатель подсказок объясняется так, будто выключает и обучение (S28).
  it("сказано, что обучение в «Помощи» работает и при выключенных подсказках", async () => {
    const { box } = await setupWithHelper({ enabled: false });
    expect(within(box).getByText(/Обучение в разделе «Помощь» доступно всегда/)).toBeTruthy();
  });

  it("выключатель сохраняет состояние целиком и здесь", async () => {
    const { user, box } = await setupWithHelper({ enabled: true, seen: ["H-M1-01"], snoozed: {} });
    await user.click(within(box).getByRole("checkbox", { name: "Показывать подсказки" }));
    await vi.waitFor(() => expect(mock.count("SaveHelperState")).toBeGreaterThan(0));
    const last = mock.calls.filter((c) => c.command === "SaveHelperState").at(-1);
    expect(last?.payload).toMatchObject({ state: { enabled: false, seen: ["H-M1-01"] } });
  });
});

describe("Settings: подробные логи для диагностики (FB-2, D-42)", () => {
  it("переключатель читает состояние и сохраняет новое", async () => {
    const user = await setup();
    const box = section("Подробные логи для диагностики");
    const toggle = within(box).getByRole("checkbox", { name: "Писать подробный технический лог" });
    expect((toggle as HTMLInputElement).checked).toBe(false);
    await user.click(toggle);
    await vi.waitFor(() => expect(mock.state.verboseLogs).toBe(true));
    expect((within(section("Подробные логи для диагностики")).getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
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

