import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import MyStats from "./MyStats";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";

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

  // Ловит: запретные часы в M1 (их ещё нет) обещаны ребёнку.
  it("запретных часов в M1 нет и они не упоминаются", async () => {
    setup();
    await screen.findByRole("region", { name: "Мои правила" });
    expect(document.body.textContent).not.toMatch(/запретн|с \d\d:\d\d до \d\d:\d\d/i);
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
