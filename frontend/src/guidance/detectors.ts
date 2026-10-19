// Детекторы рискованных ситуаций (ТЗ 8.4 п.2). Смотрят только явные признаки: «Без лимита»
// во все семь дней недели и статус «администратор» у детской учётки. Ничего не угадывают.
import { getChildRules, getToday } from "../api";
import { normalizeLimits } from "../lib/limits";
import type { TodayCard, WeekdayLimit } from "../types";

export interface Risk {
  entryId: "D-M1-01" | "D-M1-02";
  childId: number;
  name: string;
}

const NOT_CHECKED = ["notChild", "safe"];

/** «Без лимита» во все семь дней. Неполный или пустой список - не признак: правила могли не прочитаться. */
function allUnlimited(limits: WeekdayLimit[] | undefined): boolean {
  if (!limits || limits.length < 7) return false;
  return normalizeLimits(limits).every((l) => l.unlimited);
}

/** Какие напоминания нужны по карточкам и правилам детей. На ребёнка - не больше одного. */
export function findRisks(cards: readonly TodayCard[], limitsByChild: Readonly<Record<number, WeekdayLimit[]>>): Risk[] {
  const out: Risk[] = [];
  for (const card of cards) {
    const { status } = card.status;
    if (NOT_CHECKED.includes(status)) continue;
    if (status === "admin") {
      out.push({ entryId: "D-M1-02", childId: card.childId, name: card.name });
    } else if (allUnlimited(limitsByChild[card.childId])) {
      out.push({ entryId: "D-M1-01", childId: card.childId, name: card.name });
    }
  }
  return out;
}

/**
 * Читает у программы карточки и правила детей и находит риски. Не прочитался список детей -
 * ошибка наверх; не прочитались правила одного ребёнка - он пропущен, остальные проверены.
 */
export async function loadRisks(): Promise<Risk[]> {
  const { children } = await getToday();
  const limitsByChild: Record<number, WeekdayLimit[]> = {};
  await Promise.all(
    children
      .filter((c) => !NOT_CHECKED.includes(c.status.status) && c.status.status !== "admin")
      .map(async (c) => {
        try {
          limitsByChild[c.childId] = (await getChildRules(c.childId)).limits;
        } catch {
          // Нет данных - нет напоминания: промолчать лучше, чем сказать неправду.
        }
      }),
  );
  return findRisks(children, limitsByChild);
}
