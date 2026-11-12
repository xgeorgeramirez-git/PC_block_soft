import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Sites from "./Sites";
import { installBackend, mockChild, uninstallBackend, type MockBackend, type MockOptions } from "../__mocks__/backend";
import type { SiteDTO } from "../types";

// FR-21, сценарий 8.3.5: переключатели «18+» по детям с подписью источника, три списка вкладками
// со счётчиками, «Добавить сайт» (домен разбирает служба), удаление с «Отменить» на 10 с, поиск.
// Мок отвечает как служба (T8-7): удалённый домен действительно пропадает.

let mock: MockBackend;
let openLimits: ReturnType<typeof vi.fn>;

// чт, 5 ноября 2026, 12:00 по местному времени окна.
const NOW = new Date(2026, 10, 5, 12, 0, 0);

const blocked = (...domains: string[]): SiteDTO[] => domains.map((domain) => ({ domain, kind: "blocked" }));

function setup(opts: MockOptions = {}) {
  mock = installBackend({
    sessionActive: true,
    children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", adultFilter: false })],
    ...opts,
  });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  openLimits = vi.fn();
  render(<Sites onOpenLimits={openLimits} />);
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

const tabTexts = () =>
  within(screen.getByRole("tablist", { name: "Списки сайтов" }))
    .getAllByRole("tab")
    .map((t) => t.textContent);
const tab = (name: string) => within(screen.getByRole("tablist", { name: "Списки сайтов" })).getByRole("tab", { name });
const domains = (listName: string) =>
  within(screen.getByRole("list", { name: listName }))
    .getAllByRole("listitem")
    .map((li) => li.querySelector(".sites-domain")?.textContent);
const field = () => screen.getByRole("textbox", { name: "Добавить сайт" }) as HTMLInputElement;
const checkbox = (name: string) =>
  within(screen.getByRole("group", { name: "Блокировать сайты 18+" })).getByRole("checkbox", { name }) as HTMLInputElement;
const addCalls = () => mock.calls.filter((c) => c.command === "AddSite").map((c) => c.payload);
/** Живая область сообщений списка: всегда в панели вкладки, меняется только её текст. */
const listStatus = () => within(screen.getByRole("tabpanel")).getByRole("status");
const SCOPE = "Каждый сайт действует вместе со всеми адресами внутри него: например, youtube.com вместе с m.youtube.com и music.youtube.com";
const HINT =
  "Можно вставить адрес страницы целиком: программа возьмёт из него только сайт. Можно сразу несколько, по одному в строке. Например: youtube.com";
const rowBox = (domain: string) => screen.getByRole("checkbox", { name: `Отметить «${domain}»` }) as HTMLInputElement;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("Сайты: вид", () => {
  it("заголовок, пояснение про Brave, переключатели «18+» и подпись источника над списками", async () => {
    setup();
    expect(screen.getByRole("heading", { name: "Сайты" })).toBeTruthy();
    expect(screen.getByText("Программа следит за сайтами только в Brave. Другие браузеры у детей закрываются")).toBeTruthy();
    const group = await screen.findByRole("group", { name: "Блокировать сайты 18+" });
    expect(checkbox("Миша").checked).toBe(true);
    expect(checkbox("Даша").checked).toBe(false);
    const source = screen.getByText("Список сайтов для взрослых: UT1, Университет Тулуза Капитоль, лицензия CC BY\u2011SA 4.0");
    // «CC BY-SA 4.0» не рвётся: неразрывные пробелы и неразрывный дефис в разметке (текст службы прежний).
    expect(source.textContent).toContain("лицензия\u00a0CC\u00a0BY\u2011SA\u00a04.0");
    expect(group.compareDocumentPosition(source) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const tablist = screen.getByRole("tablist", { name: "Списки сайтов" });
    expect(group.compareDocumentPosition(tablist) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(source.compareDocumentPosition(tablist) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // Блок «18+» вне панели списков (PL-36).
    expect(screen.getByRole("tabpanel").contains(group)).toBe(false);
  });

  it("вкладки со счётчиками, по умолчанию «Запрещённые» с «Сайтов: N из 1000»", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    expect(tabTexts()).toEqual(["Запрещённые 1", "Учёба 1", "Видео и соцсети 1"]);
    expect(tab("Запрещённые 1").getAttribute("aria-selected")).toBe("true");
    expect(tab("Учёба 1").getAttribute("aria-selected")).toBe("false");
    const panel = screen.getByRole("tabpanel");
    expect(panel.getAttribute("aria-labelledby")).toBe(tab("Запрещённые 1").id);
    expect(screen.getByText("Сайтов: 1 из 1000")).toBeTruthy();
    expect(
      screen.getByText("Эти сайты закрываются в Brave у всех детей"),
    ).toBeTruthy();
    // B-50 (H2): без слова «поддомены», простыми словами с примером, под пояснением каждого списка.
    expect(screen.getByText(SCOPE)).toBeTruthy();
    expect(screen.queryByText(/поддомен/)).toBeNull();
    expect(domains("Запрещённые: сайты")).toEqual(["example.com"]);

    await user.click(tab("Учёба 1"));
    expect(tab("Учёба 1").getAttribute("aria-selected")).toBe("true");
    expect(domains("Учёба: сайты")).toEqual(["school.example"]);
    expect(screen.queryByText(/из 1000/)).toBeNull();
    expect(
      screen.getByText(
        "Время на этих сайтах не тратит лимит. Когда время вышло и выбрано «Оставить только учёбу», в Brave открываются только они",
      ),
    ).toBeTruthy();

    await user.click(tab("Видео и соцсети 1"));
    expect(domains("Видео и соцсети: сайты")).toEqual(["youtube.com"]);
    expect(
      screen.getByText("Время на этих сайтах идёт в лимит «Видео и соцсети» и в общее время"),
    ).toBeTruthy();
    expect(screen.getByText(SCOPE)).toBeTruthy();
    // B-50 (H6): ссылка «Открыть лимиты» ведёт в «Дети» (вкладка «Лимиты» открывается первой).
    await user.click(screen.getByRole("button", { name: "Открыть лимиты" }));
    expect(openLimits).toHaveBeenCalledTimes(1);
  });

  it("стрелки, Home и End переключают вкладки и переводят фокус", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    act(() => tab("Запрещённые 1").focus());
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(tab("Учёба 1"));
    expect(tab("Учёба 1").getAttribute("aria-selected")).toBe("true");
    expect(tab("Учёба 1").tabIndex).toBe(0);
    expect(tab("Запрещённые 1").tabIndex).toBe(-1);
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(tab("Видео и соцсети 1"));
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(tab("Запрещённые 1"));
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(tab("Видео и соцсети 1"));
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(tab("Запрещённые 1"));
    expect(tab("Запрещённые 1").getAttribute("aria-selected")).toBe("true");
  });

  it("детей нет: подсказка вместо переключателей, вкладки есть", async () => {
    setup({ children: [] });
    expect(await screen.findByText("Детей пока нет. Добавьте их в разделе «Настройки» → «Детские учётки»")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Блокировать сайты 18+" })).toBeNull();
    expect(screen.queryAllByRole("checkbox")).toEqual([]);
    expect(tabTexts()).toEqual(["Запрещённые 1", "Учёба 1", "Видео и соцсети 1"]);
  });

  it("в текстах экрана нет длинных и средних тире", async () => {
    setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    expect(document.body.textContent).not.toMatch(/[—–]/);
  });
});

describe("Сайты: добавление", () => {
  it("вставленный адрес уходит службе как есть; сообщение службы, счётчики, поле пусто и в фокусе", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "https://www.Example.org/page{Enter}");
    expect(await within(listStatus()).findByText("Добавлено: example.org")).toBeTruthy();
    expect(listStatus().textContent).toBe("Добавлено: example.org");
    expect(tabTexts()).toEqual(["Запрещённые 2", "Учёба 1", "Видео и соцсети 1"]);
    expect(screen.getByText("Сайтов: 2 из 1000")).toBeTruthy();
    expect(domains("Запрещённые: сайты")).toEqual(["example.com", "example.org"]);
    expect(field().value).toBe("");
    expect(document.activeElement).toBe(field());
    // Экран не разбирает домен сам (T8-1, PL-39).
    expect(addCalls()).toEqual([{ domain: "https://www.Example.org/page", kind: "blocked" }]);
  });

  it("кнопка «Добавить» отправляет то же, что Enter", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "  vk.com  ");
    await user.click(screen.getByRole("button", { name: "Добавить" }));
    expect(await screen.findByText("Добавлено: vk.com")).toBeTruthy();
    expect(addCalls()).toEqual([{ domain: "vk.com", kind: "blocked" }]);
    expect(document.activeElement).toBe(field());
  });

  it("подсказка с примером под полем, поле ссылается на неё", async () => {
    setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const hint = screen.getByText(HINT);
    expect(field().getAttribute("aria-describedby")).toBe(hint.id);
    expect(hint.id).not.toBe("");
  });

  it("отказы службы — у поля: не домен и повтор; поле не очищается; ввод снимает ошибку", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "просто текст{Enter}");
    expect(await screen.findByText("Это не похоже на адрес сайта")).toBeTruthy();
    expect(field().getAttribute("aria-invalid")).toBe("true");
    expect(field().value).toBe("просто текст");
    expect(document.activeElement).toBe(field());
    // Ловит: подсказка-пример пропадает, когда появилась ошибка; диктор читает только ошибку.
    const error = screen.getByRole("alert");
    const hint = screen.getByText(HINT);
    expect(field().getAttribute("aria-describedby")).toBe(`${error.id} ${hint.id}`);
    expect(error.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    await user.clear(field());
    expect(screen.queryByText("Это не похоже на адрес сайта")).toBeNull();
    expect(field().getAttribute("aria-invalid")).toBeNull();

    await user.type(field(), "example.com{Enter}");
    expect(await screen.findByText("Этот сайт уже в списке")).toBeTruthy();
    expect(field().value).toBe("example.com");
    await user.type(field(), "x");
    expect(screen.queryByText("Этот сайт уже в списке")).toBeNull();
  });

  it("пустое поле: ошибка у поля без запроса к службе", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Добавить" }));
    expect(screen.getByText("Это не похоже на адрес сайта")).toBeTruthy();
    expect(field().getAttribute("aria-invalid")).toBe("true");
    expect(mock.count("AddSite")).toBe(0);
    expect(document.activeElement).toBe(field());
  });

  it("отказ без поля — в строке сообщений списка", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    mock.override("AddSite", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз" }, 1);
    await user.type(field(), "a.ru{Enter}");
    expect((await screen.findByRole("alert")).textContent).toBe("Не удалось сохранить. Попробуйте ещё раз");
    expect(field().getAttribute("aria-invalid")).toBeNull();
    expect(field().value).toBe("a.ru");
  });

  it("домен из другого списка переносится: сообщение службы и счётчики", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(tab("Учёба 1"));
    await user.type(field(), "youtube.com{Enter}");
    expect(await screen.findByText("Сайт перенесён в «Учёбу»")).toBeTruthy();
    expect(tabTexts()).toEqual(["Запрещённые 1", "Учёба 2", "Видео и соцсети 0"]);
    expect(addCalls()).toEqual([{ domain: "youtube.com", kind: "study" }]);
    await user.click(tab("Видео и соцсети 0"));
    expect(screen.getByText("В этом списке пока нет сайтов")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Видео и соцсети: сайты" })).toBeNull();
  });

  it("один запрос за раз: второе нажатие ничего не отправляет, кнопка «Сохраняю…»", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const g = gate("AddSite");
    await user.type(field(), "a.ru");
    const button = screen.getByRole("button", { name: "Добавить" });
    await user.click(button);
    await user.click(button);
    expect(g.counter.calls).toBe(1);
    expect(button.textContent).toBe("Сохраняю…");
    await g.open();
    expect(await screen.findByText("Добавлено: a.ru")).toBeTruthy();
    expect(button.textContent).toBe("Добавить");
  });
});

