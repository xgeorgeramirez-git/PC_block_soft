import { describe, expect, it } from "vitest";
import {
  formatClock,
  formatDateLong,
  formatDuration,
  formatEventTime,
  formatHour,
  formatLimit,
  genitiveName,
  localOffsetMin,
  formatUsed,
  weekdayName,
  weekdayOfMs,
  weekdayShort,
} from "./format";

// Зеркало правил ядра (internal/core/texts.go): те же таблицы, что в Go-тестах.

// Ловит: длительность не по ТЗ 4: ведущего нуля у минут нет, округление вверх или минус.
describe("formatDuration", () => {
  const cases: [number, string][] = [
    [-5, "0 мин"],
    [0, "0 мин"],
    [59, "0 мин"],
    [60, "1 мин"],
    [2400, "40 мин"],
    [3599, "59 мин"],
    [3600, "1 ч 00 мин"],
    [3900, "1 ч 05 мин"],
    [7200, "2 ч 00 мин"],
    [9000, "2 ч 30 мин"],
  ];
  it.each(cases)("%i с -> %s", (seconds, want) => {
    expect(formatDuration(seconds)).toBe(want);
  });
});

// Ловит: лимит пишется «2 ч 00 мин» вместо «2 ч» или «1 ч 30» без «мин».
describe("formatLimit", () => {
  const cases: [number, string][] = [
    [-1, "0 мин"],
    [0, "0 мин"],
    [20, "20 мин"],
    [60, "1 ч"],
    [65, "1 ч 05 мин"],
    [90, "1 ч 30 мин"],
    [120, "2 ч"],
    [150, "2 ч 30 мин"],
    [1440, "24 ч"],
  ];
  it.each(cases)("%i мин -> %s", (minutes, want) => {
    expect(formatLimit(minutes)).toBe(want);
  });
});

// Ловит: строка «Использовано … из …» без лимита или с «из 0 мин» вместо «без лимита».
describe("formatUsed", () => {
  it("с лимитом", () => {
    expect(formatUsed(2100, 7200, false)).toBe("Использовано 35 мин из 2 ч");
    expect(formatUsed(7200, 9000, false)).toBe("Использовано 2 ч 00 мин из 2 ч 30 мин");
  });
  it("без лимита", () => {
    expect(formatUsed(2400, 0, true)).toBe("Использовано 40 мин, без лимита");
  });
  it("лимит 0 минут (нельзя) не превращается в «без лимита»", () => {
    expect(formatUsed(0, 0, false)).toBe("Использовано 0 мин из 0 мин");
  });
});

// Ловит: отсчёт паузы показывает секунды без минут («60») или не добивает нулём («1:5»).
describe("formatClock", () => {
  const cases: [number, string][] = [
    [60, "1:00"],
    [59, "0:59"],
    [65, "1:05"],
    [0, "0:00"],
    [-3, "0:00"],
    [900, "15:00"],
  ];
  it.each(cases)("%i с -> %s", (seconds, want) => {
    expect(formatClock(seconds)).toBe(want);
  });
});

describe("дни недели", () => {
  it("короткие и полные названия идут с понедельника", () => {
    expect(weekdayShort(1)).toBe("пн");
    expect(weekdayShort(7)).toBe("вс");
    expect(weekdayName(1)).toBe("Понедельник");
    expect(weekdayName(6)).toBe("Суббота");
  });
  it("день вне 1..7 не ломает окно", () => {
    expect(weekdayShort(0)).toBe("");
    expect(weekdayName(9)).toBe("");
  });
  // 19 октября 2026 года - понедельник.
  it("номер дня недели по моменту и поясу", () => {
    expect(weekdayOfMs(Date.UTC(2026, 9, 19, 12, 0), 0)).toBe(1);
    expect(weekdayOfMs(Date.UTC(2026, 9, 25, 12, 0), 0)).toBe(7);
  });
  it("у границы суток пояс сдвигает день", () => {
    const sundayLate = Date.UTC(2026, 9, 25, 22, 30);
    expect(weekdayOfMs(sundayLate, 0)).toBe(7);
    expect(weekdayOfMs(sundayLate, 180)).toBe(1);
  });
});

describe("даты и время", () => {
  // Ловит: месяц в именительном падеже («19 октябрь») или день недели с заглавной.
  it("«пн, 19 октября»", () => {
    expect(formatDateLong(Date.UTC(2026, 9, 19, 9, 0), 0)).toBe("пн, 19 октября");
    expect(formatDateLong(Date.UTC(2026, 0, 1, 9, 0), 0)).toBe("чт, 1 января");
    expect(formatDateLong(Date.UTC(2026, 4, 9, 9, 0), 0)).toBe("сб, 9 мая");
  });
  it("«21:00»", () => {
    expect(formatHour(21 * 60)).toBe("21:00");
    expect(formatHour(7 * 60 + 5)).toBe("07:05");
    expect(formatHour(0)).toBe("00:00");
  });
  // Ловит: время журнала в UTC вместо часового пояса события.
  it("время события со сдвигом пояса: «19 окт, 21:14»", () => {
    const at = Date.UTC(2026, 9, 19, 18, 14);
    expect(formatEventTime(at, 180)).toBe("19 окт, 21:14");
    expect(formatEventTime(at, 0)).toBe("19 окт, 18:14");
    expect(formatEventTime(at, -300)).toBe("19 окт, 13:14");
  });
  it("полночь после сдвига пояса переносит дату", () => {
    const at = Date.UTC(2026, 9, 19, 21, 5);
    expect(formatEventTime(at, 180)).toBe("20 окт, 00:05");
  });
});

// Ловит: «Ограничения для Миша перестанут действовать» вместо «для Миши» (текст из ТЗ 8.3.8).
describe("genitiveName", () => {
  const cases: [string, string][] = [
    ["Миша", "Миши"],
    ["Даша", "Даши"],
    ["Маша", "Маши"],
    ["Анна", "Анны"],
    ["Ольга", "Ольги"],
    ["Ника", "Ники"],
    ["Катя", "Кати"],
    ["Ваня", "Вани"],
    ["Мария", "Марии"],
    ["Иван", "Ивана"],
    ["Артём", "Артёма"],
    ["Андрей", "Андрея"],
    ["Игорь", "Игоря"],
    ["Дмитрий", "Дмитрия"],
    ["Тимофей", "Тимофея"],
    ["Женя", "Жени"],
    ["Саша", "Саши"],
    ["Tom", "Tom"],
    ["Лиза Петрова", "Лиза Петрова"],
    ["", ""],
  ];
  it.each(cases)("%s -> %s", (name, want) => {
    expect(genitiveName(name)).toBe(want);
  });
});

describe("пояс окна", () => {
  it("localOffsetMin: к востоку от UTC положительный, ноль без «минус нуля»", () => {
    expect(Object.is(localOffsetMin(new Date(2026, 9, 19)), -0)).toBe(false);
    const d = new Date(2026, 9, 19, 12);
    expect(localOffsetMin(d)).toBe(-d.getTimezoneOffset() || 0);
  });
});
