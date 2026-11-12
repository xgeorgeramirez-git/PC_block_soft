// Подменная программа для тестов и снимков экранов: ставит window.go.ui.App.{Call, Mode, Version, Finish}
// и отвечает на команды так же, как служба (форматы - internal/ipc/commands.go, примеры -
// internal/ipc/testdata/contract.json). Состояние настраивается: настроена ли программа, дети,
// лимиты, журнал, попытки пароля, родительская сессия.
//
// Файл собирается и в браузерный сценарий Playwright (e2e/helpers.ts): экспорты доступны
// там как PCBlockMock.installBackend(...).
import contract from "../../../internal/ipc/testdata/contract.json";
import { formatDuration, formatHour, formatLimit, genitiveName, localOffsetMin, weekdayOfMs, weekdayShort } from "../lib/format";
import { isViolation } from "../lib/journal";
import { g15Fields } from "./g15";
import { normalizeLimits } from "../lib/limits";
import { breakEqual, DEFAULT_BREAK, validateBreak } from "../lib/rest";
import { ADULT_SOURCE_TEXT, DUPLICATE_TEXT, NOT_DOMAIN_TEXT, cleanDomain } from "../lib/sites";
import type {
  BreakRules,
  Account,
  BonusEntry,
  ChildInfo,
  ChildStatus,
  DayBar,
  Interval,
  JournalEntry,
  JournalGroup,
  ListEventsReply,
  MyRule,
  MyTodayReply,
  Reply,
  TodayCard,
  WeekdayLimit,
  EndMode,
  PendingProgramDTO,
  ProgramCategory,
  ProgramDTO,
  ProgramsReply,
  ProgramTime,
  ListSitesReply,
  SiteDTO,
  SiteKind,
  StatsPeriod,
} from "../types";

export const DEFAULT_PASSWORD = "секрет-123";
/** Версия программы по умолчанию: одна на всех экранах (настоящий номер поднимает оркестратор). */
export const DEFAULT_VERSION = "0.0.1";
const UNDO_WINDOW_MS = 10_000;
// Текст службы (texts.go textCategoryUnchanged): смена категории ничего не изменила (B-41).
const CATEGORY_UNCHANGED = "Категория не изменилась";
const MAX_LIMIT_MIN = 1440;

/** Ребёнок в памяти подменной программы. */
export interface MockChild {
  childId: number;
  sid: string;
  name: string;
  limits: WeekdayLimit[];
  /** Общее время за сегодня, секунды. */
  usedS: number;
  /** Доп. время на сегодня, минуты. */
  extraMin: number;
  /** Баланс копилки, минуты (M2). */
  bonusBalance?: number;
  /** Запретные интервалы расписания (M2). */
  schedule?: Interval[];
  /** История копилки, новые записи первыми (M2). */
  bonusLedger?: BonusEntry[];
  /** Пауза активна до «18:30» (M2). */
  pauseUntil?: string;
  /** Заблокирован за обход защиты, пока родитель не снимет. */
  blocked: boolean;
  /** Подробность для экрана блокировки при обходе. */
  blockDetail: string;
  /** Учётка стала администратором (R1): ограничения не действуют. */
  admin?: boolean;
  /** Сегодня запретные часы сняты родителем (R24, D-51). */
  noForbiddenToday?: boolean;
  /** Сейчас запретные часы: «Можно снова: сегодня в 07:00» (R7). Статус forbidden с блокировкой. */
  forbiddenUntil?: string;
  /** «Когда время вышло» (R8, FR-18); по умолчанию экран блокировки. */
  endMode?: EndMode;
  /** «Отдых» (FR-37); по умолчанию выключен (DEFAULT_BREAK). */
  break?: BreakRules;
  /** Игры за сегодня, секунды (R5); входят и в usedS. */
  gamesUsedS?: number;
  /** Учёба за сегодня, секунды (R3); в usedS не входит. */
  studyUsedS?: number;
  /** «Видео и соцсети» за сегодня, секунды (R6); входят и в usedS. */
  videoUsedS?: number;
  /**
   * Время программ за сегодня для вкладок «Итогов» (D-58), как отдаёт служба: скрытых и запрещённых нет, время
   * без программы - строка «Остальное время» без пути. Нет - пустой список.
   */
  programsToday?: ProgramTime[];
  /**
   * Прошлые дни для периодов «Итогов» (D-61, S42), как их хранит служба (day_state и usage_daily):
   * итоги дня и время программ без строки «Остальное время». Сегодня - usedS, gamesUsedS, studyUsedS и
   * programsToday; день сегодня или позже в периоды не входит (его ведёт ядро).
   */
  history?: MockDay[];
  /** «Блокировать сайты 18+» (R14); нет поля - включено, как у нового ребёнка. */
  adultFilter?: boolean;
  /** Снимок последней «Очистить статистику» (FR-38, RM5-8): момент и счётчики дня на момент очистки. */
  statsReset?: MockStatsReset;
}

/** Снимок очистки статистики: сегодня в «Итогах» - счётчики минус снимок, учёт лимита прежний. */
export interface MockStatsReset {
  clearedAtMs: number;
  usedS: number;
  gamesS: number;
  studyS: number;
  videoS: number;
  extraMin: number;
}

/** Прошлый день ребёнка в подменной программе (периоды «Итогов», D-61). */
export interface MockDay {
  /** YYYY-MM-DD по местному времени. */
  day: string;
  /** Общее время в счёт лимита, с (игры и прочее). */
  totalS?: number;
  gamesS?: number;
  studyS?: number;
  /** «Видео и соцсети», с (R6); входят и в totalS. */
  videoS?: number;
  extraMin?: number;
  bonusSpentMin?: number;
  programs?: ProgramTime[];
}

/** «Сейчас» данных S42: среда 21.10.2026 15:00 (понедельник недели, 1-е число и 2 января уже прошли). */
export const S42_NOW = new Date(2026, 9, 21, 15, 0, 0);

const S42_GAME = "c:\\games\\game.exe";
const S42_STUDY = "c:\\school\\study.exe";

/**
 * Прошлые дни S42 для S42_NOW: в понедельник этой недели игры 30 мин (game.exe), 1-го числа игры 20 мин,
 * 2 января учёба 40 мин (study.exe), в прошлом году прочее 50 мин без программы; строка «завтра» в
 * периоды не входит.
 */
export function s42History(): MockDay[] {
  return [
    { day: "2026-10-19", totalS: 1800, gamesS: 1800, extraMin: 30, programs: [{ path: S42_GAME, name: "game", category: "games", seconds: 1800 }] },
    { day: "2026-10-01", totalS: 1200, gamesS: 1200, programs: [{ path: S42_GAME, name: "game", category: "games", seconds: 1200 }] },
    { day: "2026-01-02", studyS: 2400, bonusSpentMin: 15, programs: [{ path: S42_STUDY, name: "study", category: "study", seconds: 2400 }] },
    { day: "2025-06-15", totalS: 3000 },
    // Завтра (часы компьютера могли уйти назад): в периоды не входит, как у службы.
    { day: "2026-10-22", totalS: 999, gamesS: 999, programs: [{ path: S42_GAME, name: "game", category: "games", seconds: 999 }] },
  ];
}

/** Ребёнок S42: прошлые дни s42History и сегодня game.exe 10 мин. */
export function s42Child(over: Partial<MockChild> = {}): MockChild {
  return mockChild({
    usedS: 600,
    gamesUsedS: 600,
    studyUsedS: 0,
    programsToday: [{ path: S42_GAME, name: "game", category: "games", seconds: 600 }],
    history: s42History(),
    ...over,
  });
}

export { G15_LIMITS, G15_NOW, g15Bonus, g15Events, g15Fields, g15History } from "./g15";

/** Ребёнок G15 (S21): эталонная неделя пн 16 … вс 22.11.2026, копилка 25 мин. */
export function g15Child(over: Partial<MockChild> = {}): MockChild {
  return mockChild({ ...g15Fields(), ...over });
}

/** Промокод в подменной программе (R26, D-50). */
export interface MockPromo {
  codeDisplay: string;
  minutes: number;
  createdAtMs: number;
  usedAtMs?: number;
  usedByChild?: number;
}

export interface MockOptions {
  mode?: string;
  /** Экран окна родителя по ключу --screen (App.StartScreen, RM3-4); по умолчанию пусто. */
  startScreen?: string;
  version?: string;
  testMode?: boolean;
  /** Пароль родителя уже задан, мастер пройден. */
  configured?: boolean;
  password?: string;
  /** Сколько неверных попыток до паузы. */
  attempts?: number;
  /** Длина первой паузы, секунды. */
  pauseS?: number;
  /** Родительская сессия уже открыта (окно запущено из сессии). */
  sessionActive?: boolean;
  /** Длина окна бездействия сессии, секунды. */
  sessionIdleS?: number;
  accounts?: Account[];
  children?: MockChild[];
  /** Журнал на старте, новые сверху; без id - номера выдаются по порядку (верхняя запись - наибольший). */
  events?: MockEventInput[];
  notices?: string[];
  /**
   * Чей сеанс у окна ребёнка (MyToday): childId; null - сеанс не ребёнка (родитель в своей учётке).
   * По умолчанию - первый ребёнок.
   */
  myChildId?: number | null;
  /** Связи с программой нет: любой вызов Call отклоняется как сбой моста. */
  linkDown?: boolean;
  /** Промокоды на старте (R26). */
  promos?: MockPromo[];
  /** «Программы» на старте (FR-20); по умолчанию - как в примере контракта ListPrograms. */
  programs?: ProgramDTO[];
  pending?: PendingProgramDTO[];
  /** Списки «Сайтов» на старте (FR-21); по умолчанию - как в примере контракта ListSites. */
  sites?: SiteDTO[];
}

/** Запись журнала на старте мока: id необязателен. */
export type MockEventInput = Omit<JournalEntry, "id"> & { id?: number };

/** Группы фильтра «Что» журнала - как journalGroups службы (internal/service/journalgroups.go), плюс
 *  виды, которые пишет только мок («settings», «bonus_spend»). Вид не из групп - только «Все события». */
export const MOCK_JOURNAL_GROUPS: Record<Exclude<JournalGroup, "">, readonly string[]> = {
  violations: ["bypass", "program_forbidden_try", "site_forbidden_try"],
  time: [
    "extra_time", "extra_undone", "pause", "pause_end", "no_forbidden", "day_closed",
    "bonus_spent", "bonus_granted", "bonus_grant", "bonus_undo", "bonus_clear", "bonus_reduce",
    "promo_redeem", "promo_create", "promo_generate", "promo_history_cleared", "rest_break", "rest_ended",
    "bonus_spend",
  ],
  programs: [
    "program_decided", "program_undone", "program_hidden", "program_shown", "program_renamed", "program_pending",
    "program_auto_closed", "program_closed_games", "program_closed_study_only",
    "site_closed_video", "site_closed_study_only", "site_unread", "site_added", "site_moved", "site_removed",
    "site_adult_filter", "brave_restored",
  ],
  settings: [
    "setup_done", "password_failed", "password_changed", "password_reset", "uninstall_forgot",
    "settings_changed", "children_changed", "child_admin", "child_admin_off", "stats_cleared", "journal_cleared",
    "block_cleared", "settings",
  ],
  system: ["safe_mode", "internal_error", "db_recovered"],
};

