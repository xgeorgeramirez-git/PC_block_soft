// Журнал: что считается нарушением и откуда считать «сегодня».

/** Виды событий (R22), которые родитель видит как нарушения: обход защиты и попытка открыть
 *  запрещённую программу вручную (R13). Закрытие программ по лимиту игр или в режиме «только
 *  учёба» и запрещённая, запустившаяся сама (program_auto_closed, D-65), — не нарушение. */
export const VIOLATION_KINDS = ["bypass", "program_forbidden_try"] as const;

export function isViolation(kind: string): boolean {
  return (VIOLATION_KINDS as readonly string[]).includes(kind);
}

/** Начало местных суток для момента ms (UTC мс). */
export function startOfDayMs(ms: number): number {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}
