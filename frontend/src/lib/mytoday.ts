// Тексты окон ребёнка (FR-12, FR-32). Числа и слова приходят из MyToday готовыми: здесь они
// только раскладываются по строкам окна, ничего не пересчитывается (один источник с значком).
import type { ChildStatus, MyRule, MyTodayReply } from "../types";
import { formatDuration, formatLimit, weekdayOfMs, localOffsetMin } from "./format";

const NO_RULES = ["admin", "safe", "notChild"];

/** Строки окна «Теперь время за компьютером считается»: пересказ правил на сегодня. */
export function firstRunLines(reply: MyTodayReply): string[] {
  const s = reply.status;
  if (NO_RULES.includes(s.status)) {
    return ["Сейчас ограничения не действуют. Если это ошибка, скажи родителю."];
  }
  if (s.unlimited) {
    return ["Сегодня без ограничений по времени."];
  }
  if (s.limitS <= 0) {
    return ["Сегодня компьютером пользоваться нельзя."];
  }
  return [`Сегодня можно ${formatLimit(Math.round(s.limitS / 60))}.`, "Нажми на значок у часов: увидишь, сколько осталось."];
}

/**
 * Строки «Сегодня» для «Итогов». «Использовано …» уже есть в строке «Использовано сегодня», а «Сегодня
 * осталось …» (пока время есть) повторяет «Осталось всего» из верхней панели: одно число - одно место.
 */
export function summaryLines(reply: MyTodayReply): string[] {
  const used = `Использовано ${formatDuration(reply.status.usedS)}`;
  const repeatsLeft = reply.status.lock === "none" ? reply.status.summary : used;
  return reply.today.filter((line) => line !== used && line !== repeatsLeft);
}

/** Номер сегодняшнего дня недели для подсветки строки «Моих правил»: 1 = пн … 7 = вс. */
export function todayRuleWeekday(at: Date = new Date()): number {
  return weekdayOfMs(at.getTime(), localOffsetMin(at));
}

/** Строка окна: подпись и значение. */
export interface TodayRow {
  label: string;
  value: string;
}

/** «Видео и соцсети» появятся в M4. */
export const NOT_COUNTED = "Пока не считается";

/** Вкладка «Итогов» (D-58): «Всего» или категория учёта программ. */
export type TotalsTab = "all" | "games" | "study" | "other";

/** Строки-вкладки «Итогов» по порядку (8.2.6 п.3); «Видео и соцсети» - строка без вкладки (M4). */
export const TOTALS_TABS: { id: TotalsTab; label: string }[] = [
  { id: "all", label: "Всего" },
  { id: "games", label: "Игры" },
  { id: "study", label: "Учёба" },
  { id: "other", label: "Прочее" },
];

/** Пустая вкладка. */
export const EMPTY_TAB = "Пока ничего не запускалось";

/** Сумма вкладки за сегодня, секунды: счётчики службы, а не сумма строк (одно число - одно место). */
export function tabSeconds(reply: MyTodayReply, tab: TotalsTab): number {
  switch (tab) {
    case "all":
      return reply.allS;
    case "games":
      return reply.gamesS;
    case "study":
      return reply.studyS;
    case "other":
      return reply.otherS;
  }
}

/** Строка вкладки: название, время и доля от суммы вкладки. */
export interface TabRow {
  key: string;
  name: string;
  time: string;
  share: string;
  /** Строка «Другое»: время без программы, всегда последняя. */
  rest: boolean;
}

/** Время строки: меньше минуты - словами, иначе как везде в окне. */
function rowTime(seconds: number): string {
  return seconds > 0 && seconds < 60 ? "меньше 1 мин" : formatDuration(seconds);
}

/**
 * Строки вкладки (D-58, S39): программы самые долгие сверху, «Другое» (время без программы)
 * последней. «Всего» - все программы, одна программа одной строкой, даже если её секунды в двух
 * категориях. Доля - целые проценты от суммы вкладки, каждая округлена сама: сумма долей может
 * быть 99-101, подгонки нет.
 */
export function tabRows(reply: MyTodayReply, tab: TotalsTab): TabRow[] {
  const merged = new Map<string, { name: string; seconds: number; rest: boolean }>();
  for (const p of reply.programs) {
    if (tab !== "all" && p.category !== tab) continue;
    if (tab === "all" && p.category === "video_social") continue;
    const key = p.path === "" ? "" : p.path;
    const cur = merged.get(key);
    if (cur) cur.seconds += p.seconds;
    else merged.set(key, { name: p.name, seconds: p.seconds, rest: p.path === "" });
  }
  const rows = [...merged.entries()].filter(([, v]) => v.seconds > 0);
  const total = tabSeconds(reply, tab) || rows.reduce((n, [, v]) => n + v.seconds, 0);
  rows.sort(([ka, a], [kb, b]) => {
    if (a.rest !== b.rest) return a.rest ? 1 : -1;
    if (a.seconds !== b.seconds) return b.seconds - a.seconds;
    const na = a.name.toLowerCase();
    const nb = b.name.toLowerCase();
    if (na !== nb) return na < nb ? -1 : 1;
    return ka < kb ? -1 : 1;
  });
  return rows.map(([key, v]) => ({
    key: key === "" ? "rest" : key,
    name: v.name,
    time: rowTime(v.seconds),
    share: `${total > 0 ? Math.round((v.seconds * 100) / total) : 0}%`,
    rest: v.rest,
  }));
}