/** Номера записей журнала на старте: без id - по порядку снизу вверх (новые сверху - больше). */
function withEventIds(events: MockEventInput[]): JournalEntry[] {
  return events.map((e, i) => ({ ...e, id: e.id ?? events.length - i }));
}

export interface MockState {
  mode: string;
  startScreen: string;
  version: string;
  testMode: boolean;
  configured: boolean;
  password: string;
  attemptsMax: number;
  attemptsLeft: number;
  pauseS: number;
  pausedUntilMs: number;
  sessionActive: boolean;
  sessionIdleS: number;
  sessionExpiresMs: number;
  accounts: Account[];
  children: MockChild[];
  events: JournalEntry[];
  notices: string[];
  myChildId: number | null;
  /** Ребёнок нажал «Понятно» в окне первого входа (FirstRunSeen). */
  firstRunSeen: boolean;
  /** Окна, которые просили у службы командой OpenWindow, по порядку. */
  openedWindows: string[];
  /** Код, с которым окно закрыли через Finish; null - не закрывали. */
  finishCode: number | null;
  linkDown: boolean;
  nextChildId: number;
  nextExtraId: number;
  extras: { id: string; childId: number; delta: number; undoUntilMs: number }[];
  bonusUndo: Map<string, { childId: number; minutes: number; undoUntilMs: number }>;
  /** «Подробные логи для диагностики» включены (FR-25, D-42). */
  verboseLogs: boolean;
  /** Промокоды (R26, D-50). */
  promos: MockPromo[];
  /** Состояние детерминированного генератора «Придумать коды»: одинаковые коды в каждом прогоне снимков. */
  promoSeed: number;
  /** Настройка автобонуса N (R25, D-49). */
  bonusRate: number;
  /** Сколько раз просили открыть папку логов (FR-25, D-48). */
  openLogsCount: number;
  /** Все программы (FR-20). */
  programs: ProgramDTO[];
  /** «Ждут решения» (R12), новые сверху. */
  pending: PendingProgramDTO[];
  /** «Отменить» решений по программам: снимок списков до решения. */
  programUndo: Map<string, { programs: ProgramDTO[]; pending: PendingProgramDTO[]; undoUntilMs: number }>;
  /** Домены трёх списков «Сайтов»: у домена одна запись (RM4-5). */
  sites: SiteDTO[];
}

export type Override = Reply | "reject" | "garbage" | ((request: any) => Reply | Promise<Reply>);

export interface MockBackend {
  state: MockState;
  /** Все вызовы Call по порядку: команда и разобранная нагрузка. */
  calls: { command: string; payload: any }[];
  call(command: string, payload: string): Promise<string>;
  /** Подменяет ответ команды: times раз (по умолчанию всегда). */
  override(command: string, spec: Override, times?: number): void;
  /** Сколько раз вызывали команду. */
  count(command: string): number;
  /** Обычный ответ команды без подмены (для подмены, которая только задерживает ответ). */
  handler(command: string): (request: any) => Reply;
}

type Examples = Record<string, { request?: unknown; reply: Reply }>;
const examples = contract as unknown as Examples;

/** Папка логов, как её называет служба (FR-25, D-48). */
const MOCK_LOGS_PATH = "C:\\ProgramData\\PCBlock\\logs";

const MISHA_SID = "S-1-5-21-111-222-333-1001";

/** Ребёнок для подменной программы; всё, что не названо, - как в примерах контракта. */
export function mockChild(over: Partial<MockChild> = {}): MockChild {
  const rules = examples.GetChildRules.reply.payload as { limits: WeekdayLimit[] };
  return {
    childId: 1,
    sid: MISHA_SID,
    name: "Миша",
    limits: rules.limits.map((l) => ({ ...l })),
    usedS: 2100,
    extraMin: 0,
    blocked: false,
    blockDetail: "Изменено системное время",
    ...over,
  };
}

/** Учётки Windows как в контракте: администратор Анна, Миша (ребёнок), Даша. */
export function mockAccounts(): Account[] {
  const reply = examples.ListAccounts.reply.payload as { accounts: Account[] };
  return reply.accounts.map((a) => ({ ...a }));
}

/** «1 минуту», «2 минуты», «30 минут». */
function minutesWord(n: number): string {
  if (n % 10 === 1 && n % 100 !== 11) return "минуту";
  if (n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 10 || n % 100 >= 20)) return "минуты";
  return "минут";
}

function okReply(payload: unknown = {}): Reply {
  return { ok: true, payload };
}

function fail(error: string, field?: string, payload?: unknown): Reply {
  const reply: Reply = { ok: false, error };
  if (field) reply.field = field;
  if (payload !== undefined) reply.payload = payload;
  return reply;
}

function isoDay(ms: number): string {
  const d = new Date(ms + localOffsetMin(new Date(ms)) * 60_000);
  return d.toISOString().slice(0, 10);
}

/**
 * Семь дней лимитов, как их отдаёт служба: с полями игр (нет поля - игры без отдельного лимита) и видео
 * (нет videoUnlimited - видео без отдельного лимита, S-3). Поля видео не теряются (T9-1).
 */
function limitsWithGames(limits: WeekdayLimit[] | null | undefined): WeekdayLimit[] {
  return normalizeLimits(limits);
}

function todayLimit(child: MockChild, nowMs: number): WeekdayLimit {
  const wd = weekdayOfMs(nowMs, localOffsetMin(new Date(nowMs)));
  return normalizeLimits(child.limits)[wd - 1];
}

/** Строка о запретных часах сегодня (ТЗ 8.2.1) — упрощённо, как служба: снято (R24) или первый интервал дня. */
function forbiddenNoteOf(child: MockChild, nowMs: number): string {
  if (child.noForbiddenToday) return "Сегодня запретные часы не действуют";
  const wd = weekdayOfMs(nowMs, localOffsetMin(new Date(nowMs)));
  const iv = (child.schedule ?? []).filter((x) => x.weekday === wd).sort((a, b) => a.startMin - b.startMin)[0];
  if (iv) return `Сегодня нельзя с ${formatHour(iv.startMin)} до ${formatHour(iv.endMin)}`;
  // Запретный интервал идёт сейчас, а расписания в данных нет: строка не должна говорить «часов нет».
  const until = /(\d{1,2}:\d{2})/.exec(child.forbiddenUntil ?? "")?.[1];
  return until ? `Сегодня нельзя до ${until}` : "";
}

/** Статус ребёнка: слова и числа собираются в одном месте, как это делает служба. */
export function statusOf(child: MockChild, nowMs: number): ChildStatus {
  const st = statusCore(child, nowMs);
  // «Оставить только учёбу» (R8): время вышло, а экрана блокировки нет.
  if (st.studyOnly) {
    const text = st.lock === "forbidden" ? "Сейчас нельзя пользоваться компьютером. Доступна только учёба" : "Время вышло. Сейчас доступна только учёба";
    return { ...st, lock: "none", tooltip: text, summary: text, closeGames: true };
  }
  return st;
}

/** Остаток и лимит игр, как их считает ядро (R5): нет лимита игр - всё по нулям. */
function gamesOf(child: MockChild, nowMs: number) {
  const limit = todayLimit(child, nowMs);
  const raw = limitsWithGames(child.limits)[limit.weekday - 1];
  if (raw.gamesUnlimited) {
    return { gamesRemainingS: 0, gamesLimitS: 0, gamesUnlimited: true };
  }
  const gamesLimitS = Math.max(0, ((raw.gamesMin ?? 0) + child.extraMin) * 60);
  return { gamesRemainingS: Math.max(0, gamesLimitS - (child.gamesUsedS ?? 0)), gamesLimitS, gamesUnlimited: false };
}

/**
 * Остаток и лимит «Видео и соцсетей», как их считает ядро (R6): нет лимита видео - всё по нулям,
 * лимит с доп. временем; закрыто, когда лимит есть и остатка нет.
 */
function videoOf(child: MockChild, nowMs: number) {
  const limit = todayLimit(child, nowMs);
  const raw = limitsWithGames(child.limits)[limit.weekday - 1];
  if (raw.videoUnlimited ?? true) {
    return { videoRemainingS: 0, videoLimitS: 0, videoUnlimited: true, closeVideo: false };
  }
  const videoLimitS = Math.max(0, ((raw.videoMin ?? 0) + child.extraMin) * 60);
  const videoRemainingS = Math.max(0, videoLimitS - (child.videoUsedS ?? 0));
  return { videoRemainingS, videoLimitS, videoUnlimited: false, closeVideo: videoRemainingS === 0 };
}

/** Лимит видео есть (как hasVideoLimit службы): у админа и без лимита - нет. */
function videoLimited(v: ReturnType<typeof videoOf>): boolean {
  return !v.videoUnlimited && (v.videoLimitS > 0 || v.closeVideo);
}

function statusCore(child: MockChild, nowMs: number): ChildStatus {
  const games = gamesOf(child, nowMs);
  const video = videoOf(child, nowMs);
  const endMode: EndMode = child.endMode ?? "lock";
  const base = {
    ...games,
    ...video,
    endMode,
    closeGames: !games.gamesUnlimited && games.gamesRemainingS === 0,
    studyOnly: false,
    name: child.name,
    day: isoDay(nowMs),
    usedS: child.usedS,
    detail: "",
    until: "",
    pauseUntil: child.pauseUntil ?? "",
    bonusBalance: child.bonusBalance ?? 0,
    forbiddenNote: forbiddenNoteOf(child, nowMs),
    reason: "",
  };
  if (child.admin) {
    const text = "Администратор: ограничения не действуют";
    return {
      ...base,
      status: "admin",
      statusText: text,
      label: "?",
      tooltip: text,
      summary: text,
      remainingS: 0,
      limitS: 0,
      unlimited: true,
      lock: "none",
      used: "",
      gamesRemainingS: 0,
      gamesLimitS: 0,
      gamesUnlimited: true,
      closeGames: false,
      videoRemainingS: 0,
      videoLimitS: 0,
      videoUnlimited: true,
      closeVideo: false,
    };
  }
  const limit = todayLimit(child, nowMs);
  const limitMin = limit.totalMin + child.extraMin;
  const limitS = limit.unlimited ? 0 : limitMin * 60;
  const usedText = limit.unlimited
    ? `Сегодня: ${formatDuration(child.usedS)}`
    : `Сегодня: ${formatDuration(child.usedS)} из ${formatLimit(limitMin)}`;

  if (child.forbiddenUntil) {
    const reason = "Сейчас нельзя пользоваться компьютером";
    return {
      ...base,
      status: "forbidden",
      statusText: "Запретные часы",
      label: "0",
      tooltip: reason,
      summary: reason,
      limitS,
      unlimited: limit.unlimited,
      remainingS: limit.unlimited ? 0 : Math.max(0, limitS - child.usedS),
      lock: "forbidden",
      reason,
      until: child.forbiddenUntil,
      used: usedText,
      closeGames: true,
      studyOnly: endMode === "study_only",
    };
  }

  // Обход больше не блокирует экран (D-41): поле blocked на статус не влияет.
  if (limit.unlimited) {
    return {
      ...base,
      status: "unlimited",
      statusText: "Без лимита",
      label: "∞",
      tooltip: "Лимита на сегодня нет",
      summary: "Лимита на сегодня нет",
      remainingS: 0,
      limitS: 0,
      unlimited: true,
      lock: "none",
      used: usedText,
    };
  }
  const remainingS = Math.max(0, limitS - child.usedS);
  const common = { ...base, limitS, unlimited: false, remainingS, used: usedText };
  if (remainingS === 0) {
    return {
      ...common,
      status: "timeUp",
      statusText: "Время вышло",
      label: "0",
      tooltip: "Время на сегодня закончилось",
      summary: "Время на сегодня закончилось",
      lock: "timeUp",
      reason: "Время на сегодня закончилось",
      until: "Можно снова: завтра",
      pauseUntil: "",
      bonusBalance: 0,
      forbiddenNote: "",
      closeGames: true,
      studyOnly: endMode === "study_only",
    };
  }
  const left = formatDuration(remainingS);
  const low = remainingS <= 15 * 60;
  return {
    ...common,
    status: low ? "low" : "ok",
    statusText: low ? `Осталось ${left}` : "Можно",
    label: String(Math.floor(remainingS / 60)),
    // Подсказка значка как у службы (status.go): остаток, затем игры и видео, если у них есть лимит.
    tooltip: [
      `Осталось ${left}`,
      ...(games.gamesUnlimited ? [] : [`Игры: ${formatDuration(Math.ceil(games.gamesRemainingS / 60) * 60)}`]),
      ...(videoLimited(video) ? [`Видео: ${formatDuration(Math.ceil(video.videoRemainingS / 60) * 60)}`] : []),
    ].join("\n"),
    summary: `Сегодня осталось ${left} из ${formatLimit(limitMin)}`,
    lock: "none",
  };
}

