import { dayLabel, joinMinutes, setDay, splitMinutes, toggleUnlimited } from "../lib/limits";
import { weekdayName } from "../lib/format";
import { clampDigits } from "../lib/numinput";
import type { WeekdayLimit } from "../types";

interface Props {
  /** Для чьих правил таблица: попадает в подпись для скринридера. */
  childName: string;
  /** Семь дней по порядку. */
  limits: WeekdayLimit[];
  onChange: (next: WeekdayLimit[]) => void;
  /** Ошибки по дням недели (L-121): рамка поля и текст под ним. */
  errors?: Record<number, string>;
}

/** Пределы полей (D-45): часы 0–23, минуты 0–59; целые сутки — переключатель «Без лимита». */
const MAX_HOURS = 23;
const MAX_MINUTES = 59;

const ZERO_HINT = "0 мин: в этот день компьютером пользоваться нельзя";

/** Таблица 7 дней × «Время в день»: часы, минуты и переключатель «Без лимита» (FR-16). */
export default function LimitsTable({ childName, limits, onChange, errors = {} }: Props) {
  return (
    <table className="limits-table" data-help-anchor="limits-table">
      <caption className="visually-hidden">Лимиты на каждый день недели, {childName}</caption>
      <thead>
        <tr>
          <th scope="col">День</th>
          <th scope="col">Время в день</th>
          <th scope="col">Без лимита</th>
        </tr>
      </thead>
      <tbody>
        {limits.map((l) => {
          const day = dayLabel(l.weekday);
          const { h, m } = splitMinutes(l.totalMin);
          const error = errors[l.weekday];
          const zero = !l.unlimited && l.totalMin === 0;
          const edit = (nextH: number, nextM: number) =>
            onChange(setDay(limits, l.weekday, { totalMin: joinMinutes(nextH, nextM) }));
          return (
            <tr key={l.weekday} className={l.unlimited ? "limits-row limits-row-off" : "limits-row"}>
              <th scope="row">{weekdayName(l.weekday)}</th>
              <td>
                <div className="limits-inputs">
                  <input
                    className="field-input limits-num"
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]*"
                    maxLength={2}
                    aria-label={`Часы, ${day}`}
                    aria-invalid={error ? "true" : undefined}
                    value={h}
                    disabled={l.unlimited}
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
                    aria-label={`Минуты, ${day}`}
                    aria-invalid={error ? "true" : undefined}
                    value={m}
                    disabled={l.unlimited}
                    onChange={(e) => edit(h, clampDigits(e.target.value, MAX_MINUTES))}
                  />
                  <span className="limits-unit" aria-hidden="true">
                    мин
                  </span>
                </div>
                {zero && !error ? <p className="field-hint">{ZERO_HINT}</p> : null}
                {error ? (
                  <p className="field-error" role="alert">
                    {error}
                  </p>
                ) : null}
              </td>
              <td>
                <label className="switch">
                  <input
                    type="checkbox"
                    aria-label={`Без лимита, ${day}`}
                    checked={l.unlimited}
                    onChange={(e) => onChange(toggleUnlimited(limits, l.weekday, e.target.checked))}
                  />
                  <span>Без лимита</span>
                </label>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
