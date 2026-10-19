// Помощник окна родителя: держит движок вежливости, реестр мест для карточек, тихие места,
// тур и окно «Как исправить». Экраны знают о нём только через HintSlot, якоря и useQuietZone.
// Сбой Помощника никогда не мешает работе: нет состояния или рисков - он просто молчит.
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import Dialog from "../components/Dialog";
import { TOUR_ID, tourById, type ActionId, type AnchorId, type HintEntry, type ScreenId, type TourEntry } from "./catalog";
import { buildCandidates, slotKey, type SlotInfo } from "./candidates";
import { GuidanceContext, type GuidanceApi, type NavigateOptions } from "./context";
import { loadRisks, type Risk } from "./detectors";
import { HelperEngine, QuietZones, type Candidate, type QuietZone } from "./engine";
import { apiHelperIO, useEngineUpdates } from "./io";
import Tour from "./Tour";

interface Props {
  screen: ScreenId;
  navigate: (screen: ScreenId, opts?: NavigateOptions) => void;
  children: ReactNode;
  /** Для тестов: готовый движок и своё чтение рисков. */
  engine?: HelperEngine;
  readRisks?: () => Promise<Risk[]>;
}

/**
 * Сколько экран должен постоять спокойно (набор мест и рисков не меняется), прежде чем
 * показать карточку: так она не мигает, пока грузятся данные, и напоминание о риске
 * успевает обогнать обычную подсказку.
 */
export const SETTLE_MS = 300;

interface RegisteredSlot extends SlotInfo {
  id: number;
}

interface HowTo {
  entry: HintEntry;
}

