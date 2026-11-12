import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// NFR-14: контраст текста не меньше 4,5:1 (WCAG 2.x). Формула посчитана здесь заново и не
// зависит от кода приложения; токены читаются из styles.css, поэтому правка палитры без
// проверки не пройдёт.

const css = readFileSync(resolve(__dirname, "../styles.css"), "utf8");

function blockOf(source: string, opening: RegExp): string {
  const m = opening.exec(source);
  if (!m) {
    throw new Error(`в styles.css нет блока ${opening}`);
  }
  let depth = 0;
  for (let i = m.index + m[0].length - 1; i < source.length; i++) {
    if (source[i] === "{") depth++;
    if (source[i] === "}") {
      depth--;
      if (depth === 0) {
        return source.slice(m.index + m[0].length, i);
      }
    }
  }
  throw new Error("блок styles.css не закрыт");
}

function tokens(block: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const m of block.matchAll(/--([a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{6})\s*;/g)) {
    out[m[1]] = m[2].toLowerCase();
  }
  return out;
}

const light = tokens(blockOf(css, /:root\s*\{/));
const darkOverrides = tokens(blockOf(blockOf(css, /@media\s*\(prefers-color-scheme:\s*dark\)\s*\{/), /:root\s*\{/));
const dark = { ...light, ...darkOverrides };

function channel(v: number): number {
  const c = v / 255;
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  return 0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255);
}

function ratio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const TEXT_PAIRS: [string, string][] = [
  ["text", "bg"],
  ["text", "surface"],
  ["text-muted", "bg"],
  ["text-muted", "surface"],
  ["primary-text", "primary"],
  ["status-ok", "surface"],
  ["status-ok", "bg"],
  ["status-warn", "surface"],
  ["status-warn", "bg"],
  ["status-danger", "surface"],
  ["status-danger", "bg"],
];

describe("сама формула", () => {
  // Ловит: опечатка в формуле (тест всегда зелёный). Эталоны WCAG: чёрное на белом 21:1.
  it("чёрный на белом 21:1, одинаковые цвета 1:1", () => {
    expect(ratio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(ratio("#777777", "#777777")).toBeCloseTo(1, 5);
  });
  it("серый #767676 на белом около 4,54:1", () => {
    expect(ratio("#767676", "#ffffff")).toBeCloseTo(4.54, 2);
  });
});

describe("токены палитры (ТЗ 8.0 п.9)", () => {
  it("светлая палитра не изменена", () => {
    expect(light).toMatchObject({
      bg: "#f3f4f6",
      surface: "#ffffff",
      line: "#d8dce0",
      text: "#1b1f23",
      "text-muted": "#59616a",
      primary: "#2b3138",
      "status-warn": "#a15c00",
      "status-danger": "#b3261e",
      "status-ok": "#2c7a4b",
    });
  });

  it.each(TEXT_PAIRS)("светлая тема: %s на %s не меньше 4,5:1", (fg, bg) => {
    expect(ratio(light[fg], light[bg])).toBeGreaterThanOrEqual(4.5);
  });

  it.each(TEXT_PAIRS)("тёмная тема: %s на %s не меньше 4,5:1", (fg, bg) => {
    expect(ratio(dark[fg], dark[bg])).toBeGreaterThanOrEqual(4.5);
  });

  // Рамки полей не текст, но без видимой границы поле не найти (WCAG 1.4.11 требует 3:1).
  it("рамка поля заметна на поверхности в обеих темах (3:1)", () => {
    expect(ratio(light["field-border"], light.surface)).toBeGreaterThanOrEqual(3);
    expect(ratio(dark["field-border"], dark.surface)).toBeGreaterThanOrEqual(3);
  });

  // Ловит (WCAG 1.4.11): столбики «По дням» сливаются с карточкой; токен не задан в одной из тем.
  it("столбики заметны на поверхности в обеих темах (3:1)", () => {
    expect(light["chart-bar"]).toBe("#59616a");
    expect(dark["chart-bar"]).toBe("#a3abb3");
    expect(ratio(light["chart-bar"], light.surface)).toBeGreaterThanOrEqual(3);
    expect(ratio(dark["chart-bar"], dark.surface)).toBeGreaterThanOrEqual(3);
  });
});

/** Токен из правила styles.css: «color: var(--text-muted)» у селектора. */
function ruleVar(selector: string, prop: string): string {
  const esc = selector.replace(/[.*+?^${}()|[\]\\:]/g, "\\$&");
  const block = blockOf(css, new RegExp(`(^|\\n)${esc}\\s*\\{`));
  const m = new RegExp(`(^|[;\\s])${prop}\\s*:\\s*var\\(--([a-z0-9-]+)\\)`).exec(block);
  if (!m) throw new Error(`у ${selector} нет ${prop}: var(--…)`);
  return m[2];
}

// Подсказка в пустом поле («A1234» у кода промокода): в тёмной теме цвет браузера по умолчанию
// почти не виден (визуальная приёмка M5). Одно правило для всех полей: приглушённый текст палитры,
// не меньше 4,5:1 на фоне поля в обеих темах, без прозрачности браузера.
describe("подсказка в пустом поле (placeholder)", () => {
  it("цвет — приглушённый текст, фон поля — поверхность, без прозрачности", () => {
    expect(ruleVar("::placeholder", "color")).toBe("text-muted");
    expect(ruleVar(".field-input", "background")).toBe("surface");
    expect(blockOf(css, /(^|\n)::placeholder\s*\{/)).toMatch(/opacity\s*:\s*1\s*;/);
  });

  it("не меньше 4,5:1 на поле в обеих темах", () => {
    expect(ratio(light["text-muted"], light.surface)).toBeGreaterThanOrEqual(4.5);
    expect(ratio(dark["text-muted"], dark.surface)).toBeGreaterThanOrEqual(4.5);
  });
});

// Кнопка «Запретить» на «Программах» - обычная «.btn», как «Игра»/«Учёба»/«Прочее»: цвет текста
// тот же токен, что у соседних кнопок, а не приглушённый (иначе она выглядит неактивной, приёмка P2 п.1).
describe("кнопка «Запретить» того же вида, что соседние", () => {
  it("цвет текста обычной кнопки - основной текст, не приглушённый", () => {
    expect(ruleVar(".btn", "color")).toBe("text");
  });
});

// Приглушённая кнопка (btn btn-muted: «убавить время», «Отмена» в расписании): второстепенный вид,
// но текст читается не хуже 4,5:1 на её фоне, в покое и при наведении, в обеих темах.
describe("приглушённая второстепенная кнопка", () => {
  const fg = ruleVar(".btn-muted", "color");
  const backs = [ruleVar(".btn", "background"), ruleVar(".btn:hover:not(:disabled)", "background")];

  it("цвет текста и фона заданы токенами палитры", () => {
    expect(fg).toBe("text-muted");
    expect(backs).toEqual(["surface", "bg"]);
  });

  it.each(backs)("светлая тема: текст к фону %s не меньше 4,5:1", (bg) => {
    expect(ratio(light[fg], light[bg])).toBeGreaterThanOrEqual(4.5);
  });

  it.each(backs)("тёмная тема: текст к фону %s не меньше 4,5:1", (bg) => {
    expect(ratio(dark[fg], dark[bg])).toBeGreaterThanOrEqual(4.5);
  });
});

// Флажки и переключатели (ревью окна M5): рамка браузера в тёмной теме ниже 3:1 к карточке. Своё
// оформление: рамка 2 px цветом рамки поля, отметка цветом основной кнопки, видимый фокус (WCAG 1.4.11).
describe("флажки и переключатели своим оформлением", () => {
  const base = blockOf(css, /(^|\n)input\[type="checkbox"\],\s*input\[type="radio"\]\s*\{/);
  const checkedBox = blockOf(css, /(^|\n)input\[type="checkbox"\]:checked,\s*input\[type="checkbox"\]:indeterminate\s*\{/);
  const checkedRadio = blockOf(css, /(^|\n)input\[type="radio"\]:checked\s*\{/);
  const radioDot = blockOf(css, /(^|\n)input\[type="radio"\]:checked::before\s*\{/);
  const focus = blockOf(css, /(^|\n)input\[type="checkbox"\]:focus-visible,\s*input\[type="radio"\]:focus-visible\s*\{/);

  it("без оформления браузера: рамка 2 px цветом рамки поля на поверхности", () => {
    expect(base).toMatch(/(^|[;\s])appearance\s*:\s*none\s*;/);
    expect(base).toMatch(/border\s*:\s*2px\s+solid\s+var\(--field-border\)\s*;/);
    expect(base).toMatch(/background\s*:\s*var\(--surface\)\s*;/);
  });

  it.each(["surface", "bg"])("рамка пустого флажка к фону %s не меньше 3:1 в обеих темах", (bg) => {
    expect(ratio(light["field-border"], light[bg])).toBeGreaterThanOrEqual(3);
    expect(ratio(dark["field-border"], dark[bg])).toBeGreaterThanOrEqual(3);
  });

  it("отметка - цвет основной кнопки, не меньше 3:1 к поверхности и фону в обеих темах", () => {
    expect(checkedBox).toMatch(/background\s*:\s*var\(--primary\)\s*;/);
    expect(checkedBox).toMatch(/border-color\s*:\s*var\(--primary\)\s*;/);
    expect(checkedRadio).toMatch(/border-color\s*:\s*var\(--primary\)\s*;/);
    expect(radioDot).toMatch(/background\s*:\s*var\(--primary\)\s*;/);
    for (const bg of ["surface", "bg"]) {
      expect(ratio(light.primary, light[bg])).toBeGreaterThanOrEqual(3);
      expect(ratio(dark.primary, dark[bg])).toBeGreaterThanOrEqual(3);
    }
    // Галочка на залитом флажке - цвет текста основной кнопки.
    const tick = blockOf(css, /(^|\n)input\[type="checkbox"\]:checked::before\s*\{/);
    expect(tick).toMatch(/var\(--primary-text\)/);
  });

  it("фокус с клавиатуры виден: кольцо 2 px основным текстом", () => {
    expect(focus).toMatch(/outline\s*:\s*2px\s+solid\s+var\(--text\)\s*;/);
  });
});
