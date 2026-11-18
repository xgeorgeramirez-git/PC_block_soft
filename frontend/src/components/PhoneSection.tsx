import { useCallback, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { getRemote, remotePairCancel, remotePairConfirm, remotePairStart, remoteUnpair, setRemote } from "../api";
import Dialog from "./Dialog";
import TextField from "./TextField";
import { CallError, errorText } from "../lib/errors";
import {
  POLL_IDLE_MS,
  REMOTE_DEFAULT_URL,
  canPair,
  countdownText,
  formatCode,
  formatSas,
  pairErrorText,
  pairedLine,
  pollMs,
  remoteErrorText,
  statusLine,
} from "../lib/remote";
import type { RemoteView } from "../types";

/** Куда вернуть фокус после запроса (PL-68): ключ data-focus элемента блока. */
type FocusKey = "toggle" | "url" | "pair" | "cancel" | "match" | "unpair";

const LOAD_ERROR_TEXT = "Не удалось узнать состояние связи. Пробуем ещё раз";

/**
 * Блок «Телефон» в «Настройках» (M7, FR-40, UX-сценарий плана M7): выключатель связи, адрес сервера,
 * строка состояния, привязка телефона кодом и сверкой числа, «Отвязать». Состояние - GetRemote раз в 1 с
 * во время привязки, иначе раз в 5 с, пока блок на экране.
 */
export default function PhoneSection() {
  const [view, setView] = useState<RemoteView | null>(null);
  const [url, setUrl] = useState("");
  const [urlError, setUrlError] = useState("");
  const [loadError, setLoadError] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirmUnpair, setConfirmUnpair] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  const box = useRef<HTMLElement>(null);
  const alive = useRef(true);
  const busyRef = useRef(false);
  const seq = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const wantFocus = useRef<FocusKey | null>(null);
  const urlTouched = useRef(false);
  const urlRef = useRef<HTMLInputElement>(null);
  const pairHintId = useId();

  /** Новое состояние: поле адреса - из службы, пока родитель его не правил. */
  const apply = useCallback((v: RemoteView) => {
    setView(v);
    setNow(Date.now());
    if (!urlTouched.current) setUrl(v.url);
  }, []);

  // Таймер опроса зовёт свежий poll через ссылку (poll объявлен ниже).
  const pollRef = useRef<() => Promise<void>>(async () => undefined);
  const schedule = useCallback((ms: number) => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => void pollRef.current(), ms);
  }, []);

  const poll = useCallback(async () => {
    if (!alive.current) return;
    if (busyRef.current) {
      schedule(POLL_IDLE_MS);
      return;
    }
    const my = ++seq.current;
    try {
      const v = await getRemote();
      if (!alive.current || my !== seq.current) return;
      apply(v);
      setLoadError("");
      schedule(pollMs(v));
    } catch {
      if (!alive.current || my !== seq.current) return;
      setLoadError(LOAD_ERROR_TEXT);
      schedule(POLL_IDLE_MS);
    }
  }, [apply, schedule]);

  pollRef.current = poll;
  useEffect(() => {
    alive.current = true;
    void pollRef.current();
    return () => {
      alive.current = false;
      clearTimeout(timer.current);
    };
  }, []);

  // Отсчёт кода: раз в секунду, пока код на экране.
  const stage = view?.pair.stage;
  useEffect(() => {
    if (stage !== "code") return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [stage]);

  // Этап сменился опросом (код → сверка, срок вышел): кнопка в фокусе исчезла - фокус на главную кнопку этапа.
  const shownStage = useRef(stage);
  useEffect(() => {
    if (shownStage.current === stage) return;
    shownStage.current = stage;
    if (wantFocus.current === null && document.activeElement === document.body) {
      wantFocus.current = stage === "compare" ? "match" : stage === "code" || stage === "starting" ? "cancel" : "pair";
    }
  }, [stage]);

  // Фокус - после перерисовки без disabled (PL-68), на тот элемент, что остался на экране.
  useEffect(() => {
    if (busy || confirmUnpair || !wantFocus.current) return;
    const el = wantFocus.current === "url" ? urlRef.current : box.current?.querySelector<HTMLElement>(`[data-focus="${wantFocus.current}"]`);
    if (el && !(el as HTMLButtonElement).disabled) {
      el.focus();
      wantFocus.current = null;
    }
  }, [busy, confirmUnpair, view, urlError, error]);

  /** Одно действие родителя: занятость, свежее состояние, сообщение или ошибка, фокус. */
  const perform = async (run: () => Promise<RemoteView>, ok: (v: RemoteView) => string, focus: (v: RemoteView | null) => FocusKey) => {
    busyRef.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    const my = ++seq.current;
    let next: RemoteView | null = null;
    try {
      next = await run();
      if (!alive.current) return;
      apply(next);
      setMessage(ok(next));
    } catch (e) {
      if (!alive.current) return;
      if (e instanceof CallError && e.field === "url") {
        setUrlError(errorText(e));
        wantFocus.current = "url";
      } else {
        setError(errorText(e));
      }
    } finally {
      busyRef.current = false;
      if (alive.current) {
        if (wantFocus.current !== "url") wantFocus.current = focus(next);
        setBusy(false);
        if (my === seq.current) schedule(next ? pollMs(next) : POLL_IDLE_MS);
      }
    }
  };

  // Выключатель не сохраняет неотправленную правку адреса: шлёт сохранённый.
  const toggle = (on: boolean) => {
    urlTouched.current = false;
    void perform(
      () => setRemote(on, view?.url || REMOTE_DEFAULT_URL),
      (v) => (v.enabled ? "Связь с телефоном включена" : "Связь с телефоном выключена"),
      () => "toggle",
    );
  };

  const saveUrl = (e: FormEvent) => {
    e.preventDefault();
    if (busy || !view) return;
    setUrlError("");
    void perform(
      async () => {
        const v = await setRemote(true, url.trim());
        urlTouched.current = false;
        return v;
      },
      () => "Адрес сохранён",
      () => "url",
    );
  };

  const pairStart = () => void perform(remotePairStart, () => "", (v) => (v?.pair.stage === "code" || v?.pair.stage === "starting" ? "cancel" : "pair"));
  const pairCancel = () => void perform(remotePairCancel, () => "Подключение отменено", () => "pair");
  const confirm = (match: boolean) =>
    void perform(
      () => remotePairConfirm(match),
      (v) => (v.phone && v.pair.stage === "idle" ? "Телефон подключён" : ""),
      (v) => (v?.phone && v.pair.stage === "idle" ? "unpair" : "pair"),
    );
  const unpair = () => {
    setConfirmUnpair(false);
    void perform(remoteUnpair, () => "Телефон отвязан", () => "pair");
  };

  const enabled = view?.enabled === true;
  const pair = view?.pair;
  const can = view ? canPair(view) : { ok: false, why: "" };
  const connError = view && enabled ? remoteErrorText(view.error) : "";

  return (
    <section ref={box} className="panel phone" aria-labelledby="settings-phone">
      <h3 id="settings-phone" className="section-title">
        Телефон
      </h3>
      <label className="switch">
        <input
          type="checkbox"
          data-focus="toggle"
          checked={enabled}
          disabled={view === null || busy}
          onChange={(e) => toggle(e.target.checked)}
        />
        <span>Связь с телефоном</span>
      </label>
      <p className="field-hint">Программа будет связываться только с вашим сервером. Выключено: программа совсем без интернета</p>

      {view === null && !loadError ? <p className="panel-text">Узнаю состояние связи…</p> : null}
      {loadError ? <p className="field-hint phone-load-error">{loadError}</p> : null}

      {view && enabled ? (
        <>
          <form className="phone-url" onSubmit={saveUrl} noValidate>
            <TextField
              ref={urlRef}
              label="Адрес сервера"
              type="url"
              spellCheck={false}
              hint="Адрес вашего сервера связи. Обычно менять не нужно"
              autoComplete="off"
              maxLength={200}
              value={url}
              error={urlError || null}
              disabled={busy}
              onChange={(v) => {
                urlTouched.current = true;
                setUrl(v);
                setUrlError("");
                setMessage("");
              }}
            />
            <div className="form-actions">
              <button type="submit" className="btn" disabled={busy}>
                Сохранить
              </button>
            </div>
          </form>

          <p className="phone-state">
            <span className="phone-state-label">Состояние:</span> <span className={`phone-state-value phone-state-${view.state}`}>{statusLine(view, now)}</span>
          </p>
          {connError ? <p className="phone-conn-error">{connError}</p> : null}

          <div className="phone-pair">
            {pair?.stage === "starting" ? (
              <>
                <p className="panel-text">Запрашиваем код у сервера…</p>
                <div className="form-actions">
                  <button type="button" className="btn" data-focus="cancel" disabled={busy} onClick={pairCancel}>
                    Отмена
                  </button>
                </div>
              </>
            ) : pair?.stage === "code" ? (
              <>
                <h4 className="subsection-title">Код для телефона</h4>
                <p className="field-hint">Откройте PC Block на телефоне и введите этот код</p>
                <p className="phone-code">
                  {formatCode(pair.code)}
                </p>
                <p className="panel-text phone-code-ttl">Действует 10 минут</p>
                <p className="field-hint phone-countdown">{`Осталось ${countdownText(pair.expiresMs, now)}`}</p>
                <div className="form-actions">
                  <button type="button" className="btn" data-focus="cancel" disabled={busy} onClick={pairCancel}>
                    Отмена
                  </button>
                </div>
              </>
            ) : pair?.stage === "compare" ? (
              <>
                <h4 className="subsection-title">{pair.phoneName ? `Сверка числа: ${pair.phoneName}` : "Сверка числа"}</h4>
                <p className="panel-text">На телефоне должно быть число</p>
                <p className="phone-code">
                  {formatSas(pair.sas)}
                </p>
                <p className="field-hint">Нажмите «Совпадает» и на телефоне</p>
                <div className="form-actions">
                  <button type="button" className="btn btn-primary" data-focus="match" disabled={busy} onClick={() => confirm(true)}>
                    Совпадает
                  </button>
                  <button type="button" className="btn" disabled={busy} onClick={() => confirm(false)}>
                    Не совпадает
                  </button>
                </div>
              </>
            ) : (
              <>
                {pair?.stage === "failed" ? (
                  <p className="field-error" role="alert">
                    {pairErrorText(pair.error)}
                  </p>
                ) : null}
                {view.phone ? (
                  <div className="phone-paired">
                    <p className="panel-text phone-paired-line">{pairedLine(view.phone)}</p>
                    <div className="form-actions">
                      <button type="button" className="btn" data-focus="unpair" disabled={busy} onClick={() => setConfirmUnpair(true)}>
                        Отвязать
                      </button>
                    </div>
                  </div>
                ) : null}
                <div className="form-actions">
                  <button
                    type="button"
                    className={view.phone ? "btn" : "btn btn-primary"}
                    data-focus="pair"
                    disabled={busy}
                    aria-disabled={can.ok ? undefined : "true"}
                    aria-describedby={can.ok ? undefined : pairHintId}
                    onClick={() => can.ok && pairStart()}
                  >
                    {view.phone ? "Подключить другой телефон" : pair?.stage === "failed" ? "Подключить заново" : "Подключить телефон"}
                  </button>
                </div>
                {view.phone ? <p className="field-hint">Прежний телефон будет отвязан, когда вы подключите новый</p> : null}
                {!can.ok ? (
                  <p id={pairHintId} className="phone-why">
                    {can.why}
                  </p>
                ) : null}
              </>
            )}
          </div>
        </>
      ) : null}

      <p className="panel-note phone-message" role="status">
        {message}
      </p>
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}

      {confirmUnpair ? (
        <Dialog
          title="Отвязать телефон?"
          onClose={() => setConfirmUnpair(false)}
          actions={
            <>
              <button type="button" className="btn btn-primary" onClick={unpair}>
                Отвязать
              </button>
              <button type="button" className="btn" data-autofocus onClick={() => setConfirmUnpair(false)}>
                Отмена
              </button>
            </>
          }
        >
          <p>С него больше нельзя будет менять время</p>
        </Dialog>
      ) : null}
    </section>
  );
}

