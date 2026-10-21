import { describe, expect, it } from "vitest";
import type { Interval } from "../types";
import {
  addInterval,
  copyDayIntervals,
  dayStripSegments,
  intervalError,
  intervalsOfDay,
  intervalText,
  parseClock,
  prevDayName,
  removeInterval,
  schedulesEqual,
} from "./schedule";

describe("parseClock", () => {
  it("разбирает корректное время", () => {
    expect(parseClock("21:00")).toBe(1260);
    expect(parseClock("9:5")).toBe(545);
    expect(parseClock("00:00")).toBe(0);
    expect(parseClock("23:59")).toBe(1439);
  });
  it("отвергает мусор и выход за границы", () => {
    expect(parseClock("24:00")).toBeNull();
    expect(parseClock("21:60")).toBeNull();
    expect(parseClock("abc")).toBeNull();
    expect(parseClock("2100")).toBeNull();
    expect(parseClock("")).toBeNull();
  });
});

describe("intervalError", () => {
  it("начало равно концу — ошибка", () => {
    expect(intervalError(600, 600)).toBe("Начало и конец совпадают");
  });
  it("разные — нет ошибки (в том числе через полночь)", () => {
    expect(intervalError(1260, 420)).toBe("");
    expect(intervalError(480, 600)).toBe("");
  });
});

describe("intervalText", () => {
  it("человекочитаемо", () => {
    expect(intervalText({ weekday: 1, startMin: 1260, endMin: 420 })).toBe("с 21:00 до 07:00");
  });
});

describe("add/remove/intervalsOfDay", () => {
  it("добавляет и удаляет интервал дня", () => {
    let all: Interval[] = [];
    all = addInterval(all, 1, 1260, 420);
    all = addInterval(all, 1, 480, 540);
    const day = intervalsOfDay(all, 1);
    expect(day).toHaveLength(2);
    expect(day[0].startMin).toBe(480); // отсортировано по началу
    all = removeInterval(all, 1, 0);
    expect(intervalsOfDay(all, 1)).toHaveLength(1);
    expect(intervalsOfDay(all, 1)[0].startMin).toBe(1260);
  });
});

describe("copyDayIntervals", () => {
  it("копирует интервалы дня на другие дни", () => {
    let all: Interval[] = addInterval([], 1, 1260, 420);
    all = copyDayIntervals(all, 1, [2, 3]);
    expect(intervalsOfDay(all, 2)).toHaveLength(1);
    expect(intervalsOfDay(all, 3)[0].startMin).toBe(1260);
    expect(intervalsOfDay(all, 1)).toHaveLength(1);
  });
});

describe("schedulesEqual", () => {
  it("порядок не важен", () => {
    const a: Interval[] = [
      { weekday: 1, startMin: 1260, endMin: 420 },
      { weekday: 2, startMin: 60, endMin: 120 },
    ];
    const b: Interval[] = [
      { weekday: 2, startMin: 60, endMin: 120 },
      { weekday: 1, startMin: 1260, endMin: 420 },
    ];
    expect(schedulesEqual(a, b)).toBe(true);
  });
  it("разница видна", () => {
    expect(schedulesEqual([{ weekday: 1, startMin: 1260, endMin: 420 }], [])).toBe(false);
  });
});

describe("dayStripSegments", () => {
  it("интервал через полночь обрезается до суток и даёт хвост на следующий день", () => {
    const all: Interval[] = [{ weekday: 5, startMin: 1260, endMin: 420 }]; // пт 21:00-07:00
    const fri = dayStripSegments(all, 5);
    expect(fri.some((s) => !s.carry && s.from === 1260 / 1440 && s.to === 1)).toBe(true);
    // Суббота получает перенос с пятницы 00:00-07:00 с carry.
    const sat = dayStripSegments(all, 6);
    expect(sat.some((s) => s.carry && s.from === 0 && s.to === 420 / 1440)).toBe(true);
  });
});

describe("prevDayName", () => {
  it("правильный предыдущий день в родительном падеже", () => {
    expect(prevDayName(6)).toBe("пятницы"); // перед субботой
    expect(prevDayName(1)).toBe("воскресенья"); // перед понедельником
  });
});
