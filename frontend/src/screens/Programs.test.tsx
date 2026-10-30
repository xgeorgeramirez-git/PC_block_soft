import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Programs from "./Programs";
import { installBackend, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import type { PendingProgramDTO, ProgramDTO } from "../types";

// FR-20, сценарий 8.3.4: «Ждут решения» с кнопками, «Отменить» 10 с, список всех программ
// с поиском, фильтром, сортировкой и массовой сменой категории; загрузка и ошибки.

let mock: MockBackend;

// пт, 30 октября 2026, 12:00 по местному времени окна.
const NOW = new Date(2026, 9, 30, 12, 0, 0);
const YESTERDAY_1840 = new Date(2026, 9, 29, 18, 40, 0).getTime();
const TODAY_0905 = new Date(2026, 9, 30, 9, 5, 0).getTime();

const SUPERGAME = "c:\\users\\миша\\downloads\\supergame.exe";
const TRAINER =
  "c:\\users\\аня\\appdata\\local\\programs\\очень длинное название папки тренажёра\\версия 2026.10.1\\тренажёр.exe";
const BRAVE = "c:\\program files\\bravesoftware\\brave-browser\\application\\brave.exe";
const MINECRAFT = "c:\\program files\\minecraft launcher\\minecraftlauncher.exe";
const NOTEPAD = "c:\\windows\\system32\\notepad.exe";
const UCHI = "c:\\program files\\uchi\\uchi.exe";

const pendingSuperGame = (): PendingProgramDTO => ({
  path: SUPERGAME,
  name: "SuperGame",
  publisher: "Без подписи издателя",
  description: "Super Game Deluxe",
  attempts: 4,
  lastAttemptAt: YESTERDAY_1840,
  wasCategory: "games",
  byChild: [
    { childId: 1, name: "Миша", attempts: 3 },
    { childId: 2, name: "Аня", attempts: 1 },
  ],
});

const pendingTrainer = (): PendingProgramDTO => ({
  path: TRAINER,
  name: "Тренажёр",
  publisher: "",
  description: "",
  attempts: 2,
  lastAttemptAt: TODAY_0905,
  wasCategory: "",
  byChild: [{ childId: 2, name: "Аня", attempts: 2 }],
});

const programs = (): ProgramDTO[] => [
  { path: BRAVE, name: "Brave", publisher: "Brave Software, Inc.", category: "other", source: "snapshot", weekS: 5400 },
  { path: MINECRAFT, name: "Minecraft Launcher", publisher: "Mojang", category: "games", source: "catalog", weekS: 0 },
  { path: UCHI, name: "Учи.ру", publisher: "Учи.ру", category: "study", source: "parent", weekS: 2 * 3600 + 5 * 60 },
  { path: NOTEPAD, name: "Блокнот", publisher: "Microsoft", category: "system", source: "snapshot", weekS: 600 },
];

function setup(opts: MockOptions = {}) {
  mock = installBackend({
    sessionActive: true,
    pending: [pendingSuperGame(), pendingTrainer()],
    programs: programs(),
    ...opts,
  });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<Programs />);
  return user;
}

/** Задерживает ответы команды до вызова open(); считает, сколько раз её вызвали. */
function gate(command: string) {
  let open!: () => void;
  const until = new Promise<void>((resolve) => (open = resolve));
  const app = window.go!.ui!.App!;
  const orig = app.Call;
  const counter = { calls: 0 };
  app.Call = async (c: string, payload: string) => {
    if (c === command) {
      counter.calls += 1;
      await until;
    }
    return orig(c, payload);
  };
  return { open: () => act(() => open()), counter };
}