function cardOf(child: MockChild, nowMs: number, pendingPrograms = 0): TodayCard {
  return {
    childId: child.childId,
    sid: child.sid,
    name: child.name,
    status: statusOf(child, nowMs),
    extraMin: child.extraMin,
    bonusBalance: child.bonusBalance ?? 0,
    canClearBlock: false,
    noForbiddenToday: Boolean(child.noForbiddenToday),
    gamesText: gamesTextOf(child, nowMs),
    pendingPrograms,
    videoText: videoTextOf(child, nowMs),
  };
}

/** «Видео 10 мин из 20 мин» (8.3.2 п.3), как ipc.VideoCardText; без лимита видео дня - пусто. */
function videoTextOf(child: MockChild, nowMs: number): string {
  const v = videoOf(child, nowMs);
  if (v.videoUnlimited) return "";
  return `Видео ${formatDuration(child.videoUsedS ?? 0)} из ${formatLimit(v.videoLimitS / 60)}`;
}

/** «Игры 40 мин из 1 ч» (8.3.2 п.3), как ipc.GamesCardText; без лимита игр - пусто. */
function gamesTextOf(child: MockChild, nowMs: number): string {
  const g = gamesOf(child, nowMs);
  if (g.gamesUnlimited) return "";
  return `Игры ${formatDuration(child.gamesUsedS ?? 0)} из ${formatLimit(g.gamesLimitS / 60)}`;
}

/** «Мои правила» текстом, как их собирает служба: «2 ч», «Без лимита», «Нельзя». */
function myRules(child: MockChild): MyRule[] {
  const days: MyRule[] = normalizeLimits(child.limits).map((l) => ({
    weekday: l.weekday,
    day: weekdayShort(l.weekday),
    text: l.unlimited ? "Без лимита" : l.totalMin === 0 ? "Нельзя" : formatLimit(l.totalMin),
  }));
  // Общие правила после дней (R21), как у службы: лимит игр, лимит видео и «Когда время вышло».
  const limits = limitsWithGames(child.limits);
  const games = catRuleText(limits, "Игры", (l) => (l.gamesUnlimited ? null : (l.gamesMin ?? 0)));
  if (games) days.push({ weekday: 0, day: "", text: games });
  const video = catRuleText(limits, "Видео и соцсети", (l) => (l.videoUnlimited ? null : (l.videoMin ?? 0)));
  if (video) days.push({ weekday: 0, day: "", text: video });
  days.push({
    weekday: 0,
    day: "",
    text: child.endMode === "study_only" ? "Когда время вышло: остаётся только учёба" : "Когда время вышло: экран блокировки",
  });
  return days;
}

/**
 * «Игры: до 1 ч в день» / «Видео и соцсети: пн до 1 ч, ср нельзя» (как catRuleText службы). pick - минуты
 * лимита категории дня, null - без отдельного лимита.
 */
function catRuleText(limits: WeekdayLimit[], title: string, pick: (l: WeekdayLimit) => number | null): string {
  const word = (min: number) => (min === 0 ? "нельзя" : `до ${formatLimit(min)}`);
  const limited = limits.flatMap((l) => {
    const min = pick(l);
    return min === null ? [] : [{ weekday: l.weekday, min }];
  });
  if (!limited.length) return "";
  const first = limited[0].min;
  if (limited.length === 7 && limited.every((l) => l.min === first)) {
    return first === 0 ? `${title}: нельзя` : `${title}: ${word(first)} в день`;
  }
  return `${title}: ${limited.map((l) => `${weekdayShort(l.weekday)} ${word(l.min)}`).join(", ")}`;
}

/** Первый день периода по местному времени момента nowMs, как служба (periodFrom); неизвестный - "". */
function periodFromDay(period: string, nowMs: number): string {
  const now = new Date(nowMs);
  const day = (y: number, m: number, d: number) => isoDay(new Date(y, m, d, 12, 0, 0).getTime());
  switch (period) {
    case "today":
      return isoDay(nowMs);
    case "week":
      return day(now.getFullYear(), now.getMonth(), now.getDate() - ((now.getDay() + 6) % 7));
    case "month":
      return day(now.getFullYear(), now.getMonth(), 1);
    case "year":
      return day(now.getFullYear(), 0, 1);
    case "all":
      return "0001-01-01";
  }
  return "";
}

/**
 * Ответ MyToday ребёнка: строки «Сегодня» и правила за сегодня, «Итоги» за период (D-61): прошлые дни
 * history строго до сегодня плюс сегодня. Не ребёнок - пустые списки (не null).
 */
function myToday(child: MockChild | undefined, nowMs: number, period: StatsPeriod = "today", events: JournalEntry[] = []): MyTodayReply {
  if (!child) {
    return { ...(examples["MyToday#empty"].reply.payload as MyTodayReply), period };
  }
  const status = statusOf(child, nowMs);
  const today = [status.summary, `Использовано ${formatDuration(status.usedS)}`];
  if (status.lock === "timeUp" && status.until) {
    today.push(status.until);
  }
  const todayDay = isoDay(nowMs);
  const from = periodFromDay(period, nowMs);
  const past = (child.history ?? []).filter((d) => d.day >= from && d.day < todayDay);
  // Начало периода в мс (местная полночь); «Всё время» - без границы, как служба (dayStartMs).
  const fromMs = from === "0001-01-01" ? 0 : new Date(Number(from.slice(0, 4)), Number(from.slice(5, 7)) - 1, Number(from.slice(8, 10))).getTime();
  // «Статистика ведётся с» - дата очистки; без очистки - первый день истории до сегодня, иначе сегодня (RM5-8).
  const reset = child.statsReset;
  const since = reset
    ? isoDay(reset.clearedAtMs)
    : [...(child.history ?? []).map((d) => d.day).filter((d) => d < todayDay), todayDay].sort()[0];
  // Сегодня - счётчики дня минус снимок очистки этого дня (не ниже нуля), как служба (liveMinusReset).
  const resetToday = reset && isoDay(reset.clearedAtMs) === todayDay ? reset : undefined;
  const live = (v: number, snap: number | undefined) => Math.max(0, v - (snap ?? 0));
  const liveUsedS = live(status.usedS, resetToday?.usedS);
  const liveStudyS = live(child.studyUsedS ?? 0, resetToday?.studyS);
  const sum = (pick: (d: MockDay) => number | undefined) => past.reduce((n, d) => n + (pick(d) ?? 0), 0);
  const usedS = liveUsedS + sum((d) => d.totalS);
  const gamesS = live(child.gamesUsedS ?? 0, resetToday?.gamesS) + sum((d) => d.gamesS);
  const studyS = liveStudyS + sum((d) => d.studyS);
  const videoS = live(child.videoUsedS ?? 0, resetToday?.videoS) + sum((d) => d.videoS);
  // «Прочее» и «Всего» - как у службы (stats.go): общее время без игр и видео и общее плюс учёба.
  const otherS = Math.max(0, usedS - gamesS - videoS);
  const merged = new Map<string, ProgramTime>();
  for (const p of [...(child.programsToday ?? []), ...past.flatMap((d) => d.programs ?? [])]) {
    const key = `${p.category}|${p.path}`;
    const cur = merged.get(key);
    if (cur) cur.seconds += p.seconds;
    else merged.set(key, { ...p });
  }
  const programs = [...merged.values()];
  // Как служба: время категории, не названное строками, - строка «Остальное время» (сумма строк = итог).
  for (const [category, total] of [["games", gamesS], ["video_social", videoS], ["study", studyS], ["other", otherS]] as const) {
    const named = programs.filter((p) => p.category === category).reduce((n, p) => n + p.seconds, 0);
    if (total > named) programs.push({ path: "", name: "Остальное время", category, seconds: total - named });
  }
  return {
    status,
    today,
    rules: myRules(child),
    period,
    usedS,
    gamesS,
    videoS,
    studyS,
    otherS,
    allS: usedS + studyS,
    extraMin: live(child.extraMin ?? 0, resetToday?.extraMin) + sum((d) => d.extraMin),
    bonusSpentMin: sum((d) => d.bonusSpentMin),
    programs,
    ...mockBars(period, from, todayDay, past, liveUsedS, liveStudyS),
    violations: events
      .filter((e) => e.childId === child.childId && isViolation(e.kind) && e.atMs >= fromMs)
      .sort((a, b) => b.atMs - a.atMs)
      .slice(0, 200),
    bonus: (child.bonusLedger ?? []).filter((b) => b.atMs >= fromMs).slice(0, 200),
    statsSinceDay: since,
    statsSinceText: `Статистика ведётся с ${Number(since.slice(8, 10))} ${MONTHS_GEN[Number(since.slice(5, 7)) - 1]} ${since.slice(0, 4)}`,
  };
}

const MONTHS_SHORT = ["янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
const MONTHS_NOM = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];
const MONTHS_GEN = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

/** Подсказка столбика, как у службы (stats_days.go barTitle). */
function barTitle(when: string, totalS: number, studyS: number): string {
  return `${when}: ${formatDuration(totalS)}, учёба ${formatDuration(studyS)}`;
}

/**
 * Столбики «По дням», как служба (buildBars): «Сегодня», «Неделя», «Месяц» - по дню с начала периода по
 * сегодня; «Год» - по месяцу с января; «Всё время» - по месяцу с первого месяца данных. Пустые дни и месяцы
 * внутри - нулевые столбики; сегодня - из счётчиков дня (не из истории).
 */
