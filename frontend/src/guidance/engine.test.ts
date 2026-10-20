import { describe, expect, it } from "vitest";
import { HINTS, hintById, tourById } from "./catalog";
import {
  HelperEngine,
  LIMITS,
  QUIET_ZONES,
  QuietZones,
  budgetAllows,
  isEligible,
  pick,
  type Candidate,
  type HelperIO,
} from "./engine";
import { DAY_MS, HOUR_MS, keyOf, parse, type Raw } from "./state";

// Движок вежливости (ТЗ 8.4 п.3, 4): одна карточка за раз, лимиты показов, «Позже», крестик,
// «Больше не напоминать», тихие места. Это логика без экрана: что именно увидит родитель,
// проверяют тесты Provider и экранов.

const NOW = 1_000 * DAY_MS;
const W = 1; // метка текущего открытия окна
const NEXT_W = 2;

function cand(id: string, subject = "", name = ""): Candidate {
  const entry = hintById(id)!;
  return { key: keyOf(id, subject), entry, subject, name, slot: `${entry.anchor}|${subject}` };
}

function ctx(raw: Raw = {}, over: Partial<Parameters<typeof pick>[1]> = {}) {
  return { data: parse(raw), now: NOW, windowId: W, shown: new Set<string>(), quiet: false, hold: false, ...over };
}

describe("pick: порядок и одна карточка", () => {
  it("с чистым состоянием выбирает одну: сначала приглашение в тур", () => {
    const got = pick([cand("H-M1-01"), cand("H-M1-02"), cand("H-M1-03")], ctx());
    expect(got?.entry.id).toBe("H-M1-01");
  });

  it("напоминание о риске важнее обычной подсказки", () => {
    const got = pick([cand("H-M1-01"), cand("D-M1-01", "1", "Миша")], ctx());
    expect(got?.entry.id).toBe("D-M1-01");
  });

  it("нет кандидатов - нет карточки", () => {
    expect(pick([], ctx())).toBeNull();
  });

  // Ловит: у закрытой подсказки следующая выскакивает сразу и без остановки (Clippy).
  it("пока ждём следующего открытия экрана (hold), ничего не выбирается", () => {
    expect(pick([cand("H-M1-02")], ctx({}, { hold: true }))).toBeNull();
  });
});

describe("pick: подсказки выключены и тихие места", () => {
  // Ловит: подсказки выключены в «Настройки → Помощник», а карточки появляются (S28).
  it("при выключенных подсказках карточек нет, ни подсказок, ни напоминаний", () => {
    const raw = { enabled: false };
    expect(pick([cand("H-M1-01")], ctx(raw))).toBeNull();
    expect(pick([cand("D-M1-01", "1", "Миша")], ctx(raw))).toBeNull();
  });

  // Ловит: карточка поверх экрана блокировки, предупреждения, ввода пароля, мастера, диалога или тура.
  it.each(QUIET_ZONES)("тихое место «%s»: карточка молчит", (zone) => {
    const quiet = new QuietZones();
    quiet.enter(zone);
    expect(pick([cand("H-M1-01"), cand("D-M1-01", "1", "Миша")], ctx({}, { quiet: quiet.isQuiet() }))).toBeNull();
  });

  it("тихих мест шесть, как в ТЗ 8.4 п.4", () => {
    expect([...QUIET_ZONES].sort()).toEqual(["dialog", "lock", "password", "tour", "warning", "wizard"]);
  });
});

describe("QuietZones", () => {
  it("тишина, пока открыто хотя бы одно место; два диалога - два входа", () => {
    const q = new QuietZones();
    expect(q.isQuiet()).toBe(false);
    const a = q.enter("dialog");
    const b = q.enter("dialog");
    a();
    expect(q.isQuiet()).toBe(true);
    expect(q.active()).toEqual(["dialog"]);
    b();
    expect(q.isQuiet()).toBe(false);
  });

  it("повторный выход из того же места ничего не ломает", () => {
    const q = new QuietZones();
    const leave = q.enter("tour");
    q.enter("password");
    leave();
    leave();
    expect(q.active()).toEqual(["password"]);
  });

  it("слушатель узнаёт о входе и выходе", () => {
    const q = new QuietZones();
    let calls = 0;
    const off = q.subscribe(() => calls++);
    const leave = q.enter("wizard");
    leave();
    off();
    q.enter("lock");
    expect(calls).toBe(2);
  });
});

