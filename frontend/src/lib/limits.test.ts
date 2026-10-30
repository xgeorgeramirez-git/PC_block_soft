import { describe, expect, it } from "vitest";
import {
  GAMES_FALLBACK_MIN,
  MAX_LIMIT_MIN,
  copyDay,
  limitErrorKey,
  joinMinutes,
  limitsEqual,
  normalizeLimits,
  setDay,
  toggleUnlimited,
  wireLimits,
  splitMinutes,
  templateLimits,
  validateLimits,
} from "./limits";
import type { WeekdayLimit } from "../types";

const week = (mins: number[], unlimitedDays: number[] = []): WeekdayLimit[] =>
  mins.map((totalMin, i) => ({
    weekday: i + 1,
    totalMin,
    unlimited: unlimitedDays.includes(i + 1),
    gamesMin: 0,
    gamesUnlimited: true,
  }));

/** Неделя с играми: games[i] = null - игры без лимита. */
const withGamesWeek = (mins: number[], games: (number | null)[]): WeekdayLimit[] =>
  mins.map((totalMin, i) => ({
    weekday: i + 1,
    totalMin,
    unlimited: false,
    gamesMin: games[i] ?? 0,
    gamesUnlimited: games[i] === null,
  }));

describe("шаблон", () => {
  // Ловит: шаблон «будни 2 ч, выходные 4 ч» путает дни.
  it("будни 2 ч, выходные 4 ч", () => {
    expect(templateLimits()).toEqual(week([120, 120, 120, 120, 120, 240, 240]));
  });
  it("шаблон с другими числами", () => {
    expect(templateLimits(60, 180)).toEqual(week([60, 60, 60, 60, 60, 180, 180]));
  });
});

describe("часы и минуты", () => {
  it("разбор и сборка", () => {
    expect(splitMinutes(90)).toEqual({ h: 1, m: 30 });
    expect(splitMinutes(0)).toEqual({ h: 0, m: 0 });
    expect(splitMinutes(1440)).toEqual({ h: 24, m: 0 });
    expect(joinMinutes(1, 30)).toBe(90);
    expect(joinMinutes(0, 75)).toBe(75);
  });
  // Ловит: отрицательные и дробные значения полей уходят в программу.
  it("отрицательное и дробное приводятся к целому не меньше нуля", () => {
    expect(joinMinutes(-1, 10)).toBe(10);
    expect(joinMinutes(1.9, 0.4)).toBe(60);
    expect(joinMinutes(Number.NaN, 5)).toBe(5);
  });
});

