import { useCallback, useEffect, useRef, useState } from "react";
import { extendSession, sessionState } from "./api";
import { createSessionWatcher, type SessionView, type SessionWatcher } from "./session";

const FRESH: SessionView = { phase: "active", remainingS: 600 };

/** Следит за родительской сессией, пока окно родителя открыто (R19). */
export function useSessionWatcher() {
  const [view, setView] = useState<SessionView>(FRESH);
  const watcher = useRef<SessionWatcher | null>(null);

  useEffect(() => {
    const w = createSessionWatcher({ fetchState: sessionState, extend: extendSession, onChange: setView });
    watcher.current = w;
    w.start();
    return () => w.stop();
  }, []);

  /** «Продолжить»: продлить сессию. */
  const extend = useCallback(() => watcher.current?.extend() ?? Promise.resolve(), []);

  /** После повторного ввода пароля: начать наблюдение заново. */
  const restart = useCallback(() => {
    setView(FRESH);
    watcher.current?.start();
  }, []);

  return { view, extend, restart };
}
