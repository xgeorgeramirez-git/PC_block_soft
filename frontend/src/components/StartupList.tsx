import { useCallback, useEffect, useRef, useState, type MouseEvent } from "react";
import { listStartup, setStartup } from "../api";
import Notice from "./Notice";
import { errorText } from "../lib/errors";
import { genitiveName, localOffsetMin } from "../lib/format";
import { applyStartupItem, checkedText, isStartupEmpty, startupShortName } from "../lib/startup";
import type { ListStartupReply, StartupItemView, StartupSource } from "../types";

interface Props {
  childId: number;
  /** Имя ребёнка как в правилах: «Миша». */
  childName: string;
}

/** Пока служба проверяет Windows (checking), список перечитывается с этим шагом. */
const POLL_MS = 2000;

/**
 * Вкладка «Автозагрузка» (FR-39, ТЗ 8.3.11): «Ждут решения», «Учётка Миши», «Для всех
 * пользователей». Выключатель действует сразу, без панели «Сохранить»: решение уходит службе,
 * строка показывает её ответ. Данные хранятся вместе с номером ребёнка (PL-59): пока ответ не про
 * выбранного ребёнка - «Загружаю…», строк прежнего нет.
 */
export default function StartupList({ childId, childName }: Props) {
  const [shown, setShown] = useState<{ childId: number; data: ListStartupReply } | null>(null);
  const [loadError, setLoadError] = useState<{ childId: number; text: string } | null>(null);
  /** Строки, по которым идёт запрос: положение, которое нажал родитель. */
  const [pressing, setPressing] = useState<Record<number, boolean>>({});
  /** Итог последнего нажатия; id - строка, у которой он показан (B-73: у выключателя, а не внизу). */
  const [feedback, setFeedback] = useState<{ text: string; error: boolean; id: number } | null>(null);
  /** Строка переехала в другой блок с фокусом на выключателе: фокус переносится за ней. */
  const [focusId, setFocusId] = useState<number | null>(null);
  const alive = useRef(true);
  const current = useRef(childId);
  /** Номер последнего чтения: ответ более раннего не показывается. */
  const loadSeq = useRef(0);
  /** Число принятых решений: чтение, начатое до решения, его не затирает. */
  const decided = useRef(0);
  const poll = useRef<ReturnType<typeof setTimeout> | null>(null);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (poll.current) clearTimeout(poll.current);
    };
  }, []);

  const load = useCallback(async (id: number, refresh: boolean, quiet: boolean) => {
    if (poll.current) clearTimeout(poll.current);
    poll.current = null;
    const seq = ++loadSeq.current;
    const ver = decided.current;
    if (!quiet) setLoadError(null);
    try {
      const data = await listStartup(id, refresh);
      if (!alive.current || current.current !== id || seq !== loadSeq.current || ver !== decided.current) return;
      setShown({ childId: id, data });
      setLoadError(null);
      if (data.checking) poll.current = setTimeout(() => void load(id, false, true), POLL_MS);
    } catch (e) {
      if (!alive.current || current.current !== id || seq !== loadSeq.current) return;
      // Тихое перечитывание после решения или во время проверки: сбой не прячет показанный список.
      if (!quiet) setLoadError({ childId: id, text: errorText(e) });
    }
  }, []);

  // Открыли вкладку или сменили ребёнка: служба проверяет Windows заново (RM5b-5).
  useEffect(() => {
    current.current = childId;
    setFeedback(null);
    setPressing({});
    void load(childId, true, false);
  }, [childId, load]);

  useEffect(() => {
    if (focusId === null) return;
    root.current?.querySelector<HTMLButtonElement>(`button[data-startup-id="${focusId}"]`)?.focus();
    setFocusId(null);
  }, [focusId, shown]);

  /**
   * Решение по строке: выключатель шлёт противоположное показанному, «Оставить выключенной» у
   * ждущей записи - on = false (8.3.11 п.2). Фокус был в строке - после переезда он на её выключателе.
   */
  const decide = async (item: StartupItemView, on: boolean, e: MouseEvent<HTMLButtonElement>) => {
    if (pressing[item.id] !== undefined) return;
    const hadFocus = document.activeElement === e.currentTarget;
    const id = childId;
    setPressing((p) => ({ ...p, [item.id]: on }));
    setFeedback(null);
    try {
      const r = await setStartup(id, item.id, on);
      if (!alive.current || current.current !== id) return;
      decided.current += 1;
      setShown((s) => (s && s.childId === id ? { childId: id, data: applyStartupItem(s.data, r.item) } : s));
      setFeedback({ text: r.message, error: false, id: item.id });
      if (hadFocus) setFocusId(item.id);
      // Список после действия - как у службы (запись могла пропасть или появиться новая).
      void load(id, false, true);
    } catch (err) {
      if (!alive.current || current.current !== id) return;
      setFeedback({ text: errorText(err), error: true, id: item.id });
    } finally {
      if (alive.current && current.current === id) {
        setPressing((p) => {
          const next = { ...p };
          delete next[item.id];
          return next;
        });
      }
    }
  };

  const gen = genitiveName(childName);
  const data = shown && shown.childId === childId ? shown.data : null;
  const error = loadError && loadError.childId === childId ? loadError.text : "";

  if (error && !data) {
    return (
      <div className="startup">
        <Notice tone="warn" urgent>
          {error}
        </Notice>
        <button type="button" className="btn" onClick={() => void load(childId, true, false)}>
          Повторить
        </button>
      </div>
    );
  }
  if (!data) {
    return (
      <p className="panel-text" role="status">
        Загружаю…
      </p>
    );
  }

  const checked = checkedText(data.checkedAt, data.checking, Date.now(), localOffsetMin());
  const block = (key: string, title: string, lead: string, items: StartupItemView[]) =>
    items.length === 0 ? null : (
      <section className="startup-block" aria-labelledby={`startup-${key}-title`}>
        <h3 id={`startup-${key}-title`} className="section-title">
          {title}
        </h3>
        {lead ? <p className="startup-lead">{lead}</p> : null}
        <ul className="startup-list">
          {items.map((item) => (
            <StartupRow
              key={item.id}
              item={item}
              pressed={pressing[item.id]}
              note={feedback && feedback.id === item.id ? feedback : null}
              onDecide={(on, ev) => void decide(item, on, ev)}
            />
          ))}
        </ul>
      </section>
    );

  return (
    <div className="startup" ref={root}>
      <div className="startup-head">
        <p className="startup-intro">{`Что запускается само, когда ${childName} входит в свою учётную запись. Выключатель действует сразу, сохранять не нужно.`}</p>
        {checked ? <p className="startup-checked">{checked}</p> : null}
      </div>
      {isStartupEmpty(data) ? (
        <p className="startup-empty">{`При входе ${gen} ничего не запускается`}</p>
      ) : (
        <>
          {block("pending", "Ждут решения", "Новые записи выключены, пока вы не решите", data.pending)}
          {block("own", `Учётка ${gen}`, "", data.own)}
          {block("shared", "Для всех пользователей", `Выключатель действует только при входе ${gen}`, data.shared)}
        </>
      )}
      {/* Живая область есть всегда (PL-44) и читается диктором; глазами итог виден у строки (B-73). */}
      <div className="startup-message visually-hidden" role="status">
        {feedback ? <span>{feedback.text}</span> : null}
      </div>
    </div>
  );
}