function mockBars(
  period: StatsPeriod,
  from: string,
  todayDay: string,
  past: MockDay[],
  todayTotalS: number,
  todayStudyS: number,
): Pick<MyTodayReply, "bars" | "barsBy"> {
  const monthly = period === "year" || period === "all";
  const keyOf = (day: string) => (monthly ? day.slice(0, 7) : day);
  const sums = new Map<string, { total: number; study: number }>();
  const add = (key: string, total: number, study: number) => {
    const cur = sums.get(key) ?? { total: 0, study: 0 };
    sums.set(key, { total: cur.total + total, study: cur.study + study });
  };
  for (const d of past) add(keyOf(d.day), d.totalS ?? 0, d.studyS ?? 0);
  add(keyOf(todayDay), todayTotalS, todayStudyS);
  const noon = (day: string) => new Date(Number(day.slice(0, 4)), Number(day.slice(5, 7)) - 1, Number(day.slice(8, 10)), 12);
  const end = noon(todayDay);
  if (!monthly) {
    const bars: DayBar[] = [];
    for (let t = noon(from); t <= end; t = new Date(t.getFullYear(), t.getMonth(), t.getDate() + 1, 12)) {
      const key = isoDay(t.getTime());
      const s = sums.get(key) ?? { total: 0, study: 0 };
      const wd = weekdayShort(((t.getDay() + 6) % 7) + 1);
      bars.push({
        key,
        label: `${wd} ${t.getDate()}`,
        title: barTitle(`${wd}, ${t.getDate()} ${MONTHS_GEN[t.getMonth()]}`, s.total, s.study),
        totalS: s.total,
        studyS: s.study,
        today: key === todayDay,
      });
    }
    return { bars, barsBy: "day" };
  }
  const firstDay = period === "all" ? [...past.map((d) => d.day), todayDay].sort()[0] : from;
  const start = noon(firstDay);
  const withYear = start.getFullYear() !== end.getFullYear();
  const bars: DayBar[] = [];
  for (let t = new Date(start.getFullYear(), start.getMonth(), 1, 12); t <= end; t = new Date(t.getFullYear(), t.getMonth() + 1, 1, 12)) {
    const key = `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, "0")}`;
    const s = sums.get(key) ?? { total: 0, study: 0 };
    bars.push({
      key,
      label: withYear ? `${MONTHS_SHORT[t.getMonth()]} ${t.getFullYear()}` : MONTHS_SHORT[t.getMonth()],
      title: barTitle(`${MONTHS_NOM[t.getMonth()]} ${t.getFullYear()}`, s.total, s.study),
      totalS: s.total,
      studyS: s.study,
      today: key === todayDay.slice(0, 7),
    });
  }
  return { bars, barsBy: "month" };
}

const PERIODS_OK = new Set(["today", "week", "month", "year", "all"]);

/** Период запроса MyToday или ParentStats: нет - «Сегодня»; неизвестный - null (служба отвечает отказом). */
function periodArg(req: { period?: string } | undefined): StatsPeriod | null {
  const p = req?.period ?? "today";
  return PERIODS_OK.has(p) ? (p as StatsPeriod) : null;
}

const PARENT_COMMANDS = new Set([
  "GetToday",
  "GetChildRules",
  "SaveLimits",
  "AddExtra",
  "UndoExtra",
  "ClearBlock",
  "ListEvents",
  "ChangePassword",
  "SetChildren",
  "ExtendSession",
  "Logout",
  "GetSchedule",
  "SaveSchedule",
  "Pause",
  "PauseAll",
  "GrantBonus",
  "UndoBonus",
  "GetBonus",
  "CopyRules",
  "ReduceExtra",
  "ClearExtra",
  "ParentStats",
  "GetDiag",
  "SetDiag",
  "SetNoForbidden",
  "ClearBonus",
  "ReduceBonus",
  "CreatePromo",
  "ListPromos",
  "GetBonusRate",
  "SetBonusRate",
  "OpenLogs",
  "ListPrograms",
  "DecideProgram",
  "SetProgramsCategory",
  "SetProgramsHidden",
  "RenameProgram",
  "UndoProgram",
  "ListSites",
  "AddSite",
  "RemoveSite",
  "AddSites",
  "RemoveSites",
  "SetAdultFilter",
  "ClearStats",
  "ClearJournal",
  "GeneratePromos",
  "ClearPromoHistory",
]);

/** Нормализация промокода как в ядре (R26): без пробелов по краям и без учёта регистра. */
const promoNorm = (code: string) => code.trim().toLowerCase();

/** Новый код как core.CheckNewPromoCode (RM5-11): только A-Z, a-z, 0-9, 3-16 знаков. */
const PROMO_NEW_CODE = /^[A-Za-z0-9]{3,16}$/;

/** Составы «Придумать коды», как promoAlphabets службы (RM5-12): с буквами без I, O, 0, 1. */
const PROMO_ALPHABETS: Record<string, string> = {
  digits: "0123456789",
  letters: "ABCDEFGHJKLMNPQRSTUVWXYZ",
  mixed: "ABCDEFGHJKLMNPQRSTUVWXYZ23456789",
};

/** Форма слова по числу, как pluralRu службы: «1 код», «2 кода», «5 кодов», «11 кодов». */
function pluralRu(n: number, one: string, few: string, many: string): string {
  const m = Math.abs(n);
  if (m % 100 >= 11 && m % 100 <= 14) return many;
  if (m % 10 === 1) return one;
  if (m % 10 >= 2 && m % 10 <= 4) return few;
  return many;
}

/** Строка журнала «Придумать коды», как promoGenerateText службы. */
function promoGenerateText(n: number, minutes: number): string {
  const verb = n % 10 === 1 && n % 100 !== 11 ? "Придуман" : "Придумано";
  return `${verb} ${n} ${pluralRu(n, "промокод", "промокода", "промокодов")} на ${minutes} мин`;
}

/** Промокод для окна, как promoViewOf службы: код заглавными. */
const promoView = (p: MockPromo): MockPromo => ({ ...p, codeDisplay: p.codeDisplay.toUpperCase() });

function makeState(opts: MockOptions): MockState {
  const configured = opts.configured ?? true;
  const children = opts.children ?? (configured ? [mockChild()] : []);
  const attempts = opts.attempts ?? 5;
  return {
    mode: opts.mode ?? "window",
    startScreen: opts.startScreen ?? "",
    version: opts.version ?? DEFAULT_VERSION,
    testMode: opts.testMode ?? false,
    configured,
    password: opts.password ?? DEFAULT_PASSWORD,
    attemptsMax: attempts,
    attemptsLeft: attempts,
    pauseS: opts.pauseS ?? 60,
    pausedUntilMs: 0,
    sessionActive: opts.sessionActive ?? false,
    sessionIdleS: opts.sessionIdleS ?? 600,
    sessionExpiresMs: 0,
    accounts: opts.accounts ?? mockAccounts(),
    children,
    events: withEventIds(opts.events ?? []),
    notices: opts.notices ?? [],
    myChildId: opts.myChildId === undefined ? (children[0]?.childId ?? null) : opts.myChildId,
    firstRunSeen: false,
    openedWindows: [],
    finishCode: null,
    linkDown: opts.linkDown ?? false,
    nextChildId: Math.max(0, ...children.map((c) => c.childId)) + 1,
    nextExtraId: 17,
    extras: [],
    bonusUndo: new Map(),
    verboseLogs: false,
    promos: opts.promos ?? [],
    promoSeed: 20261111,
    bonusRate: 5,
    openLogsCount: 0,
    programs: (opts.programs ?? exampleProgramsReply().programs).map((p) => ({ ...p })),
    pending: (opts.pending ?? exampleProgramsReply().pending).map((p) => ({ ...p, byChild: [...p.byChild] })),
    programUndo: new Map(),
    sites: (opts.sites ?? exampleSites()).map((x) => ({ ...x })),
  };
}

/** Сайты из примера контракта ListSites (копия). */
function exampleSites(): SiteDTO[] {
  return JSON.parse(JSON.stringify((examples.ListSites.reply.payload as ListSitesReply).sites)) as SiteDTO[];
}

/** Порядок списков в ответе, как у хранилища (ListSiteRules: по списку, затем по домену). */
const SITE_KIND_ORDER: SiteKind[] = ["blocked", "study", "video_social"];
/** Предел «Запрещённых» (ipc.MaxBlockedSites). */
const MAX_BLOCKED_SITES = 1000;
/** Названия списков в сообщениях и журнале (service.siteListNames): винительный и родительный падежи. */
const SITE_LIST_NAMES: Record<SiteKind, { acc: string; gen: string }> = {
  blocked: { acc: "«Запрещённые»", gen: "«Запрещённых»" },
  study: { acc: "«Учёбу»", gen: "«Учёбы»" },
  video_social: { acc: "«Видео и соцсети»", gen: "«Видео и соцсетей»" },
};
const isSiteKind = (k: unknown): k is SiteKind => typeof k === "string" && (SITE_KIND_ORDER as string[]).includes(k);

/** «Программы» из примера контракта (копия). */
function exampleProgramsReply(): ProgramsReply {
  return JSON.parse(JSON.stringify(examples.ListPrograms.reply.payload)) as ProgramsReply;
}

const PROGRAM_CATEGORY_WORD: Record<string, string> = { games: "игра", study: "учёба", other: "прочее", blocked: "запрещена" };
const PROGRAM_CATEGORY_TITLE: Record<string, string> = { games: "Игры", study: "Учёба", other: "Прочее" };

/** Строка журнала о решении родителя, как у службы (R22): «Вы отнесли «X» к категории «Игры»». */
function programDecidedText(name: string, category: string): string {
  return category === "blocked" ? `Вы запретили «${name}»` : `Вы отнесли «${name}» к категории «${PROGRAM_CATEGORY_TITLE[category]}»`;
}

