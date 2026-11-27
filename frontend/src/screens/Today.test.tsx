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

  it("имя, статус словом, остаток «1:25» и «осталось из 2:00»", async () => {
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Можно")).toBeTruthy();
    expect(within(misha).getByText("1:25")).toBeTruthy();
    expect(within(misha).getByText("осталось из 2:00")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+15" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+30" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+60" })).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "Снять блокировку" })).toBeNull();
  });

  // ТЗ 8.3.2 п.3: строка игр под остатком — потраченное из карточки службы и лимит игр.
  it("«Игры 40 из 60 мин» под остатком", async () => {
    setup({ children: [mockChild({ gamesUsedS: 2400 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const games = within(misha).getByText("Игры 40 из 60 мин");
    const used = within(misha).getByText("осталось из 2:00");
    expect(used.compareDocumentPosition(games) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // ТЗ 8.3.2 п.3, R6: строка видео после строки игр.
  it("«Видео и соцсети 10 из 20 мин» после строки игр", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({
      weekday,
      totalMin: 120,
      unlimited: false,
      gamesMin: 60,
      gamesUnlimited: false,
      videoMin: 20,
      videoUnlimited: false,
    }));
    setup({ children: [mockChild({ limits, gamesUsedS: 2400, videoUsedS: 600 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const games = within(misha).getByText("Игры 40 из 60 мин");
    const video = within(misha).getByText("Видео и соцсети 10 из 20 мин");
    expect(games.compareDocumentPosition(video) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // Ловит: у ребёнка без лимита видео карточка пишет «Видео 0 мин из 0 мин».
  it("без лимита видео строки видео нет", async () => {
    const limits = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false }));
    setup({ children: [mockChild({ limits, videoUsedS: 600 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Игры 0 из 60 мин")).toBeTruthy();
    expect(within(misha).queryByText(/^Видео /)).toBeNull();
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
    const [status, num] = within(misha).getAllByText("Без лимита");
    expect(status.className).toContain("status-ok");
    expect(num.className).toContain("remain-green");
    expect(within(misha).queryByText(/осталось из/)).toBeNull();
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
    expect(within(misha).getByText("осталось из 2:30")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "Отменить" })).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "AddExtra")?.payload).toEqual({ childId: 1, minutes: 30 });
  });

  it("«Отменить» возвращает прежний лимит через UndoExtra", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "+30" }));
    await user.click(await within(misha).findByRole("button", { name: "Отменить" }));
    expect(await within(misha).findByText("Изменение доп. времени отменено")).toBeTruthy();
    expect(within(misha).getByText("осталось из 2:00")).toBeTruthy();
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
    const ev = (atMs: number, kind: string) => ({ atMs, tzOffsetMin: -NOW.getTimezoneOffset(), childId: 1, childName: "Миша", kind, text: kind });
    const user = setup({
      events: [
        ev(t - 3600_000, "bypass"),
        ev(t - 7200_000, "bypass"),
        ev(t - 1800_000, "extra_time"),
        ev(t - 3 * 24 * 3600_000, "bypass"), // не сегодня
      ],
    });
    await screen.findByRole("article", { name: "Миша" });
    const link = await within(card("Миша")).findByRole("button", { name: "Нарушения сегодня: 2" });
    await user.click(link);
    expect(openJournal).toHaveBeenCalledWith({ violations: true, childId: 1 });
  });

  // Ловит (T9-7): попытки открыть запрещённый сайт не считаются, закрытие по лимиту видео считается.
  it("«Нарушения сегодня» считают попытки открыть запрещённый сайт", async () => {
    const t = NOW.getTime();
    const ev = (atMs: number, kind: string) => ({ atMs, tzOffsetMin: -NOW.getTimezoneOffset(), childId: 1, childName: "Миша", kind, text: kind });
    setup({ events: [ev(t - 3600_000, "site_forbidden_try"), ev(t - 1800_000, "site_forbidden_try"), ev(t - 600_000, "site_closed_video")] });
    expect(await screen.findByRole("button", { name: "Нарушения сегодня: 2" })).toBeTruthy();
  });

  it("без нарушений: «Нарушения сегодня: 0»", async () => {
    setup();
    expect(await screen.findByRole("button", { name: "Нарушения сегодня: 0" })).toBeTruthy();
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

describe("Today: значок-переключатель паузы (R11, D-96)", () => {
  const ON_HINT = "Поставить на паузу: ограничения не действуют, время считается";
  const OFF_HINT = "Закончить паузу";

  // Ловит: «Пауза» без объяснения (FB-25): у значка нет подписи для диктора и подсказки.
  it("выключен: значок не нажат, подсказка объясняет паузу", async () => {
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    const toggle = within(misha).getByRole("button", { name: ON_HINT });
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    expect(toggle.getAttribute("title")).toBe(ON_HINT);
    expect(toggle.querySelector("svg")).toBeTruthy();
    expect(within(misha).queryByText(/Пауза до/)).toBeNull();
  });

  // Ловит: срок не спрашивается или пауза ставится не тому ребёнку/не на тот срок.
  it("нажатие открывает выбор срока; «1 час» ставит паузу, значок нажат, рядом «Пауза до 18:30»", async () => {
    const user = setup({ children: [mockChild({ usedS: 7200 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const toggle = within(misha).getByRole("button", { name: ON_HINT });
    await user.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    const group = within(misha).getByRole("group", { name: "На сколько поставить паузу" });
    expect(within(group).getAllByRole("button").map((b) => b.textContent)).toEqual(["15 минут", "1 час", "2 часа", "До конца дня", "Отмена"]);
    await user.click(within(group).getByRole("button", { name: "1 час" }));
    expect(mock.calls.find((c) => c.command === "Pause")?.payload).toEqual({ childId: 1, preset: "hour1" });
    const on = await within(misha).findByRole("button", { name: OFF_HINT });
    expect(on.getAttribute("aria-pressed")).toBe("true");
    expect(on.getAttribute("title")).toBe(OFF_HINT);
    // Во время паузы видно словами, что сделает нажатие (визуальная приёмка FB-25).
    expect(on.textContent).toBe("Закончить паузу");
    // «Пауза до 18:30» — рядом со значком; ниже ответ службы «Пауза 1 ч».
    expect(within(on.parentElement!).getByText("Пауза до 18:30")).toBeTruthy();
    expect(within(misha).getByRole("status").textContent).toBe("Пауза 1 ч");
    // Видно одно место показа паузы — «Пауза до 18:30» у значка; ответ службы только для диктора (визуальная приёмка T5).
    expect(within(misha).getByText("Пауза 1 ч").closest(".visually-hidden")).toBeTruthy();
    expect(within(misha).queryByRole("group", { name: "На сколько поставить паузу" })).toBeNull();
  });

  // Ловит: выбор срока нельзя закрыть без выбора (визуальная приёмка FB-25).
  it("«Отмена» закрывает выбор срока без паузы, фокус на значке", async () => {
    const user = setup({ children: [mockChild({ usedS: 7200 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const toggle = within(misha).getByRole("button", { name: ON_HINT });
    await user.click(toggle);
    const group = within(misha).getByRole("group", { name: "На сколько поставить паузу" });
    await user.click(within(group).getByRole("button", { name: "Отмена" }));
    expect(within(misha).queryByRole("group", { name: "На сколько поставить паузу" })).toBeNull();
    expect(mock.calls.some((c) => c.command === "Pause")).toBe(false);
    expect(document.activeElement).toBe(toggle);
  });

  // Ловит: паузу нельзя закончить (FB-25) или нужно подтверждение; значок держится локальным флагом.
  it("включён: нажатие сразу заканчивает паузу, карточка из ответа службы", async () => {
    const user = setup({ children: [mockChild({ usedS: 7200, pauseUntil: "18:30" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const on = within(misha).getByRole("button", { name: OFF_HINT });
    expect(within(on.parentElement!).getByText("Пауза до 18:30")).toBeTruthy();
    await user.click(on);
    expect(mock.calls.find((c) => c.command === "EndPause")?.payload).toEqual({ childId: 1 });
    expect(await within(misha).findByText("Пауза закончилась")).toBeTruthy();
    expect(within(misha).getByText("Время вышло")).toBeTruthy();
    const off = within(misha).getByRole("button", { name: ON_HINT });
    expect(off.getAttribute("aria-pressed")).toBe("false");
    expect(within(misha).queryByText("Пауза до 18:30")).toBeNull();
    expect(within(misha).queryByRole("group", { name: "На сколько поставить паузу" })).toBeNull();
  });

  // Ловит: после ответа фокус теряется на отключённом значке (PL-68).
  it("после ответа фокус на значке", async () => {
    const user = setup({ children: [mockChild({ pauseUntil: "18:30" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: OFF_HINT }));
    const off = await within(misha).findByRole("button", { name: ON_HINT });
    await vi.waitFor(() => expect(document.activeElement).toBe(off));
    await user.click(off);
    await user.click(within(misha).getByRole("button", { name: "15 минут" }));
    const on = await within(misha).findByRole("button", { name: OFF_HINT });
    await vi.waitFor(() => expect(document.activeElement).toBe(on));
  });

  // Ловит: во время запроса значок можно нажать второй раз.
  it("во время запроса значок недоступен", async () => {
    const user = setup({ children: [mockChild({ pauseUntil: "18:30" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    let release: () => void = () => undefined;
    mock.override("EndPause", (req) => new Promise((resolve) => {
      release = () => resolve(mock.handler("EndPause")(req));
    }), 1);
    await user.click(within(misha).getByRole("button", { name: OFF_HINT }));
    expect((within(misha).getByRole("button", { name: OFF_HINT }) as HTMLButtonElement).disabled).toBe(true);
    await act(async () => release());
    expect(await within(misha).findByRole("button", { name: ON_HINT })).toBeTruthy();
  });

  // Ловит: ошибка конца паузы теряется или попадает внутрь области «статус» (PL-75).
  it("паузы уже нет: «Паузы сейчас нет» предупреждением рядом со статусом", async () => {
    const user = setup({ children: [mockChild({ pauseUntil: "18:30" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    mock.override("EndPause", { ok: false, error: "Паузы сейчас нет" }, 1);
    await user.click(within(misha).getByRole("button", { name: OFF_HINT }));
    const alert = await within(misha).findByRole("alert");
    expect(alert.textContent).toBe("Паузы сейчас нет");
    expect(alert.closest('[role="status"]')).toBeNull();
    expect(within(misha).getByRole("status")).toBeTruthy();
  });

  // Ловит: Escape не закрывает выбор срока или фокус не возвращается на значок.
  it("Escape закрывает выбор срока, фокус на значке", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    const toggle = within(misha).getByRole("button", { name: ON_HINT });
    await user.click(toggle);
    expect(within(misha).getByRole("group", { name: "На сколько поставить паузу" })).toBeTruthy();
    await user.keyboard("{Escape}");
    expect(within(misha).queryByRole("group", { name: "На сколько поставить паузу" })).toBeNull();
    expect(document.activeElement).toBe(toggle);
    expect(mock.count("Pause")).toBe(0);
  });
});

describe("Today: взрослый вид карточки (8.3.2 п. 3, D-109)", () => {
  // Лимит 2 ч каждый день: остаток задаётся потраченным временем.
  const twoHours = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));
  const withLeft = (min: number, over: Parameters<typeof mockChild>[0] = {}) =>
    mockChild({ limits: twoHours, usedS: (120 - min) * 60, ...over });

  // Ловит: цвет числа остатка не по порогам 15/16 и 60/61 мин (ТЗ 8.0 п. 3, Review Focus п. 6).
  it.each([
    [61, "1:01", "remain-green"],
    [60, "1:00", "remain-blue"],
    [16, "0:16", "remain-blue"],
    [15, "0:15", "remain-red"],
  ])("остаток %i мин: число %s с классом %s", async (min, text, cls) => {
    setup({ children: [withLeft(min)] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const num = within(misha).getByText(text);
    expect(num.className).toContain(cls);
    expect(within(misha).getByText("осталось из 2:00")).toBeTruthy();
  });

  it("без лимита: «Без лимита» зелёным", async () => {
    vi.setSystemTime(new Date(2026, 9, 25, 12, 0, 0)); // воскресенье без лимита
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    const num = within(misha).getAllByText("Без лимита").find((el) => el.className.includes("remain-num"));
    expect(num?.className).toContain("remain-green");
  });

  // Ловит: строки игр и видео не посчитаны из потраченного и лимита (8.3.2 п. 3).
  it("«Игры 40 из 60 мин» и «Видео и соцсети 10 из 30 мин» из статуса", async () => {
    const limits = twoHours.map((l) => ({ ...l, gamesMin: 60, gamesUnlimited: false, videoMin: 30, videoUnlimited: false }));
    setup({ children: [mockChild({ limits, usedS: 2100, gamesUsedS: 2400, videoUsedS: 600 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Игры 40 из 60 мин")).toBeTruthy();
    expect(within(misha).getByText("Видео и соцсети 10 из 30 мин")).toBeTruthy();
  });

  // Ловит (ревью 0.8.4 M2, класс A): лимит игр уменьшили ниже сыгранного — окно писало «Игры 60 из
  // 60 мин» (лимит − остаток), телефон из того же отчёта — «Игры 70 из 60 мин». Число одно.
  it("сыграно больше лимита: «Игры 70 из 60 мин», как на телефоне", async () => {
    const limits = twoHours.map((l) => ({ ...l, gamesMin: 60, gamesUnlimited: false, videoMin: 30, videoUnlimited: false }));
    setup({ children: [mockChild({ limits, usedS: 4200, gamesUsedS: 4200, videoUsedS: 2700 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Игры 70 из 60 мин")).toBeTruthy();
    expect(within(misha).getByText("Видео и соцсети 45 из 30 мин")).toBeTruthy();
  });

  it("присутствие: зелёная точка и «За компьютером с 18:20»", async () => {
    setup({ children: [mockChild({ presenceNow: true, presenceText: "За компьютером с 18:20" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const line = within(misha).getByText("За компьютером с 18:20");
    expect(line.closest(".cc-presence")?.querySelector(".cc-dot-on")).toBeTruthy();
  });

  it("не за компьютером: серая точка", async () => {
    setup({ children: [mockChild({ presenceNow: false, presenceText: "Не за компьютером с 17:05" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const line = within(misha).getByText("Не за компьютером с 17:05");
    expect(line.closest(".cc-presence")?.querySelector(".cc-dot-on")).toBeNull();
  });

  // Ловит (визуальная приёмка T5): серая точка присутствия рядом с цветной точкой статуса — две точки подряд;
  // «Осталось 10 / мин» переносилось между числом и единицей.
  it("присутствие неизвестно: на его месте статус словами, одна точка — цвета статуса", async () => {
    setup({ children: [withLeft(10)] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    const status = within(misha).getByText("Осталось 10 мин");
    expect(status.textContent).toBe("Осталось 10\u00a0мин");
    const line = status.closest(".cc-presence")!;
    expect(line).toBeTruthy();
    expect(line.querySelector(".cc-dot")).toBeNull();
    expect(status.className).toContain("status-warn");
  });

  it("копилка и запрет на сегодня", async () => {
    setup({
      children: [mockChild({ bonusBalance: 45, schedule: [{ weekday: 1, startMin: 21 * 60, endMin: 7 * 60 }] })],
    });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("В копилке 45 мин")).toBeTruthy();
    expect(within(misha).getByText("Запрет с 21:00 до 07:00")).toBeTruthy();
  });

  it("запрет снят на сегодня: «Запрет на сегодня снят»; запретных часов нет: строки нет", async () => {
    setup({
      children: [
        mockChild({ noForbiddenToday: true, schedule: [{ weekday: 1, startMin: 21 * 60, endMin: 7 * 60 }] }),
        mockChild({ childId: 2, sid: "S-2", name: "Даша", schedule: [] }),
      ],
    });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Запрет на сегодня снят")).toBeTruthy();
    expect(within(card("Даша")).queryByText(/^Запрет/)).toBeNull();
  });

  it("кнопки справа: «+15» … «Своё», «−15» … пауза, «Начислить в копилку»", async () => {
    setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    for (const name of ["+15", "+30", "+60", "Своё время…", "−15", "−30", "−60", "Начислить в копилку", "Статистика"]) {
      expect(within(misha).getByRole("button", { name })).toBeTruthy();
    }
  });

  // FR-22: начисление с карточки — минуты и причина, итог с «Отменить».
  it("«Начислить в копилку»: минуты и причина, «Начислено 30 минут» и «Отменить»", async () => {
    const user = setup();
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "Начислить в копилку" }));
    await user.type(within(misha).getByLabelText("За что"), "за уроки");
    await user.click(within(misha).getByRole("button", { name: "Начислить" }));
    expect(await within(misha).findByText("Начислено 30 минут. Баланс 30 мин")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "GrantBonus")?.payload).toEqual({ childId: 1, minutes: 30, reason: "за уроки" });
    expect(within(misha).getByText("В копилке 30 мин")).toBeTruthy();
    await user.click(within(misha).getByRole("button", { name: "Отменить" }));
    expect(mock.calls.some((c) => c.command === "UndoBonus")).toBe(true);
  });

  it("«Нарушения сегодня: N» на карточке ребёнка ведёт в журнал этого ребёнка", async () => {
    const now = NOW.getTime();
    const user = setup({
      children: [mockChild(), mockChild({ childId: 2, sid: "S-2", name: "Даша" })],
      events: [
        { atMs: now - 60_000, tzOffsetMin: -NOW.getTimezoneOffset(), childId: 2, childName: "Даша", kind: "bypass", text: "Даша: попытка обхода" },
        { atMs: now - 120_000, tzOffsetMin: -NOW.getTimezoneOffset(), childId: 2, childName: "Даша", kind: "bypass", text: "Даша: попытка обхода" },
      ],
    });
    await screen.findByRole("article", { name: "Миша" });
    expect(await within(card("Миша")).findByRole("button", { name: "Нарушения сегодня: 0" })).toBeTruthy();
    await user.click(within(card("Даша")).getByRole("button", { name: "Нарушения сегодня: 2" }));
    expect(openJournal).toHaveBeenCalledWith({ violations: true, childId: 2 });
  });
});

describe("Today: ждут решения и последние события (8.3.2 п. 3)", () => {
  it("блок «1 программа ждёт решения» с названием и «Разобрать» ведёт в «Программы»", async () => {
    mock = installBackend({ sessionActive: true });
    const openPrograms = vi.fn<() => void>();
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Today onOpenPrograms={openPrograms} onOpenJournal={vi.fn()} />);
    const block = (await screen.findByRole("heading", { name: "1 программа ждёт решения" })).closest("section")!;
    expect(within(block).getByText("SuperGame. Пока вы не решите, ребёнок не сможет её запустить.")).toBeTruthy();
    await user.click(within(block).getByRole("button", { name: "Разобрать" }));
    expect(openPrograms).toHaveBeenCalledTimes(1);
  });

  it("никто не ждёт решения: блока нет", async () => {
    mock = installBackend({ sessionActive: true, pending: [] });
    render(<Today />);
    await screen.findByRole("article", { name: "Миша" });
    await screen.findByRole("heading", { name: "Последние события" });
    expect(screen.queryByRole("heading", { name: /ждут? решения/ })).toBeNull();
  });

  it("«Последние события»: 5 новых строк с именем и ссылка «Весь журнал»", async () => {
    const now = NOW.getTime();
    const events = [1, 2, 3, 4, 5, 6].map((i) => ({
      atMs: now - i * 60_000,
      tzOffsetMin: -NOW.getTimezoneOffset(),
      childId: 1,
      childName: "Миша",
      kind: "extra_add",
      text: `Миша: +${i} мин`,
    }));
    mock = installBackend({ sessionActive: true, events });
    const onJournal = vi.fn<(o: { violations: boolean; childId?: number }) => void>();
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Today onOpenJournal={onJournal} />);
    const block = (await screen.findByRole("heading", { name: "Последние события" })).closest("section")!;
    const rows = await within(block).findAllByRole("listitem");
    expect(rows).toHaveLength(5);
    expect(rows[0].textContent).toBe("11:59Миша +1 мин");
    expect(within(rows[0]).getByText("Миша").tagName).toBe("B");
    expect(mock.calls.some((c) => c.command === "ListEvents" && (c.payload as { limit: number }).limit === 5)).toBe(true);
    await user.click(within(block).getByRole("button", { name: "Весь журнал" }));
    expect(onJournal).toHaveBeenCalledWith({ violations: false });
  });
});
