import { afterEach, describe, expect, it, vi, type Mock } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Wizard from "./Wizard";
import { installBackend, mockAccounts, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-1, FR-2, сценарий 8.3.1: мастер из 4 шагов (пароль, дети, время, программы). Ошибки у поля, недоступное объясняет почему.

let mock: MockBackend;
let openSettings: Mock<(password: string) => void>;

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ configured: false, mode: "wizard", ...opts });
  openSettings = vi.fn<(password: string) => void>();
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
  await screen.findByText("Шаг 2 из 4");
}

async function passStep2(user: ReturnType<typeof userEvent.setup>, ...names: string[]) {
  for (const name of names) {
    await user.click(await screen.findByRole("checkbox", { name: new RegExp(name) }));
  }
  await user.click(next());
  await screen.findByText("Шаг 3 из 4");
}

/** С третьего шага на «Программы»: ждёт, пока список найден. */
async function passStep3(user: ReturnType<typeof userEvent.setup>) {
  await user.click(next());
  await screen.findByText("Шаг 4 из 4");
  await screen.findByRole("table", { name: "Программы на этом компьютере" });
}

const done = () => screen.getByRole("button", { name: "Готово" });

afterEach(() => {
  uninstallBackend();
});

describe("Wizard: шаг 1, пароль", () => {
  it("показывает прогресс, два поля пароля и подсказку", () => {
    setup();
    expect(screen.getByText("Шаг 1 из 4")).toBeTruthy();
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
    expect(screen.getByText("Шаг 1 из 4")).toBeTruthy();
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
    expect(await screen.findByText("Шаг 2 из 4")).toBeTruthy();
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
    expect(screen.getByText("Шаг 1 из 4")).toBeTruthy();
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
  it("у выбранного ребёнка таблица с шаблоном «будни 2 ч, выходные 4 ч», игры без лимита", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    expect(screen.getByRole("heading", { name: /Миша/ })).toBeTruthy();
    expect(hours("понедельник").value).toBe("2");
    expect(hours("пятница").value).toBe("2");
    expect(hours("суббота").value).toBe("4");
    expect(hours("воскресенье").value).toBe("4");
    expect((screen.getByLabelText("Игры без лимита, понедельник") as HTMLInputElement).checked).toBe(true);
    expect(screen.getByRole("columnheader", { name: "Игры" })).toBeTruthy();
    expect(screen.getByText("Время игр входит в общее. Учёба в общее время не входит")).toBeTruthy();
    expect(next()).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Готово" })).toBeNull();
  });

  it("у каждого ребёнка своя таблица, кнопка шаблона возвращает значения", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша", "Даша");
    expect(screen.getAllByRole("table")).toHaveLength(2);
    const misha = screen.getByRole("region", { name: /Миша/ });
    await user.clear(within(misha).getByLabelText("Часы, понедельник"));
    await user.type(within(misha).getByLabelText("Часы, понедельник"), "5");
    await user.click(within(misha).getByLabelText("Игры без лимита, понедельник"));
    expect((within(misha).getByLabelText("Часы, понедельник") as HTMLInputElement).value).toBe("5");
    await user.click(within(misha).getByRole("button", { name: "Шаблон: будни 2 ч, выходные 4 ч" }));
    expect((within(misha).getByLabelText("Часы, понедельник") as HTMLInputElement).value).toBe("2");
    expect((within(misha).getByLabelText("Игры без лимита, понедельник") as HTMLInputElement).checked).toBe(true);
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

  // D-45. Ловит: в мастере можно набрать лимит больше суток.
  it("лимит больше суток набрать нельзя: поле часов держит 23", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.clear(hours("вторник"));
    await user.type(hours("вторник"), "30");
    expect(hours("вторник").value).toBe("23");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("«Далее» ведёт на шаг «Программы», прогресс 4 из 4", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    expect(await screen.findByText("Шаг 4 из 4")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Программы" })).toBeTruthy();
    expect(mock.count("Setup")).toBe(0);
  });
});

describe("Wizard: шаг 4, программы (FR-2)", () => {
  const pick = (name: string) => screen.getByRole("combobox", { name: `Что это: ${name}` }) as HTMLSelectElement;

  it("пока ищет - «Ищу установленные программы», потом список с издателем и предложенной категорией", async () => {
    const user = setup();
    let release: () => void = () => undefined;
    mock.override("ScanPrograms", () => new Promise((resolve) => {
      release = () =>
        resolve({
          ok: true,
          payload: {
            programs: [
              { path: "c:\\program files\\minecraft launcher\\minecraftlauncher.exe", name: "Minecraft Launcher", publisher: "Mojang", suggestedCategory: "games" },
              { path: "c:\\program files\\geo\\geo.exe", name: "ГеоКласс", publisher: "", suggestedCategory: "study" },
              { path: "c:\\program files\\bravesoftware\\brave.exe", name: "Brave", publisher: "Brave Software, Inc.", suggestedCategory: "other" },
              { path: "c:\\program files\\x\\x.exe", name: "Странная", publisher: "X", suggestedCategory: "blocked" },
            ],
          },
        });
    }), 1);
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    expect((await screen.findByRole("status")).textContent).toBe("Ищу установленные программы");
    release();
    await screen.findByRole("table", { name: "Программы на этом компьютере" });
    expect(screen.queryByText("Ищу установленные программы")).toBeNull();
    expect(screen.getByText("Mojang")).toBeTruthy();
    expect(screen.getByText("Без подписи издателя")).toBeTruthy();
    expect(pick("Minecraft Launcher").value).toBe("games");
    expect(pick("ГеоКласс").value).toBe("study");
    expect(pick("Brave").value).toBe("other");
    // Неподходящая для мастера предложенная категория - «Прочее».
    expect(pick("Странная").value).toBe("other");
    expect(Array.from(pick("Brave").options).map((o) => o.textContent)).toEqual(["Игра", "Учёба", "Прочее"]);
    expect(screen.getByText(/Незнакомые программы будут ждать вашего решения\. Это можно изменить позже в разделе «Программы»/)).toBeTruthy();
  });

  // Ловит: корявая фраза шага «какие программы игры, а какие для учёбы» (приёмка P2 п.4).
  it("пояснение шага: какие программы игровые, а какие нужны для учёбы", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    const text = screen.getByText(/^Отметьте, какие программы/);
    expect(text.textContent).toBe(
      "Отметьте, какие программы игровые, а какие нужны для учёбы. Незнакомые программы будут ждать вашего решения. Это можно изменить позже в разделе «Программы».",
    );
  });

  // Ловит: в программу уходит «Прочее» вместо выбранной категории или лишние неизменённые отметки.
  it("«Готово» отправляет только изменённые отметки с выбранной категорией", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    await user.selectOptions(pick("Brave"), "study");
    await user.click(done());
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
    const sent = mock.calls.find((c) => c.command === "Setup")?.payload;
    expect(sent.programs).toEqual([{ path: "c:\\program files\\bravesoftware\\brave-browser\\application\\brave.exe", category: "study" }]);
  });

  it("смена категории предложенной игры на «Прочее» тоже отправляется", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    await user.selectOptions(pick("Minecraft Launcher"), "other");
    await user.click(done());
    await screen.findByText("Готово. Правила начнут действовать при следующем входе детей");
    const sent = mock.calls.find((c) => c.command === "Setup")?.payload;
    expect(sent.programs).toEqual([{ path: "c:\\program files\\minecraft launcher\\minecraftlauncher.exe", category: "other" }]);
  });

  // Ловит: в программу уходит не тот пароль, не те дети или лимиты с «без лимита» и минутами.
  it("«Готово» без правок: пароль, дети и лимиты, программ нет, итог прежний", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(screen.getByRole("checkbox", { name: "Без лимита, воскресенье" }));
    await user.click(screen.getByLabelText("Игры без лимита, понедельник"));
    await passStep3(user);
    await user.click(done());

    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
    const sent = mock.calls.find((c) => c.command === "Setup")?.payload;
    expect(sent.password).toBe("секрет-123");
    expect(sent.children).toHaveLength(1);
    expect(sent.children[0].sid).toBe("S-1-5-21-111-222-333-1001");
    expect(sent.children[0].name).toBe("Миша");
    expect(sent.children[0].limits).toEqual([
      { weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false },
      { weekday: 2, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 3, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 4, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 5, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 6, totalMin: 240, unlimited: false, gamesMin: 0, gamesUnlimited: true },
      { weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true },
    ]);
    expect(sent.programs).toBeUndefined();
    expect(sent.children[0].endMode).toBeUndefined();
    expect(mock.state.configured).toBe(true);
    expect(screen.queryByText(/Шаг \d из 4/)).toBeNull();

    await user.click(screen.getByRole("button", { name: "Открыть настройки" }));
    expect(openSettings).toHaveBeenCalledTimes(1);
    expect(openSettings).toHaveBeenCalledWith("секрет-123");
  });

  it("«Настрою позже» настраивает без программ, даже если отметки менялись", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    await user.selectOptions(pick("Brave"), "games");
    await user.click(screen.getByRole("button", { name: "Настрою позже" }));
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "Setup")?.payload.programs).toBeUndefined();
  });

  it("«Настрою позже» доступна, пока программы ищутся", async () => {
    const user = setup();
    mock.override("ScanPrograms", () => new Promise(() => undefined), 1);
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    await screen.findByText("Ищу установленные программы");
    await user.click(screen.getByRole("button", { name: "Настрою позже" }));
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
  });

  // Review Focus 4: после ошибки шаг можно повторить, «Настрою позже» есть всегда.
  it("программы не нашлись: текст службы, «Повторить» и «Настрою позже»", async () => {
    const user = setup();
    mock.override("ScanPrograms", { ok: false, error: "Не удалось найти программы. Их можно настроить позже в разделе «Программы»" }, 1);
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    expect(await screen.findByText("Не удалось найти программы. Их можно настроить позже в разделе «Программы»")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Настрою позже" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("table", { name: "Программы на этом компьютере" })).toBeTruthy();
    expect(mock.count("ScanPrograms")).toBe(2);
  });

  it("нет связи при поиске: простыми словами и «Повторить»", async () => {
    const user = setup();
    mock.override("ScanPrograms", "reject", 1);
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("table", { name: "Программы на этом компьютере" })).toBeTruthy();
  });

  it("пустой список: «Программы не найдены» и можно закончить", async () => {
    const user = setup();
    mock.override("ScanPrograms", { ok: true, payload: { programs: [] } }, 1);
    await passStep1(user);
    await passStep2(user, "Миша");
    await user.click(next());
    expect(await screen.findByText("Программы не найдены. Их можно настроить позже в разделе «Программы»")).toBeTruthy();
    await user.click(done());
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "Setup")?.payload.programs).toBeUndefined();
  });

  // Review Focus 4: «Назад» не теряет отметок и не ищет программы заново.
  it("«Назад» и снова «Далее» сохраняют отметки, поиск один", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    await user.selectOptions(pick("Brave"), "study");
    await user.click(screen.getByRole("button", { name: "Назад" }));
    expect(await screen.findByText("Шаг 3 из 4")).toBeTruthy();
    expect(hours("понедельник").value).toBe("2");
    await user.click(next());
    expect(pick("Brave").value).toBe("study");
    expect(mock.count("ScanPrograms")).toBe(1);
  });

  // Ловит: ошибка службы про пароль остаётся на последнем шаге, где поля пароля нет.
  it("ошибка службы по паролю возвращает на первый шаг, к полю", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", { ok: false, error: "Не меньше 6 символов", field: "password" }, 1);
    await user.click(done());
    expect(await screen.findByText("Шаг 1 из 4")).toBeTruthy();
    expect((await screen.findByRole("alert")).textContent).toBe("Не меньше 6 символов");
  });

  it("ошибка службы по детям возвращает на второй шаг", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", { ok: false, error: "Эту учётку нельзя сделать детской", field: "children" }, 1);
    await user.click(done());
    expect(await screen.findByText("Шаг 2 из 4")).toBeTruthy();
    expect((await screen.findByRole("alert")).textContent).toBe("Эту учётку нельзя сделать детской");
  });

  it("ошибка службы по дню возвращает на третий шаг, к дню", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", { ok: false, error: "Не больше 24 ч", field: "limits.3" }, 1);
    await user.click(done());
    const row = await screen.findByRole("row", { name: /Среда/ });
    expect(within(row).getByRole("alert").textContent).toBe("Не больше 24 ч");
    expect(screen.getByText("Шаг 3 из 4")).toBeTruthy();
  });

  it("ошибка службы по играм дня - у игр на третьем шаге", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", { ok: false, error: "Не больше 24 ч", field: "games.5" }, 1);
    await user.click(done());
    const row = await screen.findByRole("row", { name: /Пятница/ });
    expect(within(row).getByRole("alert").closest("td")?.contains(within(row).getByLabelText("Часы игр, пятница"))).toBe(true);
  });

  it("ошибка службы по программам - на шаге «Программы»", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", { ok: false, error: "Выберите категорию программы", field: "programs" }, 1);
    await user.click(done());
    expect(await screen.findByText("Выберите категорию программы")).toBeTruthy();
    expect(screen.getByText("Шаг 4 из 4")).toBeTruthy();
  });

  it("нет связи: простыми словами, можно нажать «Готово» ещё раз", async () => {
    const user = setup();
    await passStep1(user);
    await passStep2(user, "Миша");
    await passStep3(user);
    mock.override("Setup", "reject", 1);
    await user.click(done());
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(done());
    expect(await screen.findByText("Готово. Правила начнут действовать при следующем входе детей")).toBeTruthy();
  });
});
