import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Programs from "./Programs";
import { installBackend, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import { HIDE_LEAD } from "../lib/programs";
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
    expect(screen.getByText("Загружаю данные…")).toBeTruthy();
    expect(await screen.findByRole("heading", { name: "Ждут решения: 2" })).toBeTruthy();
    expect(screen.queryByText("Загружаю данные…")).toBeNull();
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
    expect(within(sg).getByText("Файл изменился, проверьте решение. Было: игра")).toBeTruthy();
    expect(within(sg).getAllByRole("button").map((b) => b.textContent)).toEqual(["Игра", "Учёба", "Прочее", "Запретить"]);
    // Ловит: «Запретить» выглядит неактивной (серый текст btn-muted, приёмка P2 п.1). Она обычная
    // второстепенная кнопка того же вида, что «Игра»/«Учёба»/«Прочее»: не красная и доступна.
    const buttons = within(sg).getAllByRole("button") as HTMLButtonElement[];
    expect(buttons.map((b) => b.className)).toEqual(["btn", "btn", "btn", "btn"]);
    const ban = within(sg).getByRole("button", { name: "Запретить" }) as HTMLButtonElement;
    expect(ban.disabled).toBe(false);
    expect(ban.getAttribute("aria-disabled")).toBeNull();
  });

  it("без издателя: «Без подписи издателя»; без описания и пометки лишних строк нет", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const tr = pendingItem("Тренажёр");
    expect(within(tr).getByText("Без подписи издателя")).toBeTruthy();
    expect(within(tr).queryByText(/Файл изменился/)).toBeNull();
    expect(within(tr).getByText("Запускали: Аня 2 раза; последняя попытка сегодня 09:05")).toBeTruthy();
  });

  // Ловит (B-39): нет пояснения, что значит каждая кнопка решения; кнопки не связаны со своей
  // строкой легенды для диктора; легенда повторена в каждой ждущей программе.
  it("легенда «Что значат кнопки» одна на блок, каждая кнопка ссылается на свою строку", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const legend = within(pendingBlock()).getByRole("list", { name: "Что значат кнопки" });
    // Ловит (ревью M4): заголовок легенды только для диктора; зрячий не видит, что это за строки.
    const title = within(pendingBlock()).getByRole("heading", { name: "Что значат кнопки", level: 4 });
    expect(title.classList.contains("visually-hidden")).toBe(false);
    expect(legend.getAttribute("aria-labelledby")).toBe(title.id);
    expect(legend.getAttribute("aria-label")).toBeNull();
    expect(title.compareDocumentPosition(legend) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // Подпись кнопки выделена, за ней двоеточие и смысл из явной строки.
    expect(Array.from(legend.querySelectorAll(".pending-legend-label")).map((el) => el.textContent)).toEqual([
      "Игра",
      "Учёба",
      "Прочее",
      "Запретить",
    ]);
    expect(within(legend).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Игра: время идёт в лимит игр и в общее время",
      "Учёба: время не тратит лимит",
      "Прочее: время идёт в общее время",
      "Запретить: программа будет закрываться сразу после запуска",
    ]);
    const hintOf = (btn: HTMLElement) => document.getElementById(btn.getAttribute("aria-describedby")!)!.textContent;
    const sg = pendingItem("SuperGame");
    expect(hintOf(within(sg).getByRole("button", { name: "Игра" }))).toBe("Игра: время идёт в лимит игр и в общее время");
    expect(hintOf(within(sg).getByRole("button", { name: "Учёба" }))).toBe("Учёба: время не тратит лимит");
    expect(hintOf(within(sg).getByRole("button", { name: "Прочее" }))).toBe("Прочее: время идёт в общее время");
    expect(hintOf(within(sg).getByRole("button", { name: "Запретить" }))).toBe(
      "Запретить: программа будет закрываться сразу после запуска",
    );
    expect(hintOf(within(pendingItem("Тренажёр")).getByRole("button", { name: "Запретить" }))).toBe(
      "Запретить: программа будет закрываться сразу после запуска",
    );
    // Легенда идёт под «Решение действует для всех детей», до списка ждущих.
    const lead = within(pendingBlock()).getByText("Решение действует для всех детей");
    expect(lead.compareDocumentPosition(legend) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(legend.compareDocumentPosition(sg) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("нет ждущих: нет и легенды кнопок", async () => {
    setup({ pending: [] });
    await screen.findByRole("region", { name: "Все программы" });
    expect(screen.queryByRole("list", { name: "Что значат кнопки" })).toBeNull();
  });

  // Ловит (B-39, P1-R2): «Без подписи издателя» без подсказки, что делать; подсказка у программы
  // с издателем или в строке списка «Все программы».
  it("неизвестный издатель: подсказка только в «Ждут решения»", async () => {
    const NOPUB = "c:\\games\\nopub\\nopub.exe";
    setup({
      pending: [{ ...pendingSuperGame(), publisher: "Super Inc" }, pendingTrainer()],
      programs: [...programs(), { path: NOPUB, name: "NoPub", publisher: "", category: "other", source: "snapshot", weekS: 0 }],
    });
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    const HINT = "Издатель неизвестен: проверьте, откуда этот файл";
    expect(within(pendingItem("Тренажёр")).getByText(HINT)).toBeTruthy();
    expect(within(pendingItem("SuperGame")).getByText("Super Inc")).toBeTruthy();
    expect(within(pendingItem("SuperGame")).queryByText(HINT)).toBeNull();
    expect(within(rowOf("NoPub")).getByText("Без подписи издателя")).toBeTruthy();
    expect(within(allBlock()).queryByText(HINT)).toBeNull();
  });

  it("издатель-заглушка от службы: та же подсказка", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    expect(within(pendingItem("SuperGame")).getByText("Издатель неизвестен: проверьте, откуда этот файл")).toBeTruthy();
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

  // Ловит: путь в таблице обрезан вторым способом (CSS-многоточие поверх shortenPath): два «…» или
  // пробел у многоточия («\\wi… \\game.exe»), имя файла отдельным куском потеряно.
  it("путь с пробелами в таблице: одно многоточие без пробела рядом, имя файла отдельно", async () => {
    const steam = "c:\\program files (x86)\\steam\\steamapps\\common\\очень длинное название игры с пробелами\\bin\\win64\\game.exe";
    setup({ programs: [{ path: steam, name: "Игра", publisher: "", category: "games", source: "parent", weekS: 0 }] });
    await screen.findByRole("region", { name: "Все программы" });
    const path = within(rowOf("Игра")).getByTitle(steam);
    expect(path.querySelector(".path-short")?.textContent).toBe("c:\\program files (x86)\\s…\\win64\\game.exe");
    expect(path.querySelector(".path-head")?.textContent).toBe("c:\\program files (x86)\\s…\\win64");
    expect(path.querySelector(".path-tail")?.textContent).toBe("\\game.exe");
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
    // Только ждущие программы: строки легенды «Что значат кнопки» (B-39) — тоже элементы списка.
    expect(Array.from(pendingBlock().querySelectorAll(".pending-list > li")).map((li) => li.getAttribute("aria-label"))).toEqual([
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

  // Ловит (B-43): у «Отменить» нет отсчёта, отсчёт попал в название кнопки или в живую область
  // (диктор зачитывал бы каждую секунду).
  it("рядом с «Отменить» отсчёт секунд: 10 с, через 3 с - 7 с; кнопка называется «Отменить»", async () => {
    const user = setup();
    await screen.findByRole("heading", { name: "Ждут решения: 2" });
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    await screen.findByText("SuperGame: игра");
    const left = () => document.querySelector(".programs-message .undo-left");
    expect(left()?.textContent).toBe("10 с");
    expect(left()?.getAttribute("aria-hidden")).toBe("true");
    act(() => {
      vi.advanceTimersByTime(3_000);
    });
    expect(left()?.textContent).toBe("7 с");
    expect(screen.getByRole("button", { name: "Отменить" }).textContent).toBe("Отменить");
    act(() => {
      vi.advanceTimersByTime(7_100);
    });
    expect(left()).toBeNull();
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
    expect(within(filters).getAllByRole("button").map((b) => b.textContent)).toEqual(["Все", "Игры", "Учёба", "Прочее", "Запрещено", "Скрытые"]);
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

  // Ловит (B-43, B-38): сообщение о действии в таблице стоит в другом месте, чем о решении сверху,
  // или живой области нет до первого сообщения (диктор пропустит первое).
  it("одно место сообщения: живая область есть до действия, сообщение строки - в ней же, над «Ждут решения»", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    const slots = document.querySelectorAll(".programs-message");
    expect(slots).toHaveLength(1);
    const slot = slots[0] as HTMLElement;
    expect(slot.getAttribute("role")).toBe("status");
    expect(slot.textContent).toBe("");
    const pendingTitle = screen.getByRole("heading", { name: "Ждут решения: 2" });
    expect(slot.compareDocumentPosition(pendingTitle) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await user.selectOptions(within(rowOf("Brave")).getByRole("combobox", { name: "Категория для «Brave»" }), "study");
    const msg = await screen.findByText("Brave: учёба");
    expect(slot.contains(msg)).toBe(true);
    expect(within(slot).getByRole("button", { name: "Отменить" })).toBeTruthy();
    expect(document.querySelectorAll('.programs-page [role="status"]')).toHaveLength(1);
    await user.click(within(pendingItem("SuperGame")).getByRole("button", { name: "Игра" }));
    expect(slot.contains(await screen.findByText("SuperGame: игра"))).toBe(true);
    expect(screen.queryByText("Brave: учёба")).toBeNull();
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
    expect(screen.queryByText(HIDE_LEAD)).toBeNull();
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

  // Ловит (B-43 п.4): «Применить» без категории выглядит доступной и объясняет ошибкой только после
  // нажатия; недоступная кнопка не говорит почему (ТЗ 8.0 п.5); нажатие всё же шлёт запрос.
  it("«Применить» без категории недоступна с причиной рядом, запроса нет", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Brave")).getByRole("checkbox"));
    const apply = screen.getByRole("button", { name: "Применить" });
    expect(apply.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Сначала выберите категорию")).toBeTruthy();
    expect(document.getElementById(apply.getAttribute("aria-describedby") ?? "")?.textContent).toBe("Сначала выберите категорию");
    await user.click(apply);
    expect(mock.count("SetProgramsCategory")).toBe(0);
    expect(screen.queryByText("Выберите категорию")).toBeNull();

    await user.selectOptions(screen.getByRole("combobox", { name: "Новая категория для отмеченных" }), "games");
    expect(screen.getByRole("button", { name: "Применить" }).getAttribute("aria-disabled")).toBeNull();
    expect(screen.getByRole("button", { name: "Применить" }).getAttribute("aria-describedby")).toBeNull();
    expect(screen.queryByText("Сначала выберите категорию")).toBeNull();
  });

  // Ловит (B-41): смена, которая ничего не изменила (Учи.ру уже «Учёба» решением родителя), даёт
  // «Изменено программ: 0» и «Отменить».
  it("та же категория: «Категория не изменилась» без «Отменить»", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Учи.ру")).getByRole("checkbox", { name: "Отметить «Учи.ру»" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Новая категория для отмеченных" }), "study");
    await user.click(screen.getByRole("button", { name: "Применить" }));
    expect(await screen.findByText("Категория не изменилась")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(mock.calls.filter((c) => c.command === "SetProgramsCategory").map((c) => c.payload)).toEqual([
      { paths: [UCHI], category: "study" },
    ]);
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

describe("Программы: скрытые (D-57, 8.3.4 п.4)", () => {
  const STALKER = "c:\\program files\\epic games\\stalker2\\stalker2.exe";
  const SHIPPING = "c:\\program files\\epic games\\stalker2\\binaries\\win64\\stalker2-win64-shipping.exe";
  const withStalker = (): ProgramDTO[] => [
    ...programs(),
    { path: STALKER, name: "Stalker2", publisher: "Epic Games", category: "games", source: "parent", weekS: 600 },
    { path: SHIPPING, name: "Stalker2-Win64-Shipping", publisher: "GSC Game World", category: "games", source: "catalog", weekS: 7200 },
  ];
  const filterButton = (name: string) => within(within(allBlock()).getByRole("group", { name: "Показать" })).getByRole("button", { name });

  // Ловит: «Скрыть» не шлёт SetProgramsHidden, строка остаётся в списке, сообщение не «Stalker2
  // скрыта», «Отменить» не возвращает строку, фокус теряется.
  it("«Скрыть» в строке: строка пропала, «Stalker2 скрыта» с «Отменить»", async () => {
    const user = setup({ programs: withStalker() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Stalker2$")).getByRole("button", { name: "Скрыть «Stalker2»" }));

    expect(await screen.findByText("Stalker2 скрыта")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "SetProgramsHidden").map((c) => c.payload)).toEqual([
      { paths: [STALKER], hidden: true },
    ]);
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Stalker2-Win64-Shipping", "Учи.ру"]);
    expect(mock.state.events.map((e) => e.text)).toContain("Вы скрыли «Stalker2»");
    // Фокус на соседней строке, а не на потерянном элементе.
    expect(document.activeElement?.closest("tr")?.getAttribute("aria-label")).toBe("Stalker2-Win64-Shipping");

    await user.click(screen.getByRole("button", { name: "Отменить" }));
    expect(await screen.findByText("Решение отменено")).toBeTruthy();
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Stalker2", "Stalker2-Win64-Shipping", "Учи.ру"]);
  });

  // Ловит: скрытая видна в «Все» и в своей категории; в «Скрытых» у неё выбор категории и время
  // за неделю; «Показать» не возвращает программу в прежнюю категорию.
  it("фильтр «Скрытые»: только скрытые, категория текстом, «Показать» возвращает в «Игры»", async () => {
    const list = withStalker();
    list[4] = { ...list[4], hidden: true };
    const user = setup({ programs: list });
    await screen.findByRole("region", { name: "Все программы" });
    expect(rowNames()).toEqual(["Brave", "Minecraft Launcher", "Stalker2-Win64-Shipping", "Учи.ру"]);
    await user.click(filterButton("Игры"));
    expect(rowNames()).toEqual(["Minecraft Launcher", "Stalker2-Win64-Shipping"]);

    await user.click(filterButton("Скрытые (1)"));
    expect(rowNames()).toEqual(["Stalker2"]);
    const row = rowOf("Stalker2$");
    expect(within(row).queryByRole("combobox")).toBeNull();
    expect(within(row).getByText("Игры")).toBeTruthy();
    expect(within(row).queryByText("нет")).toBeNull();
    // Время скрытой не считается совсем (R2, D-63): ячейка так и говорит, а не пустая (B-43 п.9).
    expect(within(row).getByText("не считается")).toBeTruthy();

    await user.click(within(row).getByRole("button", { name: "Показать «Stalker2»" }));
    expect(await screen.findByText("Stalker2 снова в списке")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "SetProgramsHidden").map((c) => c.payload)).toEqual([
      { paths: [STALKER], hidden: false },
    ]);
    expect(screen.getByText("Скрытых программ нет")).toBeTruthy();
    await user.click(filterButton("Игры"));
    expect(rowNames()).toEqual(["Minecraft Launcher", "Stalker2", "Stalker2-Win64-Shipping"]);
    expect((within(rowOf("Stalker2$")).getByRole("combobox") as HTMLSelectElement).value).toBe("games");
  });

  // Ловит: для отмеченных строк нет «Скрыть»; в «Скрытых» вместо «Показать» предлагается смена
  // категории (её у скрытой не видно).
  it("отмеченные: «Скрыть отмеченные», в «Скрытых» «Показать отмеченные» без выбора категории", async () => {
    const user = setup({ programs: withStalker() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(within(rowOf("Brave")).getByRole("checkbox"));
    await user.click(within(rowOf("Stalker2$")).getByRole("checkbox", { name: "Отметить «Stalker2»" }));
    await user.click(screen.getByRole("button", { name: "Скрыть отмеченные" }));
    expect(await screen.findByText("Скрыто программ: 2")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "SetProgramsHidden").map((c) => c.payload)).toEqual([
      { paths: [BRAVE, STALKER], hidden: true },
    ]);
    expect(screen.queryByText(/^Выбрано:/)).toBeNull();

    await user.click(filterButton("Скрытые (2)"));
    expect(rowNames()).toEqual(["Brave", "Stalker2"]);
    await user.click(screen.getByRole("checkbox", { name: "Выбрать всё в списке" }));
    expect(screen.queryByRole("combobox", { name: "Новая категория для отмеченных" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Показать отмеченные" }));
    expect(await screen.findByText("Снова в списке программ: 2")).toBeTruthy();
    expect(screen.getByText("Скрытых программ нет")).toBeTruthy();
  });

  // Ловит (ревью 0.3.1, RF9-5): у запрещённой есть «Скрыть» (скрытие сильнее запрета); отмеченные
  // только запрещённые можно «скрыть»; в смешанном наборе служба не объясняет пропуск.
  it("запрещённая: «Скрыть» в строке нет; отмечены только запрещённые: кнопка недоступна с пояснением", async () => {
    const BAD = "c:\\program files\\badgame\\bad.exe";
    const list: ProgramDTO[] = [
      ...withStalker(),
      { path: BAD, name: "BadGame", publisher: "Bad Inc.", category: "blocked", source: "parent", weekS: 0 },
    ];
    const user = setup({ programs: list });
    await screen.findByRole("region", { name: "Все программы" });
    expect(within(rowOf("BadGame")).queryByRole("button", { name: /^Скрыть/ })).toBeNull();
    expect(within(rowOf("Brave")).getByRole("button", { name: "Скрыть «Brave»" })).toBeTruthy();

    await user.click(within(rowOf("BadGame")).getByRole("checkbox"));
    const bulk = screen.getByRole("button", { name: "Скрыть отмеченные" });
    expect(bulk.getAttribute("aria-disabled")).toBe("true");
    const hint = document.getElementById(bulk.getAttribute("aria-describedby") ?? "");
    expect(hint?.textContent).toBe("Запрещённую программу нельзя скрыть: сначала разрешите её");
    await user.click(bulk);
    expect(mock.calls.filter((c) => c.command === "SetProgramsHidden")).toEqual([]);

    await user.click(within(rowOf("Brave")).getByRole("checkbox"));
    expect(screen.getByRole("button", { name: "Скрыть отмеченные" }).getAttribute("aria-disabled")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Скрыть отмеченные" }));
    expect(await screen.findByText("Brave скрыта. Запрещённую программу нельзя скрыть: сначала разрешите её")).toBeTruthy();
    expect(mock.state.programs.find((p) => p.path === BAD)?.hidden).not.toBe(true);
    expect(rowNames()).toContain("BadGame");
  });

  // Ловит (B-43 п.2): у «Скрытые» не видно, сколько скрыто; счётчик считает системные программы,
  // которых родитель в списке не видит; число не уходит, когда скрытых не осталось.
  it("«Скрытые (1)»: число скрытых у фильтра, без системных; после «Показать» снова «Скрытые»", async () => {
    const list: ProgramDTO[] = [
      ...withStalker(),
      { path: "c:\\windows\\system32\\calc.exe", name: "Калькулятор", publisher: "Microsoft", category: "system", source: "snapshot", weekS: 0, hidden: true },
    ];
    list[4] = { ...list[4], hidden: true };
    const user = setup({ programs: list });
    await screen.findByRole("region", { name: "Все программы" });
    const filters = within(allBlock()).getByRole("group", { name: "Показать" });
    expect(within(filters).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "Все",
      "Игры",
      "Учёба",
      "Прочее",
      "Запрещено",
      "Скрытые (1)",
    ]);
    await user.click(filterButton("Скрытые (1)"));
    await user.click(within(rowOf("Stalker2$")).getByRole("button", { name: "Показать «Stalker2»" }));
    expect(await screen.findByText("Stalker2 снова в списке")).toBeTruthy();
    expect(filterButton("Скрытые").getAttribute("aria-pressed")).toBe("true");
  });

  // Ловит (B-43 п.3): не сказано, что делает «Скрыть»; у запрещённой ячейка действия пустая,
  // почему скрыть нельзя, не видно (RF9-5).
  it("пояснение «Скрыть» под «Все программы»; у запрещённой «Скрыть нельзя» вместо кнопки", async () => {
    const list = withStalker().map((p) => (p.path === BRAVE ? { ...p, category: "blocked" as const } : p));
    setup({ programs: list });
    await screen.findByRole("region", { name: "Все программы" });
    expect(within(allBlock()).getByText(HIDE_LEAD)).toBeTruthy();
    const brave = rowOf("Brave");
    expect(within(brave).getByText("Скрыть нельзя")).toBeTruthy();
    expect(within(brave).queryByRole("button", { name: "Скрыть «Brave»" })).toBeNull();
    expect(within(brave).getByText("Скрыть нельзя").closest("td")?.className).toBe("col-action");
    expect(within(rowOf("Stalker2$")).queryByText("Скрыть нельзя")).toBeNull();
    expect(within(rowOf("Stalker2$")).getByRole("button", { name: "Скрыть «Stalker2»" })).toBeTruthy();
  });

  // Ловит (ревью 0.3.1): операция без изменений даёт «Отменить».
  it("ничего не изменилось: сообщение без «Отменить»", async () => {
    const user = setup({ programs: withStalker() });
    await screen.findByRole("region", { name: "Все программы" });
    mock.override("SetProgramsHidden", { ok: true, payload: { message: "Brave уже скрыта", undoId: "", undoUntilMs: 0, programs: { pending: [], programs: withStalker(), children: [] } } }, 1);
    await user.click(within(rowOf("Brave")).getByRole("button", { name: "Скрыть «Brave»" }));
    expect(await screen.findByText("Brave уже скрыта")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
  });

  it("пока скрытие в пути: повторно не нажать", async () => {
    const user = setup({ programs: withStalker() });
    await screen.findByRole("region", { name: "Все программы" });
    const g = gate("SetProgramsHidden");
    const hide = within(rowOf("Stalker2$")).getByRole("button", { name: "Скрыть «Stalker2»" });
    await user.click(hide);
    expect(within(rowOf("Stalker2$")).getByText("Сохраняю…")).toBeTruthy();
    expect((hide as HTMLButtonElement).disabled).toBe(true);
    await g.open();
    expect(await screen.findByText("Stalker2 скрыта")).toBeTruthy();
    expect(g.counter.calls).toBe(1);
  });
});

describe("Программы: своё название (D-59, 8.3.4 п.5)", () => {
  const SHIPPING = "c:\\program files\\epic games\\stalker2\\binaries\\win64\\stalker2-win64-shipping.exe";
  const withShipping = (): ProgramDTO[] => [
    ...programs(),
    { path: SHIPPING, name: "Stalker2-Win64-Shipping", publisher: "GSC Game World", category: "games", source: "catalog", weekS: 7200 },
  ];
  const renameButton = (name: string) =>
    within(rowOf(name)).getByRole("button", { name: `Переименовать «${name}»` });

  // Ловит: «Переименовать» не открывает поле на месте названия, в поле не текущее название или
  // оно не выделено, фокус не в поле; «Сохранить» не шлёт RenameProgram; строка не показывает своё
  // название и «из файла: …»; нет «Название изменено» с «Отменить»; «Отменить» не возвращает.
  it("«Переименовать» → поле → «Сохранить»: своё название и «из файла: …»", async () => {
    const user = setup({ programs: withShipping() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(renameButton("Stalker2-Win64-Shipping"));

    const field = within(rowOf("Stalker2-Win64-Shipping")).getByRole("textbox", { name: "Новое название для «Stalker2-Win64-Shipping»" }) as HTMLInputElement;
    expect(document.activeElement).toBe(field);
    expect(field.value).toBe("Stalker2-Win64-Shipping");
    expect([field.selectionStart, field.selectionEnd]).toEqual([0, "Stalker2-Win64-Shipping".length]);
    await user.keyboard("Сталкер 2");
    await user.click(within(rowOf("Stalker2-Win64-Shipping")).getByRole("button", { name: "Сохранить" }));

    expect(await screen.findByText("Название изменено")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "RenameProgram").map((c) => c.payload)).toEqual([
      { path: SHIPPING, name: "Сталкер 2" },
    ]);
    const row = rowOf("Сталкер 2");
    expect(within(row).getByText("из файла: Stalker2-Win64-Shipping")).toBeTruthy();
    expect(within(row).queryByRole("textbox")).toBeNull();
    expect(mock.state.events.map((e) => e.text)).toContain("Вы переименовали «Stalker2-Win64-Shipping» в «Сталкер 2»");
    expect(document.activeElement).toBe(renameButton("Сталкер 2"));

    await user.click(screen.getByRole("button", { name: "Отменить" }));
    expect(await screen.findByText("Решение отменено")).toBeTruthy();
    expect(within(rowOf("Stalker2-Win64-Shipping")).queryByText(/^из файла:/)).toBeNull();
  });

  // Ловит: Enter не сохраняет, Escape не закрывает поле без запроса или теряет фокус; пустое поле
  // не сбрасывает к названию из файла.
  it("Enter сохраняет, Escape отменяет, пустое возвращает название из файла", async () => {
    const list = withShipping();
    list[4] = { ...list[4], name: "Сталкер 2", fileName: "Stalker2-Win64-Shipping" };
    const user = setup({ programs: list });
    await screen.findByRole("region", { name: "Все программы" });

    await user.click(renameButton("Сталкер 2"));
    await user.keyboard("Другое{Escape}");
    expect(within(rowOf("Сталкер 2")).queryByRole("textbox")).toBeNull();
    expect(mock.calls.filter((c) => c.command === "RenameProgram")).toEqual([]);
    expect(document.activeElement).toBe(renameButton("Сталкер 2"));

    await user.click(renameButton("Сталкер 2"));
    await user.clear(within(rowOf("Сталкер 2")).getByRole("textbox"));
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Название изменено")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "RenameProgram").map((c) => c.payload)).toEqual([{ path: SHIPPING, name: "" }]);
    expect(rowNames()).toContain("Stalker2-Win64-Shipping");
  });

  // Ловит (ревью 0.3.1): поле названия подсказывает автозаполнение браузера и подчёркивает
  // названия программ как ошибки правописания.
  it("поле названия: без автозаполнения и проверки правописания, с именем", async () => {
    const user = setup({ programs: withShipping() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(renameButton("Brave"));
    const field = within(rowOf("Brave")).getByRole("textbox");
    expect(field.getAttribute("autocomplete")).toBe("off");
    expect(field.getAttribute("spellcheck")).toBe("false");
    expect(field.getAttribute("name")).toBe("programName");
  });

  // Ловит (ревью 0.3.1): то же название даёт «Название изменено» с «Отменить».
  // Ловит (B-43 п.6): пока в строке открыто поле названия, флажок, выбор категории и «Скрыть» этой
  // строки доступны; или, наоборот, недоступны и другие строки.
  it("поле названия открыто: остальные действия этой строки недоступны, других строк доступны", async () => {
    const user = setup({ programs: withShipping() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(renameButton("Brave"));
    const brave = rowOf("Brave");
    const controls = (row: HTMLElement, name: string) =>
      [
        within(row).getByRole("checkbox", { name: `Отметить «${name}»` }),
        within(row).getByRole("combobox", { name: `Категория для «${name}»` }),
        within(row).getByRole("button", { name: `Скрыть «${name}»` }),
      ] as (HTMLInputElement | HTMLSelectElement | HTMLButtonElement)[];
    expect(controls(brave, "Brave").map((c) => c.disabled)).toEqual([true, true, true]);
    expect(controls(rowOf("Учи.ру"), "Учи.ру").map((c) => c.disabled)).toEqual([false, false, false]);

    await user.keyboard("{Escape}");
    expect(within(rowOf("Brave")).queryByRole("textbox")).toBeNull();
    expect(controls(rowOf("Brave"), "Brave").map((c) => c.disabled)).toEqual([false, false, false]);
  });

  it("то же название: «Название не изменилось» без «Отменить»", async () => {
    const user = setup({ programs: withShipping() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(renameButton("Brave"));
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Название не изменилось")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
  });

  // Ловит: длинное название уходит в службу; ошибка не у поля (aria-invalid, aria-describedby);
  // ошибка службы не показывается у поля.
  it("длиннее 60 символов: «Название до 60 символов» у поля, запроса нет", async () => {
    const user = setup({ programs: withShipping() });
    await screen.findByRole("region", { name: "Все программы" });
    await user.click(renameButton("Brave"));
    const field = within(rowOf("Brave")).getByRole("textbox");
    await user.clear(field);
    await user.type(field, "я".repeat(61));
    await user.keyboard("{Enter}");
    expect(within(rowOf("Brave")).getByText("Название до 60 символов")).toBeTruthy();
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(field.getAttribute("aria-describedby")).toBeTruthy();
    expect(document.getElementById(field.getAttribute("aria-describedby")!)?.textContent).toBe("Название до 60 символов");
    expect(mock.calls.filter((c) => c.command === "RenameProgram")).toEqual([]);

    mock.override("RenameProgram", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз", field: "name" }, 1);
    await user.clear(field);
    await user.type(field, "Браузер");
    await user.click(within(rowOf("Brave")).getByRole("button", { name: "Сохранить" }));
    expect(await within(rowOf("Brave")).findByText("Не удалось сохранить. Попробуйте ещё раз")).toBeTruthy();
    expect(within(rowOf("Brave")).getByRole("textbox")).toBeTruthy();
  });
});
