import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import MyStats from "./MyStats";
import { DEFAULT_PASSWORD, installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

// FR-12, R21, сценарий 8.2.6: окно «Моя статистика» только для чтения, в M1 - «Сегодня» и «Мои правила».

let mock: MockBackend;

// Ребёнку на «ты» (A4). \b в JS не видит кириллицу, поэтому границы слов через \p{L}.
const YOU_FORM = /(?<!\p{L})(вы|вам|вас|ваш\p{L}*|попробуйте)(?!\p{L})/iu;

const NOW = new Date(2026, 9, 19, 12, 0, 0); // пн, 19 октября 2026

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ mode: "mystats", ...opts });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<MyStats />);
  return user;
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("MyStats: окно только для чтения (R21)", () => {
  it("в заголовке пометка «Только просмотр»", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Моя статистика" })).toBeTruthy();
    expect(screen.getByText("Только просмотр")).toBeTruthy();
  });

  // Ловит: в окне ребёнка появилось поле или список для ввода: он ничего не меняет (R21).
  // Единственное поле — «Ввести промокод» в разделе «Бонусы» (R26, D-50).
  it("нет полей ввода, кроме промокода в «Бонусах»", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    const bonuses = await screen.findByRole("region", { name: "Бонусы" });
    const fields = Array.from(document.querySelectorAll("input, textarea, select, [contenteditable]"));
    expect(fields).toHaveLength(1);
    expect(bonuses.contains(fields[0])).toBe(true);
    expect(screen.getByRole("textbox", { name: "Промокод" })).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  // Ловит: ребёнок видит статистику другого ребёнка или чужое имя (R21).
  it("только свои данные: чужого ребёнка нет нигде", async () => {
    setup({
      myChildId: 2,
      children: [mockChild(), mockChild({ childId: 2, sid: "S-1-5-21-111-222-333-1002", name: "Даша", usedS: 600 })],
    });
    await screen.findByRole("region", { name: "Мои правила" });
    expect(document.body.textContent).toContain("Даша");
    expect(document.body.textContent).not.toContain("Миша");
    // Окно спросило только свои данные и ничего из команд родителя: копилку — номером 0 («моя»).
    expect(new Set(mock.calls.map((c) => c.command))).toEqual(new Set(["MyToday", "GetBonus"]));
    expect(mock.calls.filter((c) => c.command === "GetBonus").every((c) => !c.payload.childId)).toBe(true);
  });

  it("не просит у программы ничего, кроме MyToday и своей копилки, и ничего не отправляет", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    await screen.findByRole("region", { name: "Бонусы" });
    expect(new Set(mock.calls.map((c) => c.command))).toEqual(new Set(["MyToday", "GetBonus"]));
  });
});

describe("MyStats: периоды", () => {
  it("пять периодов, выбран «Сегодня»", async () => {
    setup();
    const tabs = await screen.findAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Сегодня", "Неделя", "Месяц", "Год", "Всё время"]);
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
  });

  // Ловит: недоступный период без объяснения (L-17) или с обещанием не к месту.
  it("остальные периоды недоступны и объясняют почему", async () => {
    const user = setup();
    for (const name of ["Неделя", "Месяц", "Год", "Всё время"]) {
      const tab = await screen.findByRole("tab", { name });
      expect(tab.getAttribute("aria-disabled")).toBe("true");
      expect(tab.getAttribute("aria-selected")).toBe("false");
      await user.click(tab);
      expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    }
    expect(screen.getByText("Неделя, месяц, год и всё время появятся в полной статистике")).toBeTruthy();
    expect(mock.count("MyToday")).toBe(1);
  });
});

