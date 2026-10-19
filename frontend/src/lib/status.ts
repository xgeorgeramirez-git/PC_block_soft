// Статус ребёнка на карточке: цвет - только носитель статуса (ТЗ 8.0 п.3, урок L-14).
import type { StatusKind } from "../types";

export type StatusTone = "ok" | "warn" | "danger" | "neutral";

/** Зелёный - можно, янтарный - осталось мало, красный - время вышло или обход; остальное без цвета. */
export function statusTone(status: StatusKind): StatusTone {
  switch (status) {
    case "ok":
    case "unlimited":
      return "ok";
    case "low":
      return "warn";
    case "timeUp":
    case "bypass":
      return "danger";
    default:
      return "neutral";
  }
}

/** Есть ли у ребёнка учёт времени: у администратора и «не ребёнка» ограничения не действуют. */
export function hasTimeLimits(status: StatusKind): boolean {
  return status !== "admin" && status !== "notChild" && status !== "safe";
}

/** Можно ли добавить время сегодня: при блокировке за обход время ни при чём, там только «Снять блокировку». */
export function canAddTime(status: StatusKind): boolean {
  return hasTimeLimits(status) && status !== "bypass";
}
