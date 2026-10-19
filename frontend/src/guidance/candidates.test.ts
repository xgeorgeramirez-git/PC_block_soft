import { describe, expect, it } from "vitest";
import { buildCandidates, slotKey, type SlotInfo } from "./candidates";
import type { Risk } from "./detectors";

const topbar: SlotInfo = { anchor: "topbar", subject: "", name: "" };
const card = (id: number, name = "Миша"): SlotInfo => ({ anchor: "child-card", subject: String(id), name });
const risk = (entryId: Risk["entryId"], childId: number, name = "Миша"): Risk => ({ entryId, childId, name });

describe("buildCandidates", () => {
  // Ловит: подсказка без своего места на экране всё равно выбирается и показывается «в никуда».
  it("без мест на экране кандидатов нет", () => {
    expect(buildCandidates([], [risk("D-M1-01", 1)])).toEqual([]);
  });

  it("подсказка встаёт в место своего якоря", () => {
    const c = buildCandidates([topbar], []);
    expect(c.map((x) => x.key)).toEqual(["H-M1-01"]);
    expect(c[0].slot).toBe(slotKey("topbar"));
  });

  it("подсказка с несколькими местами берёт первое: она одна на окно", () => {
    const buttons = (s: string): SlotInfo => ({ anchor: "extra-buttons", subject: s, name: "" });
    const c = buildCandidates([buttons("1"), buttons("2")], []);
    expect(c).toHaveLength(1);
    expect(c[0].slot).toBe(slotKey("extra-buttons", "1"));
  });

  // Ловит: напоминание про одного ребёнка показано на карточке другого.
  it("напоминание ставится только на карточку своего ребёнка", () => {
    const c = buildCandidates([card(1, "Миша"), card(2, "Даша")], [risk("D-M1-01", 2, "Даша")]);
    expect(c).toHaveLength(1);
    expect(c[0].key).toBe("D-M1-01:2");
    expect(c[0].slot).toBe(slotKey("child-card", "2"));
    expect(c[0].name).toBe("Даша");
  });

  it("риск есть, а карточки ребёнка на экране нет: напоминания нет", () => {
    expect(buildCandidates([card(1)], [risk("D-M1-02", 5)])).toEqual([]);
  });

  // Ловит: порядок напоминаний зависит от номера записи, а не от детей: про первого ребёнка говорят в конце.
  it("напоминания идут в порядке детей", () => {
    const c = buildCandidates([card(1), card(2, "Даша")], [risk("D-M1-02", 1), risk("D-M1-01", 2, "Даша")]);
    expect(c.map((x) => x.key)).toEqual(["D-M1-02:1", "D-M1-01:2"]);
  });

  it("у двух детей два напоминания с разными ключами", () => {
    const c = buildCandidates([card(1), card(2, "Даша")], [risk("D-M1-01", 1), risk("D-M1-02", 2, "Даша")]);
    expect(c.map((x) => x.key).sort()).toEqual(["D-M1-01:1", "D-M1-02:2"]);
  });
});
