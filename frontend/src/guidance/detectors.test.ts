import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { findRisks, loadRisks, type Risk } from "./detectors";
import { installBackend, mockChild, statusOf, uninstallBackend } from "../__mocks__/backend";
import type { TodayCard, WeekdayLimit } from "../types";

// Напоминания о важном (ТЗ 8.4 п.2): у ребёнка «Без лимита» во все дни недели (D-M1-01) и
// детская учётка стала администратором (D-M1-02). Только явные признаки, никаких догадок.

const NOW = new Date(2026, 9, 19, 12, 0, 0).getTime();

const week = (fn: (weekday: number) => Partial<WeekdayLimit>): WeekdayLimit[] =>
  [1, 2, 3, 4, 5, 6, 7].map((weekday) => ({ weekday, totalMin: 120, unlimited: false, ...fn(weekday) }));

const allFree = week(() => ({ totalMin: 0, unlimited: true }));

function card(over: Partial<ReturnType<typeof mockChild>> = {}): TodayCard {
  const child = mockChild(over);
  return {
    childId: child.childId,
    sid: child.sid,
    name: child.name,
    status: statusOf(child, NOW),
    extraMin: child.extraMin,
    canClearBlock: false,
  };
}

describe("findRisks", () => {
  it("«Без лимита» во все семь дней: D-M1-01", () => {
    expect(findRisks([card()], { 1: allFree })).toEqual<Risk[]>([{ entryId: "D-M1-01", childId: 1, name: "Миша" }]);
  });

  // Ловит: один день без лимита (воскресенье в тестовых данных) принят за «нет ограничений вообще».
  it("один день без лимита - это нормально", () => {
    const limits = week((d) => (d === 7 ? { totalMin: 0, unlimited: true } : {}));
    expect(findRisks([card()], { 1: limits })).toEqual([]);
  });

  it("ноль минут во все дни - не «без лимита», а «нельзя»: напоминания нет", () => {
    expect(findRisks([card()], { 1: week(() => ({ totalMin: 0 })) })).toEqual([]);
  });

  // Ловит: короткий или пустой список правил принят за «без лимита» (пустое - не явный признак).
  it("правила не получены или неполные: молчим", () => {
    expect(findRisks([card()], {})).toEqual([]);
    expect(findRisks([card()], { 1: [] })).toEqual([]);
    const sixFree = allFree.slice(0, 6);
    expect(findRisks([card()], { 1: sixFree })).toEqual([]);
  });

  it("детская учётка стала администратором: D-M1-02", () => {
    expect(findRisks([card({ admin: true })], {})).toEqual<Risk[]>([{ entryId: "D-M1-02", childId: 1, name: "Миша" }]);
  });

  // Ловит: у администратора два напоминания сразу (его «без лимита» - следствие, а не причина).
  it("у администратора только D-M1-02, даже если правила «без лимита»", () => {
    expect(findRisks([card({ admin: true })], { 1: allFree })).toEqual<Risk[]>([
      { entryId: "D-M1-02", childId: 1, name: "Миша" },
    ]);
  });

  it("безопасный режим и «не ребёнок»: напоминаний нет", () => {
    const safe = card();
    safe.status = { ...safe.status, status: "safe" };
    const notChild = card();
    notChild.status = { ...notChild.status, status: "notChild" };
    expect(findRisks([safe, notChild], { 1: allFree })).toEqual([]);
  });

  it("несколько детей: по напоминанию на каждого, у кого риск", () => {
    const cards = [card(), card({ childId: 2, name: "Даша" }), card({ childId: 3, name: "Егор", admin: true })];
    const found = findRisks(cards, { 1: allFree, 2: week(() => ({})), 3: allFree });
    expect(found).toEqual<Risk[]>([
      { entryId: "D-M1-01", childId: 1, name: "Миша" },
      { entryId: "D-M1-02", childId: 3, name: "Егор" },
    ]);
  });
});

describe("loadRisks: данные из программы", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true, now: NOW });
  });
  afterEach(() => {
    vi.useRealTimers();
    uninstallBackend();
  });

  it("читает карточки и правила детей, у кого есть риск - тот в списке", async () => {
    installBackend({
      sessionActive: true,
      children: [
        mockChild({ limits: allFree }),
        mockChild({ childId: 2, sid: "S-2", name: "Даша" }),
        mockChild({ childId: 3, sid: "S-3", name: "Егор", admin: true }),
      ],
    });
    expect(await loadRisks()).toEqual<Risk[]>([
      { entryId: "D-M1-01", childId: 1, name: "Миша" },
      { entryId: "D-M1-02", childId: 3, name: "Егор" },
    ]);
  });

  it("правила администратора не запрашиваются: они ничего не решают", async () => {
    const mock = installBackend({ sessionActive: true, children: [mockChild({ admin: true })] });
    await loadRisks();
    expect(mock.count("GetChildRules")).toBe(0);
  });

  // Ловит: сбой чтения правил одного ребёнка роняет напоминания для остальных.
  it("правила одного ребёнка не прочитались: он пропущен, остальные проверены", async () => {
    const mock = installBackend({
      sessionActive: true,
      children: [mockChild({ limits: allFree }), mockChild({ childId: 2, sid: "S-2", name: "Даша", limits: allFree })],
    });
    mock.override("GetChildRules", "reject", 1);
    expect(await loadRisks()).toEqual<Risk[]>([{ entryId: "D-M1-01", childId: 2, name: "Даша" }]);
  });

  it("список детей не прочитался: ошибка наверх, решать будет Provider", async () => {
    const mock = installBackend({ sessionActive: true });
    mock.override("GetToday", "reject");
    await expect(loadRisks()).rejects.toBeDefined();
  });
});
