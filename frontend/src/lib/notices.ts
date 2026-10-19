// Важные сообщения службы при входе (GetToday.notices) для родителя: человеческими словами (NFR-11, L-112).

export const RULES_NOT_READ_TEXT =
  "Не удалось прочитать правила, поэтому сейчас ограничения не действуют. Прежние данные сохранены. Задайте правила заново в разделе «Дети».";

export interface HumanNotice {
  text: string;
  /** Что предложить кнопкой рядом: открыть раздел «Дети». */
  action?: "children";
}

// Служба пишет сообщение сама (internal/ipc/commands.go, Ruling 25) и может сказать «база данных»
// или «служба»: родителю эти слова ничего не говорят. Узнаём такое сообщение и говорим своими словами.
const TECHNICAL = /баз[аыуе]\s+данных|служб|повреж|sqlite/i;

export function humanNotice(raw: string): HumanNotice {
  if (TECHNICAL.test(raw)) {
    return { text: RULES_NOT_READ_TEXT, action: "children" };
  }
  return { text: raw.trim() };
}
