// Правила пароля родителя (R19): не меньше 6 символов, задаётся дважды.

export const MIN_PASSWORD = 6;
export const TOO_SHORT_TEXT = "Не меньше 6 символов";
export const MISMATCH_TEXT = "Пароли не совпадают";

/** Ошибка нового пароля или null. Длина считается в символах, не в байтах. */
export function validateNewPassword(password: string): string | null {
  return [...password].length < MIN_PASSWORD ? TOO_SHORT_TEXT : null;
}

export interface PasswordPairErrors {
  password?: string;
  again?: string;
}

/** Ошибки пары полей: длина - у первого, несовпадение - у второго. Сначала длина. */
export function validatePasswordPair(password: string, again: string): PasswordPairErrors {
  const tooShort = validateNewPassword(password);
  if (tooShort) {
    return { password: tooShort };
  }
  if (password !== again) {
    return { again: MISMATCH_TEXT };
  }
  return {};
}