describe("MyStats: Итоги", () => {
  it("строки «Сегодня» и «Всего» из ответа программы", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("Сегодня осталось 1 ч 25 мин из 2 ч")).toBeTruthy();
    const total = within(box).getByText("Всего").closest("div");
    expect(total?.textContent).toContain("35 мин");
    // «Использовано 35 мин» один раз, в «Всего».
    expect(within(box).queryByText("Использовано 35 мин")).toBeNull();
  });

  // Ловит: категории показаны нулями, как будто они считаются; в M1 учёт только общий.
  it("игры, видео и соцсети, учёба: честно «Пока не считается»", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Итоги" });
    for (const label of ["Игры", "Видео и соцсети", "Учёба"]) {
      const row = within(box).getByText(label).closest("div");
      expect(row?.textContent).toContain("Пока не считается");
    }
  });

  it("время вышло: видно, когда можно снова", async () => {
    setup({ children: [mockChild({ usedS: 7200 })] });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("Можно снова: завтра")).toBeTruthy();
  });

  // Ловит: статус на экране ребёнка расходится с остатком в значке и у родителя (Review Focus 5).
  it("остаток тот же, что у программы для значка и родителя", async () => {
    setup({ children: [mockChild({ usedS: 3600 })] });
    expect(await screen.findByText("Сегодня осталось 1 ч 00 мин из 2 ч")).toBeTruthy();
  });

  // Ловит: пустой период показывает пустоту или ноль вместо объяснения (8.2.6 п.9).
  it("нет данных: «За этот период пока нет данных»", async () => {
    setup({ myChildId: null });
    const box = await screen.findByRole("region", { name: "Итоги" });
    expect(within(box).getByText("За этот период пока нет данных")).toBeTruthy();
    expect(within(box).queryByText("Всего")).toBeNull();
  });
});

describe("MyStats: Мои правила", () => {
  it("лимиты по дням недели текстом, сегодняшний день отмечен", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    const rows = within(box).getAllByRole("row").slice(1);
    expect(rows.map((r) => r.textContent)).toEqual([
      "Понедельник2 ч (сегодня)",
      "Вторник2 ч",
      "Среда1 ч 30 мин",
      "Четверг2 ч",
      "Пятница2 ч",
      "Суббота4 ч",
      "ВоскресеньеБез лимита",
    ]);
    expect(rows[0].getAttribute("aria-current")).toBe("date");
    expect(rows.filter((r) => r.getAttribute("aria-current") === "date")).toHaveLength(1);
  });

  it("режим окончания текстом", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(within(box).getByText("Когда время выйдет, экран закроется блокировкой. Открытые окна останутся под ней.")).toBeTruthy();
  });

  // Ловит: «Мои правила» обещают запретные часы, которых у ребёнка нет (они живут в «Сегодня», D-53).
  it("в таблице правил нет запретных часов", async () => {
    setup();
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(box.textContent).not.toMatch(/запретн|с \d\d:\d\d до \d\d:\d\d/i);
  });

  it("ноль минут - «Нельзя»", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: false }));
    setup({ children: [mockChild({ limits })] });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(within(box).getAllByText("Нельзя")).toHaveLength(7);
  });

  // Ловит: правила не прочитаны (R18), а окно показывает пустую таблицу без слов.
  it("правил нет: говорит об этом и куда идти", async () => {
    setup();
    const base = (await window.go!.ui!.App!.Call("MyToday", "{}")) as string;
    const reply = JSON.parse(base);
    reply.payload.rules = [];
    mock.override("MyToday", reply);
    await userEvent.setup().click(await screen.findByRole("button", { name: "Обновить" }));
    const box = await screen.findByRole("region", { name: "Мои правила" });
    await within(box).findByText("Правила сейчас недоступны. Спроси у родителя.");
    expect(within(box).queryByRole("table")).toBeNull();
  });

  it("сеанс не ребёнка: правил нет, окно не падает", async () => {
    setup({ myChildId: null });
    const box = await screen.findByRole("region", { name: "Мои правила" });
    expect(within(box).getByText("Эта учётная запись не детская: здесь нет правил.")).toBeTruthy();
  });
});

