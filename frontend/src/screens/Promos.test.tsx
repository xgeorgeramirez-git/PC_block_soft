import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Promos from "./Promos";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// Раздел «Промокоды» (FR-35, R26, сценарий 8.3.10, RM5-11…RM5-14). Ожидания - литералы из ТЗ и ответов службы.

let mock: MockBackend;
const NOW = new Date(2026, 9, 19, 12, 0, 0); // пн, 19 октября 2026
const CREATED = new Date(2026, 9, 19, 11, 0, 0).getTime();
const USED_AT = new Date(2026, 9, 19, 11, 40, 0).getTime();

// Родителю на «вы»: ни «ты», ни «попробуй». \b в JS не видит кириллицу, поэтому границы через \p{L}.
const THOU_FORM = /(?<!\p{L})(ты|тебе|тебя|твой|твоя|твои|твоё|попробуй|введи|выдели)(?!\p{L})/iu;

type Opts = Parameters<typeof installBackend>[0];

function setup(opts: Opts = {}) {
  mock = installBackend({
    sessionActive: true,
    children: [mockChild({})],
    promos: [
      { codeDisplay: "a1234", minutes: 30, createdAtMs: CREATED },
      { codeDisplay: "Z9Z9", minutes: 15, createdAtMs: CREATED - 3600_000 },
      { codeDisplay: "pizza", minutes: 20, createdAtMs: CREATED - 86400_000, usedAtMs: USED_AT, usedByChild: 1 },
    ],
    ...opts,
  });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<Promos />);
  return user;
}

const tab = (name: RegExp) => screen.getByRole("tab", { name });
const createBox = () => screen.getByRole("region", { name: "Создать код" });
const genBox = () => screen.getByRole("region", { name: "Придумать коды" });
const activeList = () => screen.findByRole("list", { name: "Действующие промокоды" });

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("вкладки «Действующие» и «Использованные» (8.3.10 п.1)", () => {
  // Ловит: числа во вкладках не те, вкладки без стрелок или Home/End (единственная в порядке Tab).
  it("«Действующие (2)», «Использованные (1)»; стрелки, Home и End", async () => {
    const user = setup();
    await activeList();
    const active = tab(/^Действующие/);
    const used = tab(/^Использованные/);
    expect(active.textContent).toBe("Действующие (2)");
    expect(used.textContent).toBe("Использованные (1)");
    expect(active.getAttribute("aria-selected")).toBe("true");
    expect(active.tabIndex).toBe(0);
    expect(used.tabIndex).toBe(-1);
    expect(screen.getByRole("tabpanel").getAttribute("aria-labelledby")).toBe(active.id);

    active.focus();
    await user.keyboard("{ArrowRight}");
    expect(used.getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(used);
    expect(screen.getByRole("tabpanel").getAttribute("aria-labelledby")).toBe(used.id);
    expect(screen.getByRole("list", { name: "Использованные промокоды" })).toBeTruthy();
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(active);
    expect(active.getAttribute("aria-selected")).toBe("true");
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(used);
    await user.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(active);
    await user.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(used);
  });

  // Ловит: пустые списки без слов или «Очистить историю» на пустой вкладке.
  it("пустые вкладки: понятные заглушки, без «Очистить историю»", async () => {
    const user = setup({ promos: [] });
    expect(await screen.findByText("Действующих кодов нет")).toBeTruthy();
    expect(tab(/^Действующие/).textContent).toBe("Действующие (0)");
    await user.click(tab(/^Использованные/));
    expect(screen.getByText("Использованных кодов нет")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Очистить историю" })).toBeNull();
  });

  // Ловит: код не заглавными (RM5-11), месяц сокращением «19 окт» вместо «19 октября в 11:00», нет имени ребёнка.
  it("строки: код заглавными, минуты, когда создан и кем использован", async () => {
    const user = setup();
    const list = await activeList();
    const first = within(list).getAllByRole("listitem")[0];
    expect(within(first).getByText("A1234")).toBeTruthy();
    expect(within(first).getByText("30 мин")).toBeTruthy();
    expect(within(first).getByText("создан 19 октября в 11:00")).toBeTruthy();
    await user.click(tab(/^Использованные/));
    const used = screen.getByRole("list", { name: "Использованные промокоды" });
    expect(within(used).getByText("PIZZA")).toBeTruthy();
    expect(within(used).getByText("20 мин")).toBeTruthy();
    expect(within(used).getByText("использован 19 октября в 11:40, Миша")).toBeTruthy();
  });

  it("использован без имени ребёнка: без запятой в конце", async () => {
    const user = setup({
      promos: [{ codeDisplay: "OLD1", minutes: 10, createdAtMs: CREATED, usedAtMs: USED_AT, usedByChild: 99 }],
    });
    await screen.findByText("Действующих кодов нет");
    await user.click(tab(/^Использованные/));
    expect(screen.getByText("использован 19 октября в 11:40")).toBeTruthy();
  });

  // Ловит (ревью окна M5): без времени использования строка «использован , Миша» с пробелом перед запятой.
  it("использован без времени: «использован, Миша»", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild({})] });
    mock.override("ListPromos", {
      ok: true,
      payload: { active: [], used: [{ codeDisplay: "OLD1", minutes: 10, createdAtMs: CREATED, usedByName: "Миша" }] },
    });
    const user = userEvent.setup({ advanceTimers: (ms: number) => vi.advanceTimersByTime(ms) });
    render(<Promos />);
    await screen.findByText("Действующих кодов нет");
    await user.click(tab(/^Использованные/));
    const used = screen.getByRole("list", { name: "Использованные промокоды" });
    expect(within(used).getByText("использован, Миша")).toBeTruthy();
    expect(used.textContent).not.toMatch(/ ,/);
  });

  // Ловит: сбой загрузки показывает технический текст или тупик без «Повторить».
  it("сбой загрузки: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true, children: [mockChild({})] });
    mock.override("ListPromos", "reject", 1);
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Promos />);
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByText("Действующих кодов нет")).toBeTruthy();
  });
});

