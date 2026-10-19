import Notice from "./Notice";
import { humanNotice } from "../lib/notices";

interface Props {
  /** Важные сообщения службы при входе (GetToday.notices). */
  notices: string[];
  /** Открыть раздел «Дети»: кнопка рядом с предупреждением о правилах. */
  onOpenChildren?: () => void;
}

/** Предупреждения службы человеческими словами; к сообщению о правилах - кнопка в «Дети» (R18). */
export default function ServiceNotices({ notices, onOpenChildren }: Props) {
  return (
    <>
      {notices.map((raw) => {
        const n = humanNotice(raw);
        return (
          <Notice key={raw} tone="warn" urgent>
            <div className="notice-row">
              <span className="notice-text">{n.text}</span>
              {n.action === "children" && onOpenChildren ? (
                <button type="button" className="btn btn-small" onClick={onOpenChildren}>
                  Открыть «Дети»
                </button>
              ) : null}
            </div>
          </Notice>
        );
      })}
    </>
  );
}
