// Журнал: что считается нарушением, фильтр «Что», периоды и откуда считать «сегодня».

import type { JournalGroup } from "../types";

/** Виды событий (R22), которые родитель видит как нарушения: обход защиты, попытка открыть
 *  запрещённую программу вручную (R13) и запрещённый сайт или сайт для взрослых (R14,
 *  site_forbidden_try). Не нарушение: закрытие программ по лимиту игр или в режиме «только учёба»,
 *  запрещённая, запустившаяся сама (program_auto_closed, D-65), закрытие сайта по лимиту видео
 *  (site_closed_video) или в «только учёбе» (site_closed_study_only), нечитаемый адрес (site_unread),
 *  правки списков сайтов (site_added, site_moved, site_removed), переключатель «18+»
 *  (site_adult_filter) и восстановление правил Brave (brave_restored). */
export const VIOLATION_KINDS = ["bypass", "program_forbidden_try", "site_forbidden_try"] as const;

export function isViolation(kind: string): boolean {
  return (VIOLATION_KINDS as readonly string[]).includes(kind);
}

/** Начало местных суток для момента ms (UTC мс). */
export function startOfDayMs(ms: number): number {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/** Фильтр «Что» журнала (8.3.7 п.2, RM5-5): коды групп службы (internal/ipc JournalGroup*) и подписи.
 *  Какие виды входят в группу, знает служба; окно передаёт только код. */
export const JOURNAL_GROUPS: readonly { id: JournalGroup; label: string }[] = [
  { id: "", label: "Все события" },
  { id: "violations", label: "Нарушения" },
  { id: "time", label: "Время и копилка" },
  { id: "programs", label: "Программы и сайты" },
  { id: "settings", label: "Пароль и настройки" },
  { id: "system", label: "Работа программы" },
];

/** Период журнала (FR-24, D-47): сегодня, за месяц, всё. */
export type JournalPeriod = "today" | "month" | "all";

export const JOURNAL_PERIODS: readonly { id: JournalPeriod; label: string }[] = [
  { id: "today", label: "Сегодня" },
  { id: "month", label: "За месяц" },
  { id: "all", label: "Всё" },
];

/** Начало периода журнала по местному времени (fromMs запроса): «Сегодня» - полночь, «За месяц» -
 *  1-е число текущего месяца, «Всё» - 0 (без границы). */
export function journalPeriodStartMs(period: JournalPeriod, now: number): number {
  if (period === "all") return 0;
  if (period === "today") return startOfDayMs(now);
  const d = new Date(now);
  d.setDate(1);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}
