import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LobbyToday from "./LobbyToday";
import MyStats, { TODAY_POLL_MS } from "../../screens/MyStats";
import { installBackend, mockChild, uninstallBackend, type MockBackend } from "../../__mocks__/backend";
import type { ChildStatus, MyTodayReply } from "../../types";

// Блок «Сегодня» окна ребёнка «Игровое лобби» (ТЗ 8.2.6 п. 1, D-109, D-112, макет child.py).

vi.mock("../../lib/lottie", () => ({
  loadLottie: () =>
    Promise.resolve({
      loadAnimation: () => ({ totalFrames: 90, play() {}, stop() {}, goToAndStop() {}, addEventListener() {}, destroy() {} }),
    }),
}));
vi.mock("../../lib/stickerData", () => ({ loadStickerData: (n: string) => Promise.resolve({ sticker: n }) }));

const NOW = new Date(2026, 10, 27, 15, 0, 0); // пятница, 27 ноября 2026

function status(over: Partial<ChildStatus> = {}): ChildStatus {
  return {
    name: "Миша",
    status: "ok",
    statusText: "",
    label: "",
    tooltip: "",
    summary: "",
    day: "2026-11-27",
    remainingS: 5100,
    limitS: 7200,
    usedS: 2100,
    unlimited: false,
    lock: "none",
    reason: "",
    detail: "",
    until: "",
    pauseUntil: "",
    used: "",
    bonusBalance: 45,
    forbiddenNote: "",
    gamesRemainingS: 1200,
    gamesLimitS: 3600,
    gamesUnlimited: false,
    endMode: "lock",
    closeGames: false,
    studyOnly: false,
    videoRemainingS: 1200,
    videoLimitS: 1800,
    videoUnlimited: false,
    forbidState: "ahead",
    forbidFrom: "21:00",
    forbidTo: "07:00",
    ...over,
  };
}

function reply(s: Partial<ChildStatus> = {}, violationsToday = 0): MyTodayReply {
  return {
    status: status(s),
    today: [],
    rules: [],
    period: "today",
    usedS: 0,
    gamesS: 0,
    studyS: 0,
    otherS: 0,
    allS: 0,
    extraMin: 0,
    bonusSpentMin: 0,
    programs: [],
    bars: [],
    barsBy: "day",
    violations: [],
    bonus: [],
    statsSinceDay: "",
    statsSinceText: "",
    violationsToday,
  };
}

const noop = () => {};

function show(data: MyTodayReply) {
  render(<LobbyToday data={data} now={NOW} onAsk={noop} onSpend={noop} onCode={noop} onAll={noop} onParent={noop} />);
}

const mascot = () => screen.getByRole("region", { name: "Осталось сегодня" }).querySelector("img");

