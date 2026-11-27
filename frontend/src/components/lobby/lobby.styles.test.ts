import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Стили окна ребёнка «Игровое лобби» (ТЗ 8.0 п. 1, 9): своя область, окно родителя их не видит;
// крупный текст с обводкой; контраст текста на панелях не ниже 4,5:1. jsdom раскладку не считает —
// проверка по тексту styles.css, кадры — e2e.
const css = readFileSync(resolve(__dirname, "../../styles.css"), "utf8");
const MARK = "/* --- Окно ребёнка «Игровое лобби»";
const lobby = css.slice(css.indexOf(MARK));

/** Селекторы всех правил раздела (без @media и тел правил). */
function selectors(text: string): string[] {
  const body = text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/@media[^{]*\{/g, "");
  const out: string[] = [];
  for (const m of body.matchAll(/([^{}]+)\{[^{}]*\}/g)) {
    for (const sel of m[1].split(",")) if (sel.trim()) out.push(sel.trim());
  }
  return out;
}

function rule(selector: string): string {
  const at = lobby.indexOf(`\n${selector} {`);
  if (at < 0) throw new Error(`в разделе «Игровое лобби» нет правила ${selector}`);
  return lobby.slice(at, lobby.indexOf("}", at));
}

function lum(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}

function ratio(a: string, b: string): number {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

describe("«Игровое лобби»: область стилей", () => {
  it("раздел есть в styles.css", () => {
    expect(css.indexOf(MARK)).toBeGreaterThan(0);
  });

  // Ловит: правило раздела без области (.btn, .panel, :root) перекрашивает окно родителя (его e2e и вид T5).
  it("каждое правило раздела — только внутри окна ребёнка", () => {
    const bad = selectors(lobby).filter((s) => !/^(\.shell-child|\.lobby|\.lb-|html:has\(\.shell-child\))/.test(s));
    expect(bad).toEqual([]);
  });

  // Ловит: окно ребёнка получило тёмную схему браузера или тёмную тему (R-11: тёмной темы нет нигде).
  it("без prefers-color-scheme и color-scheme: dark", () => {
    expect(lobby).not.toMatch(/prefers-color-scheme/);
    expect(lobby).not.toMatch(/color-scheme:\s*dark/);
  });

  it("фон — радиальный синий макета, шрифт Rubik", () => {
    const shell = rule(".shell-child");
    expect(shell).toMatch(/radial-gradient\(ellipse 90% 70% at 50% 25%, #4a86ff 0%, #2a5ce0 45%, #15318c 100%\)/);
    expect(shell).toMatch(/--font:\s*"Rubik"/);
  });

  // Ловит (PL-25): окно ребёнка прокручивается целиком вместе с содержимым.
  it("прокручивается только страница: шапка «PC Block» скрыта, .content не меняет позиционирование", () => {
    expect(rule(".shell-child > .topbar")).toMatch(/display:\s*none/);
    expect(rule(".shell-child .content")).not.toMatch(/position:|overflow/);
    expect(lobby).not.toMatch(/position:\s*(sticky|fixed)/);
  });
});

describe("«Игровое лобби»: текст", () => {
  // Ловит (Review Focus 5): белые цифры без обводки или обводка поверх букв (тонкие буквы на ярком фоне).
  it("крупный текст — обводка под заливкой и тень", () => {
    const big = rule(".lb-big");
    expect(big).toMatch(/-webkit-text-stroke:\s*var\(--lb-stroke\) var\(--lb-ink\)/);
    expect(big).toMatch(/paint-order:\s*stroke fill/);
    expect(big).toMatch(/text-shadow:/);
    expect(rule(".lb-left-value")).toMatch(/--lb-stroke:\s*6px/);
  });

  // Ловит: мелкий вторичный текст прямо на светлой части фона (дата у шапки) ниже 4,5:1 без обводки.
  it("мелкий текст на фоне — с обводкой", () => {
    expect(lobby).toMatch(/\.lb-date,\s*\n\.lb-loading,\s*\n\.lb-foot,[^{]*\{[^}]*-webkit-text-stroke:\s*2px/);
  });

  it.each([
    ["#c7d4ff", "#13287a", "вторичный на панели"],
    ["#ffffff", "#13287a", "белый на панели"],
    ["#a9bcf5", "#132c8a", "подпись делений на главном блоке"],
    ["#bfd0ff", "#132c8a", "«Осталось сегодня»"],
    ["#9ff0c4", "#13287a", "«Занимайся сколько хочешь»"],
    ["#d9d3ff", "#2e2a8f", "часы на ночном блоке"],
    ["#ffe066", "#0e1f66", "пояснение недоступной кнопки"],
    ["#0b1d5c", "#ffb800", "текст золотой кнопки"],
    ["#0b1d5c", "#2fcb5c", "текст зелёной кнопки"],
    ["#c7d4ff", "#22388f", "текст недоступной кнопки"],
    ["#ffb199", "#13287a", "ошибка на панели"],
  ])("%s на %s ≥ 4,5 (%s)", (fg, bg) => {
    expect(lobby.toLowerCase()).toContain(fg);
    expect(ratio(fg, bg)).toBeGreaterThanOrEqual(4.5);
  });

  // Ловит: кольцо фокуса цветом текста (белое на белой цифре, синее на синем фоне) не видно.
  it("кольцо фокуса золотое 3 px", () => {
    expect(rule(".shell-child :focus-visible")).toMatch(/outline:\s*3px solid #ffe066/);
  });

  // Ловит (L-17): недоступная «Взять из копилки» выглядит рабочей.
  it("недоступная кнопка — пунктир и курсор запрета", () => {
    const r = lobby.slice(lobby.indexOf('.lb-btn[aria-disabled="true"],'));
    const body = r.slice(0, r.indexOf("}"));
    expect(body).toMatch(/border-style:\s*dashed/);
    expect(body).toMatch(/cursor:\s*not-allowed/);
  });

  // Ловит (R-9): в окне 640…760 px плитки стоят в ряд и сжимают числа.
  it("уже 760 px плитки в столбец", () => {
    expect(lobby).toMatch(/@media \(max-width: 760px\)[\s\S]*\.lb-tiles \{\s*flex-direction: column;/);
  });
});
