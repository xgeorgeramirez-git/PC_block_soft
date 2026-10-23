import { describe, expect, it } from "vitest";
import { autoBonusRule, historyText, isAutoBonus } from "./bonus";
import type { BonusEntry } from "../types";

const e = (deltaMin: number, reason: string): BonusEntry => ({ atMs: 0, tzOffsetMin: 180, deltaMin, reason, balanceAfter: 0 });

describe("история копилки", () => {
  // Ловит: списание родителем показано как «потрачено» ребёнком (D-46).
  it("списание родителем показывает причину, трата ребёнка — «потрачено»", () => {
    expect(historyText(e(-45, "Обнулено родителем"))).toBe("-45 мин: Обнулено родителем");
    expect(historyText(e(-15, "Убавлено родителем"))).toBe("-15 мин: Убавлено родителем");
    expect(historyText(e(-30, "взято из копилки"))).toBe("-30 мин: потрачено");
    expect(historyText(e(30, "за уборку"))).toBe("+30 мин: за уборку");
  });
  // Ловит: автобонус показан технической строкой службы (D-49).
  it("автобонус простыми словами", () => {
    const auto = e(15, "Бонус за свободное время: 15 мин");
    expect(isAutoBonus(auto)).toBe(true);
    expect(isAutoBonus(e(15, "за уборку"))).toBe(false);
    expect(historyText(auto)).toBe("+15 мин за неизрасходованное время прошлого дня");
  });
  it("правило автобонуса и выключенный автобонус", () => {
    expect(autoBonusRule(5)).toContain("5 мин за каждые полные 10 минут");
    expect(autoBonusRule(0)).toBe("Автобонус выключен в настройках.");
  });
});