describe("MyStats: обновление и сбои", () => {
  // Ловит: сбой связи с программой показывает технический текст или молчит (L-112, L-114).
  it("нет связи: простыми словами и «Обновить», кнопка повторяет запрос", async () => {
    const user = setup();
    await screen.findByRole("region", { name: "Итоги" });
    mock.override("MyToday", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    expect(await screen.findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/ipc|connection|pipe|undefined/i);
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    expect(await screen.findByRole("region", { name: "Итоги" })).toBeTruthy();
    expect(screen.queryByText("Не удалось получить данные. Попробуй через минуту")).toBeNull();
  });

  it("первая загрузка без связи: сообщение и «Обновить»", async () => {
    mock = installBackend({ mode: "mystats", linkDown: true });
    render(<MyStats />);
    expect(await screen.findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Обновить" })).toBeTruthy();
  });

  // Ловит: окно лежит открытым и показывает остаток часовой давности.
  it("вернулись в окно: цифры перечитываются", async () => {
    setup();
    await screen.findByRole("region", { name: "Итоги" });
    const before = mock.count("MyToday");
    window.dispatchEvent(new Event("focus"));
    await vi.waitFor(() => expect(mock.count("MyToday")).toBe(before + 1));
  });

  it("пока данные грузятся, говорит об этом", () => {
    setup();
    expect(screen.getByText("Загружаю данные")).toBeTruthy();
  });
});

describe("MyStats: тексты для ребёнка", () => {
  // Ловит: длинные тире (NFR-11) и обращение на «вы» в окне ребёнка (A4).
  it("без длинных тире и без «вы»", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(YOU_FORM);
  });
});

describe("MyStats: «Бонусы» (R25, R26, D-49, D-50)", () => {
  const AUTO = "Бонус за свободное время: 15 мин";
  const withBonus = () =>
    setup({
      children: [
        mockChild({
          bonusBalance: 45,
          bonusLedger: [
            { atMs: NOW.getTime() - 12 * 3600_000, tzOffsetMin: 180, deltaMin: 15, reason: AUTO, balanceAfter: 45 },
            { atMs: NOW.getTime() - 36 * 3600_000, tzOffsetMin: 180, deltaMin: 30, reason: "за уборку", balanceAfter: 30 },
          ],
        }),
      ],
      promos: [{ codeDisplay: "A1234", minutes: 30, createdAtMs: NOW.getTime() - 3600_000 }],
    });

  // Ловит: ребёнок не видит, за что начислен автобонус (D-49).
  it("баланс копилки и автобонус простыми словами", async () => {
    withBonus();
    const box = await screen.findByRole("region", { name: "Бонусы" });
    expect(await within(box).findByText("Копилка: 45 мин")).toBeTruthy();
    expect(within(box).getByText("+15 мин за неизрасходованное время прошлого дня")).toBeTruthy();
    expect(within(box).getByText("+30 мин: за уборку")).toBeTruthy();
    expect(box.textContent ?? "").not.toMatch(YOU_FORM);
  });

  // Ловит: верный код не добавляет время или сообщение не по ТЗ (FR-35: «Добавлено N минут»).
  it("верный промокод: «Добавлено 30 минут»", async () => {
    const user = withBonus();
    const field = await screen.findByRole("textbox", { name: "Промокод" });
    await user.type(field, " a1234 ");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(await screen.findByText("Добавлено 30 минут")).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(mock.calls.find((c) => c.command === "RedeemPromo")?.payload).toEqual({ childId: 0, code: " a1234 " });
    expect((field as HTMLInputElement).value).toBe("");
  });

  // Ловит: повторный ввод того же кода снова добавляет время (одноразовость, R26).
  it("неверный или использованный код: «Код не подходит», время не меняется", async () => {
    const user = withBonus();
    const field = await screen.findByRole("textbox", { name: "Промокод" });
    await user.type(field, "a1234");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    await screen.findByText("Добавлено 30 минут");
    await user.type(field, "a1234");
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(await screen.findByText("Код не подходит")).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(30);
  });

  it("пустой код: подсказка у поля, в программу не уходит", async () => {
    const user = withBonus();
    await screen.findByRole("textbox", { name: "Промокод" });
    await user.click(screen.getByRole("button", { name: "Ввести промокод" }));
    expect(screen.getByText("Введи код")).toBeTruthy();
    expect(mock.count("RedeemPromo")).toBe(0);
  });
});

// D-53, ТЗ 8.2.6 п.1: «Сегодня» с остатками, лимитами, запретными часами, копилкой, кнопками и ссылкой внизу.
describe("MyStats: «Сегодня» (D-53)", () => {
  const MONDAY_NIGHT = [{ weekday: 1, startMin: 21 * 60, endMin: 7 * 60 }];

  // Ловит: на «Сегодня» нет остатка общего времени, игр, «Видео и соцсетей», лимита, запретных часов или копилки.
  it("остатки, лимит дня, запретные часы и копилка", async () => {
    setup({ children: [mockChild({ bonusBalance: 45, schedule: MONDAY_NIGHT })] });
    const box = await screen.findByRole("region", { name: "Сколько осталось" });
    const rows = Object.fromEntries(
      Array.from(box.querySelectorAll("dt")).map((dt) => [dt.textContent, dt.nextElementSibling?.textContent]),
    );
    expect(rows).toEqual({
      "Осталось всего": "1 ч 25 мин",
      "Игры": "Пока не считается",
      "Видео и соцсети": "Пока не считается",
      "Лимит дня": "2 ч",
      "Запретные часы": "Сегодня нельзя с 21:00 до 07:00",
      "Копилка": "45 мин",
    });
  });

  // Ловит: раздел остатка идёт после итогов и правил, и ребёнок ищет главное внизу.
  it("раздел остатка стоит первым, перед итогами", async () => {
    setup();
    const left = await screen.findByRole("region", { name: "Сколько осталось" });
    const totals = screen.getByRole("region", { name: "Итоги" });
    expect(left.compareDocumentPosition(totals) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("кнопки «Попросить ещё» и «Взять из копилки», а «Настройки для родителя» внизу мелко", async () => {
    setup({ children: [mockChild({ bonusBalance: 45 })] });
    const ask = await screen.findByRole("button", { name: "Попросить ещё" });
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    const parent = screen.getByRole("button", { name: "Настройки для родителя" });
    const rules = screen.getByRole("region", { name: "Мои правила" });
    expect(ask.compareDocumentPosition(spend) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(rules.compareDocumentPosition(parent) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(parent.className).toContain("link");
  });

  it("сеанс не ребёнка: кнопок времени нет", async () => {
    setup({ myChildId: null });
    await screen.findByRole("region", { name: "Мои правила" });
    expect(screen.queryByRole("button", { name: "Попросить ещё" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Взять из копилки" })).toBeNull();
  });

  // Ловит: ссылка внизу ничего не открывает или открывает не окно родителя.
  it("«Настройки для родителя» просит у службы окно родителя", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Настройки для родителя" }));
    expect(mock.calls.find((c) => c.command === "OpenWindow")?.payload).toEqual({ kind: "parent" });
    expect(mock.state.openedWindows).toEqual(["parent"]);
  });

  // Ловит: окно не открылось, а ребёнок не получил ни слова (L-114).
  it("не открылось: простыми словами", async () => {
    const user = setup();
    const link = await screen.findByRole("button", { name: "Настройки для родителя" });
    mock.override("OpenWindow", "reject", 1);
    await user.click(link);
    expect(await screen.findByText("Не получилось открыть окно. Попробуй ещё раз")).toBeTruthy();
  });
});

describe("MyStats: «Попросить ещё» (FR-10)", () => {
  async function openAsk() {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Попросить ещё" }));
    const dialog = await screen.findByRole("dialog", { name: "Позови родителя" });
    return { user, dialog };
  }

  // Ловит: панель без пароля родителя (ребёнок сам себе добавил бы время) или без быстрых кнопок.
  it("панель «Позови родителя»: пароль, +15 / +30 / +60 и своё число", async () => {
    const { dialog } = await openAsk();
    expect(within(dialog).getByLabelText("Пароль родителя")).toBeTruthy();
    expect((within(dialog).getByLabelText("Пароль родителя") as HTMLInputElement).type).toBe("password");
    for (const name of ["+15", "+30", "+60"]) expect(within(dialog).getByRole("button", { name })).toBeTruthy();
    expect(within(dialog).getByLabelText("Своё число минут")).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Отмена" })).toBeTruthy();
  });

  // Ловит: служба не получает номер ребёнка (без него AskMore отвечает «Ребёнок не найден»), пароль или минуты.
  it("верный пароль и +30: время добавлено, «Добавлено 30 минут»", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.click(within(dialog).getByRole("button", { name: "+30" }));
    expect(await screen.findByText("Добавлено 30 минут")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AskMore")?.payload).toEqual({
      childId: 1,
      minutes: 30,
      password: DEFAULT_PASSWORD,
    });
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(screen.queryByRole("dialog")).toBeNull();
    // Цифры окна перечитаны: лимит дня теперь 2 ч 30 мин.
    expect(await screen.findByText("2 ч 30 мин")).toBeTruthy();
  });

  // Ловит: неверный пароль закрывает панель или показывает ошибку не у поля (L-121).
  it("неверный пароль: ошибка у поля, панель открыта, время не добавлено", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), "не тот");
    await user.click(within(dialog).getByRole("button", { name: "+15" }));
    expect(await within(dialog).findByText("Неверный пароль. Осталось попыток: 4")).toBeTruthy();
    expect(screen.getByRole("dialog", { name: "Позови родителя" })).toBeTruthy();
    expect(mock.state.children[0].extraMin).toBe(0);
  });

  it("пустой пароль: подсказка у поля, в программу не уходит", async () => {
    const { user, dialog } = await openAsk();
    await user.click(within(dialog).getByRole("button", { name: "+60" }));
    expect(within(dialog).getByText("Введи пароль родителя")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
  });

  // Ловит: своё число принимает 0 или больше 240 (граница службы).
  it("своё число: 45 минут проходит, 300 отклонено до отправки", async () => {
    const { user, dialog } = await openAsk();
    await user.type(within(dialog).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.type(within(dialog).getByLabelText("Своё число минут"), "300");
    await user.click(within(dialog).getByRole("button", { name: "Добавить" }));
    expect(within(dialog).getByText("Можно от 1 до 240 минут")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
    await user.clear(within(dialog).getByLabelText("Своё число минут"));
    await user.type(within(dialog).getByLabelText("Своё число минут"), "45");
    await user.click(within(dialog).getByRole("button", { name: "Добавить" }));
    expect(await screen.findByText("Добавлено 45 минут")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AskMore")?.payload).toMatchObject({ minutes: 45 });
  });

  it("«Отмена» и Esc закрывают панель, ничего не отправляя", async () => {
    const { user, dialog } = await openAsk();
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Попросить ещё" }));
    await screen.findByRole("dialog", { name: "Позови родителя" });
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("AskMore")).toBe(0);
  });

  // Ловит: не узнали номер ребёнка (копилка не прочиталась) и отправили пустой запрос.
  it("не знаем, чей сеанс: говорит об этом, а не шлёт пароль впустую", async () => {
    const { user, dialog } = await openAsk();
    mock.override("GetBonus", "reject");
    await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
    await user.click(screen.getByRole("button", { name: "Обновить" }));
    await user.click(await screen.findByRole("button", { name: "Попросить ещё" }));
    const again = await screen.findByRole("dialog", { name: "Позови родителя" });
    await user.type(within(again).getByLabelText("Пароль родителя"), DEFAULT_PASSWORD);
    await user.click(within(again).getByRole("button", { name: "+30" }));
    expect(await within(again).findByText("Не удалось получить данные. Попробуй через минуту")).toBeTruthy();
    expect(mock.count("AskMore")).toBe(0);
  });

  it("тексты панели ребёнку на «ты», без длинных тире", async () => {
    const { dialog } = await openAsk();
    expect(dialog.textContent).not.toMatch(/[—–]/);
    expect(dialog.textContent).not.toMatch(YOU_FORM);
  });
});

describe("MyStats: «Взять из копилки» (FR-11)", () => {
  async function openSpend(over: Parameters<typeof mockChild>[0] = {}) {
    const user = setup({ children: [mockChild({ bonusBalance: 45, ...over })] });
    await user.click(await screen.findByRole("button", { name: "Взять из копилки" }));
    const dialog = await screen.findByRole("dialog", { name: "Взять из копилки" });
    return { user, dialog };
  }

  it("варианты 15, 30 и весь баланс", async () => {
    const { dialog } = await openSpend();
    expect(within(dialog).getAllByRole("button").map((b) => b.textContent)).toEqual(["15 мин", "30 мин", "Всё: 45 мин", "Отмена"]);
  });

  // Ловит: списание без подтверждения или с неверным остатком копилки в тексте.
  it("30 минут: подтверждение с остатком, «Взять» списывает без пароля", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "30 мин" }));
    expect(await screen.findByText("Взять из копилки 30 мин? Останется 15 мин")).toBeTruthy();
    expect(mock.count("SpendBonus")).toBe(0);
    await user.click(screen.getByRole("button", { name: "Взять" }));
    expect(await screen.findByText("Взято из копилки 30 мин")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "SpendBonus")?.payload).toEqual({ childId: 0, minutes: 30 });
    expect(mock.state.children[0].bonusBalance).toBe(15);
    expect(mock.state.children[0].extraMin).toBe(30);
    expect(screen.queryByRole("dialog")).toBeNull();
    // Копилка в окне перечитана.
    const left = await screen.findByRole("region", { name: "Сколько осталось" });
    expect(within(left).getByText("Копилка").nextElementSibling?.textContent).toBe("15 мин");
  });

  it("«Отмена» на подтверждении: ничего не списано", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "Всё: 45 мин" }));
    await screen.findByText("Взять из копилки 45 мин? Останется 0 мин");
    await user.click(screen.getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mock.count("SpendBonus")).toBe(0);
    expect(mock.state.children[0].bonusBalance).toBe(45);
  });

  // Ловит: отказ службы теряется или показан техническим текстом.
  it("отказ службы показан в панели", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "15 мин" }));
    mock.state.children[0].bonusBalance = 5; // копилку уже потратили в другом месте
    await user.click(await screen.findByRole("button", { name: "Взять" }));
    expect(await screen.findByText("В копилке 5 мин, столько взять нельзя")).toBeTruthy();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  // Ловит: в запретный интервал кнопка рабочая или недоступна молча (урок L-17).
  it("запретные часы: кнопка недоступна, причина и час названы", async () => {
    setup({ children: [mockChild({ bonusBalance: 45, forbiddenUntil: "Можно снова: сегодня в 07:00" })] });
    const user = userEvent.setup();
    const btn = await screen.findByRole("button", { name: "Взять из копилки" });
    expect(btn.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("Из копилки можно взять с 07:00")).toBeTruthy();
    expect(btn.getAttribute("aria-describedby")).toBeTruthy();
    await user.click(btn);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("копилка пуста: кнопка недоступна, причина названа", async () => {
    setup();
    const btn = await screen.findByRole("button", { name: "Взять из копилки" });
    expect(btn.getAttribute("aria-disabled")).toBe("true");
    expect(screen.getByText("В копилке пока пусто")).toBeTruthy();
  });

  it("тексты панели ребёнку на «ты», без длинных тире", async () => {
    const { user, dialog } = await openSpend();
    await user.click(within(dialog).getByRole("button", { name: "30 мин" }));
    await screen.findByText("Взять из копилки 30 мин? Останется 15 мин");
    const text = screen.getByRole("dialog").textContent ?? "";
    expect(text).not.toMatch(/[—–]/);
    expect(text).not.toMatch(YOU_FORM);
  });
});
