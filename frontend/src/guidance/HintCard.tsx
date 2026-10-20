// Карточка подсказки (ТЗ 8.4 п.1): встаёт в поток страницы на своём месте и ничего не перекрывает.
// Заголовок, одна-две строки, до двух действий, «Позже», крестик; у повторяющихся ещё
// «Больше не напоминать». Esc равен «Позже».
import { useEffect, useRef } from "react";
import { LABELS, fillName, type ActionId } from "./catalog";
import type { Candidate } from "./engine";
import { IconClose, IconInfo, IconWarn } from "./icons";

interface Props {
  candidate: Candidate;
  onLater: () => void;
  onClose: () => void;
  onMute: () => void;
  onAction: (action: ActionId) => void;
}

/** Поле ввода: Esc там принадлежит человеку (отменить правку), карточку не трогаем. */
function isTyping(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName);
}

export default function HintCard({ candidate, onLater, onClose, onMute, onAction }: Props) {
  const { entry, name } = candidate;
  const later = useRef(onLater);
  later.current = onLater;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || isTyping(e.target)) return;
      later.current();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const risk = entry.kind === "detector";
  return (
    <aside
      className={risk ? "hint-card hint-card-risk" : "hint-card"}
      role="status"
      aria-live="polite"
      data-hint-id={entry.id}
    >
      <span className="hint-icon">{risk ? <IconWarn /> : <IconInfo />}</span>
      <div className="hint-main">
        <p className="hint-title">{fillName(entry.title, name)}</p>
        <p className="hint-body">{fillName(entry.body, name)}</p>
        <div className="hint-actions">
          {entry.actions.map((a) => (
            <button key={a.id} type="button" className="btn btn-small btn-primary" onClick={() => onAction(a.id)}>
              {a.label}
            </button>
          ))}
          <button type="button" className="btn btn-small" onClick={onLater}>
            {LABELS.later}
          </button>
          {entry.repeat ? (
            <button type="button" className="link hint-mute" onClick={onMute}>
              {LABELS.mute}
            </button>
          ) : null}
        </div>
      </div>
      <button type="button" className="hint-close" aria-label={LABELS.close} onClick={onClose}>
        <IconClose />
      </button>
    </aside>
  );
}