describe("Сайты: удаление и «Отменить»", () => {
  it("удалён последний: сообщение с «Отменить», фокус в поле; «Отменить» возвращает сайт", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Отменить" })).toBeTruthy();
    expect(screen.getByText("Запрещённых сайтов пока нет")).toBeTruthy();
    expect(tabTexts()[0]).toBe("Запрещённые 0");
    expect(document.activeElement).toBe(field());
    expect(screen.queryByRole("searchbox", { name: "Найти в списке" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Отменить" }));
    expect(await screen.findByText("Добавлено: example.com")).toBeTruthy();
    expect(domains("Запрещённые: сайты")).toEqual(["example.com"]);
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(tab("Запрещённые 1").getAttribute("aria-selected")).toBe("true");
    // «Отменить» — это AddSite с доменом и прежним списком из ответа (S-15).
    expect(addCalls()).toEqual([{ domain: "example.com", kind: "blocked" }]);
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Удалить example.com" }));
  });

  // Ловит: «Отменить» в «Сайтах» оформлен иначе, чем в «Программах» и «Копилке», и без срока
  // (B-50, H1): общий UndoLine — ссылка и отсчёт секунд рядом.
  it("«Отменить» — ссылка общего вида с отсчётом секунд", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    const undo = screen.getByRole("button", { name: "Отменить" });
    expect(undo.className).toBe("link");
    expect(undo.parentElement?.querySelector(".undo-left")?.textContent).toBe("10 с");
  });

  // Ловит: ошибка «Это не похоже на адрес сайта» висит рядом с «Удалён example.com».
  it("удаление убирает ошибку поля «Добавить сайт»", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "просто текст{Enter}");
    expect(await screen.findByText("Это не похоже на адрес сайта")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    expect(screen.queryByText("Это не похоже на адрес сайта")).toBeNull();
    expect(field().getAttribute("aria-invalid")).toBeNull();
  });

  it("«Отменить» пропадает через 10 с, сообщение остаётся", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(9_000);
    });
    expect(screen.getByRole("button", { name: "Отменить" })).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(1_100);
    });
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(screen.getByText("Удалён example.com")).toBeTruthy();
  });

  it("срок отмены из ответа дальше 10 с: «Отменить» всё равно не дольше 10 с", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    mock.override(
      "RemoveSite",
      (req) => {
        const r = mock.handler("RemoveSite")(req);
        return r.ok ? { ...r, payload: { ...(r.payload as object), undoUntilMs: Date.now() + 60_000 } } : r;
      },
      1,
    );
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    act(() => {
      vi.advanceTimersByTime(10_100);
    });
    expect(screen.queryByRole("button", { name: "Отменить" })).toBeNull();
  });

  it("фокус после удаления: на «Удалить» следующей строки, у последней — предыдущей", async () => {
    const user = setup({ sites: blocked("a.test", "b.test", "c.test") });
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Удалить b.test" }));
    expect(await screen.findByText("Удалён b.test")).toBeTruthy();
    expect(domains("Запрещённые: сайты")).toEqual(["a.test", "c.test"]);
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Удалить c.test" }));

    await user.click(screen.getByRole("button", { name: "Удалить c.test" }));
    expect(await screen.findByText("Удалён c.test")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Удалить a.test" }));
  });

  // Ловит: живая область появляется вместе с сообщением, и диктор пропускает «Удалён …».
  it("живые области сообщений есть до действия, сообщение меняет только их текст", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const list = listStatus();
    expect(list.textContent).toBe("");
    const adult = within(screen.getByRole("region", { name: "Блокировать сайты 18+" })).getByRole("status");
    expect(adult.textContent).toBe("");
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await within(list).findByText("Удалён example.com")).toBeTruthy();
    expect(listStatus()).toBe(list);
    expect(within(list).getByRole("button", { name: "Отменить" })).toBeTruthy();
    // Внутри живой области нет второй: одна область на место сообщения.
    expect(list.querySelectorAll('[role="status"], [role="alert"]')).toHaveLength(0);
    await user.click(checkbox("Миша"));
    expect(await within(adult).findByText("Миша: сайты 18+ не блокируются")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Блокировать сайты 18+" })).getByRole("status")).toBe(adult);
  });

  // Ловит: вкладка переключается, пока служба удаляет сайт, и «Удалён …» показывается не на своей вкладке.
  it("пока идёт запрос, вкладки не переключаются ни мышью, ни стрелками", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const g = gate("RemoveSite");
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    await user.click(tab("Учёба 1"));
    expect(tab("Запрещённые 1").getAttribute("aria-selected")).toBe("true");
    expect(tab("Учёба 1").getAttribute("aria-selected")).toBe("false");
    act(() => tab("Запрещённые 1").focus());
    await user.keyboard("{ArrowRight}");
    expect(tab("Запрещённые 1").getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(tab("Запрещённые 1"));
    await g.open();
    expect(await within(listStatus()).findByText("Удалён example.com")).toBeTruthy();
    expect(tab("Запрещённые 0").getAttribute("aria-selected")).toBe("true");
    // После ответа вкладки снова переключаются.
    await user.click(tab("Учёба 1"));
    expect(tab("Учёба 1").getAttribute("aria-selected")).toBe("true");
  });

  // Ловит: набранный во «Запрещённых» адрес уходит в «Учёбу», потому что поле не очистилось.
  it("смена вкладки очищает поле «Добавить сайт»", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "vk.com");
    await user.click(tab("Учёба 1"));
    expect(field().value).toBe("");
    await user.type(field(), "uchi.ru");
    act(() => tab("Учёба 1").focus());
    await user.keyboard("{ArrowRight}");
    expect(tab("Видео и соцсети 1").getAttribute("aria-selected")).toBe("true");
    expect(field().value).toBe("");
    expect(addCalls()).toEqual([]);
  });

  it("смена вкладки убирает сообщение списка, ошибку поля и поиск", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(screen.getByRole("button", { name: "Удалить example.com" }));
    expect(await screen.findByText("Удалён example.com")).toBeTruthy();
    await user.click(tab("Учёба 1"));
    expect(screen.queryByText("Удалён example.com")).toBeNull();
    await user.type(field(), "просто текст{Enter}");
    expect(await screen.findByText("Это не похоже на адрес сайта")).toBeTruthy();
    await user.click(tab("Запрещённые 0"));
    expect(screen.queryByText("Это не похоже на адрес сайта")).toBeNull();
  });
});

