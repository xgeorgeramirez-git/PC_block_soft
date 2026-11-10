import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  ADULT_SOURCE_TEXT,
  DEFAULT_VIDEO_SITES,
  DUPLICATE_TEXT,
  NOT_DOMAIN_TEXT,
  SITE_HINT_TEXT,
  SITE_LISTS,
  cleanDomain,
  filterSites,
  sitesOf,
  sourceDisplay,
} from "./sites";
import type { SiteDTO } from "../types";

// Помощники «Сайтов» (FR-21, RM4-4): упрощённый разборщик домена для мастера и подменной программы
// (экран его не зовёт, PL-39) и копии констант службы, которые сверяются с Go (PL-29).

describe("cleanDomain: домен из ввода родителя", () => {
  const cases: [string, string][] = [
    ["https://www.Example.com/page?x=1", "example.com"],
    ["YouTube.COM", "youtube.com"],
    ["  vk.com  ", "vk.com"],
    ["www.www.a.ru", "a.ru"],
    ["www.ru", "www.ru"],
    ["пример.рф", "xn--e1afmkfd.xn--p1ai"],
    ["user@host.ru/x", "host.ru"],
    ["192.168.0.1:8080", "192.168.0.1"],
    ["example.com.", "example.com"],
    ["view-source:https://a.ru/", "a.ru"],
    ["котики видео", ""],
    ["localhost", ""],
    ["brave://newtab", ""],
    ["file:///c:/x.html", ""],
    ["", ""],
    ["a".repeat(250) + ".com", ""],
  ];
  for (const [raw, want] of cases) {
    it(`«${raw.length > 40 ? raw.slice(0, 20) + "…" : raw}» → «${want}»`, () => {
      expect(cleanDomain(raw)).toBe(want);
    });
  }
});

describe("копии констант службы совпадают с Go", () => {
  // Ловит: мастер показывает другой стартовый набор «Видео и соцсетей», чем кладёт миграция 009.
  it("стартовый набор «Видео и соцсетей» — тот же, что ipc.DefaultVideoSites", () => {
    const go = readFileSync(resolve(__dirname, "../../../internal/ipc/commands_m4.go"), "utf8");
    const m = /DefaultVideoSites = \[\]string\{([^}]*)\}/.exec(go);
    expect(m).not.toBeNull();
    const domains = [...m![1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
    expect(domains).toEqual([...DEFAULT_VIDEO_SITES]);
  });

  it("подпись источника и отказы у поля — тексты службы", () => {
    const go = readFileSync(resolve(__dirname, "../../../internal/service/texts.go"), "utf8");
    const text = (name: string) => {
      const m = new RegExp(`${name}\\s*=\\s*"([^"]+)"`).exec(go);
      expect(m, name).not.toBeNull();
      return m![1];
    };
    expect(text("textAdultSource")).toBe(ADULT_SOURCE_TEXT);
    expect(text("textNotDomain")).toBe(NOT_DOMAIN_TEXT);
    expect(text("textSiteDuplicate")).toBe(DUPLICATE_TEXT);
  });
});

describe("списки и поиск", () => {
  it("три списка в порядке вкладок", () => {
    expect(SITE_LISTS.map((l) => l.title)).toEqual(["Запрещённые", "Учёба", "Видео и соцсети"]);
    expect(SITE_LISTS.map((l) => l.kind)).toEqual(["blocked", "study", "video_social"]);
  });

  it("sitesOf: только свой список, порядок ответа сохраняется", () => {
    const list: SiteDTO[] = [
      { domain: "b.test", kind: "blocked" },
      { domain: "s.test", kind: "study" },
      { domain: "a.test", kind: "blocked" },
    ];
    expect(sitesOf(list, "blocked").map((s) => s.domain)).toEqual(["b.test", "a.test"]);
    expect(sitesOf(list, "video_social")).toEqual([]);
  });

  it("filterSites: подстрока без учёта регистра по обрезанному запросу", () => {
    const list: SiteDTO[] = [
      { domain: "youtube.com", kind: "video_social" },
      { domain: "vk.com", kind: "video_social" },
    ];
    expect(filterSites(list, " YOU ").map((s) => s.domain)).toEqual(["youtube.com"]);
    expect(filterSites(list, "").map((s) => s.domain)).toEqual(["youtube.com", "vk.com"]);
  });
});

describe("подпись источника «18+» для показа", () => {
  // Ловит: в окне 960 px «4.0» уходит одна на новую строку или «BY-SA» рвётся на дефисе.
  it("«лицензия CC BY-SA 4.0» через неразрывные пробелы и неразрывный дефис, остальной текст тот же", () => {
    const shown = sourceDisplay(ADULT_SOURCE_TEXT);
    expect(shown).toContain("лицензия\u00a0CC\u00a0BY\u2011SA\u00a04.0");
    expect(shown).not.toContain("BY-SA");
    expect(shown.replace(/\u00a0/g, " ").replace(/\u2011/g, "-")).toBe(ADULT_SOURCE_TEXT);
  });
  it("подсказка поля «Добавить сайт» с примером", () => {
    expect(SITE_HINT_TEXT).toBe("Можно вставить адрес страницы целиком: программа возьмёт из него только сайт. Например: youtube.com");
  });
});
