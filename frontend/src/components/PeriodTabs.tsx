import { useEffect, useRef, type KeyboardEvent } from "react";
import { PERIODS } from "../lib/mytoday";
import type { StatsPeriod } from "../types";

/**
 * Вкладки периодов «Итогов» (8.2.6 п.2, D-61): «Сегодня», «Неделя», «Месяц», «Год», «Всё время». Та же
 * разметка, что у вкладок «Итогов»: role="tablist", выбранная - aria-selected и единственная в порядке
 * Tab, стрелки, Home и End переключают и переводят фокус. Панель периода - id panelId, подписана
 * выбранной вкладкой (`period-<id>`). Общая для окна ребёнка и статистики родителя.
 */
export default function PeriodTabs({
  value,
  onChange,
  panelId,
}: {
  value: StatsPeriod;
  onChange: (period: StatsPeriod) => void;
  panelId: string;
}) {
  const tabs = useRef(new Map<StatsPeriod, HTMLButtonElement>());
  // Выбор вернулся к прежней вкладке (сбой загрузки периода): фокус с вкладок не теряется, а идёт на
  // выбранную - единственную в порядке Tab.
  useEffect(() => {
    const el = tabs.current.get(value);
    const focused = document.activeElement;
    if (!el || focused === el) return;
    if ([...tabs.current.values()].some((t) => t === focused)) el.focus();
  }, [value]);
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = PERIODS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    const id = PERIODS[next].id;
    onChange(id);
    tabs.current.get(id)?.focus();
  };
  return (
    <div className="tabs" role="tablist" aria-label="Период">
      {PERIODS.map((p, i) => (
        <button
          key={p.id}
          ref={(el) => {
            if (el) tabs.current.set(p.id, el);
            else tabs.current.delete(p.id);
          }}
          type="button"
          role="tab"
          id={`period-${p.id}`}
          className="tab"
          aria-selected={p.id === value}
          aria-controls={panelId}
          tabIndex={p.id === value ? 0 : -1}
          onClick={() => onChange(p.id)}
          onKeyDown={(e) => onKey(e, i)}
        >
          {p.label}
        </button>
      ))}
    </div>
  );
}
