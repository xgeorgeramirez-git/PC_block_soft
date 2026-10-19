// Единая точка обращения интерфейса к программе. Экраны не вызывают window.go напрямую:
// так их проще тестировать с подменой (__mocks__/backend.ts) и так сбои моста превращаются
// в простые слова в одном месте (lib/errors.ts, урок L-112).
//
// Мост даёт три метода: Call(команда, JSON запроса) -> JSON ответа, Mode(), Version().
// Типы запросов и ответов - types.ts (зеркало internal/ipc/commands.go). Массивы в ответах
// приводятся к пустым спискам: Go отдаёт nil-срез как null (урок L-22).
import { CallError, linkError, replyError } from "./lib/errors";
import type {
  Account,
  AddExtraReply,
  CardReply,
  ChildInfo,
  ChildRules,
  GetTodayReply,
  HelloReply,
  JournalEntry,
  ListEventsRequest,
  LoginDenied,
  LoginReply,
  MyTodayReply,
  Reply,
  SaveLimitsReply,
  SessionStateReply,
  SetChildrenRequest,
  SetupChild,
  UninstallCheckReply,
  WeekdayLimit,
  WindowMode,
} from "./types";

function bridge(): PCBlockApp {
  const app = window.go?.ui?.App;
  if (!app) {
    throw linkError();
  }
  return app;
}

/** Вызывает команду программы. Отказ и сбой связи приходят как CallError с текстом для человека. */
export async function call<Req extends object, Resp>(command: string, req?: Req): Promise<Resp> {
  let raw: string;
  try {
    raw = await bridge().Call(command, JSON.stringify(req ?? {}));
  } catch {
    throw linkError();
  }
  let reply: Reply;
  try {
    reply = JSON.parse(raw) as Reply;
  } catch {
    throw linkError();
  }
  if (!reply || typeof reply !== "object" || typeof reply.ok !== "boolean") {
    throw linkError();
  }
  if (!reply.ok) {
    throw replyError(reply);
  }
  return (reply.payload ?? {}) as Resp;
}

/** Разбирает отказ входа: сколько попыток осталось и сколько секунд ввод закрыт. */
export function loginDenied(e: unknown): LoginDenied | null {
  if (!(e instanceof CallError)) {
    return null;
  }
  const p = e.payload as Partial<LoginDenied> | undefined;
  if (p && typeof p.attemptsLeft === "number" && typeof p.pauseS === "number") {
    return { attemptsLeft: p.attemptsLeft, pauseS: p.pauseS };
  }
  return null;
}

async function viaBridge(method: "Mode" | "Version"): Promise<string> {
  try {
    return String(await bridge()[method]());
  } catch {
    throw linkError();
  }
}

/** Режим окна по ключу запуска (internal/launch). */
export async function mode(): Promise<WindowMode> {
  return (await viaBridge("Mode")) as WindowMode;
}

/** Версия программы для показа в окне. */
export function version(): Promise<string> {
  return viaBridge("Version");
}

// --- Общие -----------------------------------------------------------------

export function hello(): Promise<HelloReply> {
  return call("Hello");
}

// --- Мастер и администратор ---------------------------------------------------

export async function listAccounts(): Promise<Account[]> {
  const r = await call<object, { accounts?: Account[] | null }>("ListAccounts");
  return r.accounts ?? [];
}

export async function setup(password: string, children: SetupChild[]): Promise<void> {
  await call("Setup", { password, children });
}

export async function resetPassword(newPassword: string): Promise<void> {
  await call("ResetPassword", { newPassword });
}

export function uninstallCheck(password: string, forgot: boolean): Promise<UninstallCheckReply> {
  return call("UninstallCheck", { password, forgot });
}

// --- Родительская сессия -------------------------------------------------------

export function login(password: string): Promise<LoginReply> {
  return call("Login", { password });
}

export function sessionState(): Promise<SessionStateReply> {
  return call("SessionState");
}

export function extendSession(): Promise<SessionStateReply> {
  return call("ExtendSession");
}

export async function logout(): Promise<void> {
  await call("Logout");
}

export async function getToday(): Promise<GetTodayReply> {
  const r = await call<object, Partial<GetTodayReply>>("GetToday");
  return { children: r.children ?? [], notices: r.notices ?? [] };
}

export async function getChildRules(childId: number): Promise<ChildRules> {
  const r = await call<object, Omit<ChildRules, "limits"> & { limits?: WeekdayLimit[] | null }>("GetChildRules", {
    childId,
  });
  return { ...r, limits: r.limits ?? [] };
}

export async function saveLimits(childId: number, limits: WeekdayLimit[]): Promise<SaveLimitsReply> {
  const r = await call<object, SaveLimitsReply & { rules: { limits?: WeekdayLimit[] | null } }>("SaveLimits", {
    childId,
    limits,
  });
  return { ...r, rules: { ...r.rules, limits: r.rules.limits ?? [] } };
}

export function addExtra(childId: number, minutes: number): Promise<AddExtraReply> {
  return call("AddExtra", { childId, minutes });
}

export function undoExtra(childId: number, extraId: string): Promise<CardReply> {
  return call("UndoExtra", { childId, extraId });
}

export function clearBlock(childId: number): Promise<CardReply> {
  return call("ClearBlock", { childId });
}

export async function listEvents(req: Partial<ListEventsRequest> = {}): Promise<JournalEntry[]> {
  const body: ListEventsRequest = {
    childId: req.childId ?? 0,
    fromMs: req.fromMs ?? 0,
    toMs: req.toMs ?? 0,
    limit: req.limit ?? 0,
  };
  const r = await call<object, { events?: JournalEntry[] | null }>("ListEvents", body);
  return r.events ?? [];
}

export async function changePassword(oldPassword: string, newPassword: string): Promise<void> {
  await call("ChangePassword", { oldPassword, newPassword });
}

export async function setChildren(req: Partial<SetChildrenRequest>): Promise<ChildInfo[]> {
  const body: SetChildrenRequest = { add: req.add ?? [], remove: req.remove ?? [], rename: req.rename ?? [] };
  const r = await call<object, { children?: ChildInfo[] | null }>("SetChildren", body);
  return r.children ?? [];
}

// --- Ребёнок о себе (только свой сеанс, только чтение) ------------------------------

/** «Сегодня» и «Мои правила» ребёнка. Пустые списки приходят как null: приводим к []. */
export async function myToday(): Promise<MyTodayReply> {
  const r = await call<object, Partial<MyTodayReply>>("MyToday");
  return { status: r.status as MyTodayReply["status"], today: r.today ?? [], rules: r.rules ?? [] };
}

/** Ребёнок прочитал окно первого входа: больше его не показывать. */
export async function firstRunSeen(): Promise<void> {
  await call("FirstRunSeen");
}

/**
 * Закрывает окно с кодом выхода (мост Finish, Ruling 14). Вне окна программы, например в
 * браузерных снимках, метода нет, и тогда ничего не происходит.
 */
export async function finish(code: number): Promise<void> {
  const app = window.go?.ui?.App;
  if (!app?.Finish) {
    return;
  }
  try {
    await app.Finish(code);
  } catch {
    // Окно уже закрывается или моста нет: сказать ребёнку нечего.
  }
}

/** Состояние Помощника профиля родителя. Содержимое разбирает каталог Помощника, не окно. */
export async function getHelperState(): Promise<Record<string, unknown>> {
  const r = await call<object, { state?: Record<string, unknown> | null }>("GetHelperState");
  return r.state ?? {};
}

export async function saveHelperState(state: Record<string, unknown>): Promise<void> {
  await call("SaveHelperState", { state });
}
