import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import { fitName } from "../lib/programs";

// Текст одной строкой под ширину места (PL-22): один замер и один помощник обрезки для всех
// экранов, где показываются названия программ («Программы», вкладки «Итогов»).

/**
 * Общий замер «помещается ли в строку» для пути и названий: после отрисовки и при смене размера
 * окна временный зонд с тем же шрифтом (вне разметки React, убирается сразу) меряет кандидатов,
 * fit выбирает самый длинный, что помещается. Без раскладки (тесты) — fallback.
 */
export function useFitted<T extends HTMLElement>(
  text: string,
  fallback: string,
  fit: (fits: (shown: string) => boolean) => string,
): [RefObject<T | null>, string] {
  const [shown, setShown] = useState(fallback);
  const box = useRef<T>(null);
  const fitRef = useRef(fit);
  fitRef.current = fit;
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const measure = () => {
      if (el.clientWidth === 0) {
        setShown(fallback);
        return;
      }
      const probe = document.createElement("span");
      probe.className = "path-probe";
      el.appendChild(probe);
      const room = el.clientWidth + 0.5;
      const fits = (candidate: string) => {
        probe.textContent = candidate;
        return probe.getBoundingClientRect().width <= room;
      };
      try {
        setShown(fitRef.current(fits));
      } finally {
        probe.remove();
      }
    };
    measure();
    const ro = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    ro?.observe(el);
    void document.fonts?.ready.then(measure);
    return () => ro?.disconnect();
    // Пересчёт по тексту: fit берётся из ссылки (последний вид), fallback зависит от текста.
  }, [text, fallback]);
  return [box, shown];
}

/**
 * Название одной строкой (D-59, PL-22): длинное обрезано с конца одним помощником fitName, «…» одно;
 * полное — во всплывающей подсказке и у диктора.
 */
export function LineText({ text, className }: { text: string; className: string }) {
  const [box, shown] = useFitted<HTMLParagraphElement>(text, text, (fits) => fitName(text, fits));
  const cut = shown !== text;
  return (
    <p className={`${className} line-fit`} ref={box} title={cut ? text : undefined}>
      {cut ? (
        <>
          <span aria-hidden="true">{shown}</span>
          <span className="visually-hidden">{text}</span>
        </>
      ) : (
        text
      )}
    </p>
  );
}
