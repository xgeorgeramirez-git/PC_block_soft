import { useState } from "react";
import { redeemPromo } from "../api";
import { childErrorText } from "../lib/errors";

interface Props {
  /** Код принят: окно может перечитать данные (копилку, остаток). */
  onRedeemed?: () => void;
}

/**
 * Поле «Ввести промокод» в окне ребёнка, раздел «Бонусы» (FR-35, R26, D-50). Ребёнку на «ты».
 * Код идёт службе как есть: регистр и пробелы по краям она не учитывает. Номер ребёнка 0 —
 * «мой сеанс»: служба берёт ребёнка по учётке, чужой код применить нельзя.
 */
export default function PromoRedeem({ onRedeemed }: Props) {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setMessage("");
    if (!code.trim()) {
      setError("Введи код");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const r = await redeemPromo(0, code);
      setMessage(r.message);
      setCode("");
      onRedeemed?.();
    } catch (e) {
      setError(childErrorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="promo-redeem">
      <label className="field">
        <span className="field-label">Промокод</span>
        <input
          className={error ? "field-input field-input-error" : "field-input"}
          value={code}
          maxLength={32}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={error ? "true" : undefined}
          disabled={busy}
          onChange={(e) => setCode(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void submit();
          }}
        />
      </label>
      <button type="button" className="btn" disabled={busy} onClick={() => void submit()}>
        Ввести промокод
      </button>
      {error ? (
        <p className="field-error" role="alert">
          {error}
        </p>
      ) : null}
      {message ? (
        <p className="child-feedback" role="status">
          {message}
        </p>
      ) : null}
    </div>
  );
}
