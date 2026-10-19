import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Wizard from "./Wizard";
import { installBackend, mockAccounts, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-1, FR-2, сценарий 8.3.1: мастер из 3 шагов. Ошибки у поля, недоступное объясняет почему.

let mock: MockBackend;
let openSettings: ReturnType<typeof vi.fn>;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ configured: false, mode: "wizard", ...opts });
  openSettings = vi.fn();
  render(<Wizard onOpenSettings={openSettings} />);
  return userEvent.setup();
}

const pw = () => screen.getByLabelText("Пароль родителя") as HTMLInputElement;
const again = () => screen.getByLabelText("Повторите пароль") as HTMLInputElement;
const next = () => screen.getByRole("button", { name: "Далее" }) as HTMLButtonElement;
const hours = (day: string) => screen.getByLabelText(`Часы, ${day}`) as HTMLInputElement;

async function passStep1(user: ReturnType<typeof userEvent.setup>, password = "секрет-123") {
  await user.type(pw(), password);
  await user.type(again(), password);
  await user.click(next());
  await screen.findByText("Шаг 2 из 3");
}

async function passStep2(user: ReturnType<typeof userEvent.setup>, ...names: string[]) {
  for (const name of names) {
    await user.click(await screen.findByRole("checkbox", { name: new RegExp(name) }));
  }
  await user.click(next());
  await screen.findByText("Шаг 3 из 3");
}

afterEach(() => {
  uninstallBackend();
});

