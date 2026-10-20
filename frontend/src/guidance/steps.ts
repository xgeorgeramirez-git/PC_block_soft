// «Первые шаги» в «Помощи» (ТЗ 8.4 п.6): пункты отмечаются сами по тому, что уже сделано.
// Ничего не меняют, только читают у программы то, что видит родитель.
import { getChildRules, getToday, hello, listEvents } from "../api";
import { normalizeLimits } from "../lib/limits";
import type { JournalEntry, TodayCard, WeekdayLimit } from "../types";
import type { ScreenId } from "./catalog";

export interface FirstStep {
  id: "password" | "children" | "limits" | "child-login" | "extra-time";
  label: string;
  done: boolean;
  /** Куда перейти, чтобы сделать (у невыполненных). */
  go?: ScreenId;
  goLabel?: string;
}

export interface StepInput {
  configured: boolean;
  cards: readonly TodayCard[];
  limitsByChild: Readonly<Record<number, WeekdayLimit[]>>;
  events: readonly JournalEntry[];
}

/** Итог прошлого дня не ноль: ребёнок уже пользовался компьютером. */
function usedBefore(events: readonly JournalEntry[]): boolean {
  return events.some((e) => e.kind === "day_closed" && !/:\s*0 мин$/.test(e.text.trim()));
}

export function computeFirstSteps(input: StepInput): FirstStep[] {
  const hasChildren = input.cards.length > 0;
  const hasLimits = input.cards.some((c) => {
    const limits = input.limitsByChild[c.childId];
    return Boolean(limits) && normalizeLimits(limits).some((l) => !l.unlimited);
  });
  const childLoggedIn = input.cards.some((c) => c.status.usedS > 0) || usedBefore(input.events);
  const timeAdded = input.events.some((e) => e.kind === "extra_time");

  const step = (s: Omit<FirstStep, "go" | "goLabel">, go: ScreenId, goLabel: string): FirstStep =>
    s.done ? s : { ...s, go, goLabel };

  return [
    { id: "password", label: "Пароль родителя задан", done: input.configured },
    step({ id: "children", label: "Дети отмечены", done: hasChildren }, "settings", "Открыть «Настройки»"),
    step({ id: "limits", label: "Лимиты заданы", done: hasLimits }, "children", "Открыть «Дети»"),
    step(
      { id: "child-login", label: "Ребёнок вошёл и увидел значок", done: childLoggedIn },
      "today",
      "Открыть «Сегодня»",
    ),
    step(
      { id: "extra-time", label: "Время добавлено с главной или с экрана блокировки", done: timeAdded },
      "today",
      "Открыть «Сегодня»",
    ),
  ];
}

/** Читает у программы всё нужное для отметок. Не прочитался список детей - ошибка наверх. */
export async function loadFirstSteps(): Promise<FirstStep[]> {
  const [{ configured }, { children }, events] = await Promise.all([
    hello(),
    getToday(),
    listEvents({ limit: 500 }).catch(() => [] as JournalEntry[]),
  ]);
  const limitsByChild: Record<number, WeekdayLimit[]> = {};
  await Promise.all(
    children.map(async (c) => {
      try {
        limitsByChild[c.childId] = (await getChildRules(c.childId)).limits;
      } catch {
        // Правил нет - «лимиты заданы» не отмечаем: честнее, чем гадать.
      }
    }),
  );
  return computeFirstSteps({ configured, cards: children, limitsByChild, events });
}
