import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { hello, loginDenied, uninstallCheck } from "../api";
import Notice from "../components/Notice";
import TextField from "../components/TextField";
import { usePause } from "../components/usePause";
import { errorText } from "../lib/errors";
import { formatClock } from "../lib/format";

export interface UninstallDecision {
  allowed: boolean;
  /** Отмечена галочка «Удалить статистику». */
  deleteData: boolean;
}

interface Props {
  /**
   * Решение окна. Окно само завершается с кодом, который ждёт установщик (internal/ui):
   * в контракте команд нет поля «Удалить статистику» и команды «завершить с кодом», поэтому
   * признак выбора остаётся здесь и уходит наружу через этот обратный вызов.
   */
  onDecision: (decision: UninstallDecision) => void;
}

/** Окно подтверждения удаления программы (FR-4): пароль родителя, «забыл пароль», «Удалить статистику». */
export default function UninstallGate({ onDecision }: Props) {
  const [configured, setConfigured] = useState<boolean | null>(null);
  const [helloError, setHelloError] = useState("");
  const [password, setPassword] = useState("");
  const [forgot, setForgot] = useState(false);
  const [deleteData, setDeleteData] = useState(false);
  const [error, setError] = useState("");
  const [allowed, setAllowed] = useState(false);
  const [busy, setBusy] = useState(false);
  const pause = usePause();
  const input = useRef<HTMLInputElement>(null);

  const probe = useCallback(async () => {
    setHelloError("");
    try {
      setConfigured((await hello()).configured);
    } catch (e) {
      setHelloError(errorText(e));
    }
  }, []);

  useEffect(() => {
    void probe();
  }, [probe]);

  useEffect(() => {
    if (!pause.active) {
      input.current?.focus();
    }
  }, [pause.active, configured, forgot]);

  const check = async (e?: FormEvent) => {
    e?.preventDefault();
    if (busy || pause.active) return;
    const needsPassword = configured === true && !forgot;
    if (needsPassword && password === "") {
      setError("Введите пароль");
      input.current?.focus();
      return;
    }
    setBusy(true);
    setError("");
    try {
      const reply = await uninstallCheck(needsPassword ? password : "", forgot);
      if (reply.allowed) {
        setAllowed(true);
        onDecision({ allowed: true, deleteData });
      } else {
        setError("Удалять программу сейчас нельзя");
      }
    } catch (err) {
      setPassword("");
      const denied = loginDenied(err);
      if (denied && denied.pauseS > 0) {
        pause.start(denied.pauseS);
      } else if (denied) {
        setError(`Неверный пароль. Осталось попыток: ${denied.attemptsLeft}`);
      } else {
        setError(errorText(err));
      }
    } finally {
      setBusy(false);
    }
  };

  if (helloError) {
    return (
      <section className="panel login-card" aria-label="Удаление программы">
        <h2 className="panel-title">Удаление PC Block</h2>
        <Notice tone="warn" urgent>
          {helloError}
        </Notice>
        <button type="button" className="btn" onClick={() => void probe()}>
          Повторить
        </button>
      </section>
    );
  }
  if (configured === null) {
    return <p className="panel-text">Проверяю состояние программы</p>;
  }

  if (allowed) {
    return (
      <section className="panel login-card" aria-label="Удаление программы">
        <h2 className="panel-title">Удаление PC Block</h2>
        <Notice tone="ok">
          {forgot || !configured ? "Удаление разрешено. Это окно закроется само" : "Пароль верный. Удаление продолжится, это окно закроется само"}
        </Notice>
      </section>
    );
  }

  const shown = pause.active ? `Слишком много попыток. Повторить можно через ${formatClock(pause.left)}` : error;

  return (
    <form className="panel login-card" onSubmit={check} noValidate>
      <h2 className="panel-title">Удаление PC Block</h2>

      {!configured ? (
        <p className="panel-text">Программа ещё не настроена, пароль не нужен.</p>
      ) : forgot ? (
        <p className="panel-text">
          Если вы забыли пароль, удалить программу можно только из учётной записи администратора. Подтвердите удаление:
          правила и защита детских учёток будут сняты.
        </p>
      ) : (
        <>
          <p className="panel-text">Чтобы удалить программу, введите пароль родителя.</p>
          <TextField
            ref={input}
            label="Пароль родителя"
            type="password"
            value={password}
            onChange={(v) => {
              setPassword(v);
              setError("");
            }}
            error={shown}
            disabled={pause.active}
            autoComplete="current-password"
            autoFocus
          />
        </>
      )}

      {forgot && error ? (
        <Notice tone="warn" urgent>
          {error}
        </Notice>
      ) : null}
      {!configured && error ? (
        <Notice tone="warn" urgent>
          {error}
        </Notice>
      ) : null}

      <label className="switch">
        <input type="checkbox" checked={deleteData} onChange={(e) => setDeleteData(e.target.checked)} />
        <span>Удалить статистику</span>
      </label>
      <p className="field-hint">Без галочки статистика останется на компьютере: её можно будет вернуть при новой установке.</p>

      <div className="form-actions">
        {forgot ? (
          <>
            <button type="submit" className="btn btn-primary" disabled={busy}>
              Подтвердить удаление без пароля
            </button>
            <button
              type="button"
              className="btn"
              onClick={() => {
                setForgot(false);
                setError("");
              }}
            >
              Вернуться к паролю
            </button>
          </>
        ) : (
          <>
            <button type="submit" className="btn btn-primary" disabled={busy || pause.active}>
              Продолжить удаление
            </button>
            {configured ? (
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setForgot(true);
                  setError("");
                  setPassword("");
                }}
              >
                Не помню пароль
              </button>
            ) : null}
          </>
        )}
      </div>
      <p className="field-hint">Чтобы отказаться от удаления, закройте это окно.</p>
    </form>
  );
}
