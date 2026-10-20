// Место для карточки на экране: объявляет Помощнику «здесь можно показать подсказку этого якоря»
// и рисует карточку, если выбрана именно она. Карточка встаёт в поток страницы, а не поверх неё.
import { useEffect } from "react";
import type { AnchorId } from "./catalog";
import { slotKey } from "./candidates";
import { useGuidance } from "./context";
import HintCard from "./HintCard";

interface Props {
  anchor: AnchorId;
  /** id ребёнка строкой: напоминание про ребёнка встаёт только в его место. */
  subject?: string;
  /** Имя ребёнка для подстановки. */
  name?: string;
}

export default function HintSlot({ anchor, subject = "", name = "" }: Props) {
  const g = useGuidance();
  const register = g?.register;
  useEffect(() => register?.(anchor, subject, name), [register, anchor, subject, name]);

  const shown = g?.visible;
  if (!g || !shown || shown.slot !== slotKey(anchor, subject)) return null;
  return (
    <HintCard
      candidate={shown}
      onLater={() => g.later(shown)}
      onClose={() => g.close(shown)}
      onMute={() => g.mute(shown)}
      onAction={(action) => g.act(shown, action)}
    />
  );
}
