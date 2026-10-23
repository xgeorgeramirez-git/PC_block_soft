import { describe, expect, it } from "vitest";
import { clampDigits } from "./numinput";
import { maskClock } from "./schedule";

// FB-1 (D-45): поля часов и минут принимают только цифры и сами держат пределы.
describe("clampDigits", () => {
  // Ловит: буквы проходят в поле или превращаются в NaN.
  it("буквы отбрасываются, пустое поле — 0", () => {
    expect(clampDigits("абв", 59)).toBe(0);
    expect(clampDigits("", 23)).toBe(0);
    expect(clampDigits("1а5", 59)).toBe(15);
  });
  // Ловит: «99 минут» сохраняется как есть.
  it("больше предела — зажимается к пределу", () => {
    expect(clampDigits("99", 59)).toBe(59);
    expect(clampDigits("25", 23)).toBe(23);
    expect(clampDigits("234", 23)).toBe(23);
  });
  it("ведущий ноль при наборе после 0 не мешает", () => {
    expect(clampDigits("05", 59)).toBe(5);
    expect(clampDigits("-3", 59)).toBe(3);
  });
});

describe("maskClock", () => {
  // Ловит: в поле времени можно набрать буквы или «25:99».
  it("пропускает только цифры и ставит двоеточие", () => {
    expect(maskClock("абв")).toBe("");
    expect(maskClock("2")).toBe("2");
    expect(maskClock("21")).toBe("21");
    expect(maskClock("21:")).toBe("21");
    expect(maskClock("210")).toBe("21:0");
    expect(maskClock("21:00")).toBe("21:00");
    expect(maskClock("2100")).toBe("21:00");
  });
  it("держит пределы часов и минут", () => {
    expect(maskClock("25")).toBe("23");
    expect(maskClock("2399")).toBe("23:59");
    expect(maskClock("21:7")).toBe("21:5");
    expect(maskClock("7")).toBe("07");
    expect(maskClock("730")).toBe("07:30");
    expect(maskClock("21:000")).toBe("21:00");
  });
});
