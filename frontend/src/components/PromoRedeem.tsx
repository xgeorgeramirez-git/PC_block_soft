import { useRef, useState } from "react";
import { redeemPromo } from "../api";
import { CallError, childErrorText } from "../lib/errors";
import { promoPicker, type StickerAnswer, type StickerPicker } from "../lib/stickers";
import PromoSticker from "./PromoSticker";

interface Props {
  /** Код принят: окно может перечитать данные (копилку, остаток). */
  onRedeemed?: () => void;
  /** Выбор стикера и надписи; по умолчанию один на окно (переживает смену вкладки). */
  picker?: StickerPicker;
}

/**
 * Поле «Ввести промокод» в окне ребёнка, раздел «Бонусы» (FR-35, R26, D-50, D-83). Ребёнку на «ты».
 * Код идёт службе как есть: регистр и пробелы по краям она не учитывает. Номер ребёнка 0 -
 * «мой сеанс»: служба берёт ребёнка по учётке, чужой код применить нельзя.
 * Ответ на код - стикер и надпись (R26): удача с минутами кода, неудача по полю ошибки «code»
 * (RM5b-10; текст службы «Код не подходит» остаётся для журнала и старых клиентов). Ответ виден,
 * пока ребёнок не начнёт вводить новый код.
 */
export default function PromoRedeem({ onRedeemed, picker = promoPicker }: Props) {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [answer, setAnswer] = useState<(StickerAnswer & { key: number }) | null>(null);
  const [busy, setBusy] = useState(false);
  const shown = useRef(0);

  const show = (a: StickerAnswer) => {
    shown.current += 1;
    setAnswer({ ...a, key: shown.current });
  };

  const submit = async () => {
    setAnswer(null);
    if (!code.trim()) {
      setError("Введи код");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const r = await redeemPromo(0, code);
      show(picker.success(r.addedMin));
      setCode("");
      onRedeemed?.();
    } catch (e) {
      if (e instanceof CallError && !e.link && e.field === "code") show(picker.fail());
      else setError(childErrorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="promo-redeem">
      <div className="promo-redeem-form">
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
            onChange={(e) => {
              setCode(e.target.value);
              setAnswer(null);
              setError("");
            }}
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
      </div>
      {answer ? <PromoSticker key={answer.key} sticker={answer.sticker} text={answer.text} ok={answer.ok} /> : null}
    </div>
  );
}