// B-49 (UX-аудит M4, H7): списки до 1000 сайтов, а добавление было по одному адресу и удаление
// по строке. Вставка нескольких адресов по одному в строке и удаление отмеченных строк.
describe("Сайты: несколько сайтов за раз", () => {
  // Ловит: вставка нескольких строк уходит одним адресом (AddSite) и получает «не похоже на адрес»;
  // строки, не похожие на адрес, пропадают из поля; итог службы не показан.
  it("вставка по одному в строке: один запрос AddSites, не-адреса остаются в поле с ошибкой", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.click(field());
    await user.paste("vk.com\nhttps://www.New.example/page\nпросто текст\nexample.com");
    expect(field().value).toBe("vk.com\nhttps://www.New.example/page\nпросто текст\nexample.com");
    expect(field().getAttribute("rows")).toBe("4");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Добавлено сайтов: 2. Уже были в списке: 1")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "AddSites").map((c) => c.payload)).toEqual([
      { text: "vk.com\nhttps://www.New.example/page\nпросто текст\nexample.com", kind: "blocked" },
    ]);
    expect(mock.count("AddSite")).toBe(0);
    expect(domains("Запрещённые: сайты")).toEqual(["example.com", "new.example", "vk.com"]);
    expect(field().value).toBe("просто текст");
    expect(screen.getByRole("alert").textContent).toBe("Это не похоже на адрес сайта");
    expect(field().getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(field());
  });

  // Ловит: Enter в многострочном поле переносит строку вместо «Добавить»; Shift+Enter добавляет.
  it("Shift+Enter — новая строка, Enter — добавить", async () => {
    const user = setup();
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(field(), "a.ru{Shift>}{Enter}{/Shift}b.ru");
    expect(field().value).toBe("a.ru\nb.ru");
    expect(mock.count("AddSites")).toBe(0);
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Добавлено сайтов: 2")).toBeTruthy();
    expect(field().value).toBe("");
    expect(field().getAttribute("rows")).toBe("1");
  });

  // Ловит: «Удалить отмеченные» удаляет по одному запросом на строку, удаляет скрытые поиском,
  // не говорит сколько; «Отменить» не возвращает их; отметка переживает смену вкладки.
  it("отмеченные удаляются одним запросом, «Отменить» возвращает все", async () => {
    const user = setup({ sites: blocked("a.test", "b.test", "c.test") });
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const bulk = () => screen.queryByRole("button", { name: "Удалить отмеченные" });
    expect(bulk()).toBeNull();
    await user.click(rowBox("a.test"));
    await user.click(rowBox("c.test"));
    expect(screen.getByText("Выбрано: 2")).toBeTruthy();
    await user.click(bulk()!);
    expect(await screen.findByText("Удалено сайтов: 2")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "RemoveSites").map((c) => c.payload)).toEqual([{ domains: ["a.test", "c.test"] }]);
    expect(mock.count("RemoveSite")).toBe(0);
    expect(domains("Запрещённые: сайты")).toEqual(["b.test"]);
    expect(bulk()).toBeNull();
    expect(within(listStatus()).getByText("10 с")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Отменить" }));
    expect(await screen.findByText("Добавлено сайтов: 2")).toBeTruthy();
    expect(mock.calls.filter((c) => c.command === "AddSites").map((c) => c.payload)).toEqual([{ text: "a.test\nc.test", kind: "blocked" }]);
    expect(domains("Запрещённые: сайты")).toEqual(["a.test", "b.test", "c.test"]);
  });

  // Ловит: «Отметить все» отмечает и скрытые поиском; повторное нажатие не снимает отметку;
  // отметка остаётся после перехода на другой список.
  it("«Отметить все» — только видимые строки; смена вкладки снимает отметку", async () => {
    const user = setup({ sites: blocked("a.test", "b.test", "x.example") });
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(screen.getByRole("searchbox", { name: "Найти в списке" }), ".test");
    const all = screen.getByRole("checkbox", { name: "Выбрать всё в списке" }) as HTMLInputElement;
    await user.click(all);
    expect([rowBox("a.test").checked, rowBox("b.test").checked]).toEqual([true, true]);
    expect(screen.getByText("Выбрано: 2")).toBeTruthy();
    await user.click(all);
    expect([rowBox("a.test").checked, rowBox("b.test").checked]).toEqual([false, false]);
    await user.click(rowBox("a.test"));
    expect(all.indeterminate).toBe(true);
    await user.click(tab("Учёба 1"));
    await user.click(tab("Запрещённые 3"));
    expect(rowBox("a.test").checked).toBe(false);
  });
});