describe("лимиты показов в час и в день (ТЗ 8.4 п.3)", () => {
  const shown = (hint: number[], remind: number[] = []): Raw => ({ log: { hint, remind } });

  it("значения лимитов как в ТЗ", () => {
    expect(LIMITS).toEqual({ hintsPerHour: 2, hintsPerDay: 5, remindersPerHour: 3 });
  });

  it("две подсказки за час - третья ждёт", () => {
    const raw = shown([NOW - 50 * 60_000, NOW - 10 * 60_000]);
    expect(pick([cand("H-M1-02")], ctx(raw))).toBeNull();
    expect(budgetAllows("hint", parse(raw), NOW)).toBe(false);
  });

  it("час прошёл - можно снова", () => {
    const raw = shown([NOW - HOUR_MS - 1, NOW - 10 * 60_000]);
    expect(pick([cand("H-M1-02")], ctx(raw))?.entry.id).toBe("H-M1-02");
  });

  it("пять подсказок за сутки - шестая ждёт, даже если в часе пусто", () => {
    const times = [1, 2, 3, 4, 5].map((i) => NOW - i * 3 * HOUR_MS);
    expect(budgetAllows("hint", parse(shown(times)), NOW)).toBe(false);
    expect(budgetAllows("hint", parse(shown(times.slice(1))), NOW)).toBe(true);
  });

  it("сутки прошли - счёт сброшен", () => {
    const old = [1, 2, 3, 4, 5].map((i) => NOW - DAY_MS - i * 60_000);
    expect(budgetAllows("hint", parse(shown(old)), NOW)).toBe(true);
  });

  // Ловит: напоминание о риске съедает бюджет подсказок или упирается в него (оно вне счёта).
  it("напоминания о важном вне счёта подсказок", () => {
    const full = shown([NOW - 1000, NOW - 2000, NOW - 3000, NOW - 4000, NOW - 5000]);
    expect(pick([cand("D-M1-01", "1", "Миша")], ctx(full))?.entry.id).toBe("D-M1-01");
    expect(budgetAllows("detector", parse(full), NOW)).toBe(true);
  });

  it("но напоминаний не больше трёх в час", () => {
    const three = shown([], [NOW - 1000, NOW - 2000, NOW - 3000]);
    expect(pick([cand("D-M1-01", "1", "Миша")], ctx(three))).toBeNull();
    const twoOld = shown([], [NOW - HOUR_MS - 1, NOW - 2000, NOW - 3000]);
    expect(pick([cand("D-M1-01", "1", "Миша")], ctx(twoOld))?.entry.id).toBe("D-M1-01");
  });

  it("пока напоминания на отдыхе, подсказка своего бюджета идёт вперёд", () => {
    const three = shown([], [NOW - 1000, NOW - 2000, NOW - 3000]);
    expect(pick([cand("D-M1-01", "1", "Миша"), cand("H-M1-02")], ctx(three))?.entry.id).toBe("H-M1-02");
  });
});