describe("LobbyToday: вид", () => {
  it("шапка: привет, дата, копилка и «Ввести код»", () => {
    show(reply());
    expect(screen.getByRole("heading", { name: "Привет, Миша!" })).toBeTruthy();
    expect(screen.getByText("Пятница, 27 ноября")).toBeTruthy();
    expect(screen.getByText("В копилке").closest(".lb-coins")?.textContent).toBe("В копилке45 мин");
    expect(screen.getByRole("button", { name: "Ввести код" })).toBeTruthy();
  });

  // Ловит: талисман — подпись для диктора (лишний шум) или не неподвижная картинка.
  it("талисман — картинка-украшение 033 при 1 ч 25 мин", () => {
    show(reply());
    expect(mascot()?.getAttribute("alt")).toBe("");
    expect(mascot()?.getAttribute("data-mascot")).toBe("033");
    expect(mascot()?.getAttribute("src")).toMatch(/033[^/]*\.png/);
  });

  // Ловит (8.2.6 п. 1, FR-10, FR-11): кнопки в разных местах экрана или в обратном порядке макета.
  it("«Взять из копилки» и «Попросить ещё» рядом в одной строке", () => {
    show(reply());
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    const ask = screen.getByRole("button", { name: "Попросить ещё" });
    expect(spend.parentElement).toBe(ask.parentElement);
    expect(spend.nextElementSibling).toBe(ask);
    expect(spend.className).toContain("lb-btn-green");
    expect(ask.className).toContain("lb-btn-gold");
  });

  // Ловит (L-17): в запретные часы кнопка рабочая или недоступна молча.
  it("запретные часы: «Взять из копилки» недоступна с пояснением, талисман 104", () => {
    show(reply({ status: "forbidden", lock: "forbidden", forbidState: "active", until: "Можно снова: сегодня в 07:00" }));
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    expect(spend.getAttribute("aria-disabled")).toBe("true");
    const note = screen.getByText("Из копилки можно взять с 07:00");
    expect(spend.getAttribute("aria-describedby")).toBe(note.id);
    expect(mascot()?.getAttribute("data-mascot")).toBe("104");
  });

  // Ловит (возврат T6, п. 1): в запретные часы шкала показывает прежний остаток (70 %), на макете — пустая.
  it("запретные часы: шкала пустая, «потратил 35 мин из 2 часов»", () => {
    show(reply({ status: "forbidden", lock: "forbidden", forbidState: "active", until: "Можно снова: сегодня в 07:00" }));
    const left = screen.getByRole("region", { name: "Осталось сегодня" });
    expect((left.querySelector(".lb-bar-fill") as HTMLElement).style.width).toBe("0%");
    expect(left.querySelector(".lb-spent")?.textContent).toBe("потратил 35 мин из 2 часов");
  });

  // Ловит (возврат T6 №2, п. 2): под пустой шкалой (048, 104) подпись о делениях — объяснять нечего.
  it("подпись «Одно деление: 15 минут» только под непустой шкалой", () => {
    show(reply({ status: "timeUp", lock: "timeUp", remainingS: 0, usedS: 7200 }));
    expect(screen.queryByText("Одно деление: 15 минут")).toBeNull();
  });

  it("запретные часы: без подписи о делениях", () => {
    show(reply({ status: "forbidden", lock: "forbidden", forbidState: "active", until: "Можно снова: сегодня в 07:00" }));
    expect(screen.queryByText("Одно деление: 15 минут")).toBeNull();
  });

  it("остаток 1 ч 25 мин: подпись о делениях есть", () => {
    show(reply());
    expect(screen.getByText("Одно деление: 15 минут")).toBeTruthy();
  });

  // Ловит (возврат T6, п. 3; FR-11, FR-6): после «время вышло» копилка заперта, хотя ТЗ запрещает её
  // только в запретные часы — взятые минуты и выводят из «время вышло».
  it("время вышло: «Взять из копилки» доступна при копилке 45 мин, талисман 048, шкала пустая", () => {
    show(reply({ status: "timeUp", lock: "timeUp", remainingS: 0, usedS: 7200, until: "Можно снова: завтра" }));
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    expect(spend.getAttribute("aria-disabled")).toBeNull();
    expect(document.querySelector(".lb-note")).toBeNull();
    expect(mascot()?.getAttribute("data-mascot")).toBe("048");
    const left = screen.getByRole("region", { name: "Осталось сегодня" });
    expect((left.querySelector(".lb-bar-fill") as HTMLElement).style.width).toBe("0%");
  });

  // Ловит (возврат T6, п. 3): пояснение недоступной кнопки уезжает в отдельную строку под всем блоком и
  // на штатном окне оказывается за краем.
  it("пояснение недоступной кнопки — в блоке кнопок, под ними", () => {
    show(reply({ status: "forbidden", lock: "forbidden", forbidState: "active", until: "Можно снова: сегодня в 07:00" }));
    const note = screen.getByText("Из копилки можно взять с 07:00");
    const spend = screen.getByRole("button", { name: "Взять из копилки" });
    const actions = spend.closest(".lb-actions");
    expect(actions).not.toBeNull();
    expect(actions?.contains(note)).toBe(true);
    expect(spend.compareDocumentPosition(note) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("пауза — «Пауза до 18:30» и 033 при малом остатке", () => {
    show(reply({ status: "paused", pauseUntil: "18:30", remainingS: 300 }));
    expect(screen.getByText("Пауза до 18:30")).toBeTruthy();
    expect(mascot()?.getAttribute("data-mascot")).toBe("033");
  });

  it("без лимита: полная шкала без делений и подписи о делениях", () => {
    show(reply({ unlimited: true, status: "unlimited", limitS: 0, remainingS: 0 }));
    const left = screen.getByRole("region", { name: "Осталось сегодня" });
    expect(left.querySelector(".lb-left-value")?.textContent).toBe("Без лимита");
    expect((left.querySelector(".lb-bar-fill") as HTMLElement).style.width).toBe("100%");
    expect(left.querySelector(".lb-bar-ticks")).toBeNull();
    expect(within(left).queryByText("Одно деление: 15 минут")).toBeNull();
  });

  // Ловит: шкала без делений по 15 мин или заполнена не долей остатка (2 ч → 8 делений, 85 из 120 мин).
  it("шкала: доля остатка от лимита и 8 делений", () => {
    show(reply());
    const left = screen.getByRole("region", { name: "Осталось сегодня" });
    expect((left.querySelector(".lb-bar-fill") as HTMLElement).style.width).toBe("70.8%");
    expect((left.querySelector(".lb-bar-ticks") as HTMLElement).style.getPropertyValue("--lb-ticks")).toBe("8");
  });

  // Ловит (ревью 0.8.4, m2): при 2 ч 10 мин шкала делилась на 9 частей по 14,4 мин под подписью
  // «Одно деление: 15 минут». Ширина деления = 15 мин от лимита: 130 / 15 делений.
  it("шкала: лимит 2 ч 10 мин — деление 15 минут", () => {
    show(reply({ limitS: 7800, remainingS: 5100 }));
    const left = screen.getByRole("region", { name: "Осталось сегодня" });
    const ticks = parseFloat((left.querySelector(".lb-bar-ticks") as HTMLElement).style.getPropertyValue("--lb-ticks"));
    expect(130 / ticks).toBeCloseTo(15, 6);
    expect(within(left).getByText("Одно деление: 15 минут")).toBeTruthy();
  });

  it("запретных часов нет — блока нет; сняты — «Сегодня запрет снят»", () => {
    show(reply({ forbidState: "none", forbidFrom: "", forbidTo: "" }));
    expect(screen.queryByRole("region", { name: "Ночью компьютер спит" })).toBeNull();
  });

  it("запрет на сегодня снят", () => {
    show(reply({ forbidState: "cleared", forbidFrom: "", forbidTo: "" }));
    expect(within(screen.getByRole("region", { name: "Ночью компьютер спит" })).getByText("Сегодня запрет снят")).toBeTruthy();
  });

  it("«Нарушений сегодня: 2»", () => {
    show(reply({}, 2));
    expect(screen.getByText("Нарушений сегодня: 2")).toBeTruthy();
  });

  it("сеанс не ребёнка: ни остатка, ни кнопок времени, ссылки на месте", () => {
    show(reply({ status: "notChild", name: "" }));
    expect(screen.getByRole("heading", { name: "Привет!" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Осталось сегодня" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Попросить ещё" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Ввести код" })).toBeNull();
    expect(screen.getByRole("button", { name: "Вся моя статистика" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Настройки для родителя" })).toBeTruthy();
  });

  // Ловит: тексты ребёнку не на «ты» или с длинным тире (скан NFR-11 смотрит и сюда).
  it("тексты без длинных тире", () => {
    show(reply({}, 1));
    expect(document.body.textContent).not.toMatch(/[—–]/);
  });
});

describe("LobbyToday в окне", () => {
  let mock: MockBackend;

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
  });

  afterEach(() => {
    vi.useRealTimers();
    uninstallBackend();
  });

  function setup(opts: Parameters<typeof installBackend>[0] = {}) {
    mock = installBackend({ mode: "mystats", ...opts });
    const user = userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
    render(<MyStats />);
    return user;
  }

  // Ловит (8.2.6 п. 1, п. 11): «Ввести код» ничего не открывает или открывает не поле промокода.
  it("«Ввести код» открывает поле промокода, Esc закрывает", async () => {
    const user = setup({ children: [mockChild({ bonusBalance: 45 })] });
    await user.click(await screen.findByRole("button", { name: "Ввести код" }));
    const dialog = await screen.findByRole("dialog", { name: "Ввести код" });
    expect(within(dialog).getByRole("textbox", { name: "Промокод" })).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Ввести промокод" })).toBeTruthy();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // Ловит: «Нарушений сегодня» берётся из периода «Итогов» (неделя), а не за сегодня.
  it("«Нарушений сегодня: 2» не зависит от выбранного периода", async () => {
    const at = (h: number) => new Date(2026, 10, 27, h, 0, 0).getTime();
    const ev = (atMs: number, n: number) => ({ atMs, tzOffsetMin: -NOW.getTimezoneOffset(), childId: 1, childName: "Миша", kind: "bypass", text: `Миша: обход защиты ${n}` });
    const user = setup({ events: [ev(at(9), 1), ev(at(10), 2), ev(new Date(2026, 10, 24, 12).getTime(), 3)] });
    expect(await screen.findByText("Нарушений сегодня: 2")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Вся моя статистика" }));
    await user.click(await screen.findByRole("tab", { name: "Неделя" }));
    await screen.findByText("Использовано за неделю");
    await user.click(screen.getByRole("button", { name: "Вернуться к «Сегодня»" }));
    expect(await screen.findByText("Нарушений сегодня: 2")).toBeTruthy();
  });

  // Ловит (8.2.6 п. 1): талисман меняется только после перезапуска окна; окно спрашивает статус, свёрнутым.
  it("остаток 85 → 17 мин: через 10 с талисман 093 без перезапуска окна", async () => {
    setup();
    await screen.findByRole("region", { name: "Осталось сегодня" });
    expect(mascot()?.getAttribute("data-mascot")).toBe("033");
    mock.state.children[0].usedS = 7200 - 17 * 60;
    const before = mock.count("MyToday");
    await vi.advanceTimersByTimeAsync(TODAY_POLL_MS);
    await vi.waitFor(() => expect(mascot()?.getAttribute("data-mascot")).toBe("093"));
    expect(mock.count("MyToday")).toBe(before + 1);
    expect(screen.getByRole("region", { name: "Осталось сегодня" }).querySelector(".lb-left-value")?.textContent).toBe("17 мин");
  });

  it("скрытое окно статус не спрашивает", async () => {
    setup();
    await screen.findByRole("region", { name: "Осталось сегодня" });
    const visibility = vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    const before = mock.count("MyToday");
    await vi.advanceTimersByTimeAsync(TODAY_POLL_MS * 3);
    expect(mock.count("MyToday")).toBe(before);
    visibility.mockRestore();
  });
});