const pendingBlock = () => screen.getByRole("region", { name: /^Ждут решения/ });
const pendingItem = (name: string) => within(pendingBlock()).getByRole("listitem", { name });
const allBlock = () => screen.getByRole("region", { name: "Все программы" });
const rowNames = () =>
  within(allBlock())
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole("cell")[1].querySelector(".program-name")?.textContent);
const rowOf = (name: string) => within(allBlock()).getByRole("row", { name: new RegExp(`^${name}`) });

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("Программы: загрузка и ошибки", () => {
  it("пока данные идут: «Загружаю данные»", async () => {
    setup();
    expect(screen.getByRole("heading", { name: "Программы" })).toBeTruthy();
    expect(screen.getByText("Загружаю данные")).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Ждут решения: 2" })).toBeTruthy();
    expect(screen.queryByText("Загружаю данные")).toBeNull();
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    const user = setup({ linkDown: true });
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    mock.state.linkDown = false;
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("heading", { name: "Ждут решения: 2" })).toBeTruthy();
  });

  it("решение не записалось: ошибка словами, строка остаётся в «Ждут решения»", async () => {
    const user = setup();
    mock.override("DecideProgram", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз" }, 1);
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Не удалось сохранить. Попробуйте ещё раз");
    expect(pendingItem("SuperGame")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    // Фокус вернулся на нажатую кнопку, а не пропал.
    expect(document.activeElement).toBe(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
  });
});

describe("Программы: ждут решения", () => {
  it("заголовок со счётчиком и пояснение, что решение для всех детей", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Ждут решения: 2" })).toBeTruthy();
    expect(within(pendingBlock()).getByText("Решение действует для всех детей")).toBeTruthy();
  });

  it("строка: название, издатель, описание, кто и когда запускал, пометка «Раньше было», 4 кнопки", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const sg = pendingItem("SuperGame");
    expect(within(sg).getByText("SuperGame")).toBeTruthy();
    expect(within(sg).getByText("Без подписи издателя")).toBeTruthy();
    expect(within(sg).getByText("Super Game Deluxe")).toBeTruthy();
    expect(within(sg).getByText("Запускали: Миша 3 раза, Аня 1 раз; последняя попытка вчера 18:40")).toBeTruthy();
    expect(within(sg).getByText("Раньше было: игра")).toBeTruthy();
    expect(within(sg).getAllByRole("button").map((b) => b.textContent)).toEqual(["Игра", "Учёба", "Прочее", "Запретить"]);
    // «Запретить» второстепенная, не красная кнопка.
    expect(within(sg).getByRole("button", { name: "Запретить" }).className).toBe("btn btn-muted");
  });

  it("без издателя: «Без подписи издателя»; без описания и пометки лишних строк нет", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const tr = pendingItem("Тренажёр");
    expect(within(tr).getByText("Без подписи издателя")).toBeTruthy();
    expect(within(tr).queryByText(/Раньше было/)).toBeNull();
    expect(within(tr).getByText("Запускали: Аня 2 раза; последняя попытка сегодня 09:05")).toBeTruthy();
  });

  it("длинный путь обрезан по центру, полный в подсказке и для чтения с клавиатуры", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const path = within(pendingItem("Тренажёр")).getByTitle(TRAINER);
    expect(path.querySelector(".path-short")?.textContent).toBe(
      "c:\\users\\аня\\appdata\\local\\programs\\очень длинное…\\версия 2026.10.1\\тренажёр.exe",
    );
    expect(path.querySelector(".path-full")?.textContent).toBe(TRAINER);
    expect(path.tabIndex).toBe(0);
    // Короткий путь не обрезан и не стоит лишней остановкой Tab.
    const short = within(pendingItem("SuperGame")).getByTitle(SUPERGAME);
    expect(short.querySelector(".path-short")?.textContent).toBe(SUPERGAME);
    expect(short.tabIndex).toBe(-1);
  });

  // Ловит: путь в таблице переносится посреди имени файла («supergame.ex» / «e»).
  it("путь в таблице одной строкой: начало сжимается, имя файла целиком отдельным куском", async () => {
    setup();
    await screen.findByRole("region", { name: "Все программы" });
    const path = within(rowOf("Brave")).getByTitle(BRAVE);
    expect(path.querySelector(".path-short")?.textContent).toBe("c:\\program files\\…\\application\\brave.exe");
    expect(path.querySelector(".path-head")?.textContent).toBe("c:\\program files\\…\\application");
    expect(path.querySelector(".path-tail")?.textContent).toBe("\\brave.exe");
    expect(path.querySelector(".path-full")?.textContent).toBe(BRAVE);
  });

  it("нет ждущих: блока нет", async () => {
    setup({ pending: [] });
    expect(await screen.findByRole("region", { name: "Все программы" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: /Ждут решения/ })).toBeNull();
  });

  it("«Игра»: решение сразу, строка уходит в список с категорией «Игры», сообщение с «Отменить»", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));

    expect(await screen.findByText("SuperGame: игра")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "DecideProgram").map((c) => c.payload)).toEqual([
      { path: SUPERGAME, category: "games" },
    ]);
    expect(screen.getByRole("heading", { name: "Ждут решения: 1" })).toBeTruthy();
    expect(within(pendingBlock()).queryByRole("listitem", { name: "SuperGame" })).toBeNull();
    const select = within(rowOf("SuperGame")).getByRole("combobox", { name: "Категория для «SuperGame»" }) as HTMLSelectElement;
    expect(select.value).toBe("games");
    expect(screen.getByRole("button", { name: "Отменить" })).toBeTruthy();
  });

  it("«Учёба» отправляет учёбу, «Запретить» запрет", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Учёба" }));
    expect(await screen.findByText("SuperGame: учёба")).toBeTruthy();
    await user.click(within(pendingItem("Тренажёр")).getByRole("button", { name: "Запретить" }));
    expect(await screen.findByText("Тренажёр: запрещена")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "DecideProgram").map((c) => c.payload.category)).toEqual(["study", "blocked"]);
  });

  it("пока решение в пути: «Сохраняю…», повторное нажатие ничего не шлёт, остальные кнопки недоступны", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const g = gate("DecideProgram");
    const game = within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" });
    await user.click(game);
    expect(game.textContent).toBe("Сохраняю…");
    expect(game.getAttribute("aria-disabled")).toBe("true");
    await user.click(game);
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Учёба" }));
    await user.click(within(pendingItem("Тренажёр")).getByRole("button", { name: "Игра" }));
    expect(g.counter.calls).toBe(1);
    expect((within(pendingItem("Тренажёр")).getByRole("button", { name: "Игра" }) as HTMLButtonElement).disabled).toBe(true);
    await g.open();
    expect(await screen.findByText("SuperGame: игра")).toBeTruthy();
    expect(g.counter.calls).toBe(1);
  });

  it("фокус после решения на следующей ждущей, после последней на заголовке «Все программы»", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    expect(document.activeElement).toBe(within(pendingItem("Тренажёр")).getByRole("button", { name: "Игра" }));

    await user.click(within(pendingItem("Тренажёр")).getByRole("button", { name: "Прочее" }));
    await screen.findByText("Тренажёр: прочее");
    expect(screen.queryByRole("heading", { name: /Ждут решения/ })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Все программы" }));
  });

  it("«Отменить»: строка возвращается в «Ждут решения» на прежнее место", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    await user.click(screen.getByRole("button", { name: "Отменить" }));

    expect(await screen.findByText("Решение отменено")).toBeTruthy();
    expect(mock.count("UndoProgram")).toBe(1);
    expect(screen.getByRole("heading", { name: "Ждут решения: 2" })).toBeTruthy();
    expect(within(pendingBlock()).getAllByRole("listitem").map((li) => li.getAttribute("aria-label"))).toEqual([
      "SuperGame",
      "Тренажёр",
    ]);
    expect(within(allBlock()).queryByRole("row", { name: /^SuperGame/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    // Фокус на вернувшейся строке.
    expect(document.activeElement).toBe(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
  });

  it("«Отменить» пропадает через 10 с", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    act(() => {
      vi.advanceTimersByTime(9_000);
    });
    expect(screen.getByRole("button", { name: "Отменить" })).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(1_100);
    });
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(screen.getByText("SuperGame: игра")).toBeTruthy();
  });

  it("пока фокус на «Отменить», кнопка не пропадает; ушёл фокус после 10 с: пропала", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    const undo = screen.getByRole("button", { name: "Отменить" });
    act(() => undo.focus());
    act(() => {
      vi.advanceTimersByTime(12_000);
    });
    expect(screen.getByRole("button", { name: "Отменить" })).toBe(undo);
    act(() => undo.blur());
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
  });

  it("сообщение в области, которую зачитывает экранный диктор", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    const msg = await screen.findByText("SuperGame: игра");
    expect(msg.closest('[role="status"]')).toBeTruthy();
  });

  it("«Отменить» достижимо с клавиатуры: Shift+Tab от следующей строки", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    // Фокус на «Игра» у «Тренажёра»; назад: путь тренажёра (обрезан, в Tab), ..., «Отменить».
    for (let i = 0; i < 8 && document.activeElement?.textContent !== "Отменить"; i++) {
      await user.tab({ shift: true });
    }
    expect(document.activeElement?.textContent).toBe("Отменить");
  });
});

