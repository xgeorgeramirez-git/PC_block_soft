import { describe, expect, it } from "vitest";
import { WHATS_NEW } from "./entries";

// Каталог «Что нового» (FR-36): записи версий для родителя, новая сверху.

/** Числа версии для сравнения: 0.10.0 новее 0.9.0, строки так не сравнить. */
function parts(v: string): number[] {
  return v.split(".").map((x) => Number.parseInt(x, 10));
}

function newer(a: string, b: string): boolean {
  const pa = parts(a);
  const pb = parts(b);
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] > pb[i];
  }
  return false;
}

/** Все тексты записи: название и пункты. */
function texts(): string[] {
  return WHATS_NEW.flatMap((e) => [e.title, ...e.items]);
}

describe("каталог «Что нового»", () => {
  it("в каталоге записи выпущенных версий, новая сверху", () => {
    expect(WHATS_NEW.map((e) => e.version)).toEqual(["0.2.5", "0.2.4", "0.2.2", "0.2.0", "0.1.0"]);
  });

  // Ловит: версия записана не как X.Y.Z (опечатка «0.2,5» или «v0.2.5»).
  it("каждая версия в виде X.Y.Z", () => {
    for (const e of WHATS_NEW) {
      expect(e.version).toMatch(/^\d+\.\d+\.\d+$/);
    }
  });

  // Ловит: одна версия записана дважды или новая запись вставлена не сверху.
  it("версии не повторяются и идут от новой к старой", () => {
    const versions = WHATS_NEW.map((e) => e.version);
    expect(new Set(versions).size).toBe(versions.length);
    for (let i = 1; i < versions.length; i++) {
      expect(newer(versions[i - 1], versions[i]), `${versions[i - 1]} должна быть новее ${versions[i]}`).toBe(true);
    }
  });

  // Ловит: запись версии без единого пункта (родитель видит пустой заголовок).
  it("у каждой записи название и хотя бы один пункт", () => {
    for (const e of WHATS_NEW) {
      expect(e.title.trim()).not.toBe("");
      expect(e.items.length).toBeGreaterThanOrEqual(1);
    }
  });

  // Ловит: длинное тире в тексте (правило 8.1, скан текстов в CI).
  it("в текстах нет длинного тире", () => {
    for (const t of texts()) {
      expect(t).not.toMatch(/—/);
    }
  });

  // Ловит: английские слова в тексте для родителя (NFR-11), кроме названий.
  it("в текстах нет латиницы, кроме «PC Block», «Windows» и «Brave»", () => {
    for (const t of texts()) {
      const rest = t.replace(/PC Block|Windows|Brave/g, "");
      expect(rest, t).not.toMatch(/[A-Za-z]/);
    }
  });
});
