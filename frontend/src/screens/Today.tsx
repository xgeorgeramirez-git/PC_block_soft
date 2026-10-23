import { useCallback, useEffect, useRef, useState } from "react";
import { addExtra, clearExtra, getToday, grantBonus, listEvents, pause, reduceExtra, setNoForbidden, undoExtra } from "../api";
import Notice from "../components/Notice";
import PausePicker from "../components/PausePicker";
import ServiceNotices from "../components/ServiceNotices";
import HintSlot from "../guidance/HintSlot";
import { errorText } from "../lib/errors";
import { formatDateLong, formatLimit, formatUsed, localOffsetMin } from "../lib/format";
import { isViolation, startOfDayMs } from "../lib/journal";
import { canAddTime, hasTimeLimits, statusTone } from "../lib/status";
import type { PausePreset, TodayCard } from "../types";

interface Props {
  /** Ссылка «Нарушения сегодня: N». */
  onOpenJournal?: (opts: { violations: boolean }) => void;
  onOpenSettings?: () => void;
  /** Кнопка «Открыть «Дети»» у предупреждения о правилах. */
  onOpenChildren?: () => void;
  /** Кнопка «Статистика» на карточке ребёнка (FR-23, D-44). */
  onOpenStats?: (childId: number) => void;
}

interface Feedback {
  seq: number;
  text: string;
  error: boolean;
  /** Сколько мс ещё работает «Отменить». */
  undo?: { extraId: string; ms: number };
}

const EXTRA_BUTTONS = [15, 30, 60];

