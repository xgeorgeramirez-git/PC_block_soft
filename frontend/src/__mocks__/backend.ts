// Подменная программа для тестов и снимков экранов: ставит window.go.ui.App.{Call, Mode, Version, Finish}
// и отвечает на команды так же, как служба (форматы - internal/ipc/commands.go, примеры -
// internal/ipc/testdata/contract.json). Состояние настраивается: настроена ли программа, дети,
// лимиты, журнал, попытки пароля, родительская сессия.
//
// Файл собирается и в браузерный сценарий Playwright (e2e/helpers.ts): экспорты доступны
// там как PCBlockMock.installBackend(...).
import contract from "../../../internal/ipc/testdata/contract.json";
import { formatDuration, formatLimit, localOffsetMin, weekdayOfMs, weekdayShort } from "../lib/format";
import { normalizeLimits } from "../lib/limits";
import type {
  Account,
  BonusEntry,
  ChildInfo,
  ChildStatus,
  Interval,
  JournalEntry,
  MyRule,
  MyTodayReply,
  Reply,
  TodayCard,
  WeekdayLimit,
} from "../types";

export const DEFAULT_PASSWORD = "секрет-123";
/** Версия программы по умолчанию: одна на всех экранах (настоящий номер поднимает оркестратор). */
export const DEFAULT_VERSION = "0.0.1";
const UNDO_WINDOW_MS = 10_000;
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
  events?: JournalEntry[];
  notices?: string[];
  /** Содержимое состояния Помощника. */
  helper?: Record<string, unknown>;
  /**
   * Чей сеанс у окна ребёнка (MyToday): childId; null - сеанс не ребёнка (родитель в своей учётке).
   * По умолчанию - первый ребёнок.
   */
  myChildId?: number | null;
  /** Связи с программой нет: любой вызов Call отклоняется как сбой моста. */
  linkDown?: boolean;
  /** Промокоды на старте (R26). */
  promos?: MockPromo[];
}

export interface MockState {
  mode: string;
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
  helper: Record<string, unknown>;
  myChildId: number | null;
  /** Ребёнок нажал «Понятно» в окне первого входа (FirstRunSeen). */
  firstRunSeen: boolean;
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
  /** Настройка автобонуса N (R25, D-49). */
  bonusRate: number;
  /** Сколько раз просили открыть папку логов (FR-25, D-48). */
  openLogsCount: number;
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
}

type Examples = Record<string, { request?: unknown; reply: Reply }>;
const examples = contract as unknown as Examples;

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

function todayLimit(child: MockChild, nowMs: number): WeekdayLimit {
  const wd = weekdayOfMs(nowMs, localOffsetMin(new Date(nowMs)));
  return normalizeLimits(child.limits)[wd - 1];
}

/** Статус ребёнка: слова и числа собираются в одном месте, как это делает служба. */
export function statusOf(child: MockChild, nowMs: number): ChildStatus {
  const base = {
    name: child.name,
    day: isoDay(nowMs),
    usedS: child.usedS,
    detail: "",
    until: "",
    pauseUntil: child.pauseUntil ?? "",
    bonusBalance: child.bonusBalance ?? 0,
    forbiddenNote: "",
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
    };
  }
  const limit = todayLimit(child, nowMs);
  const limitMin = limit.totalMin + child.extraMin;
  const limitS = limit.unlimited ? 0 : limitMin * 60;
  const usedText = limit.unlimited
    ? `Сегодня: ${formatDuration(child.usedS)}`
    : `Сегодня: ${formatDuration(child.usedS)} из ${formatLimit(limitMin)}`;

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
    };
  }
  const left = formatDuration(remainingS);
  const low = remainingS <= 15 * 60;
  return {
    ...common,
    status: low ? "low" : "ok",
    statusText: low ? `Осталось ${left}` : "Можно",
    label: String(Math.floor(remainingS / 60)),
    tooltip: `Осталось ${left}`,
    summary: `Сегодня осталось ${left} из ${formatLimit(limitMin)}`,
    lock: "none",
  };
}

function cardOf(child: MockChild, nowMs: number): TodayCard {
  return {
    childId: child.childId,
    sid: child.sid,
    name: child.name,
    status: statusOf(child, nowMs),
    extraMin: child.extraMin,
    bonusBalance: child.bonusBalance ?? 0,
    canClearBlock: false,
    noForbiddenToday: Boolean(child.noForbiddenToday),
  };
}

