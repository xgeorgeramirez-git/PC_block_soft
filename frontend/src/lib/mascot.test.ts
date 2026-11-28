import { describe, expect, it } from "vitest";
import type { ChildStatus } from "../types";
import { energyColors, mascotOf, MASCOT_SRC } from "./mascot";

// Талисман окна ребёнка (ТЗ 8.2.6 п. 1, D-109, D-112): одна функция порогов с цветом остатка у родителя
// (remainMinutes, вверх до целой минуты), запретные часы главнее остатка, пауза и «без лимита» — 033.

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
    gamesRemainingS: 0,
    gamesLimitS: 0,
    gamesUnlimited: true,
    endMode: "lock",
    closeGames: false,
    studyOnly: false,
    ...over,
  };
}

describe("mascotOf: пороги по остатку", () => {
  // Ловит: порог сравнивается в секундах (3600 > 3599) или округляется вниз: на 60/61 и 15/16 мин талисман
  // расходится с цветом остатка у родителя и с числом на значке.
  it.each([
    [3660, "033"],
    [3601, "033"],
    [3600, "093"],
    [961, "093"],
    [901, "093"],
    [900, "031"],
    [60, "031"],
    [1, "031"],
    [0, "048"],
  ] as const)("%i с → %s", (remainingS, want) => {
    expect(mascotOf(st({ remainingS, status: remainingS > 900 ? "ok" : remainingS > 0 ? "low" : "timeUp" }))).toBe(want);
  });

  // Ловит: экран «время вышло» с остатком из прошлого ответа (секунды не обнулены) показывает весёлый талисман.
  it("экран «время вышло» — 048 при любом остатке", () => {
    expect(mascotOf(st({ remainingS: 0, lock: "timeUp", status: "timeUp" }))).toBe("048");
    expect(mascotOf(st({ remainingS: 600, lock: "timeUp", status: "timeUp" }))).toBe("048");
  });

  // Ловит: «Оставить только учёбу» после конца времени — тоже «время кончилось».
  it("время вышло, открыта только учёба — 048", () => {
    expect(mascotOf(st({ remainingS: 0, status: "timeUp", studyOnly: true }))).toBe("048");
  });

  // Ловит: запретные часы проверяются после остатка, и при 3 ч остатка ночью талисман «времени много».
  it("запретные часы главнее остатка — 104", () => {
    expect(mascotOf(st({ remainingS: 10800, lock: "forbidden", status: "forbidden" }))).toBe("104");
    expect(mascotOf(st({ remainingS: 10800, status: "forbidden" }))).toBe("104");
    expect(mascotOf(st({ remainingS: 0, lock: "forbidden", status: "forbidden" }))).toBe("104");
  });

  // Ловит: пауза при малом остатке показывает тревожный талисман, хотя время не считается.
  it("пауза — 033 при любом остатке", () => {
    expect(mascotOf(st({ remainingS: 600, status: "paused", pauseUntil: "18:30" }))).toBe("033");
  });

  it("без лимита — 033", () => {
    expect(mascotOf(st({ remainingS: 0, limitS: 0, unlimited: true, status: "unlimited" }))).toBe("033");
  });

  // Ловит: учётка-администратор или безопасный режим (ограничения не действуют) с нулём в остатке — 048.
  it("ограничения не действуют — 033", () => {
    expect(mascotOf(st({ remainingS: 0, status: "admin" }))).toBe("033");
    expect(mascotOf(st({ remainingS: 0, status: "safe" }))).toBe("033");
  });

  // Ловит: перерыв на отдых считался «время вышло»: время не кончилось, талисман по остатку.
  it("перерыв на отдых — по остатку", () => {
    expect(mascotOf(st({ remainingS: 4000, lock: "rest" }))).toBe("033");
  });
});

describe("energyColors: цвет шкалы по талисману (moods.py)", () => {
  it.each([
    ["033", ["#8DFFC4", "#22C47E"]],
    ["093", ["#8DFFC4", "#22C47E"]],
    ["031", ["#FFE066", "#FFB800"]],
    ["048", ["#FFC2A8", "#FF6A4A"]],
    ["104", ["#C9C2FF", "#7B6CFF"]],
  ] as const)("%s", (m, want) => {
    expect(energyColors(m)).toEqual(want);
  });
});

describe("картинки талисмана", () => {
  // Ловит: у талисмана нет картинки (опечатка в номере): в окне пустое место вместо стикера.
  it("у каждого талисмана свой первый кадр .png", () => {
    for (const m of ["033", "093", "031", "048", "104"] as const) {
      expect(MASCOT_SRC[m]).toMatch(new RegExp(`${m}[^/]*\\.png`));
    }
  });
});
