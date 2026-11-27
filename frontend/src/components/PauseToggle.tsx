import { useEffect, useRef, useState } from "react";
import type { PausePreset } from "../types";

interface Props {
  /** Пауза сейчас идёт: из карточки службы (status === "paused"), не локальный флаг (PL-21). */
  paused: boolean;
  /** «18:30» — конец паузы из карточки. */
  until: string;
  /** Запрос к службе идёт: значок и выбор срока недоступны. */
  busy?: boolean;
  /**
   * Клетка сетки кнопок карточки «Сегодня» (8.3.2 п. 3): выключенный значок подписан «Пауза»; во время
   * паузы значок с «Закончить паузу» и «Пауза до 18:30» занимает всю строку сетки, выбор срока — справа.
   */
  compact?: boolean;
  onPick: (preset: PausePreset) => void;
  onEnd: () => void;
}

/** Подсказки значка (D-96): что делает нажатие сейчас. */
export const PAUSE_ON_HINT = "Поставить на паузу: ограничения не действуют, время считается";
export const PAUSE_OFF_HINT = "Закончить паузу";

const PRESETS: { preset: PausePreset; label: string }[] = [
  { preset: "min15", label: "15 минут" },
  { preset: "hour1", label: "1 час" },
  { preset: "hour2", label: "2 часа" },
  { preset: "endOfDay", label: "До конца дня" },
];

/**
 * Значок-переключатель паузы на карточке ребёнка (R11, D-96). Выключен: нажатие открывает выбор
 * срока; включён (значок нажат, рядом «Пауза до 18:30»): нажатие сразу заканчивает паузу.
 */
export default function PauseToggle({ paused, until, busy = false, compact = false, onPick, onEnd }: Props) {
  const [open, setOpen] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const wrap = useRef<HTMLDivElement>(null);
  // Фокус вернуть на значок после ответа службы: отключённая кнопка его теряет (PL-68).
  const refocus = useRef(false);

  useEffect(() => {
    if (!busy && refocus.current) {
      refocus.current = false;
      button.current?.focus();
    }
  }, [busy]);

  // Пауза началась или закончилась (ответ службы, опрос): выбор срока больше не нужен. Только при
  // смене, не при появлении значка: эффект монтирования после первого нажатия закрывал выбор срока.
  const wasPaused = useRef(paused);
  useEffect(() => {
    if (wasPaused.current === paused) return;
    wasPaused.current = paused;
    setOpen(false);
  }, [paused]);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (wrap.current && !wrap.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const hint = paused ? PAUSE_OFF_HINT : PAUSE_ON_HINT;
  const onClick = () => {
    if (paused) {
      refocus.current = true;
      onEnd();
    } else {
      setOpen((v) => !v);
    }
  };

  return (
    <div
      className={compact ? "pause-toggle-wrap pause-toggle-compact" : "pause-toggle-wrap"}
      data-paused={paused ? "true" : undefined}
      ref={wrap}
      onKeyDown={(e) => {
        if (e.key === "Escape" && open) {
          e.stopPropagation();
          setOpen(false);
          button.current?.focus();
        }
      }}
    >
      <button
        ref={button}
        type="button"
        className="btn pause-toggle"
        aria-label={hint}
        title={hint}
        aria-pressed={paused}
        aria-expanded={paused ? undefined : open}
        disabled={busy}
        onClick={onClick}
      >
        <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false">
          <rect x="5" y="4" width="3.5" height="12" rx="1" />
          <rect x="11.5" y="4" width="3.5" height="12" rx="1" />
        </svg>
        {/* Во время паузы видно словами, что сделает нажатие (визуальная приёмка FB-25). */}
        {paused ? <span className="pause-toggle-label">Закончить паузу</span> : compact ? <span className="pause-toggle-label">Пауза</span> : null}
      </button>
      {paused && until ? <span className="pause-toggle-until">{`Пауза до ${until}`}</span> : null}
      {open && !paused ? (
        <div className="pause-picker" role="group" aria-label="На сколько поставить паузу">
          <p className="pause-picker-hint">Ограничения не действуют, время считается. На сколько?</p>
          {PRESETS.map((o, i) => (
            <button
              key={o.preset}
              type="button"
              className="btn btn-small"
              autoFocus={i === 0}
              disabled={busy}
              onClick={() => {
                setOpen(false);
                refocus.current = true;
                onPick(o.preset);
              }}
            >
              {o.label}
            </button>
          ))}
          <button
            type="button"
            className="btn btn-small"
            onClick={() => {
              setOpen(false);
              button.current?.focus();
            }}
          >
            Отмена
          </button>
        </div>
      ) : null}
    </div>
  );
}
