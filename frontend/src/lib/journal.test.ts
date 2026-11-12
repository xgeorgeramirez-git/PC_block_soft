import { describe, expect, it } from "vitest";
import { JOURNAL_GROUPS, VIOLATION_KINDS, isViolation, journalPeriodStartMs, startOfDayMs } from "./journal";
import { MOCK_JOURNAL_GROUPS } from "../__mocks__/backend";

describe("журнал", () => {
  // Ловит: «Нарушения сегодня» считает доп. время или снятие блокировки нарушением.
  it("нарушение - попытка обойти защиту", () => {
    expect(isViolation("bypass")).toBe(true);
    expect(isViolation("program_forbidden_try")).toBe(true);
    expect(isViolation("program_closed_games")).toBe(false);
    expect(isViolation("program_auto_closed")).toBe(false); // запрещённая запустилась сама (D-65)
    expect(isViolation("program_closed_study_only")).toBe(false);
    expect(isViolation("program_pending")).toBe(false);
    expect(isViolation("program_decided")).toBe(false);
    expect(isViolation("extra_time")).toBe(false);
    expect(isViolation("block_cleared")).toBe(false);
    expect(isViolation("")).toBe(false);
  });

  // Ловит: «Нарушения сегодня» молчит о попытках открыть запрещённый сайт или считает закрытие по лимиту видео.
  it("сайты: нарушение - только попытка открыть запрещённый сайт", () => {
    expect(isViolation("site_forbidden_try")).toBe(true);
    for (const kind of [
      "site_closed_video",
      "site_closed_study_only",
      "site_unread",
      "site_added",
      "site_moved",
      "site_removed",
      "site_adult_filter",
      "brave_restored",
    ]) {
      expect(isViolation(kind), kind).toBe(false);
    }
  });

  it("начало суток - местная полночь", () => {
    const noon = new Date(2026, 9, 19, 12, 30, 15, 500);
    const start = new Date(startOfDayMs(noon.getTime()));
    expect(start.getFullYear()).toBe(2026);
    expect(start.getMonth()).toBe(9);
    expect(start.getDate()).toBe(19);
    expect(start.getHours()).toBe(0);
    expect(start.getMinutes()).toBe(0);
    expect(start.getMilliseconds()).toBe(0);
  });

  it("у границы суток момент 00:00 остаётся своим днём", () => {
    const midnight = new Date(2026, 9, 19, 0, 0, 0, 0).getTime();
    expect(startOfDayMs(midnight)).toBe(midnight);
    expect(startOfDayMs(midnight - 1)).toBe(new Date(2026, 9, 18, 0, 0, 0, 0).getTime());
  });

  // Ловит (8.3.7 п.2, RM5-5): в «Что» пропала группа, подпись разошлась с ТЗ или код не тот, что ждёт
  // служба (неизвестная группа - отказ «Не удалось разобрать запрос»).
  it("фильтр «Что»: шесть пунктов ТЗ, коды групп службы", () => {
    expect(JOURNAL_GROUPS.map((g) => g.label)).toEqual([
      "Все события",
      "Нарушения",
      "Время и копилка",
      "Программы и сайты",
      "Пароль и настройки",
      "Работа программы",
    ]);
    expect(JOURNAL_GROUPS.map((g) => g.id)).toEqual(["", "violations", "time", "programs", "settings", "system"]);
    expect(Object.keys(MOCK_JOURNAL_GROUPS).sort()).toEqual(["programs", "settings", "system", "time", "violations"]);
    // «Нарушения сегодня» на главной и группа «Нарушения» журнала считают одно и то же.
    expect([...MOCK_JOURNAL_GROUPS.violations]).toEqual([...VIOLATION_KINDS]);
  });

  it("начало периода журнала: полночь, 1-е число месяца, «Всё» без границы", () => {
    const now = new Date(2026, 10, 12, 15, 40, 0).getTime();
    expect(journalPeriodStartMs("today", now)).toBe(new Date(2026, 10, 12, 0, 0, 0, 0).getTime());
    expect(journalPeriodStartMs("month", now)).toBe(new Date(2026, 10, 1, 0, 0, 0, 0).getTime());
    expect(journalPeriodStartMs("all", now)).toBe(0);
    // 1-е число в 00:00 - уже этот месяц.
    const first = new Date(2026, 10, 1, 0, 0, 0, 0).getTime();
    expect(journalPeriodStartMs("month", first)).toBe(first);
  });
});
