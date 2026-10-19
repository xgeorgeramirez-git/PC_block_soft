import { useCallback, useEffect, useRef, useState } from "react";
import { listEvents } from "../api";
import Notice from "../components/Notice";
import { errorText } from "../lib/errors";
import { formatEventTime } from "../lib/format";
import { isViolation } from "../lib/journal";
import type { JournalEntry } from "../types";

interface Props {
  /** Открыт по ссылке «Нарушения сегодня»: сначала только нарушения. */
  violationsOnly?: boolean;
}

/** Журнал событий простыми словами (FR-24, M1: список без фильтров). */
export default function Journal({ violationsOnly = false }: Props) {
  const [events, setEvents] = useState<JournalEntry[] | null>(null);
  const [error, setError] = useState("");
  const [onlyViolations, setOnlyViolations] = useState(violationsOnly);
  const alive = useRef(true);

  const load = useCallback(async () => {
    setError("");
    try {
      const list = await listEvents({ limit: 200 });
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

  const shown = (events ?? []).filter((e) => !onlyViolations || isViolation(e.kind));

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
