import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { axisLabel, barHeights, barScale, barsSummary, showLabel, tipAlign } from "./daybars";
import { parentStats } from "../api";
import { G15_NOW, g15Child, installBackend, uninstallBackend } from "../__mocks__/backend";
import type { DayBar } from "../types";

// «По дням» (8.2.6 п.4, D-69, D-78, S21): шкала и размеры столбиков на эталонной неделе G15
// (testdata/golden/G15.json, вс 22.11.2026). Ожидания - литералы эталона.

const bar = (key: string, totalMin: number, studyMin: number, today = false): DayBar => ({
  key,
  label: key,
  title: key,
  totalS: totalMin * 60,
  studyS: studyMin * 60,
  today,
});

/** Неделя G15: пн 95 + 30 учёбы, вт 140, ср 0, чт 128 + 45, пт 180 + 20, сб 200, вс 10 (сегодня). */
const WEEK: DayBar[] = [
  bar("2026-11-16", 95, 30),
  bar("2026-11-17", 140, 0),
  bar("2026-11-18", 0, 0),
  bar("2026-11-19", 128, 45),
  bar("2026-11-20", 180, 20),
  bar("2026-11-21", 200, 0),
  bar("2026-11-22", 10, 0, true),
];

describe("barScale: потолок оси и деления", () => {
  // Ловит: потолок по одному сегменту (без учёбы) - столбик пт 180 + 20 выше оси.
  it("G15: наибольший столбик 200 мин - шаг 1 ч, ось до 4 ч", () => {
    expect(barScale(WEEK)).toEqual({ maxS: 14400, ticks: [0, 3600, 7200, 10800, 14400] });
  });

  it("все нули: ось до 1 ч с делением 30 мин", () => {
    expect(barScale([bar("a", 0, 0), bar("b", 0, 0)])).toEqual({ maxS: 3600, ticks: [0, 1800, 3600] });
    expect(barScale([])).toEqual({ maxS: 3600, ticks: [0, 1800, 3600] });
  });

  it("35 мин: шаг 10 мин, ось до 40 мин", () => {
    expect(barScale([bar("a", 35, 0)])).toEqual({ maxS: 2400, ticks: [0, 600, 1200, 1800, 2400] });
  });

  it("месяц 12 ч 33 мин + 1 ч 35 мин учёбы: шаг 4 ч, ось до 16 ч", () => {
    expect(barScale([bar("2026-11", 753, 95)])).toEqual({ maxS: 57600, ticks: [0, 14400, 28800, 43200, 57600] });
  });
});

describe("axisLabel: подписи делений", () => {
  it("0, 30 мин, 2 ч, 1 ч 30 мин", () => {
    expect([0, 1800, 7200, 5400].map(axisLabel)).toEqual(["0", "30 мин", "2 ч", "1 ч 30 мин"]);
  });
});

describe("barHeights: высоты сегментов", () => {
  it("пн G15: 95 мин - 39,6 %, учёба 30 мин - 12,5 % при оси 4 ч", () => {
    expect(barHeights(WEEK[0], 14400)).toEqual({ totalPct: 39.6, studyPct: 12.5, empty: false });
  });

  // Ловит: пустой день рисуется столбиком нулевой высоты без отметки (его не отличить от «нет дня»).
  it("ср G15: пустой столбик", () => {
    expect(barHeights(WEEK[2], 14400)).toEqual({ totalPct: 0, studyPct: 0, empty: true });
  });

  // Ловит: 30 секунд за день не видны совсем (0 %), хотя время было.
  it("30 с при оси 4 ч - не меньше 1 %", () => {
    expect(barHeights({ ...WEEK[0], totalS: 30, studyS: 0 }, 14400)).toEqual({ totalPct: 1, studyPct: 0, empty: false });
  });

  it("только учёба: столбик не пустой", () => {
    expect(barHeights({ ...WEEK[0], totalS: 0, studyS: 3600 }, 14400)).toEqual({ totalPct: 0, studyPct: 25, empty: false });
  });
});

