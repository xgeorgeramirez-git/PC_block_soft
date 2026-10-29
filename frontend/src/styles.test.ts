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
  // Ловит: переключатели и флажки «Без лимита» синие по умолчанию браузера:
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
