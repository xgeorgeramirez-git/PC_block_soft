import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Today from "./Today";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import { RULES_NOT_READ_TEXT } from "../lib/notices";

// FR-15, сценарий 8.3.2: карточки детей, статус словом и цветом, доп. время (+/−/своё), статистика.

let mock: MockBackend;
let openJournal: Mock<(opts: { violations: boolean }) => void>;
let openSettings: Mock<() => void>;
let openChildren: Mock<() => void>;
let openStats: Mock<(childId: number) => void>;

const NOW = new Date(2026, 9, 19, 12, 0, 0); // пн, 19 октября 2026

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ sessionActive: true, ...opts });
  openJournal = vi.fn<(opts: { violations: boolean }) => void>();
  openSettings = vi.fn<() => void>();
  openChildren = vi.fn<() => void>();
  openStats = vi.fn<(childId: number) => void>();
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(
    <Today onOpenJournal={openJournal} onOpenSettings={openSettings} onOpenChildren={openChildren} onOpenStats={openStats} />,
  );
  return user;
}

const card = (name: string) => screen.getByRole("article", { name });

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

describe("Today: карточка ребёнка", () => {
  it("заголовок с датой словами", async () => {
    setup();
    expect(await screen.findByRole("heading", { name: "Сегодня" })).toBeTruthy();
    expect(screen.getByText("пн, 19 октября")).toBeTruthy();
  });

  it("имя, статус словом, «Использовано … из …»", async () => {
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Можно")).toBeTruthy();
    expect(within(misha).getByText("Использовано 35 мин из 2 ч")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+15" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+30" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+60" })).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "Снять блокировку" })).toBeNull();
  });

  // ТЗ 8.3.2 п.3: строка игр от службы (card.gamesText) под «Использовано».
  it("«Игры 40 мин из 1 ч» под строкой общего времени", async () => {
    setup({ children: [mockChild({ gamesUsedS: 2400 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const games = within(misha).getByText("Игры 40 мин из 1 ч");
    const used = within(misha).getByText("Использовано 35 мин из 2 ч");
    expect(used.compareDocumentPosition(games) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // Ловит: у ребёнка без лимита игр карточка пишет «Игры 0 мин из 0 мин».
  it("без лимита игр строки игр нет", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));
    setup({ children: [mockChild({ limits })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).queryByText(/^Игры /)).toBeNull();
  });

  // Ловит: цвет статуса не по ТЗ 8.0 п.3 (зелёный - можно, янтарный - мало, красный - вышло/обход)
  // или статус передан одним цветом без слова.
  it.each([
    [{ usedS: 2100 }, "Можно", "ok"],
    [{ usedS: 7200 - 600 }, "Осталось 10 мин", "warn"],
    [{ usedS: 7200 }, "Время вышло", "danger"],
  ])("статус %j: слово «%s», цвет %s", async (over, word, tone) => {
    setup({ children: [mockChild(over)] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const status = within(misha).getByText(word);
    expect(status.className).toContain(`status-${tone}`);
  });

  it("«без лимита» - зелёный, но со словами", async () => {
    vi.setSystemTime(new Date(2026, 9, 25, 12, 0, 0)); // воскресенье без лимита
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Без лимита").className).toContain("status-ok");
    expect(within(misha).getByText("Использовано 35 мин, без лимита")).toBeTruthy();
  });

  it("обычное «время вышло» оставляет «+время»", async () => {
    setup({ children: [mockChild({ usedS: 7200 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Время вышло")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+15" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+60" })).toBeTruthy();
  });

  it("администратор: ограничения не действуют, кнопок времени нет", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetToday", {
      ok: true,
      payload: {
        children: [
          {
            childId: 5,
            sid: "S-5",
            name: "Анна",
            status: {
              name: "Анна",
              status: "admin",
              statusText: "Администратор: ограничения не действуют",
              label: "",
              tooltip: "",
              summary: "",
              day: "2026-10-19",
              remainingS: 0,
              limitS: 0,
              usedS: 0,
              unlimited: true,
              lock: "none",
              reason: "",
              detail: "",
              until: "",
              pauseUntil: "",
              bonusBalance: 0,
              forbiddenNote: "",
              used: "",
            },
            extraMin: 0,
            bonusBalance: 0,
            canClearBlock: false,
            noForbiddenToday: false,
          },
        ],
        notices: [],
      },
    });
    render(<Today />);
    const anna = await screen.findByRole("article", { name: "Анна" });
    expect(within(anna).getByText("Администратор: ограничения не действуют")).toBeTruthy();
    expect(within(anna).queryByRole("button", { name: "+30" })).toBeNull();
    expect(within(anna).queryByText(/Использовано/)).toBeNull();
  });

  // Ловит: предупреждение «правила не прочитаны» и карточка ребёнка показывают
  // противоречивые сигналы (B-4). В безопасном режиме служба отдаёт ребёнка со статусом
  // «safe» без блокировки: карточка не считает время и не предлагает «+время», как и говорит
  // предупреждение. Один источник — служба; экран их не рассогласовывает.
  it("правила не прочитаны: предупреждение и карточка согласованы (B-4)", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetToday", {
      ok: true,
      payload: {
        children: [
          {
            childId: 1,
            sid: "S-1",
            name: "Миша",
            status: {
              name: "Миша",
              status: "safe",
              statusText: "Правила не прочитаны: ограничения не действуют",
              label: "",
              tooltip: "",
              summary: "",
              day: "2026-10-19",
              remainingS: 0,
              limitS: 0,
              usedS: 0,
              unlimited: true,
              lock: "none",
              reason: "",
              detail: "",
              until: "",
              pauseUntil: "",
              bonusBalance: 0,
              forbiddenNote: "",
              used: "",
            },
            extraMin: 0,
            bonusBalance: 0,
            canClearBlock: false,
            noForbiddenToday: false,
          },
        ],
        notices: ["Служба не смогла прочитать правила"],
      },
    });
    render(<Today />);
    expect(await screen.findByText(RULES_NOT_READ_TEXT)).toBeTruthy();
    const misha = screen.getByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Правила не прочитаны: ограничения не действуют")).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "+15" })).toBeNull();
    expect(within(misha).queryByText(/Использовано/)).toBeNull();
  });
});

describe("Today: добавить время", () => {
  // Ловит: «+30» не меняет лимит на карточке сразу или молчит о результате (L-74, FR-15).
  it("«+30»: карточка сразу с новым лимитом, «Добавлено 30 минут» и «Отменить»", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "+30" }));
    expect(await within(misha).findByText("Добавлено 30 минут")).toBeTruthy();
    expect(within(misha).getByText("Использовано 35 мин из 2 ч 30 мин")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "Отменить" })).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AddExtra")?.payload).toEqual({ childId: 1, minutes: 30 });
  });

  it("«Отменить» возвращает прежний лимит через UndoExtra", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "+30" }));
    await user.click(await within(misha).findByRole("button", { name: "Отменить" }));
    expect(await within(misha).findByText("Изменение доп. времени отменено")).toBeTruthy();
    expect(within(misha).getByText("Использовано 35 мин из 2 ч")).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(mock.calls.find((c) => c.command === "UndoExtra")?.payload).toEqual({ childId: 1, extraId: "e-17" });
  });

  // Ловит: «Отменить» висит дольше 10 секунд (служба уже не примет отмену).
  it("«Отменить» пропадает через 10 секунд, сообщение остаётся", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "+15" }));
    await within(misha).findByText("Добавлено 15 минут");
    await act(async () => {
      vi.advanceTimersByTime(9_000);
    });
    expect(within(misha).getByRole("button", { name: "Отменить" })).toBeTruthy();
    await act(async () => {
      vi.advanceTimersByTime(1_500);
    });
    expect(within(misha).queryByRole("button", { name: "Отменить" })).toBeNull();
    expect(within(misha).getByText("Добавлено 15 минут")).toBeTruthy();
  });

  it("время добавляется только тому ребёнку, чья кнопка нажата", async () => {
    const user = setup({ children: [mockChild(), mockChild({ childId: 2, sid: "S-2", name: "Даша" })] });
    await screen.findByRole("article", { name: "Миша" });
    await user.click(within(card("Даша")).getByRole("button", { name: "+60" }));
    await within(card("Даша")).findByText("Добавлено 60 минут");
    expect(mock.calls.find((c) => c.command === "AddExtra")?.payload).toEqual({ childId: 2, minutes: 60 });
    expect(within(card("Миша")).queryByText(/Добавлено/)).toBeNull();
  });

  it("возвращает из «Время вышло»: статус обновляется по ответу службы", async () => {
    const user = setup({ children: [mockChild({ usedS: 7200 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Время вышло")).toBeTruthy();
    await user.click(within(misha).getByRole("button", { name: "+30" }));
    expect(await within(misha).findByText("Можно")).toBeTruthy();
  });

  // Ловит: сбой при добавлении молча теряется или показывает технический текст.
  it("сбой: простыми словами у карточки, кнопки снова доступны", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    mock.override("AddExtra", "reject", 1);
    await user.click(within(misha).getByRole("button", { name: "+30" }));
    const alert = await within(misha).findByRole("alert");
    expect(alert.textContent).toBe("Программа не отвечает. Попробуйте ещё раз через минуту");
    expect((within(misha).getByRole("button", { name: "+30" }) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("Today: убавить время и своё число (FB-3, D-43)", () => {
  // Ловит: «−30» не убавляет доп. время или молчит (R10, D-43).
  it("«−30»: убавляет доп. время, «Убрано 30 минут»", async () => {
    const user = setup({ children: [mockChild({ usedS: 2100, extraMin: 60 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "−30" }));
    expect(await within(misha).findByText("Убрано 30 минут")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "ReduceExtra")?.payload).toEqual({ childId: 1, minutes: 30 });
  });

  // Ловит: поле «своё число минут» добавляет/убавляет не введённое значение или «Убрать» не обнуляет.
  it("«Своё время…»: поле на ±минут и «Убрать всё»", async () => {
    const user = setup({ children: [mockChild({ usedS: 2100, extraMin: 45 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "Своё время…" }));
    const input = within(misha).getByLabelText("Минут:");
    await user.clear(input);
    await user.type(input, "25");
    await user.click(within(misha).getByRole("button", { name: "Добавить" }));
    expect(mock.calls.find((c) => c.command === "AddExtra")?.payload).toEqual({ childId: 1, minutes: 25 });
    await user.click(within(misha).getByRole("button", { name: "Убрать всё" }));
    expect(mock.calls.find((c) => c.command === "ClearExtra")?.payload).toEqual({ childId: 1 });
  });

  // Ловит: кнопка «Статистика» не ведёт к статистике этого ребёнка (FR-23, D-44).
  it("«Статистика» открывает статистику этого ребёнка", async () => {
    const user = setup({ children: [mockChild(), mockChild({ childId: 2, sid: "S-2", name: "Даша" })] });
    await screen.findByRole("article", { name: "Миша" });
    await user.click(within(card("Даша")).getByRole("button", { name: "Статистика" }));
    expect(openStats).toHaveBeenCalledWith(2);
  });
});

describe("Today: общее", () => {
  // Ловит: родитель читает «база данных» и «служба» (NFR-11, L-112) и не знает, куда идти дальше.
  it("испорченная база: человеческие слова по смыслу R18 и кнопка «Открыть «Дети»»", async () => {
    const user = setup({
      notices: ["База данных была повреждена и восстановлена. Ограничения не действовали, пока служба не смогла её прочитать"],
    });
    const notice = await screen.findByRole("alert");
    expect(notice.textContent).toContain(RULES_NOT_READ_TEXT);
    expect(notice.textContent).not.toMatch(/баз[аыу] данных|служб/i);
    await user.click(within(notice).getByRole("button", { name: "Открыть «Дети»" }));
    expect(openChildren).toHaveBeenCalledTimes(1);
  });

  it("предупреждение стоит над карточками детей", async () => {
    setup({ notices: ["База данных была повреждена и восстановлена"] });
    const notice = await screen.findByRole("alert");
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(notice.compareDocumentPosition(misha) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("обычное сообщение службы - как есть и без кнопки", async () => {
    setup({ notices: ["Установлено обновление"] });
    const notice = await screen.findByRole("alert");
    expect(notice.textContent).toBe("Установлено обновление");
    expect(within(notice).queryByRole("button")).toBeNull();
  });

  // Ловит: пустой список детей показан белым экраном (L-22) или без следующего шага (L-119).
  it("детей нет: объясняет и ведёт в настройки", async () => {
    const user = setup({ children: [] });
    expect(await screen.findByText("Пока нет ни одного ребёнка")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Открыть настройки" }));
    expect(openSettings).toHaveBeenCalledTimes(1);
  });

  it("null вместо списка детей не роняет экран", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetToday", { ok: true, payload: { children: null, notices: null } });
    render(<Today />);
    expect(await screen.findByText("Пока нет ни одного ребёнка")).toBeTruthy();
  });

  it("ссылка «Нарушения сегодня: N» считает только сегодняшние попытки обхода", async () => {
    const t = NOW.getTime();
    const ev = (atMs: number, kind: string) => ({ atMs, tzOffsetMin: 180, childId: 1, childName: "Миша", kind, text: kind });
    const user = setup({
      events: [
        ev(t - 3600_000, "bypass"),
        ev(t - 7200_000, "bypass"),
        ev(t - 1800_000, "extra_time"),
        ev(t - 3 * 24 * 3600_000, "bypass"), // не сегодня
      ],
    });
    const link = await screen.findByRole("button", { name: "Нарушения сегодня: 2" });
    await user.click(link);
    expect(openJournal).toHaveBeenCalledWith({ violations: true });
  });

  it("без нарушений: «Нарушения сегодня: 0»", async () => {
    setup();
    expect(await screen.findByRole("button", { name: "Нарушения сегодня: 0" })).toBeTruthy();
  });

  // 8.3.2 п.5: ссылка «Ждут решения: N» ведёт на «Программы»; число то же, что у службы.
  it("ссылка «Ждут решения: N» стоит первой и открывает «Программы»", async () => {
    mock = installBackend({ sessionActive: true });
    const openPrograms = vi.fn<() => void>();
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Today onOpenPrograms={openPrograms} onOpenJournal={vi.fn()} />);
    const link = await screen.findByRole("button", { name: "Ждут решения: 1" });
    await screen.findByRole("button", { name: "Нарушения сегодня: 0" });
    expect(link.parentElement?.firstElementChild).toBe(link);
    await user.click(link);
    expect(openPrograms).toHaveBeenCalledTimes(1);
  });

  it("никто не ждёт решения: ссылки «Ждут решения» нет", async () => {
    mock = installBackend({ sessionActive: true, pending: [] });
    render(<Today />);
    await screen.findByRole("button", { name: "Нарушения сегодня: 0" });
    expect(screen.queryByRole("button", { name: /Ждут решения/ })).toBeNull();
  });

  it("если журнал не прочитался, ссылки нет, а карточки на месте", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("ListEvents", "reject");
    render(<Today />);
    await screen.findByRole("article", { name: "Миша" });
    expect(screen.queryByRole("button", { name: /Нарушения сегодня/ })).toBeNull();
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true });
    mock.override("GetToday", "reject", 1);
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Today />);
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByRole("article", { name: "Миша" })).toBeTruthy();
  });

  it("при возврате в окно данные перечитываются", async () => {
    setup();
    await screen.findByRole("article", { name: "Миша" });
    const before = mock.count("GetToday");
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });
    expect(mock.count("GetToday")).toBe(before + 1);
  });
});

describe("Today: снять запрет на сегодня (R24, D-51)", () => {
  // Ловит: кнопка не шлёт команду или карточка не показывает, что запрет снят (класс A).
  it("«Снять запрет на сегодня» → видно на карточке, есть «Вернуть запретные часы»", async () => {
    const user = setup({ children: [mockChild({ schedule: [{ weekday: 1, startMin: 21 * 60, endMin: 7 * 60 }] })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "Снять запрет на сегодня" }));
    expect(mock.calls.find((c) => c.command === "SetNoForbidden")?.payload).toEqual({ childId: 1, on: true });
    expect(await within(misha).findByRole("button", { name: "Вернуть запретные часы" })).toBeTruthy();
    expect(within(misha).getAllByText("Сегодня запретные часы не действуют").length).toBeGreaterThan(0);
    await user.click(within(misha).getByRole("button", { name: "Вернуть запретные часы" }));
    expect(await within(misha).findByRole("button", { name: "Снять запрет на сегодня" })).toBeTruthy();
    expect(mock.state.children[0].noForbiddenToday).toBe(false);
  });

  // Ловит: кнопка висит у ребёнка без запретных часов сегодня — снимать нечего.
  it("нет запретных часов сегодня — кнопки нет", async () => {
    setup({ children: [mockChild({ schedule: [] })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).queryByRole("button", { name: "Снять запрет на сегодня" })).toBeNull();
  });
});
