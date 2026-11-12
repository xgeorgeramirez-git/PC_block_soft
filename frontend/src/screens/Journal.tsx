import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { clearJournal, getToday, listEventsPage } from "../api";
import Dialog from "../components/Dialog";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import { formatEventTime, genitiveName } from "../lib/format";
import { JOURNAL_GROUPS, JOURNAL_PERIODS, journalPeriodStartMs, type JournalPeriod } from "../lib/journal";
import type { JournalEntry, JournalGroup } from "../types";

interface Props {
  /** Открыт по ссылке «Нарушения сегодня»: «Что: Нарушения» и вкладка «Сегодня» (8.3.7 п.2). */
  violationsOnly?: boolean;
}

interface PickChild {
  childId: number;
  name: string;
}

/** Признак загрузки нового фильтра (короткий, на месте, как у «Итогов»). */
const LOADING = "Загружаю…";

/** Страница журнала: столько записей за раз, дальше - «Показать ещё» (RM5-5). */
const PAGE = 200;

interface Filter {
  period: JournalPeriod;
  childId: number;
  group: JournalGroup;
}

/** Запрос первой страницы фильтра; начало периода - от текущего момента. */
function request(f: Filter) {
  return { childId: f.childId, fromMs: journalPeriodStartMs(f.period, Date.now()), group: f.group, limit: PAGE };
}

/**
 * Журнал событий простыми словами (FR-24, 8.3.7 п.2): вкладки периода (D-47), фильтры «Ребёнок» и
 * «Что» уходят в запрос службы, внизу «Показать ещё» по курсору. При выбранном ребёнке - «Очистить
 * журнал» с подтверждением (FR-38, 8.3.7 п.3).
 */
