// Остаток времени у родителя (ТЗ 8.0 п. 3, 8.3.2 п. 3, D-109): число, цвет, полоса и строки лимитов.
// Пороги считаются от минут вверх до целой — та же минута, что на значке у часов и в ядре
// (core.RemainingMinutes), поэтому граница 15/16 и 60/61 мин одна и для числа, и для цвета.

export type RemainTone = "green" | "blue" | "red";

/** Минуты остатка для показа и порогов: вверх до целой минуты (как core.RemainingMinutes и значок). */
export function remainMinutes(remainingS: number): number {
  return remainingS > 0 ? Math.ceil(remainingS / 60) : 0;
}

/** Цвет остатка (ТЗ 8.0 п. 3, D-109): без лимита и > 60 мин — green; 16…60 — blue; ≤ 15 — red. */
export function remainTone(remainingS: number, unlimited: boolean): RemainTone {
  if (unlimited) return "green";
  const m = remainMinutes(remainingS);
  if (m > 60) return "green";
  if (m > 15) return "blue";
  return "red";
}

/** «1:25» из минут. */
export function formatHM(minutes: number): string {
  const m = Math.max(0, Math.floor(minutes));
  const mm = m % 60;
  return `${Math.floor(m / 60)}:${mm < 10 ? "0" : ""}${mm}`;
}

/** Доля шкалы 0…1: остаток / лимит дня (без лимита — 1). */
export function remainShare(remainingS: number, limitS: number, unlimited: boolean): number {
  if (unlimited) return 1;
  if (limitS <= 0) return 0;
  return Math.min(1, Math.max(0, remainingS / limitS));
}

/**
 * «Игры 40 из 60 мин» (8.3.2 п. 3): usedS — потрачено сегодня, с (TodayCard.gamesUsedS/videoUsedS —
 * то же число, что gamesSec/videoSec отчёта телефону). Минуты — вниз до целой, как у телефона;
 * больше лимита — как есть («Игры 70 из 60 мин», родитель уменьшил лимит ниже сыгранного).
 * Без отдельного лимита или без лимита на сегодня — «» (строки нет).
 */
export function limitLine(label: string, usedS: number, limitS: number, unlimited: boolean): string {
  if (unlimited || limitS <= 0) return "";
  const limitMin = Math.floor(limitS / 60);
  const used = Math.floor(Math.max(0, usedS) / 60);
  return `${label} ${used} из ${limitMin} мин`;
}

/** Форма слова по числу: «1 программа», «2 программы», «5 программ», «11 программ». */
function plural(n: number, one: string, few: string, many: string): string {
  const m = Math.abs(n) % 100;
  if (m >= 11 && m <= 14) return many;
  if (m % 10 === 1) return one;
  if (m % 10 >= 2 && m % 10 <= 4) return few;
  return many;
}

/** Заголовок блока «Сегодня»: «1 программа ждёт решения», «2 программы ждут решения». */
export function programsWaitText(n: number): string {
  const verb = plural(n, "ждёт", "ждут", "ждут");
  return `${n} ${plural(n, "программа", "программы", "программ")} ${verb} решения`;
}

/**
 * Пояснение блока: названия (служба отдаёт до 3, новые первыми) и что будет, пока родитель не решит.
 * «Roblox Studio и Steam. Пока вы не решите, ребёнок не сможет их запустить.»
 */
export function pendingLead(names: string[], total: number): string {
  const shown = names.filter((n) => n.trim() !== "");
  const rest = Math.max(0, total - shown.length);
  let list = "";
  if (shown.length > 0) {
    const parts = rest > 0 ? [...shown, `ещё ${rest}`] : shown;
    list = parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(", ")} и ${parts[parts.length - 1]}`;
    list += ". ";
  }
  const them = Math.max(total, shown.length) === 1 ? "её" : "их";
  return `${list}Пока вы не решите, ребёнок не сможет ${them} запустить.`;
}

/** Число и единица времени не разрываются переносом: «Осталось 10 мин» → «Осталось 10\u00a0мин». */
export function keepUnits(text: string): string {
  return text.replace(/(\d) (мин|ч|с)(?=$|[\s.,;:)])/g, "$1\u00a0$2");
}
