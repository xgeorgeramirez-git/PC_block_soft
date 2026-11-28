import { useCallback, useEffect, useRef, useState } from "react";
import {
  addExtra,
  clearBlock,
  clearExtra,
  endPause,
  getToday,
  grantBonus,
  listEvents,
  listEventsPage,
  pause,
  reduceExtra,
  setNoForbidden,
  undoBonus,
  undoExtra,
} from "../api";
import GrantBonusForm from "../components/GrantBonusForm";
import { Coin, Icon } from "../components/Icon";
import Notice from "../components/Notice";
import PauseToggle from "../components/PauseToggle";
import ServiceNotices from "../components/ServiceNotices";
import UndoLine from "../components/UndoLine";
import { CallError, errorText } from "../lib/errors";
import { formatDateLong, formatEventTime, formatTimeOfDay, localOffsetMin, sameLocalDay } from "../lib/format";
import { isViolation, startOfDayMs } from "../lib/journal";
import { formatHM, keepUnits, limitLine, pendingLead, programsWaitText, remainMinutes, remainShare, remainTone } from "../lib/remain";
import { canAddTime, hasTimeLimits, statusTone } from "../lib/status";
import type { ChildStatus, JournalEntry, PausePreset, TodayCard } from "../types";

interface Props {
  /** Ссылка «Нарушения сегодня: N» карточки (с ребёнком) и «Весь журнал» (без фильтра). */
  onOpenJournal?: (opts: { violations: boolean; childId?: number }) => void;
  onOpenSettings?: () => void;
  /** Кнопка «Открыть «Дети»» у предупреждения о правилах. */
  onOpenChildren?: () => void;
  /** Кнопка «Статистика» на карточке ребёнка (FR-23, D-44). */
  onOpenStats?: (childId: number) => void;
  /** «Разобрать» у блока «N программ ждут решения» (8.3.2 п. 3, 5). */
  onOpenPrograms?: () => void;
}

interface Feedback {
  seq: number;
  text: string;
  error: boolean;
  /** Сколько мс ещё работает «Отменить» и что она отменяет: доп. время или начисление в копилку. */
  undo?: { kind: "extra" | "bonus"; id: string; ms: number };
  /** Только для диктора: результат уже виден на карточке (пауза — «Пауза до 18:30» у значка). */
  quiet?: boolean;
}

const EXTRA_BUTTONS = [15, 30, 60];
/** Строк в «Последних событиях» (8.3.2 п. 3: 4–5 строк журнала). */
const RECENT_EVENTS = 5;
/** Фон аватара по порядку детей (макет parent2.py). */
const AVATAR_BG = ["#FFE7AD", "#D6E4FF", "#DDF3E4"];

