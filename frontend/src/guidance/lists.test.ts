import { describe, expect, it } from "vitest";
import type { WhatsNewEntry } from "./catalog";
import { newestFirst, plural } from "./lists";

const STEPS = ["шаг", "шага", "шагов"] as const;

describe("plural", () => {
  // Ловит: «5 шага» или «1 шагов» в списке туров (урок L-36, склонение в генерируемых текстах).
  it.each([
    [1, "шаг"],
    [2, "шага"],
    [4, "шага"],
    [5, "шагов"],
    [6, "шагов"],
    [11, "шагов"],
    [12, "шагов"],
    [14, "шагов"],
    [21, "шаг"],
    [22, "шага"],
    [25, "шагов"],
    [111, "шагов"],
    [0, "шагов"],
  ])("%i: %s", (n, word) => {
    expect(plural(n, STEPS)).toBe(word);
  });
});

const news = (version: string): WhatsNewEntry => ({ id: `N-${version}`, kind: "whatsnew", version, title: "т", items: ["п"] });

describe("newestFirst", () => {
  // Ловит: версия 0.10.0 оказывается ниже 0.9.0 (сравнение строкой вместо чисел).
  it("сравнивает версии по числам, а не по буквам", () => {
    expect(newestFirst([news("0.9.0"), news("0.10.0"), news("0.2.1")]).map((n) => n.version)).toEqual(["0.10.0", "0.9.0", "0.2.1"]);
  });

  it("версия из двух частей равна версии с нулём на конце", () => {
    expect(newestFirst([news("1.0"), news("1.0.1")]).map((n) => n.version)).toEqual(["1.0.1", "1.0"]);
  });

  it("не меняет исходный список", () => {
    const src = [news("0.1.0"), news("0.2.0")];
    newestFirst(src);
    expect(src.map((n) => n.version)).toEqual(["0.1.0", "0.2.0"]);
  });
});
