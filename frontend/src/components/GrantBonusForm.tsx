import { useState } from "react";

interface Props {
  disabled?: boolean;
  /** Ошибка от службы у поля минут (например «Можно начислить не больше N мин»). */
  minutesError?: string;
  onGrant: (minutes: number, reason: string) => void;
}

const REASONS = ["за уборку", "за уроки", "за помощь"];

/** Форма начисления в копилку (FR-22): минуты 1..600 и причина с подсказками. */
export default function GrantBonusForm({ disabled, minutesError, onGrant }: Props) {
  const [minutes, setMinutes] = useState("30");
  const [reason, setReason] = useState("");
  const [localError, setLocalError] = useState("");

  const submit = () => {
    const m = Number(minutes);
    if (!Number.isInteger(m) || m < 1 || m > 600) {
      setLocalError("Можно начислить от 1 до 600 минут");
      return;
    }
    if (!reason.trim()) {
      setLocalError("Укажите, за что начисляете");
      return;
    }
    setLocalError("");
    onGrant(m, reason.trim());
  };

  const error = localError || minutesError;

  return (
    <div className="bonus-grant" data-help-anchor="bonus-grant">
      <label className="field">
        <span className="field-label">Сколько минут</span>
        <input
          className={error ? "field-input field-input-error" : "field-input"}
          type="number"
          inputMode="numeric"
          min={1}
          max={600}
          value={minutes}
          disabled={disabled}
          onChange={(e) => setMinutes(e.target.value)}
        />
      </label>
      <label className="field">
        <span className="field-label">За что</span>
        <input
          className="field-input"
          list="bonus-reasons"
          value={reason}
          disabled={disabled}
          placeholder="за уборку"
          onChange={(e) => setReason(e.target.value)}
        />
        <datalist id="bonus-reasons">
          {REASONS.map((r) => (
            <option key={r} value={r} />
          ))}
        </datalist>
      </label>
      <button type="button" className="btn btn-primary" disabled={disabled} onClick={submit}>
        Начислить
      </button>
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