describe("Сайты: переключатели «18+»", () => {
  it("снять у Миши: запрос службе, сообщение службы, пока идёт — переключатели недоступны", async () => {
    const user = setup();
    await screen.findByRole("group", { name: "Блокировать сайты 18+" });
    const g = gate("SetAdultFilter");
    await user.click(checkbox("Миша"));
    expect(checkbox("Миша").disabled).toBe(true);
    expect(checkbox("Даша").disabled).toBe(true);
    // Не оптимистично: до ответа службы переключатель как был (T8-2).
    expect(checkbox("Миша").checked).toBe(true);
    await g.open();
    expect(await screen.findByText("Миша: сайты 18+ не блокируются")).toBeTruthy();
    expect(checkbox("Миша").checked).toBe(false);
    expect(checkbox("Миша").disabled).toBe(false);
    expect(document.activeElement).toBe(checkbox("Миша"));
    expect(mock.calls.filter((c) => c.command === "SetAdultFilter").map((c) => c.payload)).toEqual([{ childId: 1, on: false }]);
  });

  it("включить у Даши", async () => {
    const user = setup();
    await screen.findByRole("group", { name: "Блокировать сайты 18+" });
    await user.click(checkbox("Даша"));
    expect(await screen.findByText("Даша: сайты 18+ блокируются")).toBeTruthy();
    expect(checkbox("Даша").checked).toBe(true);
  });

  it("отказ службы: ошибка в блоке «18+», переключатель остаётся как был", async () => {
    const user = setup();
    await screen.findByRole("group", { name: "Блокировать сайты 18+" });
    mock.override("SetAdultFilter", { ok: false, error: "Не удалось сохранить. Попробуйте ещё раз" }, 1);
    await user.click(checkbox("Миша"));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Не удалось сохранить. Попробуйте ещё раз");
    expect(screen.getByRole("region", { name: "Блокировать сайты 18+" }).contains(alert)).toBe(true);
    expect(checkbox("Миша").checked).toBe(true);
  });
});

