// Связь Помощника с программой: состояние профиля читается и пишется через GetHelperState и
// SaveHelperState. Других обращений к программе у движка нет.
import { useEffect, useState } from "react";
import { getHelperState, saveHelperState } from "../api";
import type { HelperEngine, HelperIO } from "./engine";

export const apiHelperIO: HelperIO = {
  load: () => getHelperState(),
  save: (raw) => saveHelperState(raw),
};

/** Перерисовывает компонент, когда состояние движка изменилось; возвращает номер изменения. */
export function useEngineUpdates(engine: HelperEngine): number {
  const [tick, setTick] = useState(0);
  useEffect(() => engine.subscribe(() => setTick((n) => n + 1)), [engine]);
  return tick;
}
