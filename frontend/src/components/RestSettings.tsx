import { useId } from "react";
import catUrl from "../assets/rest-cat.svg";
import {
  BREAK_MESSAGE_MAX,
  breakField,
  breakSummary,
  DEFAULT_BREAK_MESSAGE,
  selfRestHint,
  selfRestLabel,
  setBreakNumber,
  toggleBreak,
  type BreakNumberField,
} from "../lib/rest";
import type { BreakCounts, BreakRules } from "../types";

interface Props {
  value: BreakRules;
  /** Сохранённые настройки: выключатель возвращает из них неверные поля (toggleBreak). */
  saved: BreakRules;
  onChange: (next: BreakRules) => void;
  /** Ошибки у полей по ключам «break.restMin» и т. п. (L-121). */
  errors: Record<string, string>;
}

const COUNTS: { value: BreakCounts; label: string }[] = [
  { value: "all", label: "Всё время, кроме учёбы" },
  { value: "games", label: "Только игры" },
];

const two = (n: number) => String(n).padStart(2, "0");

/**
 * Вкладка «Перерыв на отдых» (FR-37, 8.3.3 п.7): переключатель, поля в минутах, что считать, сообщение на
 * экране перерыва с образцом «Как увидит ребёнок» и итог словами. Сохраняет общая панель (FR-19).
 */
export default function RestSettings({ value, saved, onChange, errors }: Props) {
  const off = !value.enabled;
  const id = useId();
  const setNum = (field: BreakNumberField, raw: string) => onChange(setBreakNumber(value, field, raw));
  const msgError = errors[breakField("message")];
  const msgHint = `${id}-msg-hint`;
  const msgErr = `${id}-msg-error`;
  const shown = value.message.trim() || DEFAULT_BREAK_MESSAGE;
  const rest = value.restMin;
  const timer = `${two(Math.floor(rest / 60))}:${two(rest % 60)}:00`;
  return (
    <div className="rest-tab">
      <p className="panel-text">После долгой игры компьютер ненадолго закрывается, чтобы ребёнок отдохнул. Настройки свои у каждого ребёнка.</p>
      <label className="switch rest-switch">
        <input type="checkbox" checked={value.enabled} onChange={(e) => onChange(toggleBreak(value, e.target.checked, saved))} />
        <span>Перерыв на отдых</span>
      </label>
      <div className="rest-layout">
        <fieldset className="rest-fields" disabled={off}>
          <legend className="visually-hidden">Настройки перерыва</legend>
          <MinutesField label="Через сколько" field="afterMin" disabled={off} value={value.afterMin} error={errors[breakField("afterMin")]} onChange={setNum} />
          <fieldset className="rest-counts">
            <legend className="field-label">Что считать</legend>
            {COUNTS.map((c) => (
              <label key={c.value} className="rest-count">
                <input type="radio" name={`${id}-counts`} value={c.value} checked={value.counts === c.value} disabled={off} onChange={() => onChange({ ...value, counts: c.value })} />
                <span>{c.label}</span>
              </label>
            ))}
          </fieldset>
          <MinutesField label="Предупредить за" field="warnMin" disabled={off} value={value.warnMin} error={errors[breakField("warnMin")]} onChange={setNum} />
          <MinutesField label="Отдых" field="restMin" disabled={off} value={value.restMin} error={errors[breakField("restMin")]} onChange={setNum} />
          <MinutesField
            label={selfRestLabel(value.counts)}
            field="selfRestMin" disabled={off}
            value={value.selfRestMin}
            error={errors[breakField("selfRestMin")]}
            hint={selfRestHint(value.counts)}
            onChange={setNum}
          />
          <div className={msgError ? "field rest-message field-invalid" : "field rest-message"}>
            <label className="field-label" htmlFor={`${id}-msg`}>
              Сообщение на экране перерыва
            </label>
            <input
              id={`${id}-msg`}
              name="breakMessage"
              autoComplete="off"
              className="field-input"
              type="text"
              maxLength={BREAK_MESSAGE_MAX}
              value={value.message}
              disabled={off}
              aria-invalid={msgError ? "true" : undefined}
              aria-describedby={msgError ? msgErr : msgHint}
              onChange={(e) => onChange({ ...value, message: e.target.value })}
            />
            {msgError ? (
              <p id={msgErr} className="field-error" role="alert">
                {msgError}
              </p>
            ) : (
              <p id={msgHint} className="field-hint">
                До {BREAK_MESSAGE_MAX} символов, сейчас {[...value.message].length}. Пустое поле вернёт стандартный текст
              </p>
            )}
          </div>
        </fieldset>
        <figure className="rest-sample" aria-labelledby={`${id}-sample`}>
          <figcaption id={`${id}-sample`} className="field-label">
            Как увидит ребёнок
          </figcaption>
          <div className="rest-sample-screen">
            <img className="rest-sample-cat" src={catUrl} alt="" width={64} height={64} />
            <p className="rest-sample-title">Перерыв на отдых</p>
            <div className="rest-sample-tiles" role="img" aria-label={`Таймер ${timer}`}>
              {timer.split(":").map((pair, i) => (
                <span key={i} className="rest-sample-pair" aria-hidden="true">
                  {i > 0 ? <span className="rest-sample-colon">:</span> : null}
                  {[...pair].map((d, k) => (
                    <span key={k} className="rest-sample-tile">
                      {d}
                    </span>
                  ))}
                </span>
              ))}
            </div>
            <p className="rest-sample-message">{shown}</p>
          </div>
        </figure>
      </div>
      {/* Без живой области (Ruling): итог меняется на каждое нажатие цифры, читалка повторяла бы
          весь абзац; он стоит сразу под полями и читается по порядку. */}
      <p className="rest-summary">
        {breakSummary(value)}
      </p>
    </div>
  );
}

interface FieldProps {
  label: string;
  field: BreakNumberField;
  value: number;
  error?: string;
  hint?: string;
  disabled: boolean;
  onChange: (field: BreakNumberField, raw: string) => void;
}

/** Поле минут: только цифры (D-45), подпись над полем, «мин» справа, ошибка под полем. */
function MinutesField({ label, field, value, error, hint, disabled, onChange }: FieldProps) {
  const id = useId();
  const showHint = Boolean(hint) && !error;
  const described = [showHint ? `${id}-hint` : "", error ? `${id}-error` : ""].filter(Boolean).join(" ");
  return (
    <div className={error ? "field rest-num field-invalid" : "field rest-num"}>
      <label className="field-label" htmlFor={id}>
        {label}
      </label>
      <span className="rest-num-row">
        <input
          id={id}
          className="field-input limits-num"
          type="text"
          inputMode="numeric"
          autoComplete="off"
          spellCheck={false}
          pattern="[0-9]*"
          maxLength={3}
          value={value === 0 ? "" : String(value)}
          disabled={disabled}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={described || undefined}
          onChange={(e) => onChange(field, e.target.value)}
        />
        <span className="limits-unit" aria-hidden="true">
          мин
        </span>
      </span>
      {showHint ? (
        <p id={`${id}-hint`} className="field-hint">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${id}-error`} className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
