import { useEffect, useId, useRef, type ReactNode } from "react";

interface Props {
  title: string;
  children?: ReactNode;
  /** Кнопки диалога; первая получает фокус. Диалог не тупик (L-119): действие есть всегда. */
  actions: ReactNode;
  /** Esc закрывает диалог (ТЗ 8.0 п.8). Без onClose Esc ничего не делает. */
  onClose?: () => void;
}

const FOCUSABLE = 'button:not([disabled]), input:not([disabled]), select:not([disabled]), [href], [tabindex]:not([tabindex="-1"])';

/** Модальный диалог: фокус внутри, Esc закрывает, заголовок читается скринридером. */
export default function Dialog({ title, children, actions, onClose }: Props) {
  const titleId = useId();
  const box = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const first = box.current?.querySelector<HTMLElement>(".dialog-actions button");
    (first ?? box.current)?.focus();
    return () => previous?.focus?.();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && closeRef.current) {
        e.preventDefault();
        closeRef.current();
        return;
      }
      if (e.key === "Tab" && box.current) {
        const items = Array.from(box.current.querySelectorAll<HTMLElement>(FOCUSABLE));
        if (items.length === 0) {
          return;
        }
        const firstItem = items[0];
        const lastItem = items[items.length - 1];
        const active = document.activeElement;
        if (e.shiftKey && (active === firstItem || !box.current.contains(active))) {
          e.preventDefault();
          lastItem.focus();
        } else if (!e.shiftKey && (active === lastItem || !box.current.contains(active))) {
          e.preventDefault();
          firstItem.focus();
        }
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="dialog-backdrop">
      <div ref={box} className="dialog" role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>
        <h2 id={titleId} className="dialog-title">
          {title}
        </h2>
        {children ? <div className="dialog-body">{children}</div> : null}
        <div className="dialog-actions">{actions}</div>
      </div>
    </div>
  );
}