describe("showLabel: какие подписи под столбиками", () => {
  it("7 дней - все", () => {
    expect([0, 1, 2, 3, 4, 5, 6].map((i) => showLabel(i, 7, "day"))).toEqual([true, true, true, true, true, true, true]);
  });

  it("31 день - 1, 8, 15, 22 и 29 число", () => {
    const shown = Array.from({ length: 31 }, (_, i) => i).filter((i) => showLabel(i, 31, "day"));
    expect(shown).toEqual([0, 7, 14, 21, 28]);
  });

  it("36 месяцев - каждый третий", () => {
    const shown = Array.from({ length: 36 }, (_, i) => i).filter((i) => showLabel(i, 36, "month"));
    expect(shown).toEqual([0, 3, 6, 9, 12, 15, 18, 21, 24, 27, 30, 33]);
  });

  // Ловит: «ноя 2025» шире столбика, и у 12 месяцев с годом подписи налезают друг на друга.
  it("месяцы с годом: с 7 столбиков каждый третий, до 6 - все", () => {
    expect(Array.from({ length: 12 }, (_, i) => i).filter((i) => showLabel(i, 12, "month", true))).toEqual([0, 3, 6, 9]);
    expect(Array.from({ length: 6 }, (_, i) => showLabel(i, 6, "month", true)).every(Boolean)).toBe(true);
  });

  it("12 месяцев года - все", () => {
    expect(Array.from({ length: 12 }, (_, i) => showLabel(i, 12, "month")).every(Boolean)).toBe(true);
  });
});

describe("tipAlign: подсказка не выходит за край графика", () => {
  it("один столбик - по центру", () => {
    expect(tipAlign(0, 1)).toBe("center");
  });

  it("7 столбиков: левая треть от столбика вправо, правая - влево, середина - по центру", () => {
    expect([0, 1, 2, 3, 4, 5, 6].map((i) => tipAlign(i, 7))).toEqual(["start", "start", "center", "center", "center", "end", "end"]);
  });

  it("31 столбик: первый и последний у своих краёв", () => {
    expect([tipAlign(0, 31), tipAlign(15, 31), tipAlign(30, 31)]).toEqual(["start", "center", "end"]);
  });
});

describe("barsSummary и столбики подменной программы на неделе G15", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true, now: G15_NOW });
    installBackend({ sessionActive: true, children: [g15Child()] });
  });
  afterEach(() => {
    vi.useRealTimers();
    uninstallBackend();
  });

  // Ловит (S21, RM5-3): подпись и подсказки столбиков не по эталону; сумма столбиков не равна «Итогам».
  it("неделя: 7 столбиков с подписями и подсказками эталона, итог - как в «Итогах»", async () => {
    const r = await parentStats(1, "week");
    expect(r.barsBy).toBe("day");
    expect(r.bars.map((b) => [b.label, b.title, b.totalS / 60, b.studyS / 60, b.today])).toEqual([
      ["пн 16", "пн, 16 ноября: 1 ч 35 мин, учёба 30 мин", 95, 30, false],
      ["вт 17", "вт, 17 ноября: 2 ч 20 мин, учёба 0 мин", 140, 0, false],
      ["ср 18", "ср, 18 ноября: 0 мин, учёба 0 мин", 0, 0, false],
      ["чт 19", "чт, 19 ноября: 2 ч 08 мин, учёба 45 мин", 128, 45, false],
      ["пт 20", "пт, 20 ноября: 3 ч 00 мин, учёба 20 мин", 180, 20, false],
      ["сб 21", "сб, 21 ноября: 3 ч 20 мин, учёба 0 мин", 200, 0, false],
      ["вс 22", "вс, 22 ноября: 10 мин, учёба 0 мин", 10, 0, true],
    ]);
    expect(r.bars.reduce((n, b) => n + b.totalS, 0)).toBe(r.usedS);
    expect(barsSummary(r)).toBe("По дням за неделю: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
  });

  it("год: 11 месяцев, ноябрь 12 ч 33 мин, итог по месяцам", async () => {
    const r = await parentStats(1, "year");
    expect(r.barsBy).toBe("month");
    expect(r.bars).toHaveLength(11);
    expect(r.bars.at(-1)).toMatchObject({ key: "2026-11", label: "ноя", title: "ноябрь 2026: 12 ч 33 мин, учёба 1 ч 35 мин", today: true });
    expect(r.bars[0]).toMatchObject({ key: "2026-01", label: "янв", totalS: 0, studyS: 0 });
    expect(barsSummary(r)).toBe("По месяцам за год: 12 ч 33 мин в счёт лимита, учёба 1 ч 35 мин");
  });
});