/** Главная «Сегодня» (FR-15, сценарий 8.3.2): карточки детей, ждущие программы и последние события. */
export default function Today({ onOpenJournal, onOpenSettings, onOpenChildren, onOpenStats, onOpenPrograms }: Props) {
  const [cards, setCards] = useState<TodayCard[] | null>(null);
  const [notices, setNotices] = useState<string[]>([]);
  const [pending, setPending] = useState<{ names: string[]; total: number }>({ names: [], total: 0 });
  /** Нарушения сегодня по детям; null — журнал не прочитался (ссылок нет). */
  const [violations, setViolations] = useState<Map<number, number> | null>(null);
  const [recent, setRecent] = useState<JournalEntry[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [feedback, setFeedback] = useState<Record<number, Feedback>>({});
  const [grantError, setGrantError] = useState<Record<number, string>>({});
  const [busy, setBusy] = useState<number | null>(null);
  const alive = useRef(true);
  const seq = useRef(0);

  const loadJournal = useCallback(async () => {
    try {
      const events = await listEvents({ fromMs: startOfDayMs(Date.now()) });
      if (!alive.current) return;
      const byChild = new Map<number, number>();
      for (const e of events) if (isViolation(e.kind)) byChild.set(e.childId, (byChild.get(e.childId) ?? 0) + 1);
      setViolations(byChild);
    } catch {
      if (alive.current) setViolations(null);
    }
    try {
      const page = await listEventsPage({ limit: RECENT_EVENTS });
      if (alive.current) setRecent(page.events.slice(0, RECENT_EVENTS));
    } catch {
      if (alive.current) setRecent(null);
    }
  }, []);

  const load = useCallback(async () => {
    try {
      const today = await getToday();
      if (!alive.current) return;
      setCards(today.children);
      setNotices(today.notices);
      // Старая служба без pendingTotal: число из карточек (решение по программе общее, R12).
      const total = today.pendingTotal || Math.max(0, ...today.children.map((c) => c.pendingPrograms ?? 0));
      setPending({ names: today.pendingNames, total });
      setLoadError("");
      await loadJournal();
    } catch (e) {
      if (alive.current) setLoadError(errorText(e));
    }
  }, [loadJournal]);

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
    setCards((prev) => (prev ?? []).map((c) => (c.childId === card.childId ? { ...c, ...card } : c)));

  const say = (childId: number, text: string, error = false, undo?: Feedback["undo"], quiet = false) => {
    seq.current += 1;
    setFeedback((prev) => ({ ...prev, [childId]: { seq: seq.current, text, error, undo, quiet } }));
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
    say(childId, reply.message, false, left > 0 && reply.extraId ? { kind: "extra", id: reply.extraId, ms: Math.min(left, 10_000) } : undefined);
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

  const undo = (card: TodayCard, u: NonNullable<Feedback["undo"]>) =>
    run(card.childId, async () => {
      if (u.kind === "bonus") {
        const reply = await undoBonus(card.childId, u.id);
        say(card.childId, reply.message);
        await load();
        return;
      }
      const reply = await undoExtra(card.childId, u.id);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  const pauseChild = (card: TodayCard, preset: PausePreset) =>
    run(card.childId, async () => {
      const reply = await pause(card.childId, preset);
      replaceCard(reply.card);
      // Видно одно место показа паузы — «Пауза до 18:30» у значка (визуальная приёмка T5); диктор читает ответ.
      say(card.childId, reply.message, false, undefined, true);
    });

  // Закончить паузу раньше срока (D-96): правила действуют сразу, карточка - из ответа службы.
  const endChildPause = (card: TodayCard) =>
    run(card.childId, async () => {
      const reply = await endPause(card.childId);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  // «Начислить в копилку» (FR-22): минуты и причина; итог с «Отменить» на 10 с. Ошибка минут - у поля.
  const grant = async (card: TodayCard, minutes: number, reason: string): Promise<boolean> => {
    setBusy(card.childId);
    setGrantError((prev) => ({ ...prev, [card.childId]: "" }));
    try {
      const reply = await grantBonus(card.childId, minutes, reason);
      const left = reply.undoUntilMs - Date.now();
      say(card.childId, reply.message, false, left > 0 && reply.bonusId ? { kind: "bonus", id: reply.bonusId, ms: Math.min(left, 10_000) } : undefined);
      await load();
      return true;
    } catch (e) {
      if (!alive.current) return false;
      if (e instanceof CallError && e.field === "minutes") setGrantError((prev) => ({ ...prev, [card.childId]: errorText(e) }));
      else say(card.childId, errorText(e), true);
      return false;
    } finally {
      if (alive.current) setBusy(null);
    }
  };

  const noForbidden = (card: TodayCard, on: boolean) =>
    run(card.childId, async () => {
      const reply = await setNoForbidden(card.childId, on);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  const unblock = (card: TodayCard) =>
    run(card.childId, async () => {
      const reply = await clearBlock(card.childId);
      replaceCard(reply.card);
      say(card.childId, reply.message);
    });

  return (
    <section aria-labelledby="today-title" className="today">
      <div className="page-head today-head">
        <h2 id="today-title" className="panel-title today-title">
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
        <p className="panel-text">Загружаю данные…</p>
      ) : (
        <>
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
            <div className="cards today-cards">
              {cards.map((card, i) => (
                <ChildCard
                  key={card.childId}
                  card={card}
                  avatarBg={AVATAR_BG[i % AVATAR_BG.length]}
                  feedback={feedback[card.childId]}
                  grantError={grantError[card.childId] ?? ""}
                  violations={violations ? (violations.get(card.childId) ?? 0) : null}
                  busy={busy === card.childId}
                  onAdd={(m) => void add(card, m)}
                  onReduce={(m) => void reduce(card, m)}
                  onClear={() => void clear(card)}
                  onUndo={(u) => void undo(card, u)}
                  onPause={(p) => void pauseChild(card, p)}
                  onEndPause={() => void endChildPause(card)}
                  onGrant={(m, r) => grant(card, m, r)}
                  onNoForbidden={(on) => void noForbidden(card, on)}
                  onUnblock={() => void unblock(card)}
                  onStats={onOpenStats ? () => onOpenStats(card.childId) : undefined}
                  onViolations={() => onOpenJournal?.({ violations: true, childId: card.childId })}
                />
              ))}
            </div>
          )}

          <div className="today-below">
            {pending.total > 0 ? (
              <section className="today-pending" aria-labelledby="today-pending-title">
                <h3 id="today-pending-title" className="today-block-title">
                  {programsWaitText(pending.total)}
                </h3>
                <p className="today-pending-text">{pendingLead(pending.names, pending.total)}</p>
                <div>
                  <button type="button" className="btn btn-primary" onClick={() => onOpenPrograms?.()}>
                    Разобрать
                  </button>
                </div>
              </section>
            ) : null}
            {recent !== null ? (
              <section className="today-recent" aria-labelledby="today-recent-title">
                <div className="today-recent-head">
                  <h3 id="today-recent-title" className="today-block-title">
                    Последние события
                  </h3>
                  <button type="button" className="link-btn" onClick={() => onOpenJournal?.({ violations: false })}>
                    Весь журнал
                  </button>
                </div>
                {recent.length === 0 ? (
                  <p className="today-recent-empty">Пока ничего не случилось</p>
                ) : (
                  <ul className="today-recent-list">
                    {recent.map((e) => (
                      <li key={e.id} className="today-recent-row">
                        <span className="today-recent-time num">{eventTime(e)}</span>
                        <EventText entry={e} />
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            ) : null}
          </div>
        </>
      )}
    </section>
  );
}

/** Время строки: сегодняшнее — «18:42», более раннее — «19 окт, 21:14». */
function eventTime(e: JournalEntry): string {
  return sameLocalDay(e.atMs, Date.now(), e.tzOffsetMin) ? formatTimeOfDay(e.atMs, e.tzOffsetMin) : formatEventTime(e.atMs, e.tzOffsetMin);
}

/** «**Миша** +15 минут с телефона»: имя ребёнка жирным, текст журнала без повтора имени. */
function EventText({ entry }: { entry: JournalEntry }) {
  const name = entry.childName.trim();
  const prefix = `${name}: `;
  if (name && entry.text.startsWith(prefix)) {
    return (
      <span className="today-recent-text">
        <b>{name}</b> {entry.text.slice(prefix.length)}
      </span>
    );
  }
  return <span className="today-recent-text">{entry.text}</span>;
}

interface CardProps {
  card: TodayCard;
  avatarBg: string;
  feedback: Feedback | undefined;
  grantError: string;
  /** Нарушений сегодня у ребёнка; null — журнал не прочитался. */
  violations: number | null;
  busy: boolean;
  onAdd: (minutes: number) => void;
  onReduce: (minutes: number) => void;
  onClear: () => void;
  onUndo: (u: NonNullable<Feedback["undo"]>) => void;
  onPause: (preset: PausePreset) => void;
  onEndPause: () => void;
  onGrant: (minutes: number, reason: string) => Promise<boolean>;
  onNoForbidden: (on: boolean) => void;
  onUnblock: () => void;
  onStats?: () => void;
  onViolations: () => void;
}

/** Строка запретных часов (8.3.2 п. 3): «Запрет с 21:00 до 07:00», снят — «Запрет на сегодня снят», нет — "". */
function forbidLine(s: ChildStatus): string {
  switch (s.forbidState) {
    case "ahead":
    case "active":
      if (s.forbidFrom && s.forbidTo) return `Запрет с ${s.forbidFrom} до ${s.forbidTo}`;
      return s.forbidTo ? `Запрет до ${s.forbidTo}` : "Запретные часы сегодня";
    case "cleared":
      return "Запрет на сегодня снят";
    default:
      return "";
  }
}

/** Статус словами в середине карточки: число и цвет уже говорят «можно» и «осталось мало». */
const STATUS_BY_NUMBER = new Set(["ok", "low", "unlimited"]);

function ChildCard({
  card,
  avatarBg,
  feedback,
  grantError,
  violations,
  busy,
  onAdd,
  onReduce,
  onClear,
  onUndo,
  onPause,
  onEndPause,
  onGrant,
  onNoForbidden,
  onUnblock,
  onStats,
  onViolations,
}: CardProps) {
  const s = card.status;
  const limited = hasTimeLimits(s.status);
  const extraAllowed = canAddTime(s.status);
  const titleId = `child-${card.childId}-name`;
  const [customOpen, setCustomOpen] = useState(false);
  const [customMin, setCustomMin] = useState("");
  const [grantOpen, setGrantOpen] = useState(false);
  const customN = Number.parseInt(customMin, 10);
  const customValid = Number.isInteger(customN) && customN >= 1 && customN <= 240;

  const tone = remainTone(s.remainingS, s.unlimited);
  const presence = card.presenceText ?? "";
  const statusWord = keepUnits(s.statusText);
  // Статус словами (FR-15) там, где число его не передаёт: «Время вышло», «Пауза», «Запретные часы»…
  const showStatusChip = presence !== "" && !STATUS_BY_NUMBER.has(s.status);
  const forbid = forbidLine(s);
  const canLift = !card.noForbiddenToday && (s.forbidState === "ahead" || s.forbidState === "active");
  // Потрачено — из карточки службы (то же число, что отчёт телефону), не «лимит − остаток».
  const games = limitLine("Игры", card.gamesUsedS, s.gamesLimitS, s.gamesUnlimited);
  const video = limitLine("Видео и соцсети", card.videoUsedS, s.videoLimitS ?? 0, s.videoUnlimited ?? true);
  const share = remainShare(s.remainingS, s.limitS, s.unlimited);
  const paused = s.status === "paused";

  return (
    <article className="child-card" aria-labelledby={titleId}>
      <div className="cc-who">
        <div className="cc-id">
          <span className="cc-avatar" style={{ background: avatarBg }} aria-hidden="true">
            {card.name.trim().slice(0, 1).toUpperCase()}
          </span>
          <h3 id={titleId} className="child-name">
            {card.name}
          </h3>
        </div>
        {/* Своей строкой во всю колонку: «Не за компьютером с 17:05» рядом с аватаром не помещалась.
            Одна точка: присутствия (зелёная или серая) или, если оно неизвестно, цвета статуса (.status). */}
        <p className="cc-presence">
          {presence !== "" ? (
            <>
              <span className={card.presenceNow ? "cc-dot cc-dot-on" : "cc-dot"} aria-hidden="true" />
              {keepUnits(presence)}
            </>
          ) : (
            <span className={`status status-${statusTone(s.status)}`} data-status={s.status}>
              {statusWord}
            </span>
          )}
        </p>
        <p className="cc-coin num">
          <Coin size={18} />
          {`В копилке ${card.bonusBalance} мин`}
        </p>
        {forbid ? (
          <p className="cc-forbid">
            <Icon name="moon" size={16} />
            {forbid}
          </p>
        ) : null}
      </div>

      <div className="cc-time">
        {limited ? (
          <>
            <div className="cc-remain">
              <span className={`remain-num remain-${tone} num`}>{s.unlimited ? "Без лимита" : formatHM(remainMinutes(s.remainingS))}</span>
              {s.unlimited ? null : <span className="cc-of num">{`осталось из ${formatHM(Math.floor(s.limitS / 60))}`}</span>}
              {showStatusChip ? (
                <span className={`status status-${statusTone(s.status)}`} data-status={s.status}>
                  {statusWord}
                </span>
              ) : null}
            </div>
            <div className="remain-bar" aria-hidden="true">
              <div className={`remain-bar-fill remain-bar-${tone}`} style={{ width: `${Math.round(share * 1000) / 10}%` }} />
            </div>
            {games || video ? (
              <p className="cc-limits num">
                {games ? <span>{games}</span> : null}
                {video ? <span>{video}</span> : null}
              </p>
            ) : null}
          </>
        ) : presence !== "" ? (
          // Учёта нет (администратор, правила не прочитаны): статус словами вместо числа (FR-15).
          <p className="cc-nolimits">
            <span className={`status status-${statusTone(s.status)}`} data-status={s.status}>
              {statusWord}
            </span>
          </p>
        ) : null}
        {card.extraMin > 0 ? <p className="child-meta">{`Добавлено сегодня: ${card.extraMin} мин`}</p> : null}
        {card.extraMin < 0 ? <p className="child-meta">{`Убрано сегодня: ${-card.extraMin} мин`}</p> : null}
        {s.lock !== "none" && s.until ? <p className="child-meta">{s.until}</p> : null}
      </div>

      <div className="cc-actions">
        <div className="cc-grid">
          {extraAllowed ? (
            <>
              {EXTRA_BUTTONS.map((m) => (
                <button key={`add-${m}`} type="button" className="btn chip chip-plus num" disabled={busy} onClick={() => onAdd(m)}>
                  {`+${m}`}
                </button>
              ))}
              <button
                type="button"
                className="btn chip"
                aria-label="Своё время…"
                aria-expanded={customOpen}
                disabled={busy}
                onClick={() => setCustomOpen((o) => !o)}
              >
                Своё
              </button>
              {EXTRA_BUTTONS.map((m) => (
                <button key={`sub-${m}`} type="button" className="btn chip chip-minus num" disabled={busy} onClick={() => onReduce(m)}>
                  {`−${m}`}
                </button>
              ))}
            </>
          ) : null}
          <PauseToggle paused={paused} until={s.pauseUntil} busy={busy} compact onPick={onPause} onEnd={onEndPause} />
        </div>
        {customOpen && extraAllowed ? (
          <div className="child-custom">
            <label className="child-custom-label">
              Минут:
              <input
                type="number"
                min={1}
                max={240}
                inputMode="numeric"
                autoComplete="off"
                spellCheck={false}
                className="child-custom-input"
                value={customMin}
                disabled={busy}
                onChange={(e) => setCustomMin(e.target.value)}
              />
            </label>
            <button type="button" className="btn chip chip-plus" disabled={busy || !customValid} onClick={() => onAdd(customN)}>
              Добавить
            </button>
            <button type="button" className="btn chip chip-minus" disabled={busy || !customValid} onClick={() => onReduce(customN)}>
              Убавить
            </button>
            <button type="button" className="btn chip" disabled={busy} onClick={onClear}>
              Убрать всё
            </button>
          </div>
        ) : null}
        <button type="button" className="btn chip chip-gold" aria-expanded={grantOpen} disabled={busy} onClick={() => setGrantOpen((o) => !o)}>
          <Coin size={18} />
          Начислить в копилку
        </button>
        {grantOpen ? (
          <div className="child-custom cc-grant">
            <GrantBonusForm
              disabled={busy}
              minutesError={grantError}
              onGrant={(m, r) =>
                void onGrant(m, r).then((ok) => {
                  if (ok) setGrantOpen(false);
                })
              }
            />
          </div>
        ) : null}
        <div className="cc-links">
          {onStats ? (
            <button type="button" className="link-btn" disabled={busy} onClick={onStats}>
              Статистика
            </button>
          ) : null}
          {card.noForbiddenToday ? (
            <button type="button" className="link-btn" disabled={busy} onClick={() => onNoForbidden(false)}>
              Вернуть запретные часы
            </button>
          ) : canLift ? (
            <button type="button" className="link-btn" disabled={busy} onClick={() => onNoForbidden(true)}>
              Снять запрет на сегодня
            </button>
          ) : null}
          {card.canClearBlock ? (
            <button type="button" className="link-btn" disabled={busy} onClick={onUnblock}>
              Снять блокировку
            </button>
          ) : null}
          {violations !== null ? (
            <button type="button" className={violations > 0 ? "link-btn link-danger" : "link-btn"} onClick={onViolations}>
              {`Нарушения сегодня: ${violations}`}
            </button>
          ) : null}
        </div>
      </div>

      {/* Живая область есть всегда (PL-44, B-38): диктор читает сообщение, как только оно появилось. */}
      {/* Ошибка (role="alert") - вне области «статус»: вложенные живые области диктор читает непредсказуемо (B-65);
          role="alert" при появлении читается сразу - для того он и сделан. */}
      <div role="status" className="cc-feedback">
        {feedback && !feedback.error ? (
          <UndoLine
            key={feedback.seq}
            live={false}
            className={feedback.quiet ? "child-feedback visually-hidden" : "child-feedback"}
            text={feedback.text}
            undoMs={feedback.undo?.ms}
            onUndo={feedback.undo ? () => onUndo(feedback.undo!) : undefined}
          />
        ) : null}
      </div>
      {feedback?.error ? (
        <div className="cc-feedback">
          <UndoLine key={feedback.seq} className="child-feedback" text={feedback.text} error />
        </div>
      ) : null}
    </article>
  );
}
