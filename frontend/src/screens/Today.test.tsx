import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Today from "./Today";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../__mocks__/backend";
import { RULES_NOT_READ_TEXT } from "../lib/notices";

// FR-15, сценарий 8.3.2: карточки детей, статус словом и цветом, +время с «Отменить», снятие блокировки.

let mock: MockBackend;
let openJournal: Mock<(opts: { violations: boolean }) => void>;
let openSettings: Mock<() => void>;
let openChildren: Mock<() => void>;

const NOW = new Date(2026, 9, 19, 12, 0, 0); // пн, 19 октября 2026

function setup(opts: Parameters<typeof installBackend>[0] = {}) {
  mock = installBackend({ sessionActive: true, ...opts });
  openJournal = vi.fn<(opts: { violations: boolean }) => void>();
  openSettings = vi.fn<() => void>();
  openChildren = vi.fn<() => void>();
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<Today onOpenJournal={openJournal} onOpenSettings={openSettings} onOpenChildren={openChildren} />);
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

  // Ловит: цвет статуса не по ТЗ 8.0 п.3 (зелёный - можно, янтарный - мало, красный - вышло/обход)
  // или статус передан одним цветом без слова.
  it.each([
    [{ usedS: 2100 }, "Можно", "ok"],
    [{ usedS: 7200 - 600 }, "Осталось 10 мин", "warn"],
    [{ usedS: 7200 }, "Время вышло", "danger"],
    [{ blocked: true }, "Заблокирован за обход защиты", "danger"],
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

  it("при блокировке за обход видно, что случилось, и кнопка «Снять блокировку»", async () => {
    setup({ children: [mockChild({ blocked: true }), mockChild({ childId: 2, sid: "S-2", name: "Даша" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Изменено системное время")).toBeTruthy();
    expect(within(misha).getByText("Можно снова: когда родитель снимет блокировку")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "Снять блокировку" })).toBeTruthy();
    expect(within(card("Даша")).queryByRole("button", { name: "Снять блокировку" })).toBeNull();
  });

  // Ловит: у ребёнка, заблокированного за обход, лежат «+15/+30/+60» рядом со «Снять блокировку»:
  // родитель добавляет время, а блокировка остаётся.
  it("при блокировке за обход «+время» спрятано, остаётся только «Снять блокировку»", async () => {
    setup({ children: [mockChild({ blocked: true }), mockChild({ childId: 2, sid: "S-2", name: "Даша" })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    for (const m of ["+15", "+30", "+60"]) {
      expect(within(misha).queryByRole("button", { name: m })).toBeNull();
    }
    expect(within(misha).getAllByRole("button").map((b) => b.textContent)).toEqual(["Снять блокировку"]);
    // У соседа без блокировки кнопки на месте.
    expect(within(card("Даша")).getByRole("button", { name: "+15" })).toBeTruthy();
  });

  it("обычное «время вышло» оставляет «+время»", async () => {
    setup({ children: [mockChild({ usedS: 7200 })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    expect(within(misha).getByText("Время вышло")).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+15" })).toBeTruthy();
    expect(within(misha).getByRole("button", { name: "+60" })).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "Снять блокировку" })).toBeNull();
  });

  it("«Снять блокировку» снимает и говорит об этом", async () => {
    const user = setup({ children: [mockChild({ blocked: true })] });
    const misha = await screen.findByRole("article", { name: "Миша" });
    await user.click(within(misha).getByRole("button", { name: "Снять блокировку" }));
    expect(await within(misha).findByText("Блокировка снята")).toBeTruthy();
    expect(within(misha).queryByRole("button", { name: "Снять блокировку" })).toBeNull();
    expect(within(misha).getByText("Можно")).toBeTruthy();
    expect(mock.calls.find((c) => c.command === "ClearBlock")?.payload).toEqual({ childId: 1 });
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
              used: "",
            },
            extraMin: 0,
            canClearBlock: false,
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
    expect(await within(misha).findByText("Добавленное время отменено")).toBeTruthy();
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
