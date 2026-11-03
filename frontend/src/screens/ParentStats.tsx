import { useCallback, useEffect, useRef, useState } from "react";
import { getToday, parentStats } from "../api";
import Notice from "../components/Notice";
import PeriodTabs from "../components/PeriodTabs";
import { errorText } from "../lib/errors";
import type { MyTodayReply, StatsPeriod } from "../types";
import { Rules, Totals } from "./MyStats";

interface PickChild {
  childId: number;
  name: string;
}

/**
 * Статистика родителя с выбором ребёнка и периода «Итогов» (FR-23, D-44, D-61): та же «Моя статистика»,
 * что видит ребёнок, только для просмотра. «По дням» и журнал с фильтрами — в вехе M5.
 */
export default function ParentStats({ initialChildId }: { initialChildId?: number }) {
  const [children, setChildren] = useState<PickChild[]>([]);
  const [childId, setChildId] = useState<number | null>(initialChildId ?? null);
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [error, setError] = useState("");
  const [period, setPeriod] = useState<StatsPeriod>("today");
  const [busy, setBusy] = useState(false);
  const alive = useRef(true);
  // Номер последнего запроса: поздний ответ прошлого периода или ребёнка не перекрывает выбранный.
  const seq = useRef(0);

  useEffect(() => {
    alive.current = true;
    getToday().then(
      (t) => {
        if (!alive.current) return;
        setChildren(t.children.map((c) => ({ childId: c.childId, name: c.name })));
        setChildId((cur) => cur ?? t.children[0]?.childId ?? null);
      },
      (e) => {
        if (alive.current) setError(errorText(e));
      },
    );
    return () => {
      alive.current = false;
    };
  }, []);

  const load = useCallback(async (id: number, p: StatsPeriod) => {
    const my = ++seq.current;
    setBusy(true);
    setError("");
    try {
      const reply = await parentStats(id, p);
      if (alive.current && my === seq.current) setData(reply);
    } catch (e) {
      if (alive.current && my === seq.current) setError(errorText(e));
    } finally {
      if (alive.current && my === seq.current) setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (childId != null) void load(childId, period);
  }, [childId, period, load]);

  return (
    <div className="my-stats">
      <div className="page-head">
        <h2 className="panel-title">Статистика</h2>
        <span className="read-only-badge">Только просмотр</span>
        <span className="topbar-spacer" />
        {childId != null ? (
          <button type="button" className="btn btn-small" onClick={() => void load(childId, period)}>
            Обновить
          </button>
        ) : null}
      </div>

      {children.length > 1 ? (
        <div className="stat-child-picker" role="group" aria-label="Ребёнок">
          {children.map((c) => (
            <button
              key={c.childId}
              type="button"
              className={`btn btn-small${c.childId === childId ? " btn-primary" : ""}`}
              aria-pressed={c.childId === childId}
              onClick={() => setChildId(c.childId)}
            >
              {c.name}
            </button>
          ))}
        </div>
      ) : null}

      {children.length > 0 ? <PeriodTabs value={period} onChange={setPeriod} panelId="period-panel" /> : null}

      {error ? (
        <Notice tone="warn" urgent>
          {error}
        </Notice>
      ) : null}

      {children.length === 0 && !error ? (
        <p className="panel-text">Пока нет ни одного ребёнка</p>
      ) : data === null ? (
        error ? null : <p className="panel-text">Загружаю данные</p>
      ) : (
        <div id="period-panel" role="tabpanel" aria-labelledby={`period-${period}`}>
          <Totals data={data} busy={busy} />
          <Rules data={data} />
        </div>
      )}
    </div>
  );
}