describe("«Создать код» (8.3.10 п.2, RM5-11)", () => {
  // Ловит: созданный код не попадает в «Действующие» или показан не заглавными; поле не очищено.
  it("«ab12» → в «Действующих» «AB12» и сообщение службы", async () => {
    const user = setup();
    await activeList();
    const box = createBox();
    const code = within(box).getByLabelText("Код") as HTMLInputElement;
    expect(code.getAttribute("autocomplete")).toBe("off");
    expect(code.getAttribute("spellcheck")).toBe("false");
    expect(code.getAttribute("autocapitalize")).toBe("characters");
    expect(code.getAttribute("placeholder")).toBe("A1234");
    await user.type(code, "ab12");
    const minutes = within(box).getByLabelText("Минуты");
    await user.clear(minutes);
    await user.type(minutes, "45");
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    expect(await screen.findByText("Промокод «AB12» на 45 минут готов")).toBeTruthy();
    const list = await activeList();
    expect(within(list).getByText("AB12")).toBeTruthy();
    expect(within(list).getByText("45 мин")).toBeTruthy();
    expect(tab(/^Действующие/).textContent).toBe("Действующие (3)");
    expect(code.value).toBe("");
    expect(mock.calls.find((c) => c.command === "CreatePromo")?.payload).toEqual({ code: "ab12", minutes: 45 });
  });

  // Ловит: окно само отбрасывает знаки (служба не видит, что ввёл родитель) или ошибка общим абзацем.
  it("«код!» → у поля «Код» текст службы, aria-invalid, фокус; списки прежние", async () => {
    const user = setup();
    await activeList();
    const box = createBox();
    const code = within(box).getByLabelText("Код") as HTMLInputElement;
    await user.type(code, "код!");
    expect(code.value).toBe("код!");
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    const error = await within(box).findByText("Только латинские буквы A-Z и цифры");
    expect(code.getAttribute("aria-invalid")).toBe("true");
    expect(code.getAttribute("aria-describedby")).toBe(error.id);
    expect(document.activeElement).toBe(code);
    expect(within(box).getByLabelText("Минуты").getAttribute("aria-invalid")).toBeNull();
    expect(mock.calls.find((c) => c.command === "CreatePromo")?.payload).toEqual({ code: "код!", minutes: 30 });
    expect(tab(/^Действующие/).textContent).toBe("Действующие (2)");
  });

  // Ловит (R26): код из допустимых знаков, но короче 3, получает текст о знаках вместо текста о длине.
  it("«ab» → у поля «Код» «Код от 3 до 16 знаков»", async () => {
    const user = setup();
    await activeList();
    const box = createBox();
    const code = within(box).getByLabelText("Код") as HTMLInputElement;
    await user.type(code, "ab");
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    const error = await within(box).findByText("Код от 3 до 16 знаков");
    expect(code.getAttribute("aria-invalid")).toBe("true");
    expect(code.getAttribute("aria-describedby")).toBe(error.id);
    expect(within(box).queryByText("Только латинские буквы A-Z и цифры")).toBeNull();
    expect(tab(/^Действующие/).textContent).toBe("Действующие (2)");
  });

  // Ловит: повтор кода (другой регистр, пробелы по краям) создаёт второй такой же код.
  it("повтор кода → «Такой код уже есть, придумайте другой»", async () => {
    const user = setup();
    await activeList();
    const box = createBox();
    await user.type(within(box).getByLabelText("Код"), " a1234 ");
    await user.keyboard("{Enter}");
    expect(await within(box).findByText("Такой код уже есть, придумайте другой")).toBeTruthy();
    expect(within(box).getByLabelText("Код").getAttribute("aria-invalid")).toBe("true");
    expect(tab(/^Действующие/).textContent).toBe("Действующие (2)");
  });

  // Ловит: пустой код уходит службе; минуты 0 без ошибки у своего поля.
  it("пустой код и 0 минут - ошибка у своего поля без вызова службы", async () => {
    const user = setup();
    await activeList();
    const box = createBox();
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    expect(within(box).getByText("Введите код")).toBeTruthy();
    expect(document.activeElement).toBe(within(box).getByLabelText("Код"));
    await user.type(within(box).getByLabelText("Код"), "XY12");
    const minutes = within(box).getByLabelText("Минуты");
    await user.clear(minutes);
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    expect(within(box).getByText("Можно от 1 до 600 минут")).toBeTruthy();
    expect(minutes.getAttribute("aria-invalid")).toBe("true");
    expect(document.activeElement).toBe(minutes);
    expect(mock.count("CreatePromo")).toBe(0);
  });

  // Ловит: отказ службы у поля минут показан у поля кода.
  it("отказ службы у поля «minutes» - ошибка у «Минуты»", async () => {
    const user = setup();
    await activeList();
    mock.override("CreatePromo", { ok: false, error: "Можно от 1 до 600 минут", field: "minutes" }, 1);
    const box = createBox();
    await user.type(within(box).getByLabelText("Код"), "XY12");
    await user.click(within(box).getByRole("button", { name: "Создать" }));
    await within(box).findByText("Можно от 1 до 600 минут");
    expect(within(box).getByLabelText("Минуты").getAttribute("aria-invalid")).toBe("true");
    expect(within(box).getByLabelText("Код").getAttribute("aria-invalid")).toBeNull();
  });
});

