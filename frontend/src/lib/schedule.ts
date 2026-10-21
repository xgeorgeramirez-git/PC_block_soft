// Запретные интервалы расписания (R7, FR-17): клиентская логика для экрана «Запретные
// часы». Служба нормализует интервалы при сохранении (один источник истины), здесь —
// разбор полей, проверка и построение полосы суток для наглядности.
import type { Interval } from "../types";
import { formatHour } from "./format";

export const WEEKDAYS = [1, 2, 3, 4, 5, 6, 7] as const;
export const DAY_MINUTES = 24 * 60;
export const MAX_PER_DAY = 6;

/** «21:00» для подписи интервала. */
export function clockText(minutesOfDay: number): string {
  return formatHour(minutesOfDay);
}

/** Разбирает поле времени «21:00» или «9:5» в минуты от полуночи; мусор → null. */
export function parseClock(text: string): number | null {
  const m = /^(\d{1,2}):(\d{1,2})$/.exec(text.trim());
  if (!m) {
    return null;
  }
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (h > 23 || min > 59) {
    return null;
  }
  return h * 60 + min;
}

/** Интервал пересекает полночь (конец не позже начала): принадлежит дню своего начала. */
export function crossesMidnight(iv: Interval): boolean {
  return iv.endMin <= iv.startMin;
}

/** Интервалы одного дня недели, по порядку начала. */
export function intervalsOfDay(all: Interval[], weekday: number): Interval[] {
  return all.filter((iv) => iv.weekday === weekday).sort((a, b) => a.startMin - b.startMin);
}

/** Текст интервала «с 21:00 до 07:00». */
export function intervalText(iv: Interval): string {
  return `с ${clockText(iv.startMin)} до ${clockText(iv.endMin)}`;
}

/** Ошибка интервала у поля или пустая строка. Начало равно концу — пустой интервал. */
export function intervalError(startMin: number, endMin: number): string {
  if (startMin === endMin) {
    return "Начало и конец совпадают";
  }
  return "";
}

/** Заменяет все интервалы одного дня новым списком, сохраняя остальные дни. */
export function setDayIntervals(all: Interval[], weekday: number, dayIntervals: Interval[]): Interval[] {
  const rest = all.filter((iv) => iv.weekday !== weekday);
  return [...rest, ...dayIntervals].sort((a, b) => a.weekday - b.weekday || a.startMin - b.startMin);
}

/** Добавляет интервал в день. */
export function addInterval(all: Interval[], weekday: number, startMin: number, endMin: number): Interval[] {
  return setDayIntervals(all, weekday, [...intervalsOfDay(all, weekday), { weekday, startMin, endMin }]);
}

/** Удаляет интервал дня по индексу среди интервалов этого дня. */
export function removeInterval(all: Interval[], weekday: number, index: number): Interval[] {
  const day = intervalsOfDay(all, weekday).filter((_, i) => i !== index);
  return setDayIntervals(all, weekday, day);
}

/** Копирует интервалы дня from на дни to («Скопировать на другие дни»). */
export function copyDayIntervals(all: Interval[], from: number, to: number[]): Interval[] {
  const src = intervalsOfDay(all, from);
  let out = all;
  for (const wd of to) {
    out = setDayIntervals(
      out,
      wd,
      src.map((iv) => ({ weekday: wd, startMin: iv.startMin, endMin: iv.endMin })),
    );
  }
  return out;
}

/** Одинаковы ли списки интервалов (для «есть несохранённые изменения»). */
export function schedulesEqual(a: Interval[], b: Interval[]): boolean {
  const key = (list: Interval[]) =>
    list
      .map((iv) => `${iv.weekday}:${iv.startMin}-${iv.endMin}`)
      .sort()
      .join("|");
  return key(a) === key(b);
}

/** Отрезок полосы суток в долях [0..1] (для DayStrip). carry — перенос с прошлого дня. */
export interface StripSegment {
  from: number;
  to: number;
  carry: boolean;
}

/**
 * Сегменты полосы суток 0..24 для дня weekday: интервалы этого дня (через полночь
 * обрезаются до 24:00) плюс «хвост» интервала предыдущего дня, перешедший через полночь
 * (с подписью «с пятницы»). Доли от 0 до 1.
 */
export function dayStripSegments(all: Interval[], weekday: number): StripSegment[] {
  const out: StripSegment[] = [];
  for (const iv of intervalsOfDay(all, weekday)) {
    const end = crossesMidnight(iv) ? DAY_MINUTES : iv.endMin;
    out.push({ from: iv.startMin / DAY_MINUTES, to: end / DAY_MINUTES, carry: false });
  }
  const prev = ((weekday + 5) % 7) + 1; // предыдущий день недели
  for (const iv of intervalsOfDay(all, prev)) {
    if (crossesMidnight(iv) && iv.endMin > 0) {
      out.push({ from: 0, to: iv.endMin / DAY_MINUTES, carry: true });
    }
  }
  return out.sort((a, b) => a.from - b.from);
}

/** Название предыдущего дня для подписи переноса «с пятницы». */
export function prevDayName(weekday: number): string {
  const prev = ((weekday + 5) % 7) + 1;
  return ["понедельника", "вторника", "среды", "четверга", "пятницы", "субботы", "воскресенья"][prev - 1];
}