export function createBackend(opts: MockOptions = {}): MockBackend {
  const state = makeState(opts);
  const calls: MockBackend["calls"] = [];
  const overrides = new Map<string, { spec: Override; left: number }>();
  if (opts.sessionActive) {
    state.sessionExpiresMs = Date.now() + state.sessionIdleS * 1000;
  }

  const sessionOpen = (now: number) => state.sessionActive && now < state.sessionExpiresMs;
  const touch = (now: number) => {
    state.sessionExpiresMs = now + state.sessionIdleS * 1000;
  };
  const addEvent = (now: number, child: MockChild | undefined, kind: string, text: string) => {
    state.events.unshift({
      id: state.events.reduce((m, e) => Math.max(m, e.id), 0) + 1,
      atMs: now,
      tzOffsetMin: localOffsetMin(new Date(now)),
      childId: child?.childId ?? 0,
      childName: child?.name ?? "",
      kind,
      text,
    });
  };
  const childById = (id: number) => state.children.find((c) => c.childId === id);
  const card = (child: MockChild, now: number) => cardOf(child, now, state.pending.length);
  const programsReply = (): ProgramsReply => ({
    pending: state.pending.map((p) => ({ ...p, byChild: [...p.byChild] })),
    // Как служба: скрытая приходит с флагом и без времени за неделю (D-57).
    programs: [...state.programs]
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((p) => (p.hidden ? { ...p, weekS: 0 } : { ...p })),
    children: state.children.map((c) => ({ childId: c.childId, name: c.name })),
  });
  const rememberUndo = (now: number) => {
    const undoId = `p-${state.nextExtraId++}`;
    state.programUndo.clear(); // как служба: отменить можно только последнюю операцию
    state.programUndo.set(undoId, {
      programs: state.programs.map((p) => ({ ...p })),
      pending: state.pending.map((p) => ({ ...p, byChild: [...p.byChild] })),
      undoUntilMs: now + UNDO_WINDOW_MS,
    });
    return { undoId, undoUntilMs: now + UNDO_WINDOW_MS };
  };
  const sitesReply = (): ListSitesReply => ({
    sites: [...state.sites]
      .sort((a, b) => SITE_KIND_ORDER.indexOf(a.kind) - SITE_KIND_ORDER.indexOf(b.kind) || (a.domain < b.domain ? -1 : a.domain > b.domain ? 1 : 0))
      .map((x) => ({ ...x })),
    children: state.children.map((c) => ({ childId: c.childId, name: c.name, on: c.adultFilter ?? true })),
    blockedMax: MAX_BLOCKED_SITES,
    adultSource: ADULT_SOURCE_TEXT,
  });
  const parentCategory = (c: unknown): c is ProgramCategory => typeof c === "string" && c in PROGRAM_CATEGORY_WORD;

  // Неверный пароль: считаем попытки, после серии закрываем ввод на паузу (R19).
  const deny = (now: number): Reply => {
    state.attemptsLeft -= 1;
    if (state.attemptsLeft <= 0) {
      state.pausedUntilMs = now + state.pauseS * 1000;
      state.attemptsLeft = state.attemptsMax;
      state.pauseS = Math.min(state.pauseS * 2, 900);
      return fail("Слишком много попыток", "password", {
        attemptsLeft: 0,
        pauseS: Math.ceil((state.pausedUntilMs - now) / 1000),
      });
    }
    return fail(`Неверный пароль. Осталось попыток: ${state.attemptsLeft}`, "password", {
      attemptsLeft: state.attemptsLeft,
      pauseS: 0,
    });
  };
  const pausedReply = (now: number): Reply | null =>
    now < state.pausedUntilMs
      ? fail("Слишком много попыток", "password", {
          attemptsLeft: 0,
          pauseS: Math.ceil((state.pausedUntilMs - now) / 1000),
        })
      : null;

  const limitsError = (limits: WeekdayLimit[]): Reply | null => {
    for (const l of limits) {
      if (!l.unlimited && l.totalMin > MAX_LIMIT_MIN) {
        return fail("Не больше 24 ч", `limits.${l.weekday}`);
      }
      if (l.gamesUnlimited === false && (l.gamesMin ?? 0) > MAX_LIMIT_MIN) {
        return fail("Не больше 24 ч", `games.${l.weekday}`);
      }
      if (l.videoUnlimited === false && (l.videoMin ?? 0) > MAX_LIMIT_MIN) {
        return fail("Не больше 24 ч", `video.${l.weekday}`);
      }
    }
    return null;
  };

  const handlers: Record<string, (req: any, now: number) => Reply> = {
    Hello: () =>
      okReply({ protocol: 1, version: state.version, testMode: state.testMode, configured: state.configured }),

    // Набор детей один на все экраны: у каждого ребёнка есть учётка, даже если в списке она не названа.
    ListAccounts: () => {
      const known = state.accounts.map((a) => ({ ...a, child: state.children.some((c) => c.sid === a.sid) }));
      const missing: Account[] = state.children
        .filter((c) => !state.accounts.some((a) => a.sid === c.sid))
        .map((c) => ({ sid: c.sid, name: c.name, admin: false, child: true, current: false, canBeChild: true }));
      return okReply({ accounts: [...known, ...missing] });
    },

    Setup: (req, now) => {
      if (state.configured) return fail("Программа уже настроена");
      if (typeof req.password !== "string" || [...req.password].length < 6) {
        return fail("Не меньше 6 символов", "password");
      }
      if (!req.children?.length) return fail("Отметьте хотя бы одного ребёнка", "children");
      for (const c of req.children) {
        const bad = limitsError(c.limits ?? []);
        if (bad) return bad;
      }
      // Шаг «Сайты» (handlers.go): video заменяет список целиком, повторы схлопываются, не домен - отказ.
      const sites = req.sites as { adultOff?: string[]; video?: string[] } | undefined;
      const video: string[] = [];
      for (const raw of sites?.video ?? []) {
        const domain = cleanDomain(String(raw));
        if (!domain) return fail(NOT_DOMAIN_TEXT, "domain");
        if (!video.includes(domain)) video.push(domain);
      }
      state.password = req.password;
      state.configured = true;
      const adultOff = new Set(sites?.adultOff ?? []);
      state.children = req.children.map((c: { sid: string; name: string; limits: WeekdayLimit[] }) =>
        mockChild({
          childId: state.nextChildId++,
          sid: c.sid,
          name: c.name,
          limits: c.limits,
          usedS: 0,
          ...(adultOff.has(c.sid) ? { adultFilter: false } : {}),
        }),
      );
      if (sites) {
        state.sites = [...state.sites.filter((x) => x.kind !== "video_social" && !video.includes(x.domain)), ...video.map((domain) => ({ domain, kind: "video_social" as const }))];
      }
      addEvent(now, undefined, "settings", "Программа настроена");
      return okReply();
    },

    ResetPassword: (req) => {
      if ([...String(req.newPassword ?? "")].length < 6) return fail("Не меньше 6 символов", "newPassword");
      state.password = req.newPassword;
      state.attemptsLeft = state.attemptsMax;
      state.pausedUntilMs = 0;
      return okReply();
    },

    UninstallCheck: (req, now) => {
      if (!state.configured || req.forgot) return okReply({ allowed: true });
      const paused = pausedReply(now);
      if (paused) return paused;
      if (req.password === state.password) return okReply({ allowed: true });
      return deny(now);
    },

    Login: (req, now) => {
      const paused = pausedReply(now);
      if (paused) return paused;
      if (req.password !== state.password) return deny(now);
      state.attemptsLeft = state.attemptsMax;
      state.sessionActive = true;
      touch(now);
      return okReply({ token: "9f2c1b7e4a6d4c0e8b53", expiresMs: state.sessionExpiresMs, idleTimeoutS: state.sessionIdleS });
    },

    SessionState: (_req, now) => {
      const active = sessionOpen(now);
      return okReply({
        active,
        expiresMs: active ? state.sessionExpiresMs : 0,
        remainingS: active ? Math.ceil((state.sessionExpiresMs - now) / 1000) : 0,
      });
    },

    ExtendSession: (_req, now) => {
      touch(now);
      return okReply({ active: true, expiresMs: state.sessionExpiresMs, remainingS: state.sessionIdleS });
    },

    Logout: () => {
      state.sessionActive = false;
      state.sessionExpiresMs = 0;
      return okReply();
    },

    GetToday: (_req, now) =>
      okReply({ children: state.children.map((c) => card(c, now)), notices: [...state.notices] }),

    GetChildRules: (req) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      return okReply({ childId: child.childId, name: child.name, limits: limitsWithGames(child.limits), endMode: child.endMode ?? "lock", break: child.break ?? DEFAULT_BREAK });
    },

    SaveLimits: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      if (req.endMode && req.endMode !== "lock" && req.endMode !== "study_only") {
        return fail("Выберите, что делать, когда время вышло", "endMode");
      }
      if (req.break) {
        // Пределы как у службы (handlers_0_3_2.go): первая ошибка с полем.
        const bad = Object.entries(validateBreak(req.break as BreakRules))[0];
        if (bad) return fail(bad[1], bad[0]);
      }
      const onlyMode = !(req.limits ?? []).length && Boolean(req.endMode || req.break);
      if (!onlyMode) {
        const bad = limitsError(req.limits ?? []);
        if (bad) return bad;
        child.limits = limitsWithGames(req.limits);
        addEvent(now, child, "settings", `${child.name}: лимиты изменены`);
      }
      if (req.endMode && req.endMode !== (child.endMode ?? "lock")) {
        child.endMode = req.endMode;
        addEvent(now, child, "settings", `${child.name}: изменён режим, когда время вышло`);
      }
      if (req.break && !breakEqual(req.break as BreakRules, child.break ?? DEFAULT_BREAK)) {
        child.break = { ...(req.break as BreakRules), message: (req.break as BreakRules).message.trim() };
        addEvent(now, child, "settings", `${child.name}: изменены настройки отдыха`);
      }
      return okReply({
        rules: { childId: child.childId, name: child.name, limits: limitsWithGames(child.limits), endMode: child.endMode ?? "lock", break: child.break ?? DEFAULT_BREAK },
        card: card(child, now),
        message: "Сохранено. Новые правила уже действуют",
      });
    },

    GetSchedule: (req) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      return okReply({ childId: child.childId, intervals: child.schedule ?? [] });
    },

    SaveSchedule: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      for (const iv of (req.intervals ?? []) as Interval[]) {
        if (iv.startMin === iv.endMin) return fail("Начало и конец совпадают", "end");
      }
      child.schedule = (req.intervals ?? []) as Interval[];
      addEvent(now, child, "settings", `${child.name}: запретные часы изменены`);
      return okReply({ intervals: child.schedule, card: card(child, now), message: "Сохранено. Новые правила уже действуют" });
    },

    CopyRules: (req, now) => {
      const from = childById(req.fromChildId);
      const to = childById(req.toChildId);
      if (!from || !to) return fail("Ребёнок не найден");
      void now;
      return okReply({
        limits: limitsWithGames(from.limits),
        intervals: from.schedule ?? [],
        endMode: from.endMode ?? "lock",
        break: from.break ?? DEFAULT_BREAK,
        message: "Правила скопированы. Проверьте и сохраните",
      });
    },

    GetBonus: (req) => {
      // Окно ребёнка («Моя статистика» → «Бонусы») просит свою копилку номером 0 (FB-5).
      const child = childById(req.childId ? req.childId : (state.myChildId ?? -1));
      if (!child) return fail("Ребёнок не найден");
      return okReply({ childId: child.childId, balance: child.bonusBalance ?? 0, entries: child.bonusLedger ?? [] });
    },

    GrantBonus: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const m = req.minutes;
      if (!Number.isInteger(m) || m < 1 || m > 600) return fail("Можно начислить от 1 до 600 минут", "minutes");
      if (!req.reason) return fail("Укажите, за что начисляете", "reason");
      const cur = child.bonusBalance ?? 0;
      if (cur + m > 600) return fail(`Можно начислить не больше ${600 - cur} мин`, "minutes");
      const after = cur + m;
      child.bonusBalance = after;
      child.bonusLedger = [{ atMs: now, tzOffsetMin: localOffsetMin(new Date(now)), deltaMin: m, reason: req.reason, balanceAfter: after }, ...(child.bonusLedger ?? [])];
      const bonusId = `b-${state.nextExtraId++}`;
      state.bonusUndo.set(bonusId, { childId: child.childId, minutes: m, undoUntilMs: now + UNDO_WINDOW_MS });
      addEvent(now, child, "bonus_grant", `${child.name}: начислено +${m} мин: ${req.reason}`);
      return okReply({ balanceAfter: child.bonusBalance, message: `Начислено ${m} ${minutesWord(m)}. Баланс ${child.bonusBalance} мин`, bonusId, undoUntilMs: now + UNDO_WINDOW_MS });
    },

    UndoBonus: (req, now) => {
      const child = childById(req.childId);
      const rec = state.bonusUndo.get(req.bonusId);
      if (!child || !rec || rec.childId !== req.childId) return fail("Отменить уже нельзя");
      if (now > rec.undoUntilMs) return fail("Отменить уже нельзя: прошло больше 10 секунд");
      state.bonusUndo.delete(req.bonusId);
      child.bonusBalance = Math.max(0, (child.bonusBalance ?? 0) - rec.minutes);
      return okReply({ balanceAfter: child.bonusBalance, message: "Начисление отменено" });
    },

    Pause: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      child.pauseUntil = "18:30";
      addEvent(now, child, "pause", `${child.name}: пауза`);
      return okReply({ card: card(child, now), message: "Пауза до 18:30" });
    },

    PauseAll: (_req, now) => {
      for (const c of state.children) c.pauseUntil = "18:30";
      return okReply({ children: state.children.map((c) => card(c, now)), message: "Пауза для всех детей на 1 ч" });
    },

    AddExtra: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const m = req.minutes;
      if (!Number.isInteger(m) || m < 1 || m > 240) {
        return fail("Можно добавить от 1 до 240 минут", "minutes");
      }
      child.extraMin += m;
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, delta: m, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время +${m} мин`);
      return okReply({
        card: card(child, now),
        message: `Добавлено ${m} ${minutesWord(m)}`,
        extraId: extra.id,
        undoUntilMs: extra.undoUntilMs,
      });
    },

    // Убавить доп. время (R10, D-43): суммарный лимит дня не ниже 0.
    ReduceExtra: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const m = req.minutes;
      if (!Number.isInteger(m) || m < 1 || m > 240) {
        return fail("Можно убрать от 1 до 240 минут", "minutes");
      }
      const floor = -todayLimit(child, now).totalMin;
      const newExtra = Math.max(child.extraMin - m, floor);
      const removed = child.extraMin - newExtra;
      child.extraMin = newExtra;
      if (removed === 0) return okReply({ card: card(child, now), message: "Убирать нечего" });
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, delta: -removed, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время −${removed} мин`);
      return okReply({ card: card(child, now), message: `Убрано ${removed} ${minutesWord(removed)}`, extraId: extra.id, undoUntilMs: extra.undoUntilMs });
    },

    // Обнулить доп. время (R10, D-43, «Убрать доп. время»).
    ClearExtra: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const removed = child.extraMin;
      child.extraMin = 0;
      if (removed === 0) return okReply({ card: card(child, now), message: "Доп. времени нет" });
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, delta: -removed, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время убрано`);
      return okReply({ card: card(child, now), message: "Доп. время убрано", extraId: extra.id, undoUntilMs: extra.undoUntilMs });
    },

    UndoExtra: (req, now) => {
      const child = childById(req.childId);
      const extra = state.extras.find((e) => e.id === req.extraId && e.childId === req.childId);
      if (!child || !extra) return fail("Добавленное время не найдено");
      if (now > extra.undoUntilMs) return fail("Отменить уже нельзя: прошло больше 10 секунд");
      child.extraMin -= extra.delta;
      state.extras = state.extras.filter((e) => e !== extra);
      return okReply({ card: card(child, now), message: "Изменение доп. времени отменено" });
    },

    ClearBlock: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      child.blocked = false;
      return okReply({ card: card(child, now), message: "Блокировка снята" });
    },

    ParentStats: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const period = periodArg(req);
      if (!period) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      return okReply(myToday(child, now, period, state.events));
    },

    GetDiag: () => okReply({ verbose: state.verboseLogs, logsPath: MOCK_LOGS_PATH }),

    SetDiag: (req) => {
      state.verboseLogs = Boolean(req.verbose);
      return okReply({ verbose: state.verboseLogs, logsPath: MOCK_LOGS_PATH });
    },

    // Журнал (FR-24, RM5-5): ребёнок, период fromMs <= atMs < toMs, группа «Что», курсор
    // (beforeAtMs, beforeId) строго раньше; новые сверху; more - есть записи старше страницы.
    ListEvents: (req) => {
      const group: string = req.group ?? "";
      const known = Object.hasOwn(MOCK_JOURNAL_GROUPS, group);
      const kinds: readonly string[] | null | undefined =
        group === "" ? null : known ? MOCK_JOURNAL_GROUPS[group as Exclude<JournalGroup, "">] : undefined;
      if (kinds === undefined) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      const limit = Math.min(req.limit > 0 ? req.limit : 200, 1000);
      const before = Number(req.beforeAtMs ?? 0);
      const beforeId = Number(req.beforeId ?? 0);
      const matched = state.events
        .filter((e) => (req.childId ? e.childId === req.childId : true))
        .filter((e) => (req.fromMs ? e.atMs >= req.fromMs : true))
        .filter((e) => (req.toMs ? e.atMs < req.toMs : true))
        .filter((e) => (kinds ? kinds.includes(e.kind) : true))
        .filter((e) => (before ? e.atMs < before || (e.atMs === before && e.id < beforeId) : true))
        .sort((a, b) => b.atMs - a.atMs || b.id - a.id);
      const reply: ListEventsReply = { events: matched.slice(0, limit), more: matched.length > limit };
      return okReply(reply);
    },

    ChangePassword: (req) => {
      if (req.oldPassword !== state.password) return fail("Старый пароль неверный", "oldPassword");
      if ([...String(req.newPassword ?? "")].length < 6) return fail("Не меньше 6 символов", "newPassword");
      state.password = req.newPassword;
      return okReply();
    },

    SetChildren: (req, now) => {
      for (const id of req.remove ?? []) {
        const gone = childById(id);
        state.children = state.children.filter((c) => c.childId !== id);
        if (gone) addEvent(now, undefined, "settings", `${gone.name}: учётка убрана из детских`);
      }
      for (const a of req.add ?? []) {
        const acc = state.accounts.find((x) => x.sid === a.sid);
        if (!acc || acc.admin || !acc.canBeChild) return fail("Эту учётку нельзя сделать детской", "children");
        state.children.push(
          mockChild({ childId: state.nextChildId++, sid: a.sid, name: a.name, usedS: 0, limits: normalizeLimits([]) }),
        );
      }
      for (const r of req.rename ?? []) {
        const c = childById(r.childId);
        if (c) c.name = r.name;
      }
      const children: ChildInfo[] = state.children.map((c) => ({ childId: c.childId, sid: c.sid, name: c.name }));
      return okReply({ children });
    },

    MyToday: (req, now) => {
      const period = periodArg(req);
      if (!period) return fail("Не удалось разобрать запрос. Попробуй ещё раз");
      return okReply(myToday(childById(state.myChildId ?? -1), now, period, state.events));
    },

    FirstRunSeen: () => {
      state.firstRunSeen = true;
      return okReply();
    },

    // --- Фидбек-раунд 0.2.2 ---------------------------------------------------------

    SetNoForbidden: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      child.noForbiddenToday = Boolean(req.on);
      addEvent(now, child, "no_forbidden", `${child.name}: запретные часы сегодня ${req.on ? "сняты" : "снова действуют"}`);
      return okReply({
        card: card(child, now),
        message: req.on ? "Сегодня запретные часы не действуют" : "Запретные часы снова действуют сегодня",
      });
    },

    ClearBonus: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const was = child.bonusBalance ?? 0;
      if (was <= 0) return okReply({ balanceAfter: 0, message: "Копилка уже пуста" });
      child.bonusBalance = 0;
      child.bonusLedger = [{ atMs: now, tzOffsetMin: localOffsetMin(new Date(now)), deltaMin: -was, reason: "Обнулено родителем", balanceAfter: 0 }, ...(child.bonusLedger ?? [])];
      addEvent(now, child, "bonus_clear", `Копилка обнулена родителем (было ${was} мин)`);
      return okReply({ balanceAfter: 0, message: "Копилка обнулена" });
    },

    // «Очистить статистику» (FR-38, RM5-8): прошлые дни и время программ удалены, сегодня - с момента
    // очистки (снимок счётчиков); остаток, доп. время и копилка прежние (D-73).
    ClearStats: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Такого ребёнка нет в списке");
      const status = statusOf(child, now);
      child.history = [];
      child.programsToday = [];
      child.statsReset = {
        clearedAtMs: now,
        usedS: status.usedS,
        gamesS: child.gamesUsedS ?? 0,
        studyS: child.studyUsedS ?? 0,
        videoS: child.videoUsedS ?? 0,
        extraMin: child.extraMin ?? 0,
      };
      addEvent(now, child, "stats_cleared", `${child.name}: статистика очищена`);
      return okReply({ message: `Статистика ${genitiveName(child.name)} очищена` });
    },

    // «Очистить журнал» (FR-38, RM5-9): события ребёнка удалены, события без ребёнка остаются.
    ClearJournal: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Такого ребёнка нет в списке");
      state.events = state.events.filter((e) => e.childId !== child.childId);
      addEvent(now, child, "journal_cleared", `${child.name}: журнал очищен`);
      return okReply({ message: `Журнал ${genitiveName(child.name)} очищен` });
    },

    ReduceBonus: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      if (!Number.isInteger(req.minutes) || req.minutes < 1) return fail("Укажите, на сколько убавить", "minutes");
      const cur = child.bonusBalance ?? 0;
      const take = Math.min(req.minutes, cur);
      if (take <= 0) return okReply({ balanceAfter: 0, message: "Копилка уже пуста" });
      const after = cur - take;
      child.bonusBalance = after;
      child.bonusLedger = [{ atMs: now, tzOffsetMin: localOffsetMin(new Date(now)), deltaMin: -take, reason: "Убавлено родителем", balanceAfter: after }, ...(child.bonusLedger ?? [])];
      addEvent(now, child, "bonus_reduce", `Копилка убавлена родителем на ${take} мин`);
      return okReply({ balanceAfter: after, message: `Убавлено ${take} ${minutesWord(take)}. Баланс ${after} мин` });
    },

    CreatePromo: (req, now) => {
      const raw = String(req.code ?? "").trim();
      if (!raw) return fail("Введите код", "code");
      if (!PROMO_NEW_CODE.test(raw)) return fail("Только латинские буквы A-Z и цифры", "code");
      const code = raw.toUpperCase();
      if (!Number.isInteger(req.minutes) || req.minutes < 1 || req.minutes > 600) return fail("Можно от 1 до 600 минут", "minutes");
      if (state.promos.some((p) => promoNorm(p.codeDisplay) === promoNorm(code))) {
        return fail("Такой код уже есть, придумайте другой", "code");
      }
      const promo: MockPromo = { codeDisplay: code, minutes: req.minutes, createdAtMs: now };
      state.promos.push(promo);
      addEvent(now, undefined, "promo_create", `Создан промокод «${code}» на ${req.minutes} мин`);
      return okReply({ promo: promoView(promo), message: `Промокод «${code}» на ${req.minutes} ${minutesWord(req.minutes)} готов` });
    },

    // Как служба (RM5-11): действующие новые сверху, использованные по моменту использования, новые
    // сверху; позже добавленный при равном моменте - выше; коды заглавными.
    ListPromos: () => {
      const name = (id?: number) => (id ? (childById(id)?.name ?? "") : "");
      const newestFirst = (key: (p: MockPromo) => number) =>
        state.promos
          .map((p, i) => ({ p, i }))
          .sort((a, b) => key(b.p) - key(a.p) || b.i - a.i)
          .map(({ p }) => p);
      return okReply({
        active: newestFirst((p) => p.createdAtMs)
          .filter((p) => !p.usedAtMs)
          .map(promoView),
        used: newestFirst((p) => p.usedAtMs ?? 0)
          .filter((p) => p.usedAtMs)
          .map((p) => ({ ...promoView(p), usedByName: name(p.usedByChild) })),
      });
    },

    // «Придумать коды» (FR-35, RM5-12): пределы у своего поля; коды не совпадают с базой (и с
    // использованными) и между собой. Коды детерминированные (генератор с постоянным началом),
    // чтобы снимки экранов не менялись от прогона к прогону.
    GeneratePromos: (req, now) => {
      const count = Number(req.count);
      const length = Number(req.length);
      const minutes = Number(req.minutes);
      if (!Number.isInteger(count) || count < 1 || count > 50) return fail("Можно от 1 до 50 кодов", "count");
      if (!Number.isInteger(length) || length < 4 || length > 12) return fail("Длина от 4 до 12 знаков", "length");
      const alpha = PROMO_ALPHABETS[String(req.charset)];
      if (!alpha) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      if (!Number.isInteger(minutes) || minutes < 1 || minutes > 600) return fail("Можно от 1 до 600 минут", "minutes");
      const taken = new Set(state.promos.map((p) => promoNorm(p.codeDisplay)));
      const codes: string[] = [];
      let seed = state.promoSeed;
      for (let attempt = 0; attempt < count * 100 && codes.length < count; attempt++) {
        let code = "";
        for (let i = 0; i < length; i++) {
          seed = (seed * 48271) % 2147483647; // MINSTD: произведение меньше 2^53, без потери точности
          code += alpha[Math.floor(((seed - 1) / 2147483646) * alpha.length)];
        }
        if (taken.has(promoNorm(code))) continue;
        taken.add(promoNorm(code));
        codes.push(code);
      }
      if (codes.length < count) return fail("Не получилось придумать столько разных кодов. Увеличьте длину");
      state.promoSeed = seed;
      const created: MockPromo[] = codes.map((codeDisplay) => ({ codeDisplay, minutes, createdAtMs: now }));
      state.promos.push(...created);
      addEvent(now, undefined, "promo_generate", promoGenerateText(codes.length, minutes));
      return okReply({
        promos: created.map(promoView),
        message: `Готово: ${codes.length} ${pluralRu(codes.length, "код", "кода", "кодов")} на ${minutes} мин`,
      });
    },

    // «Очистить историю» промокодов (RM5-14): использованные удалены, действующие остаются.
    ClearPromoHistory: (_req, now) => {
      const n = state.promos.filter((p) => p.usedAtMs).length;
      state.promos = state.promos.filter((p) => !p.usedAtMs);
      addEvent(now, undefined, "promo_history_cleared", `История промокодов очищена (${n})`);
      return okReply({ message: "История промокодов очищена" });
    },

    RedeemPromo: (req, now) => {
      const child = childById(req.childId ? req.childId : (state.myChildId ?? -1));
      if (!child) return fail("Ребёнок не найден");
      if (!String(req.code ?? "").trim()) return fail("Введите код", "code");
      const promo = state.promos.find((p) => !p.usedAtMs && promoNorm(p.codeDisplay) === promoNorm(req.code));
      if (!promo) return fail("Код не подходит", "code");
      promo.usedAtMs = now;
      promo.usedByChild = child.childId;
      child.extraMin += promo.minutes;
      addEvent(now, child, "promo_redeem", `${child.name}: промокод «${promo.codeDisplay}» +${promo.minutes} мин`);
      return okReply({ card: card(child, now), addedMin: promo.minutes, message: `Добавлено ${promo.minutes} ${minutesWord(promo.minutes)}` });
    },

    // --- Окно ребёнка (D-53): «Попросить ещё», «Взять из копилки», «Настройки для родителя» ---

    AskMore: (req, now) => {
      const paused = pausedReply(now);
      if (paused) return paused;
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      if (!Number.isInteger(req.minutes) || req.minutes < 1 || req.minutes > 240) {
        return fail("Можно от 1 до 240 минут", "minutes");
      }
      if (req.password !== state.password) return deny(now);
      state.attemptsLeft = state.attemptsMax;
      child.extraMin += req.minutes;
      addEvent(now, child, "extra_time", `${child.name}: доп. время +${req.minutes} мин`);
      return okReply({ card: card(child, now), message: `Добавлено ${req.minutes} ${minutesWord(req.minutes)}` });
    },

    SpendBonus: (req, now) => {
      // Только свой сеанс (childSelf): чужой номер службой отклоняется.
      const child = childById(state.myChildId ?? -1);
      if (!child || (req.childId && req.childId !== child.childId)) return fail("Ребёнок не найден");
      const m = req.minutes;
      if (!Number.isInteger(m) || m < 1) return fail("Можно от 1 до 240 минут", "minutes");
      if (child.forbiddenUntil) {
        const at = /(\d{1,2}:\d{2})/.exec(child.forbiddenUntil)?.[1] ?? "";
        return fail(`Из копилки можно взять с ${at}`, "minutes");
      }
      const cur = child.bonusBalance ?? 0;
      if (m > cur) return fail(`В копилке ${cur} мин, столько взять нельзя`, "minutes");
      const after = cur - m;
      child.bonusBalance = after;
      child.extraMin += m;
      child.bonusLedger = [
        { atMs: now, tzOffsetMin: localOffsetMin(new Date(now)), deltaMin: -m, reason: "взято из копилки", balanceAfter: after },
        ...(child.bonusLedger ?? []),
      ];
      addEvent(now, child, "bonus_spend", `${child.name}: из копилки -${m} мин`);
      return okReply({ card: card(child, now), message: `Взято из копилки ${m} мин`, balanceAfter: after });
    },

    OpenWindow: (req) => {
      if (!["parent", "mystats", "firstrun"].includes(req.kind)) return fail("Не получилось открыть окно. Попробуй ещё раз");
      if (req.screen !== undefined && req.screen !== "programs") return fail(examples["OpenWindow#badScreen"].reply.error ?? "");
      state.openedWindows.push(req.screen ? `${req.kind}:${req.screen}` : req.kind);
      return okReply({ opened: true });
    },

    OpenLogs: () => {
      state.openLogsCount += 1;
      return okReply({ path: MOCK_LOGS_PATH, message: "Открываю папку с логами" });
    },

    // --- M3: программы (FR-20) -----------------------------------------------------

    ListPrograms: () => okReply(programsReply()),

    DecideProgram: (req, now) => {
      if (!parentCategory(req.category)) return fail("Выберите: игра, учёба, прочее или запретить", "category");
      const path = (req.path ?? "").toLowerCase();
      const p = state.pending.find((x) => x.path === path);
      const known = state.programs.find((x) => x.path === path);
      // Как служба: ждущая программа или уже в списке, иначе отказ.
      if (!p && !known) return fail("Такой программы нет в списке");
      // Как служба (B-41): та же категория решением родителя — без «Отменить» и без журнала.
      if (!p && known && known.category === req.category && known.source === "parent") {
        return okReply({ message: CATEGORY_UNCHANGED, undoId: "", undoUntilMs: 0, programs: programsReply() });
      }
      const undo = rememberUndo(now);
      const name = p?.name ?? known?.name ?? "";
      if (p) {
        state.pending = state.pending.filter((x) => x !== p);
        state.programs = state.programs.filter((x) => x.path !== p.path);
        state.programs.push({ path: p.path, name: p.name, publisher: p.publisher, category: req.category, source: "parent", weekS: 0 });
      } else if (known) {
        known.category = req.category;
        known.source = "parent";
      }
      addEvent(now, undefined, "program_decided", programDecidedText(name, req.category));
      return okReply({ message: `${name}: ${PROGRAM_CATEGORY_WORD[req.category]}`, ...undo, programs: programsReply() });
    },

    SetProgramsCategory: (req, now) => {
      if (!parentCategory(req.category)) return fail("Выберите: игра, учёба, прочее или запретить", "category");
      const paths = new Set<string>((req.paths ?? []).map((x: string) => x.toLowerCase()));
      if (paths.size > 200) return fail("Можно изменить не больше 200 программ за раз");
      if (!state.programs.some((p) => paths.has(p.path))) return fail("Такой программы нет в списке");
      const changing = state.programs.filter((p) => paths.has(p.path) && (p.category !== req.category || p.source !== "parent"));
      // Как служба (B-41): ничего не изменилось — без «Отменить».
      if (changing.length === 0) return okReply({ message: CATEGORY_UNCHANGED, undoId: "", undoUntilMs: 0, programs: programsReply() });
      const undo = rememberUndo(now);
      for (const p of changing) {
        p.category = req.category;
        p.source = "parent";
      }
      return okReply({ message: `Изменено программ: ${changing.length}`, ...undo, programs: programsReply() });
    },

    SetProgramsHidden: (req, now) => {
      const paths = new Set<string>((req.paths ?? []).map((x: string) => x.toLowerCase()));
      if (paths.size > 200) return fail("Можно изменить не больше 200 программ за раз");
      const hidden = req.hidden === true;
      // Как служба (RF9-5): запрещённую не скрывают; ничего не изменилось — без «Отменить».
      const listed = state.programs.filter((p) => paths.has(p.path) && p.category !== "system");
      const blocked = hidden ? listed.filter((p) => p.category === "blocked").length : 0;
      const known = listed.filter((p) => !(hidden && p.category === "blocked"));
      const blockedText = blocked === 1 ? "Запрещённую программу нельзя скрыть: сначала разрешите её" : "Запрещённые программы нельзя скрыть: сначала разрешите их";
      if (known.length === 0) return fail(blocked > 0 ? blockedText : "Такой программы нет в списке");
      const changed = known.filter((p) => (p.hidden === true) !== hidden);
      const undo = changed.length > 0 ? rememberUndo(now) : { undoId: "", undoUntilMs: 0 };
      for (const p of changed) p.hidden = hidden;
      const one = changed.length === 1 ? changed[0].name : "";
      const word = (n: number) => (n % 10 === 1 && n % 100 !== 11 ? "программу" : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14) ? "программы" : "программ");
      if (changed.length > 0) {
        const subject = one ? `«${one}»` : `${changed.length} ${word(changed.length)}`;
        addEvent(now, undefined, hidden ? "program_hidden" : "program_shown", hidden ? `Вы скрыли ${subject}` : `Вы снова показали ${subject}`);
      }
      let message: string;
      if (changed.length === 0) {
        const single = known.length === 1 ? known[0].name : "";
        message = single ? `${single} ${hidden ? "уже скрыта" : "уже в списке"}` : hidden ? "Эти программы уже скрыты" : "Эти программы уже в списке";
      } else if (one) {
        message = hidden ? `${one} скрыта` : `${one} снова в списке`;
      } else {
        message = hidden ? `Скрыто программ: ${changed.length}` : `Снова в списке программ: ${changed.length}`;
      }
      if (blocked > 0) message += `. ${blockedText}`;
      return okReply({ message, ...undo, programs: programsReply() });
    },

    // Как служба (D-59): своё название 1–60 символов, пустое — снова из файла.
    RenameProgram: (req, now) => {
      const name = String(req.name ?? "").trim();
      if (/[\u0000-\u001f\u007f-\u009f]/.test(name) || Array.from(name).length > 60) return fail("Название до 60 символов", "name");
      const p = state.programs.find((x) => x.path === String(req.path ?? "").toLowerCase() && x.category !== "system");
      if (!p) return fail("Такой программы нет в списке");
      const file = p.fileName || p.name;
      const custom = p.fileName ? p.name : "";
      if (custom === name || (custom === "" && name === p.name)) return okReply({ message: "Название не изменилось", undoId: "", undoUntilMs: 0, programs: programsReply() });
      const undo = rememberUndo(now);
      const from = p.name;
      const to = name || file;
      p.name = to;
      if (to === file) delete p.fileName;
      else p.fileName = file;
      if (from !== to) {
        addEvent(now, undefined, "program_renamed", name ? `Вы переименовали «${from}» в «${to}»` : `Вы вернули «${from}» название из файла «${to}»`);
      }
      return okReply({ message: "Название изменено", ...undo, programs: programsReply() });
    },

    UndoProgram: (req, now) => {
      const rec = state.programUndo.get(req.undoId);
      if (!rec) return fail("Отменить уже нельзя");
      if (now >= rec.undoUntilMs) return fail("Отменить уже нельзя");
      state.programUndo.delete(req.undoId);
      addEvent(now, undefined, "program_undone", "Решение по программам отменено");
      state.programs = rec.programs;
      state.pending = rec.pending;
      return okReply({ message: "Решение отменено", programs: programsReply() });
    },

    // «Сайты» (FR-21) как handlers_m4.go: разбор домена, один домен - один список, предел 1000.
    ListSites: () => okReply(sitesReply()),

    AddSite: (req, now) => {
      // Запрос приходит как any: список сужается проверкой до SiteKind (индекс SITE_LIST_NAMES без any).
      const kind: unknown = req?.kind;
      if (!isSiteKind(kind)) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      const domain = cleanDomain(String(req.domain ?? ""));
      if (!domain) return fail(NOT_DOMAIN_TEXT, "domain");
      const known = state.sites.find((x) => x.domain === domain);
      // Как blockedFull: повтор в «Запрещённых» получает свой отказ, перенос в полный список - отказ о пределе.
      const blockedCount = state.sites.filter((x) => x.kind === "blocked").length;
      if (kind === "blocked" && known?.kind !== "blocked" && blockedCount >= MAX_BLOCKED_SITES) {
        return fail("В списке запрещённых уже 1000 сайтов. Удалите ненужные", "domain");
      }
      if (known && known.kind === kind) return fail(DUPLICATE_TEXT, "domain");
      let message: string;
      if (known) {
        known.kind = kind;
        message = `Сайт перенесён в ${SITE_LIST_NAMES[kind].acc}`;
        addEvent(now, undefined, "site_moved", `Вы перенесли сайт ${domain} в ${SITE_LIST_NAMES[kind].acc}`);
      } else {
        state.sites.push({ domain, kind });
        message = `Добавлено: ${domain}`;
        addEvent(now, undefined, "site_added", `Вы добавили сайт ${domain} в ${SITE_LIST_NAMES[kind].acc}`);
      }
      return okReply({ message, domain, moved: Boolean(known), sites: sitesReply() });
    },

    RemoveSite: (req, now) => {
      const domain = cleanDomain(String(req?.domain ?? ""));
      if (!domain) return fail(NOT_DOMAIN_TEXT, "domain");
      const at = state.sites.findIndex((x) => x.domain === domain);
      if (at < 0) return fail("Этого сайта уже нет в списках");
      const [gone] = state.sites.splice(at, 1);
      addEvent(now, undefined, "site_removed", `Вы удалили сайт ${domain} из ${SITE_LIST_NAMES[gone.kind].gen}`);
      return okReply({
        message: `Удалён ${domain}`,
        domain,
        kind: gone.kind,
        undoUntilMs: now + UNDO_WINDOW_MS,
        sites: sitesReply(),
      });
    },

    // Несколько сайтов за раз (B-49) как handlers_sites_many.go: строки, повтор один раз, предел 1000.
    AddSites: (req, now) => {
      const kind: unknown = req?.kind;
      if (!isSiteKind(kind)) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      const lines = String(req.text ?? "")
        .split("\n")
        .map((l) => l.trim())
        .filter((l) => l !== "");
      if (lines.length > 1000) return fail("Можно вставить не больше 1000 адресов за раз", "domain");
      const rejected: string[] = [];
      const domains: string[] = [];
      for (const line of lines) {
        const d = cleanDomain(line);
        if (!d) rejected.push(line);
        else if (!domains.includes(d)) domains.push(d);
      }
      const rejectedText = (n: number) => (n === 1 ? NOT_DOMAIN_TEXT : "Эти строки не похожи на адрес сайта");
      if (domains.length === 0) return fail(rejectedText(Math.max(rejected.length, 1)), "domain");
      const added: string[] = [];
      let fresh = 0;
      let moved = 0;
      let same = 0;
      let full = 0;
      for (const domain of domains) {
        const known = state.sites.find((x) => x.domain === domain);
        if (known?.kind === kind) {
          same += 1;
          continue;
        }
        if (kind === "blocked" && state.sites.filter((x) => x.kind === "blocked").length >= MAX_BLOCKED_SITES) {
          full += 1;
          continue;
        }
        if (known) {
          known.kind = kind;
          moved += 1;
          addEvent(now, undefined, "site_moved", `Вы перенесли сайт ${domain} в ${SITE_LIST_NAMES[kind].acc}`);
        } else {
          state.sites.push({ domain, kind });
          fresh += 1;
          addEvent(now, undefined, "site_added", `Вы добавили сайт ${domain} в ${SITE_LIST_NAMES[kind].acc}`);
        }
        added.push(domain);
      }
      const parts = [
        fresh > 0 ? `Добавлено сайтов: ${fresh}` : "",
        moved > 0 ? `Перенесено сюда из других списков: ${moved}` : "",
        same > 0 ? `Уже были в списке: ${same}` : "",
        full > 0 ? `Не поместились (${full}): в списке запрещённых уже 1000 сайтов` : "",
      ].filter((p) => p !== "");
      return okReply({
        message: parts.join(". "),
        added,
        rejected,
        fieldError: rejected.length > 0 ? rejectedText(rejected.length) : "",
        sites: sitesReply(),
      });
    },

    RemoveSites: (req, now) => {
      const list: unknown[] = Array.isArray(req?.domains) ? req.domains : [];
      if (list.length === 0 || list.length > 1000) return fail("Не удалось разобрать запрос. Попробуйте ещё раз");
      const removed: SiteDTO[] = [];
      for (const raw of list) {
        const domain = cleanDomain(String(raw));
        const at = domain ? state.sites.findIndex((x) => x.domain === domain) : -1;
        if (at < 0) continue;
        const [gone] = state.sites.splice(at, 1);
        removed.push({ domain, kind: gone.kind });
        addEvent(now, undefined, "site_removed", `Вы удалили сайт ${domain} из ${SITE_LIST_NAMES[gone.kind].gen}`);
      }
      if (removed.length === 0) return fail("Этого сайта уже нет в списках");
      return okReply({
        message: removed.length === 1 ? `Удалён ${removed[0].domain}` : `Удалено сайтов: ${removed.length}`,
        removed,
        undoUntilMs: now + UNDO_WINDOW_MS,
        sites: sitesReply(),
      });
    },

    SetAdultFilter: (req, now) => {
      const child = childById(req?.childId);
      if (!child) return fail("Такого ребёнка нет в списке");
      child.adultFilter = Boolean(req.on);
      const text = `${child.name}: ${child.adultFilter ? "сайты 18+ блокируются" : "сайты 18+ не блокируются"}`;
      addEvent(now, child, "site_adult_filter", text);
      return okReply({ message: text, sites: sitesReply() });
    },

    GetBonusRate: () => okReply({ minutes: state.bonusRate }),

    SetBonusRate: (req) => {
      if (!Number.isInteger(req.minutes) || req.minutes < 0 || req.minutes > 600) return fail("Можно от 0 до 600 минут", "minutes");
      state.bonusRate = req.minutes;
      return okReply({ minutes: state.bonusRate });
    },
  };

  const run = async (command: string, payloadJson: string): Promise<string> => {
    let payload: unknown = {};
    try {
      payload = payloadJson ? JSON.parse(payloadJson) : {};
    } catch {
      payload = {};
    }
    calls.push({ command, payload });
    await Promise.resolve();

    if (state.linkDown) {
      throw new Error("open \\\\.\\pipe\\PCBlock: The system cannot find the file specified.");
    }
    const ov = overrides.get(command);
    if (ov && ov.left > 0) {
      ov.left -= 1;
      if (ov.spec === "reject") throw new Error("ipc: connection reset");
      if (ov.spec === "garbage") return "это не JSON";
      const reply = typeof ov.spec === "function" ? await ov.spec(payload) : ov.spec;
      return JSON.stringify(reply);
    }

    const now = Date.now();
    // Окно ребёнка читает свою копилку номером 0 — без родительской сессии (FB-5).
    const childOwnBonus = command === "GetBonus" && !(payload as { childId?: number }).childId;
    if (PARENT_COMMANDS.has(command) && !childOwnBonus) {
      if (!sessionOpen(now)) {
        return JSON.stringify(fail("Сессия закончилась. Введите пароль"));
      }
      if (command !== "Logout") touch(now);
    }
    const handler = handlers[command];
    if (handler) return JSON.stringify(handler(payload, now));

    // Команды, которые окну родителя не нужны, отдают пример из контракта.
    const example = examples[command];
    if (example) return JSON.stringify(example.reply);
    return JSON.stringify(fail("Неизвестная команда"));
  };

  return {
    state,
    calls,
    call: run,
    override(command, spec, times = Number.POSITIVE_INFINITY) {
      overrides.set(command, { spec, left: times });
    },
    count: (command) => calls.filter((c) => c.command === command).length,
    handler: (command) => (request) => handlers[command](request, Date.now()),
  };
}

/** Ставит window.go.ui.App подменной программы. Возвращает её для проверок в тесте. */
export function installBackend(opts: MockOptions = {}, target: Window = window): MockBackend {
  const backend = createBackend(opts);
  target.go = {
    ui: {
      App: {
        Call: (command, payload) => backend.call(command, payload),
        Mode: () => Promise.resolve(backend.state.mode),
        StartScreen: () => Promise.resolve(backend.state.startScreen),
        Version: () => Promise.resolve(backend.state.version),
        Finish: (code) => {
          backend.state.finishCode = code;
          return Promise.resolve();
        },
      },
    },
  };
  return backend;
}

/** Убирает подмену (для afterEach). */
export function uninstallBackend(target: Window = window): void {
  delete target.go;
}
