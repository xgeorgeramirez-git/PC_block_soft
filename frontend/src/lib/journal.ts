// Журнал: что считается нарушением и откуда считать «сегодня».

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
