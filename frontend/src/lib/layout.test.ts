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
  // Ловит: «Настройки для родителя» липкая или наложенная и пересекается с полем промокода
  // (замечание заказчика): это обычный блок в конце страницы.
  it("ссылка «Настройки для родителя» стоит обычным блоком, без sticky, fixed и absolute", () => {
    const r = rule(".my-foot");
    expect(r).not.toMatch(/position:/);
    expect(r).not.toMatch(/bottom:/);
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

  // Ловит: End уводит документ за содержимое (пустой серый кадр s34-mystats-bottom).
  it("в окне ребёнка прокручивается только страница внутри окна", () => {
    expect(css).toMatch(/html:has\(\.shell-child\) body\s*\{[^}]*overflow:\s*hidden/);
    expect(css).not.toMatch(/position:\s*sticky/);
  });

  // Ловит: шапка или страница лежат поверх области прокрутки (sticky, fixed, absolute, отрицательные
  // отступы) вместо трёх частей в потоке: тогда содержимое уходит под шапку (замечание заказчика).
  // Оболочка общая для окна родителя и окна ребёнка, поэтому правится и проверяется в одном месте.
  it("оболочка: шапка в потоке, прокручивается только область под ней", () => {
    const shell = rule(".shell");
    expect(shell).toMatch(/display:\s*flex/);
    expect(shell).toMatch(/flex-direction:\s*column/);
    const top = rule(".topbar");
    expect(top).not.toMatch(/position:|z-index|margin(-top)?:\s*-/);
    const content = rule(".content");
    expect(content).toMatch(/flex:\s*1/);
    expect(content).toMatch(/min-height:\s*0/);
    expect(content).toMatch(/overflow-y:\s*auto/);
    expect(content).not.toMatch(/position:/);
    expect(rule(".my-stats")).not.toMatch(/position:|margin(-top|-bottom)?:\s*-/);
  });

  // Ловит: блок содержимого вплотную к шапке (нулевой зазор, «Ввести промокод» сливается с названием):
  // шапка с нижней границей и тенью, между шапкой и областью прокрутки 16 px вне её.
  it("шапка отделена линией и тенью, под ней зазор 16 px, не входящий в прокрутку", () => {
    const top = rule(".topbar");
    expect(top).toMatch(/border-bottom:\s*1px solid var\(--line\)/);
    expect(top).toMatch(/box-shadow:/);
    const content = rule(".content");
    expect(content).toMatch(/scroll-padding-top:\s*var\(--space-4\)/);
    // Прокладка структурная: рамка с padding-top и overflow hidden, не поле, которое может схлопнуться.
    const frame = rule(".content-frame");
    expect(frame).toMatch(/padding-top:\s*var\(--space-4\)/);
    expect(frame).toMatch(/overflow:\s*hidden/);
    expect(frame).toMatch(/min-height:\s*0/);
    expect(rule(".shell")).toMatch(/overflow:\s*hidden/);
    expect(rule(".content-centered")).toMatch(/padding-top:\s*var\(--space-4\)/);
  });
});
