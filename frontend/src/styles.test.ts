import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// Вёрстка, которую jsdom не считает: проверяем правила в styles.css, чтобы их не потерять при правках.
// Настоящий вид сверяют снимки экранов (ci.yml) и снимок окна Windows.

const css = readFileSync(resolve(__dirname, "styles.css"), "utf8");

/** Тело правила с точным селектором (без учёта @media). */
function rule(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const m = new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`, "m").exec(css);
  if (!m) {
    throw new Error(`в styles.css нет правила ${selector}`);
  }
  return m[1];
}

describe("цвет элементов управления", () => {
  // Ловит: переключатель «Показывать подсказки» и флажки «Без лимита» синие по умолчанию браузера:
  // цвет в окне только носитель статуса (ТЗ 8.0 п.3).
  it("флажки и переключатели берут цвет кнопок, а не синий браузера", () => {
    expect(rule(":root")).toMatch(/accent-color:\s*var\(--primary\)/);
  });
});

describe("каркас окна: панель сохранения внизу, страница над ней прокручивается", () => {
  it("окно занимает высоту экрана, а не растёт вместе со страницей", () => {
    expect(rule(".shell")).toMatch(/height:\s*100%/);
    expect(rule(".shell")).not.toMatch(/min-height/);
  });

  it("прокручивается только область страницы", () => {
    expect(rule(".content")).toMatch(/overflow-y:\s*auto/);
    expect(rule(".content")).toMatch(/min-height:\s*0/);
    expect(rule(".main-col")).toMatch(/min-height:\s*0/);
  });

  // Ловит: панель внутри страницы с position: sticky перекрывает строки таблицы (FR-19).
  it("панель сохранения не липнет внутри страницы, а занимает своё место внизу", () => {
    expect(rule(".savebar")).not.toMatch(/position:\s*(sticky|fixed|absolute)/);
    expect(rule(".savebar-slot")).toMatch(/flex:\s*none/);
    expect(rule(".savebar")).not.toMatch(/max-width/);
  });
});

describe("Помощник: вёрстка карточек и тура", () => {
  // Ловит: карточка подсказки сделана плавающей и закрывает кнопки или поля (ТЗ 8.4 п.1).
  it("карточка подсказки стоит в потоке страницы, а не поверх неё", () => {
    expect(rule(".hint-card")).not.toMatch(/position:\s*(fixed|absolute|sticky)/);
  });

  it("пустое место под карточку не занимает высоты", () => {
    expect(rule(".hint-strip:empty")).toMatch(/display:\s*none/);
  });

  // Ловит: затемнение тура не накрывает окно целиком или ниже диалога.
  it("тур закрывает окно целиком и лежит выше диалогов", () => {
    expect(rule(".tour")).toMatch(/position:\s*fixed/);
    expect(rule(".tour")).toMatch(/inset:\s*0/);
    const z = (selector: string) => Number(/z-index:\s*(\d+)/.exec(rule(selector))?.[1]);
    expect(z(".tour")).toBeGreaterThan(z(".dialog-backdrop"));
  });

  it("подсветка тура не ловит нажатия", () => {
    expect(rule(".tour-spot")).toMatch(/pointer-events:\s*none/);
  });

  // Ловит: при «уменьшить анимацию» подсветка всё равно ездит по экрану (a11y, скилл in-app-guidance).
  it("при просьбе об уменьшении анимации подсветка не двигается плавно", () => {
    const m = /@media \(prefers-reduced-motion: reduce\)\s*\{[^}]*\.tour-spot\s*\{([^}]*)\}/.exec(css);
    expect(m?.[1]).toMatch(/transition:\s*none/);
  });
});
