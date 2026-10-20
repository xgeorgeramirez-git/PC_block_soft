// Тур по окну родителя (ТЗ 8.4 п.5): подсвечивает элемент и объясняет его, не больше шести
// шагов, «Пропустить тур» на каждом шаге. Тур только показывает: сам ничего не нажимает и не
// меняет. Элемент не нашёлся - шаг показывается по центру с пояснением, тур не ломается.
import { useEffect, useRef, useState } from "react";
import { LABELS, type ScreenId, type TourEntry } from "./catalog";

/** Сколько ждать, пока нужный элемент появится на экране (данные могут ещё загружаться). */
export const FIND_MS = 3000;
const POLL_MS = 200;
/** Меньше этого подсветка не делается: пустая панель «Сохранить» всё равно видна. */
const MIN_SPOT = 44;
const GAP = 12;
const CARD_W = 360;
const EDGE = 16;

interface Box {
  top: number;
  left: number;
  width: number;
  height: number;
}

type Search = "searching" | "found" | "missing";

interface Props {
  tour: TourEntry;
  screen: ScreenId;
  navigate: (screen: ScreenId) => void;
  /** Тихое место (диалог, ввод пароля): тур прячется, шаг запоминается. */
  hidden?: boolean;
  /** Закончили: true - дошли до конца, false - пропустили. */
  onEnd: (completed: boolean) => void;
}

function sameBox(a: Box | null, b: Box | null): boolean {
  return a === b || (a !== null && b !== null && a.top === b.top && a.left === b.left && a.width === b.width && a.height === b.height);
}

/** Рамка подсветки: не меньше минимального размера, у нижнего края окна - вверх от него. */
function spotOf(r: DOMRect): Box {
  const width = Math.max(r.width, MIN_SPOT);
  const height = Math.max(r.height, MIN_SPOT);
  return { top: r.height < MIN_SPOT ? r.bottom - height : r.top, left: r.left, width, height };
}

function cardPosition(spot: Box, cardH: number): { top: number; left: number } {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const below = spot.top + spot.height + GAP;
  const top = below + cardH + EDGE <= vh ? below : Math.max(EDGE, spot.top - GAP - cardH);
  const left = Math.min(Math.max(spot.left, EDGE), Math.max(EDGE, vw - CARD_W - EDGE));
  return { top, left };
}

export default function Tour({ tour, screen, navigate, hidden = false, onEnd }: Props) {
  const [index, setIndex] = useState(0);
  const [search, setSearch] = useState<Search>("searching");
  const [spot, setSpot] = useState<Box | null>(null);
  const [cardH, setCardH] = useState(190);
  const card = useRef<HTMLDivElement>(null);
  const navigated = useRef(-1);
  const scrolled = useRef(-1);
  const step = tour.steps[index];
  const last = index === tour.steps.length - 1;
  const endRef = useRef(onEnd);
  endRef.current = onEnd;

  // Вернуть фокус туда, где он был, когда тур закончится.
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    return () => previous?.focus?.();
  }, []);

  // Шаг относится к другому экрану: один раз переходим. Не пустили (несохранённые правки) - показываем как есть.
  useEffect(() => {
    if (screen !== step.screen && navigated.current !== index) {
      navigated.current = index;
      navigate(step.screen);
    }
  }, [index, screen, step.screen, navigate]);

  // Ищем подсвечиваемый элемент и следим, чтобы рамка шла за ним (окно меняют, список прокручивают).
  useEffect(() => {
    if (hidden) return;
    setSearch("searching");
    setSpot(null);
    const started = Date.now();
    const measure = () => {
      const el = document.querySelector<HTMLElement>(`[data-help-anchor="${step.anchor}"]`);
      if (!el) {
        setSpot(null);
        setSearch(Date.now() - started >= FIND_MS ? "missing" : "searching");
        return;
      }
      if (scrolled.current !== index) {
        scrolled.current = index;
        el.scrollIntoView?.({ block: "nearest" });
      }
      const next = spotOf(el.getBoundingClientRect());
      setSpot((prev) => (sameBox(prev, next) ? prev : next));
      setSearch("found");
    };
    measure();
    const timer = setInterval(measure, POLL_MS);
    window.addEventListener("resize", measure);
    return () => {
      clearInterval(timer);
      window.removeEventListener("resize", measure);
    };
  }, [index, step.anchor, hidden]);

  // Высота карточки нужна, чтобы поставить её рядом с подсветкой, не выходя за окно.
  useEffect(() => {
    const h = card.current?.offsetHeight;
    if (h && h !== cardH) setCardH(h);
  });

  // Клавиатура: Esc пропускает тур, Tab ходит по кнопкам карточки.
  useEffect(() => {
    if (hidden) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        endRef.current(false);
        return;
      }
      if (e.key !== "Tab" || !card.current) return;
      const items = Array.from(card.current.querySelectorAll<HTMLElement>("button:not([disabled])"));
      if (items.length === 0) return;
      const first = items[0];
      const end = items[items.length - 1];
      const active = document.activeElement;
      if (e.shiftKey && (active === first || !card.current.contains(active))) {
        e.preventDefault();
        end.focus();
      } else if (!e.shiftKey && (active === end || !card.current.contains(active))) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [hidden]);

  // Фокус на «Далее» при каждом показе карточки.
  useEffect(() => {
    if (hidden) return;
    card.current?.querySelector<HTMLElement>("[data-tour-next]")?.focus();
  }, [hidden, index]);

  if (hidden) return null;

  const placed = spot ? cardPosition(spot, cardH) : null;
  return (
    <div className="tour" data-tour-id={tour.id}>
      <div className={spot ? "tour-scrim" : "tour-scrim tour-scrim-dim"} aria-hidden="true" />
      {spot ? (
        <div
          className="tour-spot"
          aria-hidden="true"
          data-anchor={step.anchor}
          style={{ top: spot.top, left: spot.left, width: spot.width, height: spot.height }}
        />
      ) : null}
      <div
        ref={card}
        className={placed ? "tour-card" : "tour-card tour-card-center"}
        role="dialog"
        aria-label={tour.title}
        style={placed ? { top: placed.top, left: placed.left } : undefined}
      >
        <p className="tour-count">{`Шаг ${index + 1} из ${tour.steps.length}`}</p>
        <h2 className="tour-title">{step.title}</h2>
        <p className="tour-body">{step.body}</p>
        {search === "missing" ? <p className="tour-note">{LABELS.tourMissingNote}</p> : null}
        <div className="tour-actions">
          {index > 0 ? (
            <button type="button" className="btn btn-small" onClick={() => setIndex(index - 1)}>
              {LABELS.tourBack}
            </button>
          ) : null}
          <button
            type="button"
            className="btn btn-small btn-primary"
            data-tour-next
            onClick={() => (last ? onEnd(true) : setIndex(index + 1))}
          >
            {last ? LABELS.tourDone : LABELS.tourNext}
          </button>
          <button type="button" className="link tour-skip" onClick={() => onEnd(false)}>
            {LABELS.tourSkip}
          </button>
        </div>
      </div>
    </div>
  );
}
