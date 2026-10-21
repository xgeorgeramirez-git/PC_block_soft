import { useCallback, useEffect, useRef, useState } from "react";
import { getBonus, getToday, grantBonus, undoBonus } from "../api";
import GrantBonusForm from "../components/GrantBonusForm";
import Notice from "../components/Notice";
import HintSlot from "../guidance/HintSlot";
import { CallError, errorText } from "../lib/errors";
import { formatEventTime, formatLimit } from "../lib/format";
import type { BonusEntry, TodayCard } from "../types";

interface ChildBonus {
  card: TodayCard;
  balance: number;
  entries: BonusEntry[];
}

interface Feedback {
  seq: number;
  text: string;
  error: boolean;
  undo?: { bonusId: string; ms: number };
}

/** Копилка (FR-22, сценарий 8.3.6): баланс каждого ребёнка, начисление, история. */
export default function Bonus() {
  const [items, setItems] = useState<ChildBonus[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [feedback, setFeedback] = useState<Record<number, Feedback>>({});
  const [fieldError, setFieldError] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState<number | null>(null);
  const alive = useRef(true);
  const seq = useRef(0);

  const load = useCallback(async () => {
    try {
      const today = await getToday();
      if (!alive.current) return;
      const withBonus = await Promise.all(
        today.children.map(async (card) => {
          const b = await getBonus(card.childId);
          return { card, balance: b.balance, entries: b.entries };
        }),
      );
      if (alive.current) {
        setItems(withBonus);
        setLoadError("");
      }
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    return () => {
      alive.current = false;
    };
  }, [load]);

  const say = (childId: number, text: string, error = false, undo?: Feedback["undo"]) => {
    seq.current += 1;
    setFeedback((prev) => ({ ...prev, [childId]: { seq: seq.current, text, error, undo } }));
  };

  const grant = async (childId: number, minutes: number, reason: string) => {
    setBusy(childId);
    setFieldError((prev) => ({ ...prev, [childId]: "" }));
    try {
      const reply = await grantBonus(childId, minutes, reason);
      const left = reply.undoUntilMs - Date.now();
      say(childId, reply.message, false, left > 0 ? { bonusId: reply.bonusId, ms: Math.min(left, 10_000) } : undefined);
      await load();
    } catch (e) {
      if (!alive.current) return;
      if (e instanceof CallError && e.field === "minutes") {
        setFieldError((prev) => ({ ...prev, [childId]: errorText(e) }));
      } else {
        say(childId, errorText(e), true);
      }
    } finally {
      if (alive.current) setBusy(null);
    }
  };

  const undo = async (childId: number, bonusId: string) => {
    setBusy(childId);
    try {
      const reply = await undoBonus(childId, bonusId);
      say(childId, reply.message);
      await load();
    } catch (e) {
      if (alive.current) say(childId, errorText(e), true);
    } finally {
      if (alive.current) setBusy(null);
    }
  };

  return (
    <section aria-labelledby="bonus-title">
      <div className="page-head">
        <h2 id="bonus-title" className="panel-title">
          Копилка
        </h2>
      </div>
      <p className="panel-text">Бонусные минуты за дела по дому и учёбу. Ребёнок берёт их сам, когда время вышло.</p>

      {loadError ? (
        <div className="panel">
          <Notice tone="warn" urgent>
            {loadError}
          </Notice>
          <button type="button" className="btn" onClick={() => void load()}>
            Повторить
          </button>
        </div>
      ) : items === null ? (
        <p className="panel-text">Загружаю данные</p>
      ) : items.length === 0 ? (
        <p className="panel-text">Пока нет ни одного ребёнка.</p>
      ) : (
        <div className="cards">
          {items.map((it) => (
            <article key={it.card.childId} className="panel bonus-page" aria-label={`Копилка, ${it.card.name}`}>
              <h3 className="child-name">{it.card.name}</h3>
              <p className="bonus-balance">{`Баланс: ${formatLimit(it.balance)}`}</p>
              <HintSlot anchor="bonus-grant" subject={String(it.card.childId)} name={it.card.name} />
              <GrantBonusForm
                disabled={busy === it.card.childId}
                minutesError={fieldError[it.card.childId]}
                onGrant={(m, r) => void grant(it.card.childId, m, r)}
              />
              {feedback[it.card.childId] ? (
                <BonusFeedback feedback={feedback[it.card.childId]} onUndo={(id) => void undo(it.card.childId, id)} />
              ) : null}
              <BonusHistory entries={it.entries} />
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function BonusFeedback({ feedback, onUndo }: { feedback: Feedback; onUndo: (bonusId: string) => void }) {
  const [canUndo, setCanUndo] = useState(Boolean(feedback.undo));
  useEffect(() => {
    if (!feedback.undo) return;
    const id = setTimeout(() => setCanUndo(false), feedback.undo.ms);
    return () => clearTimeout(id);
  }, [feedback]);
  if (feedback.error) {
    return (
      <p className="field-error" role="alert">
        {feedback.text}
      </p>
    );
  }
  return (
    <div className="child-feedback" role="status">
      <span>{feedback.text}</span>
      {canUndo && feedback.undo ? (
        <button type="button" className="btn btn-small" onClick={() => onUndo(feedback.undo!.bonusId)}>
          Отменить
        </button>
      ) : null}
    </div>
  );
}

function BonusHistory({ entries }: { entries: BonusEntry[] }) {
  if (entries.length === 0) {
    return <p className="panel-text">Начислений и трат пока нет.</p>;
  }
  return (
    <ul className="bonus-history">
      {entries.map((e, i) => (
        <li key={i} className="bonus-history-item">
          <span>{formatEventTime(e.atMs, e.tzOffsetMin)}</span>
          <span>{e.deltaMin > 0 ? `+${e.deltaMin} мин: ${e.reason}` : `${e.deltaMin} мин: потрачено`}</span>
          <span>{`Баланс: ${formatLimit(e.balanceAfter)}`}</span>
        </li>
      ))}
    </ul>
  );
}
