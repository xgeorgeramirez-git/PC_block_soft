import {
  columnMinutes,
  columnUnlimited,
  dayLabel,
  joinMinutes,
  limitField,
  setMinutes,
  splitMinutes,
  toggleUnlimited,
  type LimitColumn,
} from "../lib/limits";
import { weekdayName } from "../lib/format";
import { clampDigits } from "../lib/numinput";
import type { WeekdayLimit } from "../types";

interface Props {
  /** Для чьих правил таблица: попадает в подпись для скринридера. */
  childName: string;
  /** Семь дней по порядку. */
  limits: WeekdayLimit[];
  onChange: (next: WeekdayLimit[]) => void;
  /** Ошибки у полей (L-121), ключ «limits.3» или «games.3»: рамка поля и текст под ним. */
  errors?: Record<string, string>;
}

/** Пределы полей (D-45): часы 0–23, минуты 0–59; целые сутки — переключатель «Без лимита». */
const MAX_HOURS = 23;
const MAX_MINUTES = 59;

const ZERO_HINT: Record<LimitColumn, string> = {
  total: "0 мин: в этот день компьютером пользоваться нельзя",
  games: "0 мин: в этот день играть нельзя",
};

/** Подписи полей для экранного диктора: день, столбец и что в поле. */
const LABELS: Record<LimitColumn, { hours: string; minutes: string; unlimited: string }> = {
  total: { hours: "Часы", minutes: "Минуты", unlimited: "Без лимита" },
  games: { hours: "Часы игр", minutes: "Минуты игр", unlimited: "Игры без лимита" },
};

export const GAMES_NOTE = "Время игр входит в общее. Учёба в общее время не входит";

/**
 * Таблица 7 дней × «Общее» и «Игры» (FR-16): в каждой ячейке часы, минуты и «Без лимита».
 * Табуляция идёт по строке: общее время, затем игры, затем следующий день.
 */
export default function LimitsTable({ childName, limits, onChange, errors = {} }: Props) {
  return (
    <div className="limits-wrap">
      <table className="limits-table">
        <caption className="visually-hidden">Лимиты на каждый день недели, {childName}</caption>
        <thead>
          <tr>
            <th scope="col">День</th>
            <th scope="col">Общее</th>
            <th scope="col">Игры</th>
          </tr>
        </thead>
        <tbody>
          {limits.map((l) => (
            <tr key={l.weekday} className="limits-row">
              <th scope="row">{weekdayName(l.weekday)}</th>
              <LimitCell limits={limits} day={l} column="total" error={errors[limitField("total", l.weekday)]} onChange={onChange} />
              <LimitCell limits={limits} day={l} column="games" error={errors[limitField("games", l.weekday)]} onChange={onChange} />
            </tr>
          ))}
        </tbody>
      </table>
      <p className="field-hint limits-note">{GAMES_NOTE}</p>
    </div>
  );
}

interface CellProps {
  limits: WeekdayLimit[];
  day: WeekdayLimit;
  column: LimitColumn;
  error?: string;
  onChange: (next: WeekdayLimit[]) => void;
}

function LimitCell({ limits, day, column, error, onChange }: CellProps) {
  const name = dayLabel(day.weekday);
  const labels = LABELS[column];
  const unlimited = columnUnlimited(day, column);
  const minutes = columnMinutes(day, column);
  const { h, m } = splitMinutes(minutes);
  const zero = !unlimited && minutes === 0;
  const edit = (nextH: number, nextM: number) => onChange(setMinutes(limits, day.weekday, column, joinMinutes(nextH, nextM)));
  return (
    <td className={unlimited ? "limits-cell limits-cell-off" : "limits-cell"}>
      <div className="limits-inputs">
        <input
          className="field-input limits-num"
          type="text"
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={2}
          aria-label={`${labels.hours}, ${name}`}
          aria-invalid={error ? "true" : undefined}
          value={h}
          disabled={unlimited}
          onChange={(e) => edit(clampDigits(e.target.value, MAX_HOURS), m)}
        />
        <span className="limits-unit" aria-hidden="true">
          ч
        </span>
        <input
          className="field-input limits-num"
          type="text"
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={2}
          aria-label={`${labels.minutes}, ${name}`}
          aria-invalid={error ? "true" : undefined}
          value={m}
          disabled={unlimited}
          onChange={(e) => edit(h, clampDigits(e.target.value, MAX_MINUTES))}
        />
        <span className="limits-unit" aria-hidden="true">
          мин
        </span>
      </div>
      <label className="switch limits-switch">
        <input
          type="checkbox"
          aria-label={`${labels.unlimited}, ${name}`}
          checked={unlimited}
          onChange={(e) => onChange(toggleUnlimited(limits, day.weekday, e.target.checked, column))}
        />
        <span>Без лимита</span>
      </label>
      {zero && !error ? <p className="field-hint">{ZERO_HINT[column]}</p> : null}
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </td>
  );
}
