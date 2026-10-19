// Недельные лимиты ребёнка: таблица 7 дней × «Общее» (FR-16). Чистая логика без экрана.
import type { WeekdayLimit } from "../types";
import { weekdayName } from "./format";

/** Сутки в минутах: больше лимит поставить нельзя. */
export const MAX_LIMIT_MIN = 24 * 60;
export const OVER_DAY_TEXT = "Не больше 24 ч";
export const WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const;
export const WORKDAYS = [1, 2, 3, 4, 5];

/** Шаблон мастера: будни и выходные (по умолчанию «будни 2 ч, выходные 4 ч»). */
export function templateLimits(workdayMin = 120, weekendMin = 240): WeekdayLimit[] {
  return WEEKDAYS.map((weekday) => ({
    weekday,
    totalMin: weekday <= 5 ? workdayMin : weekendMin,
    unlimited: false,
  }));
}

export function splitMinutes(total: number): { h: number; m: number } {
  const t = Math.max(0, Math.floor(total));
  return { h: Math.floor(t / 60), m: t % 60 };
}

function whole(n: number): number {
  return Number.isFinite(n) ? Math.max(0, Math.floor(n)) : 0;
}

/** Часы и минуты полей → минуты. Лишние минуты переходят в часы, отрицательное и дробное отсекаются. */
export function joinMinutes(h: number, m: number): number {
  return whole(h) * 60 + whole(m);
}

/** Ровно семь дней по порядку; чего нет в ответе - берётся из шаблона. У «без лимита» минуты 0. */
export function normalizeLimits(limits: WeekdayLimit[] | null | undefined): WeekdayLimit[] {
  const byDay = new Map<number, WeekdayLimit>();
  for (const l of limits ?? []) {
    byDay.set(l.weekday, l);
  }
  return templateLimits().map((fallback) => {
    const got = byDay.get(fallback.weekday);
    if (!got) {
      return fallback;
    }
    return got.unlimited
      ? { weekday: got.weekday, totalMin: 0, unlimited: true }
      : { weekday: got.weekday, totalMin: got.totalMin, unlimited: false };
  });
}

/** Новый список, где у дня изменены поля. */
export function setDay(
  limits: WeekdayLimit[],
  weekday: number,
  change: Partial<Pick<WeekdayLimit, "totalMin" | "unlimited">>,
): WeekdayLimit[] {
  return limits.map((l) => (l.weekday === weekday ? { ...l, ...change } : l));
}

/**
 * Переключатель «Без лимита». Минуты при включении помнятся и возвращаются при выключении,
 * чтобы случайный щелчок не обнулил день. Если помнить нечего (0 минут), берётся значение
 * шаблона: иначе «снял галочку» значило бы «компьютером нельзя».
 */
export function toggleUnlimited(limits: WeekdayLimit[], weekday: number, unlimited: boolean): WeekdayLimit[] {
  return limits.map((l) => {
    if (l.weekday !== weekday) {
      return l;
    }
    if (unlimited) {
      return { ...l, unlimited: true };
    }
    const fallback = templateLimits()[weekday - 1]?.totalMin ?? 120;
    return { ...l, unlimited: false, totalMin: l.totalMin > 0 ? l.totalMin : fallback };
  });
}

/** Лимиты в том виде, в каком их ждёт программа: у «без лимита» минуты 0. */
export function wireLimits(limits: WeekdayLimit[]): WeekdayLimit[] {
  return limits.map((l) => ({ weekday: l.weekday, totalMin: l.unlimited ? 0 : l.totalMin, unlimited: l.unlimited }));
}

/** Переносит лимит дня from на дни to («Как в понедельник: на все будни»). */
export function copyDay(limits: WeekdayLimit[], from: number, to: number[]): WeekdayLimit[] {
  const src = limits.find((l) => l.weekday === from);
  if (!src) {
    return limits;
  }
  return limits.map((l) =>
    to.includes(l.weekday) ? { weekday: l.weekday, totalMin: src.totalMin, unlimited: src.unlimited } : l,
  );
}

/** Одинаковы ли списки. У «без лимита» минуты не сравниваются. */
export function limitsEqual(a: WeekdayLimit[], b: WeekdayLimit[]): boolean {
  if (a.length !== b.length) {
    return false;
  }
  return a.every((x, i) => {
    const y = b[i];
    return x.weekday === y.weekday && x.unlimited === y.unlimited && (x.unlimited || x.totalMin === y.totalMin);
  });
}

/** Ошибки по дням недели (ключ - номер дня). Пусто - можно сохранять. */
export function validateLimits(limits: WeekdayLimit[]): Record<number, string> {
  const errors: Record<number, string> = {};
  for (const l of limits) {
    if (!l.unlimited && l.totalMin > MAX_LIMIT_MIN) {
      errors[l.weekday] = OVER_DAY_TEXT;
    }
  }
  return errors;
}

/** Название дня для подписей полей. */
export function dayLabel(weekday: number): string {
  return weekdayName(weekday).toLowerCase();
}
