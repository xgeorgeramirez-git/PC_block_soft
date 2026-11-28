import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Палитра окна родителя (ТЗ 8.0 п. 9, D-109): токены читаются из styles.css разбором текста, контраст
// считается здесь по WCAG 2.x. Правка цвета без проверки контраста не пройдёт.

const css = readFileSync(resolve(__dirname, "styles.css"), "utf8");

function rootTokens(): Record<string, string> {
  const m = /(^|\n):root\s*\{([^}]*)\}/.exec(css);
  if (!m) throw new Error("в styles.css нет :root");
  const out: Record<string, string> = {};
  for (const t of m[2].matchAll(/--([a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g)) out[t[1]] = t[2].toLowerCase();
  return out;
}

function channel(v: number): number {
  const c = v / 255;
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  return 0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255);
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const t = rootTokens();
const WHITE = "#ffffff";

describe("формула контраста", () => {
  it("чёрный на белом 21:1, #767676 на белом около 4,54:1", () => {
    expect(contrast("#000000", WHITE)).toBeCloseTo(21, 5);
    expect(contrast("#767676", WHITE)).toBeCloseTo(4.54, 2);
  });
});

describe("токены окна родителя — ТЗ 8.0 п. 9", () => {
  it("значения палитры как в ТЗ", () => {
    expect(t).toMatchObject({
      bg: "#f3f5f9",
      surface: "#ffffff",
      line: "#dde3ee",
      text: "#1a2238",
      "text-muted": "#5b6680",
      primary: "#2552d9",
      "primary-text": "#ffffff",
      "accent-soft": "#e6edff",
      "remain-green": "#138a4f",
      "remain-green-bar": "#1fa463",
      "remain-blue": "#2552d9",
      "remain-red": "#c81e1e",
      "remain-red-bar": "#e5484d",
      "plus-text": "#0f7a45",
      "plus-bg": "#e7f5ec",
      "minus-text": "#c2410c",
      "minus-bg": "#fcede6",
      coin: "#ffc53a",
      "gold-text": "#5a3b00",
      "gold-bg": "#ffe7ad",
      "pending-bg": "#fff6e0",
      "pending-line": "#f3d99a",
      "pending-text": "#6b5320",
    });
  });
});

describe("контраст текста не меньше 4,5:1", () => {
  it.each([
    ["remain-blue", "surface"],
    ["remain-red", "surface"],
    ["plus-text", "plus-bg"],
    ["plus-text", "surface"],
    ["minus-text", "minus-bg"],
    ["text-muted", "bg"],
    ["text-muted", "surface"],
    ["text", "bg"],
    ["pending-text", "pending-bg"],
    ["text", "pending-bg"],
    ["primary-text", "primary"],
    ["primary", "accent-soft"],
    ["gold-text", "gold-bg"],
    ["status-ok", "surface"],
    ["status-warn", "surface"],
    ["status-danger", "surface"],
  ])("%s на %s", (fg, bg) => {
    expect(contrast(t[fg], t[bg])).toBeGreaterThanOrEqual(4.5);
  });

  // Крупное число остатка (44 px, 800) — крупный текст WCAG: достаточно 3:1 (ТЗ 8.0 п. 9).
  it("зелёное крупное число остатка на белом не меньше 3:1", () => {
    expect(contrast(t["remain-green"], t.surface)).toBeGreaterThanOrEqual(3);
  });
});

// Ловит (визуальная приёмка T5): фокус поля — толстая двойная тёмная рамка (обводка браузера цветом текста
// поверх рамки поля). Кольцо фокуса полей — акцент #2552D9, 2 px, вплотную к рамке; не меньше 3:1 к фону.
describe("фокус полей ввода", () => {
  const block = /(^|\n)\.field-input:focus-visible[^{]*\{([^}]*)\}/.exec(css)?.[2] ?? "";
  it("кольцо акцента 2 px без отступа, рамка поля того же цвета", () => {
    expect(block).toMatch(/outline:\s*2px solid var\(--primary\)\s*;/);
    expect(block).toMatch(/outline-offset:\s*0\s*;/);
    expect(block).toMatch(/border-color:\s*var\(--primary\)\s*;/);
  });
  it.each(["surface", "bg"])("акцент к фону %s не меньше 3:1", (bg) => {
    expect(contrast(t.primary, t[bg])).toBeGreaterThanOrEqual(3);
  });
});
