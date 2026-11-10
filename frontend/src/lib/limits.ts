// Недельные лимиты ребёнка: таблица 7 дней × «Общее», «Игры» и «Видео и соцсети» (FR-16, R5, R6).
// Чистая логика без экрана. В таблице у каждого дня всегда заполнены все семь полей; в программу
// уходит wireLimits.
import type { WeekdayLimit } from "../types";
import { weekdayName } from "./format";

/** Сутки в минутах: больше лимит поставить нельзя. */
export const MAX_LIMIT_MIN = 24 * 60;
export const OVER_DAY_TEXT = "Не больше 24 ч";
export const WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const;
export const WORKDAYS = [1, 2, 3, 4, 5];
/** Сколько игр предлагать, когда родитель снял «Без лимита» у дня без минут игр. */
export const GAMES_FALLBACK_MIN = 60;
/** Сколько видео предлагать, когда родитель снял «Без лимита» у дня без минут видео. */
export const VIDEO_FALLBACK_MIN = 60;

/** Столбец таблицы: общее время, игры или видео и соцсети. */
export type LimitColumn = "total" | "games" | "video";

/** Ключ ошибки у поля, как его называет программа: limits.N (общее), games.N (игры), video.N (видео). */
export function limitField(column: LimitColumn, weekday: number): string {
  const prefix = column === "games" ? "games" : column === "video" ? "video" : "limits";
  return `${prefix}.${weekday}`;
}

/**
 * Шаблон мастера: будни и выходные (по умолчанию «будни 2 ч, выходные 4 ч») для общего времени,
 * игры и видео без отдельного лимита.
 */
