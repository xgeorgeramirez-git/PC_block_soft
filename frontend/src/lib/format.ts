// Форматирование длительностей, лимитов, дат и времени (ТЗ раздел 4 и 8.1).
// Длительности и лимиты зеркалят internal/core/texts.go, тест-таблицы совпадают с Go-тестами.

/** Длительность в секундах: «40 мин», «1 ч 05 мин». Неполная минута отбрасывается. */
export function formatDuration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds / 60));
  if (total < 60) {
    return `${total} мин`;
  }
  return `${Math.floor(total / 60)} ч ${pad2(total % 60)} мин`;
}

/** Лимит в минутах: «20 мин», «2 ч» для целых часов, иначе «1 ч 30 мин». */
export function formatLimit(minutes: number): string {
  const min = Math.max(0, Math.floor(minutes));
  if (min < 60) {
    return `${min} мин`;
  }
  if (min % 60 === 0) {
    return `${min / 60} ч`;
  }
  return `${Math.floor(min / 60)} ч ${pad2(min % 60)} мин`;
}

/** «Использовано 35 мин из 2 ч» или «Использовано 40 мин, без лимита». */
export function formatUsed(usedS: number, limitS: number, unlimited: boolean): string {
  const used = formatDuration(usedS);
  if (unlimited) {
    return `Использовано ${used}, без лимита`;
  }
  return `Использовано ${used} из ${formatLimit(Math.round(limitS / 60))}`;
}

/** Отсчёт паузы: секунды как «1:00», «0:59». */
export function formatClock(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${pad2(s % 60)}`;
}

const WEEKDAY_SHORT = ["пн", "вт", "ср", "чт", "пт", "сб", "вс"];
const WEEKDAY_NAMES = ["Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"];
const MONTH_GENITIVE = [
  "января",
  "февраля",
  "марта",
  "апреля",
  "мая",
  "июня",
  "июля",
  "августа",
  "сентября",
  "октября",
  "ноября",
  "декабря",
];
const MONTH_SHORT = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];

/** Короткое название дня недели: 1 = «пн» … 7 = «вс». */
export function weekdayShort(weekday: number): string {
  return WEEKDAY_SHORT[weekday - 1] ?? "";
}

/** Полное название дня недели: 1 = «Понедельник» … 7 = «Воскресенье». */
export function weekdayName(weekday: number): string {
  return WEEKDAY_NAMES[weekday - 1] ?? "";
}

/** Сдвигает момент UTC мс на пояс (минуты к востоку) так, что UTC-поля Date дают местное время. */
function shifted(ms: number, tzOffsetMin: number): Date {
  return new Date(ms + tzOffsetMin * 60_000);
}

/** Номер дня недели момента в поясе: 1 = пн … 7 = вс. */
export function weekdayOfMs(ms: number, tzOffsetMin: number): number {
  return ((shifted(ms, tzOffsetMin).getUTCDay() + 6) % 7) + 1;
}

/** Смещение пояса окна в минутах к востоку от UTC (Москва: 180). */
export function localOffsetMin(at: Date = new Date()): number {
  return -at.getTimezoneOffset() || 0;
}

/** Дата словами: «пн, 19 октября». */
export function formatDateLong(ms: number, tzOffsetMin: number): string {
  const d = shifted(ms, tzOffsetMin);
  return `${weekdayShort(weekdayOfMs(ms, tzOffsetMin))}, ${d.getUTCDate()} ${MONTH_GENITIVE[d.getUTCMonth()]}`;
}

/** Минуты от начала суток как часы: «21:00». */
export function formatHour(minutesOfDay: number): string {
  const m = Math.max(0, Math.floor(minutesOfDay));
  return `${pad2(Math.floor(m / 60) % 24)}:${pad2(m % 60)}`;
}

/** Время события журнала в поясе события: «19 окт, 21:14». */
export function formatEventTime(ms: number, tzOffsetMin: number): string {
  const d = shifted(ms, tzOffsetMin);
  return `${d.getUTCDate()} ${MONTH_SHORT[d.getUTCMonth()]}, ${pad2(d.getUTCHours())}:${pad2(d.getUTCMinutes())}`;
}

function pad2(n: number): string {
  return n < 10 ? `0${n}` : `${n}`;
}