/** Главная «Сегодня» (FR-15, сценарий 8.3.2): карточки детей и быстрые действия. */
export default function Today({ onOpenJournal, onOpenSettings, onOpenChildren, onOpenStats }: Props) {
  const [cards, setCards] = useState<TodayCard[] | null>(null);
  const [notices, setNotices] = useState<string[]>([]);
  const [violations, setViolations] = useState<number | null>(null);
  const [loadError, setLoadError] = useState("");
  const [feedback, setFeedback] = useState<Record<number, Feedback>>({});
  const [busy, setBusy] = useState<number | null>(null);
  const alive = useRef(true);
  const seq = useRef(0);

  const load = useCallback(async () => {
    try {
      const today = await getToday();
      if (!alive.current) return;
      setCards(today.children);
      setNotices(today.notices);
      setLoadError("");
      try {
        const events = await listEvents({ fromMs: startOfDayMs(Date.now()) });
        if (alive.current) setViolations(events.filter((e) => isViolation(e.kind)).length);
      } catch {
        if (alive.current) setViolations(null);
      }
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    void load();
    // Вернулись в окно: время ребёнка шло, пока смотрели в другое место.
    const onFocus = () => void load();
    window.addEventListener("focus", onFocus);
    return () => {
      alive.current = false;
      window.removeEventListener("focus", onFocus);
    };
  }, [load]);

  const replaceCard = (card: TodayCard) =>
    setCards((prev) => (prev ?? []).map((c) => (c.childId === card.childId ? card : c)));

  const say = (childId: number, text: string, error = false, undo?: Feedback["undo"]) => {
    seq.current += 1;
    setFeedback((prev) => ({ ...prev, [childId]: { seq: seq.current, text, error, undo } }));
  };

  const run = async (childId: number, action: () => Promise<void>) => {
    setBusy(childId);
    try {
      await action();
    } catch (e) {
      if (alive.current) say(childId, errorText(e), true);
    } finally {
      if (alive.current) setBusy(null);
    }
  };

  // sayExtra показывает сообщение об изменении доп. времени с «Отменить» на 10 с (если есть).
  const sayExtra = (childId: number, reply: { message: string; extraId: string; undoUntilMs: number }) => {
    const left = reply.undoUntilMs - Date.now();
    say(childId, reply.message, false, left > 0 && reply.extraId ? { extraId: reply.extraId, ms: Math.min(left, 10_000) } : undefined);
  };

  const add = (card: TodayCard, minutes: number) =>
    run(card.childId, async () => {
      const reply = await addExtra(card.childId, minutes);
      replaceCard(reply.card);
      sayExtra(card.childId, reply);
    });

  const reduce = (card: TodayCard, minutes: number) =>
    run(card.childId, async () => {
      const reply = await reduceExtra(card.childId, minutes);
      replaceCard(reply.card);
      sayExtra(card.childId, reply);
    });

  const clear = (card: TodayCard) =>
    run(card.childId, async () => {
      const reply = await clearExtra(card.childId);
      replaceCard(reply.card);
      sayExtra(card.childId, reply);
    });

  const undo = (card: TodayCard, extraId: string) =>
    run(card.childId, async () => {
      const reply = await undoExtra(card.childId, extraId);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  const pauseChild = (card: TodayCard, preset: PausePreset) =>
    run(card.childId, async () => {
      const reply = await pause(card.childId, preset);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  const grant = (card: TodayCard, minutes: number) =>
    run(card.childId, async () => {
      const reply = await grantBonus(card.childId, minutes, "за дела по дому");
      say(card.childId, reply.message);
      await load();
    });

  const noForbidden = (card: TodayCard, on: boolean) =>
    run(card.childId, async () => {
      const reply = await setNoForbidden(card.childId, on);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  // Подсказка про кнопки «+15» одна на экран: встаёт у первой карточки, где такие кнопки есть.
  const extraHintChild = cards?.find((c) => canAddTime(c.status.status))?.childId;

  return (
    <section aria-labelledby="today-title">
      <div className="page-head">
        <h2 id="today-title" className="panel-title">
          Сегодня
        </h2>
        <span className="page-date">{formatDateLong(Date.now(), localOffsetMin())}</span>
      </div>

      <ServiceNotices notices={notices} onOpenChildren={onOpenChildren} />

      {loadError ? (
        <div className="panel">
          <Notice tone="warn" urgent>
            {loadError}
          </Notice>
          <button type="button" className="btn" onClick={() => void load()}>
            Повторить
          </button>
        </div>
      ) : cards === null ? (
        <p className="panel-text">Загружаю данные</p>
      ) : (
        <>
          {violations !== null ? (
            <p className="today-links">
              <button type="button" className="btn btn-small" onClick={() => onOpenJournal?.({ violations: true })}>
                {`Нарушения сегодня: ${violations}`}
              </button>
            </p>
          ) : null}

          {cards.length === 0 ? (
            <div className="panel">
              <p className="panel-title">Пока нет ни одного ребёнка</p>
              <p className="panel-text">
                Добавьте детскую учётную запись Windows, и здесь появится карточка с остатком времени.
              </p>
              <button type="button" className="btn btn-primary" onClick={() => onOpenSettings?.()}>
                Открыть настройки
              </button>
            </div>
          ) : (
            <div className="cards">
              {cards.map((card) => (
                <ChildCard
                  key={card.childId}
                  card={card}
                  extraHint={card.childId === extraHintChild}
                  feedback={feedback[card.childId]}
                  busy={busy === card.childId}
                  onAdd={(m) => void add(card, m)}
                  onReduce={(m) => void reduce(card, m)}
                  onClear={() => void clear(card)}
                  onUndo={(id) => void undo(card, id)}
                  onPause={(p) => void pauseChild(card, p)}
                  onGrant={(m) => void grant(card, m)}
                  onNoForbidden={(on) => void noForbidden(card, on)}
                  onStats={onOpenStats ? () => onOpenStats(card.childId) : undefined}
                />
              ))}
            </div>
          )}
        </>
      )}
    </section>
  );
}

interface CardProps {
  card: TodayCard;
  /** Здесь встаёт подсказка про кнопки добавления времени. */
  extraHint: boolean;
  feedback: Feedback | undefined;
  busy: boolean;
  onAdd: (minutes: number) => void;
  onReduce: (minutes: number) => void;
  onClear: () => void;
  onUndo: (extraId: string) => void;
  onPause: (preset: PausePreset) => void;
  onGrant: (minutes: number) => void;
  onNoForbidden: (on: boolean) => void;
  onStats?: () => void;
}

function ChildCard({ card, extraHint, feedback, busy, onAdd, onReduce, onClear, onUndo, onPause, onGrant, onNoForbidden, onStats }: CardProps) {
  const s = card.status;
  const tone = statusTone(s.status);
  const limited = hasTimeLimits(s.status);
  const extraAllowed = canAddTime(s.status);
  const titleId = `child-${card.childId}-name`;
  const [customOpen, setCustomOpen] = useState(false);
  const [customMin, setCustomMin] = useState("");
  const customN = Number.parseInt(customMin, 10);
  const customValid = Number.isInteger(customN) && customN >= 1 && customN <= 240;
  return (
    <article className="child-card" aria-labelledby={titleId} data-help-anchor="child-card">
      <header className="child-head">
        <h3 id={titleId} className="child-name">
          {card.name}
        </h3>
        <span className={`status status-${tone}`} data-status={s.status}>
          {s.statusText}
        </span>
      </header>

      <HintSlot anchor="child-card" subject={String(card.childId)} name={card.name} />

      {limited ? <p className="child-used">{formatUsed(s.usedS, s.limitS, s.unlimited)}</p> : null}
      {card.extraMin > 0 ? <p className="child-meta">{`Добавлено сегодня: ${card.extraMin} мин`}</p> : null}
      {card.extraMin < 0 ? <p className="child-meta">{`Убрано сегодня: ${-card.extraMin} мин`}</p> : null}
      {card.bonusBalance > 0 ? <p className="child-meta">{`Копилка: ${formatLimit(card.bonusBalance)}`}</p> : null}
      {s.status === "paused" && s.pauseUntil ? <p className="child-meta">{`Пауза до ${s.pauseUntil}`}</p> : null}
      {s.lock !== "none" && s.until ? <p className="child-meta">{s.until}</p> : null}

      <div className="child-actions">
        {extraAllowed ? (
          <div className="action-group" data-help-anchor="extra-buttons">
            <span className="action-group-label">Доп. время</span>
            <div className="action-row">
              {EXTRA_BUTTONS.map((m) => (
                <button key={`add-${m}`} type="button" className="btn" disabled={busy} onClick={() => onAdd(m)}>
                  {`+${m}`}
                </button>
              ))}
              {EXTRA_BUTTONS.map((m) => (
                <button key={`sub-${m}`} type="button" className="btn btn-muted" disabled={busy} onClick={() => onReduce(m)}>
                  {`−${m}`}
                </button>
              ))}
              <button type="button" className="btn" disabled={busy} aria-expanded={customOpen} onClick={() => setCustomOpen((o) => !o)}>
                Своё время…
              </button>
            </div>
            {customOpen ? (
              <div className="child-custom">
                <label className="child-custom-label">
                  Минут:
                  <input
                    type="number"
                    min={1}
                    max={240}
                    inputMode="numeric"
                    className="child-custom-input"
                    value={customMin}
                    disabled={busy}
                    onChange={(e) => setCustomMin(e.target.value)}
                  />
                </label>
                <button type="button" className="btn" disabled={busy || !customValid} onClick={() => onAdd(customN)}>
                  Добавить
                </button>
                <button type="button" className="btn btn-muted" disabled={busy || !customValid} onClick={() => onReduce(customN)}>
                  Убавить
                </button>
                <button type="button" className="btn btn-muted" disabled={busy} onClick={onClear}>
                  Убрать всё
                </button>
              </div>
            ) : null}
          </div>
        ) : null}

        <div className="action-row action-row-foot">
          <PausePicker disabled={busy} onPick={onPause} />
          <button type="button" className="btn" disabled={busy} onClick={() => onGrant(30)} data-help-anchor="child-card-bonus">
            В копилку +30
          </button>
          {card.noForbiddenToday ? (
            <button type="button" className="btn btn-muted" disabled={busy} onClick={() => onNoForbidden(false)} data-help-anchor="no-forbidden">
              Вернуть запретные часы
            </button>
          ) : (
            <button type="button" className="btn" disabled={busy} onClick={() => onNoForbidden(true)} data-help-anchor="no-forbidden">
              Снять запрет на сегодня
            </button>
          )}
          {onStats ? (
            <button type="button" className="btn" disabled={busy} onClick={onStats}>
              Статистика
            </button>
          ) : null}
        </div>
        {card.noForbiddenToday ? (
          <p className="child-meta">Сегодня запретные часы не действуют</p>
        ) : null}
      </div>

      {extraHint ? <HintSlot anchor="extra-buttons" /> : null}

      {feedback ? <FeedbackLine key={feedback.seq} feedback={feedback} onUndo={onUndo} /> : null}
    </article>
  );
}

function FeedbackLine({ feedback, onUndo }: { feedback: Feedback; onUndo: (extraId: string) => void }) {
  const [canUndo, setCanUndo] = useState(Boolean(feedback.undo));
  useEffect(() => {
    if (!feedback.undo) return;
    const id = setTimeout(() => setCanUndo(false), feedback.undo.ms);
    return () => clearTimeout(id);
  }, [feedback]);

  if (feedback.error) {
    return (
      <p className="field-error child-feedback" role="alert">
        {feedback.text}
      </p>
    );
  }
  return (
    <div className="child-feedback" role="status">
      <span>{feedback.text}</span>
      {canUndo && feedback.undo ? (
        <button type="button" className="link" onClick={() => onUndo(feedback.undo!.extraId)}>
          Отменить
        </button>
      ) : null}
    </div>
  );
}
