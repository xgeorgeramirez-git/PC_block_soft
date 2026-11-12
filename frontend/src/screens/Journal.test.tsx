import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Journal from "./Journal";
import { installBackend, mockChild, uninstallBackend, type MockBackend, type MockEventInput, type MockOptions } from "../__mocks__/backend";

// FR-24, 8.3.7 п.2: события простыми словами, вкладки периода (D-47), фильтры «Ребёнок» и «Что» в
// запросе службы, «Показать ещё» по курсору; FR-38, 8.3.7 п.3: «Очистить журнал» с подтверждением.

let mock: MockBackend;
const NOW = new Date(2026, 10, 12, 15, 0, 0);
const MIDNIGHT = new Date(2026, 10, 12, 0, 0, 0, 0).getTime();
const MONTH_START = new Date(2026, 10, 1, 0, 0, 0, 0).getTime();

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
});

afterEach(() => {
  vi.useRealTimers();
  uninstallBackend();
});

const event = (over: Partial<MockEventInput>): MockEventInput => ({
  atMs: Date.UTC(2026, 9, 19, 18, 14),
  tzOffsetMin: 180,
  childId: 1,
  childName: "Миша",
  kind: "bypass",
  text: "Миша: попытка обойти защиту. Экран заблокирован",
  ...over,
});

const kolya = () => mockChild({ childId: 2, sid: "S-2", name: "Коля" });

function setup(opts: MockOptions, violationsOnly = false) {
  mock = installBackend({ sessionActive: true, ...opts });
  const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
  render(<Journal violationsOnly={violationsOnly} />);
  return user;
}

const listCalls = () => mock.calls.filter((c) => c.command === "ListEvents").map((c) => c.payload);
const lastList = () => listCalls()[listCalls().length - 1];
const texts = () => screen.queryAllByRole("listitem").map((li) => li.querySelector(".journal-text")?.textContent);
const clearStatus = () => document.querySelector(".stat-clear-status") as HTMLElement;

