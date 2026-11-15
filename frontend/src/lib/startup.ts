// Вкладка «Автозагрузка» (FR-39, ТЗ 8.3.11): название строки, переезд строки после решения
// и подпись «Проверено в 18:40».

import type { ListStartupReply, StartupItemView } from "../types";
import { formatDayAt, formatTimeOfDay, sameLocalDay } from "./format";

/** Суффикс SID в имени задания Планировщика: «OneDrive Reporting Task-S-1-5-21-…». */
const TASK_SID_SUFFIX = /-S-1-5-[0-9-]+$/;

/**
 * Название строки: у задания Планировщика - последний сегмент пути без суффикса «-S-1-5-…»
 * («\OneDrive Reporting Task-S-1-5-21-…» → «OneDrive Reporting Task»), у остальных - как прислала служба.
 */
export function startupShortName(item: StartupItemView): string {
  if (item.source !== "task") return item.name;
  const last = item.name.slice(item.name.lastIndexOf("\\") + 1).replace(TASK_SID_SUFFIX, "").trim();
  return last === "" ? item.name : last;
}

/** Ничего не запускается: во всех трёх блоках пусто. */
export function isStartupEmpty(data: ListStartupReply): boolean {
  return data.pending.length === 0 && data.own.length === 0 && data.shared.length === 0;
}

/**
 * Строка с решением службы встаёт в свой блок (Review Focus 4): убирается из всех блоков и
 * вставляется в «Ждут решения», «Учётку» или «Для всех пользователей» по порядку номеров, как
 * в списке службы. Исходные данные не меняются.
 */
export function applyStartupItem(data: ListStartupReply, item: StartupItemView): ListStartupReply {
  const drop = (list: StartupItemView[]) => list.filter((x) => x.id !== item.id);
  const next: ListStartupReply = { ...data, pending: drop(data.pending), own: drop(data.own), shared: drop(data.shared) };
  const key = item.decision === "pending" ? "pending" : item.scope === "shared" ? "shared" : "own";
  next[key] = [...next[key], { ...item }].sort((a, b) => a.id - b.id);
  return next;
}

/**
 * Подпись под заголовком: «Проверяю…», пока служба проверяет; «Проверено в 18:40» за сегодня;
 * «Проверено 10 ноября в 18:40» за другой день; "" - проверки ещё не было.
 */
export function checkedText(checkedAt: number, checking: boolean, nowMs: number, tzOffsetMin: number): string {
  if (checking) return "Проверяю…";
  if (checkedAt <= 0) return "";
  if (sameLocalDay(checkedAt, nowMs, tzOffsetMin)) return `Проверено в ${formatTimeOfDay(checkedAt, tzOffsetMin)}`;
  return `Проверено ${formatDayAt(checkedAt, tzOffsetMin)}`;
}
