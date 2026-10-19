// Подменная программа для тестов и снимков экранов: ставит window.go.ui.App.{Call, Mode, Version}
// и отвечает на команды так же, как служба (форматы - internal/ipc/commands.go, примеры -
// internal/ipc/testdata/contract.json). Состояние настраивается: настроена ли программа, дети,
// лимиты, журнал, попытки пароля, родительская сессия.
//
// Файл собирается и в браузерный сценарий Playwright (e2e/helpers.ts): экспорты доступны
// там как PCBlockMock.installBackend(...).
import contract from "../../../internal/ipc/testdata/contract.json";
import { formatDuration, formatLimit, localOffsetMin, weekdayOfMs } from "../lib/format";
import { normalizeLimits } from "../lib/limits";
import type {
  Account,
  ChildInfo,
  ChildStatus,
  JournalEntry,
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
  /** Заблокирован за обход защиты, пока родитель не снимет. */
  blocked: boolean;
  /** Подробность для экрана блокировки при обходе. */
  blockDetail: string;
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
  /** Связи с программой нет: любой вызов Call отклоняется как сбой моста. */
  linkDown?: boolean;
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
  linkDown: boolean;
  nextChildId: number;
  nextExtraId: number;
  extras: { id: string; childId: number; minutes: number; undoUntilMs: number }[];
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
    reason: "",
  };
  const limit = todayLimit(child, nowMs);
  const limitMin = limit.totalMin + child.extraMin;
  const limitS = limit.unlimited ? 0 : limitMin * 60;
  const usedText = limit.unlimited
    ? `Сегодня: ${formatDuration(child.usedS)}`
    : `Сегодня: ${formatDuration(child.usedS)} из ${formatLimit(limitMin)}`;

  if (child.blocked) {
    return {
      ...base,
      status: "bypass",
      statusText: "Заблокирован за обход защиты",
      label: "!",
      tooltip: "Попытка обойти защиту",
      summary: "Попытка обойти защиту",
      remainingS: 0,
      limitS,
      unlimited: limit.unlimited,
      lock: "bypass",
      reason: "Попытка обойти защиту",
      detail: child.blockDetail,
      until: "Можно снова: когда родитель снимет блокировку",
      used: usedText,
    };
  }
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
    canClearBlock: child.blocked,
  };
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
]);

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
    linkDown: opts.linkDown ?? false,
    nextChildId: Math.max(0, ...children.map((c) => c.childId)) + 1,
    nextExtraId: 17,
    extras: [],
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

    AddExtra: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      const m = req.minutes;
      if (!Number.isInteger(m) || m < 1 || m > 240) {
        return fail("Можно добавить от 1 до 240 минут", "minutes");
      }
      child.extraMin += m;
      const extra = { id: `e-${state.nextExtraId++}`, childId: child.childId, minutes: m, undoUntilMs: now + UNDO_WINDOW_MS };
      state.extras.push(extra);
      addEvent(now, child, "extra_time", `${child.name}: доп. время +${m} мин`);
      return okReply({
        card: cardOf(child, now),
        message: `Добавлено ${m} ${minutesWord(m)}`,
        extraId: extra.id,
        undoUntilMs: extra.undoUntilMs,
      });
    },

    UndoExtra: (req, now) => {
      const child = childById(req.childId);
      const extra = state.extras.find((e) => e.id === req.extraId && e.childId === req.childId);
      if (!child || !extra) return fail("Добавленное время не найдено");
      if (now > extra.undoUntilMs) return fail("Отменить уже нельзя: прошло больше 10 секунд");
      child.extraMin = Math.max(0, child.extraMin - extra.minutes);
      state.extras = state.extras.filter((e) => e !== extra);
      return okReply({ card: cardOf(child, now), message: "Добавленное время отменено" });
    },

    ClearBlock: (req, now) => {
      const child = childById(req.childId);
      if (!child) return fail("Ребёнок не найден");
      child.blocked = false;
      addEvent(now, child, "block_cleared", `${child.name}: блокировка снята родителем`);
      return okReply({ card: cardOf(child, now), message: "Блокировка снята" });
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
    if (PARENT_COMMANDS.has(command)) {
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
      },
    },
  };
  return backend;
}

/** Убирает подмену (для afterEach). */
export function uninstallBackend(target: Window = window): void {
  delete target.go;
}
