// Каталог Помощника (FR-34, ТЗ 8.4 п.8): тексты подсказок, туров и «Что нового» лежат здесь как
// данные, а не строками в коде экранов. Экраны знают только номера записей и якоря.
//
// Зеркало для проверки на Go: internal/guidance/catalog.go (номера, тексты, якоря). Тест
// catalog.test.ts сверяет его с этим файлом, а go test проверяет правила (длина, тире, якоря).
// Номера неизменяемы: смысл поменялся - новый номер.
import { genitiveName } from "../lib/format";

export type EntryKind = "hint" | "detector" | "tour" | "whatsnew";

/** Экраны окна родителя (совпадают с меню). */
export type ScreenId = "today" | "children" | "bonus" | "journal" | "settings" | "help";

/** Действия кнопок карточки; что делает каждое, решает Provider. */
export type ActionId = "ack" | "start-tour" | "open-limits" | "how-to-fix" | "open-schedule";

/** Известные якоря окна родителя: элемент помечен data-help-anchor с тем же номером. */
export const ANCHORS = [
  { id: "topbar", screen: "any" },
  { id: "menu-today", screen: "any" },
  { id: "menu-children", screen: "any" },
  { id: "menu-bonus", screen: "any" },
  { id: "savebar", screen: "any" },
  { id: "child-card", screen: "today" },
  { id: "extra-buttons", screen: "today" },
  { id: "child-card-pause", screen: "today" },
  { id: "child-card-bonus", screen: "today" },
  { id: "limits-table", screen: "children" },
  { id: "schedule-tab", screen: "children" },
  { id: "bonus-grant", screen: "bonus" },
] as const;

export type AnchorId = (typeof ANCHORS)[number]["id"];

export interface CardAction {
  id: ActionId;
  label: string;
}

/** Подсказка (hint) или напоминание о риске (detector). Тексты - шаблоны с {name} и {nameGen}. */
export interface HintEntry {
  id: string;
  kind: "hint" | "detector";
  anchor: AnchorId;
  title: string;
  body: string;
  actions: CardAction[];
  /** Приглашение: показывается один раз. */
  once?: boolean;
  /** Повторяющееся: у карточки есть «Больше не напоминать». */
  repeat?: boolean;
  /** Окно «Как исправить» у напоминания. */
  howTo?: { title: string; steps: string[] };
}

export interface TourStep {
  anchor: AnchorId;
  screen: ScreenId;
  title: string;
  body: string;
}

export interface TourEntry {
  id: string;
  kind: "tour";
  title: string;
  steps: TourStep[];
  /** Подсказки, которые тур уже объяснил: после прохождения они не показываются. */
  covers: string[];
}

export interface WhatsNewEntry {
  id: string;
  kind: "whatsnew";
  version: string;
  title: string;
  items: string[];
}

/** Подписи кнопок карточки и тура: общие для всех записей. */
export const LABELS = {
  later: "Позже",
  close: "Закрыть подсказку",
  mute: "Больше не напоминать",
  tourSkip: "Пропустить тур",
  tourNext: "Далее",
  tourBack: "Назад",
  tourDone: "Готово",
  tourMissingNote: "Этот элемент сейчас на экране не виден. Ничего страшного, продолжим.",
} as const;

export const INVITE_ID = "H-M1-01";
export const TOUR_ID = "T-M1-01";

