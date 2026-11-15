import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import StartupList from "./StartupList";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import type { Reply } from "../types";

// Вкладка «Автозагрузка» (FR-39, ТЗ 8.3.11, Review Focus 4, PL-59). Ожидания — литералы из ТЗ.

let mock: MockBackend;

const MISHA = mockChild();
const DASHA = mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша" });

/** prep - подмены ответов до первого чтения списка. */
function setup(opts: Parameters<typeof installBackend>[0] = {}, prep?: (m: MockBackend) => void) {
  mock = installBackend({ sessionActive: true, children: [MISHA, DASHA], ...opts });
  prep?.(mock);
  const user = userEvent.setup();
  const view = render(<StartupList childId={1} childName="Миша" />);
  return { user, view };
}

const sw = (name: string) => screen.getByRole("switch", { name });
const block = (title: string) => screen.getByRole("region", { name: title });
const status = () => document.querySelector(".startup-message") as HTMLElement;
const calls = (command: string) => mock.calls.filter((c) => c.command === command).map((c) => c.payload);

/** Отложенный ответ подмены: тест сам решает, когда служба ответит. */
function deferred() {
  let resolve!: (r: Reply) => void;
  const promise = new Promise<Reply>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

beforeEach(() => uninstallBackend());
afterEach(() => uninstallBackend());

describe("StartupList: блоки и строки", () => {
  // Ловит: блоки не в порядке ТЗ, ждущая запись в «Учётке», у задания весь путь с SID, у строки нет
  // издателя или «откуда»; вкладка не просит службу проверить Windows при открытии.
  it("«Ждут решения» сверху, затем «Учётка Миши» и «Для всех пользователей»", async () => {
    setup();
    await screen.findByRole("heading", { name: "Ждут решения" });
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual(["Ждут решения", "Учётка Миши", "Для всех пользователей"]);
    const names = (title: string) => within(block(title)).getAllByRole("switch").map((s) => s.getAttribute("aria-label"));
    expect(names("Ждут решения")).toEqual(["notes"]);
    expect(names("Учётка Миши")).toEqual(["OneDrive Reporting Task", "Discord", "Telegram.lnk"]);
    expect(names("Для всех пользователей")).toEqual(["Adobe Updater", "Steam.lnk"]);
    const states = screen.getAllByRole("switch").map((s) => s.getAttribute("aria-checked"));
    expect(states).toEqual(["false", "true", "true", "false", "true", "true"]);

    const onedrive = sw("OneDrive Reporting Task").closest("li")!;
    expect(within(onedrive).getByText("OneDrive Reporting Task").getAttribute("title")).toBe("\\OneDrive Reporting Task-S-1-5-21-111-222-333-1001");
    expect(within(onedrive).getByText("Microsoft Corporation")).toBeTruthy();
    expect(within(onedrive).getByText("Планировщик заданий, при входе")).toBeTruthy();
    expect(within(sw("notes").closest("li")!).getByText("Реестр, при входе")).toBeTruthy();
    expect(within(sw("Steam.lnk").closest("li")!).getByText("Папка «Автозагрузка»")).toBeTruthy();
    expect(onedrive.querySelector("svg.startup-icon-task")).toBeTruthy();
    expect(sw("Discord").closest("li")!.querySelector("svg.startup-icon-registry")).toBeTruthy();
    expect(sw("Steam.lnk").closest("li")!.querySelector("svg.startup-icon-folder")).toBeTruthy();
    // Издателя нет - строки издателя нет.
    expect(sw("notes").closest("li")!.querySelector(".startup-publisher")).toBeNull();

    expect(screen.getByText(/^Проверено в \d\d:\d\d$/)).toBeTruthy();
    expect(calls("ListStartup")[0]).toEqual({ childId: 1, refresh: true });
    expect(calls("SetStartup")).toEqual([]);
  });

  // Ловит: пустой блок показывается заголовком без строк.
  it("пустой блок не показывается: нет ждущих - нет «Ждут решения»", async () => {
    setup({ startup: [{ id: 2, scope: "own", source: "store_app", name: "SpotifyMusic", publisher: "", from: "Приложение из магазина", decision: "off" }] });
    await screen.findByRole("heading", { name: "Учётка Миши" });
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual(["Учётка Миши"]);
    expect(sw("SpotifyMusic").closest("li")!.querySelector("svg.startup-icon-store")).toBeTruthy();
  });

  it("пусто - «При входе Миши ничего не запускается»", async () => {
    setup({ startup: [] });
    expect(await screen.findByText("При входе Миши ничего не запускается")).toBeTruthy();
    expect(screen.queryAllByRole("heading", { level: 3 })).toEqual([]);
    expect(screen.queryAllByRole("switch")).toEqual([]);
  });

  // Ловит: «Проверено в …» стоит, пока служба ещё проверяет Windows.
  it("служба проверяет - «Проверяю…»", async () => {
    setup({}, (m) =>
      m.override("ListStartup", { ok: true, payload: { pending: [], own: [], shared: [], checkedAt: 1760000000000, checking: true } }, 1),
    );
    expect(await screen.findByText("Проверяю…")).toBeTruthy();
    expect(screen.queryByText(/^Проверено/)).toBeNull();
  });

  // Ловит: сбой чтения оставляет «Загружаю…» навсегда, без «Повторить».
  it("сбой чтения: текст для человека и «Повторить»", async () => {
    const { user } = setup({}, (m) => m.override("ListStartup", "reject", 1));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("switch", { name: "Discord" })).toBeTruthy();
    expect(calls("ListStartup").at(-1)).toEqual({ childId: 1, refresh: true });
  });
});

