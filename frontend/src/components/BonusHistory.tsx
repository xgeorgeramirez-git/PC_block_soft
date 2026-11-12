import { historyText } from "../lib/bonus";
import { formatEventTime, formatLimit } from "../lib/format";
import type { BonusEntry } from "../types";

/**
 * История копилки (FR-22): когда, что и баланс после записи. Одна разметка и один текст строки
 * (lib/bonus.ts) для экрана «Копилка» и раздела «Копилка» статистики за период (RM5-4).
 */
export default function BonusHistory({
  entries,
  empty = "Начислений и трат пока нет.",
  label,
}: {
  entries: BonusEntry[];
  /** Текст, когда записей нет. */
  empty?: string;
  /** Имя списка для экранного диктора. */
  label?: string;
}) {
  if (entries.length === 0) {
    return <p className="panel-text">{empty}</p>;
  }
  return (
    <ul className="bonus-history" aria-label={label}>
      {entries.map((e, i) => (
        <li key={`${e.atMs}-${i}`} className="bonus-history-item">
          <span>{formatEventTime(e.atMs, e.tzOffsetMin)}</span>
          <span>{historyText(e)}</span>
          <span>{`Баланс: ${formatLimit(e.balanceAfter)}`}</span>
        </li>
      ))}
    </ul>
  );
}
