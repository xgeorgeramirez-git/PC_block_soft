import { describe, expect, it } from "vitest";
import { firstRunLines, summaryLines, todayRuleWeekday } from "./mytoday";
import type { ChildStatus, MyTodayReply } from "../types";

// Окна ребёнка (FR-12, FR-32): тексты собираются из ответа MyToday, а не пересчитываются.

function status(over: Partial<ChildStatus> = {}): ChildStatus {
  return {
    name: "Миша",
    status: "ok",
    statusText: "Можно",
    label: "85",
    tooltip: "Осталось 1 ч 25 мин",
    summary: "Сегодня осталось 1 ч 25 мин из 2 ч",
    day: "2026-10-19",
    remainingS: 5100,
    limitS: 7200,
    usedS: 2100,
    unlimited: false,
    lock: "none",
    reason: "",
    detail: "",
    until: "",
    pauseUntil: "",
    bonusBalance: 0,
    forbiddenNote: "",
    used: "Сегодня: 35 мин из 2 ч",
    ...over,
  };
}

function reply(over: Partial<ChildStatus> = {}, today: string[] = []): MyTodayReply {
  return { status: status(over), today, rules: [] };
}

describe("firstRunLines: пересказ правил на сегодня", () => {
  // Ловит: пересказ берёт лимит не из ответа службы (расхождение с значком).
  it("лимит на сегодня и где смотреть остаток", () => {
    expect(firstRunLines(reply())).toEqual(["Сегодня можно 2 ч.", "Сколько осталось, видно в значке у часов."]);
  });

  it("доп. время уже входит в лимит дня", () => {
    expect(firstRunLines(reply({ limitS: 9000 }))[0]).toBe("Сегодня можно 2 ч 30 мин.");
  });

  it("лимит меньше часа", () => {
    expect(firstRunLines(reply({ limitS: 1800 }))[0]).toBe("Сегодня можно 30 мин.");
  });

  it("без лимита: значок без цифр, поэтому про значок не говорим", () => {
    expect(firstRunLines(reply({ status: "unlimited", unlimited: true, limitS: 0 }))).toEqual([
      "Сегодня без ограничений по времени.",
    ]);
  });

  it("0 минут на день: пользоваться нельзя", () => {
    expect(firstRunLines(reply({ limitS: 0 }))).toEqual(["Сегодня компьютером пользоваться нельзя."]);
  });

  // Ловит: окно обещает счёт времени там, где ограничения не действуют (администратор, правила не прочитаны).
  it.each(["admin", "safe", "notChild"] as const)("статус %s: честно, что ограничения не действуют", (kind) => {
    expect(firstRunLines(reply({ status: kind, unlimited: true, limitS: 0 }))).toEqual([
      "Сейчас ограничения не действуют. Если это ошибка, скажи родителю.",
    ]);
  });

  // Ловит: запретные часы в M1 (их ещё нет) попадают в пересказ.
  it("запретные часы не упоминаются", () => {
    const text = firstRunLines(reply()).join(" ");
    expect(text).not.toMatch(/запретн|с \d\d:\d\d до/i);
  });
});

describe("summaryLines: «Итоги»", () => {
  // Ловит: «Использовано» показано дважды, в строках и в строке «Всего».
  it("убирает строку «Использовано …», она есть в «Всего»", () => {
    const r = reply({ usedS: 2100 }, ["Сегодня осталось 1 ч 25 мин из 2 ч", "Использовано 35 мин"]);
    expect(summaryLines(r)).toEqual(["Сегодня осталось 1 ч 25 мин из 2 ч"]);
  });

  it("строку про «Можно снова» оставляет", () => {
    const r = reply({ usedS: 7200, status: "timeUp" }, [
      "Время на сегодня закончилось",
      "Использовано 2 ч 00 мин",
      "Можно снова: завтра",
    ]);
    expect(summaryLines(r)).toEqual(["Время на сегодня закончилось", "Можно снова: завтра"]);
  });

  it("пустой ответ - пустой список", () => {
    expect(summaryLines(reply({}, []))).toEqual([]);
  });
});

describe("день недели", () => {
  it("понедельник 19 октября 2026 - 1", () => {
    expect(todayRuleWeekday(new Date(2026, 9, 19, 12, 0, 0))).toBe(1);
  });

  it("воскресенье - 7", () => {
    expect(todayRuleWeekday(new Date(2026, 9, 25, 12, 0, 0))).toBe(7);
  });
});
