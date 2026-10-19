// Выключатель Помощника для «Настроек». В окне родителя работает через общий движок, чтобы
// карточки пропали или появились сразу; без Помощника вокруг (отдельный экран в тестах) читает и
// пишет то же состояние сам. Меняет одно поле и не затирает остальное.
import { useCallback, useEffect, useMemo, useState } from "react";
import { errorText } from "../lib/errors";
import { useGuidance } from "./context";
import { HelperEngine } from "./engine";
import { apiHelperIO, useEngineUpdates } from "./io";

export interface HelperSwitch {
  /** Состояние прочитано (или не удалось прочитать): переключатель можно трогать. */
  loaded: boolean;
  enabled: boolean;
  /** Что не получилось, простыми словами. */
  error: string;
  toggle(on: boolean): Promise<void>;
  /** Есть окно родителя с карточками: можно вернуть скрытое и пройти тур заново. */
  hasProvider: boolean;
  resetHidden(): Promise<void>;
  restartTour(): void;
}

export function useHelperSwitch(): HelperSwitch {
  const g = useGuidance();
  const hasProvider = g !== null;
  const local = useMemo(() => (hasProvider ? null : new HelperEngine(apiHelperIO)), [hasProvider]);
  const engine = (g?.engine ?? local) as HelperEngine;
  useEngineUpdates(engine);
  const [error, setError] = useState("");

  useEffect(() => {
    if (local) void local.load();
  }, [local]);

  const toggle = useCallback(
    async (on: boolean) => {
      setError("");
      try {
        await engine.setEnabled(on);
      } catch (e) {
        setError(errorText(e));
      }
    },
    [engine],
  );

  const resetHidden = useCallback(async () => {
    setError("");
    try {
      await (g ? g.resetHidden() : engine.resetHidden());
    } catch (e) {
      setError(errorText(e));
    }
  }, [engine, g]);

  return {
    loaded: engine.ready || engine.loadFailed,
    enabled: engine.ready ? engine.enabled : true,
    error: error || (engine.loadFailed ? engine.loadError : ""),
    toggle,
    hasProvider,
    resetHidden,
    restartTour: () => g?.startTour(),
  };
}
