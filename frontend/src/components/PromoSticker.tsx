import { useEffect, useRef, useState } from "react";
import { loadLottie, type AnimationItem } from "../lib/lottie";
import { loadStickerData } from "../lib/stickerData";

interface Props {
  /** Номер стикера из набора R26 («017»). */
  sticker: string;
  /** Надпись ребёнку («Ура! +30 минут»). */
  text: string;
  /** Верный код: надпись - статус; неверный - предупреждение. */
  ok: boolean;
}

/** В Windows выключены анимации (WebView2 передаёт это как prefers-reduced-motion). */
function reducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/**
 * Ответ на промокод в окне ребёнка (R26, D-83, 8.2.6 п.11): стикер 160 x 160, проигрывается
 * один раз и остаётся на последнем кадре; при выключенных анимациях стоит на последнем кадре
 * сразу. Стикер не загрузился - остаётся только надпись. Картинка скрыта от экранного диктора,
 * смысл несёт надпись.
 */
export default function PromoSticker({ sticker, text, ok }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let anim: AnimationItem | null = null;
    setFailed(false);
    Promise.all([loadLottie(), loadStickerData(sticker)])
      .then(([player, data]) => {
        const el = box.current;
        if (cancelled || !el) return;
        const still = reducedMotion();
        const a = player.loadAnimation({
          container: el,
          renderer: "svg",
          loop: false,
          autoplay: !still,
          animationData: data,
          rendererSettings: { preserveAspectRatio: "xMidYMid meet" },
        });
        anim = a;
        if (still) {
          const toEnd = () => a.goToAndStop(Math.max(0, a.totalFrames - 1), true);
          a.addEventListener("DOMLoaded", toEnd);
          if (a.totalFrames > 0) toEnd();
        }
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
      anim?.destroy();
      anim = null;
    };
  }, [sticker]);

  return (
    <div className={ok ? "promo-answer promo-answer-ok" : "promo-answer promo-answer-bad"}>
      {failed ? null : <div ref={box} className="promo-sticker" aria-hidden="true" data-sticker={sticker} />}
      <p className="promo-answer-text" role={ok ? "status" : "alert"}>
        {text}
      </p>
    </div>
  );
}
