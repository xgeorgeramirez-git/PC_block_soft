import { describe, expect, it } from "vitest";
import { formatHM, keepUnits, limitLine, pendingLead, programsWaitText, remainMinutes, remainShare, remainTone } from "./remain";

// Цвет остатка у родителя (ТЗ 8.0 п. 3, D-109): больше 1 ч зелёный, больше 15 мин до 1 ч включительно
// синий, 15 мин и меньше красный. Минуты — вверх до целой (как core.RemainingMinutes и значок у часов),
// поэтому граница 15/16 и 60/61 мин лежит между 900 и 901 с, 3600 и 3601 с (Review Focus п. 6).

describe("remainMinutes: вверх до целой минуты", () => {
  it.each([
    [0, 0],
    [1, 1],
    [59, 1],
    [60, 1],
    [61, 2],
    [900, 15],
    [901, 16],
    [3600, 60],
    [3601, 61],
  ])("%i с → %i мин", (s, m) => {
    expect(remainMinutes(s)).toBe(m);
  });

  it("отрицательный остаток считается нулём", () => {
    expect(remainMinutes(-30)).toBe(0);
  });
});

describe("remainTone: пороги 15/16 и 60/61 мин", () => {
  it.each([
    [0, "red"],
    [1, "red"],
    [900, "red"],
    [901, "blue"],
    [960, "blue"],
    [3600, "blue"],
    [3601, "green"],
    [3660, "green"],
    [7200, "green"],
  ] as const)("%i с → %s", (s, tone) => {
    expect(remainTone(s, false)).toBe(tone);
  });

  it("без лимита — зелёный при любом остатке", () => {
    expect(remainTone(0, true)).toBe("green");
    expect(remainTone(600, true)).toBe("green");
  });
});

describe("formatHM", () => {
  it.each([
    [85, "1:25"],
    [5, "0:05"],
    [0, "0:00"],
    [60, "1:00"],
    [600, "10:00"],
  ])("%i мин → %s", (m, text) => {
    expect(formatHM(m)).toBe(text);
  });
});

describe("remainShare: доля полосы", () => {
  it("остаток к лимиту дня", () => {
    expect(remainShare(3600, 7200, false)).toBe(0.5);
    expect(remainShare(0, 7200, false)).toBe(0);
  });

  it("без лимита и при нулевом лимите — полная полоса без деления на ноль", () => {
    expect(remainShare(0, 0, true)).toBe(1);
    expect(remainShare(0, 0, false)).toBe(0);
  });

  it("не выходит за 0…1", () => {
    expect(remainShare(9000, 7200, false)).toBe(1);
    expect(remainShare(-5, 7200, false)).toBe(0);
  });
});

describe("limitLine: «Игры 40 из 60 мин»", () => {
  it("потрачено из лимита — секунды, потраченные сегодня (как gamesSec отчёта телефону)", () => {
    expect(limitLine("Игры", 2400, 3600, false)).toBe("Игры 40 из 60 мин");
    expect(limitLine("Видео и соцсети", 600, 1800, false)).toBe("Видео и соцсети 10 из 30 мин");
  });

  it("ничего не потрачено и всё потрачено", () => {
    expect(limitLine("Игры", 0, 3600, false)).toBe("Игры 0 из 60 мин");
    expect(limitLine("Игры", 3600, 3600, false)).toBe("Игры 60 из 60 мин");
  });

  // Минуты потраченного — целые вниз, как у телефона (used / 60): 59 мин 30 с — не «60 из 60».
  it("неполная минута не засчитывается целой", () => {
    expect(limitLine("Игры", 3570, 3600, false)).toBe("Игры 59 из 60 мин");
  });

  // Ловит (ревью 0.8.4 M2, класс A): родитель уменьшил лимит ниже сыгранного — окно писало
  // «Игры 60 из 60 мин» (лимит − остаток, остаток не бывает < 0), телефон — «Игры 70 из 60 мин».
  it("потрачено больше лимита — как есть, не обрезано лимитом", () => {
    expect(limitLine("Игры", 4200, 3600, false)).toBe("Игры 70 из 60 мин");
    expect(limitLine("Видео и соцсети", 2700, 1800, false)).toBe("Видео и соцсети 45 из 30 мин");
  });

  it("без отдельного лимита или с нулевым лимитом без данных — строки нет", () => {
    expect(limitLine("Игры", 0, 0, true)).toBe("");
    expect(limitLine("Игры", 0, 0, false)).toBe("");
  });
});

describe("programsWaitText: слово по числу", () => {
  it.each([
    [1, "1 программа ждёт решения"],
    [2, "2 программы ждут решения"],
    [4, "4 программы ждут решения"],
    [5, "5 программ ждут решения"],
    [11, "11 программ ждут решения"],
    [12, "12 программ ждут решения"],
    [21, "21 программа ждёт решения"],
    [22, "22 программы ждут решения"],
    [111, "111 программ ждут решения"],
  ])("%i → %s", (n, text) => {
    expect(programsWaitText(n)).toBe(text);
  });
});

describe("pendingLead: пояснение блока «ждут решения»", () => {
  const tail = "Пока вы не решите, ребёнок не сможет";
  it("одна программа — «её»", () => {
    expect(pendingLead(["SuperGame"], 1)).toBe(`SuperGame. ${tail} её запустить.`);
  });
  it("две — через «и», «их»", () => {
    expect(pendingLead(["Roblox Studio", "Steam"], 2)).toBe(`Roblox Studio и Steam. ${tail} их запустить.`);
  });
  it("три названия и ещё две сверх них", () => {
    expect(pendingLead(["A", "B", "C"], 5)).toBe(`A, B, C и ещё 2. ${tail} их запустить.`);
  });
  it("без названий (старая служба) — только пояснение", () => {
    expect(pendingLead([], 2)).toBe(`${tail} их запустить.`);
  });
});

describe("keepUnits: число и единица не разрываются", () => {
  // Ловит (визуальная приёмка T5): «Осталось 10 / мин» переносилось между числом и «мин».
  it.each([
    ["Осталось 10 мин", "Осталось 10\u00a0мин"],
    ["Осталось 1 ч 05 мин", "Осталось 1\u00a0ч 05\u00a0мин"],
    ["Пауза до 18:30", "Пауза до 18:30"],
    ["Можно", "Можно"],
  ])("%s", (text, want) => {
    expect(keepUnits(text)).toBe(want);
  });
});