describe("Journal", () => {
  it("события простыми словами: время и что произошло, новые сверху", async () => {
    setup({
      events: [event({ atMs: Date.UTC(2026, 9, 19, 15, 0), kind: "extra_time", text: "Миша: доп. время +30 мин" }), event({})],
    });
    const items = await screen.findAllByRole("listitem");
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText("19 окт, 21:14")).toBeTruthy();
    expect(within(items[0]).getByText("Миша: попытка обойти защиту. Экран заблокирован")).toBeTruthy();
    expect(within(items[1]).getByText("19 окт, 18:00")).toBeTruthy();
    expect(within(items[1]).getByText("Миша: доп. время +30 мин")).toBeTruthy();
  });

  // Ловит (8.3.7 п.2, PL-49): пустой результат - пустой экран, падение или прежние тексты
  // «Пока событий нет» / «Нарушений пока нет» вместо одного текста ТЗ.
  it("ничего не нашлось: «За этот период таких событий нет»", async () => {
    setup({});
    expect(await screen.findByText("За этот период таких событий нет")).toBeTruthy();
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.queryByText("Пока событий нет")).toBeNull();
  });

  it("null вместо списка событий не роняет экран", async () => {
    setup({});
    mock.override("ListEvents", { ok: true, payload: { events: null } });
    await screen.findByText("За этот период таких событий нет");
    await userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) }).click(screen.getByRole("tab", { name: "За месяц" }));
    expect(await screen.findByText("За этот период таких событий нет")).toBeTruthy();
  });

  // Ловит (RM5-5): фильтр снова делается на экране после чтения 1000 строк, старые события теряются.
  it("по умолчанию: «Всё», все дети, все события - страница 200 записей от службы", async () => {
    setup({});
    await screen.findByText("За этот период таких событий нет");
    expect(listCalls()).toEqual([{ childId: 0, fromMs: 0, toMs: 0, group: "", beforeAtMs: 0, beforeId: 0, limit: 200 }]);
    expect(screen.getByRole("tab", { name: "Всё" }).getAttribute("aria-selected")).toBe("true");
    expect((screen.getByLabelText("Ребёнок") as HTMLSelectElement).value).toBe("0");
    expect((screen.getByLabelText("Что") as HTMLSelectElement).value).toBe("");
    expect(screen.getByRole("heading", { name: "Журнал" })).toBeTruthy();
  });

  it("в «Что» шесть пунктов ТЗ, в «Ребёнок» - «Все дети» и дети", async () => {
    setup({ children: [mockChild(), kolya()] });
    await screen.findByRole("option", { name: "Коля" });
    const opts = (label: string) => within(screen.getByLabelText(label)).getAllByRole("option").map((o) => o.textContent);
    expect(opts("Что")).toEqual(["Все события", "Нарушения", "Время и копилка", "Программы и сайты", "Пароль и настройки", "Работа программы"]);
    expect(opts("Ребёнок")).toEqual(["Все дети", "Миша", "Коля"]);
  });

  // Вкладки периода (FR-24, D-47): начало периода уходит в запрос, служба отбирает сама.
  it("вкладки периода: «Сегодня» - с полуночи, «За месяц» - с 1-го числа", async () => {
    const user = setup({
      events: [
        event({ atMs: NOW.getTime() - 60_000, kind: "extra_time", text: "Миша: доп. время +30 мин (сегодня)" }),
        event({ atMs: Date.UTC(2026, 0, 2, 10, 0), kind: "extra_time", text: "Миша: давнее событие" }),
      ],
    });
    expect(await screen.findByText(/давнее событие/)).toBeTruthy();
    await user.click(screen.getByRole("tab", { name: "Сегодня" }));
    await vi.waitFor(() => expect(screen.queryByText(/давнее событие/)).toBeNull());
    expect(screen.getByText(/\(сегодня\)/)).toBeTruthy();
    expect(lastList()).toMatchObject({ fromMs: MIDNIGHT, childId: 0, group: "" });
    await user.click(screen.getByRole("tab", { name: "За месяц" }));
    await vi.waitFor(() => expect(lastList()).toMatchObject({ fromMs: MONTH_START }));
  });

  it("вкладки периода переключаются стрелками, выбранная - одна в порядке Tab", async () => {
    const user = setup({});
    await screen.findByText("За этот период таких событий нет");
    const all = screen.getByRole("tab", { name: "Всё" });
    expect(all.tabIndex).toBe(0);
    expect(screen.getByRole("tab", { name: "Сегодня" }).tabIndex).toBe(-1);
    all.focus();
    await user.keyboard("{ArrowRight}");
    const today = screen.getByRole("tab", { name: "Сегодня" });
    expect(today.getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(today);
    await vi.waitFor(() => expect(lastList()).toMatchObject({ fromMs: MIDNIGHT }));
    expect(screen.getByRole("tabpanel").getAttribute("aria-labelledby")).toBe(today.id);
  });

  // Ловит (8.3.7 п.2): выбор ребёнка или группы не доходит до службы.
  it("фильтры «Ребёнок» и «Что» уходят в запрос", async () => {
    const user = setup({
      children: [mockChild(), kolya()],
      events: [
        event({ atMs: Date.UTC(2026, 9, 19, 18, 14), text: "Миша: попытка обойти защиту" }),
        event({ atMs: Date.UTC(2026, 9, 19, 18, 10), childId: 2, childName: "Коля", text: "Коля: попытка обойти защиту" }),
        event({ atMs: Date.UTC(2026, 9, 19, 18, 5), kind: "extra_time", text: "Миша: доп. время +30 мин" }),
      ],
    });
    await screen.findByRole("option", { name: "Коля" });
    await screen.findByText("Коля: попытка обойти защиту");
    await user.selectOptions(screen.getByLabelText("Ребёнок"), "Миша");
    await vi.waitFor(() => expect(texts()).toEqual(["Миша: попытка обойти защиту", "Миша: доп. время +30 мин"]));
    expect(lastList()).toMatchObject({ childId: 1, group: "", fromMs: 0 });
    await user.selectOptions(screen.getByLabelText("Что"), "Время и копилка");
    await vi.waitFor(() => expect(texts()).toEqual(["Миша: доп. время +30 мин"]));
    expect(lastList()).toMatchObject({ childId: 1, group: "time" });
    await user.selectOptions(screen.getByLabelText("Ребёнок"), "Все дети");
    await user.selectOptions(screen.getByLabelText("Что"), "Нарушения");
    await vi.waitFor(() => expect(texts()).toEqual(["Миша: попытка обойти защиту", "Коля: попытка обойти защиту"]));
    expect(lastList()).toMatchObject({ childId: 0, group: "violations" });
  });

  // Ловит (RM5-5): «Показать ещё» без курсора (повтор первой страницы), список заменяется, а не
  // дописывается, или кнопка остаётся, когда старых записей нет.
  it("«Показать ещё» передаёт курсор последней строки и дописывает старые записи", async () => {
    const base = Date.UTC(2026, 9, 1, 12, 0);
    const events = Array.from({ length: 205 }, (_, i) =>
      event({ id: i + 1, atMs: base + i * 60_000, kind: "extra_time", text: `Миша: доп. время №${i + 1}` }),
    );
    const user = setup({ events });
    await screen.findByText("Миша: доп. время №205");
    expect(screen.getAllByRole("listitem")).toHaveLength(200);
    expect(screen.queryByText("Миша: доп. время №5")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Показать ещё" }));
    await screen.findByText("Миша: доп. время №1");
    expect(lastList()).toEqual({ childId: 0, fromMs: 0, toMs: 0, group: "", beforeAtMs: base + 5 * 60_000, beforeId: 6, limit: 200 });
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(205);
    expect(within(items[0]).getByText("Миша: доп. время №205")).toBeTruthy();
    expect(within(items[204]).getByText("Миша: доп. время №1")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Показать ещё" })).toBeNull();
  });

  it("сбой «Показать ещё»: уведомление и «Повторить», загруженное остаётся", async () => {
    const base = Date.UTC(2026, 9, 1, 12, 0);
    const user = setup({ events: Array.from({ length: 201 }, (_, i) => event({ id: i + 1, atMs: base + i * 60_000, text: `Запись ${i + 1}` })) });
    await screen.findByText("Запись 201");
    mock.override("ListEvents", "reject", 1);
    await user.click(screen.getByRole("button", { name: "Показать ещё" }));
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    expect(screen.getAllByRole("listitem")).toHaveLength(200);
    await user.click(screen.getByRole("button", { name: "Повторить" }));
    await screen.findByText("Запись 1");
    expect(screen.getAllByRole("listitem")).toHaveLength(201);
    expect(screen.queryByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeNull();
  });

  // Ловит: поздний ответ прежнего фильтра перекрывает выбранный.
  it("поздний ответ прежнего периода не перекрывает выбранный", async () => {
    let release: (v: unknown) => void = () => undefined;
    mock = installBackend({
      sessionActive: true,
      events: [event({ atMs: NOW.getTime() - 60_000, kind: "extra_time", text: "Сегодняшнее" })],
    });
    mock.override(
      "ListEvents",
      () => new Promise((r) => (release = r)).then(() => ({ ok: true, payload: { events: [{ ...event({}), id: 99, text: "Ответ прежнего периода" }], more: false } })),
      1,
    );
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<Journal />);
    await user.click(screen.getByRole("tab", { name: "Сегодня" }));
    await screen.findByText("Сегодняшнее");
    release(undefined);
    await new Promise((r) => setTimeout(r, 0));
    await vi.waitFor(() => expect(texts()).toEqual(["Сегодняшнее"]));
  });

  // Ловит (ревью окна M5): после смены вкладки или фильтра до ответа прежний список выглядит как новый.
  it("смена фильтра до ответа: «Загружаю…», список занят и приглушён", async () => {
    const user = setup({ events: [event({})] });
    const list = await screen.findByRole("list", { name: "События" });
    const loading = () => document.querySelector(".journal-loading") as HTMLElement;
    expect(loading().getAttribute("role")).toBe("status");
    expect(loading().textContent).toBe("");
    expect(list.getAttribute("aria-busy")).toBe("false");
    let release: () => void = () => undefined;
    const gate = new Promise<void>((r) => (release = r));
    const real = mock.handler("ListEvents");
    mock.override("ListEvents", (p: unknown) => gate.then(() => real(p)), 1);
    await user.click(screen.getByRole("tab", { name: "Сегодня" }));
    expect(loading().textContent).toBe("Загружаю…");
    expect(screen.getByRole("list", { name: "События" }).getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("list", { name: "События" }).className).toContain("journal-stale");
    release();
    await vi.waitFor(() => expect(loading().textContent).toBe(""));
    expect(screen.queryByRole("list", { name: "События" })?.getAttribute("aria-busy") ?? "false").toBe("false");
  });

  it("нет связи: простыми словами и «Повторить»", async () => {
    mock = installBackend({ sessionActive: true, events: [event({})] });
    mock.override("ListEvents", "reject", 1);
    render(<Journal />);
    expect(await screen.findByText("Программа не отвечает. Попробуйте ещё раз через минуту")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(await screen.findByText(/попытка обойти защиту/)).toBeTruthy();
  });

  // Ловит (8.3.7 п.2): ссылка «Нарушения сегодня» ведёт на журнал, где нарушения утонули среди
  // прочего или видны вчерашние.
  it("из ссылки о нарушениях: «Что: Нарушения» и вкладка «Сегодня»", async () => {
    const user = setup(
      {
        events: [
          event({ atMs: NOW.getTime() - 60_000 }),
          event({ atMs: NOW.getTime() - 120_000, kind: "extra_time", text: "Миша: доп. время +30 мин" }),
          event({ atMs: MIDNIGHT - 60_000, text: "Миша: вчерашняя попытка обойти защиту" }),
        ],
      },
      true,
    );
    expect(await screen.findByText("Миша: попытка обойти защиту. Экран заблокирован")).toBeTruthy();
    expect(texts()).toEqual(["Миша: попытка обойти защиту. Экран заблокирован"]);
    expect(listCalls()).toEqual([{ childId: 0, fromMs: MIDNIGHT, toMs: 0, group: "violations", beforeAtMs: 0, beforeId: 0, limit: 200 }]);
    expect(screen.getByRole("tab", { name: "Сегодня" }).getAttribute("aria-selected")).toBe("true");
    expect((screen.getByLabelText("Что") as HTMLSelectElement).value).toBe("violations");
    // Прежней кнопки-переключателя нет: всё показывает «Что: Все события».
    expect(screen.queryByRole("button", { name: "Показать все события" })).toBeNull();
    await user.selectOptions(screen.getByLabelText("Что"), "Все события");
    await screen.findByText("Миша: доп. время +30 мин");
  });

  // Ловит (R13, D-65, FB-18; R14, T9-7): тексты службы показаны как есть во всех событиях.
  it("сайты и автозакрытие: во всех событиях - всё, тексты службы как есть", async () => {
    const list = [
      ["site_forbidden_try", "Миша: попытка открыть запрещённое: сайт example.com"],
      ["site_closed_video", "Миша: закрыт сайт youtube.com: время на видео и соцсети закончилось"],
      ["program_auto_closed", "Миша: программа «Revo Uninstaller» запускалась сама и закрыта"],
      ["brave_restored", "Правила Brave восстановлены"],
    ];
    setup({
      events: list.map(([kind, text], i) => event({ atMs: Date.UTC(2026, 9, 19, 18, 14 - i), kind, text, ...(kind === "brave_restored" ? { childId: 0, childName: "" } : {}) })),
    });
    await screen.findByText(list[0][1]);
    expect(texts()).toEqual(list.map(([, t]) => t));
  });

  it("нарушений сегодня нет: «За этот период таких событий нет»", async () => {
    setup({ events: [event({ atMs: NOW.getTime() - 60_000, kind: "extra_time", text: "Миша: доп. время +30 мин" })] }, true);
    expect(await screen.findByText("За этот период таких событий нет")).toBeTruthy();
  });

  describe("«Очистить журнал» (FR-38, 8.3.7 п.3)", () => {
    const twoChildren = (): MockOptions => ({
      children: [mockChild(), kolya()],
      events: [
        event({ atMs: Date.UTC(2026, 9, 19, 18, 14), text: "Миша: попытка обойти защиту" }),
        event({ atMs: Date.UTC(2026, 9, 19, 18, 10), childId: 2, childName: "Коля", text: "Коля: попытка обойти защиту" }),
        event({ atMs: Date.UTC(2026, 9, 19, 18, 0), childId: 0, childName: "", kind: "password_failed", text: "Неверный пароль родителя" }),
      ],
    });

    async function pickMisha(user: ReturnType<typeof setup>) {
      await screen.findByRole("option", { name: "Миша" });
      await user.selectOptions(screen.getByLabelText("Ребёнок"), "Миша");
      return screen.findByRole("button", { name: "Очистить журнал" });
    }

    // Ловит (8.3.7 п.3): кнопка при «Все дети» - непонятно, чей журнал удалится.
    it("при «Все дети» кнопки нет, при выбранном ребёнке - есть", async () => {
      const user = setup(twoChildren());
      await screen.findByText("Коля: попытка обойти защиту");
      expect(screen.queryByRole("button", { name: "Очистить журнал" })).toBeNull();
      await pickMisha(user);
      await user.selectOptions(screen.getByLabelText("Ребёнок"), "Все дети");
      expect(screen.queryByRole("button", { name: "Очистить журнал" })).toBeNull();
    });

    // Ловит: фокус на «Удалить» - Enter сразу удаляет весь журнал.
    it("окно: текст 8.3.7 п.3, фокус на «Отмена»; «Отмена» и Esc ничего не удаляют", async () => {
      const user = setup(twoChildren());
      await user.click(await pickMisha(user));
      const dialog = screen.getByRole("dialog", { name: "Удалить весь журнал Миши?" });
      expect(within(dialog).getByText("Это нельзя отменить.")).toBeTruthy();
      expect(within(dialog).getAllByRole("button").map((b) => b.textContent)).toEqual(["Удалить", "Отмена"]);
      expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Отмена" }));
      await user.click(within(dialog).getByRole("button", { name: "Отмена" }));
      expect(screen.queryByRole("dialog")).toBeNull();
      await user.click(screen.getByRole("button", { name: "Очистить журнал" }));
      await user.keyboard("{Escape}");
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(mock.count("ClearJournal")).toBe(0);
      expect(clearStatus().textContent).toBe("");
    });

    // Ловит (FR-38, S48): очистка уходит без номера ребёнка или дважды, список не обновился, у
    // другого ребёнка и у событий без ребёнка что-то пропало.
    it("«Удалить»: один вызов, «Журнал Миши очищен», в журнале Миши одна запись", async () => {
      const user = setup(twoChildren());
      await user.click(await pickMisha(user));
      expect(clearStatus().getAttribute("role")).toBe("status");
      await user.click(screen.getByRole("button", { name: "Удалить" }));
      await vi.waitFor(() => expect(clearStatus().textContent).toBe("Журнал Миши очищен"));
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(mock.calls.filter((c) => c.command === "ClearJournal").map((c) => c.payload)).toEqual([{ childId: 1 }]);
      await vi.waitFor(() => expect(texts()).toEqual(["Миша: журнал очищен"]));
      expect(lastList()).toMatchObject({ childId: 1 });
      await user.selectOptions(screen.getByLabelText("Ребёнок"), "Все дети");
      await vi.waitFor(() =>
        expect(texts()).toEqual(["Миша: журнал очищен", "Коля: попытка обойти защиту", "Неверный пароль родителя"]),
      );
      // Сообщение - про прежний выбор, при смене ребёнка оно уходит.
      expect(clearStatus().textContent).toBe("");
    });

    it("ошибка очистки: уведомление, список прежний", async () => {
      const user = setup(twoChildren());
      const btn = await pickMisha(user);
      await vi.waitFor(() => expect(texts()).toEqual(["Миша: попытка обойти защиту"]));
      mock.override("ClearJournal", { ok: false, error: "Не удалось очистить журнал. Попробуйте ещё раз" });
      await user.click(btn);
      await user.click(screen.getByRole("button", { name: "Удалить" }));
      expect(await screen.findByText("Не удалось очистить журнал. Попробуйте ещё раз")).toBeTruthy();
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(texts()).toEqual(["Миша: попытка обойти защиту"]);
      expect(clearStatus().textContent).toBe("");
    });

    it("пока идёт очистка, «Удалить» недоступна: второй вызов не уходит", async () => {
      const user = setup(twoChildren());
      const btn = await pickMisha(user);
      mock.override("ClearJournal", () => new Promise<never>(() => {}));
      await user.click(btn);
      const del = screen.getByRole("button", { name: "Удалить" });
      await user.click(del);
      expect((del as HTMLButtonElement).disabled).toBe(true);
      await user.click(del);
      expect(mock.count("ClearJournal")).toBe(1);
    });
  });
});
