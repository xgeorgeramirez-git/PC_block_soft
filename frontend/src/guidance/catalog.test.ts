import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { ANCHORS, HINTS, LABELS, TOURS, WHATS_NEW, fillName, hintById, tourById } from "./catalog";

// Каталог Помощника - данные (ТЗ 8.4 п.8). Правила длины и тире проверяет Go-валидатор
// (internal/guidance), здесь - что окно и Go-зеркало говорят одно и то же и что каталог
// пригоден для движка.

const goSource = readFileSync(resolve(__dirname, "../../../internal/guidance/catalog.go"), "utf8");

/** Все тексты каталога окна: номера, якоря, заголовки, тексты, подписи, шаги, пункты. */
function allStrings(): Set<string> {
  const out = new Set<string>();
  const add = (s: string | undefined) => {
    if (s) out.add(s);
  };
  for (const h of HINTS) {
    [h.id, h.anchor, h.title, h.body, h.howTo?.title].forEach(add);
    h.actions.forEach((a) => add(a.label));
    h.howTo?.steps.forEach(add);
  }
  for (const t of TOURS) {
    [t.id, t.title].forEach(add);
    t.covers.forEach(add);
    for (const s of t.steps) {
      [s.anchor, s.screen, s.title, s.body].forEach(add);
    }
  }
  for (const n of WHATS_NEW) {
    [n.id, n.version, n.title].forEach(add);
    n.items.forEach(add);
  }
  Object.values(LABELS).forEach(add);
  return out;
}

/** Строковые литералы Go-зеркала от «func Catalog()» до конца файла: записи каталога. */
function goLiterals(): Set<string> {
  const out = new Set<string>();
  const from = goSource.indexOf("func Catalog()");
  for (const m of goSource.slice(from).matchAll(/"((?:[^"\\\n]|\\.)*)"/g)) {
    out.add(m[1]);
  }
  return out;
}