describe("«Придумать коды» (8.3.10 п.3, RM5-12)", () => {
  // Ловит: значения по умолчанию не по ТЗ (10 / 6 / «Буквы и цифры» / 30).
  it("по умолчанию 10, 6, «Буквы и цифры», 30", async () => {
    setup();
    await activeList();
    const box = genBox();
    expect((within(box).getByLabelText("Сколько") as HTMLInputElement).value).toBe("10");
    expect((within(box).getByLabelText("Длина") as HTMLInputElement).value).toBe("6");
    expect((within(box).getByLabelText("Минуты") as HTMLInputElement).value).toBe("30");
    const group = within(box).getByRole("group", { name: "Из чего" });
    expect(within(group).getAllByRole("radio").map((r) => r.parentElement?.textContent)).toEqual([
      "Только цифры",
      "Только буквы",
      "Буквы и цифры",
    ]);
    expect((within(group).getByRole("radio", { name: "Буквы и цифры" }) as HTMLInputElement).checked).toBe(true);
    expect(within(box).queryByRole("list", { name: "Новые коды" })).toBeNull();
  });

  // Ловит: окно шлёт не то, что выбрано, или само придумывает коды вместо ответа службы.
  it("5 × 4 × «Только цифры» × 15 → ушло ровно это, показаны коды ответа и «Готово: 5 кодов на 15 мин»", async () => {
    const user = setup();
    await activeList();
    const box = genBox();
    const set = async (label: string, value: string) => {
      const field = within(box).getByLabelText(label);
      await user.clear(field);
      await user.type(field, value);
    };
    await set("Сколько", "5");
    await set("Длина", "4");
    await user.click(within(box).getByRole("radio", { name: "Только цифры" }));
    await set("Минуты", "15");
    await user.click(within(box).getByRole("button", { name: "Придумать" }));
    expect(await screen.findByText("Готово: 5 кодов на 15 мин")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "GeneratePromos")?.payload).toEqual({
      count: 5,
      length: 4,
      charset: "digits",
      minutes: 15,
    });
    const fresh = within(box).getByRole("list", { name: "Новые коды" });
    const codes = within(fresh).getAllByRole("listitem").map((li) => li.textContent ?? "");
    expect(codes).toHaveLength(5);
    for (const c of codes) expect(c).toMatch(/^[0-9]{4}$/);
    expect(new Set(codes).size).toBe(5);
    const list = await activeList();
    for (const c of codes) expect(within(list).getByText(c)).toBeTruthy();
    expect(tab(/^Действующие/).textContent).toBe("Действующие (7)");
    expect(within(box).getByRole("button", { name: "Скопировать все" })).toBeTruthy();
  });

  // Ловит: отказ службы по числу кодов показан общим текстом, без фокуса на поле.
  it("«Сколько» 0 → ошибка службы у «Сколько», фокус на нём, коды не созданы", async () => {
    const user = setup();
    await activeList();
    const box = genBox();
    const count = within(box).getByLabelText("Сколько");
    await user.clear(count);
    await user.keyboard("{Enter}");
    const error = await within(box).findByText("Можно от 1 до 50 кодов");
    expect(count.getAttribute("aria-invalid")).toBe("true");
    expect(count.getAttribute("aria-describedby")).toBe(error.id);
    expect(document.activeElement).toBe(count);
    expect(within(box).getByLabelText("Длина").getAttribute("aria-invalid")).toBeNull();
    expect(tab(/^Действующие/).textContent).toBe("Действующие (2)");
  });

  // Ловит: отказ без поля («Не получилось придумать столько разных кодов») молча теряется.
  it("отказ без поля - сообщение над формой", async () => {
    const user = setup();
    await activeList();
    mock.override("GeneratePromos", { ok: false, error: "Не получилось придумать столько разных кодов. Увеличьте длину" }, 1);
    await user.click(within(genBox()).getByRole("button", { name: "Придумать" }));
    expect(await within(genBox()).findByText("Не получилось придумать столько разных кодов. Увеличьте длину")).toBeTruthy();
  });
});

