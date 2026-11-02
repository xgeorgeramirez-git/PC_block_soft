import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import { fitName } from "../lib/programs";

// Текст в одну или две строки под ширину места (PL-22): один замер и один помощник обрезки для
// всех экранов, где показываются названия программ («Программы», вкладки «Итогов»).

/**
 * Общий замер «помещается ли в строку» для пути и названий: после отрисовки и при смене размера
 * окна временный зонд с тем же шрифтом (вне разметки React, убирается сразу) меряет кандидатов,
 * fit выбирает самый длинный, что помещается. lines > 1: зонд шириной с место и с переносом по
 * словам, «помещается» = не выше lines строк. slack — запас к ширине места: у названий +0,5 px
 * (обрезка CSS скрывает долю пикселя), у пути −1 px (путь может переноситься, и лишняя доля
 * пикселя переносит имя файла на вторую строку). Без раскладки (тесты) — fallback.
 */
export function useFitted<T extends HTMLElement>(
  text: string,
  fallback: string,
  fit: (fits: (shown: string) => boolean) => string,
  lines = 1,
  slack = 0.5,
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
      probe.className = lines > 1 ? "path-probe fit-probe-lines" : "path-probe";
      if (lines > 1) probe.style.width = `${el.clientWidth}px`;
      el.appendChild(probe);
      const room = el.clientWidth + slack;
      const style = getComputedStyle(el);
      const lineH = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.45;
      const tall = lineH * lines + 0.5;
      const fits = (candidate: string) => {
        probe.textContent = candidate;
        const r = probe.getBoundingClientRect();
        return lines > 1 ? r.height <= tall : r.width <= room;
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
  }, [text, fallback, lines, slack]);
  return [box, shown];
}

/**
 * Название в одну строку или (lines = 2) с переносом по словам не больше чем в две (D-59, PL-22):
 * длинное обрезано с конца одним помощником fitName только когда места нет, «…» одно; полное —
 * во всплывающей подсказке и у диктора.
 */
export function LineText({ text, className, lines = 1 }: { text: string; className: string; lines?: 1 | 2 }) {
  const [box, shown] = useFitted<HTMLParagraphElement>(text, text, (fits) => fitName(text, fits), lines);
  const cut = shown !== text;
  return (
    <p className={`${className} line-fit${lines > 1 ? " line-fit-2" : ""}`} data-lines={lines} ref={box} title={cut ? text : undefined}>
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
