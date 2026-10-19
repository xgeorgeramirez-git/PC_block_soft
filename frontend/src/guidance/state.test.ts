import { describe, expect, it } from "vitest";
import {
  DAY_MS,
  HOUR_MS,
  keyOf,
  parse,
  withAdded,
  withClosed,
  withEnabled,
  withShown,
  withSnooze,
  withoutHidden,
} from "./state";

// Состояние Помощника профиля (ТЗ 8.4 п.7): лежит в базе программы как есть, окно читает его
// терпимо и меняет только то, что нужно, не затирая чужие поля.

describe("parse: чтение состояния", () => {
  it("пустое состояние - «как впервые»: подсказки включены, ничего не показано", () => {
    const d = parse({});
    expect(d.enabled).toBe(true);
    expect([d.seen.size, d.done.size, d.muted.size, d.tours.size]).toEqual([0, 0, 0, 0]);
    expect(d.snoozed).toEqual({});
    expect(d.closed).toEqual({});
    expect(d.hintLog).toEqual([]);
    expect(d.remindLog).toEqual([]);
  });

  // Ловит: битое состояние роняет окно или отключает подсказки навсегда (грабли: «битый JSON - как впервые»).
  it.each([null, undefined, "строка", 42, [], [1, 2], true])("мусор %j читается как пустое состояние", (raw) => {
    const d = parse(raw);
    expect(d.enabled).toBe(true);
    expect(d.seen.size).toBe(0);
  });

  it("поля неверного вида пропускаются, верные остаются", () => {
    const d = parse({
      enabled: "нет",
      seen: ["H-M1-01", 7, null, "H-M1-02"],
      snoozed: { "H-M1-01": { n: 1, w: 5 }, "H-M1-02": "позже", "H-M1-03": { n: "один", w: 1 } },
      closed: { "D-M1-01:1": { n: 2, w: 9 }, "D-M1-01:2": null },
      log: { hint: [1, "два", 3, NaN], remind: "нет" },
    });
    expect(d.enabled).toBe(true);
    expect([...d.seen]).toEqual(["H-M1-01", "H-M1-02"]);
    expect(d.snoozed).toEqual({ "H-M1-01": { n: 1, w: 5 } });
    expect(d.closed).toEqual({ "D-M1-01:1": { n: 2, w: 9 } });
    expect(d.hintLog).toEqual([1, 3]);
    expect(d.remindLog).toEqual([]);
  });

  it("выключено только при явном false", () => {
    expect(parse({ enabled: false }).enabled).toBe(false);
    expect(parse({ enabled: true }).enabled).toBe(true);
    expect(parse({ enabled: 0 }).enabled).toBe(true);
  });
});

describe("keyOf: ключ записи для одного ребёнка", () => {
  it("без ребёнка - номер записи, с ребёнком - номер и его id", () => {
    expect(keyOf("H-M1-02")).toBe("H-M1-02");
    expect(keyOf("D-M1-01", "7")).toBe("D-M1-01:7");
    expect(keyOf("D-M1-01", "")).toBe("D-M1-01");
  });
});

describe("изменения состояния", () => {
  // Ловит: переключатель или отметка затирают поля, о которых окно ничего не знает (позднейшие версии).
  it("чужие поля не теряются", () => {
    const raw = { enabled: true, seen: ["H-M1-01"], snoozed: {}, будущее: { x: 1 } };
    expect(withEnabled(raw, false)).toEqual({ enabled: false, seen: ["H-M1-01"], snoozed: {}, будущее: { x: 1 } });
    expect(withAdded(raw, "done", "H-M1-02")).toEqual({ ...raw, done: ["H-M1-02"] });
  });

  it("исходный объект не меняется", () => {
    const raw = { seen: ["H-M1-01"] };
    const frozen = JSON.stringify(raw);
    withAdded(raw, "seen", "H-M1-02");
    withEnabled(raw, false);
    withSnooze(raw, "H-M1-01", 5);
    withClosed(raw, "H-M1-01", 5);
    withShown(raw, "hint", 1);
    withoutHidden(raw);
    expect(JSON.stringify(raw)).toBe(frozen);
  });

  it("выключатель на пустом состоянии пишет только «включено»", () => {
    expect(withEnabled({}, false)).toEqual({ enabled: false });
  });

  it("повторная отметка не плодит дубли", () => {
    const once = withAdded({}, "seen", "H-M1-01");
    expect(withAdded(once, "seen", "H-M1-01")).toEqual({ seen: ["H-M1-01"] });
  });

  it("«Позже» считается: первое и второе нажатие", () => {
    const one = withSnooze({}, "H-M1-01", 100);
    expect(parse(one).snoozed["H-M1-01"]).toEqual({ n: 1, w: 100 });
    const two = withSnooze(one, "H-M1-01", 200);
    expect(parse(two).snoozed["H-M1-01"]).toEqual({ n: 2, w: 200 });
  });

  it("крестик считается: дважды - и запись замолкает", () => {
    const one = withClosed({}, "D-M1-01:1", 100);
    expect(parse(one).closed["D-M1-01:1"]).toEqual({ n: 1, w: 100 });
    expect(parse(withClosed(one, "D-M1-01:1", 200)).closed["D-M1-01:1"]).toEqual({ n: 2, w: 200 });
  });

  it("показы пишутся в журнал своего вида", () => {
    const a = withShown({}, "hint", 1000);
    const b = withShown(a, "detector", 2000);
    const d = parse(b);
    expect(d.hintLog).toEqual([1000]);
    expect(d.remindLog).toEqual([2000]);
  });

  // Ловит: журнал показов растёт без конца (база за годы раздувается).
  it("журнал помнит только сутки", () => {
    const now = 10 * DAY_MS;
    const raw = { log: { hint: [now - 2 * DAY_MS, now - DAY_MS - 1, now - HOUR_MS], remind: [now - 3 * DAY_MS] } };
    const d = parse(withShown(raw, "hint", now));
    expect(d.hintLog).toEqual([now - HOUR_MS, now]);
    expect(d.remindLog).toEqual([]);
  });

  it("«Вернуть скрытые»: убирает отметки, но не выключатель, туры и журнал показов", () => {
    const raw = {
      enabled: false,
      seen: ["H-M1-01"],
      done: ["H-M1-02"],
      muted: ["D-M1-01:1"],
      snoozed: { "H-M1-03": { n: 1, w: 1 } },
      closed: { "D-M1-02:1": { n: 1, w: 1 } },
      tours: ["T-M1-01"],
      log: { hint: [5], remind: [6] },
      будущее: 1,
    };
    expect(withoutHidden(raw)).toEqual({
      enabled: false,
      tours: ["T-M1-01"],
      log: { hint: [5], remind: [6] },
      будущее: 1,
    });
  });
});
