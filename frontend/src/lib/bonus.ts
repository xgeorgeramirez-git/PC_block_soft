// Копилка и бонусы (FR-22, R25, D-46, D-49): чистые правила показа истории без экрана.
import type { BonusEntry } from "../types";

/** Причина автобонуса, которую пишет служба (R25): «Бонус за свободное время: K мин». */
export const AUTO_BONUS_PREFIX = "Бонус за свободное время";

/** Трата ребёнком из копилки (FR-11): служба пишет эту причину. */
const SPEND_REASON = "взято из копилки";

/** Запись — автоматический бонус за неизрасходованное время (R25). */
export function isAutoBonus(e: BonusEntry): boolean {
  return e.deltaMin > 0 && e.reason.startsWith(AUTO_BONUS_PREFIX);
}

/** Автобонус простыми словами (D-49): «+15 мин за неизрасходованное время прошлого дня». */
export function autoBonusText(e: BonusEntry): string {
  return `+${e.deltaMin} мин за неизрасходованное время прошлого дня`;
}

/** Строка истории копилки: начисление с причиной, трата ребёнка, списание родителем с причиной. */
export function historyText(e: BonusEntry): string {
  if (e.deltaMin > 0) {
    return isAutoBonus(e) ? autoBonusText(e) : `+${e.deltaMin} мин: ${e.reason}`;
  }
  if (!e.reason || e.reason === SPEND_REASON) {
    return `${e.deltaMin} мин: потрачено`;
  }
  return `${e.deltaMin} мин: ${e.reason}`;
}

/** Как считается автобонус, одной фразой для родителя и ребёнка (R25). */
export function autoBonusRule(rate: number): string {
  if (rate <= 0) {
    return "Автобонус выключен в настройках.";
  }
  return `В полночь копилка пополняется: ${rate} мин за каждые полные 10 минут, которые остались неизрасходованными за день.`;
}