/** «Мои правила» текстом, как их собирает служба: «2 ч», «Без лимита», «Нельзя». */
function myRules(child: MockChild): MyRule[] {
  return normalizeLimits(child.limits).map((l) => ({
    weekday: l.weekday,
    day: weekdayShort(l.weekday),
    text: l.unlimited ? "Без лимита" : l.totalMin === 0 ? "Нельзя" : formatLimit(l.totalMin),
  }));
}

/** Ответ MyToday ребёнка: строки «Сегодня» и правила. Не ребёнок - пустые списки (не null). */
function myToday(child: MockChild | undefined, nowMs: number): MyTodayReply {
  if (!child) {
    return examples["MyToday#empty"].reply.payload as MyTodayReply;
  }
  const status = statusOf(child, nowMs);
  const today = [status.summary, `Использовано ${formatDuration(status.usedS)}`];
  if (status.lock === "timeUp" && status.until) {
    today.push(status.until);
  }
  return { status, today, rules: myRules(child) };
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
  "GetHelperState",
  "SaveHelperState",
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
]);

/** Нормализация промокода как в ядре (R26): без пробелов по краям и без учёта регистра. */
const promoNorm = (code: string) => code.trim().toLowerCase();

function makeState(opts: MockOptions): MockState {
  const configured = opts.configured ?? true;
  const children = opts.children ?? (configured ? [mockChild()] : []);
  const attempts = opts.attempts ?? 5;
  return {
    mode: opts.mode ?? "window",
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
    events: opts.events ?? [],
    notices: opts.notices ?? [],
    helper: opts.helper ?? {},
    myChildId: opts.myChildId === undefined ? (children[0]?.childId ?? null) : opts.myChildId,
    firstRunSeen: false,
    finishCode: null,
    linkDown: opts.linkDown ?? false,
    nextChildId: Math.max(0, ...children.map((c) => c.childId)) + 1,
    nextExtraId: 17,
    extras: [],
    bonusUndo: new Map(),
    verboseLogs: false,
    promos: opts.promos ?? [],
    bonusRate: 5,
    openLogsCount: 0,
  };
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
      atMs: now,
      tzOffsetMin: localOffsetMin(new Date(now)),
      childId: child?.childId ?? 0,
      childName: child?.name ?? "",
      kind,
      text,
    });
  };
  const childById = (id: number) => state.children.find((c) => c.childId === id);

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
      state.password = req.password;
      state.configured = true;
      state.children = req.children.map((c: { sid: string; name: string; limits: WeekdayLimit[] }) =>
        mockChild({ childId: state.nextChildId++, sid: c.sid, name: c.name, limits: c.limits, usedS: 0 }),
      );
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
      okReply({ children: state.children.map((c) => cardOf(c, now)), notices: [...state.notices] }),

    GetChildRules: (req) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      return okReply({ childId: child.childId, name: child.name, limits: normalizeLimits(child.limits) });
    },

    SaveLimits: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const bad = limitsError(req.limits ?? []);
      if (bad) return bad;
      child.limits = normalizeLimits(req.limits);
      addEvent(now, child, "settings", `${child.name}: лимиты изменены`);
      return okReply({
        rules: { childId: child.childId, name: child.name, limits: child.limits },
        card: cardOf(child, now),
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
      return okReply({ intervals: child.schedule, card: cardOf(child, now), message: "Сохранено. Новые правила уже действуют" });
    },

    CopyRules: (req, now) => {
      const from = childById(req.fromChildId);
      const to = childById(req.toChildId);
      if (!from || !to) return fail("Ребёнок не найден");
      void now;
      return okReply({ limits: normalizeLimits(from.limits), intervals: from.schedule ?? [], message: "Правила скопированы. Проверьте и сохраните" });
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
      return okReply({ card: cardOf(child, now), message: "Пауза до 18:30" });
    },

    PauseAll: (_req, now) => {
      for (const c of state.children) c.pauseUntil = "18:30";
      return okReply({ children: state.children.map((c) => cardOf(c, now)), message: "Пауза для всех детей на 1 ч" });
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
        card: cardOf(child, now),
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
      if (removed === 0) return okReply({ card: cardOf(child, now), message: "Убирать нечего" });
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, delta: -removed, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время −${removed} мин`);
      return okReply({ card: cardOf(child, now), message: `Убрано ${removed} ${minutesWord(removed)}`, extraId: extra.id, undoUntilMs: extra.undoUntilMs });
    },

    // Обнулить доп. время (R10, D-43, «Убрать доп. время»).
    ClearExtra: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const removed = child.extraMin;
      child.extraMin = 0;
      if (removed === 0) return okReply({ card: cardOf(child, now), message: "Доп. времени нет" });
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, delta: -removed, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время убрано`);
      return okReply({ card: cardOf(child, now), message: "Доп. время убрано", extraId: extra.id, undoUntilMs: extra.undoUntilMs });
    },

    UndoExtra: (req, now) => {
      const child = childById(req.childId);
      const extra = state.extras.find((e) => e.id === req.extraId && e.childId === req.childId);
      if (!child || !extra) return fail("Добавленное время не найдено");
      if (now > extra.undoUntilMs) return fail("Отменить уже нельзя: прошло больше 10 секунд");
      child.extraMin -= extra.delta;
      state.extras = state.extras.filter((e) => e !== extra);
      return okReply({ card: cardOf(child, now), message: "Изменение доп. времени отменено" });
    },

    ClearBlock: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      child.blocked = false;
      return okReply({ card: cardOf(child, now), message: "Блокировка снята" });
    },

    ParentStats: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      return okReply(myToday(child, now));
    },

    GetDiag: () => okReply({ verbose: state.verboseLogs }),

    SetDiag: (req) => {
      state.verboseLogs = Boolean(req.verbose);
      return okReply({ verbose: state.verboseLogs });
    },

    ListEvents: (req) => {
      const limit = Math.min(req.limit > 0 ? req.limit : 200, 500);
      const events = state.events
        .filter((e) => (req.childId ? e.childId === req.childId : true))
        .filter((e) => (req.fromMs ? e.atMs >= req.fromMs : true))
        .filter((e) => (req.toMs ? e.atMs <= req.toMs : true))
        .sort((a, b) => b.atMs - a.atMs)
        .slice(0, limit);
      return okReply({ events });
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

    MyToday: (_req, now) => okReply(myToday(childById(state.myChildId ?? -1), now)),

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
        card: cardOf(child, now),
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
      const code = String(req.code ?? "").trim();
      if (!code) return fail("Введите код", "code");
      if (!Number.isInteger(req.minutes) || req.minutes < 1 || req.minutes > 600) return fail("Можно от 1 до 600 минут", "minutes");
      if (state.promos.some((p) => promoNorm(p.codeDisplay) === promoNorm(code))) {
        return fail("Такой код уже есть, придумайте другой", "code");
      }
      const promo: MockPromo = { codeDisplay: code, minutes: req.minutes, createdAtMs: now };
      state.promos.push(promo);
      addEvent(now, undefined, "promo_create", `Создан промокод «${code}» на ${req.minutes} мин`);
      return okReply({ promo, message: `Промокод «${code}» на ${req.minutes} ${minutesWord(req.minutes)} готов` });
    },

    ListPromos: () => {
      const name = (id?: number) => (id ? (childById(id)?.name ?? "") : "");
      return okReply({
        active: state.promos.filter((p) => !p.usedAtMs),
        used: state.promos.filter((p) => p.usedAtMs).map((p) => ({ ...p, usedByName: name(p.usedByChild) })),
      });
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
      return okReply({ card: cardOf(child, now), addedMin: promo.minutes, message: `Добавлено ${promo.minutes} ${minutesWord(promo.minutes)}` });
    },

    OpenLogs: () => {
      state.openLogsCount += 1;
      return okReply({ path: "C:\\ProgramData\\PCBlock\\logs", message: "Открываю папку с логами" });
    },

    GetBonusRate: () => okReply({ minutes: state.bonusRate }),

    SetBonusRate: (req) => {
      if (!Number.isInteger(req.minutes) || req.minutes < 0 || req.minutes > 600) return fail("Можно от 0 до 600 минут", "minutes");
      state.bonusRate = req.minutes;
      return okReply({ minutes: state.bonusRate });
    },

    GetHelperState: () => okReply({ state: state.helper }),

    SaveHelperState: (req) => {
      state.helper = req.state ?? {};
      return okReply();
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
