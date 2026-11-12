// Тексты окон ребёнка (FR-12, FR-32). Числа и слова приходят из MyToday готовыми: здесь они
// только раскладываются по строкам окна, ничего не пересчитывается (один источник с значком).
import type { ChildStatus, MyRule, MyTodayReply, StatsPeriod } from "../types";
import { formatDuration, formatLimit, weekdayOfMs, localOffsetMin } from "./format";

const NO_RULES = ["admin", "safe", "notChild"];

/**
 * Время сейчас не закрыто экраном: экрана нет или идёт перерыв на отдых (R27) - время на сегодня
 * не кончилось, остаток показывается как есть.
 */
function timeOpen(s: ChildStatus): boolean {
  return s.lock === "none" || s.lock === "rest";
}

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
  const repeatsLeft = timeOpen(reply.status) ? reply.status.summary : used;
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

/** Вкладка «Итогов» (D-58): «Всего» или категория учёта программ и сайтов. */
export type TotalsTab = "all" | "games" | "video_social" | "study" | "other";

/**
 * Строки-вкладки «Итогов» по порядку (8.2.6 п.3, как statsCatOrder службы). list - имя списка строк
 * вкладки для экранного диктора: в «Видео и соцсетях» сайты, во «Всего» программы и сайты.
 */
export const TOTALS_TABS: { id: TotalsTab; label: string; list: string }[] = [
  { id: "all", label: "Всего", list: "Всего: по программам и сайтам" },
  { id: "games", label: "Игры", list: "Игры: по программам" },
  { id: "video_social", label: "Видео и соцсети", list: "Видео и соцсети: по сайтам" },
  { id: "study", label: "Учёба", list: "Учёба: по программам" },
  { id: "other", label: "Прочее", list: "Прочее: по программам" },
];

/** Пустая вкладка. */
export const EMPTY_TAB = "Пока ничего не запускалось";

/** Период «Итогов» (8.2.6 п.2-3, D-61): вкладка, строка «Использовано …» и подпись суммы вкладки. */
export interface PeriodInfo {
  id: StatsPeriod;
  label: string;
  /** «Использовано сегодня», «Использовано за неделю». */
  used: string;
  /** «Игры за неделю: 40 мин» - слова после названия вкладки. */
  sum: string;
}

/** Периоды по порядку вкладок; по умолчанию «Сегодня». */
export const PERIODS: PeriodInfo[] = [
  { id: "today", label: "Сегодня", used: "Использовано сегодня", sum: "за сегодня" },
  { id: "week", label: "Неделя", used: "Использовано за неделю", sum: "за неделю" },
  { id: "month", label: "Месяц", used: "Использовано за месяц", sum: "за месяц" },
  { id: "year", label: "Год", used: "Использовано за год", sum: "за год" },
  { id: "all", label: "Всё время", used: "Использовано за всё время", sum: "за всё время" },
];

/** Сведения о периоде; неизвестный - «Сегодня». */
export function periodOf(id: StatsPeriod): PeriodInfo {
  return PERIODS.find((p) => p.id === id) ?? PERIODS[0];
}

/**
 * Пустой период (8.2.6 п.9): за неделю, месяц, год или всё время ни секунды и ни минуты доп. времени -
 * «За этот период пока нет данных». «Сегодня» пустым не бывает: там вкладки с нулями и
 * «Пока ничего не запускалось», как раньше.
 */
export function periodEmpty(reply: MyTodayReply): boolean {
  return reply.period !== "today" && reply.allS <= 0 && reply.extraMin <= 0 && reply.programs.length === 0;
}

/**
 * Пояснение у вкладок «Итогов», когда «Всего» больше «Использовано …» на учёбу (D-7, FB-10):
 * «Всего = 22 мин в счёт лимита + 3 мин учёбы вне лимита». Числа - те, что на экране: первое как в
 * «Использовано …» за тот же период, второе - разница с «Всего» в целых минутах (секунды не теряют
 * минуту в сумме). Без учёбы или без видимой разницы - пусто.
 */
export function allWithStudyText(reply: MyTodayReply): string {
  const usedMin = Math.floor(Math.max(0, reply.usedS) / 60);
  const studyMin = Math.floor(Math.max(0, reply.allS) / 60) - usedMin;
  if (reply.studyS <= 0 || studyMin <= 0) return "";
  return `Всего = ${formatDuration(usedMin * 60)} в счёт лимита + ${formatDuration(studyMin * 60)} учёбы вне лимита`;
}

