import { forwardRef, useId } from "react";

interface Props {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: "text" | "password";
  /** Ошибка у поля (L-121): рамка и текст под полем. */
  error?: string | null;
  hint?: string;
  disabled?: boolean;
  autoFocus?: boolean;
  autoComplete?: string;
  maxLength?: number;
}

/** Поле с подписью, подсказкой и ошибкой под ним. Ошибка читается скринридером сразу. */
const TextField = forwardRef<HTMLInputElement, Props>(function TextField(
  { label, value, onChange, type = "text", error, hint, disabled, autoFocus, autoComplete, maxLength },
  ref,
) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const described = [hint ? hintId : "", error ? errorId : ""].filter(Boolean).join(" ");
  return (
    <div className={error ? "field field-invalid" : "field"}>
      <label className="field-label" htmlFor={id}>
        {label}
      </label>
      <input
        id={id}
        ref={ref}
        className="field-input"
        type={type}
        value={value}
        disabled={disabled}
        autoFocus={autoFocus}
        autoComplete={autoComplete}
        maxLength={maxLength}
        aria-invalid={error ? "true" : undefined}
        aria-describedby={described || undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint ? (
        <p id={hintId} className="field-hint">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="field-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
});

export default TextField;