interface RowProps {
  item: StartupItemView;
  /** Идёт запрос: положение, которое нажал родитель; undefined - запроса нет. */
  pressed?: boolean;
  /** Итог последнего нажатия на этой строке: показывается под ней (B-73). */
  note: { text: string; error: boolean } | null;
  onDecide: (on: boolean, e: MouseEvent<HTMLButtonElement>) => void;
}

/** Издатель записи - Microsoft: часть Windows или программа Microsoft (B-73, пояснение перед выключением). */
function isMicrosoft(publisher: string): boolean {
  return publisher.trim().toLowerCase().startsWith("microsoft");
}

/**
 * Строка: значок вида записи, название, издатель, «откуда», выключатель «Включено / Выключено»; у
 * ждущей записи справа от выключателя ещё «Оставить выключенной» (8.3.11 п.2).
 */
function StartupRow({ item, pressed, note, onDecide }: RowProps) {
  const name = startupShortName(item);
  const busy = pressed !== undefined;
  const on = busy ? pressed : item.decision === "on";
  const pending = item.decision === "pending";
  const metaId = `startup-${item.id}-meta`;
  return (
    <li className={pending ? "startup-row startup-row-pending" : "startup-row"}>
      <SourceIcon source={item.source} />
      <div className="startup-main">
        <span className="startup-name" title={item.name}>
          {name}
        </span>
        <span id={metaId} className="startup-meta">
          {item.publisher ? (
            <span className="startup-publisher" title={item.publisher}>
              {item.publisher}
            </span>
          ) : null}
          <span className="startup-from">{item.from}</span>
        </span>
        {!pending && isMicrosoft(item.publisher) ? (
          <span className="startup-hint">Программа Windows или Microsoft. Выключайте, только если знаете, что она делает</span>
        ) : null}
      </div>
      <button
        type="button"
        role="switch"
        className="startup-switch"
        aria-checked={on}
        aria-label={name}
        aria-describedby={metaId}
        aria-disabled={busy ? true : undefined}
        aria-busy={busy ? true : undefined}
        data-startup-id={item.id}
        onClick={(e) => {
          if (!busy) onDecide(item.decision !== "on", e);
        }}
      >
        <span className="startup-switch-track" aria-hidden="true">
          <span className="startup-switch-thumb" />
        </span>
        <span className="startup-switch-text" aria-hidden="true">
          {on ? "Включено" : "Выключено"}
        </span>
      </button>
      {pending ? (
        <button
          type="button"
          className="btn btn-small startup-keep"
          aria-label={`Оставить выключенной: ${name}`}
          aria-disabled={busy ? true : undefined}
          onClick={(e) => {
            if (!busy) onDecide(false, e);
          }}
        >
          Оставить выключенной
        </button>
      ) : null}
      {note ? (
        <p className={note.error ? "startup-row-note field-error" : "startup-row-note"} aria-hidden="true">
          {note.text}
        </p>
      ) : null}
    </li>
  );
}

/** Значок вида записи (RM5b-6): реестр, папка, задание Планировщика, приложение из магазина. */
function SourceIcon({ source }: { source: StartupSource }) {
  const kind = source === "startup_folder" ? "folder" : source === "task" ? "task" : source === "store_app" ? "store" : "registry";
  return (
    <svg className={`startup-icon startup-icon-${kind}`} viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false">
      {kind === "registry" ? (
        <>
          <rect x="3" y="3" width="6" height="6" rx="1" />
          <rect x="11" y="3" width="6" height="6" rx="1" />
          <rect x="3" y="11" width="6" height="6" rx="1" />
          <rect x="11" y="11" width="6" height="6" rx="1" />
        </>
      ) : kind === "folder" ? (
        <path d="M2.5 5.5a1 1 0 0 1 1-1h4l1.5 2h7.5a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1h-13a1 1 0 0 1-1-1z" />
      ) : kind === "task" ? (
        <>
          <circle cx="10" cy="10" r="7" />
          <path d="M10 6v4l3 2" />
        </>
      ) : (
        <>
          <path d="M4 7h12l-1 10H5z" />
          <path d="M7.5 7V5.5a2.5 2.5 0 0 1 5 0V7" />
        </>
      )}
    </svg>
  );
}
