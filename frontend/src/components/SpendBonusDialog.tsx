import { useState } from "react";
import { spendBonus } from "../api";
import { childErrorText } from "../lib/errors";
import { spendChoices, spendConfirmText } from "../lib/mytoday";
import Dialog from "./Dialog";
import Notice from "./Notice";

interface Props {
  /** Баланс копилки, мин. */
  balance: number;
  /** Время взято: готовый текст службы («Взято из копилки 30 мин»). */
  onDone: (message: string) => void;
  onClose: () => void;
}

/**
 * «Взять из копилки» (FR-11): 15, 30, 60 минут или весь баланс, потом подтверждение «Взять из копилки
 * 30 мин? Останется 15 мин». Пароль не нужен: копилка принадлежит ребёнку. Ребёнку на «ты».
 */
export default function SpendBonusDialog({ balance, onDone, onClose }: Props) {
  const [minutes, setMinutes] = useState<number | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    if (minutes === null) return;
    setBusy(true);
    setNote("");
    try {
      // Номер 0: служба берёт ребёнка этого сеанса, чужую копилку тронуть нельзя.
      const reply = await spendBonus(0, minutes);
      onDone(reply.message);
    } catch (e) {
      setNote(childErrorText(e));
      setBusy(false);
    }
  };

  const cancel = (
    <button type="button" className="btn" onClick={onClose}>
      Отмена
    </button>
  );

  if (minutes === null) {
    return (
      <Dialog title="Взять из копилки" onClose={onClose} actions={cancel}>
        <p className="panel-text">{`В копилке ${balance} мин. Сколько взять?`}</p>
        <div className="action-row" role="group" aria-label="Сколько взять">
          {spendChoices(balance).map((c) => (
            <button key={c.minutes} type="button" className="btn" onClick={() => setMinutes(c.minutes)}>
              {c.label}
            </button>
          ))}
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog
      title="Взять из копилки"
      onClose={onClose}
      actions={
        <>
          <button type="button" className="btn btn-primary" autoFocus disabled={busy} onClick={() => void confirm()}>
            Взять
          </button>
          {cancel}
        </>
      }
    >
      {note ? (
        <Notice tone="warn" urgent>
          {note}
        </Notice>
      ) : null}
      <p className="panel-text">{spendConfirmText(minutes, balance)}</p>
    </Dialog>
  );
}