describe("«Позже», крестик, «Больше не напоминать»", () => {
  // Ловит: закрытая подсказка появляется снова в том же окне.
  it("уже показанное в этом окне не возвращается", () => {
    const c = cand("H-M1-02");
    expect(pick([c], ctx({}, { shown: new Set([c.key]) }))).toBeNull();
  });

  it("обычная подсказка, показанная раньше и не закрытая, больше не нужна", () => {
    expect(pick([cand("H-M1-02")], ctx({ seen: ["H-M1-02"] }))).toBeNull();
  });

  it("напоминание о риске показывается снова в новом окне, пока риск есть", () => {
    const c = cand("D-M1-01", "1", "Миша");
    expect(isEligible(c, ctx({ seen: [c.key] }, { windowId: NEXT_W }))).toBe(true);
  });

  it("«Позже»: в этом окне нет, в следующем да", () => {
    const raw = { snoozed: { "H-M1-01": { n: 1, w: W } }, seen: ["H-M1-01"] };
    expect(isEligible(cand("H-M1-01"), ctx(raw))).toBe(false);
    expect(isEligible(cand("H-M1-01"), ctx(raw, { windowId: NEXT_W }))).toBe(true);
  });

  // Ловит: «Позже» возвращает запись снова и снова (ТЗ: не больше одного раза).
  it("«Позже» второй раз - больше не возвращается никогда", () => {
    const raw = { snoozed: { "H-M1-01": { n: 2, w: W } }, seen: ["H-M1-01"] };
    expect(isEligible(cand("H-M1-01"), ctx(raw, { windowId: NEXT_W }))).toBe(false);
    expect(isEligible(cand("H-M1-01"), ctx(raw, { windowId: 99 }))).toBe(false);
  });

  it("крестик один раз: в этом окне нет, в следующем второй шанс", () => {
    const raw = { closed: { "D-M1-01:1": { n: 1, w: W } } };
    const c = cand("D-M1-01", "1", "Миша");
    expect(isEligible(c, ctx(raw))).toBe(false);
    expect(isEligible(c, ctx(raw, { windowId: NEXT_W }))).toBe(true);
  });

  // Ловит: закрытая дважды крестиком подсказка опять появляется (ТЗ 8.4 п.3).
  it("крестик дважды - больше не появляется", () => {
    const raw = { closed: { "D-M1-01:1": { n: 2, w: W } } };
    const c = cand("D-M1-01", "1", "Миша");
    expect(isEligible(c, ctx(raw, { windowId: NEXT_W }))).toBe(false);
    expect(isEligible(c, ctx(raw, { windowId: 500 }))).toBe(false);
  });

  it("«Больше не напоминать» - навсегда", () => {
    const raw = { muted: ["D-M1-01:1"] };
    expect(isEligible(cand("D-M1-01", "1", "Миша"), ctx(raw, { windowId: 777 }))).toBe(false);
  });

  it("принятое действием больше не показывается", () => {
    expect(isEligible(cand("H-M1-02"), ctx({ done: ["H-M1-02"] }))).toBe(false);
  });

  // Ловит: «Больше не напоминать» про одного ребёнка глушит напоминание про другого.
  it("отметки про одного ребёнка не касаются другого", () => {
    const raw = { muted: ["D-M1-01:1"], closed: { "D-M1-01:1": { n: 2, w: W } } };
    expect(isEligible(cand("D-M1-01", "1", "Миша"), ctx(raw))).toBe(false);
    expect(isEligible(cand("D-M1-01", "2", "Даша"), ctx(raw))).toBe(true);
  });
});

// --- Движок с сохранением -----------------------------------------------------

function memoryIO(initial: Raw = {}) {
  const saves: Raw[] = [];
  const io: HelperIO & { stored: Raw; failSave: boolean; failLoad: boolean; delay: number } = {
    stored: initial,
    failSave: false,
    failLoad: false,
    delay: 0,
    async load() {
      if (io.failLoad) throw new Error("нет связи");
      return structuredClone(io.stored);
    },
    async save(raw) {
      if (io.delay) await new Promise((r) => setTimeout(r, io.delay));
      if (io.failSave) throw new Error("нет связи");
      saves.push(structuredClone(raw));
      io.stored = structuredClone(raw);
    },
  };
  return { io, saves };
}

function engineOf(initial: Raw = {}, now = () => NOW) {
  const m = memoryIO(initial);
  const engine = new HelperEngine(m.io, { now, windowId: W });
  return { engine, ...m };
}

describe("HelperEngine: загрузка", () => {
  it("до загрузки не готов и карточек не выбирает", () => {
    const { engine } = engineOf();
    expect(engine.ready).toBe(false);
    expect(engine.pick([cand("H-M1-01")], false, false)).toBeNull();
  });

  it("после загрузки читает сохранённое", async () => {
    const { engine } = engineOf({ enabled: false, seen: ["H-M1-01"] });
    await engine.load();
    expect(engine.ready).toBe(true);
    expect(engine.data.enabled).toBe(false);
    expect(engine.data.seen.has("H-M1-01")).toBe(true);
  });

  // Ловит: сбой чтения состояния делает Помощника «как впервые» и затирает сохранённое при следующей записи.
  it("не прочиталось: не готов, ничего не записывает и не показывает", async () => {
    const { engine, io, saves } = engineOf({ seen: ["H-M1-01"] });
    io.failLoad = true;
    await engine.load();
    expect(engine.ready).toBe(false);
    expect(engine.loadFailed).toBe(true);
    expect(engine.pick([cand("H-M1-02")], false, false)).toBeNull();
    engine.markShown(cand("H-M1-02"));
    await engine.flush();
    expect(saves).toEqual([]);
    // Связь вернулась: следующая попытка читает и работает.
    io.failLoad = false;
    await engine.load();
    expect(engine.ready).toBe(true);
    expect(engine.loadFailed).toBe(false);
  });

  // Ловит: ответ на карточку до прочтения состояния пишет в базу «пустое» и затирает сохранённое.
  it("не прочиталось: ни один ответ на карточку ничего не пишет", async () => {
    const { engine, io, saves } = engineOf({ seen: ["H-M1-01"], muted: ["D-M1-01:1"] });
    io.failLoad = true;
    await engine.load();
    const c = cand("D-M1-01", "1", "Миша");
    engine.later(c);
    engine.close(c);
    engine.mute(c);
    engine.accept(c);
    engine.startedTour();
    engine.finishTour(tourById("T-M1-01")!);
    await engine.resetHidden();
    await engine.flush();
    expect(saves).toEqual([]);
    expect(io.stored).toEqual({ seen: ["H-M1-01"], muted: ["D-M1-01:1"] });
  });

  // Ловит: причина сбоя чтения - техническая строка, которую увидит родитель в «Настройках».
  it("причина сбоя чтения - человеческими словами", async () => {
    const { engine, io } = engineOf();
    io.failLoad = true;
    await engine.load();
    expect(engine.loadError).not.toBe("");
    expect(engine.loadError).not.toMatch(/Error|undefined|fail|ipc/i);
    io.failLoad = false;
    await engine.load();
    expect(engine.loadError).toBe("");
  });

  it("не объект вместо состояния - как впервые", async () => {
    const m = memoryIO();
    m.io.load = async () => "ерунда" as unknown as Raw;
    const engine = new HelperEngine(m.io, { now: () => NOW, windowId: W });
    await engine.load();
    expect(engine.ready).toBe(true);
    expect(engine.data.enabled).toBe(true);
  });
});

