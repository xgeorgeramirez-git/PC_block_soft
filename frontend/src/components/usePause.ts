import { useCallback, useEffect, useState } from "react";

/** Отсчёт паузы ввода пароля (R19): секунды до конца по реальным часам, раз в четверть секунды. */
export function usePause() {
  const [endsAt, setEndsAt] = useState<number | null>(null);
  const [left, setLeft] = useState(0);

  useEffect(() => {
    if (endsAt === null) {
      return;
    }
    const tick = () => {
      const seconds = Math.ceil((endsAt - Date.now()) / 1000);
      if (seconds <= 0) {
        setEndsAt(null);
        setLeft(0);
      } else {
        setLeft(seconds);
      }
    };
    tick();
    const id = setInterval(tick, 250);
    return () => clearInterval(id);
  }, [endsAt]);

  const start = useCallback((seconds: number) => {
    setLeft(seconds);
    setEndsAt(Date.now() + seconds * 1000);
  }, []);

  return { active: endsAt !== null, left, start };
}
