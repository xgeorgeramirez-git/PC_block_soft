// Ответ ребёнку на промокод: стикер заказчика и надпись (R26, D-83, план M5b T8).
// Наборы и надписи - дословно из ТЗ R26; страж экрана блокировки держит ту же таблицу
// (internal/guard/stickers.go) и сверяет её с этим файлом.

/** Стикеры удачи: верный действующий код. */
export const SUCCESS: readonly string[] = ["017", "033", "044", "049", "050", "077", "093", "096", "112"];

/** Стикеры неудачи: неверный, несуществующий или уже использованный код. */
export const FAIL: readonly string[] = ["023", "028", "048", "069", "082", "084", "095", "110"];

/** Надписи удачи; {m} - минуты кода со словом «минута» в нужной форме («30 минут»). */
export const SUCCESS_TEXTS: readonly string[] = [
  "Ура! +{m}",
  "Есть! Тебе +{m}",
  "Код сработал! +{m}",
  "Отлично, лови +{m}!",
  "Вот это да! +{m}",
  "Супер! Твои +{m}",
];

/** Надписи неудачи. */
export const FAIL_TEXTS: readonly string[] = [
  "Такого кода нет. Проверь буквы и цифры",
  "Хм, код не подходит. Посмотри внимательнее",
  "Не получилось. Проверь, нет ли опечатки",
  "Этот код не работает. Попробуй ещё раз",
  "Ой, мимо! Сверь код с запиской",
  "Код не сработал. Может, его уже использовали?",
];

/** «1 минута», «2 минуты», «5 минут», «11 минут», «21 минута». */
export function minutesPhrase(n: number): string {
  const d = n % 10;
  const dd = n % 100;
  if (d === 1 && dd !== 11) return `${n} минута`;
  if (d >= 2 && d <= 4 && (dd < 12 || dd > 14)) return `${n} минуты`;
  return `${n} минут`;
}

/** Надпись удачи номер i с минутами кода. */
export function successText(i: number, minutes: number): string {
  return SUCCESS_TEXTS[i].replace("{m}", minutesPhrase(minutes));
}

/** Случайный элемент списка, не равный прошлому; в списке из одного - он сам. */
export function pickNext<T>(list: readonly T[], prev: T | undefined, rnd: () => number): T {
  const rest = list.length > 1 ? list.filter((x) => x !== prev) : list;
  const i = Math.min(rest.length - 1, Math.max(0, Math.floor(rnd() * rest.length)));
  return rest[i];
}

/** Что показать ребёнку: номер стикера, надпись, удача ли это. */
export interface StickerAnswer {
  sticker: string;
  text: string;
  ok: boolean;
}

interface Last {
  sticker?: string;
  text?: number;
}

const indices = (n: number): number[] => Array.from({ length: n }, (_, i) => i);

/**
 * Выбор стикера и надписи. Прошлые стикер и надпись помнятся отдельно для удачи и неудачи:
 * два неверных кода подряд - разные стикер и надпись, даже если между ними был верный.
 */
export class StickerPicker {
  private ok: Last = {};
  private bad: Last = {};

  constructor(private readonly rnd: () => number = () => Math.random()) {}

  success(minutes: number): StickerAnswer {
    const sticker = pickNext(SUCCESS, this.ok.sticker, this.rnd);
    const text = pickNext(indices(SUCCESS_TEXTS.length), this.ok.text, this.rnd);
    this.ok = { sticker, text };
    return { sticker, text: successText(text, minutes), ok: true };
  }

  fail(): StickerAnswer {
    const sticker = pickNext(FAIL, this.bad.sticker, this.rnd);
    const text = pickNext(indices(FAIL_TEXTS.length), this.bad.text, this.rnd);
    this.bad = { sticker, text };
    return { sticker, text: FAIL_TEXTS[text], ok: false };
  }

  /** Забыть прошлые ответы (только для тестов). */
  reset(): void {
    this.ok = {};
    this.bad = {};
  }
}

/**
 * Один выбор на окно (уточнение 10 плана M5b): переживает смену вкладки и перерисовку
 * раздела «Бонусы», поэтому следующий ответ не повторяет прошлый.
 */
export const promoPicker = new StickerPicker();
