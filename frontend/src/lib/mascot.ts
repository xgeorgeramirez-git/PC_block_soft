// Талисман окна ребёнка «Игровое лобби» (ТЗ 8.2.6 п. 1, D-109, D-112): неподвижный первый кадр стикера
// заказчика по остатку общего времени. Пороги — та же минута вверх (remainMinutes), что у цвета остатка
// родителя и у значка у часов (core.RemainingMinutes): на 60/61 и 15/16 мин всё переключается вместе.
import m031 from "../../../assets/stickers/031.png?url";
import m033 from "../../../assets/stickers/033.png?url";
import m048 from "../../../assets/stickers/048.png?url";
import m093 from "../../../assets/stickers/093.png?url";
import m104 from "../../../assets/stickers/104.png?url";
import type { ChildStatus } from "../types";
import { remainMinutes } from "./remain";

/** 033 — времени много, 093 — половина, 031 — осталось мало, 048 — время кончилось, 104 — компьютер спит. */
export type Mascot = "033" | "093" | "031" | "048" | "104";

/** Первый кадр каждого талисмана (assets/stickers/NNN.png): в сборку идут ровно эти 5 картинок. */
export const MASCOT_SRC: Record<Mascot, string> = { "033": m033, "093": m093, "031": m031, "048": m048, "104": m104 };

/**
 * Талисман по статусу: идут запретные часы — 104 (главнее остатка); пауза, без лимита, ограничения не
 * действуют — 033; экран «время вышло» — 048; иначе по минутам остатка: > 60 — 033, 16…60 — 093, 1…15 — 031,
 * 0 — 048. Картинка — украшение (alt=""): смысл несут текст и число рядом.
 */
export function mascotOf(s: ChildStatus): Mascot {
  // Запретные часы главнее остатка: ночью компьютер спит при любом остатке.
  if (s.lock === "forbidden" || s.status === "forbidden") return "104";
  // Пауза, без лимита и «ограничения не действуют»: время не уходит.
  if (s.status === "paused" || s.unlimited || s.status === "unlimited" || s.status === "admin" || s.status === "safe") return "033";
  // Экран «время вышло» или блокировка за обход: время кончилось, даже если в ответе ещё остались секунды.
  if (s.lock === "timeUp" || s.lock === "bypass") return "048";
  const m = remainMinutes(s.remainingS);
  if (m > 60) return "033";
  if (m > 15) return "093";
  if (m > 0) return "031";
  return "048";
}

/** Цвет шкалы-энергии по талисману (moods.py): светлый верх и основной цвет. */
export function energyColors(m: Mascot): [string, string] {
  switch (m) {
    case "031":
      return ["#FFE066", "#FFB800"];
    case "048":
      return ["#FFC2A8", "#FF6A4A"];
    case "104":
      return ["#C9C2FF", "#7B6CFF"];
    default:
      return ["#8DFFC4", "#22C47E"];
  }
}