export const HINTS: HintEntry[] = [
  {
    id: "H-M1-01",
    kind: "hint",
    anchor: "topbar",
    title: "Короткий тур по окну родителя: 1 минута",
    body: "Покажем, где смотреть время детей и как менять лимиты. Это необязательно.",
    actions: [{ id: "start-tour", label: "Начать" }],
    once: true,
  },
  {
    id: "H-M1-02",
    kind: "hint",
    anchor: "limits-table",
    title: "Лимиты по дням недели",
    body: "Задайте время на каждый день. «Без лимита» значит, что ограничений по времени нет.",
    actions: [],
  },
  {
    id: "H-M1-03",
    kind: "hint",
    anchor: "extra-buttons",
    title: "Кнопки добавляют время только на сегодня",
    body: "+15, +30 и +60 прибавляют минуты к сегодняшнему лимиту. Завтра лимит снова обычный.",
    actions: [],
  },
  {
    id: "D-M1-01",
    kind: "detector",
    anchor: "child-card",
    title: "Похоже, у {nameGen} нет ограничений ни в один день",
    body: "Время за компьютером не ограничено ни в один день недели. Задайте лимиты, если это не так.",
    actions: [{ id: "open-limits", label: "Задать лимиты" }],
    repeat: true,
  },
  {
    id: "D-M1-02",
    kind: "detector",
    anchor: "child-card",
    title: "У {nameGen} теперь права администратора: ограничения не работают",
    body: "Windows считает эту учётную запись взрослой. Верните обычные права, и ограничения заработают.",
    actions: [{ id: "how-to-fix", label: "Как исправить" }],
    repeat: true,
    howTo: {
      title: "Как вернуть обычные права",
      steps: [
        "Войдите в Windows в своей учётной записи администратора.",
        "Откройте «Параметры», затем «Учётные записи» и «Другие пользователи».",
        "Выберите учётную запись ребёнка и нажмите «Изменить тип учётной записи».",
        "Выберите «Стандартный пользователь» и сохраните. Программа сама заметит перемену.",
      ],
    },
  },
];

export const TOURS: TourEntry[] = [
  {
    id: "T-M1-01",
    kind: "tour",
    title: "Короткий тур по окну родителя",
    steps: [
      {
        anchor: "menu-today",
        screen: "today",
        title: "Сегодня",
        body: "Здесь у каждого ребёнка видно, сколько времени осталось и не заблокирован ли компьютер.",
      },
      {
        anchor: "child-card",
        screen: "today",
        title: "Карточка ребёнка",
        body: "Статус словом и цветом, сколько времени использовано из лимита.",
      },
      {
        anchor: "extra-buttons",
        screen: "today",
        title: "Добавить время",
        body: "+15, +30 и +60 прибавляют минуты на сегодня. Ошибку можно отменить в течение 10 секунд.",
      },
      {
        anchor: "menu-children",
        screen: "children",
        title: "Дети",
        body: "Здесь правила каждого ребёнка.",
      },
      {
        anchor: "limits-table",
        screen: "children",
        title: "Лимиты по дням",
        body: "Задайте время на каждый день недели. «Без лимита» значит, что ограничений нет.",
      },
      {
        anchor: "savebar",
        screen: "children",
        title: "Сохранить",
        body: "Правки начинают действовать после «Сохранить» внизу окна. Лишнее можно отменить.",
      },
    ],
    covers: ["H-M1-02", "H-M1-03"],
  },
];

export const WHATS_NEW: WhatsNewEntry[] = [
  {
    id: "N-0.1.0",
    kind: "whatsnew",
    version: "0.1.0",
    title: "Первая рабочая версия",
    items: [
      "Лимит времени на каждый день недели для каждого ребёнка",
      "Значок у часов с остатком времени",
      "Предупреждения за 15, 5 и 1 минуту",
      "Экран блокировки, когда время вышло, и снятие паролем родителя",
    ],
  },
];

export function hintById(id: string): HintEntry | undefined {
  return HINTS.find((h) => h.id === id);
}

export function tourById(id: string): TourEntry | undefined {
  return TOURS.find((t) => t.id === id);
}

/** Самое длинное имя ребёнка в карточке; дальше усекается (шаблоны проверены на таком имени). */
export const MAX_NAME = 40;

function shortName(name: string): string {
  const chars = [...name.trim()];
  return chars.length > MAX_NAME ? `${chars.slice(0, MAX_NAME - 1).join("")}…` : chars.join("");
}

/** Подставляет имя в шаблон: {name} как есть, {nameGen} в родительном падеже (урок L-36). */
export function fillName(template: string, name: string): string {
  const short = shortName(name);
  return template.replaceAll("{nameGen}", genitiveName(short)).replaceAll("{name}", short);
}