/** Сумма вкладки за период, секунды: итоги службы, а не сумма строк (одно число - одно место). */
export function tabSeconds(reply: MyTodayReply, tab: TotalsTab): number {
  switch (tab) {
    case "all":
      return reply.allS;
    case "games":
      return reply.gamesS;
    case "video_social":
      return reply.videoS ?? 0;
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
  /** Строка «Остальное время»: время без программы, всегда последняя. */
  rest: boolean;
}

/** Время строки: меньше минуты - словами, иначе как везде в окне. */
function rowTime(seconds: number): string {
  return seconds > 0 && seconds < 60 ? "меньше 1 мин" : formatDuration(seconds);
}

/**
 * Строки вкладки (D-58, S39): программы и сайты самые долгие сверху, «Остальное время» (время без
 * программы) последней. «Всего» - все программы и сайты (у сайта path - домен), одна программа одной
 * строкой, даже если её секунды в двух категориях. Строки - как их отдала служба: категория сайта за
 * прошлые дни не пересчитывается по нынешним спискам. Доля - целые проценты от суммы вкладки, каждая округлена сама: сумма долей может
 * быть 99-101, подгонки нет.
 */
export function tabRows(reply: MyTodayReply, tab: TotalsTab): TabRow[] {
  const merged = new Map<string, { name: string; seconds: number; rest: boolean }>();
  for (const p of reply.programs) {
    if (tab !== "all" && p.category !== tab) continue;
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

/** Строка таблицы «Программы» или «Сайты»: название, время и доля от суммы таблицы. */
export interface UsageRow {
  key: string;
  name: string;
  time: string;
  share: string;
}

/**
 * Таблица «Программы» (kind "program"; строка с путём без kind - тоже программа) или «Сайты» (kind "site")
 * за период (8.2.6 п.5, RM5-4): строка - сумма секунд одного path из всех категорий (программа могла
 * сменить категорию); строки «Остальное время» (path "") не входят. Доля - от суммы строк этой таблицы,
 * каждая округлена сама (сумма 99-101 % допустима). Самые долгие сверху, при равенстве по имени, затем
 * по path. Скрытых и запрещённых служба не отдаёт (PL-38): окно их не фильтрует повторно.
 */
export function usageRows(reply: MyTodayReply, kind: "program" | "site"): UsageRow[] {
  const merged = new Map<string, { name: string; seconds: number }>();
  for (const p of reply.programs) {
    if (p.path === "") continue;
    const k = p.kind === "site" ? "site" : "program";
    if (k !== kind) continue;
    const cur = merged.get(p.path);
    if (cur) cur.seconds += p.seconds;
    else merged.set(p.path, { name: p.name, seconds: p.seconds });
  }
  const rows = [...merged.entries()].filter(([, v]) => v.seconds > 0);
  const total = rows.reduce((n, [, v]) => n + v.seconds, 0);
  rows.sort(([ka, a], [kb, b]) => {
    if (a.seconds !== b.seconds) return b.seconds - a.seconds;
    const na = a.name.toLowerCase();
    const nb = b.name.toLowerCase();
    if (na !== nb) return na < nb ? -1 : 1;
    return ka < kb ? -1 : 1;
  });
  return rows.map(([key, v]) => ({
    key,
    name: v.name,
    time: rowTime(v.seconds),
    share: `${total > 0 ? Math.round((v.seconds * 100) / total) : 0}%`,
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
  const left = timeOpen(s) && !s.studyOnly ? upToMinute(s.gamesRemainingS) : 0;
  return `Осталось ${formatDuration(left)} из ${formatLimit(Math.round(s.gamesLimitS / 60))}`;
}

/**
 * Есть ли сегодня лимит «Видео и соцсетей» (R6) - то же правило, что у службы (hasVideoLimit,
 * internal/ipc/status.go): нет поля videoUnlimited или лимит 0 без закрытия видео - лимита нет.
 */
export function hasVideoLimit(s: ChildStatus): boolean {
  return s.videoUnlimited === false && ((s.videoLimitS ?? 0) > 0 || s.closeVideo === true);
}

/**
 * Строка «Видео и соцсети» в «Сколько осталось» (R6): как «Игры» - остаток из ответа службы, как в
 * подсказке значка («Видео: 20 мин»), и лимит видео дня с доп. временем. Время вышло или открыта только
 * учёба - 0.
 */
export function videoLeftText(s: ChildStatus): string {
  if (s.status === "admin" || s.status === "safe") {
    return "Не действует";
  }
  if (!hasVideoLimit(s)) {
    return "Без отдельного лимита";
  }
  const limitS = s.videoLimitS ?? 0;
  if (limitS <= 0) {
    return "Сегодня нельзя";
  }
  const left = timeOpen(s) && !s.studyOnly ? upToMinute(s.videoRemainingS ?? 0) : 0;
  return `Осталось ${formatDuration(left)} из ${formatLimit(Math.round(limitS / 60))}`;
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
    left = formatDuration(timeOpen(s) ? upToMinute(s.remainingS) : 0);
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
    { label: "Видео и соцсети", value: videoLeftText(s) },
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
