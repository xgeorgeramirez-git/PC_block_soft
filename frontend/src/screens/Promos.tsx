import { useCallback, useEffect, useId, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { clearPromoHistory, createPromo, generatePromos, listPromos } from "../api";
import Dialog from "../components/Dialog";
import Notice from "../components/Notice";
import { CallError, errorText } from "../lib/errors";
import { formatDayAt, localOffsetMin } from "../lib/format";
import { clampDigits } from "../lib/numinput";
import type { PromoCharset, PromoView } from "../types";

type Tab = "active" | "used";

const TABS: Tab[] = ["active", "used"];

const CHARSETS: { id: PromoCharset; label: string }[] = [
  { id: "digits", label: "Только цифры" },
  { id: "letters", label: "Только буквы" },
  { id: "mixed", label: "Буквы и цифры" },
];

/** Ошибки полей «Придумать коды» по имени поля службы (RM5-12). */
type GenErrors = { count?: string; length?: string; minutes?: string };
type GenField = keyof GenErrors;

const when = (ms: number) => formatDayAt(ms, localOffsetMin(new Date(ms)));

/**
 * Раздел «Промокоды» (FR-35, R26, сценарий 8.3.10): вкладки «Действующие» и «Использованные»,
 * «Создать код», «Придумать коды», «Очистить историю» с подтверждением. Окно не придумывает ни кодов,
 * ни текстов ошибок: коды, итоги и ошибки полей приходят от службы; код проверяет служба (RM5-11).
 */
export default function Promos() {
  const [active, setActive] = useState<PromoView[] | null>(null);
  const [used, setUsed] = useState<PromoView[]>([]);
  const [loadError, setLoadError] = useState("");
  const [tab, setTab] = useState<Tab>("active");

  const [code, setCode] = useState("");
  const [minutes, setMinutes] = useState(30);
  const [codeError, setCodeError] = useState("");
  const [minutesError, setMinutesError] = useState("");

  const [count, setCount] = useState(10);
  const [length, setLength] = useState(6);
  const [charset, setCharset] = useState<PromoCharset>("mixed");
  const [genMinutes, setGenMinutes] = useState(30);
  const [genErrors, setGenErrors] = useState<GenErrors>({});
  const [genError, setGenError] = useState("");
  const [generated, setGenerated] = useState<PromoView[]>([]);
  const [copyError, setCopyError] = useState(false);

  const [message, setMessage] = useState("");
  const [actionError, setActionError] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);

  const alive = useRef(true);
  const codeRef = useRef<HTMLInputElement>(null);
  const minutesRef = useRef<HTMLInputElement>(null);
  const genRefs = useRef<Record<GenField, HTMLInputElement | null>>({ count: null, length: null, minutes: null });
  const tabRefs = useRef(new Map<Tab, HTMLButtonElement>());
  const ids = useId();
  const fid = (name: string) => `${ids}-${name}`;

  const load = useCallback(async () => {
    try {
      const r = await listPromos();
      if (!alive.current) return;
      setActive(r.active ?? []);
      setUsed(r.used ?? []);
      setLoadError("");
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

  const resetFeedback = () => {
    setMessage("");
    setActionError("");
    setCopyError(false);
  };

  const create = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    resetFeedback();
    setCodeError("");
    setMinutesError("");
    if (!code.trim()) {
      setCodeError("Введите код");
      codeRef.current?.focus();
      return;
    }
    if (minutes < 1) {
      setMinutesError("Можно от 1 до 600 минут");
      minutesRef.current?.focus();
      return;
    }
    setBusy(true);
    try {
      const r = await createPromo(code.trim(), minutes);
      if (!alive.current) return;
      setMessage(r.message);
      setCode("");
      setTab("active");
      await load();
    } catch (err) {
      if (!alive.current) return;
      // Ошибка у своего поля, фокус на нём (PL-36): родитель сразу видит, что поправить.
      if (err instanceof CallError && err.field === "minutes") {
        setMinutesError(errorText(err));
        minutesRef.current?.focus();
      } else {
        setCodeError(errorText(err));
        codeRef.current?.focus();
      }
    } finally {
      if (alive.current) setBusy(false);
    }
  };

  const generate = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    resetFeedback();
    setGenErrors({});
    setGenError("");
    setBusy(true);
    try {
      const r = await generatePromos({ count, length, charset, minutes: genMinutes });
      if (!alive.current) return;
      setGenerated(r.promos ?? []);
      setMessage(r.message);
      setTab("active");
      await load();
    } catch (err) {
      if (!alive.current) return;
      const field = err instanceof CallError ? err.field : undefined;
      if (field === "count" || field === "length" || field === "minutes") {
        setGenErrors({ [field]: errorText(err) });
        genRefs.current[field]?.focus();
      } else {
        setGenError(errorText(err));
      }
    } finally {
      if (alive.current) setBusy(false);
    }
  };

  const copyAll = async () => {
    setMessage("");
    setCopyError(false);
    const text = generated.map((p) => p.codeDisplay).join("\n");
    try {
      if (!navigator.clipboard) throw new Error("no clipboard");
      await navigator.clipboard.writeText(text);
      if (alive.current) setMessage("Коды скопированы");
    } catch {
      if (alive.current) setCopyError(true);
    }
  };

  const clearHistory = async () => {
    if (busy) return;
    setBusy(true);
    resetFeedback();
    try {
      const r = await clearPromoHistory();
      if (!alive.current) return;
      setConfirm(false);
      setMessage(r.message);
      await load();
      // Кнопка «Очистить историю» исчезла вместе со списком: фокус - на вкладку, а не в никуда.
      if (alive.current) tabRefs.current.get("used")?.focus();
    } catch (err) {
      // Ничего не удалено: списки прежние, причина простыми словами.
      if (!alive.current) return;
      setConfirm(false);
      setActionError(errorText(err));
    } finally {
      if (alive.current) setBusy(false);
    }
  };

  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const last = TABS.length - 1;
    let next: number;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = index === last ? 0 : index + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = index === 0 ? last : index - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    setTab(TABS[next]);
    tabRefs.current.get(TABS[next])?.focus();
  };

  const tabLabel = (t: Tab) => {
    const name = t === "active" ? "Действующие" : "Использованные";
    if (active === null) return name;
    return `${name} (${t === "active" ? active.length : used.length})`;
  };

  const numField = (
    field: GenField,
    label: string,
    value: number,
    max: number,
    onChange: (n: number) => void,
    className = "promo-num",
  ) => {
    const error = genErrors[field];
    return (
      <div className={`field ${className}`}>
        <label className="field-label" htmlFor={fid(`gen-${field}`)}>
          {label}
        </label>
        <input
          id={fid(`gen-${field}`)}
          ref={(el) => {
            genRefs.current[field] = el;
          }}
          className="field-input"
          type="text"
          inputMode="numeric"
          autoComplete="off"
          spellCheck={false}
          pattern="[0-9]*"
          maxLength={String(max).length}
          value={value}
          aria-invalid={error ? "true" : undefined}
          aria-describedby={error ? fid(`gen-${field}-error`) : undefined}
          onChange={(e) => onChange(clampDigits(e.target.value, max))}
        />
        {error ? (
          <p id={fid(`gen-${field}-error`)} className="field-error" role="alert">
            {error}
          </p>
        ) : null}
      </div>
    );
  };

  return (
    <section aria-labelledby="promos-title" className="promos-screen">
      <div className="page-head">
        <h2 id="promos-title" className="panel-title">
          Промокоды
        </h2>
      </div>
      <p className="panel-text">
        Ребёнок вводит код на экране блокировки или в разделе «Бонусы», и минуты сразу добавляются к сегодняшнему
        дню. Каждый код действует один раз.
      </p>
      {/* Живая область есть всегда (PL-44): диктор читает сообщение, как только оно появилось. */}
      <p className="child-feedback promo-status" role="status">
        {message}
      </p>
      {actionError ? (
        <Notice tone="warn" urgent>
          {actionError}
        </Notice>
      ) : null}

      <section className="panel" aria-labelledby={fid("create-title")}>
        <h3 id={fid("create-title")} className="section-title">
          Создать код
        </h3>
        <form className="promo-form" noValidate onSubmit={(e) => void create(e)}>
          <div className="field promo-code-field">
            <label className="field-label" htmlFor={fid("code")}>
              Код
            </label>
            <input
              id={fid("code")}
              ref={codeRef}
              className="field-input"
              type="text"
              value={code}
              maxLength={16}
              autoComplete="off"
              spellCheck={false}
              autoCapitalize="characters"
              placeholder="A1234"
              aria-invalid={codeError ? "true" : undefined}
              aria-describedby={codeError ? fid("code-error") : undefined}
              onChange={(e) => setCode(e.target.value)}
            />
            {codeError ? (
              <p id={fid("code-error")} className="field-error" role="alert">
                {codeError}
              </p>
            ) : null}
          </div>
          <div className="field promo-num">
            <label className="field-label" htmlFor={fid("minutes")}>
              Минуты
            </label>
            <input
              id={fid("minutes")}
              ref={minutesRef}
              className="field-input"
              type="text"
              inputMode="numeric"
              autoComplete="off"
              spellCheck={false}
              pattern="[0-9]*"
              maxLength={3}
              value={minutes}
              aria-invalid={minutesError ? "true" : undefined}
              aria-describedby={minutesError ? fid("minutes-error") : undefined}
              onChange={(e) => setMinutes(clampDigits(e.target.value, 600))}
            />
            {minutesError ? (
              <p id={fid("minutes-error")} className="field-error" role="alert">
                {minutesError}
              </p>
            ) : null}
          </div>
          <button type="submit" className="btn btn-primary promo-submit" disabled={busy}>
            Создать
          </button>
        </form>
      </section>

      <section className="panel" aria-labelledby={fid("gen-title")}>
        <h3 id={fid("gen-title")} className="section-title">
          Придумать коды
        </h3>
        {genError ? (
          <Notice tone="warn" urgent>
            {genError}
          </Notice>
        ) : null}
        <form className="promo-form" noValidate onSubmit={(e) => void generate(e)}>
          {numField("count", "Сколько", count, 50, setCount)}
          {numField("length", "Длина", length, 12, setLength)}
          <fieldset className="promo-charset">
            <legend className="field-label">Из чего</legend>
            <div className="promo-charset-options">
              {CHARSETS.map((c) => (
                <label key={c.id} className="promo-charset-option">
                  <input
                    type="radio"
                    name={fid("charset")}
                    value={c.id}
                    checked={charset === c.id}
                    onChange={() => setCharset(c.id)}
                  />
                  <span>{c.label}</span>
                </label>
              ))}
            </div>
          </fieldset>
          {numField("minutes", "Минуты", genMinutes, 600, setGenMinutes)}
          <button type="submit" className="btn btn-primary promo-submit" disabled={busy}>
            Придумать
          </button>
        </form>
        {generated.length > 0 ? (
          <div className="promo-generated">
            <ul className="promo-codes" aria-label="Новые коды">
              {generated.map((p) => (
                <li key={p.codeDisplay}>
                  <code className="promo-code">{p.codeDisplay}</code>
                </li>
              ))}
            </ul>
            <button type="button" className="btn" onClick={() => void copyAll()}>
              Скопировать все
            </button>
            {copyError ? (
              <p className="field-error" role="alert">
                Не удалось скопировать, выделите коды вручную
              </p>
            ) : null}
          </div>
        ) : null}
      </section>

      <div className="tabs promo-tabs" role="tablist" aria-label="Промокоды">
        {TABS.map((t, i) => (
          <button
            key={t}
            ref={(el) => {
              if (el) tabRefs.current.set(t, el);
              else tabRefs.current.delete(t);
            }}
            type="button"
            role="tab"
            id={`promos-tab-${t}`}
            className="tab"
            aria-selected={tab === t}
            aria-controls="promos-panel"
            tabIndex={tab === t ? 0 : -1}
            onClick={() => setTab(t)}
            onKeyDown={(e) => onTabKey(e, i)}
          >
            {tabLabel(t)}
          </button>
        ))}
      </div>

      <div id="promos-panel" role="tabpanel" className="panel" aria-labelledby={`promos-tab-${tab}`}>
        {loadError ? (
          <>
            <Notice tone="warn" urgent>
              {loadError}
            </Notice>
            <button type="button" className="btn" onClick={() => void load()}>
              Повторить
            </button>
          </>
        ) : active === null ? (
          <p className="panel-text">Загружаю промокоды…</p>
        ) : tab === "active" ? (
          active.length === 0 ? (
            <p className="panel-text">Действующих кодов нет</p>
          ) : (
            <ul className="promo-list" aria-label="Действующие промокоды">
              {active.map((p) => (
                <li key={p.codeDisplay} className="promo-item">
                  <code className="promo-code">{p.codeDisplay}</code>
                  <span className="promo-minutes">{`${p.minutes} мин`}</span>
                  <span className="promo-when">{`создан ${when(p.createdAtMs)}`}</span>
                </li>
              ))}
            </ul>
          )
        ) : used.length === 0 ? (
          <p className="panel-text">Использованных кодов нет</p>
        ) : (
          <>
            <div className="promo-panel-head">
              <button type="button" className="btn" disabled={busy} onClick={() => setConfirm(true)}>
                Очистить историю
              </button>
            </div>
            <ul className="promo-list" aria-label="Использованные промокоды">
              {used.map((p) => (
                <li key={p.codeDisplay} className="promo-item promo-item-used">
                  <code className="promo-code">{p.codeDisplay}</code>
                  <span className="promo-minutes">{`${p.minutes} мин`}</span>
                  <span className="promo-when">
                    {`использован${p.usedAtMs ? ` ${when(p.usedAtMs)}` : ""}${p.usedByName ? `, ${p.usedByName}` : ""}`}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      {confirm ? (
        <Dialog
          title="Удалить все использованные коды?"
          onClose={() => {
            if (!busy) setConfirm(false);
          }}
          actions={
            <>
              <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void clearHistory()}>
                Удалить
              </button>
              <button type="button" className="btn" data-autofocus disabled={busy} onClick={() => setConfirm(false)}>
                Отмена
              </button>
            </>
          }
        >
          <p>Действующие останутся.</p>
        </Dialog>
      ) : null}
    </section>
  );
}
