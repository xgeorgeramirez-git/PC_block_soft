import { useEffect, useRef, useState } from "react";
import { loadLottie, type AnimationItem } from "../lib/lottie";
import { loadStickerData } from "../lib/stickerData";

interface Props {
  /** Номер стикера из набора R26 («017»). */
  sticker: string;
  /** Надпись ребёнку («Ура! +30 минут»). */
  text: string;
  /** Верный код (оформление удачи) или неверный. */
  ok: boolean;
}

/** В Windows выключены анимации (WebView2 передаёт это как prefers-reduced-motion). */
function reducedMotion(): boolean {
  return typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** Сколько стоит неподвижный стикер, если длина анимации неизвестна (стикеры идут около 3 с). */
const STILL_MS = 3000;

/**
 * Ответ на промокод в окне ребёнка (R26, D-83, D-85, 8.2.6 п.11): стикер 160 x 160 проигрывается
 * один раз и после конца проигрывания исчезает (места не занимает), надпись остаётся. При
 * выключенных анимациях стикер стоит неподвижно на последнем кадре столько, сколько длится
 * анимация (около 3 с), и тоже исчезает. Стикер не загрузился - только надпись. Картинка скрыта
 * от экранного диктора, смысл несёт надпись; живую зону для неё держит PromoRedeem.
 */
export default function PromoSticker({ sticker, text, ok }: Props) {
  const box = useRef<HTMLDivElement>(null);
  /** Картинки нет: стикер не загрузился или уже доигран. */
  const [gone, setGone] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let anim: AnimationItem | null = null;
    let timer: number | undefined;
    setGone(false);
    const finish = () => {
      if (cancelled) return;
      window.clearTimeout(timer);
      anim?.destroy();
      anim = null;
      setGone(true);
    };
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
          const ms = a.getDuration(false) * 1000;
          timer = window.setTimeout(finish, Number.isFinite(ms) && ms > 0 ? ms : STILL_MS);
        } else {
          a.addEventListener("complete", finish);
        }
      })
      .catch(() => {
        if (!cancelled) setGone(true);
      });
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      anim?.destroy();
      anim = null;
    };
  }, [sticker]);

  return (
    <div className={ok ? "promo-answer promo-answer-ok" : "promo-answer promo-answer-bad"}>
      {gone ? null : <div ref={box} className="promo-sticker" aria-hidden="true" data-sticker={sticker} />}
      <p className="promo-answer-text">{text}</p>
    </div>
  );
}
