// Кого можно показать на экране сейчас: записи каталога, у которых на экране есть своё место
// (HintSlot с тем же якорем), а у напоминаний ещё и найденный риск для этого ребёнка.
import { HINTS, hintById, type AnchorId } from "./catalog";
import type { Candidate } from "./engine";
import type { Risk } from "./detectors";
import { keyOf } from "./state";

/** Место на экране, где может встать карточка. */
export interface SlotInfo {
  anchor: AnchorId;
  /** id ребёнка строкой или пусто. */
  subject: string;
  name: string;
}

export function slotKey(anchor: string, subject = ""): string {
  return `${anchor}|${subject}`;
}

export function buildCandidates(slots: readonly SlotInfo[], risks: readonly Risk[]): Candidate[] {
  const out: Candidate[] = [];
  for (const entry of HINTS) {
    if (entry.kind !== "hint") continue;
    // Подсказка одна на окно: встаёт в первое подходящее место.
    const slot = slots.find((s) => s.anchor === entry.anchor);
    if (slot) {
      out.push({ key: entry.id, entry, subject: slot.subject, name: slot.name, slot: slotKey(slot.anchor, slot.subject) });
    }
  }
  // Напоминания идут в порядке находок (по порядку детей): с первого ребёнка и начинаем.
  for (const risk of risks) {
    const entry = hintById(risk.entryId);
    if (!entry || entry.kind !== "detector") continue;
    const subject = String(risk.childId);
    const slot = slots.find((s) => s.anchor === entry.anchor && s.subject === subject);
    if (slot) {
      out.push({ key: keyOf(entry.id, subject), entry, subject, name: risk.name, slot: slotKey(slot.anchor, subject) });
    }
  }
  return out;
}
