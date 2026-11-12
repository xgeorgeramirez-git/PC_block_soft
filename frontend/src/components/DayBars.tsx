import { useEffect, useId, useRef, useState, type CSSProperties, type KeyboardEvent } from "react";
import { axisLabel, barHeights, barScale, barsSummary, showLabel, tipAlign } from "../lib/daybars";
import { formatDuration } from "../lib/format";
import type { MyTodayReply } from "../types";

/** Ширина столбика и зазора в единицах рисунка: столбик 6 из 10. */
const COL = 10;
const BAR = 6;

/**
 * «По дням» (8.2.6 п.4, D-69, D-78): столбик на день или месяц периода, высота - время в счёт лимита,
 * учёба - отдельная часть сверху со штриховкой (отличается не только цветом). Только время: ни линии
 * лимита, ни отметок «сверх лимита». Подписи и подсказки - из ответа службы (RM5-3). Наведение или
 * фокус на столбик - подсказка; стрелки, Home и End ходят по столбикам, Esc прячет подсказку,
 * открытую и мышью (WCAG 1.4.13). Для экранного диктора - итог графика и скрытая таблица тех же чисел.
 */
export default function DayBars({ reply }: { reply: MyTodayReply }) {
  const bars = reply.bars;
  const by = reply.barsBy === "month" ? "month" : "day";
  const n = bars.length;
  const { maxS, ticks } = barScale(bars);
  const todayIndex = bars.findIndex((b) => b.today);
  const home = todayIndex >= 0 ? todayIndex : n - 1;
  const [active, setActive] = useState(home);
  const [tip, setTip] = useState<number | null>(null);
  const hits = useRef<(HTMLDivElement | null)[]>([]);
  const scroll = useRef<HTMLDivElement>(null);
  const pattern = `daybars-study-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const keys = bars.map((b) => b.key).join(",");

  // Другой период - другие столбики: в порядке Tab снова «сегодня», подсказка спрятана, сегодня виден.
  useEffect(() => {
    setActive(home);
    setTip(null);
    const el = scroll.current;
    if (el) el.scrollLeft = el.scrollWidth;
  }, [keys, home]);

  // Esc прячет и подсказку, открытую мышью, где бы ни был фокус (WCAG 1.4.13): слушаем документ, пока она видна.
  const tipOpen = tip !== null;
  useEffect(() => {
    if (!tipOpen) return;
    const onEsc = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") setTip(null);
    };
    document.addEventListener("keydown", onEsc);
    return () => document.removeEventListener("keydown", onEsc);
  }, [tipOpen]);

  if (n === 0) return null;
  const hasStudy = bars.some((b) => b.studyS > 0);
  const longLabels = bars.some((b) => b.label.includes(" ")) && by === "month";
  const current = Math.min(Math.max(active, 0), n - 1);

  const go = (i: number) => {
    setActive(i);
    setTip(i);
    hits.current[i]?.focus();
  };
  const onKey = (e: KeyboardEvent<HTMLDivElement>, i: number) => {
    if (e.key === "ArrowRight") go(Math.min(n - 1, i + 1));
    else if (e.key === "ArrowLeft") go(Math.max(0, i - 1));
    else if (e.key === "Home") go(0);
    else if (e.key === "End") go(n - 1);
    else if (e.key === "Escape") {
      if (tip === null) return;
      setTip(null);
    } else return;
    e.preventDefault();
    e.stopPropagation();
  };

  const align = tip === null ? "center" : tipAlign(tip, n);
  return (
    <figure className="daybars">
      <div
        ref={scroll}
        className="daybars-scroll"
        style={{ "--n": n, "--col-min": by === "day" ? "13px" : "28px" } as CSSProperties}
      >
        <div className="daybars-plot">
          <div className="daybars-axis" aria-hidden="true">
            {/* Замер: те же подписи в потоке, невидимые и без высоты, - колонка оси по самой длинной из них. */}
            <div className="daybars-axis-sizer">
              {ticks.map((t) => (
                <span key={t}>{axisLabel(t)}</span>
              ))}
            </div>
            {ticks.map((t) => (
              <span key={t} className="daybars-tick" style={{ top: `${100 - (t / maxS) * 100}%` }}>
                {axisLabel(t)}
              </span>
            ))}
          </div>
          <div className="daybars-area">
            <svg
              className="daybars-svg"
              viewBox={`0 0 ${n * COL} 100`}
              preserveAspectRatio="none"
              width="100%"
              height="100%"
              role="img"
              aria-label={barsSummary(reply)}
            >
              <defs>
                <pattern id={pattern} width="10" height="4" patternUnits="userSpaceOnUse">
                  <rect className="daybar-hatch" width="10" height="2" />
                </pattern>
              </defs>
              {ticks.map((t) => (
                <line
                  key={t}
                  className="daybars-grid"
                  x1={0}
                  x2={n * COL}
                  y1={100 - (t / maxS) * 100}
                  y2={100 - (t / maxS) * 100}
                  vectorEffect="non-scaling-stroke"
                />
              ))}
              {bars.map((b, i) => {
                const x = i * COL + (COL - BAR) / 2;
                const h = barHeights(b, maxS);
                if (h.empty) {
                  return <line key={b.key} className="daybar-empty" x1={x} x2={x + BAR} y1={100} y2={100} vectorEffect="non-scaling-stroke" />;
                }
                const studyTop = Math.max(0, 100 - h.totalPct - h.studyPct);
                return (
                  <g key={b.key}>
                    {h.totalPct > 0 ? <rect className="daybar-total" x={x} width={BAR} y={100 - h.totalPct} height={h.totalPct} /> : null}
                    {h.studyPct > 0 ? (
                      <rect
                        className="daybar-study"
                        x={x}
                        width={BAR}
                        y={studyTop}
                        height={Math.min(h.studyPct, 100 - h.totalPct)}
                        fill={`url(#${pattern})`}
                        vectorEffect="non-scaling-stroke"
                      />
                    ) : null}
                  </g>
                );
              })}
            </svg>
            <div className="daybars-hits" role="group" aria-label="Столбики: стрелки влево и вправо">
              {bars.map((b, i) => (
                <div
                  key={b.key}
                  ref={(el) => {
                    hits.current[i] = el;
                  }}
                  className="daybar-hit"
                  role="img"
                  aria-label={b.title}
                  aria-current={b.today ? "date" : undefined}
                  tabIndex={i === current ? 0 : -1}
                  onFocus={() => {
                    setActive(i);
                    setTip(i);
                  }}
                  onBlur={() => setTip((t) => (t === i ? null : t))}
                  onMouseEnter={() => setTip(i)}
                  onMouseLeave={() => setTip((t) => (t === i ? null : t))}
                  onKeyDown={(e) => onKey(e, i)}
                />
              ))}
            </div>
            {tip !== null && tip < n ? (
              <div className={`daybars-tip daybars-tip-${align}`} aria-hidden="true" style={{ left: `${((tip + 0.5) / n) * 100}%` }}>
                {bars[tip].title}
              </div>
            ) : null}
          </div>
          <div aria-hidden="true" />
          <div className="daybars-labels" aria-hidden="true">
            {bars.map((b, i) => (
              <span key={b.key} className={b.today ? "daybars-label daybars-label-today" : "daybars-label"}>
                {showLabel(i, n, by, longLabels) ? b.label : ""}
              </span>
            ))}
          </div>
        </div>
      </div>
      <figcaption className="daybars-legend">
        <span className="daybars-key">
          <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
            <rect className="daybar-total" width="12" height="12" />
          </svg>
          В счёт лимита
        </span>
        {hasStudy ? (
          <span className="daybars-key">
            <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
              <rect className="daybar-study" x="0.5" y="0.5" width="11" height="11" fill={`url(#${pattern})`} />
            </svg>
            Учёба, не в счёт лимита
          </span>
        ) : null}
      </figcaption>
      <table className="visually-hidden">
        <caption>{by === "day" ? "По дням" : "По месяцам"}</caption>
        <thead>
          <tr>
            <th scope="col">{by === "day" ? "День" : "Месяц"}</th>
            <th scope="col">В счёт лимита</th>
            <th scope="col">Учёба</th>
          </tr>
        </thead>
        <tbody>
          {bars.map((b) => (
            <tr key={b.key}>
              <th scope="row">{b.label}</th>
              <td>{formatDuration(b.totalS)}</td>
              <td>{formatDuration(b.studyS)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </figure>
  );
}