describe("Программы: все программы", () => {
  it("строки без системных; время за неделю словами", async () => {
    setup();
    await screen.findByRole("region", { name: "Все программы" });
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Учи.ру"]);
    expect(within(rowOf("Brave")).getByText("1 ч 30 мин")).toBeTruthy();
    expect(within(rowOf("Учи.ру")).getByText("2 ч 05 мин")).toBeTruthy();
    expect(within(rowOf("Minecraft Launcher")).getByText("нет")).toBeTruthy();
    expect(within(rowOf("Brave")).getByText("Brave Software, Inc.")).toBeTruthy();
    expect(screen.queryByText("Блокнот")).toBeNull();
  });

  it("фильтр по категории", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    const filters = within(allBlock()).getByRole("group", { name: "Показать" });
    expect(within(filters).getAllByRole("button").map((b) => b.textContent)).toEqual(["Все", "Игры", "Учёба", "Прочее", "Запрещено"]);
    expect(within(filters).getByRole("button", { name: "Все" }).getAttribute("aria-pressed")).toBe("true");

    await user.click(within(filters).getByRole("button", { name: "Игры" }));
    expect(rowNames()).toEqual(["Minecraft Launcher"]);
    expect(within(filters).getByRole("button", { name: "Игры" }).getAttribute("aria-pressed")).toBe("true");
    await user.click(within(filters).getByRole("button", { name: "Учёба" }));
    expect(rowNames()).toEqual(["Учи.ру"]);
  });

  it("поиск по названию и пути; «Ничего не найдено» и «Сбросить поиск»", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    const search = within(allBlock()).getByRole("searchbox", { name: "Поиск по названию или папке" });
    await user.type(search, "minecraft");
    expect(rowNames()).toEqual(["Minecraft Launcher"]);
    await user.clear(search);
    await user.type(search, "program files\\uchi");
    expect(rowNames()).toEqual(["Учи.ру"]);
    await user.clear(search);
    await user.type(search, "нет такой");
    expect(within(allBlock()).getByText("Ничего не найдено")).toBeTruthy();
    await user.click(within(allBlock()).getByRole("button", { name: "Сбросить поиск" }));
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Учи.ру"]);
    expect((search as HTMLInputElement).value).toBe("");
  });

  it("«Сбросить поиск» сбрасывает и фильтр", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(allBlock()).getByRole("button", { name: "Запрещено" }));
    expect(within(allBlock()).getByText("Ничего не найдено")).toBeTruthy();
    await user.click(within(allBlock()).getByRole("button", { name: "Сбросить поиск" }));
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Учи.ру"]);
  });

  it("сортировка по названию и по времени за неделю", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    const nameHead = within(allBlock()).getByRole("columnheader", { name: /Программа/ });
    const weekHead = within(allBlock()).getByRole("columnheader", { name: /За неделю/ });
    expect(nameHead.getAttribute("aria-sort")).toBe("ascending");
    expect(weekHead.getAttribute("aria-sort")).toBe("none");

    await user.click(within(weekHead).getByRole("button"));
    expect(rowNames()).toEqual(["Учи.ру", "Brave", "Minecraft Launcher"]);
    expect(weekHead.getAttribute("aria-sort")).toBe("descending");
    await user.click(within(weekHead).getByRole("button"));
    expect(rowNames()).toEqual(["Minecraft Launcher", "Brave", "Учи.ру"]);
    expect(weekHead.getAttribute("aria-sort")).toBe("ascending");

    await user.click(within(nameHead).getByRole("button"));
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Учи.ру"]);
    await user.click(within(nameHead).getByRole("button"));
    expect(rowNames()).toEqual(["Учи.ру", "Minecraft Launcher", "Brave"]);
    expect(nameHead.getAttribute("aria-sort")).toBe("descending");
  });

  it("категория меняется выбором в строке: сообщение с «Отменить»", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    const select = within(rowOf("Brave")).getByRole("combobox", { name: "Категория для «Brave»" }) as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.textContent)).toEqual(["Игры", "Учёба", "Прочее", "Запрещено"]);
    await user.selectOptions(select, "study");
    expect(await screen.findByText("Brave: учёба")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "DecideProgram").map((c) => c.payload)).toEqual([{ path: BRAVE, category: "study" }]);
    expect((within(rowOf("Brave")).getByRole("combobox") as HTMLSelectElement).value).toBe("study");
    expect(screen.getByRole("button", { name: "Отменить" })).toBeTruthy();
    // Фокус остался на выборе этой строки.
    expect(document.activeElement).toBe(within(rowOf("Brave")).getByRole("combobox"));
  });

  it("пустой список: «Программ пока нет…»", async () => {
    setup({ pending: [], programs: [] });
    expect(await screen.findByText("Программ пока нет: они появятся, когда ребёнок запустит что-то новое")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("только системные: тоже «Программ пока нет…»", async () => {
    setup({ pending: [], programs: programs().filter((p) => p.category === "system") });
    expect(await screen.findByText("Программ пока нет: они появятся, когда ребёнок запустит что-то новое")).toBeTruthy();
  });
});

describe("Программы: массовая смена категории", () => {
  it("отметил две строки: «Выбрано: 2», выбрал категорию, «Применить», одно сообщение с «Отменить»", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    expect(screen.queryByText(/^Выбрано:/)).toBeNull();
    await user.click(within(rowOf("Brave")).getByRole("checkbox", { name: "Отметить «Brave»" }));
    await user.click(within(rowOf("Minecraft Launcher")).getByRole("checkbox", { name: "Отметить «Minecraft Launcher»" }));
    expect(screen.getByText("Выбрано: 2")).toBeTruthy();
    await user.selectOptions(screen.getByRole("combobox", { name: "Новая категория для отмеченных" }), "blocked");
    await user.click(screen.getByRole("button", { name: "Применить" }));

    expect(await screen.findByText("Изменено программ: 2")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "SetProgramsCategory").map((c) => c.payload)).toEqual([
      { paths: [BRAVE, MINECRAFT], category: "blocked" },
    ]);
    expect(screen.getAllByRole("button", { name: "Отменить" })).toHaveLength(1);
    expect(screen.queryByText(/^Выбрано:/)).toBeNull();
    expect((within(rowOf("Brave")).getByRole("combobox") as HTMLSelectElement).value).toBe("blocked");

    await user.click(screen.getByRole("button", { name: "Отменить" }));
    expect(await screen.findByText("Решение отменено")).toBeTruthy();
    expect((within(rowOf("Brave")).getByRole("combobox") as HTMLSelectElement).value).toBe("other");
    expect((within(rowOf("Minecraft Launcher")).getByRole("combobox") as HTMLSelectElement).value).toBe("games");
  });

  it("«Выбрать всё в списке» отмечает только видимые строки", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(allBlock()).getByRole("button", { name: "Игры" }));
    await user.click(screen.getByRole("checkbox", { name: "Выбрать всё в списке" }));
    expect(screen.getByText("Выбрано: 1")).toBeTruthy();
    await user.selectOptions(screen.getByRole("combobox", { name: "Новая категория для отмеченных" }), "study");
    await user.click(screen.getByRole("button", { name: "Применить" }));
    await screen.findByText("Изменено программ: 1");
    expect(mock.calls.filter((c) => c.command === "SetProgramsCategory").map((c) => c.payload.paths)).toEqual([[MINECRAFT]]);
  });

  it("«Применить» без категории: подсказка у поля, запроса нет", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Brave")).getByRole("checkbox"));
    await user.click(screen.getByRole("button", { name: "Применить" }));
    expect(screen.getByText("Выберите категорию")).toBeTruthy();
    expect(mock.count("SetProgramsCategory")).toBe(0);
  });

  it("пока массовая смена в пути: «Сохраняю…» и повторно не нажать", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Brave")).getByRole("checkbox"));
    await user.selectOptions(screen.getByRole("combobox", { name: "Новая категория для отмеченных" }), "games");
    const g = gate("SetProgramsCategory");
    const apply = screen.getByRole("button", { name: "Применить" });
    await user.click(apply);
    expect(apply.textContent).toBe("Сохраняю…");
    await user.click(apply);
    expect(g.counter.calls).toBe(1);
    await g.open();
    expect(await screen.findByText("Изменено программ: 1")).toBeTruthy();
    expect(g.counter.calls).toBe(1);
  });
});
