import type { Interval } from "../types";
import { dayStripSegments, prevDayName } from "../lib/schedule";

interface Props {
  all: Interval[];
  weekday: number;
}

/**
 * Полоса суток 0..24 для одного дня (FR-17): закрашены запретные отрезки, включая перенос
 * интервала с предыдущего дня (штриховка с подписью «с пятницы»). Только показ, не ввод.
 */
export default function DayStrip({ all, weekday }: Props) {
  const segments = dayStripSegments(all, weekday);
  const hasCarry = segments.some((s) => s.carry);
  return (
    <div className="day-strip" aria-hidden="true">
      <div className="day-strip-track">
        {segments.map((s, i) => (
          <div
            key={i}
            className={s.carry ? "day-strip-fill day-strip-carry" : "day-strip-fill"}
            style={{ left: `${s.from * 100}%`, width: `${(s.to - s.from) * 100}%` }}
          />
        ))}
      </div>
      <div className="day-strip-scale">
        <span>0</span>
        <span>6</span>
        <span>12</span>
        <span>18</span>
        <span>24</span>
      </div>
      {hasCarry ? <p className="day-strip-note">Штриховка — запрет с {prevDayName(weekday)}</p> : null}
    </div>
  );
}