describe("normalizeLimits", () => {
  it("всегда семь дней по порядку", () => {
    const messy: WeekdayLimit[] = [
      { weekday: 3, totalMin: 90, unlimited: false },
      { weekday: 1, totalMin: 60, unlimited: false },
    ];
    const got = normalizeLimits(messy);
    expect(got).toHaveLength(7);
    expect(got.map((l) => l.weekday)).toEqual([1, 2, 3, 4, 5, 6, 7]);
    expect(got[0]).toEqual({ weekday: 1, totalMin: 60, unlimited: false, gamesMin: 0, gamesUnlimited: true });
    expect(got[2]).toEqual({ weekday: 3, totalMin: 90, unlimited: false, gamesMin: 0, gamesUnlimited: true });
  });
  // Ловит: пустой ответ (null вместо массива) роняет таблицу.
  it("пусто и null дают шаблон", () => {
    expect(normalizeLimits([])).toEqual(templateLimits());
    expect(normalizeLimits(null)).toEqual(templateLimits());
    expect(normalizeLimits(undefined)).toEqual(templateLimits());
  });
  it("у «без лимита» минуты сбрасываются в 0", () => {
    const got = normalizeLimits([{ weekday: 7, totalMin: 55, unlimited: true }]);
    expect(got[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
  });
  // Ловит: лимит игр из ответа программы теряется при чтении (таблица показывает «без лимита»).
  it("лимит игр из ответа сохраняется, нет полей игр - игры без лимита", () => {
    const got = normalizeLimits([
      { weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false },
      { weekday: 3, totalMin: 90, unlimited: false, gamesMin: 0, gamesUnlimited: false },
      { weekday: 6, totalMin: 240, unlimited: false, gamesMin: 45, gamesUnlimited: true },
      { weekday: 7, totalMin: 0, unlimited: true },
    ]);
    expect(got[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false });
    expect(got[2]).toEqual({ weekday: 3, totalMin: 90, unlimited: false, gamesMin: 0, gamesUnlimited: false });
    expect(got[5]).toEqual({ weekday: 6, totalMin: 240, unlimited: false, gamesMin: 0, gamesUnlimited: true });
    expect(got[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
  });
});

describe("правка дней", () => {
  const base = week([120, 120, 90, 120, 120, 240, 0], [7]);

  it("setDay меняет один день и не трогает исходный список", () => {
    const next = setDay(base, 1, { totalMin: 60 });
    expect(next[0].totalMin).toBe(60);
    expect(base[0].totalMin).toBe(120);
    expect(next[1]).toEqual(base[1]);
  });

  // Ловит: случайный щелчок по «Без лимита» и обратно обнуляет день (а 0 - это «нельзя»).
  it("«без лимита» помнит минуты дня", () => {
    const on = toggleUnlimited(base, 2, true);
    expect(on[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: true, gamesMin: 0, gamesUnlimited: true });
    const off = toggleUnlimited(on, 2, false);
    expect(off[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true });
  });

  it("снять «без лимита» с дня без минут - не 0, а значение шаблона", () => {
    const sunday = toggleUnlimited(base, 7, false);
    expect(sunday[6]).toEqual({ weekday: 7, totalMin: 240, unlimited: false, gamesMin: 0, gamesUnlimited: true });
    const tuesday = toggleUnlimited(toggleUnlimited(setDay(base, 2, { totalMin: 0 }), 2, true), 2, false);
    expect(tuesday[1].totalMin).toBe(120);
  });

  it("другие дни и исходный список не меняются", () => {
    const next = toggleUnlimited(base, 2, true);
    expect(next[0]).toBe(base[0]);
    expect(base[1].unlimited).toBe(false);
  });

  // Ловит: в программу уходят скрытые минуты «без лимита» (контракт: тогда TotalMin = 0).
  it("wireLimits обнуляет минуты у «без лимита»", () => {
    const hidden = toggleUnlimited(base, 1, true);
    const sent = wireLimits(hidden);
    expect(sent[0]).toEqual({ weekday: 1, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
    expect(sent[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true });
    expect(hidden[0].totalMin).toBe(120);
  });

  // Ловит: «Как в понедельник» захватывает субботу или не трогает пятницу.
  it("«Как в понедельник: на все будни» - вторник…пятница", () => {
    const start = week([45, 120, 90, 120, 120, 240, 0], [7]);
    const next = copyDay(start, 1, [2, 3, 4, 5]);
    expect(next.map((l) => l.totalMin)).toEqual([45, 45, 45, 45, 45, 240, 0]);
    expect(next[5]).toEqual(start[5]);
  });

  it("«Как в субботу: на воскресенье» переносит и «без лимита»", () => {
    const start = week([120, 120, 120, 120, 120, 0, 60], [6]);
    const next = copyDay(start, 6, [7]);
    expect(next[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true, gamesMin: 0, gamesUnlimited: true });
  });
});

describe("limitsEqual", () => {
  it("одинаковые списки равны, любая разница - нет", () => {
    const a = week([120, 120, 90, 120, 120, 240, 0], [7]);
    expect(limitsEqual(a, a.map((l) => ({ ...l })))).toBe(true);
    expect(limitsEqual(a, setDay(a, 3, { totalMin: 91 }))).toBe(false);
    expect(limitsEqual(a, setDay(a, 7, { unlimited: false }))).toBe(false);
  });
  // Ловит: у «без лимита» разные скрытые минуты считаются правкой.
  it("минуты при «без лимита» не считаются", () => {
    const a = week([120, 120, 90, 120, 120, 240, 0], [7]);
    const b = a.map((l) => (l.weekday === 7 ? { ...l, totalMin: 33 } : l));
    expect(limitsEqual(a, b)).toBe(true);
  });
});

describe("validateLimits", () => {
  it("верные лимиты без ошибок", () => {
    expect(validateLimits(week([120, 120, 90, 120, 120, 240, 0], [7]))).toEqual({});
    expect(validateLimits(week([0, 0, 0, 0, 0, 0, 0]))).toEqual({});
  });
  // Ловит: лимит больше суток уходит в программу без ошибки у поля.
  it("больше 24 ч - ошибка у этого дня", () => {
    const got = validateLimits(week([120, 120, MAX_LIMIT_MIN + 1, 120, 120, 240, 0]));
    expect(got).toEqual({ "limits.3": "Не больше 24 ч" });
  });
  it("«без лимита» не проверяется по минутам", () => {
    expect(validateLimits(week([120, 120, 5000, 120, 120, 240, 0], [3]))).toEqual({});
  });
});

describe("столбец «Игры» (FR-16, R5)", () => {
  // Ловит: столбцы «Общее» и «Игры» перепутаны при отправке в программу.
  it("wireLimits отправляет общее в totalMin, игры в gamesMin", () => {
    const sent = wireLimits(withGamesWeek([120, 120, 90, 120, 120, 240, 240], [60, null, 0, null, null, 150, null]));
    expect(sent[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 60, gamesUnlimited: false });
    expect(sent[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true });
    expect(sent[2]).toEqual({ weekday: 3, totalMin: 90, unlimited: false, gamesMin: 0, gamesUnlimited: false });
    expect(sent[5]).toEqual({ weekday: 6, totalMin: 240, unlimited: false, gamesMin: 150, gamesUnlimited: false });
  });

  // Ловит: у игр «без лимита» в программу уходят скрытые минуты (контракт: тогда gamesMin = 0).
  it("wireLimits обнуляет минуты игр у «без лимита»", () => {
    const hidden = toggleUnlimited(withGamesWeek([120, 120, 120, 120, 120, 240, 240], [45, 45, 45, 45, 45, 90, 90]), 1, true, "games");
    expect(hidden[0].gamesMin).toBe(45);
    expect(wireLimits(hidden)[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 0, gamesUnlimited: true });
  });

  // Ловит: поля игр не доходят до программы, и она читает их как «играть нельзя».
  it("wireLimits у дня без полей игр шлёт «без отдельного лимита»", () => {
    expect(wireLimits([{ weekday: 2, totalMin: 60, unlimited: false }])).toEqual([
      { weekday: 2, totalMin: 60, unlimited: false, gamesMin: 0, gamesUnlimited: true },
    ]);
  });

  it("сериализация туда и обратно не меняет таблицу", () => {
    const table = withGamesWeek([120, 120, 90, 120, 120, 240, 240], [60, null, 0, null, null, 150, null]);
    expect(normalizeLimits(wireLimits(table))).toEqual(table);
  });

  it("игры «без лимита» помнят минуты, снять с 0 - не «нельзя», а 1 ч", () => {
    const base = withGamesWeek([120, 120, 120, 120, 120, 240, 240], [30, null, null, null, null, null, null]);
    const on = toggleUnlimited(base, 1, true, "games");
    expect(on[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: false, gamesMin: 30, gamesUnlimited: true });
    expect(toggleUnlimited(on, 1, false, "games")[0].gamesMin).toBe(30);
    const tue = toggleUnlimited(base, 2, false, "games");
    expect(tue[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false, gamesMin: GAMES_FALLBACK_MIN, gamesUnlimited: false });
    expect(GAMES_FALLBACK_MIN).toBe(60);
  });

  // Ловит: переключатель игр трогает общий лимит или наоборот.
  it("переключатель игр не трогает общий лимит, общий - игры", () => {
    const base = withGamesWeek([120, 120, 120, 120, 120, 240, 240], [30, null, null, null, null, null, null]);
    expect(toggleUnlimited(base, 1, true, "games")[0].unlimited).toBe(false);
    const total = toggleUnlimited(base, 1, true);
    expect(total[0]).toEqual({ weekday: 1, totalMin: 120, unlimited: true, gamesMin: 30, gamesUnlimited: false });
  });

  // Ловит: «Как в понедельник» переносит только общее время.
  it("«Как в понедельник» и «Как в субботу» переносят оба столбца", () => {
    const start = withGamesWeek([45, 120, 90, 120, 120, 240, 60], [20, null, 0, null, null, 90, null]);
    const workdays = copyDay(start, 1, [2, 3, 4, 5]);
    for (const d of [1, 2, 3, 4, 5]) {
      expect(workdays[d - 1]).toEqual({ weekday: d, totalMin: 45, unlimited: false, gamesMin: 20, gamesUnlimited: false });
    }
    expect(workdays[5]).toEqual(start[5]);
    const sunday = copyDay(start, 6, [7]);
    expect(sunday[6]).toEqual({ weekday: 7, totalMin: 240, unlimited: false, gamesMin: 90, gamesUnlimited: false });
  });

  it("limitsEqual видит правку игр, а минуты игр «без лимита» не сравнивает", () => {
    const a = withGamesWeek([120, 120, 120, 120, 120, 240, 240], [60, null, null, null, null, null, null]);
    expect(limitsEqual(a, setDay(a, 1, { gamesMin: 61 }))).toBe(false);
    expect(limitsEqual(a, setDay(a, 1, { gamesUnlimited: true }))).toBe(false);
    expect(limitsEqual(a, setDay(a, 2, { gamesMin: 33 }))).toBe(true);
    expect(limitsEqual(a, a.map((l) => ({ weekday: l.weekday, totalMin: l.totalMin, unlimited: l.unlimited, ...(l.gamesUnlimited ? {} : { gamesMin: l.gamesMin, gamesUnlimited: false }) })))).toBe(true);
  });

  it("игры больше общего лимита допустимы, больше суток - ошибка у игр этого дня", () => {
    expect(validateLimits(withGamesWeek([60, 60, 60, 60, 60, 60, 60], [120, null, null, null, null, null, null]))).toEqual({});
    expect(validateLimits(withGamesWeek([60, 60, 60, 60, 60, 60, 60], [null, MAX_LIMIT_MIN + 1, null, null, null, null, null]))).toEqual({
      "games.2": "Не больше 24 ч",
    });
  });

  it("ключ ошибки программы: limits.N и games.N, прочее - нет", () => {
    expect(limitErrorKey("limits.4")).toBe("limits.4");
    expect(limitErrorKey("games.7")).toBe("games.7");
    expect(limitErrorKey("games.8")).toBeNull();
    expect(limitErrorKey("endMode")).toBeNull();
    expect(limitErrorKey(undefined)).toBeNull();
  });
});