export default function Journal({ violationsOnly = false }: Props) {
  const [filter, setFilter] = useState<Filter>({
    period: violationsOnly ? "today" : "all",
    childId: 0,
    group: violationsOnly ? "violations" : "",
  });
  const [children, setChildren] = useState<PickChild[]>([]);
  const [events, setEvents] = useState<JournalEntry[] | null>(null);
  const [more, setMore] = useState(false);
  const [error, setError] = useState("");
  // Идёт запрос нового фильтра: прежний список стоит приглушённым, рядом короткое «Загружаю…».
  const [busy, setBusy] = useState(false);
  const [moreBusy, setMoreBusy] = useState(false);
  const [moreError, setMoreError] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearMsg, setClearMsg] = useState("");
  const [clearError, setClearError] = useState("");
  const alive = useRef(true);
  // Номер последнего запроса: поздний ответ прежнего фильтра не перекрывает выбранный и не
  // дописывается к его списку.
  const seq = useRef(0);
  const tabs = useRef(new Map<JournalPeriod, HTMLButtonElement>());

  useEffect(() => {
    alive.current = true;
    // Без списка детей журнал всё равно работает: в «Ребёнок» остаётся «Все дети».
    getToday().then(
      (t) => {
        if (alive.current) setChildren(t.children.map((c) => ({ childId: c.childId, name: c.name })));
      },
      () => undefined,
    );
    return () => {
      alive.current = false;
    };
  }, []);

  const load = useCallback(async (f: Filter) => {
    const my = ++seq.current;
    setError("");
    setMoreError("");
    setMoreBusy(false);
    setBusy(true);
    try {
      const page = await listEventsPage(request(f));
      if (!alive.current || my !== seq.current) return;
      setEvents(page.events);
      setMore(page.more);
    } catch (e) {
      if (alive.current && my === seq.current) setError(errorText(e));
    } finally {
      if (alive.current && my === seq.current) setBusy(false);
    }
  }, []);

  useEffect(() => {
    void load(filter);
  }, [filter, load]);

  const loadMore = async () => {
    const last = events?.[events.length - 1];
    if (!last || moreBusy) return;
    const my = seq.current;
    setMoreBusy(true);
    setMoreError("");
    try {
      const page = await listEventsPage({ ...request(filter), beforeAtMs: last.atMs, beforeId: last.id });
      if (!alive.current || my !== seq.current) return;
      setEvents((cur) => [...(cur ?? []), ...page.events]);
      setMore(page.more);
    } catch (e) {
      // Уже загруженное остаётся на экране.
      if (alive.current && my === seq.current) setMoreError(errorText(e));
    } finally {
      if (alive.current && my === seq.current) setMoreBusy(false);
    }
  };

  const change = (next: Partial<Filter>) => {
    if (next.childId !== undefined && next.childId !== filter.childId) {
      // Сообщение об очистке - про прежнего ребёнка.
      setClearMsg("");
      setClearError("");
    }
    setFilter((cur) => ({ ...cur, ...next }));
  };

  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = JOURNAL_PERIODS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    const id = JOURNAL_PERIODS[next].id;
    change({ period: id });
    tabs.current.get(id)?.focus();
  };

  // Выбранный ребёнок пропал из списка (удалён) - кнопки очистки нет, фильтр показывает «Все дети».
  const picked = children.find((c) => c.childId === filter.childId);

  const doClear = async () => {
    if (!picked || clearing) return;
    setClearing(true);
    setClearError("");
    try {
      const r = await clearJournal(picked.childId);
      if (!alive.current) return;
      setConfirm(false);
      setClearMsg(r.message);
      await load(filter);
    } catch (e) {
      // Ничего не удалено: список прежний, причина простыми словами.
      if (!alive.current) return;
      setConfirm(false);
      setClearError(errorText(e));
    } finally {
      if (alive.current) setClearing(false);
    }
  };

  return (
    <section aria-labelledby="journal-title" className="journal-screen">
      <div className="page-head">
        <h2 id="journal-title" className="panel-title">
          Журнал
        </h2>
      </div>

      <div className="tabs" role="tablist" aria-label="Период журнала">
        {JOURNAL_PERIODS.map((p, i) => (
          <button
            key={p.id}
            ref={(el) => {
              if (el) tabs.current.set(p.id, el);
              else tabs.current.delete(p.id);
            }}
            type="button"
            role="tab"
            id={`journal-period-${p.id}`}
            aria-selected={filter.period === p.id}
            aria-controls="journal-panel"
            tabIndex={filter.period === p.id ? 0 : -1}
            className="tab"
            onClick={() => change({ period: p.id })}
            onKeyDown={(e) => onTabKey(e, i)}
          >
            {p.label}
          </button>
        ))}
      </div>

      <div className="journal-filters">
        <div className="journal-filter">
          <label className="field-label" htmlFor="journal-child">
            Ребёнок
          </label>
          <select
            id="journal-child"
            className="field-input"
            value={picked ? String(picked.childId) : "0"}
            onChange={(e) => change({ childId: Number(e.target.value) })}
          >
            <option value="0">Все дети</option>
            {children.map((c) => (
              <option key={c.childId} value={String(c.childId)}>
                {c.name}
              </option>
            ))}
          </select>
        </div>
        <div className="journal-filter">
          <label className="field-label" htmlFor="journal-group">
            Что
          </label>
          <select
            id="journal-group"
            className="field-input"
            value={filter.group}
            onChange={(e) => change({ group: e.target.value as JournalGroup })}
          >
            {JOURNAL_GROUPS.map((g) => (
              <option key={g.id || "all"} value={g.id}>
                {g.label}
              </option>
            ))}
          </select>
        </div>
        <span className="topbar-spacer" />
        {picked ? (
          <button type="button" className="btn journal-clear" onClick={() => setConfirm(true)}>
            Очистить журнал
          </button>
        ) : null}
      </div>
      {/* Живая область есть всегда (PL-44): диктор читает сообщение, как только оно появилось. */}
      <p className="child-feedback stat-clear-status" role="status">
        {clearMsg}
      </p>
      {clearError ? (
        <Notice tone="warn" urgent>
          {clearError}
        </Notice>
      ) : null}

      <div id="journal-panel" role="tabpanel" aria-labelledby={`journal-period-${filter.period}`}>
        {/* Живая область есть всегда (PL-44); при первой загрузке говорит строка «Загружаю журнал…». */}
        <p className="journal-loading" role="status">
          {busy && events !== null && !error ? LOADING : ""}
        </p>
        {error ? (
          <div className="panel">
            <Notice tone="warn" urgent>
              {error}
            </Notice>
            <button type="button" className="btn" onClick={() => void load(filter)}>
              Повторить
            </button>
          </div>
        ) : events === null ? (
          <p className="panel-text">Загружаю журнал…</p>
        ) : events.length === 0 ? (
          <p className="panel-text">За этот период таких событий нет</p>
        ) : (
          <>
            <ul className={busy ? "journal journal-stale" : "journal"} aria-label="События" aria-busy={busy}>
              {events.map((e) => (
                <li key={e.id} className="journal-item">
                  <time className="journal-time" dateTime={new Date(e.atMs).toISOString()}>
                    {formatEventTime(e.atMs, e.tzOffsetMin)}
                  </time>
                  <span className="journal-text">{e.text}</span>
                </li>
              ))}
            </ul>
            {moreError ? (
              <Notice tone="warn" urgent>
                {moreError}
              </Notice>
            ) : null}
            {more ? (
              <div className="journal-more">
                <button type="button" className="btn" disabled={moreBusy} onClick={() => void loadMore()}>
                  {moreError ? "Повторить" : "Показать ещё"}
                </button>
              </div>
            ) : null}
          </>
        )}
      </div>

      {confirm && picked ? (
        <Dialog
          title={`Удалить весь журнал ${genitiveName(picked.name)}?`}
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
          <p>Это нельзя отменить.</p>
        </Dialog>
      ) : null}
    </section>
  );
}
