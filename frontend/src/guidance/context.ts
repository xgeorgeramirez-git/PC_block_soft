// Связка Помощника с экранами: контекст, который даёт GuidanceProvider, и крючки для экранов.
// Без Provider (отдельные тесты экранов, окна ребёнка) всё это безвредно ничего не делает.
import { createContext, useContext, useEffect } from "react";
import type { ActionId, AnchorId, ScreenId, TourEntry } from "./catalog";
import type { Candidate, HelperEngine, QuietZone } from "./engine";

export interface NavigateOptions {
  /** Какого ребёнка выбрать в «Дети». */
  childId?: number;
}

export interface GuidanceApi {
  engine: HelperEngine;
  screen: ScreenId;
  /** Подсказки включены в «Настройки → Помощник». «Помощь» и тур от этого не зависят. */
  enabled: boolean;
  /** Карточка, которую видно сейчас (нет в тихом месте, нет при выключенных подсказках). */
  visible: Candidate | null;
  /** Идущий тур. */
  tour: TourEntry | null;
  /** Сообщает, что на экране есть место для карточек этого якоря; возвращает «снять». */
  register(anchor: AnchorId, subject: string, name: string): () => void;
  later(c: Candidate): void;
  close(c: Candidate): void;
  mute(c: Candidate): void;
  act(c: Candidate, action: ActionId): void;
  startTour(id?: string): void;
  navigate(screen: ScreenId, opts?: NavigateOptions): void;
  resetHidden(): Promise<void>;
  /** Входит в тихое место; возвращает выход. */
  enterQuiet(zone: QuietZone): () => void;
}

export const GuidanceContext = createContext<GuidanceApi | null>(null);

/** Помощник окна родителя или null, если окно без него. */
export function useGuidance(): GuidanceApi | null {
  return useContext(GuidanceContext);
}

/**
 * Пока компонент на экране, Помощник молчит (ТЗ 8.4 п.4): открытый диалог, мастер и подобное.
 * Без Provider ничего не делает.
 */
export function useQuietZone(zone: QuietZone, on = true): void {
  const g = useGuidance();
  const enter = g?.enterQuiet;
  useEffect(() => {
    if (!on || !enter) return;
    return enter(zone);
  }, [enter, zone, on]);
}
