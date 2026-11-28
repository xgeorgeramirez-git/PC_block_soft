import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Окно родителя всегда светлое (ТЗ 3.3, 8.0 п. 9, R-11): тёмная тема Windows не меняет его токены.
const css = readFileSync(resolve(__dirname, "styles.css"), "utf8");

/** Тела всех блоков @media (prefers-color-scheme: dark). */
function darkBlocks(): string[] {
  const out: string[] = [];
  const re = /@media\s*\(prefers-color-scheme:\s*dark\)\s*\{/g;
  for (let m = re.exec(css); m; m = re.exec(css)) {
    let depth = 0;
    for (let i = m.index + m[0].length - 1; i < css.length; i++) {
      if (css[i] === "{") depth++;
      if (css[i] === "}" && --depth === 0) {
        out.push(css.slice(m.index + m[0].length, i));
        break;
      }
    }
  }
  return out;
}

describe("окно родителя без тёмной темы", () => {
  // Ловит: вернули блок, перекрашивающий :root (фон, текст, кнопки окна родителя) в тёмный.
  it("в prefers-color-scheme: dark нет :root и токенов окна родителя", () => {
    for (const block of darkBlocks()) {
      expect(block).not.toMatch(/:root\s*\{/);
      expect(block).not.toMatch(/--(bg|surface|line|text|text-muted|primary|primary-text)\s*:/);
    }
  });

  it("схема цвета — только светлая", () => {
    const root = /(^|\n):root\s*\{([^}]*)\}/.exec(css)?.[2] ?? "";
    expect(root).toMatch(/color-scheme:\s*light\s*;/);
    expect(css).not.toMatch(/color-scheme:\s*dark/);
  });
});
