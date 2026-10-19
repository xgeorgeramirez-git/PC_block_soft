// Журнал: что считается нарушением и откуда считать «сегодня».

/** Виды событий (R22), которые родитель видит как нарушения. */
export const VIOLATION_KINDS = ["bypass"] as const;

export function isViolation(kind: string): boolean {
  return (VIOLATION_KINDS as readonly string[]).includes(kind);
}

/** Начало местных суток для момента ms (UTC мс). */
export function startOfDayMs(ms: number): number {
  const d = new Date(ms);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}