describe("Сайты: поиск", () => {
  const twelve = () => blocked(...Array.from({ length: 12 }, (_, i) => `s${i + 1}.test`));

  it("подстрока домена; «Ничего не найдено» и «Сбросить поиск»", async () => {
    const user = setup({ sites: twelve() });
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    const search = screen.getByRole("searchbox", { name: "Найти в списке" });
    await user.type(search, "s1");
    expect(domains("Запрещённые: сайты")).toEqual(["s1.test", "s10.test", "s11.test", "s12.test"]);
    await user.clear(search);
    await user.type(search, "zzz");
    expect(screen.getByText("Ничего не найдено")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Сбросить поиск" }));
    expect(domains("Запрещённые: сайты")).toHaveLength(12);
    expect((screen.getByRole("searchbox", { name: "Найти в списке" }) as HTMLInputElement).value).toBe("");
  });

  it("смена вкладки очищает поиск; у пустого списка поиска нет", async () => {
    const user = setup({ sites: twelve() });
    await screen.findByRole("tablist", { name: "Списки сайтов" });
    await user.type(screen.getByRole("searchbox", { name: "Найти в списке" }), "s1");
    await user.click(tab("Учёба 0"));
    expect(screen.getByText("Учебных сайтов пока нет")).toBeTruthy();
    expect(screen.queryByRole("searchbox", { name: "Найти в списке" })).toBeNull();
    await user.click(tab("Запрещённые 12"));
    expect((screen.getByRole("searchbox", { name: "Найти в списке" }) as HTMLInputElement).value).toBe("");
    expect(domains("Запрещённые: сайты")).toHaveLength(12);
  });
});

describe("Сайты: загрузка и ошибки", () => {
  it("пока данные идут: «Загружаю данные»", async () => {
    setup();
    expect(screen.getByText("Загружаю данные…")).toBeTruthy();
    expect(await screen.findByRole("tablist", { name: "Списки сайтов" })).toBeTruthy();
    expect(screen.queryByText("Загружаю данные…")).toBeNull();
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    const user = setup({ linkDown: true });
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    mock.state.linkDown = false;
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("tablist", { name: "Списки сайтов" })).toBeTruthy();
    expect(domains("Запрещённые: сайты")).toEqual(["example.com"]);
  });

  it("пустые списки приходят как null: экран не падает", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("ListSites", { ok: true, payload: { sites: null, children: null, blockedMax: 1000, adultSource: "" } });
    render(<Sites />);
    expect(await screen.findByText("Запрещённых сайтов пока нет")).toBeTruthy();
    expect(tabTexts()).toEqual(["Запрещённые 0", "Учёба 0", "Видео и соцсети 0"]);
    expect(screen.getByText("Сайтов: 0 из 1000")).toBeTruthy();
  });
});
