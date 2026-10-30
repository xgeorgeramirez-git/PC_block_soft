import { useCallback, useEffect, useRef, useState } from "react";
import { clearBonus, getBonus, getBonusRate, getToday, grantBonus, reduceBonus, undoBonus } from "../api";
import Dialog from "../components/Dialog";
import GrantBonusForm from "../components/GrantBonusForm";
import Notice from "../components/Notice";
import PromoAdmin from "../components/PromoAdmin";
import { autoBonusRule, autoBonusText, historyText, isAutoBonus } from "../lib/bonus";
import { CallError, errorText } from "../lib/errors";
import { formatEventTime, formatLimit, genitiveName } from "../lib/format";
import { clampDigits } from "../lib/numinput";
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

type Tab = "piggy" | "bonuses";

/** Что спрашиваем в подтверждении (D-46): обнулить или убавить копилку ребёнка. */
type Confirm = { kind: "clear" | "reduce"; childId: number; name: string; balance: number };

/**
 * Копилка (FR-22, сценарий 8.3.6): баланс каждого ребёнка, начисление, обнулить и убавить
 * (D-46), история. Вкладка «Бонусы»: автобонусы (R25, D-49) и промокоды (FR-35, R26, D-50).
 */
export default function Bonus() {
  const [tab, setTab] = useState<Tab>("piggy");
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [rate, setRate] = useState<number | null>(null);
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
    try {
      const r = await getBonusRate();
      if (alive.current) setRate(r.minutes);
    } catch {
      // Без настройки вкладка «Бонусы» просто не покажет правило: это не повод для ошибки.
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

  // Обнулить или убавить (D-46): только после подтверждения; итог — сообщением у карточки.
  const applyConfirm = async (minutes: number) => {
    if (!confirm) return;
    const { kind, childId } = confirm;
    setConfirm(null);
    setBusy(childId);
    try {
      const reply = kind === "clear" ? await clearBonus(childId) : await reduceBonus(childId, minutes);
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

      <div className="tabs" role="tablist" aria-label="Копилка и бонусы">
        <button type="button" role="tab" className="tab" aria-selected={tab === "piggy"} onClick={() => setTab("piggy")}>
          Копилка
        </button>
        <button
          type="button"
          role="tab"
          className="tab"
          aria-selected={tab === "bonuses"}
          onClick={() => setTab("bonuses")}
        >
          Бонусы
        </button>
      </div>

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
      ) : tab === "piggy" ? (
        <div role="tabpanel" aria-label="Копилка">
          {items.length === 0 ? (
            <p className="panel-text">Пока нет ни одного ребёнка.</p>
          ) : (
            <div className="cards">
              {items.map((it) => (
                <article key={it.card.childId} className="panel bonus-page" aria-label={`Копилка, ${it.card.name}`}>
                  <h3 className="child-name">{it.card.name}</h3>
                  <p className="bonus-balance">{`Баланс: ${formatLimit(it.balance)}`}</p>
                  <GrantBonusForm
                    disabled={busy === it.card.childId}
                    minutesError={fieldError[it.card.childId]}
                    onGrant={(m, r) => void grant(it.card.childId, m, r)}
                  />
                  <div className="bonus-adjust">
                    <button
                      type="button"
                      className="btn"
                      disabled={busy === it.card.childId || it.balance <= 0}
                      onClick={() =>
                        setConfirm({ kind: "reduce", childId: it.card.childId, name: it.card.name, balance: it.balance })
                      }
                    >
                      Убавить
                    </button>
                    <button
                      type="button"
                      className="btn"
                      disabled={busy === it.card.childId || it.balance <= 0}
                      onClick={() =>
                        setConfirm({ kind: "clear", childId: it.card.childId, name: it.card.name, balance: it.balance })
                      }
                    >
                      Обнулить копилку
                    </button>
                    {it.balance <= 0 ? <span className="field-hint">Копилка пуста: убавлять нечего</span> : null}
                  </div>
                  {feedback[it.card.childId] ? (
                    <BonusFeedback feedback={feedback[it.card.childId]} onUndo={(id) => void undo(it.card.childId, id)} />
                  ) : null}
                  <BonusHistory entries={it.entries} />
                </article>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div role="tabpanel" aria-label="Бонусы">
          <section className="panel" aria-label="Автобонусы">
            <h3 className="section-title">Автобонусы за неизрасходованное время</h3>
            {rate !== null ? <p className="panel-text">{autoBonusRule(rate)}</p> : null}
            {items.map((it) => (
              <AutoBonusList key={it.card.childId} name={it.card.name} entries={it.entries} />
            ))}
          </section>
          <PromoAdmin />
        </div>
      )}

      {confirm ? <ConfirmDialog confirm={confirm} onCancel={() => setConfirm(null)} onApply={(m) => void applyConfirm(m)} /> : null}
    </section>
  );
}

const REDUCE_STEPS = [15, 30, 60];

/** Подтверждение «Обнулить копилку?» или «Убавить копилку?» с выбором −15/−30/−60/своё (D-46). */
function ConfirmDialog({ confirm, onCancel, onApply }: { confirm: Confirm; onCancel: () => void; onApply: (minutes: number) => void }) {
  const [step, setStep] = useState<number | "own">(15);
  const [own, setOwn] = useState(10);
  const minutes = step === "own" ? own : step;
  const who = genitiveName(confirm.name);
  if (confirm.kind === "clear") {
    return (
      <Dialog
        title="Обнулить копилку?"
        onClose={onCancel}
        actions={
          <>
            <button type="button" className="btn btn-primary" onClick={() => onApply(0)}>
              Обнулить
            </button>
            <button type="button" className="btn" onClick={onCancel}>
              Отмена
            </button>
          </>
        }
      >
        <p>{`Баланс ${who} станет 0 мин (сейчас ${confirm.balance} мин). Запись попадёт в журнал.`}</p>
      </Dialog>
    );
  }
  return (
    <Dialog
      title="Убавить копилку?"
      onClose={onCancel}
      actions={
        <>
          <button type="button" className="btn btn-primary" disabled={minutes < 1} onClick={() => onApply(minutes)}>
            {`Убавить на ${minutes} мин`}
          </button>
          <button type="button" className="btn" onClick={onCancel}>
            Отмена
          </button>
        </>
      }
    >
      <p>{`Сейчас у ${who} ${confirm.balance} мин. Ниже нуля копилка не опустится. Запись попадёт в журнал.`}</p>
      <div className="reduce-steps" role="radiogroup" aria-label="На сколько убавить">
        {REDUCE_STEPS.map((m) => (
          <label key={m} className="reduce-step">
            <input type="radio" name="reduce" checked={step === m} onChange={() => setStep(m)} />
            <span>{`−${m}`}</span>
          </label>
        ))}
        <label className="reduce-step">
          <input type="radio" name="reduce" checked={step === "own"} onChange={() => setStep("own")} />
          <span>Своё</span>
        </label>
      </div>
      {step === "own" ? (
        <label className="field">
          <span className="field-label">Сколько минут убавить</span>
          <input
            className="field-input"
            type="text"
            inputMode="numeric"
            pattern="[0-9]*"
            maxLength={3}
            value={own}
            onChange={(e) => setOwn(clampDigits(e.target.value, 600))}
          />
        </label>
      ) : null}
    </Dialog>
  );
}

/** Автобонусы одного ребёнка простыми словами (R25, D-49). */
function AutoBonusList({ name, entries }: { name: string; entries: BonusEntry[] }) {
  const auto = entries.filter(isAutoBonus);
  return (
    <section className="auto-bonus" aria-label={`Автобонусы, ${name}`}>
      <h4 className="child-name">{name}</h4>
      {auto.length === 0 ? (
        <p className="panel-text">Автобонусов пока не было. Они начисляются в полночь за прошедший день</p>
      ) : (
        <ul className="bonus-history">
          {auto.map((e, i) => (
            <li key={i} className="bonus-history-item">
              <span>{formatEventTime(e.atMs, e.tzOffsetMin)}</span>
              <span>{autoBonusText(e)}</span>
            </li>
          ))}
        </ul>
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
          <span>{historyText(e)}</span>
          <span>{`Баланс: ${formatLimit(e.balanceAfter)}`}</span>
        </li>
      ))}
    </ul>
  );
}
