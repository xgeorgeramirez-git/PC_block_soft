import { useCallback, useEffect, useRef, useState } from "react";
import { listEvents } from "../api";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import { formatEventTime } from "../lib/format";
import { isViolation, startOfDayMs } from "../lib/journal";
import type { JournalEntry } from "../types";

interface Props {
  /** Открыт по ссылке «Нарушения сегодня»: сначала только нарушения. */
  violationsOnly?: boolean;
}

/** Период журнала (FR-24, D-47): сегодня, за месяц, всё. */
type Period = "today" | "month" | "all";

const PERIODS: { id: Period; label: string }[] = [
  { id: "today", label: "Сегодня" },
  { id: "month", label: "За месяц" },
  { id: "all", label: "Всё" },
];

/** periodStartMs — начало выбранного периода по местному времени; «всё» — 0. */
function periodStartMs(period: Period, now: number): number {
  if (period === "all") return 0;
  if (period === "today") return startOfDayMs(now);
  const d = new Date(now);
  d.setDate(1);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/** Журнал событий простыми словами (FR-24, D-47: вкладки периода). */
export default function Journal({ violationsOnly = false }: Props) {
  const [events, setEvents] = useState<JournalEntry[] | null>(null);
  const [error, setError] = useState("");
  const [onlyViolations, setOnlyViolations] = useState(violationsOnly);
  const [period, setPeriod] = useState<Period>("all");
  const alive = useRef(true);

  const load = useCallback(async () => {
    setError("");
    try {
      const list = await listEvents({ limit: 1000 });
      if (alive.current) setEvents(list);
    } catch (e) {
      if (alive.current) setError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  const from = periodStartMs(period, Date.now());
  const shown = (events ?? []).filter((e) => e.atMs >= from && (!onlyViolations || isViolation(e.kind)));

  return (
    <section aria-labelledby="journal-title">
      <div className="page-head">
        <h2 id="journal-title" className="panel-title">
          {onlyViolations ? "Журнал: нарушения" : "Журнал"}
        </h2>
        {violationsOnly || onlyViolations ? (
          <button type="button" className="btn" onClick={() => setOnlyViolations((v) => !v)}>
            {onlyViolations ? "Показать все события" : "Показать только нарушения"}
          </button>
        ) : null}
      </div>

      <div className="tabs" role="tablist" aria-label="Период журнала" data-help-anchor="journal-periods">
        {PERIODS.map((p) => (
          <button
            key={p.id}
            type="button"
            role="tab"
            aria-selected={period === p.id}
            className="tab"
            onClick={() => setPeriod(p.id)}
          >
            {p.label}
          </button>
        ))}
      </div>

      {error ? (
        <div className="panel">
          <Notice tone="warn" urgent>
            {error}
          </Notice>
          <button type="button" className="btn" onClick={() => void load()}>
            Повторить
          </button>
        </div>
      ) : events === null ? (
        <p className="panel-text">Загружаю журнал</p>
      ) : shown.length === 0 ? (
        <p className="panel-text">{onlyViolations ? "Нарушений пока нет" : "Пока событий нет"}</p>
      ) : (
        <ul className="journal">
          {shown.map((e, i) => (
            <li key={`${e.atMs}-${i}`} className="journal-item">
              <time className="journal-time" dateTime={new Date(e.atMs).toISOString()}>
                {formatEventTime(e.atMs, e.tzOffsetMin)}
              </time>
              <span className="journal-text">{e.text}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
