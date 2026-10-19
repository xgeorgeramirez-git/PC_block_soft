import { describe, expect, it } from "vitest";
import {
  MAX_LIMIT_MIN,
  copyDay,
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
  mins.map((totalMin, i) => ({ weekday: i + 1, totalMin, unlimited: unlimitedDays.includes(i + 1) }));

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
    expect(got[0]).toEqual({ weekday: 1, totalMin: 60, unlimited: false });
    expect(got[2]).toEqual({ weekday: 3, totalMin: 90, unlimited: false });
  });
  // Ловит: пустой ответ (null вместо массива) роняет таблицу.
  it("пусто и null дают шаблон", () => {
    expect(normalizeLimits([])).toEqual(templateLimits());
    expect(normalizeLimits(null)).toEqual(templateLimits());
    expect(normalizeLimits(undefined)).toEqual(templateLimits());
  });
  it("у «без лимита» минуты сбрасываются в 0", () => {
    const got = normalizeLimits([{ weekday: 7, totalMin: 55, unlimited: true }]);
    expect(got[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true });
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
    expect(on[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: true });
    const off = toggleUnlimited(on, 2, false);
    expect(off[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false });
  });

  it("снять «без лимита» с дня без минут - не 0, а значение шаблона", () => {
    const sunday = toggleUnlimited(base, 7, false);
    expect(sunday[6]).toEqual({ weekday: 7, totalMin: 240, unlimited: false });
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
    expect(sent[0]).toEqual({ weekday: 1, totalMin: 0, unlimited: true });
    expect(sent[1]).toEqual({ weekday: 2, totalMin: 120, unlimited: false });
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
    expect(next[6]).toEqual({ weekday: 7, totalMin: 0, unlimited: true });
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
    expect(got).toEqual({ 3: "Не больше 24 ч" });
  });
  it("«без лимита» не проверяется по минутам", () => {
    expect(validateLimits(week([120, 120, 5000, 120, 120, 240, 0], [3]))).toEqual({});
  });
});
