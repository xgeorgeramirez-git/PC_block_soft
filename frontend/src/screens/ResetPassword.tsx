import { useState, type FormEvent } from "react";
import { resetPassword } from "../api";
import Notice from "../components/Notice";
import TextField from "../components/TextField";
import { CallError, errorText } from "../lib/errors";
import { validatePasswordPair, type PasswordPairErrors } from "../lib/password";

/** Сброс пароля родителя из учётной записи администратора (FR-26, R20). */
export default function ResetPassword() {
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [errors, setErrors] = useState<PasswordPairErrors>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    const found = validatePasswordPair(password, again);
    setErrors(found);
    if (found.password || found.again) return;
    setBusy(true);
    setFormError("");
    try {
      await resetPassword(password);
      setDone(true);
    } catch (err) {
      if (err instanceof CallError && err.field === "newPassword") {
        setErrors({ password: err.message });
      } else {
        setFormError(errorText(err));
      }
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <section className="panel login-card" aria-label="Сброс пароля родителя">
        <h2 className="panel-title">Пароль изменён</h2>
        <p className="panel-text">Теперь входите в настройки с новым паролем. Это окно можно закрыть.</p>
      </section>
    );
  }

  return (
    <form className="panel login-card" onSubmit={submit} noValidate>
      <h2 className="panel-title">Сброс пароля родителя</h2>
      <p className="panel-text">
        Вы в учётной записи администратора. Задайте новый пароль родителя, старый не нужен. Не сообщайте его детям.
      </p>
      <TextField
        label="Новый пароль"
        type="password"
        value={password}
        onChange={(v) => {
          setPassword(v);
          setErrors({});
        }}
        error={errors.password}
        hint="Не меньше 6 символов"
        autoComplete="new-password"
        autoFocus
      />
      <TextField
        label="Повторите пароль"
        type="password"
        value={again}
        onChange={(v) => {
          setAgain(v);
          setErrors({});
        }}
        error={errors.again}
        autoComplete="new-password"
      />
      {formError ? (
        <Notice tone="warn" urgent>
          {formError}
        </Notice>
      ) : null}
      <div className="form-actions">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          Задать новый пароль
        </button>
      </div>
    </form>
  );
}
