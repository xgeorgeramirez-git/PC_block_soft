import { useCallback, useEffect, useRef, useState } from "react";
import { createPromo, listPromos } from "../api";
import { clampDigits } from "../lib/numinput";
import { CallError, errorText } from "../lib/errors";
import { formatEventTime, localOffsetMin } from "../lib/format";
import HintSlot from "../guidance/HintSlot";
import type { PromoView } from "../types";
import Notice from "./Notice";

/**
 * Промокоды у родителя (FR-35, R26, D-50): создать код (буквы и цифры, минуты 1–600) и два
 * списка — «Действующие» и «Использованные» (когда и каким ребёнком).
 */
export default function PromoAdmin() {
  const [active, setActive] = useState<PromoView[] | null>(null);
  const [used, setUsed] = useState<PromoView[]>([]);
  const [loadError, setLoadError] = useState("");
  const [code, setCode] = useState("");
  const [minutes, setMinutes] = useState(30);
  const [codeError, setCodeError] = useState("");
  const [minutesError, setMinutesError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const alive = useRef(true);

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

  const submit = async () => {
    setMessage("");
    setCodeError("");
    setMinutesError("");
    if (!code.trim()) {
      setCodeError("Введите код");
      return;
    }
    if (minutes < 1) {
      setMinutesError("Можно от 1 до 600 минут");
      return;
    }
    setBusy(true);
    try {
      const r = await createPromo(code.trim(), minutes);
      if (!alive.current) return;
      setMessage(r.message);
      setCode("");
      await load();
    } catch (e) {
      if (!alive.current) return;
      const field = e instanceof CallError ? e.field : undefined;
      if (field === "minutes") setMinutesError(errorText(e));
      else setCodeError(errorText(e));
    } finally {
      if (alive.current) setBusy(false);
    }
  };

  const when = (ms: number) => formatEventTime(ms, localOffsetMin(new Date(ms)));

  return (
    <section className="panel promo-admin" aria-label="Промокоды" data-help-anchor="promo-create">
      <h3 className="section-title">Промокоды</h3>
      <p className="panel-text">
        Придумайте код из букв и цифр и назначьте минуты. Ребёнок введёт код на экране блокировки или в разделе
        «Бонусы», и время сразу добавится к сегодняшнему дню. Каждый код действует один раз.
      </p>
      <HintSlot anchor="promo-create" />
      <div className="promo-form">
        <label className="field">
          <span className="field-label">Код</span>
          <input
            className={codeError ? "field-input field-input-error" : "field-input"}
            value={code}
            maxLength={32}
            autoComplete="off"
            spellCheck={false}
            placeholder="a1234"
            aria-invalid={codeError ? "true" : undefined}
            disabled={busy}
            onChange={(e) => setCode(e.target.value.replace(/[^\p{L}\p{N} ]/gu, ""))}
          />
        </label>
        <label className="field promo-minutes">
          <span className="field-label">Минут</span>
          <input
            className={minutesError ? "field-input field-input-error" : "field-input"}
            type="text"
            inputMode="numeric"
            pattern="[0-9]*"
            maxLength={3}
            value={minutes}
            aria-invalid={minutesError ? "true" : undefined}
            disabled={busy}
            onChange={(e) => setMinutes(clampDigits(e.target.value, 600))}
          />
        </label>
        <button type="button" className="btn btn-primary" disabled={busy} onClick={() => void submit()}>
          Создать промокод
        </button>
      </div>
      {codeError || minutesError ? (
        <p className="field-error" role="alert">
          {codeError || minutesError}
        </p>
      ) : null}
      {message ? (
        <p className="child-feedback" role="status">
          {message}
        </p>
      ) : null}

      {loadError ? (
        <Notice tone="warn">{loadError}</Notice>
      ) : active === null ? (
        <p className="panel-text">Загружаю промокоды</p>
      ) : (
        <div className="promo-lists">
          <div>
            <h4 className="promo-list-title">Действующие</h4>
            {active.length === 0 ? (
              <p className="panel-text">Действующих кодов нет</p>
            ) : (
              <ul className="promo-list" aria-label="Действующие промокоды">
                {active.map((p) => (
                  <li key={p.codeDisplay} className="promo-item">
                    <code className="promo-code">{p.codeDisplay}</code>
                    <span>{`${p.minutes} мин`}</span>
                    <span className="promo-when">{`создан ${when(p.createdAtMs)}`}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div>
            <h4 className="promo-list-title">Использованные</h4>
            {used.length === 0 ? (
              <p className="panel-text">Использованных кодов пока нет</p>
            ) : (
              <ul className="promo-list" aria-label="Использованные промокоды">
                {used.map((p) => (
                  <li key={p.codeDisplay} className="promo-item promo-item-used">
                    <code className="promo-code">{p.codeDisplay}</code>
                    <span>{`${p.minutes} мин`}</span>
                    <span className="promo-when">
                      {`использован ${p.usedAtMs ? when(p.usedAtMs) : ""}${p.usedByName ? `, ${p.usedByName}` : ""}`}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
