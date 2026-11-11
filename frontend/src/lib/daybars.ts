// «По дням» (8.2.6 п.4, D-69, D-78): шкала и размеры столбиков. Подписи и подсказки столбиков собирает
// служба (RM5-3), числа итога - те же, что в «Итогах» (RM5-4); здесь только геометрия графика.
import type { DayBar, MyTodayReply } from "../types";
import { formatDuration, formatLimit } from "./format";
import { periodOf } from "./mytoday";

/** Шаги делений оси, секунды: 10 мин … 200 ч. */
export const STEPS = [600, 900, 1800, 3600, 7200, 10800, 14400, 21600, 36000, 72000, 180000, 360000, 720000];

/** Делений над нулём не больше четырёх: подписи оси не теснятся. */
const MAX_TICKS = 4;

/**
 * Шкала: потолок оси и деления (секунды). Шаг - наименьший из STEPS, при котором ceil(max/шаг) ≤ 4;
 * max - наибольшее totalS + studyS; все нули - {maxS: 3600, ticks: [0, 1800, 3600]}.
 */
export function barScale(bars: DayBar[]): { maxS: number; ticks: number[] } {
  const max = bars.reduce((m, b) => Math.max(m, Math.max(0, b.totalS) + Math.max(0, b.studyS)), 0);
  if (max <= 0) return { maxS: 3600, ticks: [0, 1800, 3600] };
  const last = STEPS[STEPS.length - 1];
  const step = STEPS.find((s) => Math.ceil(max / s) <= MAX_TICKS) ?? Math.ceil(max / MAX_TICKS / last) * last;
  const count = Math.ceil(max / step);
  const ticks: number[] = [];
  for (let i = 0; i <= count; i++) ticks.push(i * step);
  return { maxS: count * step, ticks };
}

/** Подпись деления: 0 → «0», < 1 ч → «30 мин», целые часы → «2 ч», иначе «1 ч 30 мин». */
export function axisLabel(seconds: number): string {
  if (seconds <= 0) return "0";
  return formatLimit(Math.round(seconds / 60));
}

/** Процент высоты с точностью 0,1; ненулевой сегмент - не меньше 1 % (виден на графике). */
function pct(seconds: number, maxS: number): number {
  if (seconds <= 0 || maxS <= 0) return 0;
  const v = Math.round((seconds / maxS) * 1000) / 10;
  return Math.min(100, Math.max(1, v));
}

/** Высоты сегментов в процентах высоты графика (0…100, до 0,1). empty - за день (месяц) ни секунды. */
export function barHeights(bar: DayBar, maxS: number): { totalPct: number; studyPct: number; empty: boolean } {
  return {
    totalPct: pct(bar.totalS, maxS),
    studyPct: pct(bar.studyS, maxS),
    empty: Math.max(0, bar.totalS) + Math.max(0, bar.studyS) === 0,
  };
}

/**
 * Показывать подпись под столбиком: до 12 столбиков - все; иначе по дням - каждая неделя, по месяцам -
 * каждый квартал. long - подписи месяцев с годом («ноя 2025»): они шире столбика, поэтому уже с 7
 * столбиков - каждый третий, иначе соседние подписи налезают.
 */
export function showLabel(index: number, count: number, by: "day" | "month", long = false): boolean {
  if (long && by === "month" && count > 6) return index % 3 === 0;
  if (count <= 12) return true;
  return index % (by === "day" ? 7 : 3) === 0;
}

/**
 * Выравнивание подсказки (Ruling T5-1): подсказка шире столбика, поэтому у левой трети графика она
 * начинается от столбика вправо, у правой трети - заканчивается у столбика, в середине - по центру.
 * Так подсказка не выходит за край области графика и не даёт прокрутки вбок. Один столбик - по центру.
 */
export function tipAlign(index: number, count: number): "start" | "center" | "end" {
  if (count <= 1) return "center";
  const at = (index + 0.5) / count;
  if (at < 1 / 3) return "start";
  if (at > 2 / 3) return "end";
  return "center";
}

/**
 * Итоговая подпись графика для экранного диктора: «По дням за неделю: 12 ч 33 мин в счёт лимита,
 * учёба 1 ч 35 мин». Числа - те же, что в «Итогах» (сумма столбиков равна «Итогам», RM5-4).
 */
export function barsSummary(reply: MyTodayReply): string {
  const head = reply.barsBy === "month" ? "По месяцам" : "По дням";
  return `${head} ${periodOf(reply.period).sum}: ${formatDuration(reply.usedS)} в счёт лимита, учёба ${formatDuration(reply.studyS)}`;
}
