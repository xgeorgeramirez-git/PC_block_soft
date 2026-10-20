// Тексты окон ребёнка (FR-12, FR-32). Числа и слова приходят из MyToday готовыми: здесь они
// только раскладываются по строкам окна, ничего не пересчитывается (один источник с значком).
import type { MyTodayReply } from "../types";
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
  return [`Сегодня можно ${formatLimit(Math.round(s.limitS / 60))}.`, "Сколько осталось, видно в значке у часов."];
}

/** Строки «Сегодня» для «Итогов»: «Использовано …» уже есть в строке «Всего», второй раз не нужна. */
export function summaryLines(reply: MyTodayReply): string[] {
  const used = `Использовано ${formatDuration(reply.status.usedS)}`;
  return reply.today.filter((line) => line !== used);
}

/** Номер сегодняшнего дня недели для подсветки строки «Моих правил»: 1 = пн … 7 = вс. */
export function todayRuleWeekday(at: Date = new Date()): number {
  return weekdayOfMs(at.getTime(), localOffsetMin(at));
}