export default function GuidanceProvider({ screen, navigate, children, engine: injected, readRisks = loadRisks }: Props) {
  const engine = useMemo(() => injected ?? new HelperEngine(apiHelperIO), [injected]);
  const version = useEngineUpdates(engine);
  const quiet = useMemo(() => new QuietZones(), []);
  const [zones, setZones] = useState<QuietZone[]>([]);
  const [slots, setSlots] = useState<RegisteredSlot[]>([]);
  const [risks, setRisks] = useState<Risk[]>([]);
  const [risksRead, setRisksRead] = useState(false);
  const [active, setActive] = useState<Candidate | null>(null);
  const [hold, setHold] = useState(false);
  const [tour, setTour] = useState<TourEntry | null>(null);
  const [howTo, setHowTo] = useState<HowTo | null>(null);
  const slotSeq = useRef(0);
  const riskSeq = useRef(0);

  // --- Состояние профиля ---------------------------------------------------------------
  useEffect(() => {
    void engine.load();
    // Не прочиталось: пробуем ещё раз, когда человек вернулся в окно.
    const onFocus = () => {
      if (engine.loadFailed) void engine.load();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [engine]);

  // --- Тихие места ---------------------------------------------------------------------
  useEffect(() => quiet.subscribe(() => setZones(quiet.active())), [quiet]);

  // Поле пароля в фокусе: Помощник молчит.
  useEffect(() => {
    let leave: (() => void) | null = null;
    const isPassword = (t: EventTarget | null) => t instanceof HTMLInputElement && t.type === "password";
    const onIn = (e: FocusEvent) => {
      if (isPassword(e.target)) {
        leave ??= quiet.enter("password");
      } else {
        leave?.();
        leave = null;
      }
    };
    const onOut = () => {
      leave?.();
      leave = null;
    };
    document.addEventListener("focusin", onIn);
    document.addEventListener("focusout", onOut);
    return () => {
      document.removeEventListener("focusin", onIn);
      document.removeEventListener("focusout", onOut);
      leave?.();
    };
  }, [quiet]);

  // Идущий тур тоже тихое место для карточек.
  useEffect(() => {
    if (!tour) return;
    return quiet.enter("tour");
  }, [tour, quiet]);

  // --- Места для карточек --------------------------------------------------------------
  const register = useCallback((anchor: AnchorId, subject: string, name: string) => {
    const id = ++slotSeq.current;
    setSlots((prev) => [...prev, { id, anchor, subject, name }]);
    return () => setSlots((prev) => prev.filter((s) => s.id !== id));
  }, []);

  // --- Напоминания о риске -------------------------------------------------------------
  const enabled = engine.ready && engine.enabled;
  const wantsRisks = enabled && slots.some((s) => s.anchor === "child-card");
  const refreshRisks = useCallback(() => {
    const seq = ++riskSeq.current;
    const done = (r: Risk[]) => {
      if (seq !== riskSeq.current) return;
      setRisks(r);
      setRisksRead(true);
    };
    readRisks().then(done, () => done([]));
  }, [readRisks]);
  useEffect(() => {
    if (!wantsRisks) {
      setRisksRead(false);
      return;
    }
    refreshRisks();
    window.addEventListener("focus", refreshRisks);
    return () => window.removeEventListener("focus", refreshRisks);
  }, [wantsRisks, screen, refreshRisks]);

  // --- Выбор карточки ------------------------------------------------------------------
  // Ушли на другой экран: показанная карточка «прошла мимо» и уже не вернётся в этом окне.
  useEffect(() => {
    setActive(null);
    setHold(false);
  }, [screen]);

  // Место карточки пропало с экрана: снимаем и карточку.
  useEffect(() => {
    if (active && !slots.some((s) => slotKey(s.anchor, s.subject) === active.slot)) setActive(null);
  }, [slots, active]);

  const candidates = useMemo(() => buildCandidates(slots, risks), [slots, risks]);
  const isQuiet = zones.length > 0;

  // Пока читаем риски, обычную подсказку не показываем: напоминание о важном идёт первым.
  const waitingRisks = wantsRisks && !risksRead;
  useEffect(() => {
    if (active || tour || waitingRisks) return;
    const timer = setTimeout(() => {
      const next = engine.pick(candidates, isQuiet, hold);
      if (next) {
        engine.markShown(next);
        setActive(next);
      }
    }, SETTLE_MS);
    return () => clearTimeout(timer);
  }, [active, tour, waitingRisks, candidates, isQuiet, hold, engine, version]);

  const visible = enabled && !isQuiet && !tour ? active : null;

  // --- Ответы на карточку --------------------------------------------------------------
  const startTour = useCallback((id: string = TOUR_ID) => {
    const t = tourById(id);
    if (!t) return;
    setActive(null);
    setTour(t);
  }, []);

  const respond = useCallback((apply: () => void) => {
    setActive(null);
    setHold(true);
    apply();
  }, []);

  const later = useCallback((c: Candidate) => respond(() => engine.later(c)), [engine, respond]);
  const close = useCallback((c: Candidate) => respond(() => engine.close(c)), [engine, respond]);
  const mute = useCallback((c: Candidate) => respond(() => engine.mute(c)), [engine, respond]);

  const act = useCallback(
    (c: Candidate, action: ActionId) =>
      respond(() => {
        switch (action) {
          case "ack":
            engine.accept(c);
            break;
          case "start-tour":
            engine.startedTour();
            startTour();
            break;
          case "open-limits":
            // Риск остаётся, пока лимиты не заданы: напоминание вернётся в следующем окне.
            navigate("children", { childId: Number(c.subject) || undefined });
            break;
          case "how-to-fix":
            setHowTo({ entry: c.entry });
            break;
        }
      }),
    [engine, navigate, respond, startTour],
  );

  const endTour = useCallback(
    (completed: boolean) => {
      if (completed && tour) {
        engine.finishTour(tour);
        setActive((a) => (a && tour.covers.includes(a.key) ? null : a));
      }
      setTour(null);
      setHold(true);
    },
    [engine, tour],
  );

  const resetHidden = useCallback(async () => {
    setActive(null);
    setHold(false);
    await engine.resetHidden();
  }, [engine]);

  const api = useMemo<GuidanceApi>(
    () => ({
      engine,
      screen,
      enabled,
      visible,
      tour,
      register,
      later,
      close,
      mute,
      act,
      startTour,
      navigate,
      resetHidden,
      enterQuiet: (zone) => quiet.enter(zone),
    }),
    // version: состояние движка изменилось, потребителям (Помощь, Настройки) нужно перечитать его.
    [engine, screen, enabled, visible, tour, register, later, close, mute, act, startTour, navigate, resetHidden, quiet, version],
  );

  const onlyTour = zones.every((z) => z === "tour");

  return (
    <GuidanceContext.Provider value={api}>
      {children}
      {tour ? <Tour tour={tour} screen={screen} navigate={navigate} hidden={!onlyTour} onEnd={endTour} /> : null}
      {howTo?.entry.howTo ? (
        <Dialog
          title={howTo.entry.howTo.title}
          onClose={() => setHowTo(null)}
          actions={
            <button type="button" className="btn btn-primary" onClick={() => setHowTo(null)}>
              Закрыть
            </button>
          }
        >
          <ol className="howto-steps">
            {howTo.entry.howTo.steps.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        </Dialog>
      ) : null}
    </GuidanceContext.Provider>
  );
}
