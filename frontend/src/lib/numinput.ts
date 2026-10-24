// Числовые поля (FB-1, D-45): только цифры, значение само держится в пределах.

/** Текст поля → число 0..max: всё, кроме цифр, отбрасывается; пусто — 0; больше max — max. */
export function clampDigits(raw: string, max: number): number {
  const digits = raw.replace(/\D/g, "");
  if (digits === "") {
    return 0;
  }
  const n = Number(digits.slice(-6));
  return Math.min(max, Math.max(0, n));
}
