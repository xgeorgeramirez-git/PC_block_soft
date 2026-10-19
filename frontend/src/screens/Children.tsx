import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { getToday } from "../api";
import Notice from "../components/Notice";
import { useAttemptNavigation } from "../guard";
import { errorText } from "../lib/errors";
import type { TodayCard } from "../types";
import ChildLimits from "./ChildLimits";

interface Props {
  onOpenSettings: () => void;
}

/** Раздел «Дети» (сценарий 8.3.3): выбор ребёнка и его лимиты. Другой ребёнок с правками - вопрос. */
export default function Children({ onOpenSettings }: Props) {
  const [children, setChildren] = useState<TodayCard[] | null>(null);
  const [selected, setSelected] = useState<number | null>(null);
  const [error, setError] = useState("");
  const attempt = useAttemptNavigation();
  const alive = useRef(true);
  const tabs = useRef(new Map<number, HTMLButtonElement>());

  const load = useCallback(async () => {
    setError("");
    try {
      const today = await getToday();
      if (!alive.current) return;
      setChildren(today.children);
      setSelected((prev) => (today.children.some((c) => c.childId === prev) ? prev : (today.children[0]?.childId ?? null)));
    } catch (e) {
      if (alive.current) setError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  if (error) {
    return (
      <section className="panel">
        <Notice tone="warn" urgent>
          {error}
        </Notice>
        <button type="button" className="btn" onClick={() => void load()}>
          Повторить
        </button>
      </section>
    );
  }
  if (children === null) {
    return <p className="panel-text">Загружаю данные</p>;
  }
  if (children.length === 0) {
    return (
      <section className="panel">
        <p className="panel-title">Пока нет ни одного ребёнка</p>
        <p className="panel-text">Добавьте детскую учётную запись Windows в разделе «Настройки», и здесь появятся её лимиты.</p>
        <button type="button" className="btn btn-primary" onClick={onOpenSettings}>
          Открыть настройки
        </button>
      </section>
    );
  }

  // Стрелки влево и вправо переключают ребёнка, как на любых вкладках.
  const onKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
    e.preventDefault();
    const step = e.key === "ArrowRight" ? 1 : children.length - 1;
    const next = children[(index + step) % children.length];
    attempt(() => {
      setSelected(next.childId);
      tabs.current.get(next.childId)?.focus();
    });
  };

  return (
    <div>
      {children.length > 1 ? (
        <div className="tabs" role="tablist" aria-label="Дети">
          {children.map((c, i) => (
            <button
              key={c.childId}
              ref={(el) => {
                if (el) tabs.current.set(c.childId, el);
                else tabs.current.delete(c.childId);
              }}
              id={`child-tab-${c.childId}`}
              type="button"
              role="tab"
              className="tab"
              aria-selected={c.childId === selected}
              aria-controls="child-panel"
              tabIndex={c.childId === selected ? 0 : -1}
              onClick={() => attempt(() => setSelected(c.childId))}
              onKeyDown={(e) => onKey(e, i)}
            >
              {c.name}
            </button>
          ))}
        </div>
      ) : null}
      {selected !== null ? (
        <div id="child-panel" role={children.length > 1 ? "tabpanel" : undefined} aria-labelledby={children.length > 1 ? `child-tab-${selected}` : undefined}>
          <ChildLimits key={selected} childId={selected} />
        </div>
      ) : null}
    </div>
  );
}
