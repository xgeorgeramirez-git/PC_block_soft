import { useCallback, useEffect, useRef, useState } from "react";
import { getToday, parentStats } from "../api";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import type { MyTodayReply } from "../types";
import { Rules, Totals } from "./MyStats";

interface PickChild {
  childId: number;
  name: string;
}

/**
 * Статистика родителя за сегодня с выбором ребёнка (FR-23, D-44): та же «Моя статистика»,
 * что видит ребёнок, только для просмотра. Полная версия с периодами — в вехе M5.
 */
export default function ParentStats({ initialChildId }: { initialChildId?: number }) {
  const [children, setChildren] = useState<PickChild[]>([]);
  const [childId, setChildId] = useState<number | null>(initialChildId ?? null);
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [error, setError] = useState("");
  const alive = useRef(true);

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

  const load = useCallback(async (id: number) => {
    setError("");
    try {
      const reply = await parentStats(id);
      if (alive.current) setData(reply);
    } catch (e) {
      if (alive.current) setError(errorText(e));
    }
  }, []);

  useEffect(() => {
    if (childId != null) void load(childId);
  }, [childId, load]);

  return (
    <div className="my-stats">
      <div className="page-head">
        <h2 className="panel-title">Статистика</h2>
        <span className="read-only-badge">Только просмотр</span>
        <span className="topbar-spacer" />
        {childId != null ? (
          <button type="button" className="btn btn-small" onClick={() => void load(childId)}>
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

      <p className="field-hint periods-note">Пока показываем сегодня. Неделя, месяц, год и всё время появятся позже.</p>

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
        <div>
          <Totals data={data} />
          <Rules data={data} />
        </div>
      )}
    </div>
  );
}