describe("«Скопировать все» (8.3.10 п.3)", () => {
  const original = Object.getOwnPropertyDescriptor(window.navigator, "clipboard");
  afterEach(() => {
    if (original) Object.defineProperty(window.navigator, "clipboard", original);
    else delete (window.navigator as { clipboard?: unknown }).clipboard;
  });
  const stubClipboard = (writeText: (text: string) => Promise<void>) =>
    Object.defineProperty(window.navigator, "clipboard", { value: { writeText }, configurable: true, writable: true });

  async function generateThree(user: ReturnType<typeof userEvent.setup>) {
    await activeList();
    const count = within(genBox()).getByLabelText("Сколько");
    await user.clear(count);
    await user.type(count, "3");
    await user.click(within(genBox()).getByRole("button", { name: "Придумать" }));
    await screen.findByText("Готово: 3 кода на 30 мин");
    return within(within(genBox()).getByRole("list", { name: "Новые коды" }))
      .getAllByRole("listitem")
      .map((li) => li.textContent ?? "");
  }

  // Ловит: коды через запятую или пробел, а не по одному в строке.
  it("пишет коды через перевод строки и говорит «Коды скопированы»", async () => {
    const user = setup();
    const writeText = vi.fn(() => Promise.resolve());
    stubClipboard(writeText);
    const codes = await generateThree(user);
    await user.click(within(genBox()).getByRole("button", { name: "Скопировать все" }));
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(codes.join("\n"));
    expect(await screen.findByText("Коды скопированы")).toBeTruthy();
  });

  // Ловит: отказ буфера обмена молчит или показывает технический текст.
  it("отказ буфера → «Не удалось скопировать, выделите коды вручную»", async () => {
    const user = setup();
    stubClipboard(() => Promise.reject(new Error("NotAllowedError")));
    await generateThree(user);
    await user.click(within(genBox()).getByRole("button", { name: "Скопировать все" }));
    expect(await within(genBox()).findByText("Не удалось скопировать, выделите коды вручную")).toBeTruthy();
    expect(screen.queryByText("Коды скопированы")).toBeNull();
    expect(document.body.textContent).not.toMatch(/NotAllowed/);
  });
});

