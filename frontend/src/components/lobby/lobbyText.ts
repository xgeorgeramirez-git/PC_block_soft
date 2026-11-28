// Тексты блока «Сегодня» окна ребёнка «Игровое лобби» (ТЗ 8.2.6 п. 1, макет plans/design-0.8.4/child.py).
// Ребёнку на «ты». Минуты остатка — вверх до целой (remainMinutes): число, шкала и талисман не расходятся
// со значком у часов.
import { formatDuration } from "../../lib/format";
import { hasVideoLimit } from "../../lib/mytoday";
import { remainMinutes, remainShare } from "../../lib/remain";
import type { ChildStatus } from "../../types";

const WEEKDAYS = ["Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"];
const MONTHS = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

/** Плитка «Игры» или «Видео и соцсети»: крупное число, доля полоски (null — полоски нет), подпись. */
export interface Tile {
  big: string;
  share: number | null;
  /** Строка на месте полоски, когда полоски нет (своего лимита нет). */
  free?: string;
  sub: string;
}

/** «Привет, Миша!»; без имени — «Привет!». */
export function greeting(name: string): string {
  const n = name.trim();
  return n ? `Привет, ${n}!` : "Привет!";
}

/** «Пятница, 27 ноября» по местному времени компьютера. */
export function dateLine(d: Date): string {
  return `${WEEKDAYS[d.getDay()]}, ${d.getDate()} ${MONTHS[d.getMonth()]}`;
}

/** Родительный падеж после «из»: 1, 21, 31… — «часа», «минуты»; остальные — «часов», «минут». */
function genitive(n: number, one: string, many: string): string {
  return n % 10 === 1 && n % 100 !== 11 ? one : many;
}

/** «из 2 часов», «из 1 часа», «из 30 минут»; не целые часы — «из 1 ч 30 мин». */
export function ofTime(minutes: number): string {
  const m = Math.max(0, Math.round(minutes));
  if (m >= 60 && m % 60 === 0) {
    const h = m / 60;
    return `из ${h} ${genitive(h, "часа", "часов")}`;
  }
  if (m < 60) return `из ${m} ${genitive(m, "минуты", "минут")}`;
  return `из ${formatDuration(m * 60)}`;
}

/** Время считается: нет экрана «время вышло» и блокировки (перерыв на отдых время не отнимает). */
function timeOpen(s: ChildStatus): boolean {
  return s.lock === "none" || s.lock === "rest";
}

function noRules(s: ChildStatus): boolean {
  return s.status === "admin" || s.status === "safe" || s.status === "notChild";
}

function forbidden(s: ChildStatus): boolean {
  return s.lock === "forbidden" || s.status === "forbidden";
}

/** Минуты остатка на экране: экран «время вышло» — 0 при любых секундах в ответе. */
function leftMinutes(s: ChildStatus): number {
  return timeOpen(s) ? remainMinutes(s.remainingS) : 0;
}

/** Крупно в главном блоке: «1 ч 25 мин», «Без лимита», «Пауза до 18:30», «Сейчас нельзя». */
export function remainBig(s: ChildStatus): string {
  if (forbidden(s)) return "Сейчас нельзя";
  if (s.status === "paused" && s.pauseUntil) return `Пауза до ${s.pauseUntil}`;
  if (s.unlimited || noRules(s)) return "Без лимита";
  return formatDuration(leftMinutes(s) * 60);
}

/**
 * Справа в главном блоке: «потратил 35 мин» и «из 2 часов». Потрачено — usedS той же функцией, что «Итоги»
 * этого окна, окно родителя и телефон (PL-92): не «лимит минус остаток», который врёт, когда лимит
 * уменьшили ниже потраченного, при лимите 0 и с доп. временем. Минуты вниз, остаток вверх — сумма сходится.
 */
export function spentLines(s: ChildStatus): string[] {
  if (noRules(s)) return [];
  if (s.unlimited) return [`потратил ${formatDuration(s.usedS)}`];
  return [`потратил ${formatDuration(s.usedS)}`, ofTime(s.limitS / 60)];
}

/**
 * Доля шкалы-энергии (moods.py): остаток / лимит дня; запретные часы, «время вышло» и блокировка — пустая
 * шкала (сейчас взять нечего); без лимита и без ограничений — полная.
 */
export function mainShare(s: ChildStatus): number {
  if (s.unlimited || noRules(s)) return 1;
  if (forbidden(s) || !timeOpen(s)) return 0;
  return remainShare(s.remainingS, s.limitS, false);
}

/**
 * Число делений шкалы: ровно по 15 мин, дробное при лимите не кратном 15 мин (2 ч 10 мин — 8 и две трети,
 * последнее деление короче), чтобы подпись «Одно деление: 15 минут» не врала. Без лимита или лимит меньше
 * получаса — без делений.
 */
export function ticksOf(s: ChildStatus): number {
  if (s.unlimited || noRules(s)) return 0;
  const n = s.limitS / 900;
  return n >= 2 ? n : 0;
}

function tile(s: ChildStatus, limited: boolean, remainingS: number, limitS: number, none: string): Tile {
  if (noRules(s) || !limited) return { big: "Без лимита", share: null, free: "Своего лимита нет", sub: "входит в общее время" };
  if (limitS <= 0) return { big: "0 мин", share: 0, sub: none };
  const left = timeOpen(s) && !s.studyOnly ? remainMinutes(remainingS) : 0;
  return {
    big: formatDuration(left * 60),
    share: left === 0 ? 0 : Math.min(1, Math.max(0, remainingS / limitS)),
    sub: `осталось ${ofTime(limitS / 60)}`,
  };
}

/** Плитка «Игры» (R5). */
export function tileGames(s: ChildStatus): Tile {
  return tile(s, !s.gamesUnlimited, s.gamesRemainingS, s.gamesLimitS, "сегодня игры нельзя");
}

/** Плитка «Видео и соцсети» (R6): лимит — по тому же правилу, что у службы (hasVideoLimit). */
export function tileVideo(s: ChildStatus): Tile {
  return tile(s, hasVideoLimit(s), s.videoRemainingS ?? 0, s.videoLimitS ?? 0, "сегодня видео нельзя");
}

/** Блок «Ночью компьютер спит»: «с 21:00 до 07:00», «Сегодня запрет снят»; запретных часов нет — null. */
export function nightLine(s: ChildStatus): string | null {
  switch (s.forbidState) {
    case "ahead":
    case "active":
      if (s.forbidFrom && s.forbidTo) return `с ${s.forbidFrom} до ${s.forbidTo}`;
      return s.forbidTo ? `до ${s.forbidTo}` : null;
    case "cleared":
      return "Сегодня запрет снят";
    default:
      return null;
  }
}

/** Низ окна: «Нарушений сегодня нет» / «Нарушений сегодня: 2». */
export function violationsLine(n: number): string {
  return n > 0 ? `Нарушений сегодня: ${n}` : "Нарушений сегодня нет";
}