/** Остаток в минутах тем же правилом, что у значка и табло: вверх (B-5). */
function upToMinute(seconds: number): number {
  return Math.ceil(Math.max(0, seconds) / 60) * 60;
}

/**
 * Строка «Игры» в «Сколько осталось» (R5): остаток игр из ответа службы, как в подсказке значка
 * («Игры: 20 мин»), и лимит игр дня с доп. временем. Время вышло или открыта только учёба - 0.
 */
export function gamesLeftText(s: ChildStatus): string {
  if (s.status === "admin" || s.status === "safe") {
    return "Не действует";
  }
  if (s.gamesUnlimited) {
    return "Без отдельного лимита";
  }
  if (s.gamesLimitS <= 0) {
    return "Сегодня нельзя";
  }
  const left = s.lock === "none" && !s.studyOnly ? upToMinute(s.gamesRemainingS) : 0;
  return `Осталось ${formatDuration(left)} из ${formatLimit(Math.round(s.gamesLimitS / 60))}`;
}

/** «Мои правила» (R21): дни недели для таблицы и общие правила (лимит игр, «Когда время вышло») строками. */
export function splitRules(rules: MyRule[]): { days: MyRule[]; general: string[] } {
  return {
    days: rules.filter((r) => r.weekday >= 1 && r.weekday <= 7),
    general: rules.filter((r) => !(r.weekday >= 1 && r.weekday <= 7)).map((r) => r.text),
  };
}

/**
 * Строки «Сколько осталось» на «Сегодня» (ТЗ 8.2.6 п.1): остаток общего времени, игр и «Видео и
 * соцсетей», лимит дня, запретные часы, копилка. Остаток переводится в минуты тем же правилом, что у
 * значка и табло (вверх, B-5): окно и значок не расходятся на минуту.
 */
export function todayRows(s: ChildStatus): TodayRow[] {
  const noRules = s.status === "admin" || s.status === "safe";
  let left: string;
  if (noRules) {
    left = "Ограничения не действуют";
  } else if (s.unlimited) {
    left = "Без ограничений";
  } else if (s.status === "forbidden") {
    left = "Сейчас нельзя";
  } else {
    left = formatDuration(s.lock === "none" ? upToMinute(s.remainingS) : 0);
  }
  let limit: string;
  if (noRules) {
    limit = "Не действует";
  } else if (s.unlimited) {
    limit = "Без лимита";
  } else {
    limit = formatLimit(Math.round(s.limitS / 60));
  }
  return [
    { label: "Осталось всего", value: left },
    { label: "Игры", value: gamesLeftText(s) },
    { label: "Видео и соцсети", value: NOT_COUNTED },
    { label: "Лимит дня", value: limit },
    { label: "Запретные часы", value: s.forbiddenNote || "Сегодня запретных часов нет" },
    { label: "Копилка", value: formatLimit(s.bonusBalance) },
  ];
}

/** Вариант «Взять из копилки»: сколько минут и подпись кнопки. */
export interface SpendChoice {
  minutes: number;
  label: string;
}

const SPEND_PRESETS = [15, 30, 60];

/** 15, 30, 60 минут или весь баланс (FR-11); больше баланса не предлагается, пустая копилка - нет вариантов. */
export function spendChoices(balance: number): SpendChoice[] {
  if (balance <= 0) {
    return [];
  }
  const out: SpendChoice[] = SPEND_PRESETS.filter((m) => m < balance).map((m) => ({ minutes: m, label: `${m} мин` }));
  out.push({ minutes: balance, label: `Всё: ${balance} мин` });
  return out;
}

/** Подтверждение перед списанием (FR-11): «Взять из копилки 30 мин? Останется 15 мин». */
export function spendConfirmText(minutes: number, balance: number): string {
  return `Взять из копилки ${minutes} мин? Останется ${Math.max(0, balance - minutes)} мин`;
}

/**
 * Почему «Взять из копилки» сейчас недоступна (урок L-17); пустая строка - доступна. В запретный
 * интервал время из копилки не поможет: пояснение называет, с какого часа можно.
 */
export function spendBlockedNote(s: ChildStatus, balance: number): string {
  if (s.status === "forbidden") {
    const m = /(?:^|:\s)(?:(завтра|сегодня)\s)?в\s(\d{1,2}:\d{2})/.exec(s.until);
    if (!m) {
      return "Из копилки можно взять, когда закончатся запретные часы";
    }
    const day = m[1] === "завтра" ? "завтра " : "";
    return `Из копилки можно взять ${day}с ${m[2]}`;
  }
  return balance <= 0 ? "В копилке пока пусто" : "";
}

/** Быстрые кнопки «Попросить ещё» (FR-10). */
export const ASK_PRESETS = [15, 30, 60] as const;
const ASK_MAX = 240;

/** «Своё число» минут: целое от 1 до 240 (граница службы); иначе null. */
export function askMinutes(raw: string): number | null {
  const t = raw.trim();
  if (!/^\d{1,3}$/.test(t)) {
    return null;
  }
  const n = Number(t);
  return n >= 1 && n <= ASK_MAX ? n : null;
}