describe("Wizard: шаг 1, пароль", () => {
  it("показывает прогресс, два поля пароля и подсказку", () => {
    setup();
    expect(screen.getByText("Шаг 1 из 3")).toBeTruthy();
    expect(pw().type).toBe("password");
    expect(again().type).toBe("password");
    expect(screen.getByText(/Не сообщайте пароль детям/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Назад" })).toBeNull();
  });

  // Ловит: короткий пароль принят или ошибка показана не у поля (L-121).
  it("короткий пароль: «Не меньше 6 символов» у первого поля", async () => {
    const user = setup();
    await user.type(pw(), "123");
    await user.type(again(), "123");
    await user.click(next());
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Не меньше 6 символов");
    expect(pw().getAttribute("aria-invalid")).toBe("true");
    expect(pw().getAttribute("aria-describedby")).toContain(alert.id);
    expect(screen.getByText("Шаг 1 из 3")).toBeTruthy();
  });

  it("пароли разные: «Пароли не совпадают» у второго поля", async () => {
    const user = setup();
    await user.type(pw(), "секрет-123");
    await user.type(again(), "секрет-124");
    await user.click(next());
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Пароли не совпадают");
    expect(again().getAttribute("aria-describedby")).toContain(alert.id);
    expect(pw().getAttribute("aria-invalid")).toBeNull();
  });

  it("Enter в поле идёт дальше", async () => {
    const user = setup();
    await user.type(pw(), "секрет-123");
    await user.type(again(), "секрет-123{Enter}");
    expect(await screen.findByText("Шаг 2 из 3")).toBeTruthy();
  });

  it("ошибка уходит, когда родитель правит поле", async () => {
    const user = setup();
    await user.type(pw(), "123");
    await user.click(next());
    await screen.findByRole("alert");
    await user.type(pw(), "456");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("Wizard: шаг 2, кто дети", () => {
  // Ловит: администратора можно отметить (D-38) или не сказано, почему нельзя (L-17).
  it("администратор недоступен и объяснён, обычные учётки доступны", async () => {
    const user = setup();
    await passStep1(user);
    const admin = await screen.findByRole("checkbox", { name: /Анна/ });
    expect((admin as HTMLInputElement).disabled).toBe(true);
    expect(
      screen.getByText(/Администратор: на такой учётке ограничения не работают\. Сделайте её обычной в Параметрах Windows/),
    ).toBeTruthy();
    expect(screen.getByText(/Это ваша учётная запись/)).toBeTruthy();
    expect((screen.getByRole("checkbox", { name: /Миша/ }) as HTMLInputElement).disabled).toBe(false);
    expect((screen.getByRole("checkbox", { name: /Даша/ }) as HTMLInputElement).disabled).toBe(false);
  });

  // Ловит: можно пройти дальше без ребёнка, или кнопка молча серая (L-17).
  it("нужен хотя бы один ребёнок: «Далее» недоступна и объясняет почему", async () => {
    const user = setup();
    await passStep1(user);
    await screen.findByRole("checkbox", { name: /Миша/ });
    expect(next().disabled).toBe(true);
    const hint = screen.getByText("Отметьте хотя бы одного ребёнка, чтобы продолжить");
    expect(next().getAttribute("aria-describedby")).toContain(hint.id);
    await user.click(screen.getByRole("checkbox", { name: /Миша/ }));
    expect(next().disabled).toBe(false);
    expect(screen.queryByText("Отметьте хотя бы одного ребёнка, чтобы продолжить")).toBeNull();
    await user.click(screen.getByRole("checkbox", { name: /Миша/ }));
    expect(next().disabled).toBe(true);
  });

  it("«Назад» возвращает на первый шаг с введённым паролем", async () => {
    const user = setup();
    await passStep1(user);
    await user.click(screen.getByRole("button", { name: "Назад" }));
    expect(screen.getByText("Шаг 1 из 3")).toBeTruthy();
    expect(pw().value).toBe("секрет-123");
    expect(again().value).toBe("секрет-123");
  });

  it("нет обычных учёток: объясняет, что делать, и даёт обновить список", async () => {
    const onlyAdmin = mockAccounts().filter((a) => a.admin);
    const user = setup({ accounts: onlyAdmin });
    await passStep1(user);
    expect(await screen.findByText(/Обычных учётных записей пока нет/)).toBeTruthy();
    expect(next().disabled).toBe(true);
    const before = mock.count("ListAccounts");
    await user.click(screen.getByRole("button", { name: "Обновить список" }));
    await vi.waitFor(() => expect(mock.count("ListAccounts")).toBe(before + 1));
  });

  it("не удалось прочитать учётки: простыми словами и «Повторить»", async () => {
    const user = setup();
    mock.override("ListAccounts", "reject", 1);
    await passStep1Only(user);
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("checkbox", { name: /Миша/ })).toBeTruthy();
  });
});

async function passStep1Only(user: ReturnType<typeof userEvent.setup>) {
  await user.type(pw(), "секрет-123");
  await user.type(again(), "секрет-123");
  await user.click(next());
}

describe("Wizard: шаг 3, время", () => {
  it("у выбранного ребёнка таблица с шаблоном «будни 2 ч, выходные 4 ч»", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    expect(screen.getByRole("heading", { name: /Миша/ })).toBeTruthy();
    expect(hours("понедельник").value).toBe("2");
    expect(hours("пятница").value).toBe("2");
    expect(hours("суббота").value).toBe("4");
    expect(hours("воскресенье").value).toBe("4");
    expect(screen.getByRole("button", { name: "Готово" })).toBeTruthy();
  });

  it("у каждого ребёнка своя таблица, кнопка шаблона возвращает значения", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша", "Даша");
    expect(screen.getAllByRole("table")).toHaveLength(2);
    const misha = screen.getByRole("region", { name: /Миша/ });
    await user.clear(within(misha).getByLabelText("Часы, понедельник"));
    await user.type(within(misha).getByLabelText("Часы, понедельник"), "5");
    expect((within(misha).getByLabelText("Часы, понедельник") as HTMLInputElement).value).toBe("5");
    await user.click(within(misha).getByRole("button", { name: "Шаблон: будни 2 ч, выходные 4 ч" }));
    expect((within(misha).getByLabelText("Часы, понедельник") as HTMLInputElement).value).toBe("2");
  });

  it("«Назад» с третьего шага сохраняет выбор детей и правки", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.clear(hours("понедельник"));
    await user.type(hours("понедельник"), "3");
    await user.click(screen.getByRole("button", { name: "Назад" }));
    expect((await screen.findByRole("checkbox", { name: /Миша/ }) as HTMLInputElement).checked).toBe(true);
    await user.click(next());
    expect(hours("понедельник").value).toBe("3");
  });

  // Ловит: в программу уходит не тот пароль, не те дети или лимиты с «без лимита» и минутами.
  it("«Готово» отправляет пароль, детей и лимиты, итог говорит, когда начнут действовать правила", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(screen.getByRole("checkbox", { name: "Без лимита, воскресенье" }));
    await user.click(screen.getByRole("button", { name: "Готово" }));

    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
    const sent = mock.calls.find((c) => c.command === "Setup")?.payload;
    expect(sent.password).toBe("секрет-123");
    expect(sent.children).toHaveLength(1);
    expect(sent.children[0].sid).toBe("S-1-5-21-111-222-333-1001");
    expect(sent.children[0].name).toBe("Миша");
    expect(sent.children[0].limits).toEqual([
      { weekday: 1, totalMin: 120, unlimited: false },
      { weekday: 2, totalMin: 120, unlimited: false },
      { weekday: 3, totalMin: 120, unlimited: false },
      { weekday: 4, totalMin: 120, unlimited: false },
      { weekday: 5, totalMin: 120, unlimited: false },
      { weekday: 6, totalMin: 240, unlimited: false },
      { weekday: 7, totalMin: 0, unlimited: true },
    ]);
    expect(mock.state.configured).toBe(true);
    expect(screen.queryByText(/Шаг \d из 3/)).toBeNull();

    await user.click(screen.getByRole("button", { name: "Открыть настройки" }));
    expect(openSettings).toHaveBeenCalledTimes(1);
  });

  it("лимит больше суток: ошибка у дня, мастер не отправляется", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.clear(hours("вторник"));
    await user.type(hours("вторник"), "30");
    await user.click(screen.getByRole("button", { name: "Готово" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Не больше 24 ч");
    expect(mock.count("Setup")).toBe(0);
  });

  // Ловит: ошибка службы про пароль остаётся на третьем шаге, где поля пароля нет.
  it("ошибка службы по паролю возвращает на первый шаг, к полю", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    mock.override("Setup", { ok: false, error: "Не меньше 6 символов", field: "password" }, 1);
    await user.click(screen.getByRole("button", { name: "Готово" }));
    expect(await screen.findByText("Шаг 1 из 3")).toBeTruthy();
    expect((await screen.findByRole("alert")).textContent).toBe("Не меньше 6 символов");
  });

  it("ошибка службы по детям возвращает на второй шаг", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    mock.override("Setup", { ok: false, error: "Эту учётку нельзя сделать детской", field: "children" }, 1);
    await user.click(screen.getByRole("button", { name: "Готово" }));
    expect(await screen.findByText("Шаг 2 из 3")).toBeTruthy();
    expect((await screen.findByRole("alert")).textContent).toBe("Эту учётку нельзя сделать детской");
  });

  it("ошибка службы по дню - у дня на третьем шаге", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    mock.override("Setup", { ok: false, error: "Не больше 24 ч", field: "limits.3" }, 1);
    await user.click(screen.getByRole("button", { name: "Готово" }));
    const row = await screen.findByRole("row", { name: /Среда/ });
    expect(within(row).getByRole("alert").textContent).toBe("Не больше 24 ч");
    expect(screen.getByText("Шаг 3 из 3")).toBeTruthy();
  });

  it("нет связи: простыми словами, можно нажать «Готово» ещё раз", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    mock.override("Setup", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Готово" }));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Готово" }));
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
  });
});
