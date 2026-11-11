import { useCallback, useEffect, useRef, useState } from "react";
import { clearStats, getToday, parentStats } from "../api";
import Dialog from "../components/Dialog";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import { genitiveName } from "../lib/format";
import type { MyTodayReply, StatsPeriod } from "../types";
import { Rules, StatsSince, Totals } from "./MyStats";

interface PickChild {
  childId: number;
  name: string;
}

/**
 * Статистика родителя с выбором ребёнка и периода (FR-23, D-44, D-61, 8.3.7): та же «Моя статистика»,
 * что видит ребёнок («Итоги», «По дням», «Программы», «Сайты», «Нарушения», «Копилка» за период), только
 * для просмотра, и «Очистить статистику» выбранного ребёнка с подтверждением (FR-38).
 */
export default function ParentStats({ initialChildId }: { initialChildId?: number }) {
  const [children, setChildren] = useState<PickChild[]>([]);
  const [childId, setChildId] = useState<number | null>(initialChildId ?? null);
  const [data, setData] = useState<MyTodayReply | null>(null);
  const [error, setError] = useState("");
  const [period, setPeriod] = useState<StatsPeriod>("today");
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearMsg, setClearMsg] = useState("");
  const [clearError, setClearError] = useState("");
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

  const pickChild = (id: number) => {
    setChildId(id);
    // Сообщение об очистке - про прежнего ребёнка.
    setClearMsg("");
    setClearError("");
  };

  const name = children.find((c) => c.childId === childId)?.name ?? "";

  const doClear = async () => {
    if (childId == null || clearing) return;
    setClearing(true);
    setClearError("");
    try {
      const r = await clearStats(childId);
      if (!alive.current) return;
      setConfirm(false);
      setClearMsg(r.message);
      await load(childId, period);
    } catch (e) {
      // Ничего не удалено: данные экрана прежние, причина простыми словами.
      if (!alive.current) return;
      setConfirm(false);
      setClearError(errorText(e));
    } finally {
      if (alive.current) setClearing(false);
    }
  };

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

      {children.length > 0 ? (
        <>
          <div className="stat-child-row">
            {children.length > 1 ? (
              <div className="stat-child-picker" role="group" aria-label="Ребёнок">
                {children.map((c) => (
                  <button
                    key={c.childId}
                    type="button"
                    className={`btn btn-small${c.childId === childId ? " btn-primary" : ""}`}
                    aria-pressed={c.childId === childId}
                    onClick={() => pickChild(c.childId)}
                  >
                    {c.name}
                  </button>
                ))}
              </div>
            ) : null}
            <span className="topbar-spacer" />
            {childId != null && data ? (
              <button type="button" className="btn" onClick={() => setConfirm(true)}>
                Очистить статистику
              </button>
            ) : null}
          </div>
          {/* Живая область есть всегда (PL-44): диктор читает сообщение, как только оно появилось. */}
          <p className="child-feedback stat-clear-status" role="status">
            {clearMsg}
          </p>
        </>
      ) : null}

      {error ? (
        <Notice tone="warn" urgent>
          {error}
        </Notice>
      ) : null}
      {clearError ? (
        <Notice tone="warn" urgent>
          {clearError}
        </Notice>
      ) : null}

      {children.length === 0 ? (
        error ? null : <p className="panel-text">Пока нет ни одного ребёнка</p>
      ) : data === null ? (
        // Пока данных нет, нет и вкладок периода: им не на что ссылаться (B-46).
        error ? null : <p className="panel-text">Загружаю данные</p>
      ) : (
        <>
          <StatsSince data={data} />
          <Totals data={data} busy={busy} periods={{ value: period, onChange: setPeriod }} />
          <Rules data={data} />
        </>
      )}

      {confirm ? (
        <Dialog
          title={`Удалить всю статистику ${genitiveName(name)}?`}
          onClose={() => {
            if (!clearing) setConfirm(false);
          }}
          actions={
            <>
              <button type="button" className="btn btn-primary" disabled={clearing} onClick={() => void doClear()}>
                Удалить
              </button>
              <button type="button" className="btn" data-autofocus disabled={clearing} onClick={() => setConfirm(false)}>
                Отмена
              </button>
            </>
          }
        >
          <p>Остаток на сегодня и копилка не изменятся. Это нельзя отменить.</p>
        </Dialog>
      ) : null}
    </div>
  );
}
