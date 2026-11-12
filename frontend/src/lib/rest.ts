// Вкладка «Перерыв на отдых» (FR-37, R27, 8.3.3 п.7): черновик, пределы полей как CHECK миграции 008 (D-45),
// итог словами. Тексты ошибок те же, что у службы (handlers_0_3_2.go).

import { clampDigits } from "./numinput";
import type { BreakCounts, BreakRules } from "../types";

/** Стандартное сообщение на экране перерыва (служба хранит его как «»). */
export const DEFAULT_BREAK_MESSAGE = "Разомнись, попей воды, посмотри в окно";

/** Сообщение на экране перерыва: не длиннее (символов). */
export const BREAK_MESSAGE_MAX = 120;

/** «Отдых» по умолчанию (FR-37): выключено, 60 / всё, кроме учёбы / 5 / 15 / 15. */
export const DEFAULT_BREAK: BreakRules = {
  enabled: false,
  afterMin: 60,
  counts: "all",
  warnMin: 5,
  restMin: 15,
  selfRestMin: 15,
  message: "",
};

export type BreakNumberField = "afterMin" | "warnMin" | "restMin" | "selfRestMin";

/** Пределы числовых полей (минуты) и текст ошибки вне их. */
export const BREAK_RANGES: Record<BreakNumberField, { min: number; max: number; error: string }> = {
  afterMin: { min: 15, max: 240, error: "Можно от 15 до 240 мин" },
  warnMin: { min: 1, max: 15, error: "Можно от 1 до 15 мин" },
  restMin: { min: 5, max: 60, error: "Можно от 5 до 60 мин" },
  selfRestMin: { min: 5, max: 120, error: "Можно от 5 до 120 мин" },
};

/** Порог «сам отошёл» меньше «Отдыха»: ошибка с числом (B-44), как textBreakSelfRestLess службы. */
export const selfRestLess = (restMin: number) => `Не меньше ${restMin} мин: столько длится отдых`;
export const MESSAGE_LONG = "Не больше 120 символов";

/** Ключ ошибки поля: тот же, что Reply.field службы («break.restMin»). */
export const breakField = (f: BreakNumberField | "message" | "counts") => `break.${f}`;

/** Черновик из правил службы: пустое сообщение показывается стандартным текстом. */
export function breakDraft(b: BreakRules): BreakRules {
  return { ...b, message: b.message.trim() === "" ? DEFAULT_BREAK_MESSAGE : b.message };
}

/** В службу: сообщение без пробелов по краям; пустое и стандартное — «» (стандартное). */
export function wireBreak(d: BreakRules): BreakRules {
  const text = d.message.trim();
  return { ...d, message: text === DEFAULT_BREAK_MESSAGE ? "" : text };
}

/** Одинаковы ли настройки по смыслу (стандартное сообщение и «» — одно и то же). */
export function breakEqual(a: BreakRules, b: BreakRules): boolean {
  const x = wireBreak(a);
  const y = wireBreak(b);
  return (
    x.enabled === y.enabled &&
    x.afterMin === y.afterMin &&
    x.counts === y.counts &&
    x.warnMin === y.warnMin &&
    x.restMin === y.restMin &&
    x.selfRestMin === y.selfRestMin &&
    x.message === y.message
  );
}

/**
 * Число из поля: только цифры, не больше верхнего предела (нижний проверяет validateBreak, иначе
 * не набрать «60» через «6»). «Отдых» выше порога «сам отошёл» поднимает порог вместе с собой.
 */
export function setBreakNumber(d: BreakRules, field: BreakNumberField, raw: string): BreakRules {
  const value = clampDigits(raw, BREAK_RANGES[field].max);
  const next = { ...d, [field]: value };
  if (field === "restMin" && value > next.selfRestMin) next.selfRestMin = Math.min(value, BREAK_RANGES.selfRestMin.max);
  return next;
}

/** Ошибки полей по ключам breakField; пусто — можно сохранять. */
export function validateBreak(d: BreakRules): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of Object.keys(BREAK_RANGES) as BreakNumberField[]) {
    const r = BREAK_RANGES[f];
    if (d[f] < r.min || d[f] > r.max) out[breakField(f)] = r.error;
  }
  if (!out[breakField("selfRestMin")] && !out[breakField("restMin")] && d.selfRestMin < d.restMin) {
    out[breakField("selfRestMin")] = selfRestLess(d.restMin);
  }
  if ([...d.message.trim()].length > BREAK_MESSAGE_MAX) out[breakField("message")] = MESSAGE_LONG;
  return out;
}

/**
 * Переключатель «Перерыв на отдых»: значения остаются; при выключении неверные поля берутся из
 * сохранённых (в недоступном поле ошибку не исправить).
 */
export function toggleBreak(d: BreakRules, on: boolean, saved: BreakRules): BreakRules {
  const next = { ...d, enabled: on };
  if (on) return next;
  const bad = validateBreak(next);
  for (const f of Object.keys(BREAK_RANGES) as BreakNumberField[]) {
    if (bad[breakField(f)]) next[f] = saved[f];
  }
  if (bad[breakField("selfRestMin")] || bad[breakField("restMin")]) {
    next.restMin = saved.restMin;
    next.selfRestMin = saved.selfRestMin;
  }
  if (bad[breakField("message")]) next.message = saved.message;
  return next;
}

/** Итог словами под полями (FR-37, 8.3.3 п.7). */
export function breakSummary(d: BreakRules): string {
  if (!d.enabled) return "Перерыв на отдых выключен";
  const what = d.counts === "games" ? "игр" : "за компьютером";
  // «Сам отошёл» (R27): тишина - нет засчитанных к перерыву секунд, поэтому в режиме «всё, кроме
  // учёбы» учёба тоже тишина, и итог говорит об этом прямо.
  const quiet =
    d.counts === "games"
      ? `не играл ${d.selfRestMin} мин подряд`
      : `${d.selfRestMin} мин подряд не был за компьютером или занимался только учёбой`;
  return (
    `После ${d.afterMin} мин ${what} ребёнок увидит «Пора отдохнуть», через ${d.warnMin} мин компьютер закроется на ${d.restMin} мин. ` +
    `Если сам ${quiet}, это засчитается как отдых`
  );
}

/**
 * Подпись поля «сам отошёл» по режиму «Что считать» (B-45, R27, PL-35): в режиме «Только игры»
 * тишина — не играл (буква FR-37), в режиме «Всё время, кроме учёбы» — не был за компьютером.
 */
export function selfRestLabel(counts: BreakCounts): string {
  return counts === "games" ? "Засчитать как отдых, если не играл подряд" : "Засчитать как отдых, если не был за компьютером подряд";
}

/** Подсказка того же поля: в режиме «всё время» учёба тоже тишина, как в итоге словами (breakSummary). */
export function selfRestHint(counts: BreakCounts): string {
  return counts === "games"
    ? "Если ребёнок сам не играл столько минут подряд, перерыв уже не нужен"
    : "Если ребёнок столько минут подряд не был за компьютером или занимался только учёбой, перерыв уже не нужен";
}