describe("HelperEngine: выбор и отметки", () => {
  it("показ записывается: журнал, «показано» и запись в базу", async () => {
    const { engine, saves } = engineOf();
    await engine.load();
    const c = cand("H-M1-02");
    expect(engine.pick([c], false, false)?.key).toBe("H-M1-02");
    engine.markShown(c);
    await engine.flush();
    expect(saves.at(-1)).toMatchObject({ seen: ["H-M1-02"], log: { hint: [NOW], remind: [] } });
    // Показанное в этом окне второй раз не выбирается.
    expect(engine.pick([c], false, false)).toBeNull();
  });

  it("напоминание пишется в свой журнал и не помечается «обычной показанной»", async () => {
    const { engine, saves } = engineOf();
    await engine.load();
    engine.markShown(cand("D-M1-01", "1", "Миша"));
    await engine.flush();
    expect(saves.at(-1)).toMatchObject({ log: { hint: [], remind: [NOW] } });
  });

  it("«Позже»: записано, в этом окне не вернётся, в новом вернётся один раз", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    const c = cand("H-M1-01");
    engine.markShown(c);
    engine.later(c);
    await engine.flush();
    expect(parse(io.stored).snoozed["H-M1-01"]).toEqual({ n: 1, w: W });
    expect(engine.pick([c], false, false)).toBeNull();

    const next = new HelperEngine(io, { now: () => NOW + 1, windowId: NEXT_W });
    await next.load();
    expect(next.pick([c], false, false)?.key).toBe("H-M1-01");
    next.markShown(c);
    next.later(c);
    await next.flush();
    expect(parse(io.stored).snoozed["H-M1-01"].n).toBe(2);
    const third = new HelperEngine(io, { now: () => NOW + 2, windowId: 3 });
    await third.load();
    expect(third.pick([c], false, false)).toBeNull();
  });

  it("крестик дважды в разных окнах - навсегда", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    const c = cand("D-M1-01", "1", "Миша");
    engine.markShown(c);
    engine.close(c);
    await engine.flush();
    const next = new HelperEngine(io, { now: () => NOW, windowId: NEXT_W });
    await next.load();
    expect(next.pick([c], false, false)?.key).toBe("D-M1-01:1");
    next.markShown(c);
    next.close(c);
    await next.flush();
    const third = new HelperEngine(io, { now: () => NOW, windowId: 3 });
    await third.load();
    expect(third.pick([c], false, false)).toBeNull();
  });

  it("крестик на приглашении, которое показывают один раз, закрывает его насовсем", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    const c = cand("H-M1-01");
    engine.markShown(c);
    engine.close(c);
    await engine.flush();
    expect(parse(io.stored).done.has("H-M1-01")).toBe(true);
  });

  it("«Больше не напоминать» и «принято» записываются", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    const risk = cand("D-M1-01", "1", "Миша");
    engine.mute(risk);
    engine.accept(cand("H-M1-02"));
    await engine.flush();
    const d = parse(io.stored);
    expect(d.muted.has("D-M1-01:1")).toBe(true);
    expect(d.done.has("H-M1-02")).toBe(true);
  });

  it("при сбое записи окно не падает и работает с тем, что есть в памяти", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    io.failSave = true;
    const c = cand("H-M1-02");
    engine.markShown(c);
    engine.accept(c);
    await expect(engine.flush()).resolves.toBeUndefined();
    expect(engine.data.done.has("H-M1-02")).toBe(true);
  });

  // Ловит: записи идут вперемешку, и более старое состояние затирает новое.
  it("записи уходят по порядку, последняя - с самым новым состоянием", async () => {
    const { engine, io, saves } = engineOf();
    await engine.load();
    io.delay = 5;
    engine.accept(cand("H-M1-02"));
    engine.accept(cand("H-M1-03"));
    engine.mute(cand("D-M1-01", "1", "Миша"));
    await engine.flush();
    const last = parse(saves.at(-1));
    expect([...last.done].sort()).toEqual(["H-M1-02", "H-M1-03"]);
    expect(last.muted.has("D-M1-01:1")).toBe(true);
  });
});