describe("«Очистить историю» (8.3.10 п.4, RM5-14)", () => {
  // Ловит: удаление без подтверждения, фокус не на безопасной «Отмена», действующие тоже удалены.
  it("окно с фокусом на «Отмена»; «Отмена» - без вызова; «Удалить» - один вызов и пустая вкладка", async () => {
    const user = setup();
    await activeList();
    await user.click(tab(/^Использованные/));
    await user.click(screen.getByRole("button", { name: "Очистить историю" }));
    const dialog = screen.getByRole("dialog", { name: "Удалить все использованные коды?" });
    expect(within(dialog).getByText("Действующие останутся.")).toBeTruthy();
    expect(within(dialog).getAllByRole("button").map((b) => b.textContent)).toEqual(["Удалить", "Отмена"]);
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Отмена" }));
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("ClearPromoHistory")).toBe(0);
    expect(screen.getByRole("list", { name: "Использованные промокоды" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Очистить историю" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Удалить" }));
    expect(await screen.findByText("История промокодов очищена")).toBeTruthy();
    expect(mock.count("ClearPromoHistory")).toBe(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("Использованных кодов нет")).toBeTruthy();
    expect(tab(/^Использованные/).textContent).toBe("Использованные (0)");
    expect(tab(/^Действующие/).textContent).toBe("Действующие (2)");
    expect(document.activeElement).toBe(tab(/^Использованные/));
  });

  // Ловит: сбой очистки молчит или списки пропадают.
  it("сбой очистки - сообщение, использованные на месте", async () => {
    const user = setup();
    await activeList();
    await user.click(tab(/^Использованные/));
    mock.override("ClearPromoHistory", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Очистить историю" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Удалить" }));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(screen.getByRole("list", { name: "Использованные промокоды" })).toBeTruthy();
    expect(tab(/^Использованные/).textContent).toBe("Использованные (1)");
  });
});

describe("тексты раздела", () => {
  // Ловит: длинные тире (NFR-11) и обращение на «ты» к родителю.
  it("на «вы», без длинных тире", async () => {
    const user = setup();
    await activeList();
    await user.click(within(createBox()).getByRole("button", { name: "Создать" }));
    await user.click(within(genBox()).getByRole("button", { name: "Придумать" }));
    await screen.findByText(/^Готово: /);
    await user.click(tab(/^Использованные/));
    await user.click(screen.getByRole("button", { name: "Очистить историю" }));
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(THOU_FORM);
  });
});
