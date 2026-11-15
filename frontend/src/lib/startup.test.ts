import { describe, expect, it } from "vitest";
import { applyStartupItem, checkedText, isStartupEmpty, startupShortName } from "./startup";
import type { ListStartupReply, StartupItemView } from "../types";

// Вкладка «Автозагрузка» (FR-39, 8.3.11): название строки, переезд строки и подпись проверки.
// Ожидания — литералы.

const item = (over: Partial<StartupItemView>): StartupItemView => ({
  id: 1,
  scope: "own",
  source: "run",
  name: "Discord",
  publisher: "",
  from: "Реестр, при входе",
  decision: "on",
  ...over,
});

describe("startupShortName", () => {
  // Ловит: у задания Планировщика в строке весь путь и SID («\OneDrive Reporting Task-S-1-5-21-…»).
  it.each([
    ["\\OneDrive Reporting Task-S-1-5-21-111-222-333-1001", "OneDrive Reporting Task"],
    ["\\OneDrive Standalone Update Task", "OneDrive Standalone Update Task"],
    ["\\Vendor\\Updater\\Check at logon", "Check at logon"],
    ["\\GoogleUpdateTaskUserS-1-5-21-1-2-3-1001Core", "GoogleUpdateTaskUserS-1-5-21-1-2-3-1001Core"],
    ["\\", "\\"],
  ])("задание %s -> %s", (name, want) => {
    expect(startupShortName(item({ source: "task", name }))).toBe(want);
  });

  // Ловит: путь обрезается и у записи реестра, где «\» - часть названия.
  it("у записей не из Планировщика название как есть", () => {
    expect(startupShortName(item({ source: "run", name: "A\\B-S-1-5-21-1" }))).toBe("A\\B-S-1-5-21-1");
    expect(startupShortName(item({ source: "startup_folder", name: "Steam.lnk" }))).toBe("Steam.lnk");
  });
});

describe("applyStartupItem", () => {
  const data = (): ListStartupReply => ({
    pending: [item({ id: 7, name: "notes", decision: "pending" }), item({ id: 9, scope: "shared", name: "pcbshared", decision: "pending" })],
    own: [item({ id: 1 }), item({ id: 8, decision: "off" })],
    shared: [item({ id: 2, scope: "shared" })],
    checkedAt: 100,
    checking: false,
  });

  // Ловит (Review Focus 4): включённая из «Ждут решения» строка остаётся там или дублируется.
  it("строка из «Ждут решения» переходит в свой блок по номеру, без дубля", () => {
    const next = applyStartupItem(data(), item({ id: 7, name: "notes", decision: "on" }));
    expect(next.pending.map((x) => x.id)).toEqual([9]);
    expect(next.own.map((x) => [x.id, x.decision])).toEqual([
      [1, "on"],
      [7, "on"],
      [8, "off"],
    ]);
    expect(next.shared.map((x) => x.id)).toEqual([2]);
  });

  it("общая запись из «Ждут решения» уходит в «Для всех пользователей»", () => {
    const next = applyStartupItem(data(), item({ id: 9, scope: "shared", decision: "off" }));
    expect(next.pending.map((x) => x.id)).toEqual([7]);
    expect(next.shared.map((x) => [x.id, x.decision])).toEqual([
      [2, "on"],
      [9, "off"],
    ]);
  });

  it("решение в своём блоке меняет строку на месте; прочее не трогает", () => {
    const before = data();
    const next = applyStartupItem(before, item({ id: 1, decision: "off" }));
    expect(next.own.map((x) => [x.id, x.decision])).toEqual([
      [1, "off"],
      [8, "off"],
    ]);
    expect(next.checkedAt).toBe(100);
    expect(before.own[0].decision).toBe("on");
  });
});

describe("isStartupEmpty", () => {
  it("пусто, только когда во всех трёх блоках ничего", () => {
    const empty: ListStartupReply = { pending: [], own: [], shared: [], checkedAt: 0, checking: false };
    expect(isStartupEmpty(empty)).toBe(true);
    expect(isStartupEmpty({ ...empty, pending: [item({})] })).toBe(false);
    expect(isStartupEmpty({ ...empty, own: [item({})] })).toBe(false);
    expect(isStartupEmpty({ ...empty, shared: [item({})] })).toBe(false);
  });
});

describe("checkedText", () => {
  const now = Date.UTC(2026, 10, 11, 15, 50); // 18:50 в Москве
  // Ловит: подпись «Проверено» стоит, пока служба ещё проверяет; время не в поясе окна.
  it("идёт проверка - «Проверяю…»", () => {
    expect(checkedText(Date.UTC(2026, 10, 11, 15, 40), true, now, 180)).toBe("Проверяю…");
    expect(checkedText(0, true, now, 180)).toBe("Проверяю…");
  });
  it("сегодня - «Проверено в 18:40»", () => {
    expect(checkedText(Date.UTC(2026, 10, 11, 15, 40), false, now, 180)).toBe("Проверено в 18:40");
    expect(checkedText(Date.UTC(2026, 10, 11, 6, 5), false, now, 0)).toBe("Проверено в 06:05");
  });
  it("не сегодня - с датой", () => {
    expect(checkedText(Date.UTC(2026, 10, 10, 15, 40), false, now, 180)).toBe("Проверено 10 ноября в 18:40");
    // 21:00 UTC 10 ноября - уже 11-е в Москве: «сегодня».
    expect(checkedText(Date.UTC(2026, 10, 10, 21, 0), false, now, 180)).toBe("Проверено в 00:00");
  });
  it("проверки ещё не было - без подписи", () => {
    expect(checkedText(0, false, now, 180)).toBe("");
  });
});
