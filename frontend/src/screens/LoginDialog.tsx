import { useEffect, useRef, useState, type FormEvent } from "react";
import { loginDenied, login } from "../api";
import TextField from "../components/TextField";
import { usePause } from "../components/usePause";
import { errorText } from "../lib/errors";
import { formatClock } from "../lib/format";
import type { LoginReply } from "../types";

interface Props {
  onSuccess: (reply: LoginReply) => void;
  /** Подзаголовок: зачем нужен пароль. */
  lead?: string;
}

/** Вход родителя (FR-14, R19): пароль, ошибки и пауза у поля, Enter = «Войти». */
export default function LoginDialog({ onSuccess, lead = "Введите пароль, чтобы открыть настройки" }: Props) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const pause = usePause();
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!pause.active) {
      input.current?.focus();
    }
  }, [pause.active]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy || pause.active) {
      return;
    }
    if (password === "") {
      setError("Введите пароль");
      input.current?.focus();
      return;
    }
    setBusy(true);
    try {
      const reply = await login(password);
      setError("");
      onSuccess(reply);
    } catch (err) {
      setPassword("");
      const denied = loginDenied(err);
      if (denied && denied.pauseS > 0) {
        setError("");
        pause.start(denied.pauseS);
      } else if (denied) {
        setError(`Неверный пароль. Осталось попыток: ${denied.attemptsLeft}`);
      } else {
        setError(errorText(err));
      }
      input.current?.focus();
    } finally {
      setBusy(false);
    }
  };

  const shown = pause.active ? `Слишком много попыток. Повторить можно через ${formatClock(pause.left)}` : error;

  return (
    <form className="login-card" onSubmit={submit} noValidate>
      <h2 className="panel-title">Пароль родителя</h2>
      <p className="panel-text">{lead}</p>
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
      <div className="form-actions">
        <button type="submit" className="btn btn-primary" disabled={pause.active || busy}>
          Войти
        </button>
      </div>
      <p className="field-hint login-forgot">
        Забыли пароль? Сбросить его можно только из учётной записи администратора: меню «Пуск», пункт «PC Block, сброс
        пароля родителя».
      </p>
    </form>
  );
}
