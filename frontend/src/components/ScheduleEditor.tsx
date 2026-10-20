import { useState } from "react";
import { weekdayName } from "../lib/format";
import {
  addInterval,
  clockText,
  copyDayIntervals,
  intervalError,
  intervalsOfDay,
  intervalText,
  MAX_PER_DAY,
  parseClock,
  removeInterval,
  WEEKDAYS,
} from "../lib/schedule";
import type { Interval } from "../types";
import DayStrip from "./DayStrip";

interface Props {
  childName: string;
  schedule: Interval[];
  onChange: (next: Interval[]) => void;
}

/** Запретные часы по дням недели (FR-17, сценарий 8.3.3): список интервалов, полоса суток. */
export default function ScheduleEditor({ childName, schedule, onChange }: Props) {
  return (
    <div className="schedule-editor" data-help-anchor="schedule-tab">
      <p className="visually-hidden">Запретные часы на каждый день недели, {childName}</p>
      {WEEKDAYS.map((weekday) => (
        <DayRow key={weekday} weekday={weekday} schedule={schedule} onChange={onChange} />
      ))}
    </div>
  );
}

function DayRow({ weekday, schedule, onChange }: { weekday: number; schedule: Interval[]; onChange: (n: Interval[]) => void }) {
  const day = intervalsOfDay(schedule, weekday);
  const full = day.length >= MAX_PER_DAY;
  return (
    <section className="schedule-day">
      <h3 className="schedule-day-name">{weekdayName(weekday)}</h3>
      <DayStrip all={schedule} weekday={weekday} />
      <ul className="schedule-list">
        {day.length === 0 ? <li className="schedule-empty">Запретов нет: можно весь день</li> : null}
        {day.map((iv, index) => (
          <li key={`${iv.startMin}-${iv.endMin}-${index}`} className="schedule-item">
            <span>{intervalText(iv)}</span>
            <button
              type="button"
              className="btn btn-small"
              onClick={() => onChange(removeInterval(schedule, weekday, index))}
              aria-label={`Удалить интервал ${clockText(iv.startMin)} до ${clockText(iv.endMin)} в ${weekdayName(weekday)}`}
            >
              Удалить
            </button>
          </li>
        ))}
      </ul>
      {full ? (
        <p className="schedule-note">Больше шести интервалов в день не задать</p>
      ) : (
        <AddInterval weekday={weekday} onAdd={(s, e) => onChange(addInterval(schedule, weekday, s, e))} />
      )}
      <CopyRow weekday={weekday} onCopy={(to) => onChange(copyDayIntervals(schedule, weekday, to))} />
    </section>
  );
}

function AddInterval({ weekday, onAdd }: { weekday: number; onAdd: (startMin: number, endMin: number) => void }) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [error, setError] = useState("");

  const submit = () => {
    const s = parseClock(from);
    const e = parseClock(to);
    if (s === null) {
      setError("Время «с» не похоже на часы, например 21:00");
      return;
    }
    if (e === null) {
      setError("Время «до» не похоже на часы, например 07:00");
      return;
    }
    const err = intervalError(s, e);
    if (err) {
      setError(err);
      return;
    }
    setError("");
    setFrom("");
    setTo("");
    onAdd(s, e);
  };

  return (
    <div className="schedule-add" data-help-anchor="interval-add">
      <label className="field schedule-time">
        <span className="field-label">с</span>
        <input
          className={error ? "field-input field-input-error" : "field-input"}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          placeholder="21:00"
          inputMode="numeric"
          aria-label={`Начало запретного времени в ${weekdayName(weekday)}`}
        />
      </label>
      <label className="field schedule-time">
        <span className="field-label">до</span>
        <input
          className={error ? "field-input field-input-error" : "field-input"}
          value={to}
          onChange={(e) => setTo(e.target.value)}
          placeholder="07:00"
          inputMode="numeric"
          aria-label={`Конец запретного времени в ${weekdayName(weekday)}`}
        />
      </label>
      <button type="button" className="btn" onClick={submit}>
        Добавить интервал
      </button>
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function CopyRow({ weekday, onCopy }: { weekday: number; onCopy: (to: number[]) => void }) {
  const others = WEEKDAYS.filter((d) => d !== weekday);
  return (
    <details className="schedule-copy">
      <summary>Скопировать на другие дни</summary>
      <div className="schedule-copy-days">
        <button type="button" className="btn btn-small" onClick={() => onCopy(others)}>
          На все остальные дни
        </button>
        <button type="button" className="btn btn-small" onClick={() => onCopy([1, 2, 3, 4, 5].filter((d) => d !== weekday))}>
          На будни
        </button>
        <button type="button" className="btn btn-small" onClick={() => onCopy([6, 7].filter((d) => d !== weekday))}>
          На выходные
        </button>
      </div>
    </details>
  );
}
