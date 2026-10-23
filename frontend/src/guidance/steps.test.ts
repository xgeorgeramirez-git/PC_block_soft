import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { computeFirstSteps, loadFirstSteps, type StepInput } from "./steps";
import { installBackend, mockChild, statusOf, uninstallBackend } from "../__mocks__/backend";
import type { JournalEntry, TodayCard, WeekdayLimit } from "../types";

// «Первые шаги» в «Помощи» (ТЗ 8.4 п.6): пункты отмечаются сами по факту.

const NOW = new Date(2026, 9, 19, 12, 0, 0).getTime();

const free: WeekdayLimit[] = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 0, unlimited: true }));
const normal: WeekdayLimit[] = [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false }));

function card(over: Parameters<typeof mockChild>[0] = {}): TodayCard {
  const child = mockChild(over);
  return {
    childId: child.childId,
    sid: child.sid,
    name: child.name,
    status: statusOf(child, NOW),
    extraMin: child.extraMin,
    bonusBalance: 0,
    canClearBlock: false,
    noForbiddenToday: false,
  };
}

const ev = (kind: string, text: string): JournalEntry => ({ atMs: NOW - 1000, tzOffsetMin: 180, childId: 1, childName: "Миша", kind, text });

function input(over: Partial<StepInput> = {}): StepInput {
  return { configured: true, cards: [], limitsByChild: {}, events: [], ...over };
}

const doneIds = (steps: ReturnType<typeof computeFirstSteps>) => steps.filter((s) => s.done).map((s) => s.id);

describe("computeFirstSteps: пункты и отметки", () => {
  it("пять пунктов в порядке ТЗ 8.4 п.6", () => {
    expect(computeFirstSteps(input()).map((s) => s.id)).toEqual(["password", "children", "limits", "child-login", "extra-time"]);
  });

  it("новая установка: сделан только пароль", () => {
    expect(doneIds(computeFirstSteps(input()))).toEqual(["password"]);
  });

  it("пароль не задан - не отмечен", () => {
    expect(doneIds(computeFirstSteps(input({ configured: false })))).toEqual([]);
  });

  it("дети отмечены - пункт сделан", () => {
    expect(doneIds(computeFirstSteps(input({ cards: [card({ usedS: 0 })] })))).toEqual(["password", "children"]);
  });

  // Ловит: «лимиты заданы» отмечено, хотя у всех детей «Без лимита» во все дни.
  it("лимиты заданы, если хотя бы у одного ребёнка есть день с лимитом", () => {
    const base = input({ cards: [card({ usedS: 0 })] });
    expect(doneIds(computeFirstSteps({ ...base, limitsByChild: { 1: free } }))).not.toContain("limits");
    expect(doneIds(computeFirstSteps({ ...base, limitsByChild: { 1: normal } }))).toContain("limits");
    expect(doneIds(computeFirstSteps({ ...base, limitsByChild: {} }))).not.toContain("limits");
  });

  it("ребёнок вошёл: сегодня уже есть время за компьютером", () => {
    const used = card({ usedS: 600 });
    expect(doneIds(computeFirstSteps(input({ cards: [used] })))).toContain("child-login");
    const fresh = card({ usedS: 0 });
    expect(doneIds(computeFirstSteps(input({ cards: [fresh] })))).not.toContain("child-login");
  });

  it("ребёнок вошёл раньше: в журнале итог прошлого дня не ноль", () => {
    const cards = [card({ usedS: 0 })];
    const past = [ev("day_closed", "Миша: итог дня 18 октября: 1 ч 05 мин")];
    expect(doneIds(computeFirstSteps(input({ cards, events: past })))).toContain("child-login");
    const zero = [ev("day_closed", "Миша: итог дня 18 октября: 0 мин")];
    expect(doneIds(computeFirstSteps(input({ cards, events: zero })))).not.toContain("child-login");
  });

  it("время добавлено: в журнале есть «доп. время»", () => {
    const events = [ev("extra_time", "Миша: доп. время +30 мин")];
    expect(doneIds(computeFirstSteps(input({ events })))).toContain("extra-time");
    expect(doneIds(computeFirstSteps(input({ events: [ev("bypass", "x")] })))).not.toContain("extra-time");
  });

  // Ловит: у каждого невыполненного пункта нет пути к месту, где его сделать (L-08).
  it("у невыполненного пункта есть переход туда, где его сделать", () => {
    const steps = computeFirstSteps(input());
    for (const s of steps.filter((x) => !x.done)) {
      expect(s.go, s.id).toBeDefined();
      expect(s.goLabel, s.id).toBeTruthy();
    }
    expect(steps.find((s) => s.id === "children")?.go).toBe("settings");
    expect(steps.find((s) => s.id === "limits")?.go).toBe("children");
    expect(steps.find((s) => s.id === "extra-time")?.go).toBe("today");
  });

  it("тексты без длинных тире и без эмодзи", () => {
    for (const s of computeFirstSteps(input())) {
      expect(`${s.label} ${s.goLabel ?? ""}`).not.toMatch(/[‒-―−]|\p{Extended_Pictographic}/u);
    }
  });
});

describe("loadFirstSteps: данные из программы", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
  });
  afterEach(() => {
    vi.useRealTimers();
    uninstallBackend();
  });

  it("читает детей, правила и журнал", async () => {
    installBackend({
      sessionActive: true,
      children: [mockChild({ usedS: 600 })],
      events: [ev("extra_time", "Миша: доп. время +30 мин")],
    });
    const steps = await loadFirstSteps();
    expect(doneIds(steps)).toEqual(["password", "children", "limits", "child-login", "extra-time"]);
  });

  it("журнал не прочитался: пункт про время просто не отмечен, остальное считается", async () => {
    const mock = installBackend({ sessionActive: true, children: [mockChild()] });
    mock.override("ListEvents", "reject");
    const steps = await loadFirstSteps();
    expect(doneIds(steps)).toEqual(["password", "children", "limits", "child-login"]);
  });

  it("список детей не прочитался: ошибка наверх", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("GetToday", "reject");
    await expect(loadFirstSteps()).rejects.toBeDefined();
  });
});
