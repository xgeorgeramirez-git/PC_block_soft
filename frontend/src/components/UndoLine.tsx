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
}

/**
 * Сообщение о результате действия с кнопкой «Отменить» на 10 с («Добавлено 30 минут»,
 * «SuperGame: игра»). Пока фокус внутри сообщения, кнопка не пропадает: до неё можно дойти
 * клавишей Tab и нажать, не теряя фокус. Новое сообщение - новый ключ у родителя.
 */
export default function UndoLine({ text, error = false, undoMs, onUndo, disabled = false, className = "" }: Props) {
  const withUndo = Boolean(onUndo && undoMs && undoMs > 0);
  const [expired, setExpired] = useState(!withUndo);
  const [focused, setFocused] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!withUndo) return;
    const id = setTimeout(() => setExpired(true), undoMs);
    return () => clearTimeout(id);
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
      role="status"
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
    </div>
  );
}