describe("зеркало каталога на Go", () => {
  // Ловит: тексты правили в одном месте. Окно показывает одно, а проверка в CI проверяет другое.
  it("каждый текст каталога окна есть в internal/guidance/catalog.go", () => {
    const lost = [...allStrings()].filter((s) => !goSource.includes(`"${s}"`));
    expect(lost).toEqual([]);
  });

  it("в Go-зеркале нет записей и текстов, которых нет в окне", () => {
    const own = allStrings();
    // Идентификаторы действий и экранов в Go - константы (ActionAck, ScreenToday), литералами они не пишутся.
    const extra = [...goLiterals()].filter((s) => !own.has(s));
    expect(extra).toEqual([]);
  });

  it("общие подписи совпадают с Go-зеркалом", () => {
    const labelsBlock = goSource.slice(goSource.indexOf("var Labels"), goSource.indexOf("func Catalog()"));
    const goLabels = Object.fromEntries([...labelsBlock.matchAll(/"(\w+)":\s+"([^"]*)"/g)].map((m) => [m[1], m[2]]));
    expect(goLabels).toEqual(LABELS);
  });

  it("список якорей совпадает с Go-зеркалом", () => {
    const screenOf: Record<string, string> = {
      ScreenAny: "any",
      ScreenToday: "today",
      ScreenChildren: "children",
      ScreenBonus: "bonus",
      ScreenJournal: "journal",
      ScreenSettings: "settings",
      ScreenHelp: "help",
    };
    const goAnchors = [...goSource.matchAll(/\{ID: "([^"]+)", Screen: (Screen\w+)\}/g)].map((m) => [m[1], screenOf[m[2]]]);
    expect(goAnchors).toEqual(ANCHORS.map((a) => [a.id, a.screen]));
  });

  it("виды и признаки записей совпадают", () => {
    for (const h of HINTS) {
      const block = goSource.slice(goSource.indexOf(`ID:     "${h.id}"`));
      const head = block.slice(0, block.indexOf("\n\t\t},"));
      expect(head, h.id).toContain(h.kind === "hint" ? "KindHint" : "KindDetector");
      expect(head.includes("Once:   true") || head.includes("Once: true"), `${h.id} Once`).toBe(Boolean(h.once));
      expect(/Repeat:\s+true/.test(head), `${h.id} Repeat`).toBe(Boolean(h.repeat));
    }
  });

  it("порядок номеров записей тот же", () => {
    const goIds = [...goSource.matchAll(/ID:\s+"([HDTN]-[^"]+)"/g)].map((m) => m[1]);
    const own = [...HINTS.map((h) => h.id), ...TOURS.map((t) => t.id), ...WHATS_NEW.map((n) => n.id)];
    expect(goIds.sort()).toEqual(own.sort());
  });
});

describe("состав каталога M1+M2 (раздел «Покрытие Помощника» планов)", () => {
  it("подсказки, напоминания, туры и «Что нового» по вехам", () => {
    expect(HINTS.filter((h) => h.kind === "hint").map((h) => h.id)).toEqual([
      "H-M1-01",
      "H-M1-02",
      "H-M1-03",
      "H-M2-01",
      "H-M2-02",
      "H-M2-03",
      "H-M2-04",
      "H-M2-05",
      "H-M2-06",
      "H-M2-07",
    ]);
    expect(HINTS.filter((h) => h.kind === "detector").map((h) => h.id)).toEqual(["D-M1-01", "D-M1-02", "D-M2-01"]);
    expect(TOURS.map((t) => t.id)).toEqual(["T-M1-01", "T-M2-01"]);
    expect(WHATS_NEW.map((n) => n.id)).toEqual(["N-0.1.0", "N-0.2.0", "N-0.2.2"]);
  });

  it("номера уникальны", () => {
    const ids = [...HINTS.map((h) => h.id), ...TOURS.map((t) => t.id), ...WHATS_NEW.map((n) => n.id)];
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("каждый якорь каталога есть в списке якорей", () => {
    const known = new Set<string>(ANCHORS.map((a) => a.id));
    for (const h of HINTS) expect(known.has(h.anchor), h.id).toBe(true);
    for (const t of TOURS) for (const s of t.steps) expect(known.has(s.anchor), `${t.id} ${s.anchor}`).toBe(true);
  });

  // Ловит: в карточке больше двух кнопок-действий, у риска нет кнопки, у приглашения нет «Больше не напоминать».
  it("правила карточек: до двух действий, у напоминаний действие и «повторяющееся»", () => {
    for (const h of HINTS) {
      expect(h.actions.length, h.id).toBeLessThanOrEqual(2);
      if (h.kind === "detector") {
        expect(h.actions.length, h.id).toBeGreaterThan(0);
        expect(h.repeat, h.id).toBe(true);
      }
    }
    expect(hintById("H-M1-01")?.once).toBe(true);
  });

  it("тур: не больше шести шагов, шаги ведут на экраны", () => {
    const tour = tourById("T-M1-01")!;
    expect(tour.steps.length).toBeGreaterThan(0);
    expect(tour.steps.length).toBeLessThanOrEqual(6);
    expect(tour.steps.map((s) => s.anchor)).toEqual([
      "menu-today",
      "child-card",
      "extra-buttons",
      "menu-children",
      "limits-table",
      "savebar",
    ]);
  });

  it("тур закрывает подсказки, которые сам объяснил", () => {
    for (const id of tourById("T-M1-01")!.covers) expect(hintById(id)?.kind).toBe("hint");
  });

  // Ловит: длинные тире и эмодзи в данных (NFR-11, L-45); подробнее - Go-валидатор.
  it("в текстах нет длинных тире и эмодзи", () => {
    for (const s of allStrings()) {
      expect(s, s).not.toMatch(/[‒-―−]/);
      expect(s, s).not.toMatch(/\p{Extended_Pictographic}/u);
    }
  });

  // Ловит: родителю пишут на «ты» (A4).
  it("родителю на «вы»: без «ты»-форм", () => {
    for (const s of allStrings()) {
      expect(s, s).not.toMatch(/(?<!\p{L})(ты|тебе|тебя|твой\p{L}*|задай|нажми|попробуй)(?!\p{L})/iu);
    }
  });
});

describe("fillName: имя в шаблоне", () => {
  it("родительный падеж для {nameGen} (L-36)", () => {
    expect(fillName("Похоже, у {nameGen} нет ограничений", "Миша")).toBe("Похоже, у Миши нет ограничений");
    expect(fillName("У {nameGen} теперь права", "Даша")).toBe("У Даши теперь права");
    expect(fillName("У {nameGen} теперь права", "Катя")).toBe("У Кати теперь права");
    expect(fillName("У {nameGen} теперь права", "Женя")).toBe("У Жени теперь права");
    expect(fillName("У {nameGen}", "Артём")).toBe("У Артёма");
  });

  it("{name} без изменений", () => {
    expect(fillName("Привет, {name}", "Миша")).toBe("Привет, Миша");
  });

  // Ловит: имя, которое не склоняется правилом, ломается или теряется.
  it("иностранное и составное имя остаются как есть", () => {
    expect(fillName("У {nameGen}", "Mary Ann")).toBe("У Mary Ann");
    expect(fillName("У {nameGen}", "Анна-Мария")).toBe("У Анна-Марии");
  });

  // Ловит: длинное имя растягивает карточку (проверка усечения на длинном значении).
  it("длинное имя усекается", () => {
    const long = "Ы".repeat(60);
    const out = fillName("{name}", long);
    expect([...out].length).toBe(40);
    expect(out.endsWith("…")).toBe(true);
  });

  it("пробелы по краям имени убираются", () => {
    expect(fillName("{name}!", "  Миша ")).toBe("Миша!");
  });

  it("имя со служебными символами остаётся текстом", () => {
    expect(fillName("{name}", "<b>Миша</b>")).toBe("<b>Миша</b>");
  });
});
