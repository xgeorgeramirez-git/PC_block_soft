import { describe, expect, it } from "vitest";
import type { ChildStatus } from "../../types";
import { dateLine, greeting, nightLine, ofTime, remainBig, spentLines, tileGames, tileVideo, ticksOf, violationsLine } from "./lobbyText";

// Тексты блока «Сегодня» окна ребёнка «Игровое лобби» (ТЗ 8.2.6 п. 1, макет child.py): ребёнку на «ты»,
// минуты остатка вверх до целой (как значок и цвет у родителя).

function st(over: Partial<ChildStatus> = {}): ChildStatus {
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

describe("шапка", () => {
  it("«Привет, Миша!»; без имени — «Привет!»", () => {
    expect(greeting("Миша")).toBe("Привет, Миша!");
    expect(greeting("  ")).toBe("Привет!");
  });

  // Ловит: дата из UTC (после 21:00 по Москве показывает завтра) или день недели с маленькой буквы.
  it("«Пятница, 27 ноября» по местному времени", () => {
    expect(dateLine(new Date(2026, 10, 27, 23, 30))).toBe("Пятница, 27 ноября");
    expect(dateLine(new Date(2026, 10, 30, 0, 5))).toBe("Понедельник, 30 ноября");
    expect(dateLine(new Date(2027, 0, 3, 12, 0))).toBe("Воскресенье, 3 января");
  });
});

describe("ofTime: «из 2 часов» (родительный падеж после «из»)", () => {
  it.each([
    [120, "из 2 часов"],
    [60, "из 1 часа"],
    [1260, "из 21 часа"],
    [300, "из 5 часов"],
    [30, "из 30 минут"],
    [1, "из 1 минуты"],
    [21, "из 21 минуты"],
    [11, "из 11 минут"],
    [90, "из 1 ч 30 мин"],
  ])("%i мин → %s", (min, want) => {
    expect(ofTime(min)).toBe(want);
  });
});

describe("главный блок", () => {
  // Ловит: остаток вниз (1 ч 24 мин при 5041 с) — окно расходится со значком на минуту.
  it("остаток вверх до минуты: 5041 с → «1 ч 25 мин», 59 с → «1 мин»", () => {
    expect(remainBig(st({ remainingS: 5041 }))).toBe("1 ч 25 мин");
    expect(remainBig(st({ remainingS: 59 }))).toBe("1 мин");
    expect(remainBig(st({ remainingS: 0, status: "timeUp", lock: "timeUp" }))).toBe("0 мин");
  });

  it("без лимита, пауза, запретные часы", () => {
    expect(remainBig(st({ unlimited: true, status: "unlimited" }))).toBe("Без лимита");
    expect(remainBig(st({ status: "paused", pauseUntil: "18:30" }))).toBe("Пауза до 18:30");
    expect(remainBig(st({ status: "forbidden", lock: "forbidden" }))).toBe("Сейчас нельзя");
  });

  // Ловит: экран «время вышло» с секундами из ответа показывает «5 мин» (кнопкой времени не воспользоваться).
  it("экран «время вышло» — 0 мин при любых секундах", () => {
    expect(remainBig(st({ remainingS: 300, status: "timeUp", lock: "timeUp" }))).toBe("0 мин");
  });

  // Ловит: «потратил» считается из usedS, а не «лимит − остаток»: 35 + 1 ч 25 мин ≠ 2 часа на экране.
  it("«потратил 35 мин» / «из 2 часов» — лимит минус показанный остаток", () => {
    expect(spentLines(st({ remainingS: 5041, usedS: 2159 }))).toEqual(["потратил 35 мин", "из 2 часов"]);
    expect(spentLines(st({ remainingS: 0, limitS: 5400, status: "timeUp", lock: "timeUp" }))).toEqual(["потратил 1 ч 30 мин", "из 1 ч 30 мин"]);
  });

  it("без лимита — только сколько потратил", () => {
    expect(spentLines(st({ unlimited: true, status: "unlimited", usedS: 2400 }))).toEqual(["потратил 40 мин"]);
  });

  it("ограничения не действуют — строки нет", () => {
    expect(spentLines(st({ status: "admin" }))).toEqual([]);
  });

  // Ловит: шкала без делений или с дробным числом делений (лимит 2 ч = 8 делений по 15 мин).
  it("деления шкалы по 15 мин", () => {
    expect(ticksOf(st({ limitS: 7200 }))).toBe(8);
    expect(ticksOf(st({ limitS: 5400 }))).toBe(6);
    expect(ticksOf(st({ limitS: 600 }))).toBe(0);
    expect(ticksOf(st({ unlimited: true, status: "unlimited" }))).toBe(0);
  });
});

describe("плитки «Игры» и «Видео и соцсети»", () => {
  it("«20 мин», полоска, «осталось из 1 часа»", () => {
    expect(tileGames(st())).toEqual({ big: "20 мин", share: 1200 / 3600, sub: "осталось из 1 часа" });
    expect(tileVideo(st())).toEqual({ big: "20 мин", share: 1200 / 1800, sub: "осталось из 30 минут" });
  });

  it("без отдельного лимита — «Без лимита» без полоски", () => {
    expect(tileGames(st({ gamesUnlimited: true }))).toEqual({ big: "Без лимита", share: null, sub: "входит в общее время" });
    expect(tileVideo(st({ videoUnlimited: true, videoLimitS: 0 }))).toEqual({ big: "Без лимита", share: null, sub: "входит в общее время" });
    // Старая служба без полей видео: лимита нет.
    expect(tileVideo(st({ videoUnlimited: undefined, videoLimitS: undefined, videoRemainingS: undefined })).big).toBe("Без лимита");
  });

  // Ловит: после «время вышло» плитки показывают прежний остаток игр, будто играть ещё можно.
  it("время вышло или только учёба — 0 мин", () => {
    expect(tileGames(st({ lock: "timeUp", status: "timeUp", remainingS: 0 })).big).toBe("0 мин");
    expect(tileGames(st({ studyOnly: true })).big).toBe("0 мин");
    expect(tileVideo(st({ studyOnly: true })).share).toBe(0);
  });

  it("лимит игр 0 — «Сегодня нельзя»", () => {
    expect(tileGames(st({ gamesLimitS: 0, gamesRemainingS: 0 }))).toEqual({ big: "0 мин", share: 0, sub: "сегодня игры нельзя" });
  });
});

describe("ночь и нарушения", () => {
  it("запретные часы: будут, идут, сняты, нет", () => {
    expect(nightLine(st())).toBe("с 21:00 до 07:00");
    expect(nightLine(st({ forbidState: "active" }))).toBe("с 21:00 до 07:00");
    expect(nightLine(st({ forbidState: "cleared", forbidFrom: "", forbidTo: "" }))).toBe("Сегодня запрет снят");
    expect(nightLine(st({ forbidState: "none" }))).toBeNull();
    expect(nightLine(st({ forbidState: undefined }))).toBeNull();
  });

  it("«Нарушений сегодня нет» / «Нарушений сегодня: 2»", () => {
    expect(violationsLine(0)).toBe("Нарушений сегодня нет");
    expect(violationsLine(2)).toBe("Нарушений сегодня: 2");
  });
});
