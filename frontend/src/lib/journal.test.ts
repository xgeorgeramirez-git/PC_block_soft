import { describe, expect, it } from "vitest";
import { isViolation, startOfDayMs } from "./journal";

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
});
