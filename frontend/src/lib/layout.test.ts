import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Вёрстка окна ребёнка проверяется по styles.css: jsdom не считает раскладку, а снимки идут только в CI.
const css = readFileSync(resolve(__dirname, "../styles.css"), "utf8");

function rule(selector: string): string {
  const at = css.indexOf(`\n${selector} {`);
  if (at < 0) throw new Error(`в styles.css нет правила ${selector}`);
  return css.slice(at, css.indexOf("}", at));
}

describe("окно «Моя статистика»: вёрстка", () => {
  // Ловит: «Настройки для родителя» уезжает под прокрутку окна 800 x 640 (кадр s34-mystats-today).
  it("ссылка «Настройки для родителя» липнет к нижнему краю окна", () => {
    const r = rule(".my-foot");
    expect(r).toMatch(/position:\s*sticky/);
    expect(r).toMatch(/bottom:\s*0/);
    expect(r).toMatch(/background:\s*var\(--bg\)/);
  });

  // Ловит: недоступная «Взять из копилки» выглядит как рабочая (кадр s06-spend-forbidden).
  it("недоступная кнопка приглушена и показывает курсор запрета", () => {
    const r = rule('.btn[aria-disabled="true"]');
    expect(r).toMatch(/color:\s*var\(--text-muted\)/);
    expect(r).toMatch(/cursor:\s*not-allowed/);
  });

  // Ловит: причина недоступности снова стала мелкой серой подсказкой.
  it("причина недоступности крупнее подсказки и обычного цвета текста", () => {
    const r = rule(".spend-note");
    expect(r).toMatch(/color:\s*var\(--text\)/);
    expect(r).toMatch(/font-size:\s*16px/);
  });

  // Ловит: липкая полоса перекрывает низ последнего блока, а End уводит документ за содержимое
  // (пустой серый кадр s34-mystats-bottom).
  it("запас под липкой полосой и прокрутка только внутри окна ребёнка", () => {
    expect(css).toMatch(/\.content:has\(\.my-foot\)\s*\{[^}]*scroll-padding-bottom:\s*64px/);
    expect(css).toMatch(/\.my-stats\s*\{[^}]*padding-bottom:\s*var\(--space-6\)/);
    expect(css).toMatch(/html:has\(\.shell-child\) body\s*\{[^}]*overflow:\s*hidden/);
  });
});
