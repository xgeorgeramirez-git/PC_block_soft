import { createContext, useContext } from "react";
import { createPortal } from "react-dom";

/** Место внизу окна, куда встаёт панель сохранения: она прижата к нижнему краю, а страница над ней прокручивается. */
export const SaveBarSlotContext = createContext<HTMLElement | null>(null);

interface Props {
  dirty: boolean;
  saving: boolean;
  /**
   * Итог последнего действия: «Сохранено. Новые правила уже действуют»; при правках - пояснение к
   * ним («Правила скопированы. Проверьте и сохраните»), оно видно рядом со словами о правках.
   */
  savedText?: string;
  /** Общая ошибка сохранения (не у поля): видна там, куда смотрит родитель. */
  error?: string;
  onSave: () => void;
  onDiscard: () => void;
}

/** Единственная панель «Сохранить» страницы (FR-19, L-44). Без правок и без итога её нет. */
export default function SaveBar({ dirty, saving, savedText, error, onSave, onDiscard }: Props) {
  const slot = useContext(SaveBarSlotContext);
  if (!dirty && !savedText) return null;

  const bar = (
    <div className="savebar" role="region" aria-label="Сохранение изменений">
      <div className="savebar-main">
        {dirty ? (
          <>
            <span className="savebar-text">Есть несохранённые изменения</span>
            {savedText ? (
              <span className="savebar-note" role="status">
                {savedText}
              </span>
            ) : null}
          </>
        ) : (
          <span className="savebar-text savebar-saved" role="status">
            {savedText}
          </span>
        )}
        {error ? (
          <span className="savebar-error" role="alert">
            {error}
          </span>
        ) : null}
      </div>
      {dirty ? (
        <div className="savebar-actions">
          <button type="button" className="btn" onClick={onDiscard} disabled={saving}>
            Отменить изменения
          </button>
          <button type="button" className="btn btn-primary" onClick={onSave} disabled={saving}>
            Сохранить
          </button>
        </div>
      ) : null}
    </div>
  );
  return slot ? createPortal(bar, slot) : bar;
}