describe("StartupList: выключатель", () => {
  // Ловит (Review Focus 4): включённая из «Ждут решения» запись остаётся там или видна дважды;
  // нужен «Сохранить»; сообщение не в живой области под списком.
  it("включил запись из «Ждут решения» - строка в «Учётке Миши», сообщение под списком", async () => {
    const { user } = setup();
    await user.click(await screen.findByRole("switch", { name: "notes" }));
    expect(await within(status()).findByText("Будет запускаться при входе Миши")).toBeTruthy();
    expect(status().getAttribute("role")).toBe("status");
    expect(calls("SetStartup")).toEqual([{ childId: 1, id: 7, on: true }]);
    expect(screen.queryByRole("heading", { name: "Ждут решения" })).toBeNull();
    expect(screen.getAllByRole("switch", { name: "notes" })).toHaveLength(1);
    expect(within(block("Учётка Миши")).getAllByRole("switch").map((s) => s.getAttribute("aria-label"))).toEqual([
      "OneDrive Reporting Task",
      "Discord",
      "Telegram.lnk",
      "notes",
    ]);
    expect(sw("notes").getAttribute("aria-checked")).toBe("true");
    // Фокус переехал вместе со строкой, а не потерялся.
    expect(document.activeElement).toBe(sw("notes"));
    // После действия список перечитан у службы, без новой проверки Windows.
    await vi.waitFor(() => expect(calls("ListStartup").at(-1)).toEqual({ childId: 1 }));
    expect(screen.queryByText("Есть несохранённые изменения")).toBeNull();
  });

  // Ловит (8.3.11 п.2, ревью T5): ждущую запись нельзя одним действием оставить выключенной; кнопка
  // есть у решённых строк; во время запроса можно нажать дважды; фокус теряется после переезда.
  it("«Оставить выключенной» - строка в «Учётке Миши» с выключенным выключателем", async () => {
    const { user } = setup();
    const keep = await screen.findByRole("button", { name: "Оставить выключенной: notes" });
    expect(keep.textContent).toBe("Оставить выключенной");
    expect(screen.getAllByRole("button", { name: /^Оставить выключенной/ })).toHaveLength(1);
    expect(within(sw("notes").closest("li")!).getByRole("button", { name: "Оставить выключенной: notes" })).toBe(keep);

    const wait = deferred();
    mock.override("SetStartup", () => wait.promise, 1);
    await user.click(keep);
    expect(keep.getAttribute("aria-disabled")).toBe("true");
    expect(sw("notes").getAttribute("aria-disabled")).toBe("true");
    expect(sw("notes").getAttribute("aria-checked")).toBe("false");
    await user.click(keep);
    expect(calls("SetStartup")).toEqual([{ childId: 1, id: 7, on: false }]);
    wait.resolve(mock.handler("SetStartup")({ childId: 1, id: 7, on: false }));

    expect(await within(status()).findByText("Не будет запускаться при входе Миши")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Ждут решения" })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Оставить выключенной/ })).toBeNull();
    expect(within(block("Учётка Миши")).getByRole("switch", { name: "notes" }).getAttribute("aria-checked")).toBe("false");
    expect(screen.getAllByRole("switch", { name: "notes" })).toHaveLength(1);
    expect(document.activeElement).toBe(sw("notes"));
    expect(mock.state.startup.get(1)?.find((x) => x.id === 7)?.decision).toBe("off");
  });

  // Ловит: выключатель показывает нажатие, а не ответ службы; второй щелчок во время запроса шлёт
  // второе решение.
  it("во время запроса выключатель недоступен, затем показывает решение службы", async () => {
    const { user } = setup();
    await screen.findByRole("switch", { name: "Discord" });
    const wait = deferred();
    mock.override("SetStartup", () => wait.promise, 1);
    await user.click(sw("Discord"));
    expect(sw("Discord").getAttribute("aria-checked")).toBe("false");
    expect(sw("Discord").getAttribute("aria-disabled")).toBe("true");
    await user.click(sw("Discord"));
    expect(calls("SetStartup")).toEqual([{ childId: 1, id: 2, on: false }]);
    wait.resolve(mock.handler("SetStartup")({ childId: 1, id: 2, on: false }));
    expect(await within(status()).findByText("Не будет запускаться при входе Миши")).toBeTruthy();
    expect(sw("Discord").getAttribute("aria-checked")).toBe("false");
    expect(sw("Discord").getAttribute("aria-disabled")).toBeNull();
  });

  // Ловит: при сбое службы выключатель остаётся в нажатом положении, текста нет или он технический.
  it("сбой службы - выключатель возвращается, «Не удалось сохранить. Попробуйте ещё раз»", async () => {
    const { user } = setup();
    await screen.findByRole("switch", { name: "Discord" });
    mock.override("SetStartup", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз" }, 1);
    await user.click(sw("Discord"));
    expect(await within(status()).findByText("Не удалось сохранить. Попробуйте ещё раз")).toBeTruthy();
    expect(sw("Discord").getAttribute("aria-checked")).toBe("true");
    expect(sw("Discord").getAttribute("aria-disabled")).toBeNull();
    expect(within(block("Учётка Миши")).getByRole("switch", { name: "Discord" })).toBeTruthy();
    expect(mock.state.startup.get(1)?.find((x) => x.id === 2)?.decision).toBe("on");
  });

  // Ловит: сбой связи у ждущей записи переносит её в «Учётку» как решённую.
  it("сбой связи у ждущей записи - строка остаётся в «Ждут решения»", async () => {
    const { user } = setup();
    await screen.findByRole("switch", { name: "notes" });
    mock.override("SetStartup", "reject", 1);
    await user.click(sw("notes"));
    expect(await within(status()).findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(within(block("Ждут решения")).getByRole("switch", { name: "notes" }).getAttribute("aria-checked")).toBe("false");
  });
});

describe("StartupList: смена ребёнка (PL-59)", () => {
  // Ловит: до ответа о новом ребёнке видны строки и выключатели прежнего (нажатие решит не за того).
  it("до ответа - «Загружаю…», строк прежнего нет; затем строки нового", async () => {
    const { view } = setup();
    await screen.findByRole("switch", { name: "Discord" });
    const wait = deferred();
    mock.override("ListStartup", (p: { childId: number }) => (p.childId === 2 ? wait.promise : mock.handler("ListStartup")(p)));
    view.rerender(<StartupList childId={2} childName="Даша" />);
    expect(await screen.findByText("Загружаю…")).toBeTruthy();
    expect(screen.queryAllByRole("switch")).toEqual([]);
    expect(screen.queryByRole("heading", { name: "Учётка Миши" })).toBeNull();
    wait.resolve(mock.handler("ListStartup")({ childId: 2 }));
    expect(await screen.findByRole("heading", { name: "Учётка Даши" })).toBeTruthy();
    expect(calls("ListStartup").at(-1)).toEqual({ childId: 2, refresh: true });
  });

  // Ловит: опоздавший ответ решения прежнего ребёнка пишет «…при входе Миши» на вкладке Даши.
  it("ответ решения прежнего ребёнка после смены не показывается", async () => {
    const { user, view } = setup();
    await screen.findByRole("switch", { name: "Discord" });
    const wait = deferred();
    mock.override("SetStartup", () => wait.promise, 1);
    await user.click(sw("Discord"));
    view.rerender(<StartupList childId={2} childName="Даша" />);
    await screen.findByRole("heading", { name: "Учётка Даши" });
    wait.resolve(mock.handler("SetStartup")({ childId: 1, id: 2, on: false }));
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByText("Не будет запускаться при входе Миши")).toBeNull();
    expect(sw("Discord").getAttribute("aria-checked")).toBe("true");
    expect(sw("Discord").getAttribute("aria-disabled")).toBeNull();
  });
});