describe("HelperEngine: выключатель", () => {
  it("setEnabled пишет только «включено» и не трогает остальное", async () => {
    const { engine, saves } = engineOf({ enabled: true, seen: ["H-M1-01"], snoozed: {} });
    await engine.load();
    await engine.setEnabled(false);
    expect(saves.at(-1)).toEqual({ enabled: false, seen: ["H-M1-01"], snoozed: {} });
    expect(engine.enabled).toBe(false);
  });

  // Ловит: выключатель показывает «выключено», хотя запись не удалась.
  it("не записалось: возвращает прежнее и объясняет ошибкой", async () => {
    const { engine, io } = engineOf({ enabled: true });
    await engine.load();
    io.failSave = true;
    await expect(engine.setEnabled(false)).rejects.toThrow();
    expect(engine.enabled).toBe(true);
  });

  it("слушатели узнают об изменении и о возврате", async () => {
    const { engine, io } = engineOf({ enabled: true });
    await engine.load();
    const seen: boolean[] = [];
    engine.subscribe(() => seen.push(engine.enabled));
    io.failSave = true;
    await expect(engine.setEnabled(false)).rejects.toThrow();
    expect(seen).toEqual([false, true]);
  });

  it("выключатель можно переключить и после сбоя чтения: запись начнёт состояние заново", async () => {
    const { engine, io, saves } = engineOf({ seen: ["H-M1-01"] });
    io.failLoad = true;
    await engine.load();
    await engine.setEnabled(false);
    expect(saves.at(-1)).toEqual({ enabled: false });
    expect(engine.enabled).toBe(false);
  });
});

describe("HelperEngine: туры и «Вернуть скрытые»", () => {
  it("пройденный тур записывается и закрывает подсказки, которые он объяснил", async () => {
    const { engine, io } = engineOf({ seen: ["H-M1-01"] });
    await engine.load();
    engine.finishTour(tourById("T-M1-01")!);
    await engine.flush();
    const d = parse(io.stored);
    expect(d.tours.has("T-M1-01")).toBe(true);
    expect(d.done.has("H-M1-02")).toBe(true);
    expect(d.done.has("H-M1-03")).toBe(true);
    expect(d.done.has("D-M1-01")).toBe(false);
  });

  it("приглашение в тур считается принятым, когда тур запущен", async () => {
    const { engine, io } = engineOf();
    await engine.load();
    engine.startedTour();
    await engine.flush();
    expect(parse(io.stored).done.has("H-M1-01")).toBe(true);
    expect(parse(io.stored).tours.has("T-M1-01")).toBe(false);
  });

  it("«Вернуть скрытые подсказки»: отметки сняты, подсказки снова выбираются", async () => {
    const { engine, io } = engineOf({
      enabled: true,
      seen: ["H-M1-02"],
      done: ["H-M1-03"],
      muted: ["D-M1-01:1"],
      tours: ["T-M1-01"],
    });
    await engine.load();
    const c = cand("H-M1-02");
    engine.markShown(c);
    expect(engine.pick([c], false, false)).toBeNull();
    await engine.resetHidden();
    expect(engine.pick([c], false, false)?.key).toBe("H-M1-02");
    const d = parse(io.stored);
    expect([d.seen.size, d.done.size, d.muted.size]).toEqual([0, 0, 0]);
    expect(d.tours.has("T-M1-01")).toBe(true);
    expect(d.enabled).toBe(true);
  });
});

describe("каталог и движок", () => {
  it("у каждой подсказки и напоминания каталога есть вид, по которому движок считает бюджет", () => {
    for (const h of HINTS) expect(["hint", "detector"]).toContain(h.kind);
  });
});
