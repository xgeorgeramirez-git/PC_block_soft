import { useEffect, useRef, useState } from "react";
import { askMore } from "../api";
import { childErrorText, errorField } from "../lib/errors";
import { ASK_PRESETS, askMinutes } from "../lib/mytoday";
import Dialog from "./Dialog";
import Notice from "./Notice";
import TextField from "./TextField";

const LOAD_ERROR = "Не удалось получить данные. Попробуй через минуту";

interface Props {
  /** Номер ребёнка своего сеанса; null - окно его не узнало (служба без него «Попросить ещё» не принимает). */
  childId: number | null;
  /** Время добавлено: готовый текст службы («Добавлено 30 минут»). */
  onDone: (message: string) => void;
  onClose: () => void;
}

/**
 * Панель «Позови родителя» (FR-10, ТЗ 8.2.6): пароль родителя и +15 / +30 / +60 / своё число минут.
 * Пароль проверяет служба; ребёнок сам себе время не добавит. Ребёнку на «ты».
 */
export default function AskMoreDialog({ childId, onDone, onClose }: Props) {
  const [password, setPassword] = useState("");
  const [custom, setCustom] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [minutesError, setMinutesError] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const passwordRef = useRef<HTMLInputElement>(null);

  // Диалог ставит фокус на первую кнопку; ребёнку нужно сначала поле пароля.
  useEffect(() => {
    const t = setTimeout(() => passwordRef.current?.focus(), 0);
    return () => clearTimeout(t);
  }, []);

  const submit = async (minutes: number) => {
    setPasswordError("");
    setMinutesError("");
    setNote("");
    if (password === "") {
      setPasswordError("Введи пароль родителя");
      return;
    }
    if (childId === null) {
      setNote(LOAD_ERROR);
      return;
    }
    setBusy(true);
    try {
      const reply = await askMore(childId, minutes, password);
      onDone(reply.message);
    } catch (e) {
      const text = childErrorText(e);
      const field = errorField(e);
      if (field === "password") setPasswordError(text);
      else if (field === "minutes") setMinutesError(text);
      else setNote(text);
      setBusy(false);
    }
  };

  const submitCustom = () => {
    const n = askMinutes(custom);
    if (n === null) {
      setMinutesError("Можно от 1 до 240 минут");
      return;
    }
    void submit(n);
  };

  return (
    <Dialog
      title="Позови родителя"
      onClose={onClose}
      actions={
        <button type="button" className="btn" onClick={onClose}>
          Отмена
        </button>
      }
    >
      <p className="panel-text">Пусть родитель введёт свой пароль и выберет, сколько времени добавить на сегодня.</p>
      {note ? (
        <Notice tone="warn" urgent>
          {note}
        </Notice>
      ) : null}
      <TextField
        ref={passwordRef}
        label="Пароль родителя"
        type="password"
        value={password}
        onChange={setPassword}
        error={passwordError}
        disabled={busy}
        autoComplete="off"
      />
      <div className="action-row ask-presets" role="group" aria-label="Сколько добавить">
        {ASK_PRESETS.map((m) => (
          <button key={m} type="button" className="btn" disabled={busy} onClick={() => void submit(m)}>
            {`+${m} мин`}
          </button>
        ))}
      </div>
      <div className="ask-custom">
        <TextField
          label="Своё число минут"
          value={custom}
          onChange={(v) => setCustom(v.replace(/\D/g, ""))}
          error={minutesError}
          disabled={busy}
          maxLength={3}
          autoComplete="off"
        />
        <button type="button" className="btn" disabled={busy} onClick={submitCustom}>
          Добавить
        </button>
      </div>
    </Dialog>
  );
}
