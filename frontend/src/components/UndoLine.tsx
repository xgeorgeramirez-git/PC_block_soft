import { useEffect, useRef, useState } from "react";

interface Props {
  text: string;
  /** Ошибка читается сразу (alert) и без «Отменить». */
  error?: boolean;
  /** Сколько мс работает «Отменить» (урок L-74: 10 с); нет - кнопки нет. */
  undoMs?: number;
  onUndo?: () => void;
  /** «Отменить» недоступна, пока идёт другой запрос. */
  disabled?: boolean;
  className?: string;
  /**
   * Своя живая область (role="status"). false - сообщение стоит внутри постоянной области
   * родителя, которая есть в DOM до сообщения (иначе диктор может пропустить первое).
   */
  live?: boolean;
}

/**
 * Сообщение о результате действия с кнопкой «Отменить» на 10 с (одно оформление во всех
 * разделах: ссылка и отсчёт секунд рядом, B-50) («Добавлено 30 минут»,
 * «SuperGame: игра») и отсчётом секунд рядом с кнопкой. Пока фокус внутри сообщения, кнопка не пропадает: до неё можно дойти
 * клавишей Tab и нажать, не теряя фокус. Новое сообщение - новый ключ у родителя.
 */
export default function UndoLine({
  text,
  error = false,
  undoMs,
  onUndo,
  disabled = false,
  className = "",
  live = true,
}: Props) {
  const withUndo = Boolean(onUndo && undoMs && undoMs > 0);
  const [expired, setExpired] = useState(!withUndo);
  const [focused, setFocused] = useState(false);
  /** Сколько целых секунд осталось у «Отменить» (B-43): видно, сколько ещё можно передумать. */
  const [left, setLeft] = useState(withUndo ? Math.ceil((undoMs ?? 0) / 1000) : 0);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!withUndo) return;
    const id = setTimeout(() => setExpired(true), undoMs);
    // Первый шаг - на остаток доли секунды, дальше ровно по секунде: «7 с» сменяется на «6 с», когда
    // до конца осталось 6 с.
    const ms = undoMs ?? 0;
    let tick: ReturnType<typeof setInterval> | undefined;
    const first = setTimeout(() => {
      setLeft((n) => n - 1);
      tick = setInterval(() => setLeft((n) => Math.max(0, n - 1)), 1000);
    }, ms % 1000 || 1000);
    return () => {
      clearTimeout(id);
      clearTimeout(first);
      if (tick) clearInterval(tick);
    };
  }, [withUndo, undoMs]);

  if (error) {
    return (
      <p className={`field-error ${className}`.trim()} role="alert">
        {text}
      </p>
    );
  }
  const showUndo = withUndo && (!expired || focused);
  return (
    <div
      ref={box}
      className={className || undefined}
      role={live ? "status" : undefined}
      onFocus={() => setFocused(true)}
      onBlur={(e) => {
        if (!box.current?.contains(e.relatedTarget as Node | null)) setFocused(false);
      }}
    >
      <span>{text}</span>
      {showUndo ? (
        <button type="button" className="link" disabled={disabled} onClick={() => onUndo?.()}>
          Отменить
        </button>
      ) : null}
      {/* Отсчёт только для глаз: диктор не зачитывает каждую секунду живой области. */}
      {showUndo && !expired && left > 0 ? (
        <span className="undo-left" aria-hidden="true">{`${left} с`}</span>
      ) : null}
    </div>
  );
}