export function templateLimits(workdayMin = 120, weekendMin = 240): WeekdayLimit[] {
  return WEEKDAYS.map((weekday) => ({
    weekday,
    totalMin: weekday <= 5 ? workdayMin : weekendMin,
    unlimited: false,
    gamesMin: 0,
    gamesUnlimited: true,
    videoMin: 0,
    videoUnlimited: true,
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

/** Игры без лимита: так же, как у программы, нет поля - нет отдельного лимита игр. */
function gamesOff(l: WeekdayLimit): boolean {
  return l.gamesUnlimited ?? l.gamesMin === undefined;
}

/**
 * Видео без лимита: как у службы, нет поля videoUnlimited - видео без отдельного лимита (S-3), даже
 * если videoMin есть. У игр правило другое (gamesOff).
 */
function videoOff(l: WeekdayLimit): boolean {
  return l.videoUnlimited ?? true;
}

/** Поля видео дня в виде для таблицы и программы: у «без лимита» минуты 0. */
function videoFields(l: WeekdayLimit): Pick<WeekdayLimit, "videoMin" | "videoUnlimited"> {
  const videoUnlimited = videoOff(l);
  return { videoMin: videoUnlimited ? 0 : (l.videoMin ?? 0), videoUnlimited };
}

/**
 * Ровно семь дней по порядку; чего нет в ответе - берётся из шаблона. У «без лимита» минуты 0.
 * Нет полей игр - игры без отдельного лимита, нет videoUnlimited - видео без отдельного лимита
 * (как у программы).
 */
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
    const gamesUnlimited = gamesOff(got);
    return {
      weekday: got.weekday,
      totalMin: got.unlimited ? 0 : got.totalMin,
      unlimited: got.unlimited,
      gamesMin: gamesUnlimited ? 0 : (got.gamesMin ?? 0),
      gamesUnlimited,
      ...videoFields(got),
    };
  });
}

/** Новый список, где у дня изменены поля. */
export function setDay(
  limits: WeekdayLimit[],
  weekday: number,
  change: Partial<Pick<WeekdayLimit, "totalMin" | "unlimited" | "gamesMin" | "gamesUnlimited" | "videoMin" | "videoUnlimited">>,
): WeekdayLimit[] {
  return limits.map((l) => (l.weekday === weekday ? { ...l, ...change } : l));
}

/** Минуты дня в столбце. */
export function columnMinutes(l: WeekdayLimit, column: LimitColumn): number {
  if (column === "games") {
    return l.gamesMin ?? 0;
  }
  return column === "video" ? (l.videoMin ?? 0) : l.totalMin;
}

/** «Без лимита» дня в столбце. */
export function columnUnlimited(l: WeekdayLimit, column: LimitColumn): boolean {
  if (column === "games") {
    return gamesOff(l);
  }
  return column === "video" ? videoOff(l) : l.unlimited;
}

/** Новые минуты дня в столбце. */
export function setMinutes(limits: WeekdayLimit[], weekday: number, column: LimitColumn, minutes: number): WeekdayLimit[] {
  if (column === "games") {
    return setDay(limits, weekday, { gamesMin: minutes });
  }
  return setDay(limits, weekday, column === "video" ? { videoMin: minutes } : { totalMin: minutes });
}

/**
 * Переключатель «Без лимита» в столбце. Минуты при включении помнятся и возвращаются при выключении,
 * чтобы случайный щелчок не обнулил день. Если помнить нечего (0 минут), берётся значение шаблона
 * (у игр и видео - 1 ч): иначе «снял галочку» значило бы «компьютером нельзя», «играть нельзя» или
 * «видео нельзя».
 */
export function toggleUnlimited(
  limits: WeekdayLimit[],
  weekday: number,
  unlimited: boolean,
  column: LimitColumn = "total",
): WeekdayLimit[] {
  return limits.map((l) => {
    if (l.weekday !== weekday) {
      return l;
    }
    if (column === "games") {
      if (unlimited) {
        return { ...l, gamesUnlimited: true, gamesMin: l.gamesMin ?? 0 };
      }
      const g = l.gamesMin ?? 0;
      return { ...l, gamesUnlimited: false, gamesMin: g > 0 ? g : GAMES_FALLBACK_MIN };
    }
    if (column === "video") {
      if (unlimited) {
        return { ...l, videoUnlimited: true, videoMin: l.videoMin ?? 0 };
      }
      const v = l.videoMin ?? 0;
      return { ...l, videoUnlimited: false, videoMin: v > 0 ? v : VIDEO_FALLBACK_MIN };
    }
    if (unlimited) {
      return { ...l, unlimited: true };
    }
    const fallback = templateLimits()[weekday - 1]?.totalMin ?? 120;
    return { ...l, unlimited: false, totalMin: l.totalMin > 0 ? l.totalMin : fallback };
  });
}

/**
 * Лимиты в том виде, в каком их ждёт программа: все семь полей каждого дня (правила целиком, RM3-9),
 * у «без лимита» минуты 0. Поля видео уходят всегда: без них программа сняла бы лимит видео.
 */
export function wireLimits(limits: WeekdayLimit[]): WeekdayLimit[] {
  return limits.map((l) => {
    const gamesUnlimited = gamesOff(l);
    return {
      weekday: l.weekday,
      totalMin: l.unlimited ? 0 : l.totalMin,
      unlimited: l.unlimited,
      gamesMin: gamesUnlimited ? 0 : (l.gamesMin ?? 0),
      gamesUnlimited,
      ...videoFields(l),
    };
  });
}

/** Переносит все столбцы дня from на дни to («Как в понедельник: на все будни»). */
export function copyDay(limits: WeekdayLimit[], from: number, to: number[]): WeekdayLimit[] {
  const src = limits.find((l) => l.weekday === from);
  if (!src) {
    return limits;
  }
  return limits.map((l) =>
    to.includes(l.weekday)
      ? {
          weekday: l.weekday,
          totalMin: src.totalMin,
          unlimited: src.unlimited,
          gamesMin: src.gamesMin ?? 0,
          gamesUnlimited: gamesOff(src),
          videoMin: src.videoMin ?? 0,
          videoUnlimited: videoOff(src),
        }
      : l,
  );
}

/** Одинаковы ли списки. У «без лимита» минуты не сравниваются. */
export function limitsEqual(a: WeekdayLimit[], b: WeekdayLimit[]): boolean {
  if (a.length !== b.length) {
    return false;
  }
  return a.every((x, i) => {
    const y = b[i];
    const gx = gamesOff(x);
    const gy = gamesOff(y);
    const vx = videoOff(x);
    const vy = videoOff(y);
    return (
      x.weekday === y.weekday &&
      x.unlimited === y.unlimited &&
      (x.unlimited || x.totalMin === y.totalMin) &&
      gx === gy &&
      (gx || (x.gamesMin ?? 0) === (y.gamesMin ?? 0)) &&
      vx === vy &&
      (vx || (x.videoMin ?? 0) === (y.videoMin ?? 0))
    );
  });
}

/**
 * Ошибки у полей (ключ - limitField: «limits.3», «games.3», «video.3»). Пусто - можно сохранять.
 * Игры и видео больше общего лимита допустимы: общее время закончится раньше.
 */
export function validateLimits(limits: WeekdayLimit[]): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const l of limits) {
    if (!l.unlimited && l.totalMin > MAX_LIMIT_MIN) {
      errors[limitField("total", l.weekday)] = OVER_DAY_TEXT;
    }
    if (!gamesOff(l) && (l.gamesMin ?? 0) > MAX_LIMIT_MIN) {
      errors[limitField("games", l.weekday)] = OVER_DAY_TEXT;
    }
    if (!videoOff(l) && (l.videoMin ?? 0) > MAX_LIMIT_MIN) {
      errors[limitField("video", l.weekday)] = OVER_DAY_TEXT;
    }
  }
  return errors;
}

/** Ошибка программы у поля таблицы («limits.3», «games.3», «video.3»): ключ для errors; иначе null. */
export function limitErrorKey(field: string | undefined): string | null {
  return /^(limits|games|video)\.[1-7]$/.test(field ?? "") ? (field as string) : null;
}

/** Название дня для подписей полей. */
export function dayLabel(weekday: number): string {
  return weekdayName(weekday).toLowerCase();
}
